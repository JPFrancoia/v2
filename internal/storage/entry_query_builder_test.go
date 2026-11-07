// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"context"
	"database/sql"
	_ "embed"
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
		store := NewStorage(db)
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
