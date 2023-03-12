// Copyright (C) 2021 Charalampos Mitsakis (go.mitsakis.org/tmpfox)
// Licensed under the EUPL-1.2-or-later

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
)

type extensionMetadata struct {
	Addons struct {
		ByGUID map[string]int
	}
}

type extensionXML struct {
	Scripts []struct {
		ID    string `xml:"id,attr"`
		Inner string `xml:",innerxml"`
	} `xml:"body>script"`
}

func extractGUIDFromHTML(pageHTML []byte) (string, error) {
	// parse HTML to extract JSON
	d := xml.NewDecoder(bytes.NewReader(pageHTML))
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity
	var x extensionXML
	err := d.Decode(&x)
	if err != nil {
		return "", fmt.Errorf("decode failed: %v", err)
	}
	var jsonString string
	for _, script := range x.Scripts {
		if script.ID == "redux-store-state" {
			jsonString = script.Inner
			break
		}
	}
	if jsonString == "" {
		return "", fmt.Errorf("jsonString is empty")
	}

	// parse JSON to extract GUID
	var m extensionMetadata
	err = json.Unmarshal([]byte(jsonString), &m)
	if err != nil {
		return "", fmt.Errorf("json unmarshal failed: %s", err)
	}
	for key := range m.Addons.ByGUID {
		return key, nil
	}
	return "", fmt.Errorf("not found")
}

type extensionManifest struct {
	BrowserSpecificSettings struct {
		Gecko struct {
			ID string
		}
	} `json:"browser_specific_settings"`
	Applications struct {
		Gecko struct {
			ID string
		}
	}
}

func extractGUIDFromXPI(xpiPath string) (string, error) {
	rc, err := zip.OpenReader(xpiPath)
	if err != nil {
		return "", fmt.Errorf("failed to open XPI file: %s", err)
	}
	defer rc.Close()
	for _, f := range rc.File {
		if f.Name != "manifest.json" {
			continue
		}
		guid, err := extractGUIDFromZipFileManifest(f)
		if err != nil {
			return "", fmt.Errorf("failed to extract GUID from manifest.json: %s", err)
		}
		return guid, nil
	}
	return "", fmt.Errorf("manifest.json file not found")
}

func extractGUIDFromZipFileManifest(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("failed to open file in ZIP: %s", err)
	}
	defer rc.Close()
	var m extensionManifest
	err = json.NewDecoder(rc).Decode(&m)
	if err != nil {
		return "", fmt.Errorf("failed to decode JSON: %s", err)
	}
	id := m.BrowserSpecificSettings.Gecko.ID
	if id == "" {
		id = m.Applications.Gecko.ID
	}
	if id == "" {
		return "", fmt.Errorf("ID not found")
	}
	return id, nil
}
