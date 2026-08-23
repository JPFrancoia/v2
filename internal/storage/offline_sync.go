// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"

	"miniflux.app/v2/internal/model"
)

// ApplyOfflineEntryPatch applies one retry-safe offline entry patch.
func (s *Storage) ApplyOfflineEntryPatch(ctx context.Context, userID int64, patch *model.OfflineEntryPatch) (result model.OfflineEntryPatchResult, err error) {
	result.EntryID = patch.EntryID
	result.Conflicts = make([]model.OfflineEntryConflict, 0)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, fmt.Errorf(`store: unable to begin offline entry transaction: %v`, err)
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	state, err := offlineEntryState(ctx, tx, userID, patch.EntryID)
	if err == sql.ErrNoRows {
		result.Result = model.OfflineSyncResultNotFound
		if err = tx.Commit(); err != nil {
			return result, fmt.Errorf(`store: unable to commit missing offline entry transaction: %v`, err)
		}
		return result, nil
	}
	if err != nil {
		return result, err
	}

	final := *state
	scalarChanged := applyOfflineScalarValues(&final, state, patch, &result)
	tagChanged, err := applyOfflineTagDelta(ctx, tx, userID, patch)
	if err != nil {
		return result, err
	}

	if scalarChanged || tagChanged {
		err = tx.QueryRowContext(ctx, `
			UPDATE entries
			SET status=$1, starred=$2, saved_for_later=$3, vote=$4, changed_at=now()
			WHERE user_id=$5 AND id=$6
			RETURNING changed_at
		`, final.Status, final.Starred, final.SavedForLater, final.Vote, userID, patch.EntryID).Scan(&final.ChangedAt)
		if err != nil {
			return result, fmt.Errorf(`store: unable to update offline entry #%d: %v`, patch.EntryID, err)
		}
	}

	final.UserTagIDs, err = offlineEntryUserTagIDs(ctx, tx, userID, patch.EntryID)
	if err != nil {
		return result, err
	}
	result.State = &final
	switch {
	case len(result.Conflicts) > 0:
		result.Result = model.OfflineSyncResultConflict
	case scalarChanged || tagChanged:
		result.Result = model.OfflineSyncResultApplied
	default:
		result.Result = model.OfflineSyncResultUnchanged
	}

	if err = tx.Commit(); err != nil {
		return result, fmt.Errorf(`store: unable to commit offline entry transaction: %v`, err)
	}
	return result, nil
}

func offlineEntryState(ctx context.Context, tx *sql.Tx, userID, entryID int64) (*model.OfflineEntryState, error) {
	state := &model.OfflineEntryState{UserTagIDs: make([]int64, 0)}
	err := tx.QueryRowContext(ctx, `
		SELECT status, starred, saved_for_later, vote, changed_at
		FROM entries
		WHERE user_id=$1 AND id=$2
		FOR UPDATE
	`, userID, entryID).Scan(&state.Status, &state.Starred, &state.SavedForLater, &state.Vote, &state.ChangedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf(`store: unable to fetch offline entry #%d: %v`, entryID, err)
	}
	return state, nil
}

func applyOfflineScalarValues(final, current *model.OfflineEntryState, patch *model.OfflineEntryPatch, result *model.OfflineEntryPatchResult) bool {
	changed := false

	if patch.Set.Status != nil {
		statusConflict := current.Status != *patch.Base.Status && current.Status != *patch.Set.Status
		savedConflict := current.SavedForLater != *patch.Base.SavedForLater && current.SavedForLater != *patch.Set.SavedForLater
		if statusConflict || savedConflict {
			result.Conflicts = append(result.Conflicts, model.OfflineEntryConflict{
				Field:  "status_saved_for_later",
				Server: map[string]any{"status": current.Status, "saved_for_later": current.SavedForLater},
				Client: map[string]any{"status": *patch.Set.Status, "saved_for_later": *patch.Set.SavedForLater},
			})
		}
		if !statusConflict && !savedConflict {
			changed = current.Status != *patch.Set.Status || current.SavedForLater != *patch.Set.SavedForLater
			final.Status = *patch.Set.Status
			final.SavedForLater = *patch.Set.SavedForLater
		}
	}

	if patch.Set.Starred != nil {
		if current.Starred != *patch.Base.Starred && current.Starred != *patch.Set.Starred {
			result.Conflicts = append(result.Conflicts, model.OfflineEntryConflict{Field: "starred", Server: current.Starred, Client: *patch.Set.Starred})
		} else if current.Starred != *patch.Set.Starred {
			final.Starred = *patch.Set.Starred
			changed = true
		}
	}

	if patch.Set.Vote != nil {
		if current.Vote != *patch.Base.Vote && current.Vote != *patch.Set.Vote {
			result.Conflicts = append(result.Conflicts, model.OfflineEntryConflict{Field: "vote", Server: current.Vote, Client: *patch.Set.Vote})
		} else if current.Vote != *patch.Set.Vote {
			final.Vote = *patch.Set.Vote
			changed = true
		}
	}

	return changed
}

func applyOfflineTagDelta(ctx context.Context, tx *sql.Tx, userID int64, patch *model.OfflineEntryPatch) (bool, error) {
	changed := false
	if len(patch.RemoveUserTagIDs) > 0 {
		result, err := tx.ExecContext(ctx, `
			DELETE FROM entry_user_tags eut
			USING user_tags ut
			WHERE eut.entry_id=$1 AND eut.user_tag_id=ut.id AND ut.user_id=$2 AND eut.user_tag_id=ANY($3)
		`, patch.EntryID, userID, pq.Array(patch.RemoveUserTagIDs))
		if err != nil {
			return false, fmt.Errorf(`store: unable to remove offline entry user tags: %v`, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf(`store: unable to count removed offline entry user tags: %v`, err)
		}
		changed = count > 0
	}

	if len(patch.AddUserTagIDs) > 0 {
		result, err := tx.ExecContext(ctx, `
			INSERT INTO entry_user_tags (entry_id, user_tag_id)
			SELECT $1, id FROM user_tags WHERE user_id=$2 AND id=ANY($3)
			ON CONFLICT DO NOTHING
		`, patch.EntryID, userID, pq.Array(patch.AddUserTagIDs))
		if err != nil {
			return false, fmt.Errorf(`store: unable to add offline entry user tags: %v`, err)
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, fmt.Errorf(`store: unable to count added offline entry user tags: %v`, err)
		}
		changed = changed || count > 0
	}
	return changed, nil
}

func offlineEntryUserTagIDs(ctx context.Context, tx *sql.Tx, userID, entryID int64) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT eut.user_tag_id
		FROM entry_user_tags eut
		JOIN user_tags ut ON ut.id=eut.user_tag_id
		WHERE ut.user_id=$1 AND eut.entry_id=$2
	`, userID, entryID)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch offline entry user tags: %v`, err)
	}
	defer rows.Close()

	tagIDs := make([]int64, 0)
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf(`store: unable to scan offline entry user tag: %v`, err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to iterate offline entry user tags: %v`, err)
	}
	sort.Slice(tagIDs, func(i, j int) bool { return tagIDs[i] < tagIDs[j] })
	return tagIDs, nil
}

// OfflineEntryUserTagIDs returns user-tag IDs for the requested entries.
func (s *Storage) OfflineEntryUserTagIDs(ctx context.Context, userID int64, entryIDs []int64) (map[int64][]int64, error) {
	tagIDsByEntryID := make(map[int64][]int64, len(entryIDs))
	for _, entryID := range entryIDs {
		tagIDsByEntryID[entryID] = make([]int64, 0)
	}
	if len(entryIDs) == 0 {
		return tagIDsByEntryID, nil
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT eut.entry_id, eut.user_tag_id
		FROM entry_user_tags eut
		JOIN user_tags ut ON ut.id=eut.user_tag_id
		WHERE ut.user_id=$1 AND eut.entry_id=ANY($2)
		ORDER BY eut.entry_id, eut.user_tag_id
	`, userID, pq.Array(entryIDs))
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch offline entry user tag IDs: %v`, err)
	}
	defer rows.Close()

	for rows.Next() {
		var entryID, tagID int64
		if err := rows.Scan(&entryID, &tagID); err != nil {
			return nil, fmt.Errorf(`store: unable to scan offline entry user tag ID: %v`, err)
		}
		tagIDsByEntryID[entryID] = append(tagIDsByEntryID[entryID], tagID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to iterate offline entry user tag IDs: %v`, err)
	}
	return tagIDsByEntryID, nil
}

// OfflineManifest returns the entries that belong in a user's offline cache.
func (s *Storage) OfflineManifest(ctx context.Context, userID int64, now time.Time) (*model.OfflineManifest, error) {
	manifest := &model.OfflineManifest{
		UserID:                userID,
		GeneratedAt:           now,
		UnreadEntryIDs:        make([]int64, 0),
		SavedForLaterEntryIDs: make([]int64, 0),
		StarredEntryIDs:       make([]int64, 0),
		HistoryEntryIDs:       make([]int64, 0),
		UserTags:              make([]model.OfflineManifestUserTag, 0),
		EntryVersions:         make(map[int64]time.Time),
	}

	var err error
	manifest.UnreadEntryIDs, err = s.offlineEntryIDs(ctx, `status='unread' AND published_at >= $2`, userID, now.AddDate(0, 0, -30), 0)
	if err != nil {
		return nil, err
	}
	manifest.SavedForLaterEntryIDs, err = s.offlineEntryIDs(ctx, `saved_for_later=true`, userID, time.Time{}, 0)
	if err != nil {
		return nil, err
	}
	manifest.StarredEntryIDs, err = s.offlineEntryIDs(ctx, `starred=true`, userID, time.Time{}, 0)
	if err != nil {
		return nil, err
	}
	manifest.HistoryEntryIDs, err = s.offlineEntryIDs(ctx, `status='read'`, userID, time.Time{}, 100)
	if err != nil {
		return nil, err
	}
	manifest.UserTags, err = s.offlineManifestUserTags(ctx, userID)
	if err != nil {
		return nil, err
	}

	entryIDs := make(map[int64]struct{})
	addEntryIDs := func(ids []int64) {
		for _, entryID := range ids {
			entryIDs[entryID] = struct{}{}
		}
	}
	addEntryIDs(manifest.UnreadEntryIDs)
	addEntryIDs(manifest.SavedForLaterEntryIDs)
	addEntryIDs(manifest.StarredEntryIDs)
	addEntryIDs(manifest.HistoryEntryIDs)
	for _, tag := range manifest.UserTags {
		addEntryIDs(tag.EntryIDs)
	}
	manifest.EntryVersions, err = s.offlineEntryVersions(ctx, userID, entryIDs)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func (s *Storage) offlineEntryIDs(ctx context.Context, condition string, userID int64, date time.Time, limit int) ([]int64, error) {
	query := `SELECT id FROM entries WHERE user_id=$1 AND ` + condition + ` ORDER BY changed_at DESC, id DESC`
	args := []any{userID}
	if !date.IsZero() {
		args = append(args, date)
	}
	if limit > 0 {
		query += fmt.Sprintf(` LIMIT %d`, limit)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch offline entry IDs: %v`, err)
	}
	defer rows.Close()

	entryIDs := make([]int64, 0)
	for rows.Next() {
		var entryID int64
		if err := rows.Scan(&entryID); err != nil {
			return nil, fmt.Errorf(`store: unable to scan offline entry ID: %v`, err)
		}
		entryIDs = append(entryIDs, entryID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to iterate offline entry IDs: %v`, err)
	}
	return entryIDs, nil
}

func (s *Storage) offlineEntryVersions(ctx context.Context, userID int64, entryIDs map[int64]struct{}) (map[int64]time.Time, error) {
	versions := make(map[int64]time.Time, len(entryIDs))
	if len(entryIDs) == 0 {
		return versions, nil
	}

	ids := make([]int64, 0, len(entryIDs))
	for entryID := range entryIDs {
		ids = append(ids, entryID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, changed_at FROM entries WHERE user_id=$1 AND id=ANY($2)`, userID, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch offline entry versions: %v`, err)
	}
	defer rows.Close()
	for rows.Next() {
		var entryID int64
		var changedAt time.Time
		if err := rows.Scan(&entryID, &changedAt); err != nil {
			return nil, fmt.Errorf(`store: unable to scan offline entry version: %v`, err)
		}
		versions[entryID] = changedAt
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to iterate offline entry versions: %v`, err)
	}
	return versions, nil
}

func (s *Storage) offlineManifestUserTags(ctx context.Context, userID int64) ([]model.OfflineManifestUserTag, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ut.id, ut.title, tagged.entry_id
		FROM user_tags ut
		LEFT JOIN LATERAL (
			SELECT eut.entry_id
			FROM entry_user_tags eut
			JOIN entries e ON e.id=eut.entry_id
			WHERE eut.user_tag_id=ut.id
			ORDER BY e.published_at DESC, e.id DESC
			LIMIT 500
		) tagged ON true
		WHERE ut.user_id=$1
		ORDER BY ut.title, tagged.entry_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch offline user tags: %v`, err)
	}
	defer rows.Close()

	tags := make([]model.OfflineManifestUserTag, 0)
	byID := make(map[int64]int)
	for rows.Next() {
		var tagID int64
		var title string
		var entryID sql.NullInt64
		if err := rows.Scan(&tagID, &title, &entryID); err != nil {
			return nil, fmt.Errorf(`store: unable to scan offline user tag: %v`, err)
		}
		index, ok := byID[tagID]
		if !ok {
			index = len(tags)
			byID[tagID] = index
			tags = append(tags, model.OfflineManifestUserTag{ID: tagID, Title: title, EntryIDs: make([]int64, 0)})
		}
		if entryID.Valid {
			tags[index].EntryIDs = append(tags[index].EntryIDs, entryID.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to iterate offline user tags: %v`, err)
	}
	return tags, nil
}
