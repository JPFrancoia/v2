// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package validator // import "miniflux.app/v2/internal/validator"

import (
	"testing"

	"miniflux.app/v2/internal/model"
)

func TestValidateUserTagCreationWithEmptyTitle(t *testing.T) {
	request := &model.UserTagCreationRequest{Title: ""}
	err := ValidateUserTagCreation(nil, 1, request)
	if err == nil {
		t.Fatal(`An empty title should generate an error`)
	}
}

func TestValidateUserTagModificationWithEmptyTitle(t *testing.T) {
	emptyTitle := ""
	request := &model.UserTagModificationRequest{Title: &emptyTitle}
	err := ValidateUserTagModification(nil, 1, 1, request)
	if err == nil {
		t.Fatal(`An empty title should generate an error`)
	}
}

func TestValidateUserTagModificationWithNilTitle(t *testing.T) {
	request := &model.UserTagModificationRequest{Title: nil}
	err := ValidateUserTagModification(nil, 1, 1, request)
	if err != nil {
		t.Fatal(`A nil title should not generate an error`)
	}
}
