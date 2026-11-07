// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	_ "embed"
	"encoding/json"
	"testing"

	"miniflux.app/v2/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/entry_importance.sql
var entryImportanceFixture string

//go:embed testdata/entry_importance_missing.sql
var entryImportanceMissing string

//go:embed testdata/entry_importance_read.sql
var entryImportanceRead string

//go:embed testdata/entry_importance_status.sql
var entryStatusByIdQuery string

// newImportanceQueryFixture creates isolated predictions for list and navigation checks.
func newImportanceQueryFixture(t *testing.T) *Storage {
	t.Helper()
	store := newEntryQueryFixture(t)
	_, err := store.db.ExecContext(t.Context(), entryQueryCountedFixture)
	require.NoError(t, err)
	_, err = store.db.ExecContext(t.Context(), entryImportanceFixture)
	require.NoError(t, err)
	return store
}

// TestEntryImportanceReads checks optional flags, missing predictions, and API serialization.
func TestEntryImportanceReads(t *testing.T) {
	store := newImportanceQueryFixture(t)
	_, err := store.db.ExecContext(t.Context(), entryImportanceMissing)
	require.NoError(t, err)
	for _, counted := range []bool{false, true} {
		builder := NewEntryQueryBuilder(t.Context(), store, 42).WithImportance().WithSorting("id", "asc").WithLimit(3).WithoutContent()
		var entries model.Entries
		if counted {
			var count int
			entries, count, err = builder.GetEntriesWithCount()
			assert.Equal(t, 22, count)
		} else {
			entries, err = builder.GetEntries()
		}
		require.NoError(t, err)
		require.Len(t, entries, 3)
		for _, entry := range entries {
			assert.False(t, entry.IsImportant, "false and absent predictions receive no flag")
			assert.Empty(t, entry.Content)
		}
	}
	entry, err := NewEntryQueryBuilder(t.Context(), store, 42).WithEntryID(202).WithImportance().GetEntry()
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.True(t, entry.IsImportant)
	encoded, err := json.Marshal(entry)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "is_important")
	assert.NotContains(t, string(encoded), "IsImportant")
}

// TestImportantEntryPages checks membership, visibility, ordering, and counted page results.
func TestImportantEntryPages(t *testing.T) {
	store := newImportanceQueryFixture(t)
	for _, order := range []string{"score", "published_at"} {
		for _, direction := range []string{"asc", "desc"} {
			makeBuilder := func() *EntryQueryBuilder {
				return NewEntryQueryBuilder(t.Context(), store, 42).WithImportant().WithImportance().WithStatus(model.EntryStatusUnread).WithGloballyVisible().WithSorting(order, direction).WithSorting("id", direction).WithoutContent()
			}
			all, err := makeBuilder().GetEntries()
			require.NoError(t, err)
			require.NotEmpty(t, all)
			for _, entry := range all {
				assert.True(t, entry.IsImportant)
				assert.Equal(t, model.EntryStatusUnread, entry.Status)
				assert.EqualValues(t, 42, entry.UserID)
				assert.EqualValues(t, 9, entry.FeedID)
				assert.Empty(t, entry.Content)
			}
			for _, offset := range []int{0, 1, len(all), len(all) + 10} {
				got, count, err := makeBuilder().WithLimit(2).WithOffset(offset).GetEntriesWithCount()
				require.NoError(t, err)
				start := min(offset, len(all))
				end := min(start+2, len(all))
				assert.Equal(t, all[start:end], got)
				if start == end {
					assert.Zero(t, count)
				} else {
					assert.Equal(t, len(all), count)
				}
			}
			count, err := makeBuilder().CountEntries()
			require.NoError(t, err)
			assert.Equal(t, len(all), count)
		}
	}
}

// TestImportantEntryNavigation checks sibling selection without changing read state on refresh.
func TestImportantEntryNavigation(t *testing.T) {
	store := newImportanceQueryFixture(t)
	for _, order := range []string{"score", "published_at"} {
		for _, direction := range []string{"asc", "desc"} {
			all, err := NewEntryQueryBuilder(t.Context(), store, 42).WithImportant().WithStatus(model.EntryStatusUnread).WithGloballyVisible().WithSorting(order, direction).WithSorting("id", direction).GetEntries()
			require.NoError(t, err)
			for i, entry := range all {
				builder := NewEntryPaginationBuilder(t.Context(), store, 42, entry.ID, order, direction)
				builder.WithImportant()
				builder.WithGloballyVisible()
				builder.WithStatusOrEntryID(model.EntryStatusUnread, entry.ID)
				previous, next, err := builder.Entries()
				require.NoError(t, err)
				if i == 0 {
					assert.Nil(t, previous)
				} else {
					require.NotNil(t, previous)
					assert.Equal(t, all[i-1].ID, previous.ID)
				}
				if i == len(all)-1 {
					assert.Nil(t, next)
				} else {
					require.NotNil(t, next)
					assert.Equal(t, all[i+1].ID, next.ID)
				}
			}
		}
	}
	_, err := store.db.ExecContext(t.Context(), entryImportanceRead)
	require.NoError(t, err)
	builder := NewEntryPaginationBuilder(t.Context(), store, 42, 202, "id", "asc")
	builder.WithImportant()
	builder.WithGloballyVisible()
	builder.WithStatusOrEntryID(model.EntryStatusUnread, 202)
	_, next, err := builder.Entries()
	require.NoError(t, err)
	require.NotNil(t, next)
	assert.EqualValues(t, 204, next.ID)
	var status string
	require.NoError(t, store.db.QueryRowContext(t.Context(), entryStatusByIdQuery).Scan(&status))
	assert.Equal(t, model.EntryStatusRead, status)
}

// TestEntryQueriesWithoutImportanceSchema keeps existing clients independent of Feedoscope tables.
func TestEntryQueriesWithoutImportanceSchema(t *testing.T) {
	store := newEntryQueryFixture(t)
	for _, counted := range []bool{false, true} {
		builder := NewEntryQueryBuilder(t.Context(), store, 42)
		var err error
		if counted {
			_, _, err = builder.GetEntriesWithCount()
		} else {
			_, err = builder.GetEntries()
		}
		require.NoError(t, err)
	}
}
