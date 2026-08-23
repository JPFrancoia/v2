// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"testing"

	"miniflux.app/v2/internal/model"
)

func TestValidateOfflineSnapshotRequest(t *testing.T) {
	valid := &model.OfflineSnapshotRequest{EntryIDs: []int64{1, 2}}
	if err := ValidateOfflineSnapshotRequest(valid); err != nil {
		t.Fatalf(`Unexpected validation error: %v`, err)
	}

	for _, request := range []*model.OfflineSnapshotRequest{
		{EntryIDs: nil},
		{EntryIDs: []int64{0}},
		{EntryIDs: make([]int64, maxOfflineSnapshotBatchSize+1)},
	} {
		if err := ValidateOfflineSnapshotRequest(request); err == nil {
			t.Fatal(`Expected an invalid snapshot request error`)
		}
	}
}

func TestValidateOfflineSyncRequest(t *testing.T) {
	baseStatus, setStatus := model.EntryStatusUnread, model.EntryStatusRead
	baseSaved, setSaved := true, false
	vote := 1

	request := &model.OfflineSyncRequest{Entries: []model.OfflineEntryPatch{{
		EntryID: 42,
		Base: model.OfflineEntryValues{
			Status:        &baseStatus,
			SavedForLater: &baseSaved,
		},
		Set: model.OfflineEntryValues{
			Status:        &setStatus,
			SavedForLater: &setSaved,
			Vote:          &vote,
		},
	}}}

	if err := ValidateOfflineSyncRequest(request); err == nil {
		t.Fatal(`Expected a missing vote base value error`)
	}

	baseVote := 0
	request.Entries[0].Base.Vote = &baseVote
	if err := ValidateOfflineSyncRequest(request); err != nil {
		t.Fatalf(`Unexpected validation error: %v`, err)
	}
}

func TestValidateOfflineSyncRequestRejectsTooManyTagChanges(t *testing.T) {
	tagIDs := make([]int64, maxOfflineTagChanges+1)
	for i := range tagIDs {
		tagIDs[i] = int64(i + 1)
	}
	request := &model.OfflineSyncRequest{Entries: []model.OfflineEntryPatch{{EntryID: 42, AddUserTagIDs: tagIDs}}}

	if err := ValidateOfflineSyncRequest(request); err == nil {
		t.Fatal(`Expected a user tag change limit error`)
	}
}

func TestValidateOfflineSyncRequestRejectsInvalidInvariant(t *testing.T) {
	status := model.EntryStatusRead
	saved := true
	request := &model.OfflineSyncRequest{Entries: []model.OfflineEntryPatch{{
		EntryID: 42,
		Base: model.OfflineEntryValues{
			Status:        &status,
			SavedForLater: &saved,
		},
		Set: model.OfflineEntryValues{
			Status:        &status,
			SavedForLater: &saved,
		},
	}}}

	if err := ValidateOfflineSyncRequest(request); err == nil {
		t.Fatal(`Expected a read and saved-for-later invariant error`)
	}
}
