// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
	"miniflux.app/v2/internal/ui/view"
)

func (h *handler) showUserTagEntryPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	userTagID := request.RouteInt64Param(r, "userTagID")
	tag, err := h.store.UserTagByID(user.ID, userTagID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if tag == nil {
		response.HTMLNotFound(w, r)
		return
	}

	entryID := request.RouteInt64Param(r, "entryID")

	builder := h.store.NewEntryQueryBuilder(user.ID)
	builder.WithUserTagID(userTagID)
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

	if entry.ShouldMarkAsReadOnView(user) {
		err = h.store.SetEntriesStatus(user.ID, []int64{entry.ID}, model.EntryStatusRead)
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}

		entry.Status = model.EntryStatusRead
	}

	entryPaginationBuilder := storage.NewEntryPaginationBuilder(h.store, user.ID, entry.ID, user.EntryOrder, user.EntryDirection)
	entryPaginationBuilder.WithUserTagID(userTagID)
	prevEntry, nextEntry, err := entryPaginationBuilder.Entries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	nextEntryRoute := ""
	if nextEntry != nil {
		nextEntryRoute = h.routePath("/user-tag/%d/entry/%d", userTagID, nextEntry.ID)
	}

	prevEntryRoute := ""
	if prevEntry != nil {
		prevEntryRoute = h.routePath("/user-tag/%d/entry/%d", userTagID, prevEntry.ID)
	}

	// Fetch user tags for the checkbox section on the entry detail page.
	userTags, err := h.store.UserTags(user.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	entryUserTagIDs, err := h.store.EntryUserTagIDs(user.ID, entry.ID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	v := view.New(h.tpl, r)
	v.Set("entry", entry)
	v.Set("prevEntry", prevEntry)
	v.Set("nextEntry", nextEntry)
	v.Set("nextEntryRoute", nextEntryRoute)
	v.Set("prevEntryRoute", prevEntryRoute)
	v.Set("user", user)
	v.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	v.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))
	v.Set("hasSaveEntry", h.store.HasSaveEntry(user.ID))
	v.Set("userTags", userTags)
	v.Set("entryUserTagIDs", entryUserTagIDs)

	response.HTML(w, r, v.Render("entry"))
}
