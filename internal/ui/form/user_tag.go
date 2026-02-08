// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package form // import "miniflux.app/v2/internal/ui/form"

import (
	"net/http"
)

// UserTagForm represents a user tag form in the UI.
type UserTagForm struct {
	Title string
}

// NewUserTagForm returns a new UserTagForm.
func NewUserTagForm(r *http.Request) *UserTagForm {
	return &UserTagForm{
		Title: r.FormValue("title"),
	}
}
