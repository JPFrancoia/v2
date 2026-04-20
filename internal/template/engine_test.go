// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package template // import "miniflux.app/v2/internal/template"

import (
	"testing"

	"miniflux.app/v2/internal/config"
)

// TestParseTemplates verifies that all embedded templates can be parsed
// successfully with the registered function map. This catches any template
// that references an undefined function (e.g. a renamed or removed helper),
// which would otherwise only be discovered at runtime startup.
func TestParseTemplates(t *testing.T) {
	config.Opts = config.NewConfigOptions()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ParseTemplates() panicked: %v", r)
		}
	}()

	NewEngine("").ParseTemplates()
}
