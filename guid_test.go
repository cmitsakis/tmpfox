// Copyright (C) 2021 Charalampos Mitsakis (go.mitsakis.org/tmpfox)
// Licensed under the EUPL-1.2-or-later
package main

import (
	"net/http"
	"testing"
	"time"
)

func TestExtractGuidFromHTML(t *testing.T) {
	tr := &http.Transport{}
	defer tr.CloseIdleConnections()
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}
	addonSlug := "ublock-origin"
	addonPageURL := "https://addons.mozilla.org/en-US/firefox/addon/" + addonSlug + "/"
	pageHTML, err := openURLHTML(client, addonPageURL)
	if err != nil {
		t.Fatalf("cannot open url %s - error: %s", addonPageURL, err)
	}
	guid, err := extractGUIDFromHTML(pageHTML)
	if err != nil {
		t.Fatalf("failed: %s\n", err)
	}
	if guid != "uBlock0@raymondhill.net" {
		t.Fatalf("wrong guid: %s", guid)
	}
}
