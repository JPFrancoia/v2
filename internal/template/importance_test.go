// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package template // import "miniflux.app/v2/internal/template"

import (
	"bytes"
	"testing"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestImportantCardTemplates checks highlights across authenticated lists without exposing article content.
func TestImportantCardTemplates(t *testing.T) {
	config.Opts = config.NewConfigOptions()
	engine := NewEngine("")
	engine.ParseTemplates()
	entry := model.NewEntry()
	entry.ID, entry.Title, entry.URL = 1, "Synthetic article", "https://example.test/article"
	entry.Status, entry.Date = model.EntryStatusUnread, time.Now()
	entry.Feed.ID, entry.Feed.Title = 1, "Synthetic feed"
	entry.Feed.Category.ID, entry.Feed.Category.Title = 1, "Synthetic category"
	user := &model.User{Timezone: "UTC"}
	for _, name := range []string{"unread_entries.html", "important_entries.html", "saved_for_later_entries.html", "starred_entries.html", "history_entries.html", "feed_entries.html", "category_entries.html", "tag_entries.html", "user_tag_entries.html", "to_review_entries.html", "search.html", "shared_entries.html"} {
		t.Run(name, func(t *testing.T) {
			for _, important := range []bool{false, true} {
				entry.IsImportant = important
				data := map[string]any{"entries": model.Entries{entry}, "user": user, "pagination": map[string]any{}, "feed": entry.Feed, "category": entry.Feed.Category, "tagName": "Synthetic", "searchQuery": "Synthetic", "tag": &model.UserTag{ID: 1, Title: "Synthetic"}}
				var output bytes.Buffer
				require.NoError(t, engine.templates[name].ExecuteTemplate(&output, "content", data))
				assert.Equal(t, important, bytes.Contains(output.Bytes(), []byte("item-important")))
			}
		})
	}
}

// TestImportantPageLinks checks sorting, page-only read controls, and the selected navigation tab.
func TestImportantPageLinks(t *testing.T) {
	config.Opts = config.NewConfigOptions()
	engine := NewEngine("/reader")
	engine.ParseTemplates()
	entry := model.NewEntry()
	entry.ID, entry.Title, entry.Status = 7, "Synthetic important article", model.EntryStatusUnread
	entry.IsImportant = true
	data := map[string]any{
		"entries": model.Entries{entry}, "user": &model.User{Timezone: "UTC"},
		"pagination": map[string]any{}, "sortOrder": "score", "sortDirection": "desc",
		"scoreSortDirection": "asc", "publishedSortDirection": "desc", "countImportant": 1,
	}
	var output bytes.Buffer
	require.NoError(t, engine.templates["important_entries.html"].ExecuteTemplate(&output, "page_header", data))
	assert.Contains(t, output.String(), `/reader/important?direction=asc&amp;order=score`)
	assert.Contains(t, output.String(), `data-action="markPageAsRead"`)
	assert.NotContains(t, output.String(), "mark-all-as-read")
	output.Reset()
	require.NoError(t, engine.templates["important_entries.html"].ExecuteTemplate(&output, "content", data))
	assert.Contains(t, output.String(), `/reader/important/entry/7?direction=desc&amp;order=score`)
}

// TestImportantArticleReturnLink preserves sorting without adding prediction details to public articles.
func TestImportantArticleReturnLink(t *testing.T) {
	config.Opts = config.NewConfigOptions()
	engine := NewEngine("")
	engine.ParseTemplates()
	var output bytes.Buffer
	data := map[string]any{"importantReturnURL": "/important", "sortOrder": "published_at", "sortDirection": "asc"}
	require.NoError(t, engine.templates["entry.html"].ExecuteTemplate(&output, "entry_pagination", data))
	assert.Contains(t, output.String(), `/important?direction=asc&amp;order=published_at`)
	output.Reset()
	require.NoError(t, engine.templates["entry.html"].ExecuteTemplate(&output, "entry_pagination", map[string]any{}))
	assert.NotContains(t, output.String(), "/important")
	assert.NotContains(t, output.String(), "item-important")
}
