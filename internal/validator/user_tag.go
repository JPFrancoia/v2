// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package validator // import "miniflux.app/v2/internal/validator"

import (
	"context"

	"miniflux.app/v2/internal/locale"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
)

// ValidateUserTagCreation validates user tag creation.
func ValidateUserTagCreation(ctx context.Context, store *storage.Storage, userID int64, request *model.UserTagCreationRequest) *locale.LocalizedError {
	if request.Title == "" {
		return locale.NewLocalizedError("error.tag_title_required")
	}

	if store.UserTagTitleExists(ctx, userID, request.Title) {
		return locale.NewLocalizedError("error.tag_already_exists")
	}

	return nil
}

// ValidateUserTagModification validates user tag modification.
func ValidateUserTagModification(ctx context.Context, store *storage.Storage, userID, tagID int64, request *model.UserTagModificationRequest) *locale.LocalizedError {
	if request.Title != nil {
		if *request.Title == "" {
			return locale.NewLocalizedError("error.tag_title_required")
		}

		if store.AnotherUserTagExists(ctx, userID, tagID, *request.Title) {
			return locale.NewLocalizedError("error.tag_already_exists")
		}
	}

	return nil
}
