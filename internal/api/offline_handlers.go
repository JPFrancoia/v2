// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package api // import "miniflux.app/v2/internal/api"

import (
	"crypto/sha256"
	"encoding/hex"
	json_parser "encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
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
const offlineSectionMaxPageSize = 20

// offlineSectionParameters keeps native paging and offline preparation separate.
type offlineSectionParameters struct {
	limit     int
	asOf      int64
	selection string
}

// parseOfflineSectionParameters validates bounded pages before database access.
func parseOfflineSectionParameters(r *http.Request, now time.Time) (offlineSectionParameters, error) {
	params := offlineSectionParameters{limit: offlineSectionPageSize}
	query := r.URL.Query()
	if query.Has("limit") {
		limit, err := strconv.Atoi(query.Get("limit"))
		if err != nil || limit < 1 || limit > offlineSectionMaxPageSize {
			return params, errors.New(`api: invalid section limit`)
		}
		params.limit = limit
	}
	switch query.Get("scope") {
	case "":
		if query.Has("as_of") || query.Has("selection") {
			return params, errors.New(`api: preparation parameters require offline scope`)
		}
	case "offline":
		if r.PathValue("section") == "history" {
			return params, errors.New(`api: History has no offline preparation scope`)
		}
		params.asOf = now.Unix()
		if query.Has("as_of") {
			asOf, err := strconv.ParseInt(query.Get("as_of"), 10, 64)
			if err != nil || asOf <= 0 || asOf > now.Unix() {
				return params, errors.New(`api: invalid preparation time`)
			}
			params.asOf = asOf
		} else if query.Get("cursor") != "" {
			return params, errors.New(`api: offline continuation requires preparation time`)
		}
		if r.PathValue("section") == "review" && query.Get("cursor") != "" && !query.Has("selection") {
			return params, errors.New(`api: Review continuation requires selection`)
		}
		if query.Has("selection") {
			params.selection = query.Get("selection")
			decoded, err := hex.DecodeString(params.selection)
			if r.PathValue("section") != "review" || err != nil || len(decoded) != sha256.Size || strings.ToLower(params.selection) != params.selection {
				return params, errors.New(`api: invalid Review selection`)
			}
		}
	default:
		return params, errors.New(`api: invalid section scope`)
	}
	return params, nil
}

// offlineCursorSection binds offline markers to the preparation window.
func offlineCursorSection(section string, asOf int64) string {
	if asOf > 0 {
		return fmt.Sprintf(`%s-offline-%d`, section, asOf)
	}
	return section
}

// offlineSectionPreparationQuery applies the bounded library profile, not browser rules.
func offlineSectionPreparationQuery(builder *storage.EntryQueryBuilder, section, sort string, asOf int64) error {
	if section == "history" {
		return errors.New(`api: History has no offline preparation scope`)
	}
	now := time.Unix(asOf, 0)
	if err := offlineSectionQueryWithScope(builder, section, sort, now, true); err != nil {
		return fmt.Errorf(`api: unable to build preparation query: %w`, err)
	}
	builder.BeforeCreatedDate(now)
	if section == "unread" {
		builder.AfterPublishedDate(now.Add(-7 * 24 * time.Hour))
	}
	return nil
}

// offlineReviewSelection identifies canonical membership and ordering without a long database transaction.
func offlineReviewSelection(entries model.Entries) string {
	hash := sha256.New()
	for _, entry := range entries {
		fmt.Fprintf(hash, "%d:%016x:%d;", entry.ID, math.Float64bits(*entry.RawRelevanceScore), entry.Date.UnixMicro())
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// offlineReviewPage derives a page from one canonical query, keeping its token and rows consistent.
func offlineReviewPage(entries model.Entries, order string, first, second, id, target int64) model.Entries {
	if order == "newest" {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Date.Equal(entries[j].Date) {
				return entries[i].ID > entries[j].ID
			}
			return entries[i].Date.After(entries[j].Date)
		})
	}
	if id == 0 {
		return entries
	}
	remaining := entries[:0]
	for _, entry := range entries {
		published := entry.Date.UnixMicro()
		after := published < first || published == first && entry.ID < id
		if order == "score" {
			distance := math.Abs(*entry.RawRelevanceScore - float64(target))
			cursorDistance := math.Float64frombits(uint64(first))
			after = distance > cursorDistance || distance == cursorDistance &&
				(published < second || published == second && entry.ID < id)
		}
		if after {
			remaining = append(remaining, entry)
		}
	}
	return remaining
}

// offlineSectionQuery applies the same filters as the web sections to a scoped device query.
func offlineSectionQuery(builder *storage.EntryQueryBuilder, section, sort string, now time.Time) error {
	if err := offlineSectionQueryWithScope(builder, section, sort, now, false); err != nil {
		return fmt.Errorf(`api: unable to build online section query: %w`, err)
	}
	return nil
}

// offlineSectionQueryWithScope preserves online Saved while preparing every saved article.
func offlineSectionQueryWithScope(builder *storage.EntryQueryBuilder, section, sort string, now time.Time, offline bool) error {
	if sort != "score" && sort != "newest" {
		return errors.New(`api: invalid section sort`)
	}
	switch section {
	case "unread":
		builder.WithStatus(model.EntryStatusUnread)
	case "saved":
		builder.WithSavedForLater(true)
		if !offline {
			builder.WithStatus(model.EntryStatusUnread)
		}
	case "review":
		builder.WithStatus(model.EntryStatusUnread).WithVote(0).WithRawRelevanceScore().AfterPublishedDate(now.Add(-5 * 24 * time.Hour))
	case "history":
		builder.WithStatus(model.EntryStatusRead)
	default:
		return errors.New(`api: invalid section`)
	}
	if section != "history" && (!offline || section == "review") {
		builder.WithGloballyVisible()
	}
	builder.WithoutContent()
	switch {
	case section == "review" && sort == "score":
		builder.WithRawRelevanceDistanceSorting(config.Opts.ToReviewRawRelevanceTarget())
	case section == "history" && sort == "newest":
		builder.WithSorting("e.changed_at", "DESC")
		builder.WithSorting("e.published_at", "DESC")
	case sort == "score":
		builder.WithSorting("e.score", "DESC")
	default:
		builder.WithSorting("e.published_at", "DESC")
	}
	if section == "review" && sort == "score" {
		builder.WithSorting("e.published_at", "DESC")
	}
	builder.WithSorting("e.id", "DESC")
	return nil
}

// parseOfflineSectionCursor rejects malformed or cross-section page markers.
func parseOfflineSectionCursor(raw, section, sort string, asOf int64) (first, second, id int64, err error) {
	parts := strings.Split(raw, ".")
	if len(raw) > 128 || len(parts) != 6 || parts[0] != offlineCursorSection(section, asOf) || parts[1] != sort {
		return 0, 0, 0, errors.New(`api: invalid section cursor`)
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
		return 0, 0, 0, errors.New(`api: invalid section cursor`)
	}
	encodedTarget, targetErr := strconv.ParseInt(parts[5], 10, 64)
	if targetErr != nil || (section == "review" && sort == "score" && encodedTarget != config.Opts.ToReviewRawRelevanceTarget()) ||
		(section != "review" || sort != "score") && encodedTarget != 0 {
		return 0, 0, 0, errors.New(`api: invalid section cursor`)
	}
	if section == "review" && sort == "score" {
		distance := math.Float64frombits(uint64(first))
		if first < 0 || math.IsNaN(distance) || math.IsInf(distance, 0) || distance < 0 || distance > 100 || second <= 0 {
			return 0, 0, 0, errors.New(`api: invalid section cursor`)
		}
	} else if sort == "newest" && first <= 0 {
		return 0, 0, 0, errors.New(`api: invalid section cursor`)
	}
	return first, second, id, nil
}

// offlineSectionCursor encodes the last visible sort tuple as a URL-safe page marker.
func offlineSectionCursor(entry *model.Entry, section, sort string, target, asOf int64) string {
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
	return fmt.Sprintf(`%s.%s.%d.%d.%d.%d`, offlineCursorSection(section, asOf), sort, first, second, entry.ID, target)
}

// getOfflineSectionHandler returns one bounded page without downloading article bodies.
func (h *handler) getOfflineSectionHandler(w http.ResponseWriter, r *http.Request) {
	section := r.PathValue("section")
	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "score"
	}
	now := time.Now()
	params, err := parseOfflineSectionParameters(r, now)
	if err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	builder := h.store.NewEntryQueryBuilder(r.Context(), request.UserID(r))
	if params.asOf > 0 {
		err = offlineSectionPreparationQuery(builder, section, sort, params.asOf)
	} else {
		err = offlineSectionQuery(builder, section, sort, now)
	}
	if err != nil {
		response.JSONBadRequest(w, r, err)
		return
	}
	cursor := r.URL.Query().Get("cursor")
	var first, second, id int64
	if cursor != "" {
		first, second, id, err = parseOfflineSectionCursor(cursor, section, sort, params.asOf)
		if err != nil {
			response.JSONBadRequest(w, r, err)
			return
		}
		builder.AfterOfflineSectionCursor(section, sort, first, second, id, config.Opts.ToReviewRawRelevanceTarget())
	}
	selection := ""
	var entries model.Entries
	if params.asOf > 0 && section == "review" {
		canonical := h.store.NewEntryQueryBuilder(r.Context(), request.UserID(r))
		if err := offlineSectionPreparationQuery(canonical, section, "score", params.asOf); err != nil {
			response.JSONBadRequest(w, r, err)
			return
		}
		candidates, err := canonical.WithLimit(100).GetEntries()
		if err != nil {
			response.JSONServerError(w, r, err)
			return
		}
		selection = offlineReviewSelection(candidates)
		if params.selection != "" && selection != params.selection {
			response.NewBuilder(w, r).WithStatus(http.StatusConflict).
				WithHeader("Content-Type", "application/json").
				WithBodyAsBytes([]byte(`{"error_message":"api: Review selection changed; retry Sync"}`)).Write()
			return
		}
		entries = offlineReviewPage(candidates, sort, first, second, id, config.Opts.ToReviewRawRelevanceTarget())
	} else {
		entries, err = builder.WithLimit(params.limit + 1).GetEntries()
		if err != nil {
			response.JSONServerError(w, r, err)
			return
		}
	}
	more := len(entries) > params.limit
	if more {
		entries = entries[:params.limit]
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
		nextCursor = offlineSectionCursor(entries[len(entries)-1], section, sort, config.Opts.ToReviewRawRelevanceTarget(), params.asOf)
	}
	response.JSON(w, r, &model.OfflineSectionPage{
		UserID: request.UserID(r), Entries: rows, HasMore: more, NextCursor: nextCursor,
		AsOf: params.asOf, Selection: selection,
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
