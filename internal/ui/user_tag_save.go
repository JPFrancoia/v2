// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/ui/form"
	"miniflux.app/v2/internal/ui/view"
	"miniflux.app/v2/internal/validator"
)

func (h *handler) saveUserTag(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	tagForm := form.NewUserTagForm(r)

	v := view.New(h.tpl, r)
	v.Set("form", tagForm)
	v.Set("menu", "settings")
	v.Set("user", user)
	v.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	v.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))

	tagCreationRequest := &model.UserTagCreationRequest{Title: tagForm.Title}

	if validationErr := validator.ValidateUserTagCreation(h.store, user.ID, tagCreationRequest); validationErr != nil {
		v.Set("errorMessage", validationErr.Translate(user.Language))
		response.HTML(w, r, v.Render("create_user_tag"))
		return
	}

	if _, err = h.store.CreateUserTag(user.ID, tagCreationRequest); err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	response.HTMLRedirect(w, r, h.routePath("/user-tags"))
}
