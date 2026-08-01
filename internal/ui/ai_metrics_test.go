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

// TestBuildAIMetricModelViewsFreshness checks model ordering and Freshness-specific chart/null handling.
func TestBuildAIMetricModelViewsFreshness(t *testing.T) {
	evalDate := time.Date(2026, 7, 26, 0, 0, 0, 0, time.UTC)
	views := buildAIMetricModelViews(model.ModelEvals{
		&model.ModelEval{Model: "Other", EvalDate: evalDate},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate, MetricsRPS: floatPointer(0.1), MetricsF1: floatPointer(0.2), MetricsWeightedKappa: floatPointer(-0.3)},
		&model.ModelEval{Model: "Freshness", EvalDate: evalDate.Add(-24 * time.Hour), MetricsRPS: floatPointer(0.2), MetricsF1: floatPointer(0.1)},
		&model.ModelEval{Model: "Relevance", EvalDate: evalDate},
		&model.ModelEval{Model: "Urgency", EvalDate: evalDate},
	}, "UTC")

	if len(views) != 4 || views[0].Name != "Relevance" || views[1].Name != "Urgency" || views[2].Name != "Freshness" || views[3].Name != "Other" {
		t.Fatalf("unexpected model ordering: %#v", views)
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

	if strings.Contains(views[0].ChartData, "rps") || !strings.Contains(views[0].ChartData, "average_precision") {
		t.Fatalf("binary chart data changed unexpectedly: %s", views[0].ChartData)
	}
}
