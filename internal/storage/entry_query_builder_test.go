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

// TestEntryQueryBuilderScoreDistanceSorting verifies that the target score remains a bound argument.
func TestEntryQueryBuilderScoreDistanceSorting(t *testing.T) {
	builder := NewEntryQueryBuilder(context.Background(), nil, 42)
	builder.WithCategoryID(7).WithScoreDistanceSorting(123)

	if result := builder.sortExpressions[0]; result != "ABS(e.score - $3) ASC" {
		t.Fatalf(`Unexpected score sorting expression: %q`, result)
	}
	if result := builder.args[2]; result != int64(123) {
		t.Fatalf(`Unexpected score sorting argument: %v`, result)
	}
}
