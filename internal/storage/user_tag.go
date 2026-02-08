// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"database/sql"
	"errors"
	"fmt"

	"miniflux.app/v2/internal/model"
)

// UserTagByID returns a user tag by its ID.
func (s *Storage) UserTagByID(userID, tagID int64) (*model.UserTag, error) {
	var tag model.UserTag

	query := `SELECT id, user_id, title FROM user_tags WHERE user_id=$1 AND id=$2`
	err := s.db.QueryRow(query, userID, tagID).Scan(&tag.ID, &tag.UserID, &tag.Title)

	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf(`store: unable to fetch user tag: %v`, err)
	default:
		return &tag, nil
	}
}

// UserTagByTitle returns a user tag by its title.
func (s *Storage) UserTagByTitle(userID int64, title string) (*model.UserTag, error) {
	var tag model.UserTag

	query := `SELECT id, user_id, title FROM user_tags WHERE user_id=$1 AND lower(title)=lower($2)`
	err := s.db.QueryRow(query, userID, title).Scan(&tag.ID, &tag.UserID, &tag.Title)

	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf(`store: unable to fetch user tag: %v`, err)
	default:
		return &tag, nil
	}
}

// UserTags returns all user tags with entry counts.
func (s *Storage) UserTags(userID int64) (model.UserTags, error) {
	query := `
		SELECT
			ut.id,
			ut.user_id,
			ut.title,
			(SELECT count(*) FROM entry_user_tags eut WHERE eut.user_tag_id = ut.id) AS entry_count
		FROM user_tags ut
		WHERE ut.user_id = $1
		ORDER BY ut.title ASC
	`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch user tags: %v`, err)
	}
	defer rows.Close()

	tags := make(model.UserTags, 0)
	for rows.Next() {
		var tag model.UserTag
		if err := rows.Scan(&tag.ID, &tag.UserID, &tag.Title, &tag.EntryCount); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch user tag row: %v`, err)
		}
		tags = append(tags, &tag)
	}

	return tags, nil
}

// UserTagIDExists checks if the given user tag exists.
func (s *Storage) UserTagIDExists(userID, tagID int64) bool {
	var result bool
	query := `SELECT true FROM user_tags WHERE user_id=$1 AND id=$2 LIMIT 1`
	s.db.QueryRow(query, userID, tagID).Scan(&result)
	return result
}

// UserTagTitleExists checks if a user tag with the given title exists.
func (s *Storage) UserTagTitleExists(userID int64, title string) bool {
	var result bool
	query := `SELECT true FROM user_tags WHERE user_id=$1 AND lower(title)=lower($2) LIMIT 1`
	s.db.QueryRow(query, userID, title).Scan(&result)
	return result
}

// AnotherUserTagExists checks if another user tag exists with the same title.
func (s *Storage) AnotherUserTagExists(userID, tagID int64, title string) bool {
	var result bool
	query := `SELECT true FROM user_tags WHERE user_id=$1 AND id != $2 AND lower(title)=lower($3) LIMIT 1`
	s.db.QueryRow(query, userID, tagID, title).Scan(&result)
	return result
}

// CreateUserTag creates a new user tag.
func (s *Storage) CreateUserTag(userID int64, request *model.UserTagCreationRequest) (*model.UserTag, error) {
	var tag model.UserTag

	query := `
		INSERT INTO user_tags
			(user_id, title)
		VALUES
			($1, $2)
		RETURNING
			id,
			user_id,
			title
	`
	err := s.db.QueryRow(
		query,
		userID,
		request.Title,
	).Scan(
		&tag.ID,
		&tag.UserID,
		&tag.Title,
	)

	if err != nil {
		return nil, fmt.Errorf(`store: unable to create user tag %q for user ID %d: %v`, request.Title, userID, err)
	}

	return &tag, nil
}

// UpdateUserTag updates an existing user tag.
func (s *Storage) UpdateUserTag(tag *model.UserTag) error {
	query := `UPDATE user_tags SET title=$1 WHERE id=$2 AND user_id=$3`
	_, err := s.db.Exec(query, tag.Title, tag.ID, tag.UserID)

	if err != nil {
		return fmt.Errorf(`store: unable to update user tag: %v`, err)
	}

	return nil
}

// RemoveUserTag deletes a user tag and all its entry associations.
func (s *Storage) RemoveUserTag(userID, tagID int64) error {
	query := `DELETE FROM user_tags WHERE id = $1 AND user_id = $2`
	result, err := s.db.Exec(query, tagID, userID)
	if err != nil {
		return fmt.Errorf(`store: unable to remove this user tag: %v`, err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf(`store: unable to remove this user tag: %v`, err)
	}

	if count == 0 {
		return errors.New(`store: no user tag has been removed`)
	}

	return nil
}

// EntryUserTagIDs returns the IDs of user tags assigned to an entry.
func (s *Storage) EntryUserTagIDs(userID, entryID int64) ([]int64, error) {
	query := `
		SELECT eut.user_tag_id
		FROM entry_user_tags eut
		JOIN user_tags ut ON ut.id = eut.user_tag_id
		WHERE ut.user_id = $1 AND eut.entry_id = $2
	`
	rows, err := s.db.Query(query, userID, entryID)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch entry user tag IDs: %v`, err)
	}
	defer rows.Close()

	tagIDs := make([]int64, 0)
	for rows.Next() {
		var tagID int64
		if err := rows.Scan(&tagID); err != nil {
			return nil, fmt.Errorf(`store: unable to fetch entry user tag ID row: %v`, err)
		}
		tagIDs = append(tagIDs, tagID)
	}

	return tagIDs, nil
}

// SetEntryUserTags replaces all user tags for an entry.
func (s *Storage) SetEntryUserTags(userID, entryID int64, tagIDs []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf(`store: unable to begin transaction: %v`, err)
	}

	// Delete existing user tag associations for this entry (scoped to user's tags).
	_, err = tx.Exec(`
		DELETE FROM entry_user_tags
		WHERE entry_id = $1
		AND user_tag_id IN (SELECT id FROM user_tags WHERE user_id = $2)
	`, entryID, userID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf(`store: unable to clear entry user tags: %v`, err)
	}

	// Insert new associations.
	for _, tagID := range tagIDs {
		_, err = tx.Exec(`
			INSERT INTO entry_user_tags (entry_id, user_tag_id)
			VALUES ($1, $2)
		`, entryID, tagID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf(`store: unable to set entry user tag: %v`, err)
		}
	}

	return tx.Commit()
}
