// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package database // import "miniflux.app/v2/internal/database"

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"net/url"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed testdata/article_summary_before.sql
var articleSummaryBefore string

//go:embed testdata/article_summary_after.sql
var articleSummaryAfter string

// TestDropArticleSummary preserves article data while removing the stored summary.
func TestDropArticleSummary(t *testing.T) {
	databaseURL := os.Getenv("TEST_STORAGE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Set TEST_STORAGE_DATABASE_URL to isolated local PostgreSQL")
	}
	u, err := url.Parse(databaseURL)
	require.NoError(t, err)
	require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, u.Hostname())
	db, err := sql.Open("postgres", databaseURL)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { tx.Rollback() })
	_, err = tx.Exec(articleSummaryBefore)
	require.NoError(t, err)
	require.NoError(t, migrations[141](tx))
	var encoded []byte
	require.NoError(t, tx.QueryRow(articleSummaryAfter).Scan(&encoded))
	var row map[string]any
	require.NoError(t, json.Unmarshal(encoded, &row))
	require.Equal(t, map[string]any{"id": float64(1), "content": "Original article body."}, row)
}
