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

// TestEntryPaginationBuilderRawRelevanceSorting verifies the raw target and null filter.
func TestEntryPaginationBuilderRawRelevanceSorting(t *testing.T) {
	builder := &entryPaginationBuilder{args: []any{int64(42)}}
	builder.WithRawRelevanceDistanceSorting(50)

	if result := builder.sortExpressions[0]; result != "abs(e.raw_relevance_score - $2) asc" {
		t.Fatalf(`Unexpected score sorting expression: %q`, result)
	}
	if result := builder.args[1]; result != int64(50) {
		t.Fatalf(`Unexpected score sorting argument: %v`, result)
	}
	if result := builder.conditions[0]; result != "e.raw_relevance_score is not null" {
		t.Fatalf(`Unexpected review filter: %q`, result)
	}
}
