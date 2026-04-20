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

func (h *handler) updateUserTag(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	tagID := request.RouteInt64Param(r, "userTagID")
	tag, err := h.store.UserTagByID(request.UserID(r), tagID)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if tag == nil {
		response.HTMLNotFound(w, r)
		return
	}

	tagForm := form.NewUserTagForm(r)

	v := view.New(h.tpl, r)
	v.Set("form", tagForm)
	v.Set("tag", tag)
	v.Set("menu", "settings")
	v.Set("user", user)
	v.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	v.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))

	tagRequest := &model.UserTagModificationRequest{
		Title: model.SetOptionalField(tagForm.Title),
	}

	if validationErr := validator.ValidateUserTagModification(h.store, user.ID, tag.ID, tagRequest); validationErr != nil {
		v.Set("errorMessage", validationErr.Translate(user.Language))
		response.HTML(w, r, v.Render("edit_user_tag"))
		return
	}

	tagRequest.Patch(tag)
	if err := h.store.UpdateUserTag(tag); err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	response.HTMLRedirect(w, r, h.routePath("/user-tags"))
}
