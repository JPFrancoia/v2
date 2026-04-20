// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
)

func (h *handler) removeUserTag(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	tagID := request.RouteInt64Param(r, "userTagID")
	tag, err := h.store.UserTagByID(request.UserID(r), tagID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if tag == nil {
		response.HTMLNotFound(w, r)
		return
	}

	if err := h.store.RemoveUserTag(user.ID, tag.ID); err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	response.HTMLRedirect(w, r, h.routePath("/user-tags"))
}
