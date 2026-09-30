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
			marker := offlineSectionCursor(entry, section, sort, config.Opts.ToReviewRawRelevanceTarget(), 0)
			first, _, id, err := parseOfflineSectionCursor(marker, section, sort, 0)
			if err != nil || id != entry.ID {
				t.Fatalf("invalid %s %s cursor: %q: %v", section, sort, marker, err)
			}
			if section == "review" && sort == "score" && math.Float64frombits(uint64(first)) != 0.125 {
				t.Fatal("cursor lost fractional raw distance")
			}
		}
	}
	if _, _, _, err := parseOfflineSectionCursor("review.score.4.1790668800000000.55.100", "review", "score", 0); err == nil {
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

// TestOfflineSectionParameters pins limits, scope isolation, and preparation time validation.
func TestOfflineSectionParameters(t *testing.T) {
	now := time.Unix(1790762400, 0)
	for _, tc := range []struct {
		query string
		limit int
		asOf  int64
	}{
		{"", 10, 0},
		{"limit=1", 1, 0},
		{"limit=20", 20, 0},
		{"scope=offline&limit=20", 20, now.Unix()},
		{"scope=offline&as_of=1790762300", 10, 1790762300},
	} {
		r := httptest.NewRequest(http.MethodGet, "/?"+tc.query, nil)
		r.SetPathValue("section", "unread")
		params, err := parseOfflineSectionParameters(r, now)
		if err != nil || params.limit != tc.limit || params.asOf != tc.asOf {
			t.Fatalf("%q: params=%+v, error=%v", tc.query, params, err)
		}
	}
	for _, query := range []string{
		"limit=0", "limit=21", "limit=-1", "limit=", "limit=no", "limit=9999999999999999999999999",
		"scope=other", "as_of=1790762400", "selection=abc",
		"scope=offline&as_of=", "scope=offline&as_of=0", "scope=offline&as_of=-1",
		"scope=offline&as_of=no", "scope=offline&as_of=1790762401",
		"scope=offline&cursor=anything", "scope=offline&selection=abc",
	} {
		r := httptest.NewRequest(http.MethodGet, "/?"+query, nil)
		r.SetPathValue("section", "unread")
		if _, err := parseOfflineSectionParameters(r, now); err == nil {
			t.Fatalf("accepted invalid parameters %q", query)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/?scope=offline", nil)
	r.SetPathValue("section", "history")
	if _, err := parseOfflineSectionParameters(r, now); err == nil {
		t.Fatal("accepted bulk History")
	}
}

// TestOfflineCursorScope prevents a library cursor from traversing another scope or time window.
func TestOfflineCursorScope(t *testing.T) {
	entry := model.NewEntry()
	entry.ID, entry.Score = 55, 80
	entry.Date = time.Unix(1790762200, 123000)
	asOf := int64(1790762400)
	cursor := offlineSectionCursor(entry, "unread", "newest", 50, asOf)
	if _, _, id, err := parseOfflineSectionCursor(cursor, "unread", "newest", asOf); err != nil || id != entry.ID {
		t.Fatalf("failed scoped cursor round trip: %v", err)
	}
	for _, wrongTime := range []int64{0, asOf - 1, asOf + 1} {
		if _, _, _, err := parseOfflineSectionCursor(cursor, "unread", "newest", wrongTime); err == nil {
			t.Fatalf("accepted cursor from wrong time %d", wrongTime)
		}
	}
	online := offlineSectionCursor(entry, "unread", "newest", 50, 0)
	if _, _, _, err := parseOfflineSectionCursor(online, "unread", "newest", asOf); err == nil {
		t.Fatal("accepted online cursor in offline scope")
	}
}

// TestOfflineReviewSelection detects membership and rank changes, including an empty queue.
func TestOfflineReviewSelection(t *testing.T) {
	raw := 49.875
	makeEntries := func(ids ...int64) model.Entries {
		entries := make(model.Entries, 0, len(ids))
		for _, id := range ids {
			entries = append(entries, &model.Entry{ID: id, RawRelevanceScore: &raw, Date: time.Unix(1790762200, 0)})
		}
		return entries
	}
	selection := offlineReviewSelection(makeEntries(1, 2, 3))
	for _, ids := range [][]int64{{2, 1, 3}, {1, 2}, {1, 2, 4}, nil} {
		if selection == offlineReviewSelection(makeEntries(ids...)) {
			t.Fatalf("selection did not change for %v", ids)
		}
	}
	raw = 50.125
	if selection == offlineReviewSelection(makeEntries(1, 2, 3)) {
		t.Fatal("selection did not change when raw rank values changed")
	}
	now := time.Unix(1790762400, 0)
	for _, section := range []string{"review", "unread"} {
		r := httptest.NewRequest(http.MethodGet, "/?scope=offline&selection="+selection, nil)
		r.SetPathValue("section", section)
		params, err := parseOfflineSectionParameters(r, now)
		if (err == nil) != (section == "review") || section == "review" && params.selection != selection {
			t.Fatalf("invalid selection scope behavior: %s, %+v, %v", section, params, err)
		}
	}
}

// TestOfflineReviewContinuationRequiresSelection rejects continuation without the consistency token.
func TestOfflineReviewContinuationRequiresSelection(t *testing.T) {
	path := "/?scope=offline&as_of=1790762400&cursor=marker"
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.SetPathValue("section", "review")
	if _, err := parseOfflineSectionParameters(r, time.Unix(1790762400, 0)); err == nil {
		t.Fatal("accepted Review continuation without selection")
	}
	r = httptest.NewRequest(http.MethodGet, path+"&selection="+offlineReviewSelection(nil), nil)
	r.SetPathValue("section", "review")
	if _, err := parseOfflineSectionParameters(r, time.Unix(1790762400, 0)); err != nil {
		t.Fatalf("rejected complete continuation parameters: %v", err)
	}
}

// TestOfflineReviewPage verifies canonical membership, fractional cursor ties, and both page orders.
func TestOfflineReviewPage(t *testing.T) {
	raw := []float64{49.875, 50.125, 50.25, 51}
	published := []int64{100, 100, 300, 200}
	entries := make(model.Entries, 4)
	for i := range entries {
		entries[i] = &model.Entry{ID: int64(4 - i), RawRelevanceScore: &raw[i], Date: time.Unix(published[i], 0)}
	}
	ids := func(rows model.Entries) []int64 {
		result := make([]int64, len(rows))
		for i, row := range rows {
			result[i] = row.ID
		}
		return result
	}
	copyEntries := func() model.Entries { return append(model.Entries(nil), entries...) }
	for _, tc := range []struct {
		order  string
		first  int64
		second int64
		id     int64
		want   []int64
	}{
		{"score", 0, 0, 0, []int64{4, 3, 2, 1}},
		{"newest", 0, 0, 0, []int64{2, 1, 4, 3}},
		{"score", int64(math.Float64bits(0.125)), 100000000, 4, []int64{3, 2, 1}},
		{"score", int64(math.Float64bits(0.125)), 100000000, 3, []int64{2, 1}},
		{"newest", 100000000, 0, 4, []int64{3}},
		{"newest", 100000000, 0, 3, []int64{}},
	} {
		got := ids(offlineReviewPage(copyEntries(), tc.order, tc.first, tc.second, tc.id, 50))
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s after %d: got %v, want %v", tc.order, tc.id, got, tc.want)
		}
	}
	if got := offlineReviewPage(nil, "newest", 0, 0, 0, 50); len(got) != 0 {
		t.Fatal("empty canonical queue gained members")
	}
}
