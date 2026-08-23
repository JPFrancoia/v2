// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"
	"strings"
	"time"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/ui/static"
)

// Increment this version when cached page rendering or media extraction changes.
const offlineSnapshotSchemaVersion = "2"

func (h *handler) showOfflineManifest(w http.ResponseWriter, r *http.Request) {
	manifest, err := h.store.OfflineManifest(r.Context(), request.UserID(r), time.Now())
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	webSession := request.WebSession(r)
	manifest.SnapshotVersion = strings.Join([]string{
		offlineSnapshotSchemaVersion,
		static.JavascriptBundles["app.js"].Checksum,
		static.StylesheetBundles[webSession.Theme()+".css"].Checksum,
		webSession.Language(),
	}, ":")
	response.JSON(w, r, manifest)
}
