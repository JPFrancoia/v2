// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	json_parser "encoding/json"
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/validator"
)

func (h *handler) syncOfflineEntries(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var syncRequest model.OfflineSyncRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&syncRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	if err := validator.ValidateOfflineSyncRequest(&syncRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	syncResponse := model.OfflineSyncResponse{Entries: make([]model.OfflineEntryPatchResult, 0, len(syncRequest.Entries))}
	for i := range syncRequest.Entries {
		result, err := h.store.ApplyOfflineEntryPatch(r.Context(), request.UserID(r), &syncRequest.Entries[i])
		if err != nil {
			response.JSONServerError(w, r, err)
			return
		}
		syncResponse.Entries = append(syncResponse.Entries, result)
	}
	response.JSON(w, r, &syncResponse)
}
