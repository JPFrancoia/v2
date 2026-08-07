// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import "testing"

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
