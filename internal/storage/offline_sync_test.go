// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"testing"

	"miniflux.app/v2/internal/model"
)

func TestApplyOfflineScalarValuesIsRetrySafe(t *testing.T) {
	status := model.EntryStatusRead
	saved, starred, vote := false, true, 1
	current := &model.OfflineEntryState{Status: status, SavedForLater: saved, Starred: starred, Vote: vote}
	final := *current
	patch := &model.OfflineEntryPatch{
		Base: model.OfflineEntryValues{Status: &status, SavedForLater: &saved, Starred: &starred, Vote: &vote},
		Set:  model.OfflineEntryValues{Status: &status, SavedForLater: &saved, Starred: &starred, Vote: &vote},
	}
	result := &model.OfflineEntryPatchResult{}

	if changed := applyOfflineScalarValues(&final, current, patch, result); changed {
		t.Fatal(`Expected an identical retry to remain unchanged`)
	}
	if len(result.Conflicts) != 0 {
		t.Fatalf(`Expected no retry conflict, got %d`, len(result.Conflicts))
	}
}

func TestApplyOfflineScalarValuesReportsOnlyConflictingFields(t *testing.T) {
	baseVote, setVote := 0, 1
	baseStarred, setStarred := false, true
	current := &model.OfflineEntryState{Status: model.EntryStatusUnread, Vote: -1, Starred: false}
	final := *current
	patch := &model.OfflineEntryPatch{
		Base: model.OfflineEntryValues{Vote: &baseVote, Starred: &baseStarred},
		Set:  model.OfflineEntryValues{Vote: &setVote, Starred: &setStarred},
	}
	result := &model.OfflineEntryPatchResult{}

	if changed := applyOfflineScalarValues(&final, current, patch, result); !changed {
		t.Fatal(`Expected the non-conflicting starred field to change`)
	}
	if !final.Starred {
		t.Fatal(`Expected the starred field to use the client value`)
	}
	if final.Vote != -1 {
		t.Fatalf(`Expected the server vote to remain -1, got %d`, final.Vote)
	}
	if len(result.Conflicts) != 1 || result.Conflicts[0].Field != "vote" {
		t.Fatalf(`Expected one vote conflict, got %#v`, result.Conflicts)
	}
}
