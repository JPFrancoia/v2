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

func (h *handler) showSearchPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(r.Context(), request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	searchQuery := request.QueryStringParam(r, "q", "")
	unreadOnly := request.QueryBoolParam(r, "unread", false)
	offset := request.QueryIntParam(r, "offset", 0)

	var entries model.Entries
	var entriesCount int

	if searchQuery != "" {
		builder := h.store.NewEntryQueryBuilder(r.Context(), user.ID)
		builder.WithSearchQuery(searchQuery)
		if unreadOnly {
			builder.WithStatus(model.EntryStatusUnread)
		}
		builder.WithContentPreview()
		builder.WithOffset(offset)
		builder.WithLimit(user.EntriesPerPage)

		entries, entriesCount, err = builder.GetEntriesWithCount()
		if err != nil {
			response.HTMLServerError(w, r, err)
			return
		}
	}

	view := view.New(h.tpl, r)
	pagination := getPagination(h.routePath("/search"), entriesCount, offset, user.EntriesPerPage)
	pagination.SearchQuery = searchQuery
	pagination.UnreadOnly = unreadOnly

	view.Set("searchQuery", searchQuery)
	view.Set("searchUnreadOnly", unreadOnly)
	view.Set("entries", entries)
	view.Set("total", entriesCount)
	view.Set("pagination", pagination)
	view.Set("menu", "search")
	view.Set("user", user)
	view.Set("countUnread", h.store.CountUnreadEntries(r.Context(), user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(r.Context(), user.ID))
	view.Set("hasSaveEntry", h.store.HasSaveEntry(r.Context(), user.ID))

	response.HTML(w, r, view.Render("search"))
}
