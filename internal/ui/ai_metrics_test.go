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
}

// TestBuildAIMetricModelViews checks filtering, canonical ordering, metric mapping, and model-specific chart data.
func TestBuildAIMetricModelViews(t *testing.T) {
	evalDate := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	views := buildAIMetricModelViews(model.ModelEvals{
		&model.ModelEval{Model: "Other", EvalDate: evalDate},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate, MetricsRPS: floatPointer(0.1), MetricsF1: floatPointer(0.2), MetricsWeightedKappa: floatPointer(-0.3)},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate.Add(-24 * time.Hour), MetricsRPS: floatPointer(0.2), MetricsF1: floatPointer(0.1)},
		&model.ModelEval{Model: "Relevance", EvaluationModel: relevancePrecisionAt50EvaluationModel, EvalDate: evalDate, MetricsPrecision: floatPointer(0.94), MetricsAveragePrecision: floatPointer(0.9)},
		&model.ModelEval{Model: "Relevance", EvalDate: evalDate.Add(-24 * time.Hour), MetricsPrecision: floatPointer(0.85), MetricsF1: floatPointer(0.8), MetricsAveragePrecision: floatPointer(0.8)},
		&model.ModelEval{Model: "Super-important", EvalDate: evalDate},
		&model.ModelEval{Model: "Urgency", EvalDate: evalDate, MetricsF1: floatPointer(0.7), MetricsROCAUC: floatPointer(0.8), MetricsAveragePrecision: floatPointer(0.9)},
	}, "UTC")

	if len(views) != 4 || views[0].Name != "Relevance" || views[1].Name != "Urgency" || views[2].Name != "Freshness" || views[3].Name != "Other" {
		t.Fatalf("unexpected model filtering or ordering: %#v", views)
	}

	relevance := views[0]
	if !relevance.IsRelevance {
		t.Fatal("relevance view is not marked as relevance")
	}
	if !relevance.Latest.HasMetricsPrecisionAt50 {
		t.Fatal("new relevance precision should be marked as Precision@50")
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

	freshness := views[2]
	if !freshness.IsFreshness {
		t.Fatal("freshness view is not marked as freshness")
	}
	if freshness.Rows[1].HasMetricsWeightedKappa {
		t.Fatal("nil weighted kappa should remain undefined")
	}
	if freshness.Latest.HasMetricsROCAUC {
		t.Fatal("nil long-lived AUC should remain undefined")
	}
	for _, expected := range []string{`"rps":0.1`, `"f1":0.2`, `"weighted_kappa":-0.3`} {
		if !strings.Contains(freshness.ChartData, expected) {
			t.Errorf("freshness chart data %q does not contain %q", freshness.ChartData, expected)
		}
	}
	if strings.Count(freshness.ChartData, `"weighted_kappa"`) != 1 {
		t.Fatalf("missing weighted kappa should be omitted from chart data: %s", freshness.ChartData)
	}

	if strings.Contains(views[1].ChartData, "rps") || !strings.Contains(views[1].ChartData, `"f1":0.7`) {
		t.Fatalf("urgency chart data changed unexpectedly: %s", views[1].ChartData)
	}
	if relevance.Latest.EvaluationModel != relevancePrecisionAt50EvaluationModel {
		t.Fatalf("precision metric contract was not preserved: %q", relevance.Latest.EvaluationModel)
	}
	if relevance.Rows[1].EvaluationModel != "-" {
		t.Fatalf("missing evaluation model should display as '-', got %q", relevance.Rows[1].EvaluationModel)
	}
}
