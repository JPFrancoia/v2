// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import "fmt"

// UserTag represents a user-defined tag.
type UserTag struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Title      string `json:"title"`
	EntryCount *int   `json:"entry_count,omitempty"`
}

func (t *UserTag) String() string {
	return fmt.Sprintf("ID=%d, UserID=%d, Title=%s", t.ID, t.UserID, t.Title)
}

// UserTagCreationRequest represents a request to create a user tag.
type UserTagCreationRequest struct {
	Title string `json:"title"`
}

// UserTagModificationRequest represents a request to modify a user tag.
type UserTagModificationRequest struct {
	Title *string `json:"title"`
}

// Patch applies the modification request to the given tag.
func (r *UserTagModificationRequest) Patch(tag *UserTag) {
	if r.Title != nil {
		tag.Title = *r.Title
	}
}

// UserTags represents a list of user tags.
type UserTags []*UserTag
