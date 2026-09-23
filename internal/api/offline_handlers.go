// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api // import "miniflux.app/v2/internal/api"

import (
	json_parser "encoding/json"
	"net/http"
	"time"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/validator"
)

const offlineDeviceSnapshotVersion = "1"

// getOfflineManifestHandler returns the authenticated user's offline entry set.
// The device version is independent of browser themes and sessions.
func (h *handler) getOfflineManifestHandler(w http.ResponseWriter, r *http.Request) {
	manifest, err := h.store.OfflineManifest(r.Context(), request.UserID(r), time.Now())
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	manifest.SnapshotVersion = offlineDeviceSnapshotVersion
	response.JSON(w, r, manifest)
}

// getOfflineEntriesHandler returns bounded, user-scoped article data for native clients.
// It does not render the browser's session-dependent HTML pages.
func (h *handler) getOfflineEntriesHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var snapshotRequest model.OfflineSnapshotRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&snapshotRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	if err := validator.ValidateOfflineSnapshotRequest(&snapshotRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	userID := request.UserID(r)
	entries, err := h.store.NewEntryQueryBuilder(r.Context(), userID).WithEntryIDs(snapshotRequest.EntryIDs).GetEntries()
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	userTagIDs, err := h.store.OfflineEntryUserTagIDs(r.Context(), userID, snapshotRequest.EntryIDs)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	snapshotResponse := model.OfflineDeviceSnapshotResponse{Entries: make([]model.OfflineDeviceEntrySnapshot, 0, len(entries))}
	for _, entry := range entries {
		snapshotResponse.Entries = append(snapshotResponse.Entries, newOfflineDeviceEntrySnapshot(entry, userTagIDs[entry.ID]))
	}
	response.JSON(w, r, &snapshotResponse)
}

// newOfflineDeviceEntrySnapshot selects the article fields needed for offline reading.
// It preserves the stored, sanitized content and assigned user tags.
func newOfflineDeviceEntrySnapshot(entry *model.Entry, userTagIDs []int64) model.OfflineDeviceEntrySnapshot {
	return model.OfflineDeviceEntrySnapshot{
		EntryID:       entry.ID,
		Version:       entry.ChangedAt,
		FeedID:        entry.FeedID,
		FeedTitle:     entry.Feed.Title,
		CategoryID:    entry.Feed.Category.ID,
		CategoryTitle: entry.Feed.Category.Title,
		Title:         entry.Title,
		Author:        entry.Author,
		URL:           entry.URL,
		PublishedAt:   entry.Date,
		Content:       entry.Content,
		Status:        entry.Status,
		Starred:       entry.Starred,
		SavedForLater: entry.SavedForLater,
		Vote:          entry.Vote,
		UserTagIDs:    userTagIDs,
	}
}

// syncOfflineEntriesHandler applies bounded device patches for the authenticated user.
// It uses the same conflict and retry rules as the browser's offline sync.
func (h *handler) syncOfflineEntriesHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var syncRequest model.OfflineSyncRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&syncRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	if err := validator.ValidateOfflineSyncRequest(&syncRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	userID := request.UserID(r)
	syncResponse := model.OfflineSyncResponse{Entries: make([]model.OfflineEntryPatchResult, 0, len(syncRequest.Entries))}
	for i := range syncRequest.Entries {
		result, err := h.store.ApplyOfflineEntryPatch(r.Context(), userID, &syncRequest.Entries[i])
		if err != nil {
			response.JSONServerError(w, r, err)
			return
		}
		syncResponse.Entries = append(syncResponse.Entries, result)
	}
	response.JSON(w, r, &syncResponse)
}
