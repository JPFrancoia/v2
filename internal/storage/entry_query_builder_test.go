// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"math"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"miniflux.app/v2/internal/model"
)

//go:embed testdata/entry_query_text.sql
var entryQueryTextFixture string

//go:embed testdata/entry_query_stored_text.sql
var entryQueryStoredText string

//go:embed testdata/entry_query_counted.sql
var entryQueryCountedFixture string

// TestEntryQueryBuilderContentColumn checks text selection without changing stored data.
func TestEntryQueryBuilderContentColumn(t *testing.T) {
	builder := &EntryQueryBuilder{}
	if result := builder.contentColumn(); result != "e.content" {
		t.Fatalf(`Unexpected full content column: %q`, result)
	}

	builder.WithoutContent()
	if result := builder.contentColumn(); result != "'' AS content" {
		t.Fatalf(`Unexpected empty content column: %q`, result)
	}

	t.Run("PostgreSQL", func(t *testing.T) {
		store := newEntryQueryFixture(t)
		db := store.db
		for _, tc := range []struct {
			name      string
			configure func(*EntryQueryBuilder)
			content   string
			summary   string
		}{
			{"full", func(*EntryQueryBuilder) {}, strings.Repeat("Original article body. ", 400), "Stored generated summary."},
			{"without_content", func(b *EntryQueryBuilder) { b.WithoutContent() }, "", "Stored generated summary."},
			{"without_summary", func(b *EntryQueryBuilder) { b.WithoutSummary() }, strings.Repeat("Original article body. ", 400), ""},
			{"list", func(b *EntryQueryBuilder) { b.WithoutContent().WithoutSummary() }, "", ""},
		} {
			for _, counted := range []bool{false, true} {
				name := tc.name + "/plain"
				if counted {
					name = tc.name + "/counted"
				}
				t.Run(name, func(t *testing.T) {
					b := NewEntryQueryBuilder(t.Context(), store, 42)
					b.WithSearchQuery("needle").WithSorting("e.id", "asc").WithLimit(2).WithOffset(1)
					tc.configure(b)
					var entries model.Entries
					var err error
					if counted {
						var count int
						entries, count, err = b.GetEntriesWithCount()
						if err == nil && count != 3 {
							t.Fatalf("Unexpected count: %d", count)
						}
					} else {
						entries, err = b.GetEntries()
					}
					if err != nil {
						t.Fatal(err)
					}
					if len(entries) != 2 {
						t.Fatalf("Unexpected entries: %d", len(entries))
					}
					for i, entry := range entries {
						if entry.ID != int64(102+i) || entry.Title != "Fixture article" || entry.Score != 55 {
							t.Fatalf("Changed list metadata: id=%d title=%q score=%d", entry.ID, entry.Title, entry.Score)
						}
						if entry.Content != tc.content || entry.OneLineSummary != tc.summary {
							t.Errorf("Unexpected text: content length=%d, summary=%q", len(entry.Content), entry.OneLineSummary)
						}
					}
				})
			}
		}

		t.Run("narrow_count_stage", func(t *testing.T) {
			builder := NewEntryQueryBuilder(t.Context(), store, 42)
			builder.WithoutContent().WithoutSummary().WithSorting("e.score", "desc").WithSorting("e.id", "desc").WithLimit(2)
			var raw []byte
			if err := db.QueryRowContext(t.Context(), "explain (format json) "+builder.entryQuery(true), builder.args...).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var plans []struct{ Plan map[string]any }
			if err := json.Unmarshal(raw, &plans); err != nil {
				t.Fatal(err)
			}
			var visit func(map[string]any)
			windows := 0
			visit = func(node map[string]any) {
				if node["Node Type"] == "WindowAgg" {
					windows++
					// The count stage needs IDs, a count, and sort keys, not feed metadata.
					if width := node["Plan Width"].(float64); width > 64 {
						t.Errorf("Count stage carries %.0f bytes per row, want at most 64", width)
					}
				}
				if children, ok := node["Plans"].([]any); ok {
					for _, child := range children {
						visit(child.(map[string]any))
					}
				}
			}
			visit(plans[0].Plan)
			if windows != 1 {
				t.Fatalf("Unexpected count stages: %d", windows)
			}
		})
		var content, summary string
		if err := db.QueryRowContext(t.Context(), entryQueryStoredText, 101).Scan(&content, &summary); err != nil {
			t.Fatal(err)
		}
		if content != strings.Repeat("Original article body. ", 400) || summary != "Stored generated summary." {
			t.Fatal("Stored article text changed")
		}
	})
}

// TestEntryQueryBuilderRawRelevanceSorting verifies the raw target and null filter.
func TestEntryQueryBuilderRawRelevanceSorting(t *testing.T) {
	builder := NewEntryQueryBuilder(context.Background(), nil, 42)
	builder.WithCategoryID(7).WithRawRelevanceDistanceSorting(50)

	if result := builder.sortExpressions[0]; result != "abs(e.raw_relevance_score - $3) asc" {
		t.Fatalf(`Unexpected score sorting expression: %q`, result)
	}
	if result := builder.args[2]; result != int64(50) {
		t.Fatalf(`Unexpected score sorting argument: %v`, result)
	}
	if result := builder.conditions[len(builder.conditions)-1]; result != "e.raw_relevance_score is not null" {
		t.Fatalf(`Unexpected review filter: %q`, result)
	}
}

// TestOfflineSectionCursorPredicates verifies ties, raw precision, and bound arguments.
func TestOfflineSectionCursorPredicates(t *testing.T) {
	published := int64(1790762400000000)
	for _, tc := range []struct {
		section   string
		sort      string
		first     int64
		second    int64
		condition string
		args      []any
	}{
		{"review", "score", int64(math.Float64bits(0.125)), published,
			`(abs(e.raw_relevance_score-$2) > $3 or (abs(e.raw_relevance_score-$2) = $3 and (e.published_at, e.id) < ($4, $5)))`,
			[]any{int64(42), int64(50), 0.125, time.UnixMicro(published), int64(55)}},
		{"history", "newest", published + 1000000, published,
			`(e.changed_at, e.published_at, e.id) < ($2, $3, $4)`,
			[]any{int64(42), time.UnixMicro(published + 1000000), time.UnixMicro(published), int64(55)}},
		{"unread", "newest", published, 0,
			`(e.published_at, e.id) < ($2, $3)`,
			[]any{int64(42), time.UnixMicro(published), int64(55)}},
		{"saved", "score", 21, 0,
			`(e.score, e.id) < ($2, $3)`,
			[]any{int64(42), int64(21), int64(55)}},
	} {
		t.Run(tc.section+"-"+tc.sort, func(t *testing.T) {
			builder := NewEntryQueryBuilder(context.Background(), nil, 42)
			builder.AfterOfflineSectionCursor(tc.section, tc.sort, tc.first, tc.second, 55, 50)
			if builder.conditions[1] != tc.condition || !reflect.DeepEqual(builder.args, tc.args) {
				t.Fatalf(`Unexpected cursor predicate: %q, args: %#v`, builder.conditions[1], builder.args)
			}
		})
	}
}

// TestEntryQueryBuilderPreparationCutoff excludes new arrivals with a bound timestamp.
func TestEntryQueryBuilderPreparationCutoff(t *testing.T) {
	asOf := time.Unix(1790762400, 0)
	builder := NewEntryQueryBuilder(context.Background(), nil, 42)
	builder.BeforeCreatedDate(asOf).AfterPublishedDate(asOf.Add(-7 * 24 * time.Hour))
	if got := builder.conditions[1]; got != "e.created_at <= $2" {
		t.Fatalf("unexpected creation cutoff: %q", got)
	}
	if got := builder.args[1]; got != asOf {
		t.Fatalf("unexpected bound cutoff: %v", got)
	}
}

// newEntryQueryFixture creates connection-local tables without changing stored rows.
func newEntryQueryFixture(t *testing.T) *Storage {
	t.Helper()
	databaseURL := os.Getenv("TEST_STORAGE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Set TEST_STORAGE_DATABASE_URL to an isolated local PostgreSQL database")
	}
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		t.Fatal("Storage text checks require a loopback database")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.ExecContext(t.Context(), entryQueryTextFixture); err != nil {
		t.Fatal(err)
	}
	return NewStorage(db)
}

// TestCountedEntryPages preserves complete results while counting before metadata joins.
func TestCountedEntryPages(t *testing.T) {
	store := newEntryQueryFixture(t)
	if _, err := store.db.ExecContext(t.Context(), entryQueryCountedFixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		configure func(*EntryQueryBuilder)
	}{
		{"all", func(b *EntryQueryBuilder) {}},
		{"visible_unread", func(b *EntryQueryBuilder) { b.WithGloballyVisible().WithStatus(model.EntryStatusUnread) }},
		{"saved", func(b *EntryQueryBuilder) { b.WithSavedForLater(true) }},
		{"starred", func(b *EntryQueryBuilder) { b.WithStarred(true) }},
		{"search", func(b *EntryQueryBuilder) { b.WithSearchQuery("needle") }},
		{"review", func(b *EntryQueryBuilder) { b.WithRawRelevanceDistanceSorting(25).WithVote(0) }},
		{"tags", func(b *EntryQueryBuilder) { b.WithUserTagID(17) }},
		{"feed_tags", func(b *EntryQueryBuilder) { b.WithTags([]string{"fixture"}) }},
		{"hidden_category", func(b *EntryQueryBuilder) { b.WithCategoryID(8) }},
		{"feed", func(b *EntryQueryBuilder) { b.WithFeedID(9) }},
		{"shared", func(b *EntryQueryBuilder) { b.WithShareCodeNotEmpty() }},
		{"missing", func(b *EntryQueryBuilder) { b.WithEntryID(99999) }},
		{"text_and_enclosures", func(b *EntryQueryBuilder) { b.WithEnclosures() }},
		{"without_text", func(b *EntryQueryBuilder) { b.WithoutContent().WithoutSummary() }},
	} {
		for _, direction := range []string{"asc", "desc"} {
			t.Run(tc.name+"/"+direction, func(t *testing.T) {
				makeBuilder := func() *EntryQueryBuilder {
					builder := NewEntryQueryBuilder(t.Context(), store, 42)
					tc.configure(builder)
					builder.WithSorting("e.published_at", direction).WithSorting("e.id", direction)
					return builder
				}
				all, err := makeBuilder().GetEntries()
				if err != nil {
					t.Fatal(err)
				}
				for _, page := range []struct{ limit, offset int }{{0, 0}, {2, 0}, {2, 1}, {2, 100}} {
					builder := makeBuilder().WithLimit(page.limit).WithOffset(page.offset)
					got, count, err := builder.GetEntriesWithCount()
					if err != nil {
						t.Fatal(err)
					}
					start := min(page.offset, len(all))
					end := len(all)
					if page.limit > 0 {
						end = min(start+page.limit, end)
					}
					wantCount := len(all)
					if start == end {
						wantCount = 0
					}
					if count != wantCount || !reflect.DeepEqual(got, all[start:end]) {
						t.Errorf("limit=%d offset=%d count=%d want=%d rows=%d want=%d", page.limit, page.offset, count, wantCount, len(got), end-start)
					}
				}
			})
		}
	}
}

// TestCountedEntrySorts preserves every supported API sort name in both query stages.
func TestCountedEntrySorts(t *testing.T) {
	store := newEntryQueryFixture(t)
	if _, err := store.db.ExecContext(t.Context(), entryQueryCountedFixture); err != nil {
		t.Fatal(err)
	}
	for _, order := range []string{"id", "status", "changed_at", "published_at", "created_at", "category_title", "category_id", "title", "author", "score"} {
		for _, direction := range []string{"asc", "desc"} {
			t.Run(order+"/"+direction, func(t *testing.T) {
				configure := func() *EntryQueryBuilder {
					return NewEntryQueryBuilder(t.Context(), store, 42).WithSorting(order, direction).WithSorting("e.id", direction)
				}
				want, err := configure().GetEntries()
				if err != nil {
					t.Fatal(err)
				}
				got, count, err := configure().WithLimit(2).WithOffset(1).GetEntriesWithCount()
				if err != nil {
					t.Fatal(err)
				}
				if count != len(want) || !reflect.DeepEqual(got, want[1:3]) {
					t.Fatal("Counted sorting changed API results")
				}
			})
		}
	}
}
