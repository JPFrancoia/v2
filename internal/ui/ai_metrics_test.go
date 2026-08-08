// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui

import (
	"strings"
	"testing"
	"time"

	"miniflux.app/v2/internal/model"
)

func floatPointer(value float64) *float64 {
	return &value
}

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
	if !strings.HasPrefix(result.ChartData, `[{"date":"2026-07-27","rate":null}`) ||
		!strings.Contains(result.ChartData, `{"date":"2026-08-03","rate":0.147`) {
		t.Fatalf("unexpected chronological chart data: %s", result.ChartData)
	}
}

// TestBuildAIMetricModelViewsHidesRemovedModels checks filtering and Relevance metrics.
func TestBuildAIMetricModelViewsHidesRemovedModels(t *testing.T) {
	evalDate := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	views := buildAIMetricModelViews(model.ModelEvals{
		&model.ModelEval{Model: "Other", EvalDate: evalDate},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate},
		&model.ModelEval{Model: "Relevance", EvaluationModel: relevancePrecisionAt50EvaluationModel, EvalDate: evalDate, MetricsPrecision: floatPointer(0.94), MetricsAveragePrecision: floatPointer(0.9)},
		&model.ModelEval{Model: "Relevance", EvalDate: evalDate.Add(-24 * time.Hour), MetricsPrecision: floatPointer(0.85), MetricsAveragePrecision: floatPointer(0.8)},
		&model.ModelEval{Model: "Super-important", EvalDate: evalDate},
		&model.ModelEval{Model: "Urgency", EvalDate: evalDate},
	}, "UTC")

	if len(views) != 2 || views[0].Name != "Relevance" || views[1].Name != "Other" {
		t.Fatalf("unexpected model filtering or ordering: %#v", views)
	}

	relevance := views[0]
	if !relevance.IsRelevance || !relevance.Latest.HasMetricsPrecisionAt50 {
		t.Fatalf("unexpected relevance view: %#v", relevance)
	}
	if relevance.Rows[1].HasMetricsPrecisionAt50 {
		t.Fatal("historical threshold precision should not be marked as Precision@50")
	}
	for _, expected := range []string{`"average_precision":0.9`, `"precision_at_50":0.94`} {
		if !strings.Contains(relevance.ChartData, expected) {
			t.Errorf("relevance chart data %q does not contain %q", relevance.ChartData, expected)
		}
	}
	if strings.Count(relevance.ChartData, `"precision_at_50"`) != 1 {
		t.Fatalf("historical threshold precision should be omitted from chart data: %s", relevance.ChartData)
	}
	for _, removedName := range []string{"Urgency", "Freshness"} {
		if strings.Contains(relevance.ChartData, removedName) {
			t.Fatalf("removed model leaked into chart: %s", relevance.ChartData)
		}
	}
}
