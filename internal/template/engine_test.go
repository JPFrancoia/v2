// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package template // import "miniflux.app/v2/internal/template"

import (
	"bytes"
	"testing"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"
)

// TestParseTemplates verifies that all embedded templates can be parsed
// successfully with the registered function map. This catches any template
// that references an undefined function (e.g. a renamed or removed helper),
// which would otherwise only be discovered at runtime startup.
func TestParseTemplates(t *testing.T) {
	config.Opts = config.NewConfigOptions()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseTemplates() panicked: %v", r)
		}
	}()

	NewEngine("").ParseTemplates()
}

// TestRenderWithoutArticleSummaries checks that lists omit previews and summaries while full articles retain content.
func TestRenderWithoutArticleSummaries(t *testing.T) {
	config.Opts = config.NewConfigOptions()
	engine := NewEngine("")
	engine.ParseTemplates()
	entry := &model.Entry{
		ID:      1,
		Title:   "Article title",
		URL:     "https://example.com/article",
		Content: "<p>Article body preview.</p>",
		Date:    time.Now(),
		Status:  "unread",
		Feed: &model.Feed{
			ID:       1,
			Title:    "Example feed",
			Icon:     &model.FeedIcon{},
			Category: &model.Category{ID: 1, Title: "Example category"},
		},
	}
	user := &model.User{Timezone: "UTC"}
	cases := []struct {
		name     string
		view     string
		fragment string
		data     map[string]any
		body     bool
	}{
		{"list", "unread_entries.html", "item_meta", map[string]any{"entry": entry, "user": user}, false},
		{"shared list", "shared_entries.html", "content", map[string]any{"entries": model.Entries{entry}, "user": user, "pagination": map[string]any{}}, false},
		{"article", "entry.html", "page_header", map[string]any{"entry": entry, "user": user}, false},
		{"public article", "entry.html", "page_header", map[string]any{"entry": entry}, false},
		{"article body", "entry.html", "content", map[string]any{"entry": entry, "user": user}, true},
		{"public article body", "entry.html", "content", map[string]any{"entry": entry}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := engine.templates[tc.view].ExecuteTemplate(&output, tc.fragment, tc.data); err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(output.Bytes(), []byte(`class="item-preview"`)) {
				t.Error("view contains an article preview paragraph")
			}
			if bytes.Contains(output.Bytes(), []byte("Article body preview.")) != tc.body {
				t.Errorf("article content presence does not match full-article view: %v", tc.body)
			}
		})
	}
}
