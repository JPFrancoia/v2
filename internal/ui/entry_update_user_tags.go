// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"
	"strconv"
	"strings"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response/html"
	"miniflux.app/v2/internal/http/response/json"
)

func (h *handler) updateEntryUserTags(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		html.ServerError(w, r, err)
		return
	}

	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		html.ServerError(w, r, err)
		return
	}

	entryID := request.RouteInt64Param(r, "entryID")

	// Parse selected tag IDs from checkbox form values.
	var tagIDs []int64
	for _, idStr := range r.Form["user_tag_ids"] {
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			continue
		}
		tagIDs = append(tagIDs, id)
	}

	if err := h.store.SetEntryUserTags(user.ID, entryID, tagIDs); err != nil {
		html.ServerError(w, r, err)
		return
	}

	// Return JSON for AJAX requests.
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		json.NoContent(w, r)
		return
	}

	// Redirect back to the referring page for non-AJAX requests.
	redirectURL := r.FormValue("redirect_url")
	if redirectURL == "" {
		redirectURL = r.Referer()
	}
	if redirectURL == "" {
		redirectURL = "/"
	}

	html.Redirect(w, r, redirectURL)
}
