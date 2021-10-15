// Copyright (C) 2021 Charalampos Mitsakis (go.mitsakis.org/tmpfox)
// Licensed under the EUPL-1.2-or-later
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

func main() {
	runtime.GOMAXPROCS(1)
	var o options
	o.Extensions = make(setOfStrings)
	flag.BoolVar(&o.Help, "h", false, "Print usage")
	flag.BoolVar(&o.License, "license", false, "Licensing information")
	flag.StringVar(&o.ProfilesDir, "dir", filepath.Join(os.TempDir(), appName), "Profiles' directory")
	flag.BoolVar(&o.Keep, "keep", false, "Do not delete profile on exit")
	flag.StringVar(&o.UserJsURL, "userjs", "https://raw.githubusercontent.com/arkenfox/user.js/master/user.js", "user.js download URL")
	flag.Var(&o.Extensions, "ext", "Extension to install in the profile. Use the slug name of the extension as argument. You can find the slug at the last part of the URL of the extension: https://addons.mozilla.org/en-US/firefox/addon/slug/. You can use this option multiple times to download multiple extensions. Additionally the following recommended extensions are downloaded: uBlock Origin, ClearURLs, Simple Temporary Containers")
	flag.BoolVar(&o.ExtensionNoRec, "ext-no-rec", false, "Do not download the recommended extensions (uBlock Origin, ClearURLs, Simple Temporary Containers)")
	flag.Parse()
	if !o.ExtensionNoRec {
		o.Extensions["ublock-origin"] = struct{}{}
		o.Extensions["clearurls"] = struct{}{}
		o.Extensions["simple-temporary-containers"] = struct{}{}
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

	// create directories
	err := os.MkdirAll(o.ProfilesDir, 0700)
	if err != nil {
		return fmt.Errorf("cannot create profiles directory: %s", err)
	}
	profileDirPath, err := os.MkdirTemp(o.ProfilesDir, time.Now().Format("20060102_1504_"))
	if err != nil {
		return fmt.Errorf("cannot create profile directory: %s", err)
	}
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
		if o.UserJsURL != "" {
			log.Printf("downloading user.js %s --> %s", o.UserJsURL, userJsPath)
			err = downloadFile(client, o.UserJsURL, userJsPath)
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
			pageHTML, err := openURLHTML(client, extensionPageURL)
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
			err = downloadFile(client, extensionXpiURL, extensionXpiDownloadPath)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := exec.CommandContext(ctx, "firefox", "--no-remote", "--profile", profileDirPath).Run(); err != nil {
		return fmt.Errorf("firefox execution failed: %s", err)
	}
	return nil
}
