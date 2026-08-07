// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"net/http"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/validator"
)

func entryListSorting(r *http.Request, user *model.User) (string, string) {
	order := request.QueryStringParam(r, "order", user.EntryOrder)
	if order != "score" && order != "published_at" {
		order = user.EntryOrder
	}

	direction := request.QueryStringParam(r, "direction", user.EntryDirection)
	if err := validator.ValidateDirection(direction); err != nil {
		direction = user.EntryDirection
	}

	return order, direction
}

func nextEntryListSortDirection(order, direction, selectedOrder string) string {
	if order == selectedOrder && direction == "desc" {
		return "asc"
	}

	return "desc"
}
