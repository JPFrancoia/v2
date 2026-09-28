// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"context"
	"testing"
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
