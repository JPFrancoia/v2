// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"
	"testing"

	"miniflux.app/v2/internal/model"
)

func TestEntryListSorting(t *testing.T) {
	user := &model.User{EntryOrder: "created_at", EntryDirection: "asc"}
	tests := []struct {
		name          string
		query         string
		wantOrder     string
		wantDirection string
	}{
		{"user settings", "", "created_at", "asc"},
		{"score descending", "?order=score&direction=desc", "score", "desc"},
		{"published ascending", "?order=published_at&direction=asc", "published_at", "asc"},
		{"invalid values", "?order=title&direction=sideways", "created_at", "asc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := http.NewRequest(http.MethodGet, "/unread"+tc.query, nil)
			if err != nil {
				t.Fatal(err)
			}

			order, direction := entryListSorting(r, user)
			if order != tc.wantOrder || direction != tc.wantDirection {
				t.Errorf(`Got order=%q, direction=%q; want order=%q, direction=%q`, order, direction, tc.wantOrder, tc.wantDirection)
			}
		})
	}
}

func TestNextEntryListSortDirection(t *testing.T) {
	if got := nextEntryListSortDirection("score", "desc", "score"); got != "asc" {
		t.Errorf(`Got %q; want "asc"`, got)
	}

	if got := nextEntryListSortDirection("score", "asc", "score"); got != "desc" {
		t.Errorf(`Got %q; want "desc"`, got)
	}

	if got := nextEntryListSortDirection("score", "asc", "published_at"); got != "desc" {
		t.Errorf(`Got %q; want "desc"`, got)
	}
}
