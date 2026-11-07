// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildImportantDiscoveryView checks weekly rate formatting and empty-week handling.
func TestBuildImportantDiscoveryView(t *testing.T) {
	weekStart := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	result := buildImportantDiscoveryView(model.ImportantDiscoveryWeeks{
		&model.ImportantDiscoveryWeek{WeekStart: weekStart, ImportantCount: 5, ReadCount: 34},
		&model.ImportantDiscoveryWeek{WeekStart: weekStart.Add(-7 * 24 * time.Hour)},
	})

	if len(result.Rows) != 2 || result.Latest.WeekStart != "2026-08-03" {
		t.Fatalf("unexpected discovery weeks: %#v", result)
	}
	if !result.Latest.HasRate || result.Latest.RatePercent < 14.70 || result.Latest.RatePercent > 14.71 {
		t.Fatalf("unexpected discovery rate: %#v", result.Latest)
	}
	if result.Rows[1].HasRate || result.Rows[1].RatePercent != 0 {
		t.Fatalf("an empty week should not have a rate: %#v", result.Rows[1])
	}
	var points []map[string]any
	require.NoError(t, json.Unmarshal([]byte(result.ChartData), &points))
	require.Len(t, points, 2)
	assert.Equal(t, "2026-07-27", points[0]["date"])
	assert.Nil(t, points[0]["rate"])
	assert.InDelta(t, 5.0/34, points[1]["rate"], 0.00001)
}

// TestBuildAIMetricModelViewsHidesRemovedModels checks filtering and Relevance metrics.
func TestBuildAIMetricModelViewsHidesRemovedModels(t *testing.T) {
	evalDate := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	views := buildAIMetricModelViews(model.ModelEvals{
		&model.ModelEval{Model: "Other", EvalDate: evalDate},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate},
		&model.ModelEval{Model: "Relevance", EvaluationModel: "future implementation name", EvalDate: evalDate, Metrics: map[string]float64{"precision_at_50": 0.95, "average_precision": 0.91}},
		&model.ModelEval{Model: "Relevance", EvaluationModel: "renamed implementation", EvalDate: evalDate.Add(-24 * time.Hour), Metrics: map[string]float64{"precision_at_50": 0.94, "average_precision": 0.9}},
		&model.ModelEval{Model: "Relevance", EvalDate: evalDate.Add(-48 * time.Hour), Metrics: map[string]float64{"precision": 0.85, "average_precision": 0.8}},
		&model.ModelEval{Model: "Super-important", EvalDate: evalDate},
		&model.ModelEval{Model: "Urgency", EvalDate: evalDate},
	}, "UTC")

	if len(views) != 2 || views[0].Name != "Relevance" || views[1].Name != "Other" {
		t.Fatalf("unexpected model filtering or ordering: %#v", views)
	}

	relevance := views[0]
	if !relevance.IsRelevance || !relevance.Latest.HasMetricsPrecisionAt50 || relevance.Latest.MetricsPrecisionAt50 != 0.95 {
		t.Fatalf("unexpected relevance view: %#v", relevance)
	}
	if !relevance.Rows[1].HasMetricsPrecisionAt50 {
		t.Fatalf("old Precision@50 contract should remain visible: %#v", relevance.Rows[1])
	}
	if relevance.Rows[2].HasMetricsPrecisionAt50 {
		t.Fatal("historical threshold precision should not be marked as Precision@50")
	}
	for _, expected := range []string{`"average_precision":0.91`, `"precision_at_50":0.95`, `"precision_at_50":0.94`} {
		if !strings.Contains(relevance.ChartData, expected) {
			t.Errorf("relevance chart data %q does not contain %q", relevance.ChartData, expected)
		}
	}
	if strings.Count(relevance.ChartData, `"precision_at_50"`) != 2 {
		t.Fatalf("historical threshold precision should be omitted from chart data: %s", relevance.ChartData)
	}
	for _, removedName := range []string{"Urgency", "Freshness"} {
		if strings.Contains(relevance.ChartData, removedName) {
			t.Fatalf("removed model leaked into chart: %s", relevance.ChartData)
		}
	}
}

// TestBuildImportantDiscoveryClassifierUpvoteRate checks counts, missing rates, and normalized chart values.
func TestBuildImportantDiscoveryClassifierUpvoteRate(t *testing.T) {
	weekStart := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	result := buildImportantDiscoveryView(model.ImportantDiscoveryWeeks{
		{WeekStart: weekStart, ImportantCount: 5, ReadCount: 10, ClassifierReadCount: 6, ClassifierUpvoteCount: 3},
		{WeekStart: weekStart.AddDate(0, 0, -7), ReadCount: 2, ClassifierReadCount: 2},
		{WeekStart: weekStart.AddDate(0, 0, -14), ImportantCount: 1, ReadCount: 1},
	})
	assert.Equal(t, 3, result.Latest.ClassifierUpvoteCount)
	assert.Equal(t, 6, result.Latest.ClassifierReadCount)
	assert.True(t, result.Latest.HasClassifierUpvoteRate)
	assert.Equal(t, 50.0, result.Latest.ClassifierUpvoteRatePercent)
	assert.Equal(t, 50.0, result.Latest.RatePercent)
	assert.True(t, result.Rows[1].HasClassifierUpvoteRate)
	assert.Zero(t, result.Rows[1].ClassifierUpvoteRatePercent)
	assert.False(t, result.Rows[2].HasClassifierUpvoteRate)
	assert.Equal(t, 100.0, result.Rows[2].RatePercent)
	assert.JSONEq(t, `[
		{"date":"2026-07-20","rate":1,"classifier_upvote_rate":null,"recall":null},
		{"date":"2026-07-27","rate":0,"classifier_upvote_rate":0,"recall":null},
		{"date":"2026-08-03","rate":0.5,"classifier_upvote_rate":0.5,"recall":1}
	]`, result.ChartData)
}

// TestImportantDiscoveryClassifierRendering checks displayed counts and separate chart controls.
func TestImportantDiscoveryClassifierRendering(t *testing.T) {
	config.Opts = config.NewConfigOptions()
	engine := template.NewEngine("")
	engine.ParseTemplates()
	view := buildImportantDiscoveryView(model.ImportantDiscoveryWeeks{
		{WeekStart: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), ClassifierReadCount: 6, ClassifierUpvoteCount: 3},
		{WeekStart: time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)},
		{WeekStart: time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC), ClassifierReadCount: 2},
	})
	output := string(engine.Render("ai_metrics.html", map[string]any{
		"language": "en_US", "theme": "light", "total": 0, "importantDiscovery": view,
	}))
	assert.Contains(t, output, "Classifier upvote rate")
	assert.Contains(t, output, "50.0% (3/6)")
	assert.Contains(t, output, "- (0/0)")
	assert.Contains(t, output, "0.0% (0/2)")
	assert.Contains(t, output, `data-ai-metrics-series-toggle="rate"`)
	assert.Contains(t, output, `data-ai-metrics-series-toggle="classifier_upvote_rate"`)
	assert.Contains(t, output, `data-label-classifier-upvote-rate="Classifier upvote rate"`)
	assert.Contains(t, output, `ai-metrics-series-average-precision`)
	assert.Contains(t, output, "Current predictions and feedback can change earlier weeks.")
}

// TestImportantDiscoveryRecall checks found/missed percentages and normalized graph gaps.
func TestImportantDiscoveryRecall(t *testing.T) {
	week := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name          string
		found, missed int
		want          float64
		has           bool
	}{
		{"seven_of_ten", 7, 3, 70, true}, {"all_missed", 0, 3, 0, true},
		{"all_found", 4, 0, 100, true}, {"no_classified_upvotes", 0, 0, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := buildImportantDiscoveryView(model.ImportantDiscoveryWeeks{{WeekStart: week, ClassifierUpvoteCount: tc.found, ClassifierMissedCount: tc.missed}})
			assert.Equal(t, tc.found+tc.missed, result.Latest.ClassifierUpvoteTotal)
			assert.Equal(t, tc.has, result.Latest.HasRecall)
			assert.Equal(t, tc.want, result.Latest.RecallPercent)
			var points []map[string]any
			require.NoError(t, json.Unmarshal([]byte(result.ChartData), &points))
			require.Len(t, points, 1)
			if tc.has {
				assert.InDelta(t, tc.want/100, points[0]["recall"], 0.00001)
			} else {
				assert.Nil(t, points[0]["recall"])
			}
		})
	}
}

// TestImportantDiscoveryRecallRendering checks the short definition, counts, and separate graph control.
func TestImportantDiscoveryRecallRendering(t *testing.T) {
	config.Opts = config.NewConfigOptions()
	engine := template.NewEngine("")
	engine.ParseTemplates()
	view := buildImportantDiscoveryView(model.ImportantDiscoveryWeeks{{ClassifierUpvoteCount: 7, ClassifierMissedCount: 3}})
	output := string(engine.Render("ai_metrics.html", map[string]any{"language": "en_US", "theme": "light", "total": 0, "importantDiscovery": view}))
	assert.Contains(t, output, "70.0% (7/10)")
	assert.Contains(t, output, "Recall is the share of your important articles that Jev found.")
	assert.Contains(t, output, "Only read, upvoted articles with a stored Jev prediction count.")
	assert.Contains(t, output, `data-ai-metrics-series-toggle="recall"`)
	assert.Contains(t, output, `data-label-recall="Recall"`)
	assert.NotContains(t, output, "/missed")
}
