// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/lib/pq"

	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/timezone"
)

// EntryQueryBuilder builds a SQL query to fetch entries.
type EntryQueryBuilder struct {
	ctx             context.Context
	store           *Storage
	args            []any
	conditions      []string
	sortExpressions []string
	limit           int
	offset          int
	fetchEnclosures bool
	excludeContent  bool
}

// WithEnclosures fetches enclosures for each entry.
func (e *EntryQueryBuilder) WithEnclosures() *EntryQueryBuilder {
	e.fetchEnclosures = true
	return e
}

// WithoutContent excludes the content column from the query results,
// replacing it with an empty string. This significantly reduces data
// transfer from PostgreSQL on list pages where content is not displayed.
func (e *EntryQueryBuilder) WithoutContent() *EntryQueryBuilder {
	e.excludeContent = true
	return e
}

// WithSearchQuery adds full-text search query to the condition.
func (e *EntryQueryBuilder) WithSearchQuery(query string) *EntryQueryBuilder {
	if query != "" {
		nArgs := len(e.args) + 1
		e.conditions = append(e.conditions, fmt.Sprintf("e.document_vectors @@ plainto_tsquery($%d)", nArgs))
		e.args = append(e.args, query)

		// 0.0000001 = 0.1 / (seconds_in_a_day)
		e.WithSorting(
			fmt.Sprintf("ts_rank(document_vectors, plainto_tsquery($%d)) - extract (epoch from now() - published_at)::float * 0.0000001", nArgs),
			"DESC",
		)
	}
	return e
}

// WithStarred adds starred filter.
func (e *EntryQueryBuilder) WithStarred(starred bool) *EntryQueryBuilder {
	if starred {
		e.conditions = append(e.conditions, "e.starred is true")
	} else {
		e.conditions = append(e.conditions, "e.starred is false")
	}
	return e
}

// WithSavedForLater adds saved-for-later filter.
func (e *EntryQueryBuilder) WithSavedForLater(savedForLater bool) *EntryQueryBuilder {
	if savedForLater {
		e.conditions = append(e.conditions, "e.saved_for_later is true")
	} else {
		e.conditions = append(e.conditions, "e.saved_for_later is false")
	}
	return e
}

// WithVote adds vote filter.
func (e *EntryQueryBuilder) WithVote(vote int) *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.vote = $"+strconv.Itoa(len(e.args)+1))
	e.args = append(e.args, vote)
	return e
}

// BeforeCreatedDate excludes entries added after preparation starts.
func (e *EntryQueryBuilder) BeforeCreatedDate(date time.Time) *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.created_at <= $"+strconv.Itoa(len(e.args)+1))
	e.args = append(e.args, date)
	return e
}

// BeforeChangedDate adds a condition < changed_at
func (e *EntryQueryBuilder) BeforeChangedDate(date time.Time) *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.changed_at < $"+strconv.Itoa(len(e.args)+1))
	e.args = append(e.args, date)
	return e
}

// AfterChangedDate adds a condition > changed_at
func (e *EntryQueryBuilder) AfterChangedDate(date time.Time) *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.changed_at > $"+strconv.Itoa(len(e.args)+1))
	e.args = append(e.args, date)
	return e
}

// BeforePublishedDate adds a condition < published_at
func (e *EntryQueryBuilder) BeforePublishedDate(date time.Time) *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.published_at < $"+strconv.Itoa(len(e.args)+1))
	e.args = append(e.args, date)
	return e
}

// AfterPublishedDate adds a condition > published_at
func (e *EntryQueryBuilder) AfterPublishedDate(date time.Time) *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.published_at > $"+strconv.Itoa(len(e.args)+1))
	e.args = append(e.args, date)
	return e
}

// BeforeEntryID adds a condition < entryID.
func (e *EntryQueryBuilder) BeforeEntryID(entryID int64) *EntryQueryBuilder {
	if entryID != 0 {
		e.conditions = append(e.conditions, "e.id < $"+strconv.Itoa(len(e.args)+1))
		e.args = append(e.args, entryID)
	}
	return e
}

// AfterEntryID adds a condition > entryID.
func (e *EntryQueryBuilder) AfterEntryID(entryID int64) *EntryQueryBuilder {
	if entryID != 0 {
		e.conditions = append(e.conditions, "e.id > $"+strconv.Itoa(len(e.args)+1))
		e.args = append(e.args, entryID)
	}
	return e
}

// WithEntryIDs filter by entry IDs.
func (e *EntryQueryBuilder) WithEntryIDs(entryIDs []int64) *EntryQueryBuilder {
	if len(entryIDs) == 1 {
		e.conditions = append(e.conditions, fmt.Sprintf("e.id = $%d", len(e.args)+1))
		e.args = append(e.args, entryIDs[0])
	} else if len(entryIDs) > 1 {
		e.conditions = append(e.conditions, fmt.Sprintf("e.id = ANY($%d)", len(e.args)+1))
		e.args = append(e.args, pq.Int64Array(entryIDs))
	}
	return e
}

// WithEntryID filter by entry ID.
func (e *EntryQueryBuilder) WithEntryID(entryID int64) *EntryQueryBuilder {
	if entryID != 0 {
		e.conditions = append(e.conditions, "e.id = $"+strconv.Itoa(len(e.args)+1))
		e.args = append(e.args, entryID)
	}
	return e
}

// WithFeedID filter by feed ID.
func (e *EntryQueryBuilder) WithFeedID(feedID int64) *EntryQueryBuilder {
	if feedID > 0 {
		e.conditions = append(e.conditions, "e.feed_id = $"+strconv.Itoa(len(e.args)+1))
		e.args = append(e.args, feedID)
	}
	return e
}

// WithCategoryID filter by category ID.
func (e *EntryQueryBuilder) WithCategoryID(categoryID int64) *EntryQueryBuilder {
	if categoryID > 0 {
		e.conditions = append(e.conditions, "f.category_id = $"+strconv.Itoa(len(e.args)+1))
		e.args = append(e.args, categoryID)
	}
	return e
}

// WithStatus filter by entry status.
func (e *EntryQueryBuilder) WithStatus(status string) *EntryQueryBuilder {
	if status != "" {
		e.conditions = append(e.conditions, "e.status = $"+strconv.Itoa(len(e.args)+1))
		e.args = append(e.args, status)
	}
	return e
}

// WithStatuses filter by a list of entry statuses.
func (e *EntryQueryBuilder) WithStatuses(statuses []string) *EntryQueryBuilder {
	if len(statuses) == 1 {
		e.conditions = append(e.conditions, fmt.Sprintf("e.status = $%d", len(e.args)+1))
		e.args = append(e.args, statuses[0])
	} else if len(statuses) > 1 {
		e.conditions = append(e.conditions, fmt.Sprintf("e.status = ANY($%d)", len(e.args)+1))
		e.args = append(e.args, pq.StringArray(statuses))
	}
	return e
}

// WithUserTagID filter by user tag ID.
func (e *EntryQueryBuilder) WithUserTagID(userTagID int64) *EntryQueryBuilder {
	if userTagID > 0 {
		e.conditions = append(e.conditions, fmt.Sprintf("e.id IN (SELECT entry_id FROM entry_user_tags WHERE user_tag_id = $%d)", len(e.args)+1))
		e.args = append(e.args, userTagID)
	}
	return e
}

// WithTags filter by a list of entry tags.
func (e *EntryQueryBuilder) WithTags(tags []string) *EntryQueryBuilder {
	if len(tags) > 0 {
		for _, cat := range tags {
			e.conditions = append(e.conditions, fmt.Sprintf("LOWER($%d) = ANY(LOWER(e.tags::text)::text[])", len(e.args)+1))
			e.args = append(e.args, cat)
		}
	}
	return e
}

// WithoutStatus set the entry status that should not be returned.
func (e *EntryQueryBuilder) WithoutStatus(status string) *EntryQueryBuilder {
	if status != "" {
		e.conditions = append(e.conditions, "e.status <> $"+strconv.Itoa(len(e.args)+1))
		e.args = append(e.args, status)
	}
	return e
}

// WithShareCode set the entry share code.
func (e *EntryQueryBuilder) WithShareCode(shareCode string) *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.share_code = $"+strconv.Itoa(len(e.args)+1))
	e.args = append(e.args, shareCode)
	return e
}

// WithShareCodeNotEmpty adds a filter for non-empty share code.
func (e *EntryQueryBuilder) WithShareCodeNotEmpty() *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.share_code <> ''")
	return e
}

// WithSorting qualifies ambiguous names and adds a sort expression.
func (e *EntryQueryBuilder) WithSorting(column, direction string) *EntryQueryBuilder {
	switch column {
	case "title":
		column = "e.title"
	case "category_title":
		column = "c.title"
	}
	e.sortExpressions = append(e.sortExpressions, column+" "+direction)
	return e
}

// WithRawRelevanceScore selects entries with a stored raw relevance prediction.
func (e *EntryQueryBuilder) WithRawRelevanceScore() *EntryQueryBuilder {
	e.conditions = append(e.conditions, "e.raw_relevance_score is not null")
	return e
}

// WithRawRelevanceDistanceSorting sorts predicted entries nearest a raw target first.
func (e *EntryQueryBuilder) WithRawRelevanceDistanceSorting(score int64) *EntryQueryBuilder {
	e.WithRawRelevanceScore()
	e.sortExpressions = append(e.sortExpressions, fmt.Sprintf("abs(e.raw_relevance_score - $%d) asc", len(e.args)+1))
	e.args = append(e.args, score)
	return e
}

// AfterOfflineSectionCursor keeps a device page after the last row in the current sort order.
// Review distance uses the float's bits to preserve fractional scores in URL-safe cursors.
func (e *EntryQueryBuilder) AfterOfflineSectionCursor(section, sort string, first, second, id, target int64) *EntryQueryBuilder {
	n := len(e.args) + 1
	switch {
	case section == "review" && sort == "score":
		e.conditions = append(e.conditions, fmt.Sprintf(`(abs(e.raw_relevance_score-$%d) > $%d or (abs(e.raw_relevance_score-$%d) = $%d and (e.published_at, e.id) < ($%d, $%d)))`, n, n+1, n, n+1, n+2, n+3))
		e.args = append(e.args, target, math.Float64frombits(uint64(first)), time.UnixMicro(second), id)
	case section == "history" && sort == "newest":
		e.conditions = append(e.conditions, fmt.Sprintf(`(e.changed_at, e.published_at, e.id) < ($%d, $%d, $%d)`, n, n+1, n+2))
		e.args = append(e.args, time.UnixMicro(first), time.UnixMicro(second), id)
	case sort == "newest":
		e.conditions = append(e.conditions, fmt.Sprintf(`(e.published_at, e.id) < ($%d, $%d)`, n, n+1))
		e.args = append(e.args, time.UnixMicro(first), id)
	default:
		e.conditions = append(e.conditions, fmt.Sprintf(`(e.score, e.id) < ($%d, $%d)`, n, n+1))
		e.args = append(e.args, first, id)
	}
	return e
}

// WithLimit set the limit.
func (e *EntryQueryBuilder) WithLimit(limit int) *EntryQueryBuilder {
	if limit > 0 {
		e.limit = limit
	}
	return e
}

// WithOffset set the offset.
func (e *EntryQueryBuilder) WithOffset(offset int) *EntryQueryBuilder {
	if offset > 0 {
		e.offset = offset
	}
	return e
}

func (e *EntryQueryBuilder) WithGloballyVisible() *EntryQueryBuilder {
	e.conditions = append(e.conditions, "c.hide_globally IS FALSE")
	e.conditions = append(e.conditions, "f.hide_globally IS FALSE")
	return e
}

// CountEntries count the number of entries that match the condition.
func (e *EntryQueryBuilder) CountEntries() (count int, err error) {
	query := `
		SELECT count(*)
		FROM entries e
			JOIN feeds f ON f.id = e.feed_id
			JOIN categories c ON c.id = f.category_id
		WHERE ` + e.buildCondition()

	err = e.store.db.QueryRowContext(e.ctx, query, e.args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: unable to count entries: %v", err)
	}

	return count, nil
}

// GetEntry returns a single entry that match the condition.
func (e *EntryQueryBuilder) GetEntry() (*model.Entry, error) {
	e.limit = 1
	entries, err := e.GetEntries()
	if err != nil {
		return nil, err
	}

	if len(entries) != 1 {
		return nil, nil
	}

	entries[0].Enclosures, err = e.store.GetEnclosures(e.ctx, entries[0].ID)
	if err != nil {
		return nil, err
	}

	return entries[0], nil
}

// GetEntries returns a list of entries that match the condition.
func (e *EntryQueryBuilder) GetEntries() (model.Entries, error) {
	entries, _, err := e.fetchEntries(false)
	return entries, err
}

// GetEntriesWithCount returns a list of entries and the total count of matching
// rows (ignoring limit/offset) in a single query using a window function.
// This avoids a separate CountEntries() round-trip.
func (e *EntryQueryBuilder) GetEntriesWithCount() (model.Entries, int, error) {
	return e.fetchEntries(true)
}

// entryQuery builds the row query separately so execution-plan checks can inspect real database work.
func (e *EntryQueryBuilder) entryQuery(withCount bool) string {
	countColumn := ""
	prefix := ""
	condition := e.buildCondition()
	sorting := e.buildSorting()
	pageJoin := ""
	if withCount {
		countColumn = "page_entries.total_count,"
		prefix = `with page_entries as materialized (
            select e.id, count(*) over () as total_count
            from entries e
            left join feeds f on f.id = e.feed_id
            left join categories c on c.id = f.category_id
            where ` + condition + " " + sorting + `
        ) `
		pageJoin = " join page_entries on page_entries.id = e.id "
		condition = "true"
		sorting = ""
		if len(e.sortExpressions) > 0 {
			sorting = " order by " + strings.Join(e.sortExpressions, ", ")
		}
	}

	return prefix + `
		SELECT
			` + countColumn + `
			e.id,
			e.user_id,
			e.feed_id,
			e.hash,
			e.published_at at time zone u.timezone,
			e.title,
			e.url,
			e.comments_url,
			e.author,
			e.share_code,
			` + e.contentColumn() + `,
			e.status,
			e.starred,
			e.saved_for_later,
			e.reading_time,
			e.created_at,
			e.changed_at,
			e.tags,
			e.score,
			e.raw_relevance_score,
			e.vote,
			f.title as feed_title,
			f.feed_url,
			f.site_url,
			f.description,
			f.checked_at,
			f.category_id,
			c.title as category_title,
			c.hide_globally as category_hidden,
			f.scraper_rules,
			f.rewrite_rules,
			f.crawler,
			f.user_agent,
			f.cookie,
			f.hide_globally,
			f.no_media_player,
			f.webhook_url,
			fi.icon_id,
			i.external_id AS icon_external_id,
			u.timezone
		FROM
			entries e ` + pageJoin + `
		LEFT JOIN
			feeds f ON f.id=e.feed_id
		LEFT JOIN
			categories c ON c.id=f.category_id
		LEFT JOIN
			feed_icons fi ON fi.feed_id=f.id
		LEFT JOIN
			icons i ON i.id=fi.icon_id
		LEFT JOIN
			users u ON u.id=e.user_id
		WHERE ` + condition + " " + sorting
}

// fetchEntries is the shared implementation for GetEntries and GetEntriesWithCount.
// When withCount is true, count(*) OVER() is included in the SELECT and the total
// count of matching rows is returned; otherwise the returned count is 0.
func (e *EntryQueryBuilder) fetchEntries(withCount bool) (model.Entries, int, error) {
	query := e.entryQuery(withCount)

	rows, err := e.store.db.QueryContext(e.ctx, query, e.args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: unable to get entries: %v", err)
	}
	defer rows.Close()

	entries := make(model.Entries, 0)
	entryMap := make(map[int64]*model.Entry)
	var entryIDs []int64
	var totalCount int

	for rows.Next() {
		var iconID sql.NullInt64
		var externalIconID sql.NullString
		var tz string

		entry := model.NewEntry()

		dest := []any{
			&entry.ID,
			&entry.UserID,
			&entry.FeedID,
			&entry.Hash,
			&entry.Date,
			&entry.Title,
			&entry.URL,
			&entry.CommentsURL,
			&entry.Author,
			&entry.ShareCode,
			&entry.Content,
			&entry.Status,
			&entry.Starred,
			&entry.SavedForLater,
			&entry.ReadingTime,
			&entry.CreatedAt,
			&entry.ChangedAt,
			pq.Array(&entry.Tags),
			&entry.Score,
			&entry.RawRelevanceScore,
			&entry.Vote,
			&entry.Feed.Title,
			&entry.Feed.FeedURL,
			&entry.Feed.SiteURL,
			&entry.Feed.Description,
			&entry.Feed.CheckedAt,
			&entry.Feed.Category.ID,
			&entry.Feed.Category.Title,
			&entry.Feed.Category.HideGlobally,
			&entry.Feed.ScraperRules,
			&entry.Feed.RewriteRules,
			&entry.Feed.Crawler,
			&entry.Feed.UserAgent,
			&entry.Feed.Cookie,
			&entry.Feed.HideGlobally,
			&entry.Feed.NoMediaPlayer,
			&entry.Feed.WebhookURL,
			&iconID,
			&externalIconID,
			&tz,
		}

		if withCount {
			dest = append([]any{&totalCount}, dest...)
		}

		err := rows.Scan(dest...)

		if err != nil {
			return nil, 0, fmt.Errorf("store: unable to fetch entry row: %v", err)
		}

		if iconID.Valid && externalIconID.Valid && externalIconID.String != "" {
			entry.Feed.Icon.FeedID = entry.FeedID
			entry.Feed.Icon.IconID = iconID.Int64
			entry.Feed.Icon.ExternalIconID = externalIconID.String
		} else {
			entry.Feed.Icon.IconID = 0
		}

		// Make sure that timestamp fields contain timezone information (API)
		entry.Date = timezone.Convert(tz, entry.Date)
		entry.CreatedAt = timezone.Convert(tz, entry.CreatedAt)
		entry.ChangedAt = timezone.Convert(tz, entry.ChangedAt)
		entry.Feed.CheckedAt = timezone.Convert(tz, entry.Feed.CheckedAt)

		entry.Feed.ID = entry.FeedID
		entry.Feed.UserID = entry.UserID
		entry.Feed.Icon.FeedID = entry.FeedID
		entry.Feed.Category.UserID = entry.UserID

		entries = append(entries, entry)
		entryMap[entry.ID] = entry
		entryIDs = append(entryIDs, entry.ID)
	}

	if e.fetchEnclosures && len(entryIDs) > 0 {
		enclosures, err := e.store.GetEnclosuresForEntries(e.ctx, entryIDs)
		if err != nil {
			return nil, 0, fmt.Errorf("store: unable to fetch enclosures: %w", err)
		}

		for entryID, entryEnclosures := range enclosures {
			if entry, exists := entryMap[entryID]; exists {
				entry.Enclosures = entryEnclosures
			}
		}
	}

	return entries, totalCount, nil
}

// GetEntryIDs returns a list of entry IDs that match the condition.
func (e *EntryQueryBuilder) GetEntryIDs() ([]int64, error) {
	query := `
		SELECT
			e.id
		FROM
			entries e
		LEFT JOIN
			feeds f
		ON
			f.id=e.feed_id
		WHERE ` + e.buildCondition() + " " + e.buildSorting()

	rows, err := e.store.db.QueryContext(e.ctx, query, e.args...)
	if err != nil {
		return nil, fmt.Errorf("store: unable to get entries: %v", err)
	}
	defer rows.Close()

	var entryIDs []int64
	for rows.Next() {
		var entryID int64

		err := rows.Scan(&entryID)
		if err != nil {
			return nil, fmt.Errorf("store: unable to fetch entry row: %v", err)
		}

		entryIDs = append(entryIDs, entryID)
	}

	return entryIDs, nil
}

// contentColumn selects full article content or an empty value for metadata queries.
func (e *EntryQueryBuilder) contentColumn() string {
	if e.excludeContent {
		return "'' AS content"
	}
	return "e.content"
}

func (e *EntryQueryBuilder) buildCondition() string {
	return strings.Join(e.conditions, " AND ")
}

func (e *EntryQueryBuilder) buildSorting() string {
	var parts string

	if len(e.sortExpressions) > 0 {
		parts += " ORDER BY " + strings.Join(e.sortExpressions, ", ")
	}

	if e.limit > 0 {
		parts += " LIMIT " + strconv.Itoa(e.limit)
	}

	if e.offset > 0 {
		parts += " OFFSET " + strconv.Itoa(e.offset)
	}

	return parts
}

// NewEntryQueryBuilder returns a new EntryQueryBuilder.
func NewEntryQueryBuilder(ctx context.Context, store *Storage, userID int64) *EntryQueryBuilder {
	return &EntryQueryBuilder{
		ctx:        ctx,
		store:      store,
		args:       []any{userID},
		conditions: []string{"e.user_id = $1"},
	}
}

// NewAnonymousQueryBuilder returns a new EntryQueryBuilder suitable for anonymous users.
func NewAnonymousQueryBuilder(ctx context.Context, store *Storage) *EntryQueryBuilder {
	return &EntryQueryBuilder{
		ctx:   ctx,
		store: store,
	}
}
