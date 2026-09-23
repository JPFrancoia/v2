// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api // import "miniflux.app/v2/internal/api"

import (
	"reflect"
	"testing"
	"time"

	"miniflux.app/v2/internal/model"
)

// TestNewOfflineDeviceEntrySnapshot verifies that device snapshots keep the
// article identity, content, feed, category, state, and user tags.
func TestNewOfflineDeviceEntrySnapshot(t *testing.T) {
	publishedAt := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	changedAt := publishedAt.Add(time.Hour)
	entry := model.NewEntry()
	entry.ID = 42
	entry.FeedID = 7
	entry.Feed.ID = 7
	entry.Feed.Title = "Feed"
	entry.Feed.Category.ID = 3
	entry.Feed.Category.Title = "Category"
	entry.Title = "Title"
	entry.Author = "Author"
	entry.URL = "https://example.org/article"
	entry.Date = publishedAt
	entry.ChangedAt = changedAt
	entry.Content = "<p>Content</p>"
	entry.Status = model.EntryStatusUnread
	entry.Starred = true
	entry.SavedForLater = true
	entry.Vote = 1

	expected := model.OfflineDeviceEntrySnapshot{
		EntryID:       42,
		Version:       changedAt,
		FeedID:        7,
		FeedTitle:     "Feed",
		CategoryID:    3,
		CategoryTitle: "Category",
		Title:         "Title",
		Author:        "Author",
		URL:           "https://example.org/article",
		PublishedAt:   publishedAt,
		Content:       "<p>Content</p>",
		Status:        model.EntryStatusUnread,
		Starred:       true,
		SavedForLater: true,
		Vote:          1,
		UserTagIDs:    []int64{5, 9},
	}

	if got := newOfflineDeviceEntrySnapshot(entry, []int64{5, 9}); !reflect.DeepEqual(got, expected) {
		t.Fatalf("Unexpected device snapshot: %#v", got)
	}
}
