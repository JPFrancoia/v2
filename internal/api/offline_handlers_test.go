// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api // import "miniflux.app/v2/internal/api"

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
)

// TestOfflineSectionValidation rejects invalid pages before database access.
func TestOfflineSectionValidation(t *testing.T) {
	for _, path := range []string{
		"/v1/offline/sections/unread?sort=score&cursor=bad",
		"/v1/offline/sections/saved?sort=score&cursor=unread.score.5.0.12.0",
		"/v1/offline/sections/unread?sort=unknown",
		"/v1/offline/sections/unknown?sort=score",
	} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.SetPathValue("section", r.URL.Path[len("/v1/offline/sections/"):])
			w := httptest.NewRecorder()
			(&handler{}).getOfflineSectionHandler(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", w.Code)
			}
		})
	}
}

// TestOfflineSectionQuery verifies all four filters and both supported sorts.
func TestOfflineSectionQuery(t *testing.T) {
	options, err := config.NewConfigParser().ParseEnvironmentVariables()
	if err != nil {
		t.Fatal(err)
	}
	prior := config.Opts
	config.Opts = options
	t.Cleanup(func() { config.Opts = prior })
	for _, section := range []string{"unread", "saved", "review", "history"} {
		for _, sort := range []string{"score", "newest"} {
			builder := storage.NewEntryQueryBuilder(context.Background(), nil, 42)
			if err := offlineSectionQuery(builder, section, sort, time.Now()); err != nil {
				t.Fatalf("%s %s: %v", section, sort, err)
			}
		}
	}
}

// TestOfflineSectionCursorRoundTrip checks marker fields and stale Review targets.
func TestOfflineSectionCursorRoundTrip(t *testing.T) {
	t.Setenv("TO_REVIEW_RAW_RELEVANCE_TARGET", "50")
	options, err := config.NewConfigParser().ParseEnvironmentVariables()
	if err != nil {
		t.Fatal(err)
	}
	prior := config.Opts
	config.Opts = options
	t.Cleanup(func() { config.Opts = prior })
	entry := model.NewEntry()
	entry.ID = 55
	entry.Score = 25
	raw := 49.875
	entry.RawRelevanceScore = &raw
	entry.Date = time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)
	entry.ChangedAt = entry.Date.Add(time.Hour)
	for _, section := range []string{"unread", "saved", "review", "history"} {
		for _, sort := range []string{"score", "newest"} {
			marker := offlineSectionCursor(entry, section, sort, config.Opts.ToReviewRawRelevanceTarget())
			first, _, id, err := parseOfflineSectionCursor(marker, section, sort)
			if err != nil || id != entry.ID {
				t.Fatalf("invalid %s %s cursor: %q: %v", section, sort, marker, err)
			}
			if section == "review" && sort == "score" && math.Float64frombits(uint64(first)) != 0.125 {
				t.Fatal("cursor lost fractional raw distance")
			}
		}
	}
	if _, _, _, err := parseOfflineSectionCursor("review.score.4.1790668800000000.55.100", "review", "score"); err == nil {
		t.Fatal("accepted a cursor with a stale Review target")
	}
}

// TestOfflineDeviceSnapshotVersion pins the device schema version after score is added.
func TestOfflineDeviceSnapshotVersion(t *testing.T) {
	if got := offlineDeviceSnapshotVersion; got != "2" {
		t.Fatalf("Expected device snapshot version 2, got %q", got)
	}
}

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
	entry.Score = 73
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
		Score:         73,
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
