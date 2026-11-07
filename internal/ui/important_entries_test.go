// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"context"
	"database/sql"
	_ "embed"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
	"miniflux.app/v2/internal/template"
	"miniflux.app/v2/internal/ui/static"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/important_pages.sql
var importantPagesFixture string

//go:embed testdata/important_status.sql
var importantStatusQuery string

// newImportantHandlerFixture builds a real handler with connection-local synthetic tables.
func newImportantHandlerFixture(t *testing.T) (*handler, *sql.DB) {
	t.Helper()
	databaseURL := os.Getenv("TEST_STORAGE_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("Set TEST_STORAGE_DATABASE_URL to an isolated local PostgreSQL database")
	}
	parsed, err := url.Parse(databaseURL)
	require.NoError(t, err)
	require.Contains(t, []string{"localhost", "127.0.0.1", "::1"}, parsed.Hostname())
	db, err := sql.Open("postgres", databaseURL)
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	_, err = db.ExecContext(t.Context(), importantPagesFixture)
	require.NoError(t, err)
	config.Opts = config.NewConfigOptions()
	require.NoError(t, static.GenerateBinaryBundles())
	require.NoError(t, static.GenerateStylesheetsBundles())
	require.NoError(t, static.GenerateJavascriptBundles(false))
	engine := template.NewEngine("")
	engine.ParseTemplates()
	return &handler{store: storage.NewStorage(db), tpl: engine}, db
}

// importantHandlerRequest creates an authenticated request without bypassing storage checks.
func importantHandlerRequest(t *testing.T, h *handler, target string) *http.Request {
	t.Helper()
	user, err := h.store.UserByID(t.Context(), 42)
	require.NoError(t, err)
	session, _ := model.NewWebSession("important tests", "127.0.0.1")
	session.SetUser(user)
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(context.WithValue(r.Context(), request.WebSessionContextKey, session))
}

// TestImportantHandlerOffsetRecovery checks the first page after an old offset no longer matches results.
func TestImportantHandlerOffsetRecovery(t *testing.T) {
	h, _ := newImportantHandlerFixture(t)
	r := importantHandlerRequest(t, h, "/important?order=score&direction=desc&offset=999")
	w := httptest.NewRecorder()
	h.showImportantPage(w, r)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `data-id="103"`)
	assert.Contains(t, w.Body.String(), `data-id="102"`)
	assert.NotContains(t, w.Body.String(), `data-id="101"`)
	for _, id := range []string{"104", "105", "106", "107", "108"} {
		assert.NotContains(t, w.Body.String(), `data-id="`+id+`"`)
	}
	assert.Contains(t, w.Body.String(), "Important (3)")
	assert.Contains(t, w.Body.String(), "offset=2")
}

// TestImportantHandlerReadOnView keeps refresh navigation without moving the read timestamp twice.
func TestImportantHandlerReadOnView(t *testing.T) {
	h, db := newImportantHandlerFixture(t)
	var changedBefore time.Time
	var status string
	require.NoError(t, db.QueryRowContext(t.Context(), importantStatusQuery, 102).Scan(&status, &changedBefore))
	r := importantHandlerRequest(t, h, "/important/entry/102?order=score&direction=desc")
	r.SetPathValue("entryID", "102")
	w := httptest.NewRecorder()
	h.showImportantEntryPage(w, r)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `/important/entry/103?direction=desc&amp;order=score`)
	assert.Contains(t, w.Body.String(), `/important/entry/101?direction=desc&amp;order=score`)
	var changedAfter time.Time
	require.NoError(t, db.QueryRowContext(t.Context(), importantStatusQuery, 102).Scan(&status, &changedAfter))
	assert.Equal(t, model.EntryStatusRead, status)
	assert.False(t, changedAfter.Before(changedBefore))
	w = httptest.NewRecorder()
	h.showImportantEntryPage(w, r)
	require.Equal(t, http.StatusOK, w.Code)
	var changedRefresh time.Time
	require.NoError(t, db.QueryRowContext(t.Context(), importantStatusQuery, 102).Scan(&status, &changedRefresh))
	assert.Equal(t, changedAfter, changedRefresh)
	assert.Equal(t, model.EntryStatusRead, status)
	assert.Contains(t, w.Body.String(), `/important/entry/101?direction=desc&amp;order=score`)
}

// TestImportantHandlerExclusions redirects false, missing, hidden, and other-user articles without changing their status.
func TestImportantHandlerExclusions(t *testing.T) {
	h, db := newImportantHandlerFixture(t)
	for _, id := range []string{"104", "105", "106", "107", "108", "999"} {
		r := importantHandlerRequest(t, h, "/important/entry/"+id)
		r.SetPathValue("entryID", id)
		w := httptest.NewRecorder()
		h.showImportantEntryPage(w, r)
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/important", w.Header().Get("Location"))
		if id != "999" {
			var status string
			var changed time.Time
			require.NoError(t, db.QueryRowContext(t.Context(), importantStatusQuery, id).Scan(&status, &changed))
			assert.Equal(t, model.EntryStatusUnread, status)
		}
	}
}
