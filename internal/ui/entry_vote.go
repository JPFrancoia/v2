// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"errors"
	"net/http"
	"strconv"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
)

func (h *handler) updateEntryVote(w http.ResponseWriter, r *http.Request) {
	entryID := request.RouteInt64Param(r, "entryID")

	// Parse vote value manually to allow negative values
	voteString := request.RouteStringParam(r, "vote")
	voteValue64, err := strconv.ParseInt(voteString, 10, 64)
	if err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	// Validate vote value
	if voteValue64 < -1 || voteValue64 > 1 {
		response.JSONBadRequest(w, r, errors.New("invalid vote value: must be -1, 0, or 1"))
		return
	}

	voteValue := int(voteValue64)

	if err := h.store.UpdateEntryVote(request.UserID(r), entryID, voteValue); err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	response.JSON(w, r, "OK")
}
