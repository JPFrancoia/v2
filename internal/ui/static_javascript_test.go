// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"miniflux.app/v2/internal/ui/static"
)

func TestServiceWorkerAllowsApplicationScope(t *testing.T) {
	if err := static.GenerateJavascriptBundles(false); err != nil {
		t.Fatalf(`Unable to generate JavaScript bundles: %v`, err)
	}

	h := &handler{basePath: "/reader"}
	request := httptest.NewRequest(http.MethodGet, "/reader/js/checksum/service-worker.js", nil)
	request.SetPathValue("filename", "service-worker.js")
	response := httptest.NewRecorder()
	h.showJavascript(response, request)

	if value := response.Header().Get("Service-Worker-Allowed"); value != "/reader/" {
		t.Fatalf(`Expected service worker scope /reader/, got %q`, value)
	}
}
