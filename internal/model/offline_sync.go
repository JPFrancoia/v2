// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import "time"

const (
	OfflineSyncResultApplied   = "applied"
	OfflineSyncResultUnchanged = "unchanged"
	OfflineSyncResultConflict  = "conflict"
	OfflineSyncResultNotFound  = "not_found"
)

// OfflineEntryValues contains optional scalar entry values in an offline patch.
type OfflineEntryValues struct {
	Status        *string `json:"status,omitempty"`
	Starred       *bool   `json:"starred,omitempty"`
	SavedForLater *bool   `json:"saved_for_later,omitempty"`
	Vote          *int    `json:"vote,omitempty"`
}

// OfflineEntryPatch contains one retry-safe entry change from an offline client.
type OfflineEntryPatch struct {
	EntryID          int64              `json:"entry_id"`
	Base             OfflineEntryValues `json:"base"`
	Set              OfflineEntryValues `json:"set"`
	AddUserTagIDs    []int64            `json:"add_user_tag_ids,omitempty"`
	RemoveUserTagIDs []int64            `json:"remove_user_tag_ids,omitempty"`
}

// OfflineSyncRequest contains a bounded batch of offline entry patches.
type OfflineSyncRequest struct {
	Entries []OfflineEntryPatch `json:"entries"`
}

// OfflineEntryState contains the server state returned after a patch.
type OfflineEntryState struct {
	Status        string    `json:"status"`
	Starred       bool      `json:"starred"`
	SavedForLater bool      `json:"saved_for_later"`
	Vote          int       `json:"vote"`
	UserTagIDs    []int64   `json:"user_tag_ids"`
	ChangedAt     time.Time `json:"changed_at"`
}

// OfflineEntryConflict describes one scalar field conflict.
type OfflineEntryConflict struct {
	Field  string `json:"field"`
	Server any    `json:"server"`
	Client any    `json:"client"`
}

// OfflineEntryPatchResult contains the result for one entry patch.
type OfflineEntryPatchResult struct {
	EntryID   int64                  `json:"entry_id"`
	Result    string                 `json:"result"`
	State     *OfflineEntryState     `json:"state,omitempty"`
	Conflicts []OfflineEntryConflict `json:"conflicts,omitempty"`
}

// OfflineSyncResponse contains one result for each submitted patch.
type OfflineSyncResponse struct {
	Entries []OfflineEntryPatchResult `json:"entries"`
}

// OfflineManifestUserTag contains one user tag and its cached entry IDs.
type OfflineManifestUserTag struct {
	ID       int64   `json:"id"`
	Title    string  `json:"title"`
	EntryIDs []int64 `json:"entry_ids"`
}

// OfflineManifest identifies the entries that belong in the offline cache.
type OfflineManifest struct {
	UserID                int64                    `json:"user_id"`
	GeneratedAt           time.Time                `json:"generated_at"`
	UnreadEntryIDs        []int64                  `json:"unread_entry_ids"`
	SavedForLaterEntryIDs []int64                  `json:"saved_for_later_entry_ids"`
	StarredEntryIDs       []int64                  `json:"starred_entry_ids"`
	HistoryEntryIDs       []int64                  `json:"history_entry_ids"`
	UserTags              []OfflineManifestUserTag `json:"user_tags"`
	EntryVersions         map[int64]time.Time      `json:"entry_versions"`
}
