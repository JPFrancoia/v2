// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/ui/view"
)

func (h *handler) showUserTagEntriesPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(r.Context(), request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	userTagID := request.RouteInt64Param(r, "userTagID")
	tag, err := h.store.UserTagByID(r.Context(), user.ID, userTagID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if tag == nil {
		response.HTMLNotFound(w, r)
		return
	}

	offset := request.QueryIntParam(r, "offset", 0)
	builder := h.store.NewEntryQueryBuilder(r.Context(), user.ID)
	builder.WithUserTagID(userTagID)
	builder.WithSorting("status", "asc")
	builder.WithSorting(user.EntryOrder, user.EntryDirection)
	builder.WithSorting("id", user.EntryDirection)
	builder.WithOffset(offset)
	builder.WithLimit(user.EntriesPerPage)
	builder.WithContentPreview()

	entries, err := builder.GetEntries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	count, err := builder.CountEntries()
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	v := view.New(h.tpl, r)
	v.Set("tag", tag)
	v.Set("total", count)
	v.Set("entries", entries)
	v.Set("pagination", getPagination(h.routePath("/user-tag/%d/entries", userTagID), count, offset, user.EntriesPerPage))
	v.Set("menu", "tags")
	v.Set("user", user)
	v.Set("countUnread", h.store.CountUnreadEntries(r.Context(), user.ID))
	v.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(r.Context(), user.ID))
	v.Set("hasSaveEntry", h.store.HasSaveEntry(r.Context(), user.ID))
	v.Set("showOnlyUnreadEntries", false)

	response.HTML(w, r, v.Render("user_tag_entries"))
}
