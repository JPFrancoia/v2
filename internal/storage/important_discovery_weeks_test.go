// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/important_discovery_weeks.sql
var importantDiscoveryWeeksFixture string

//go:embed testdata/original_discovery_counts.sql
var originalDiscoveryCountsQuery string

//go:embed testdata/important_recall_exclusions.sql
var importantRecallExclusionsFixture string

// TestImportantDiscoveryWeeksClassifierCounts checks feedback, read state, and prediction exclusions without changing discovery counts.
func TestImportantDiscoveryWeeksClassifierCounts(t *testing.T) {
	store := newEntryQueryFixture(t)
	_, err := store.db.ExecContext(t.Context(), importantDiscoveryWeeksFixture)
	require.NoError(t, err)

	weeks, err := store.ImportantDiscoveryWeeks(t.Context(), 42, "America/Los_Angeles")
	require.NoError(t, err)
	require.Len(t, weeks, 12)
	assert.Equal(t, 6, weeks[1].ClassifierReadCount)
	assert.Equal(t, 3, weeks[1].ClassifierUpvoteCount)
	assert.Equal(t, 1, weeks[1].ClassifierMissedCount)
	assert.Equal(t, 8, weeks[1].ReadCount)
	assert.Equal(t, 6, weeks[1].ImportantCount)
	assert.Zero(t, weeks[0].ClassifierReadCount)
	assert.Equal(t, 1, weeks[3].ClassifierReadCount)
	assert.Zero(t, weeks[3].ClassifierUpvoteCount)
	assert.Zero(t, weeks[3].ImportantCount)
	assert.Zero(t, weeks[11].ReadCount)
	for i := 1; i < len(weeks); i++ {
		assert.Equal(t, weeks[i-1].WeekStart.AddDate(0, 0, -7), weeks[i].WeekStart)
	}

	// Compare every original count with the same calculation without the prediction join.
	for _, week := range weeks {
		var readCount, importantCount int
		err := store.db.QueryRowContext(t.Context(), originalDiscoveryCountsQuery, 42, week.WeekStart, "America/Los_Angeles").Scan(&readCount, &importantCount)
		require.NoError(t, err)
		assert.Equal(t, week.ReadCount, readCount)
		assert.Equal(t, week.ImportantCount, importantCount)
	}
}

// TestImportantDiscoveryWeeksTimezoneAndUser checks local Monday boundaries and user isolation.
func TestImportantDiscoveryWeeksTimezoneAndUser(t *testing.T) {
	store := newEntryQueryFixture(t)
	_, err := store.db.ExecContext(t.Context(), importantDiscoveryWeeksFixture)
	require.NoError(t, err)

	localWeeks, err := store.ImportantDiscoveryWeeks(t.Context(), 42, "America/Los_Angeles")
	require.NoError(t, err)
	utcWeeks, err := store.ImportantDiscoveryWeeks(t.Context(), 42, "UTC")
	require.NoError(t, err)
	// Find the same calendar week even if UTC has already reached the next Monday.
	found := false
	for _, week := range utcWeeks {
		if week.WeekStart.Equal(localWeeks[1].WeekStart) {
			found = true
			assert.Equal(t, 7, week.ClassifierReadCount)
			assert.Equal(t, 4, week.ClassifierUpvoteCount)
		}
	}
	assert.True(t, found)
	assert.Equal(t, 1, localWeeks[2].ClassifierReadCount)
	assert.Equal(t, 1, localWeeks[2].ClassifierUpvoteCount)
	otherWeeks, err := store.ImportantDiscoveryWeeks(t.Context(), 43, "America/Los_Angeles")
	require.NoError(t, err)
	assert.Equal(t, 1, otherWeeks[1].ReadCount)
	assert.Equal(t, 1, otherWeeks[1].ClassifierReadCount)
	assert.Equal(t, 1, otherWeeks[1].ClassifierUpvoteCount)
}

// TestImportantDiscoveryRecallExclusions checks false predictions without treating unclassified articles as missed.
func TestImportantDiscoveryRecallExclusions(t *testing.T) {
	store := newEntryQueryFixture(t)
	_, err := store.db.ExecContext(t.Context(), importantDiscoveryWeeksFixture)
	require.NoError(t, err)
	_, err = store.db.ExecContext(t.Context(), importantRecallExclusionsFixture)
	require.NoError(t, err)
	weeks, err := store.ImportantDiscoveryWeeks(t.Context(), 42, "America/Los_Angeles")
	require.NoError(t, err)
	assert.Equal(t, 3, weeks[1].ClassifierUpvoteCount)
	assert.Equal(t, 1, weeks[1].ClassifierMissedCount)
	assert.Zero(t, weeks[3].ClassifierMissedCount)
	other, err := store.ImportantDiscoveryWeeks(t.Context(), 43, "America/Los_Angeles")
	require.NoError(t, err)
	assert.Equal(t, 1, other[1].ClassifierUpvoteCount)
	assert.Equal(t, 1, other[1].ClassifierMissedCount)
}
