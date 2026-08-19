// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"
	"time"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
)

func (h *handler) showOfflineManifest(w http.ResponseWriter, r *http.Request) {
	manifest, err := h.store.OfflineManifest(r.Context(), request.UserID(r), time.Now())
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	response.JSON(w, r, manifest)
}
