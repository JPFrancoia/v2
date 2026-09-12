// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
)

func (h *handler) saveEntryForLater(w http.ResponseWriter, r *http.Request) {
	entryID := request.RouteInt64Param(r, "entryID")
	unreadCountDelta, savedForLater, err := h.store.ToggleEntrySavedForLater(r.Context(), request.UserID(r), entryID)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSONCreated(w, r, struct {
		UnreadCountDelta int  `json:"unread_count_delta"`
		SavedForLater    bool `json:"saved_for_later"`
	}{
		UnreadCountDelta: unreadCountDelta,
		SavedForLater:    savedForLater,
	})
}
