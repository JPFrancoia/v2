// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEntryJSONWithoutSummary preserves article content without a summary response field.
func TestEntryJSONWithoutSummary(t *testing.T) {
	encoded, err := json.Marshal(Entry{ID: 1, Content: "Article body."})
	require.NoError(t, err)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(encoded, &entry))
	require.Equal(t, "Article body.", entry["content"])
	require.NotContains(t, entry, "one_line_summary")
}
