// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/ui/view"
)

func (h *handler) showCreateUserTagPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(r.Context(), request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	v := view.New(h.tpl, r)
	v.Set("menu", "settings")
	v.Set("user", user)
	v.Set("countUnread", h.store.CountUnreadEntries(r.Context(), user.ID))
	v.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(r.Context(), user.ID))

	response.HTML(w, r, v.Render("create_user_tag"))
}
