// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/ui/form"
	"miniflux.app/v2/internal/ui/view"
)

func (h *handler) showEditUserTagPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	tag, err := h.store.UserTagByID(request.UserID(r), request.RouteInt64Param(r, "userTagID"))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if tag == nil {
		response.HTMLNotFound(w, r)
		return
	}

	tagForm := form.UserTagForm{
		Title: tag.Title,
	}

	v := view.New(h.tpl, r)
	v.Set("form", tagForm)
	v.Set("tag", tag)
	v.Set("menu", "settings")
	v.Set("user", user)
	v.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	v.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))

	response.HTML(w, r, v.Render("edit_user_tag"))
}
