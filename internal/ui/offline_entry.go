// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	json_parser "encoding/json"
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/ui/view"
	"miniflux.app/v2/internal/validator"
)

func (h *handler) showOfflineEntry(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(r.Context(), request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	entryID := request.RouteInt64Param(r, "entryID")
	builder := h.store.NewEntryQueryBuilder(r.Context(), user.ID)
	builder.WithEntryID(entryID)
	entry, err := builder.GetEntry()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}
	if entry == nil {
		response.HTMLNotFound(w, r)
		return
	}

	userTags, err := h.store.UserTags(r.Context(), user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}
	entryUserTagIDs, err := h.store.EntryUserTagIDs(r.Context(), user.ID, entry.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	response.HTML(w, r, h.renderOfflineEntrySnapshot(r, user, userTags, entry, entryUserTagIDs, h.store.HasSaveEntry(r.Context(), user.ID)))
}

func (h *handler) showOfflineEntries(w http.ResponseWriter, r *http.Request) {
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

	user, err := h.store.UserByID(r.Context(), request.UserID(r))
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	userTags, err := h.store.UserTags(r.Context(), user.ID)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	entries, err := h.store.NewEntryQueryBuilder(r.Context(), user.ID).WithEntryIDs(snapshotRequest.EntryIDs).GetEntries()
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	entryUserTagIDs, err := h.store.OfflineEntryUserTagIDs(r.Context(), user.ID, snapshotRequest.EntryIDs)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	hasSaveEntry := h.store.HasSaveEntry(r.Context(), user.ID)

	snapshotResponse := model.OfflineSnapshotResponse{Entries: make([]model.OfflineSnapshot, 0, len(entries))}
	for _, entry := range entries {
		entry.Enclosures, err = h.store.GetEnclosures(r.Context(), entry.ID)
		if err != nil {
			response.JSONServerError(w, r, err)
			return
		}
		snapshotResponse.Entries = append(snapshotResponse.Entries, model.OfflineSnapshot{
			EntryID: entry.ID,
			HTML:    string(h.renderOfflineEntrySnapshot(r, user, userTags, entry, entryUserTagIDs[entry.ID], hasSaveEntry)),
		})
	}
	response.JSON(w, r, snapshotResponse)
}

func (h *handler) renderOfflineEntrySnapshot(r *http.Request, user *model.User, userTags model.UserTags, entry *model.Entry, entryUserTagIDs []int64, hasSaveEntry bool) []byte {
	view := view.New(h.tpl, r)
	view.Set("entry", entry)
	view.Set("offlineSnapshot", true)
	view.Set("menu", "unread")
	view.Set("user", user)
	view.Set("countUnread", 0)
	view.Set("countErrorFeeds", 0)
	view.Set("hasSaveEntry", hasSaveEntry)
	view.Set("userTags", userTags)
	view.Set("entryUserTagIDs", entryUserTagIDs)
	return view.Render("entry")
}
