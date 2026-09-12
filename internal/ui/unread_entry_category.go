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

func (h *handler) showUnreadCategoryEntryPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(r.Context(), request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	categoryID := request.RouteInt64Param(r, "categoryID")
	entryID := request.RouteInt64Param(r, "entryID")

	builder := h.store.NewEntryQueryBuilder(r.Context(), user.ID)
	builder.WithCategoryID(categoryID)
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
		err = h.store.SetEntriesStatus(r.Context(), user.ID, []int64{entry.ID}, model.EntryStatusRead)
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}

		entry.Status = model.EntryStatusRead
	}

	entryPaginationBuilder := storage.NewEntryPaginationBuilder(r.Context(), h.store, user.ID, entry.ID, user.EntryOrder, user.EntryDirection)
	entryPaginationBuilder.WithCategoryID(categoryID)
	entryPaginationBuilder.WithStatus(model.EntryStatusUnread)

	if entry.Status == model.EntryStatusRead {
		err = h.store.SetEntriesStatus(r.Context(), user.ID, []int64{entry.ID}, model.EntryStatusUnread)
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}
	}

	prevEntry, nextEntry, err := entryPaginationBuilder.Entries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	nextEntryRoute := ""
	if nextEntry != nil {
		nextEntryRoute = h.routePath("/unread/category/%d/entry/%d", categoryID, nextEntry.ID)
	}

	prevEntryRoute := ""
	if prevEntry != nil {
		prevEntryRoute = h.routePath("/unread/category/%d/entry/%d", categoryID, prevEntry.ID)
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
	view.Set("menu", "categories")
	view.Set("user", user)
	view.Set("countUnread", h.store.CountUnreadEntries(r.Context(), user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(r.Context(), user.ID))
	view.Set("hasSaveEntry", h.store.HasSaveEntry(r.Context(), user.ID))
	view.Set("userTags", userTags)
	view.Set("entryUserTagIDs", entryUserTagIDs)

	response.HTML(w, r, view.Render("entry"))
}
