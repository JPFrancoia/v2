// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response/html"
	"miniflux.app/v2/internal/http/route"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/ui/form"
	"miniflux.app/v2/internal/ui/session"
	"miniflux.app/v2/internal/ui/view"
	"miniflux.app/v2/internal/validator"
)

func (h *handler) updateUserTag(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		html.ServerError(w, r, err)
		return
	}

	tagID := request.RouteInt64Param(r, "userTagID")
	tag, err := h.store.UserTagByID(request.UserID(r), tagID)
	if err != nil {
		html.ServerError(w, r, err)
		return
	}

	if tag == nil {
		html.NotFound(w, r)
		return
	}

	tagForm := form.NewUserTagForm(r)

	sess := session.New(h.store, request.SessionID(r))
	view := view.New(h.tpl, r, sess)
	view.Set("form", tagForm)
	view.Set("tag", tag)
	view.Set("menu", "settings")
	view.Set("user", user)
	view.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))

	tagRequest := &model.UserTagModificationRequest{
		Title: model.SetOptionalField(tagForm.Title),
	}

	if validationErr := validator.ValidateUserTagModification(h.store, user.ID, tag.ID, tagRequest); validationErr != nil {
		view.Set("errorMessage", validationErr.Translate(user.Language))
		html.OK(w, r, view.Render("edit_user_tag"))
		return
	}

	tagRequest.Patch(tag)
	if err := h.store.UpdateUserTag(tag); err != nil {
		html.ServerError(w, r, err)
		return
	}

	html.Redirect(w, r, route.Path(h.router, "userTags"))
}
