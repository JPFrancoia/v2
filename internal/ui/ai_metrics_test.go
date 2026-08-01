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

// TestBuildAIMetricModelViews checks canonical ordering, metric mapping, and model-specific chart data.
func TestBuildAIMetricModelViews(t *testing.T) {
	evalDate := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	views := buildAIMetricModelViews(model.ModelEvals{
		&model.ModelEval{Model: "Other", EvalDate: evalDate},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate, MetricsRPS: floatPointer(0.1), MetricsF1: floatPointer(0.2), MetricsWeightedKappa: floatPointer(-0.3)},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate.Add(-24 * time.Hour), MetricsRPS: floatPointer(0.2), MetricsF1: floatPointer(0.1)},
		&model.ModelEval{Model: "Relevance", EvalDate: evalDate},
		&model.ModelEval{
			Model:                                 "Super-important",
			EvalDate:                              evalDate,
			MetricsSuperImportantAveragePrecision: floatPointer(0.91),
			MetricsRelevanceAveragePrecision:      floatPointer(0.82),
			MetricsRecallAt10:                     floatPointer(0.3),
			MetricsRecallAt25:                     floatPointer(0.5),
			MetricsRecallAt50:                     floatPointer(0.7),
			MetricsSuperImportantBonus:            floatPointer(1.4),
		},
		&model.ModelEval{Model: "Urgency", EvalDate: evalDate},
	}, "UTC")

	if len(views) != 5 || views[0].Name != "Relevance" || views[1].Name != "Super-important" || views[2].Name != "Urgency" || views[3].Name != "Freshness" || views[4].Name != "Other" {
		t.Fatalf("unexpected model ordering: %#v", views)
	}

	superImportant := views[1]
	if !superImportant.IsSuperImportant {
		t.Fatal("super-important view is not marked as super-important")
	}
	if superImportant.Latest.MetricsRecallAt10 != 0.3 || superImportant.Latest.MetricsRecallAt25 != 0.5 || superImportant.Latest.MetricsSuperImportantBonus != 1.4 {
		t.Fatalf("super-important fields mapped incorrectly: %#v", superImportant.Latest)
	}
	for _, expected := range []string{`"super_important_average_precision":0.91`, `"relevance_average_precision":0.82`, `"recall_at_50":0.7`} {
		if !strings.Contains(superImportant.ChartData, expected) {
			t.Errorf("super-important chart data %q does not contain %q", superImportant.ChartData, expected)
		}
	}
	if strings.Contains(superImportant.ChartData, `"recall_at_10"`) || strings.Contains(superImportant.ChartData, `"bonus"`) {
		t.Fatalf("super-important chart contains detail-only metrics: %s", superImportant.ChartData)
	}

	freshness := views[3]
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

	if strings.Contains(views[0].ChartData, "rps") || !strings.Contains(views[0].ChartData, "average_precision") {
		t.Fatalf("binary chart data changed unexpectedly: %s", views[0].ChartData)
	}
}
