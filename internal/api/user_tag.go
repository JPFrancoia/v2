// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api // import "miniflux.app/v2/internal/api"

import (
	json_parser "encoding/json"
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response/json"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/validator"
)

func (h *handler) getUserTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.store.UserTags(request.UserID(r))
	if err != nil {
		json.ServerError(w, r, err)
		return
	}
	json.OK(w, r, tags)
}

func (h *handler) createUserTag(w http.ResponseWriter, r *http.Request) {
	userID := request.UserID(r)

	var tagCreationRequest model.UserTagCreationRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&tagCreationRequest); err != nil {
		json.BadRequest(w, r, err)
		return
	}

	if validationErr := validator.ValidateUserTagCreation(h.store, userID, &tagCreationRequest); validationErr != nil {
		json.BadRequest(w, r, validationErr.Error())
		return
	}

	tag, err := h.store.CreateUserTag(userID, &tagCreationRequest)
	if err != nil {
		json.ServerError(w, r, err)
		return
	}

	json.Created(w, r, tag)
}

func (h *handler) updateUserTag(w http.ResponseWriter, r *http.Request) {
	userID := request.UserID(r)
	tagID := request.RouteInt64Param(r, "userTagID")

	tag, err := h.store.UserTagByID(userID, tagID)
	if err != nil {
		json.ServerError(w, r, err)
		return
	}

	if tag == nil {
		json.NotFound(w, r)
		return
	}

	var tagModificationRequest model.UserTagModificationRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&tagModificationRequest); err != nil {
		json.BadRequest(w, r, err)
		return
	}

	if validationErr := validator.ValidateUserTagModification(h.store, userID, tag.ID, &tagModificationRequest); validationErr != nil {
		json.BadRequest(w, r, validationErr.Error())
		return
	}

	tagModificationRequest.Patch(tag)

	if err := h.store.UpdateUserTag(tag); err != nil {
		json.ServerError(w, r, err)
		return
	}

	json.Created(w, r, tag)
}

func (h *handler) removeUserTag(w http.ResponseWriter, r *http.Request) {
	userID := request.UserID(r)
	tagID := request.RouteInt64Param(r, "userTagID")

	if !h.store.UserTagIDExists(userID, tagID) {
		json.NotFound(w, r)
		return
	}

	if err := h.store.RemoveUserTag(userID, tagID); err != nil {
		json.ServerError(w, r, err)
		return
	}

	json.NoContent(w, r)
}

func (h *handler) getUserTagEntries(w http.ResponseWriter, r *http.Request) {
	userID := request.UserID(r)
	tagID := request.RouteInt64Param(r, "userTagID")

	tag, err := h.store.UserTagByID(userID, tagID)
	if err != nil {
		json.ServerError(w, r, err)
		return
	}

	if tag == nil {
		json.NotFound(w, r)
		return
	}

	statuses := request.QueryStringParamList(r, "status")
	for _, status := range statuses {
		if err := validator.ValidateEntryStatus(status); err != nil {
			json.BadRequest(w, r, err)
			return
		}
	}

	order := request.QueryStringParam(r, "order", model.DefaultSortingOrder)
	if err := validator.ValidateEntryOrder(order); err != nil {
		json.BadRequest(w, r, err)
		return
	}

	direction := request.QueryStringParam(r, "direction", model.DefaultSortingDirection)
	if err := validator.ValidateDirection(direction); err != nil {
		json.BadRequest(w, r, err)
		return
	}

	limit := request.QueryIntParam(r, "limit", 100)
	offset := request.QueryIntParam(r, "offset", 0)
	if err := validator.ValidateRange(offset, limit); err != nil {
		json.BadRequest(w, r, err)
		return
	}

	builder := h.store.NewEntryQueryBuilder(userID)
	builder.WithUserTagID(tagID)
	builder.WithStatuses(statuses)
	builder.WithSorting(order, direction)
	builder.WithOffset(offset)
	builder.WithLimit(limit)
	builder.WithEnclosures()
	builder.WithoutStatus(model.EntryStatusRemoved)

	configureFilters(builder, r)

	entries, err := builder.GetEntries()
	if err != nil {
		json.ServerError(w, r, err)
		return
	}

	count, err := builder.CountEntries()
	if err != nil {
		json.ServerError(w, r, err)
		return
	}

	json.OK(w, r, &entriesResponse{Total: count, Entries: entries})
}

func (h *handler) setEntryUserTags(w http.ResponseWriter, r *http.Request) {
	userID := request.UserID(r)
	entryID := request.RouteInt64Param(r, "entryID")

	var req struct {
		UserTagIDs []int64 `json:"user_tag_ids"`
	}
	if err := json_parser.NewDecoder(r.Body).Decode(&req); err != nil {
		json.BadRequest(w, r, err)
		return
	}

	if err := h.store.SetEntryUserTags(userID, entryID, req.UserTagIDs); err != nil {
		json.ServerError(w, r, err)
		return
	}

	json.NoContent(w, r)
}
