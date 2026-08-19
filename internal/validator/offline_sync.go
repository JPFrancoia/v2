// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package validator // import "miniflux.app/v2/internal/validator"

import (
	"errors"
	"fmt"

	"miniflux.app/v2/internal/model"
)

const (
	maxOfflineSyncBatchSize = 100
	maxOfflineTagChanges    = 100
)

// ValidateOfflineSyncRequest validates an offline synchronization batch.
func ValidateOfflineSyncRequest(request *model.OfflineSyncRequest) error {
	if len(request.Entries) == 0 {
		return errors.New(`the offline synchronization batch cannot be empty`)
	}
	if len(request.Entries) > maxOfflineSyncBatchSize {
		return fmt.Errorf(`the offline synchronization batch cannot contain more than %d entries`, maxOfflineSyncBatchSize)
	}

	for i := range request.Entries {
		if err := validateOfflineEntryPatch(&request.Entries[i]); err != nil {
			return fmt.Errorf(`invalid offline entry patch at index %d: %v`, i, err)
		}
	}
	return nil
}

func validateOfflineEntryPatch(patch *model.OfflineEntryPatch) error {
	if patch.EntryID <= 0 {
		return errors.New(`the entry ID must be greater than zero`)
	}
	if !hasOfflineEntryChange(patch) {
		return errors.New(`the patch must contain a change`)
	}
	if len(patch.AddUserTagIDs)+len(patch.RemoveUserTagIDs) > maxOfflineTagChanges {
		return fmt.Errorf(`the patch cannot contain more than %d user tag changes`, maxOfflineTagChanges)
	}
	if patch.Set.Status != nil {
		if err := ValidateEntryStatus(*patch.Set.Status); err != nil {
			return err
		}
		if patch.Base.Status == nil || patch.Set.SavedForLater == nil || patch.Base.SavedForLater == nil {
			return errors.New(`status and saved-for-later values must include complete base and set states`)
		}
	}
	if patch.Set.SavedForLater != nil {
		if patch.Base.SavedForLater == nil || patch.Set.Status == nil || patch.Base.Status == nil {
			return errors.New(`saved-for-later and status values must include complete base and set states`)
		}
	}
	if patch.Set.Starred != nil && patch.Base.Starred == nil {
		return errors.New(`starred changes must include the base value`)
	}
	if patch.Set.Vote != nil {
		if patch.Base.Vote == nil {
			return errors.New(`vote changes must include the base value`)
		}
		if *patch.Set.Vote < -1 || *patch.Set.Vote > 1 {
			return errors.New(`vote value must be -1, 0, or 1`)
		}
	}
	if patch.Set.Status != nil {
		if *patch.Set.Status == model.EntryStatusRead && *patch.Set.SavedForLater {
			return errors.New(`a read entry cannot be saved for later`)
		}
		if *patch.Set.SavedForLater && *patch.Set.Status != model.EntryStatusUnread {
			return errors.New(`an entry saved for later must be unread`)
		}
	}
	for _, tagID := range append(patch.AddUserTagIDs, patch.RemoveUserTagIDs...) {
		if tagID <= 0 {
			return errors.New(`user tag IDs must be greater than zero`)
		}
	}
	return nil
}

func hasOfflineEntryChange(patch *model.OfflineEntryPatch) bool {
	return patch.Set.Status != nil || patch.Set.Starred != nil || patch.Set.SavedForLater != nil || patch.Set.Vote != nil ||
		len(patch.AddUserTagIDs) > 0 || len(patch.RemoveUserTagIDs) > 0
}
