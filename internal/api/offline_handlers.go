// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api // import "miniflux.app/v2/internal/api"

import (
	json_parser "encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/storage"
	"miniflux.app/v2/internal/validator"
)

const offlineDeviceSnapshotVersion = "2"
const offlineSectionPageSize = 10

// offlineSectionQuery applies the same filters as the web sections to a scoped device query.
func offlineSectionQuery(builder *storage.EntryQueryBuilder, section, sort string, now time.Time) error {
	if sort != "score" && sort != "newest" {
		return fmt.Errorf(`api: invalid section sort`)
	}
	switch section {
	case "unread":
		builder.WithStatus(model.EntryStatusUnread)
	case "saved":
		builder.WithStatus(model.EntryStatusUnread).WithSavedForLater(true)
	case "review":
		builder.WithStatus(model.EntryStatusUnread).WithVote(0).WithRawRelevanceScore().AfterPublishedDate(now.Add(-5 * 24 * time.Hour))
	case "history":
		builder.WithStatus(model.EntryStatusRead)
	default:
		return fmt.Errorf(`api: invalid section`)
	}
	if section != "history" {
		builder.WithGloballyVisible()
	}
	builder.WithoutContent()
	if section == "review" && sort == "score" {
		builder.WithRawRelevanceDistanceSorting(config.Opts.ToReviewRawRelevanceTarget())
	} else if section == "history" && sort == "newest" {
		builder.WithSorting("e.changed_at", "DESC")
		builder.WithSorting("e.published_at", "DESC")
	} else if sort == "score" {
		builder.WithSorting("e.score", "DESC")
	} else {
		builder.WithSorting("e.published_at", "DESC")
	}
	if section == "review" && sort == "score" {
		builder.WithSorting("e.published_at", "DESC")
	}
	builder.WithSorting("e.id", "DESC")
	return nil
}

// parseOfflineSectionCursor rejects malformed or cross-section page markers.
func parseOfflineSectionCursor(raw, section, sort string) (first, second, id int64, err error) {
	parts := strings.Split(raw, ".")
	if len(raw) > 128 || len(parts) != 6 || parts[0] != section || parts[1] != sort {
		return 0, 0, 0, fmt.Errorf(`api: invalid section cursor`)
	}
	first, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf(`api: invalid section cursor: %w`, err)
	}
	second, err = strconv.ParseInt(parts[3], 10, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf(`api: invalid section cursor: %w`, err)
	}
	id, err = strconv.ParseInt(parts[4], 10, 64)
	if err != nil || id <= 0 {
		return 0, 0, 0, fmt.Errorf(`api: invalid section cursor`)
	}
	encodedTarget, targetErr := strconv.ParseInt(parts[5], 10, 64)
	if targetErr != nil || (section == "review" && sort == "score" && encodedTarget != config.Opts.ToReviewRawRelevanceTarget()) ||
		(section != "review" || sort != "score") && encodedTarget != 0 {
		return 0, 0, 0, fmt.Errorf(`api: invalid section cursor`)
	}
	if section == "review" && sort == "score" {
		distance := math.Float64frombits(uint64(first))
		if first < 0 || math.IsNaN(distance) || math.IsInf(distance, 0) || distance < 0 || distance > 100 || second <= 0 {
			return 0, 0, 0, fmt.Errorf(`api: invalid section cursor`)
		}
	} else if sort == "newest" && first <= 0 {
		return 0, 0, 0, fmt.Errorf(`api: invalid section cursor`)
	}
	return first, second, id, nil
}

// offlineSectionCursor encodes the last visible sort tuple as a URL-safe page marker.
func offlineSectionCursor(entry *model.Entry, section, sort string, target int64) string {
	first, second := entry.Score, int64(0)
	switch {
	case section == "review" && sort == "score":
		first = int64(math.Float64bits(math.Abs(*entry.RawRelevanceScore - float64(target))))
		second = entry.Date.UnixMicro()
	case section == "history" && sort == "newest":
		first, second = entry.ChangedAt.UnixMicro(), entry.Date.UnixMicro()
	case sort == "newest":
		first = entry.Date.UnixMicro()
	}
	if section != "review" || sort != "score" {
		target = 0
	}
	return fmt.Sprintf(`%s.%s.%d.%d.%d.%d`, section, sort, first, second, entry.ID, target)
}

// getOfflineSectionHandler returns one bounded page without downloading article bodies.
func (h *handler) getOfflineSectionHandler(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "score"
	}
	builder := h.store.NewEntryQueryBuilder(r.Context(), request.UserID(r))
	if err := offlineSectionQuery(builder, section, sort, time.Now()); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	cursor := r.URL.Query().Get("cursor")
	if cursor != "" {
		first, second, id, err := parseOfflineSectionCursor(cursor, section, sort)
		if err != nil {
			response.JSONBadRequest(w, r, err)
			return
		}
		builder.AfterOfflineSectionCursor(section, sort, first, second, id, config.Opts.ToReviewRawRelevanceTarget())
	}
	builder.WithLimit(offlineSectionPageSize + 1)
	entries, err := builder.GetEntries()
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	more := len(entries) > offlineSectionPageSize
	if more {
		entries = entries[:offlineSectionPageSize]
	}
	rows := make([]model.OfflineSectionEntry, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, model.OfflineSectionEntry{
			EntryID: entry.ID, Title: entry.Title, PublishedAt: entry.Date, ChangedAt: entry.ChangedAt,
			Score: entry.Score, Status: entry.Status, Starred: entry.Starred,
			SavedForLater: entry.SavedForLater, Vote: entry.Vote,
		})
	}
	nextCursor := ""
	if more {
		nextCursor = offlineSectionCursor(entries[len(entries)-1], section, sort, config.Opts.ToReviewRawRelevanceTarget())
	}
	response.JSON(w, r, &model.OfflineSectionPage{
		UserID: request.UserID(r), Entries: rows, HasMore: more, NextCursor: nextCursor,
	})
}

// getOfflineManifestHandler returns the authenticated user's offline entry set.
// The device version is independent of browser themes and sessions.
func (h *handler) getOfflineManifestHandler(w http.ResponseWriter, r *http.Request) {
	manifest, err := h.store.OfflineManifest(r.Context(), request.UserID(r), time.Now())
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	manifest.SnapshotVersion = offlineDeviceSnapshotVersion
	response.JSON(w, r, manifest)
}

// getOfflineEntriesHandler returns bounded, user-scoped article data for native clients.
// It does not render the browser's session-dependent HTML pages.
func (h *handler) getOfflineEntriesHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var snapshotRequest model.OfflineSnapshotRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&snapshotRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	if err := validator.ValidateOfflineSnapshotRequest(&snapshotRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	userID := request.UserID(r)
	entries, err := h.store.NewEntryQueryBuilder(r.Context(), userID).WithEntryIDs(snapshotRequest.EntryIDs).GetEntries()
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}
	userTagIDs, err := h.store.OfflineEntryUserTagIDs(r.Context(), userID, snapshotRequest.EntryIDs)
	if err != nil {
		response.JSONServerError(w, r, err)
		return
	}

	snapshotResponse := model.OfflineDeviceSnapshotResponse{Entries: make([]model.OfflineDeviceEntrySnapshot, 0, len(entries))}
	for _, entry := range entries {
		snapshotResponse.Entries = append(snapshotResponse.Entries, newOfflineDeviceEntrySnapshot(entry, userTagIDs[entry.ID]))
	}
	response.JSON(w, r, &snapshotResponse)
}

// newOfflineDeviceEntrySnapshot selects the article fields needed for offline reading.
// It preserves the stored, sanitized content and assigned user tags.
func newOfflineDeviceEntrySnapshot(entry *model.Entry, userTagIDs []int64) model.OfflineDeviceEntrySnapshot {
	return model.OfflineDeviceEntrySnapshot{
		EntryID:       entry.ID,
		Version:       entry.ChangedAt,
		FeedID:        entry.FeedID,
		FeedTitle:     entry.Feed.Title,
		CategoryID:    entry.Feed.Category.ID,
		CategoryTitle: entry.Feed.Category.Title,
		Title:         entry.Title,
		Author:        entry.Author,
		URL:           entry.URL,
		PublishedAt:   entry.Date,
		Score:         entry.Score,
		Content:       entry.Content,
		Status:        entry.Status,
		Starred:       entry.Starred,
		SavedForLater: entry.SavedForLater,
		Vote:          entry.Vote,
		UserTagIDs:    userTagIDs,
	}
}

// syncOfflineEntriesHandler applies bounded device patches for the authenticated user.
// It uses the same conflict and retry rules as the browser's offline sync.
func (h *handler) syncOfflineEntriesHandler(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var syncRequest model.OfflineSyncRequest
	if err := json_parser.NewDecoder(r.Body).Decode(&syncRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	if err := validator.ValidateOfflineSyncRequest(&syncRequest); err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}

	userID := request.UserID(r)
	syncResponse := model.OfflineSyncResponse{Entries: make([]model.OfflineEntryPatchResult, 0, len(syncRequest.Entries))}
	for i := range syncRequest.Entries {
		result, err := h.store.ApplyOfflineEntryPatch(r.Context(), userID, &syncRequest.Entries[i])
		if err != nil {
			response.JSONServerError(w, r, err)
			return
		}
		syncResponse.Entries = append(syncResponse.Entries, result)
	}
	response.JSON(w, r, &syncResponse)
}
