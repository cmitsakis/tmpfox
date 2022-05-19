// Copyright (C) 2021 Charalampos Mitsakis (go.mitsakis.org/tmpfox)
// Licensed under the EUPL-1.2-or-later

package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const appName = "tmpfox"

type setOfStrings map[string]struct{}

func (s *setOfStrings) String() string {
	return ""
}

func (s *setOfStrings) Set(v string) error {
	(*s)[v] = struct{}{}
	return nil
}

type options struct {
	Help           bool
	License        bool
	ProfilesDir    string
	Keep           bool
	UserJsURL      string
	Extensions     setOfStrings
	ExtensionNoRec bool
}

const notSetUserJsURL = "matching arkenfox version"

func main() {
	runtime.GOMAXPROCS(1)
	var o options
	o.Extensions = make(setOfStrings)
	flag.BoolVar(&o.Help, "h", false, "Print usage")
	flag.BoolVar(&o.License, "license", false, "Licensing information")
	flag.StringVar(&o.ProfilesDir, "dir", filepath.Join(os.TempDir(), appName), "Profiles' directory")
	flag.BoolVar(&o.Keep, "keep", false, "Do not delete profile on exit")
	flag.StringVar(&o.UserJsURL, "userjs", notSetUserJsURL, "user.js download URL. If not set, download an arkenfox version matching the installed firefox version. If set to empty, do not download user.js.")
	flag.Var(&o.Extensions, "ext", "Extension to install in the profile. Use the slug name of the extension as argument. You can find the slug at the last part of the URL of the extension: https://addons.mozilla.org/en-US/firefox/addon/slug/. You can use this option multiple times to download multiple extensions. Additionally the following recommended extensions are downloaded: uBlock Origin, ClearURLs, Simple Temporary Containers, Bypass Twitter login wall")
	flag.BoolVar(&o.ExtensionNoRec, "ext-no-rec", false, "Do not download the recommended extensions (uBlock Origin, ClearURLs, Simple Temporary Containers, Bypass Twitter login wall)")
	flag.Parse()
	if !o.ExtensionNoRec {
		o.Extensions["ublock-origin"] = struct{}{}
		o.Extensions["clearurls"] = struct{}{}
		o.Extensions["simple-temporary-containers"] = struct{}{}
		o.Extensions["bypass-twitter-login-wall"] = struct{}{}
	}
	if err := run(o); err != nil {
		log.Printf("fatal error: %s\n", err)
		os.Exit(1)
	}
}

func run(o options) error {
	if o.Help {
		flag.PrintDefaults()
		return nil
	}
	if o.License {
		fmt.Printf("%s\n\n[Third party licenses]\n\n%s\n", license, strings.Join(licenseDeps, "\n"))
		return nil
	}

	// cleanup
	profileName, err := randomProfileName()
	if err != nil {
		return fmt.Errorf("randomProfileName() failed: %s", err)
	}
	profileDirPath := filepath.Join(o.ProfilesDir, time.Now().Format("20060102_1504_")+profileName)
	profileCreated := false
	defer func() {
		// delete profile if keep flag is enabled, or the profile has not been created successfully
		if !o.Keep || !profileCreated {
			err := os.RemoveAll(profileDirPath)
			if err != nil {
				log.Printf("failed to delete profile at %s - error: %s", profileDirPath, err)
				return
			}
			log.Printf("deleted profile at %s", profileDirPath)
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)
	go func() {
		<-signals
		cancel()
	}()

	// create directories
	err = os.MkdirAll(o.ProfilesDir, 0700)
	if err != nil {
		return fmt.Errorf("cannot create profiles directory: %s", err)
	}
	err = os.Mkdir(profileDirPath, 0700)
	if err != nil {
		return fmt.Errorf("cannot create profile directory: %s", err)
	}
	profileExtensionsDirPath := filepath.Join(profileDirPath, "extensions")
	err = os.MkdirAll(profileExtensionsDirPath, 0700)
	if err != nil {
		return fmt.Errorf("cannot create extensions directory: %s", err)
	}

	if err = func() error {
		// create HTTP client
		tr := &http.Transport{}
		defer tr.CloseIdleConnections()
		client := &http.Client{
			Transport: tr,
			Timeout:   30 * time.Second,
		}

		// download user.js file
		userJsPath := filepath.Join(profileDirPath, "user.js")
		if o.UserJsURL == notSetUserJsURL {
			// if flag -userjs is not set, download an arkenfox version matching the installed firefox version

			// find installed firefox version
			output, err := exec.CommandContext(ctx, "firefox", "--version").Output()
			if err != nil {
				return fmt.Errorf("failed to run command 'firefox --version': %s", err)
			}
			r := regexp.MustCompile(`Mozilla Firefox ([0-9]+)\.[0-9]+\.[0-9]+`)
			matches := r.FindSubmatch(output)
			if len(matches) < 2 {
				return fmt.Errorf("regular expression failed to find matches on the output of command 'firefox --version'")
			}
			firefoxVersionMajor, err := strconv.Atoi(string(matches[1]))
			if err != nil {
				return fmt.Errorf("failed to identify firefox version on the output of command 'firefox --version': %s", err)
			}

			// query github for all releases of arkenfox
			tagsJSON, err := openURLHTML(ctx, client, fmt.Sprintf("https://api.github.com/repos/arkenfox/user.js/git/matching-refs/tags/%v.", firefoxVersionMajor))
			if err != nil {
				return fmt.Errorf("failed to query github: %s", err)
			}
			var tags []githubTag
			if err := json.Unmarshal(tagsJSON, &tags); err != nil {
				return fmt.Errorf("failed to unmarshal github response: %s", err)
			}
			if len(tags) == 0 {
				log.Println("no matching arkefox version found. downloading latest.")
				err = downloadFile(ctx, client, "https://raw.githubusercontent.com/arkenfox/user.js/master/user.js", userJsPath)
				if err != nil {
					return fmt.Errorf("failed to download user.js: %s", err)
				}
			} else {
				var choosenTag githubTag
				if len(tags) == 1 {
					choosenTag = tags[0]
				} else {
					// if multiple matching tags have been found, choose the one with the highest minor version
					var maxMinor int
					for _, tag := range tags {
						major, minor, err := tag.VersionMajorMinor()
						if err != nil {
							continue
						}
						if major != firefoxVersionMajor {
							continue
						}
						if minor > maxMinor {
							maxMinor = minor
							choosenTag = tag
						}
					}
				}
				versionString, err := choosenTag.VersionString()
				if err != nil {
					return fmt.Errorf("invalid tag version")
				}
				zipURL := fmt.Sprintf("https://github.com/arkenfox/user.js/archive/refs/tags/%s.zip", versionString)
				fmt.Println("url", zipURL)
				zipPath := filepath.Join(profileDirPath, "arkenfox.zip")
				err = downloadFile(ctx, client, zipURL, zipPath)
				if err != nil {
					return fmt.Errorf("failed to download user.js: %s", err)
				}
				defer func() {
					err := os.Remove(zipPath)
					if err != nil {
						log.Printf("failed to delete arkenfox zip file at %s - error: %s", zipPath, err)
						return
					}
				}()
				zipReadCloser, err := zip.OpenReader(zipPath)
				if err != nil {
					return fmt.Errorf("failed to open zip: %s", err)
				}
				defer zipReadCloser.Close()
				for _, fileInZip := range zipReadCloser.File {
					if fileInZip.Name == fmt.Sprintf("user.js-%s/user.js", versionString) {
						fo, err := fileInZip.Open()
						if err != nil {
							return fmt.Errorf("failed to open file in zip: %s", err)
						}
						defer fo.Close()
						userJsFile, err := os.Create(userJsPath)
						if err != nil {
							return fmt.Errorf("failed to create file: %s", err)
						}
						_, err = io.Copy(userJsFile, fo)
						if err != nil {
							return fmt.Errorf("failed to copy file from zip: %s", err)
						}
						break
					}
				}
			}
			// make sure user.js file has been downloaded
			if _, err := os.Stat(userJsPath); err != nil {
				return fmt.Errorf("failed to access user.js file: %s", err)
			}
		} else if o.UserJsURL != "" {
			log.Printf("downloading user.js %s --> %s", o.UserJsURL, userJsPath)
			err = downloadFile(ctx, client, o.UserJsURL, userJsPath)
			if err != nil {
				return fmt.Errorf("failed to download user.js: %s", err)
			}
		}

		// append extra preferences to user.js
		prefs := []string{`user_pref("dom.always_stop_slow_scripts", true);`}
		if len(o.Extensions) > 0 {
			prefsIfExtensions := []string{
				`user_pref("browser.startup.page", 1);`,
				`user_pref("browser.startup.homepage", "about:addons");`,
				`user_pref("extensions.getAddons.showPane", false);`,
				`user_pref("browser.startup.homepage_override.mstone", "ignore");`,
			}
			prefs = append(prefs, prefsIfExtensions...)
		}
		f, err := os.OpenFile(userJsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("failed to open %s - error: %s", userJsPath, err)
		}
		defer f.Close()
		if _, err = f.WriteString(strings.Join(prefs, "\n")); err != nil {
			return fmt.Errorf("failed to write to %s - error: %s", userJsPath, err)
		}

		// download extensions
		for extensionSlug := range o.Extensions {
			extensionPageURL := "https://addons.mozilla.org/en-US/firefox/addon/" + extensionSlug + "/"
			log.Println("visiting", extensionPageURL)
			pageHTML, err := openURLHTML(ctx, client, extensionPageURL)
			if err != nil {
				return fmt.Errorf("cannot open url %s - error: %s", extensionPageURL, err)
			}
			extensionGUID, err := extractGUIDFromHTML(pageHTML)
			if err != nil {
				return fmt.Errorf("failed to extract GUID from html: %s", err)
			}
			extensionXpiURL := "https://addons.mozilla.org/firefox/downloads/latest/" + extensionSlug + "/" + extensionSlug + ".xpi"
			extensionXpiDownloadPath := filepath.Join(profileExtensionsDirPath, extensionGUID+".xpi")
			log.Println("downloading extension", extensionXpiURL, "-->", extensionXpiDownloadPath)
			err = downloadFile(ctx, client, extensionXpiURL, extensionXpiDownloadPath)
			if err != nil {
				return fmt.Errorf("failed to download extension from url %s - error: %s", extensionXpiURL, err)
			}
		}
		return nil
	}(); err != nil {
		return err
	}
	profileCreated = true

	// start firefox
	cmd := exec.CommandContext(ctx, "firefox", "--no-remote", "--profile", profileDirPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("firefox execution failed: %s", err)
	}
	return nil
}

func randomProfileName() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("random number generator failed: %s", err)
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]), nil
}

type githubTag struct {
	Ref string
	URL string
}

func (t githubTag) VersionString() (string, error) {
	r := regexp.MustCompile(`refs/tags/(.+)`)
	matches := r.FindStringSubmatch(t.Ref)
	if len(matches) < 2 {
		return "", fmt.Errorf("failed to parse version of tag")
	}
	return matches[1], nil
}

func (t githubTag) VersionMajorMinor() (int, int, error) {
	r := regexp.MustCompile(`refs/tags/([0-9]+)\.([0-9]+)`)
	matches := r.FindStringSubmatch(t.Ref)
	if len(matches) < 3 {
		return 0, 0, fmt.Errorf("failed to parse version of tag")
	}
	versionMajor, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse version")
	}
	versionMinor, err := strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse version")
	}
	return versionMajor, versionMinor, nil
}
