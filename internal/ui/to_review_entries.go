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

const toReviewScoreTarget int64 = 50

func (h *handler) showToReviewPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	offset := request.QueryIntParam(r, "offset", 0)
	builder := h.store.NewEntryQueryBuilder(user.ID)
	builder.WithStatus(model.EntryStatusUnread)
	builder.WithVote(0)
	builder.WithScoreDistanceSorting(toReviewScoreTarget)
	builder.WithSorting("published_at", "DESC")
	builder.WithSorting("id", "DESC")
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
		builder.WithStatus(model.EntryStatusUnread)
		builder.WithVote(0)
		builder.WithScoreDistanceSorting(toReviewScoreTarget)
		builder.WithSorting("published_at", "DESC")
		builder.WithSorting("id", "DESC")
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
	view.Set("pagination", getPagination(h.routePath("/to-review"), count, offset, user.EntriesPerPage))
	view.Set("menu", "to_review")
	view.Set("user", user)
	view.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))
	view.Set("hasSaveEntry", h.store.HasSaveEntry(user.ID))

	response.HTML(w, r, view.Render("to_review_entries"))
}
