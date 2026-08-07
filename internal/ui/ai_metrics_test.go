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

// TestBuildAIMetricModelViews checks filtering, canonical ordering, metric mapping, and model-specific chart data.
func TestBuildAIMetricModelViews(t *testing.T) {
	evalDate := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	views := buildAIMetricModelViews(model.ModelEvals{
		&model.ModelEval{Model: "Other", EvalDate: evalDate},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate, MetricsRPS: floatPointer(0.1), MetricsF1: floatPointer(0.2), MetricsWeightedKappa: floatPointer(-0.3)},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate.Add(-24 * time.Hour), MetricsRPS: floatPointer(0.2), MetricsF1: floatPointer(0.1)},
		&model.ModelEval{Model: "Relevance", EvalDate: evalDate, MetricsAveragePrecision: floatPointer(0.9), MetricsRecallAt10: floatPointer(0.2), MetricsRecallAt25: floatPointer(0.4), MetricsRecallAt50: floatPointer(0.6)},
		&model.ModelEval{Model: "Relevance", EvalDate: evalDate.Add(-24 * time.Hour), MetricsAveragePrecision: floatPointer(0.8)},
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
	if relevance.Rows[1].HasMetricsRecallAt10 || relevance.Rows[1].HasMetricsRecallAt25 || relevance.Rows[1].HasMetricsRecallAt50 {
		t.Fatal("nil relevance recall metrics should remain undefined")
	}
	for _, expected := range []string{`"average_precision":0.9`, `"recall_at_10":0.2`, `"recall_at_25":0.4`, `"recall_at_50":0.6`} {
		if !strings.Contains(relevance.ChartData, expected) {
			t.Errorf("relevance chart data %q does not contain %q", relevance.ChartData, expected)
		}
	}
	for _, key := range []string{`"recall_at_10"`, `"recall_at_25"`, `"recall_at_50"`} {
		if strings.Count(relevance.ChartData, key) != 1 {
			t.Fatalf("missing historical relevance metric should be omitted from chart data: %s", relevance.ChartData)
		}
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
	if views[0].Latest.EvaluationModel != "-" {
		t.Fatalf("missing evaluation model should display as '-', got %q", views[0].Latest.EvaluationModel)
	}
}
