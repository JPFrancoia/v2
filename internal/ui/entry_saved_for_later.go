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

func (h *handler) showSavedForLaterEntryPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	entryID := request.RouteInt64Param(r, "entryID")
	builder := h.store.NewEntryQueryBuilder(user.ID)
	builder.WithEntryID(entryID)
	builder.WithSavedForLater(true)
	builder.WithStatus(model.EntryStatusUnread)
	builder.WithGloballyVisible()

	entry, err := builder.GetEntry()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if entry == nil {
		response.HTMLRedirect(w, r, h.routePath("/saved-for-later"))
		return
	}

	entryPaginationBuilder := storage.NewEntryPaginationBuilder(h.store, user.ID, entry.ID, user.EntryOrder, user.EntryDirection)
	entryPaginationBuilder.WithSavedForLater()
	entryPaginationBuilder.WithStatus(model.EntryStatusUnread)
	entryPaginationBuilder.WithGloballyVisible()
	prevEntry, nextEntry, err := entryPaginationBuilder.Entries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	nextEntryRoute := ""
	if nextEntry != nil {
		nextEntryRoute = h.routePath("/saved-for-later/entry/%d", nextEntry.ID)
	}

	prevEntryRoute := ""
	if prevEntry != nil {
		prevEntryRoute = h.routePath("/saved-for-later/entry/%d", prevEntry.ID)
	}

	if entry.ShouldMarkAsReadOnView(user) {
		err = h.store.SetEntriesStatus(user.ID, []int64{entry.ID}, model.EntryStatusRead)
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}

		entry.Status = model.EntryStatusRead
		entry.SavedForLater = false
	}

	if user.AlwaysOpenExternalLinks {
		response.HTMLRedirect(w, r, entry.URL)
		return
	}

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

	view := view.New(h.tpl, r)
	view.Set("entry", entry)
	view.Set("prevEntry", prevEntry)
	view.Set("nextEntry", nextEntry)
	view.Set("nextEntryRoute", nextEntryRoute)
	view.Set("prevEntryRoute", prevEntryRoute)
	view.Set("menu", "saved_for_later")
	view.Set("user", user)
	view.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))
	view.Set("hasSaveEntry", h.store.HasSaveEntry(user.ID))
	view.Set("userTags", userTags)
	view.Set("entryUserTagIDs", entryUserTagIDs)

	response.HTML(w, r, view.Render("entry"))
}
