// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import "testing"

// TestEntryPaginationBuilderSorting verifies standard pagination sorting expressions.
func TestEntryPaginationBuilderSorting(t *testing.T) {
	tests := []struct {
		order     string
		direction string
		want      string
	}{
		{"score", "desc", "e.score desc, e.id desc"},
		{"published_at", "asc", "e.published_at asc, e.id asc"},
	}

	for _, tc := range tests {
		builder := &entryPaginationBuilder{order: tc.order, direction: tc.direction}
		if got := builder.buildSorting(); got != tc.want {
			t.Errorf(`Got %q; want %q`, got, tc.want)
		}
	}
}

// TestEntryPaginationBuilderScoreDistanceSorting verifies that the target score remains a bound argument.
func TestEntryPaginationBuilderScoreDistanceSorting(t *testing.T) {
	builder := &entryPaginationBuilder{args: []any{int64(42)}}
	builder.WithScoreDistanceSorting(123)

	if result := builder.sortExpressions[0]; result != "ABS(e.score - $2) ASC" {
		t.Fatalf(`Unexpected score sorting expression: %q`, result)
	}
	if result := builder.args[1]; result != int64(123) {
		t.Fatalf(`Unexpected score sorting argument: %v`, result)
	}
}
