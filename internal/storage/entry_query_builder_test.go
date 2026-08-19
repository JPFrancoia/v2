// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import "testing"

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
	if result := builder.contentColumn(); result != "left(e.content, 4096) as content" {
		t.Fatalf(`Unexpected preview content column: %q`, result)
	}
}
