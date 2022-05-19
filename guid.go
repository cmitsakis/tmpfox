// Copyright (C) 2021 Charalampos Mitsakis (go.mitsakis.org/tmpfox)
// Licensed under the EUPL-1.2-or-later

package main

import (
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
