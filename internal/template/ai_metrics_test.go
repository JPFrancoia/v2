// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package template // import "miniflux.app/v2/internal/template"

import (
	"bytes"
	"strings"
	"testing"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/locale"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiscoveryHelpPopups checks that each metric header opens its own unchanged explanation.
func TestDiscoveryHelpPopups(t *testing.T) {
	config.Opts = config.NewConfigOptions()
	engine := NewEngine("")
	engine.ParseTemplates()
	printer := locale.NewPrinter("en_US")
	engine.templates["ai_metrics.html"].Funcs(map[string]any{"t": printer.Printf})
	week := map[string]any{
		"WeekStart": "2026-11-02", "ImportantCount": 6, "ReadCount": 45,
		"HasRate": true, "RatePercent": 13.3,
		"HasClassifierUpvoteRate": true, "ClassifierUpvoteRatePercent": 11.8,
		"ClassifierUpvoteCount": 2, "ClassifierReadCount": 17,
		"HasRecall": true, "RecallPercent": 100.0, "ClassifierUpvoteTotal": 2,
	}
	var output bytes.Buffer
	require.NoError(t, engine.templates["ai_metrics.html"].ExecuteTemplate(&output, "content", map[string]any{
		"total":              0,
		"importantDiscovery": map[string]any{"Latest": week, "Rows": []any{week}, "ChartData": "[]"},
	}))
	doc, err := goquery.NewDocumentFromReader(&output)
	require.NoError(t, err)
	assert.Equal(t, 3, doc.Find("th button[popovertarget]").Length())
	assert.Empty(t, doc.Find(".ai-metrics-detail-card > p.ai-metrics-note").Nodes)
	for _, tc := range []struct {
		metric      string
		title       string
		description string
	}{
		{"rate", "Rate", "Share of read articles that you starred or upvoted and did not downvote."},
		{"classifier_upvote_rate", "Classifier upvote rate", "Share of read articles with a current important prediction that you upvoted. Counts show upvoted/read articles. Weeks use the time of the latest article state change in your timezone. Current predictions and feedback can change earlier weeks."},
		{"recall", "Recall", "Recall is the share of your important articles that Jev found. Only read, upvoted articles with a stored Jev prediction count."},
	} {
		t.Run(tc.metric, func(t *testing.T) {
			id := "discovery-" + tc.metric + "-help"
			button := doc.Find("th button[popovertarget='" + id + "']")
			require.Equal(t, 1, button.Length())
			assert.Equal(t, tc.title, button.AttrOr("aria-label", ""))
			assert.Contains(t, button.Parent().Text(), tc.title)
			popup := doc.Find("#" + id + "[popover]")
			require.Equal(t, 1, popup.Length())
			assert.Equal(t, "dialog", popup.AttrOr("role", ""))
			assert.Equal(t, id+"-title", popup.AttrOr("aria-labelledby", ""))
			assert.Equal(t, tc.title, popup.Find("h3#"+id+"-title").Text())
			assert.Equal(t, id+"-text", popup.AttrOr("aria-describedby", ""))
			assert.Equal(t, tc.description, popup.Find("p#"+id+"-text").Text())
			assert.Zero(t, popup.ParentsFiltered(".ai-metrics-table-wrapper").Length())
			closeButton := popup.Find("button[popovertarget='" + id + "'][popovertargetaction='hide']")
			require.Equal(t, 1, closeButton.Length())
			assert.Equal(t, printer.Printf("page.keyboard_shortcuts.close_modal"), closeButton.AttrOr("aria-label", ""))
		})
	}
	assert.Equal(t, "2026-11-02 6 45 13.3% 11.8% (2/17) 100.0% (2/2)", strings.Join(strings.Fields(doc.Find("tbody").Text()), " "))
}
