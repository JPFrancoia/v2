// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/ui/view"
)

func (h *handler) showSavedForLaterPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	offset := request.QueryIntParam(r, "offset", 0)
	order, direction := entryListSorting(r, user)
	builder := h.store.NewEntryQueryBuilder(user.ID)
	builder.WithSavedForLater(true)
	builder.WithStatus(model.EntryStatusUnread)
	builder.WithSorting(order, direction)
	builder.WithSorting("id", direction)
	builder.WithOffset(offset)
	builder.WithLimit(user.EntriesPerPage)
	builder.WithGloballyVisible()
	builder.WithoutContent()

	entries, count, err := builder.GetEntriesWithCount()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if offset >= count && count > 0 {
		offset = 0
		builder = h.store.NewEntryQueryBuilder(user.ID)
		builder.WithSavedForLater(true)
		builder.WithStatus(model.EntryStatusUnread)
		builder.WithSorting(order, direction)
		builder.WithSorting("id", direction)
		builder.WithLimit(user.EntriesPerPage)
		builder.WithGloballyVisible()
		builder.WithoutContent()

		entries, count, err = builder.GetEntriesWithCount()
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}
	}

	view := view.New(h.tpl, r)
	view.Set("total", count)
	view.Set("entries", entries)
	pagination := getPagination(h.routePath("/saved-for-later"), count, offset, user.EntriesPerPage)
	pagination.Order = order
	pagination.Direction = direction
	view.Set("pagination", pagination)
	view.Set("sortOrder", order)
	view.Set("sortDirection", direction)
	view.Set("scoreSortDirection", nextEntryListSortDirection(order, direction, "score"))
	view.Set("publishedSortDirection", nextEntryListSortDirection(order, direction, "published_at"))
	view.Set("menu", "saved_for_later")
	view.Set("user", user)
	view.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))
	view.Set("hasSaveEntry", h.store.HasSaveEntry(user.ID))

	response.HTML(w, r, view.Render("saved_for_later_entries"))
}
