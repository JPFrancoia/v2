// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"
)

// TestEntryQueryBuilderContentColumn verifies full, empty, and preview content selections.
func TestEntryQueryBuilderContentColumn(t *testing.T) {
	builder := &EntryQueryBuilder{}
	if result := builder.contentColumn(); result != "e.content" {
		t.Fatalf(`Unexpected full content column: %q`, result)
	}

	builder.WithoutContent()
	if result := builder.contentColumn(); result != "'' AS content" {
		t.Fatalf(`Unexpected empty content column: %q`, result)
	}

	builder.WithContentPreview()
	if result := builder.contentColumn(); result != "(select left(preview.content, 4096) from entries preview where preview.id = e.id) as content" {
		t.Fatalf(`Unexpected preview content column: %q`, result)
	}
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
