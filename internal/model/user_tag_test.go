// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import (
	"testing"
)

func TestUserTagString(t *testing.T) {
	tag := &UserTag{ID: 42, UserID: 1, Title: "golang"}
	expected := "ID=42, UserID=1, Title=golang"
	if tag.String() != expected {
		t.Fatalf(`expected %q, got %q`, expected, tag.String())
	}
}

func TestUserTagModificationRequestPatchTitle(t *testing.T) {
	tag := &UserTag{ID: 1, UserID: 1, Title: "old-title"}
	newTitle := "new-title"
	request := &UserTagModificationRequest{Title: &newTitle}
	request.Patch(tag)

	if tag.Title != "new-title" {
		t.Fatalf(`expected title to be "new-title", got %q`, tag.Title)
	}
}

func TestUserTagModificationRequestPatchNilTitle(t *testing.T) {
	tag := &UserTag{ID: 1, UserID: 1, Title: "unchanged"}
	request := &UserTagModificationRequest{Title: nil}
	request.Patch(tag)

	if tag.Title != "unchanged" {
		t.Fatalf(`expected title to remain "unchanged", got %q`, tag.Title)
	}
}

func TestUserTagModificationRequestPatchEmptyTitle(t *testing.T) {
	tag := &UserTag{ID: 1, UserID: 1, Title: "old-title"}
	emptyTitle := ""
	request := &UserTagModificationRequest{Title: &emptyTitle}
	request.Patch(tag)

	if tag.Title != "" {
		t.Fatalf(`expected title to be empty, got %q`, tag.Title)
	}
}

func TestUserTagModificationRequestPatchPreservesOtherFields(t *testing.T) {
	tag := &UserTag{ID: 42, UserID: 7, Title: "old-title"}
	newTitle := "new-title"
	request := &UserTagModificationRequest{Title: &newTitle}
	request.Patch(tag)

	if tag.ID != 42 {
		t.Fatalf(`expected ID to remain 42, got %d`, tag.ID)
	}
	if tag.UserID != 7 {
		t.Fatalf(`expected UserID to remain 7, got %d`, tag.UserID)
	}
}

func TestUserModificationRequestPatchShowFeedTags(t *testing.T) {
	user := &User{ShowFeedTags: true}
	showFeedTags := false
	request := &UserModificationRequest{ShowFeedTags: &showFeedTags}
	request.Patch(user)

	if user.ShowFeedTags != false {
		t.Fatal(`expected ShowFeedTags to be false`)
	}
}

func TestUserModificationRequestPatchShowFeedTagsNil(t *testing.T) {
	user := &User{ShowFeedTags: true}
	request := &UserModificationRequest{ShowFeedTags: nil}
	request.Patch(user)

	if user.ShowFeedTags != true {
		t.Fatal(`expected ShowFeedTags to remain true when patch field is nil`)
	}
}

func TestUserModificationRequestPatchShowFeedTagsEnable(t *testing.T) {
	user := &User{ShowFeedTags: false}
	showFeedTags := true
	request := &UserModificationRequest{ShowFeedTags: &showFeedTags}
	request.Patch(user)

	if user.ShowFeedTags != true {
		t.Fatal(`expected ShowFeedTags to be true`)
	}
}
