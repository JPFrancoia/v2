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

func (h *handler) showUnreadEntryPage(w http.ResponseWriter, r *http.Request) {
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
		response.HTMLRedirect(w, r, h.routePath("/unread"))
		return
	}

	// Make sure we always get the pagination in unread mode even if the page is refreshed.
	if entry.Status == model.EntryStatusRead {
		err = h.store.SetEntriesStatus(r.Context(), user.ID, []int64{entry.ID}, model.EntryStatusUnread)
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}
	}

	order, direction := entryListSorting(r, user)
	entryPaginationBuilder := storage.NewEntryPaginationBuilder(r.Context(), h.store, user.ID, entry.ID, order, direction)
	entryPaginationBuilder.WithStatus(model.EntryStatusUnread)
	entryPaginationBuilder.WithGloballyVisible()
	prevEntry, nextEntry, err := entryPaginationBuilder.Entries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	nextEntryRoute := ""
	if nextEntry != nil {
		nextEntryRoute = h.routePath("/unread/entry/%d", nextEntry.ID)
	}

	prevEntryRoute := ""
	if prevEntry != nil {
		prevEntryRoute = h.routePath("/unread/entry/%d", prevEntry.ID)
	}

	if entry.ShouldMarkAsReadOnView(user) {
		entry.Status = model.EntryStatusRead
	}

	// Restore entry read status if needed after fetching the pagination.
	if entry.Status == model.EntryStatusRead {
		err = h.store.SetEntriesStatus(r.Context(), user.ID, []int64{entry.ID}, model.EntryStatusRead)
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}
	}

	if user.AlwaysOpenExternalLinks {
		response.HTMLRedirect(w, r, entry.URL)
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

	view := view.New(h.tpl, r)
	view.Set("entry", entry)
	view.Set("prevEntry", prevEntry)
	view.Set("nextEntry", nextEntry)
	view.Set("nextEntryRoute", nextEntryRoute)
	view.Set("prevEntryRoute", prevEntryRoute)
	view.Set("sortOrder", order)
	view.Set("sortDirection", direction)
	view.Set("menu", "unread")
	view.Set("user", user)
	view.Set("hasSaveEntry", h.store.HasSaveEntry(r.Context(), user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(r.Context(), user.ID))

	// Fetching the counter here avoid to be off by one.
	view.Set("countUnread", h.store.CountUnreadEntries(r.Context(), user.ID))
	view.Set("userTags", userTags)
	view.Set("entryUserTagIDs", entryUserTagIDs)

	response.HTML(w, r, view.Render("entry"))
}
