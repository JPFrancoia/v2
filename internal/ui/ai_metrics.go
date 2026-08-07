// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"

	"miniflux.app/v2/internal/http/request"
	"miniflux.app/v2/internal/http/response"
	"miniflux.app/v2/internal/model"
	"miniflux.app/v2/internal/timezone"
	"miniflux.app/v2/internal/ui/view"
)

const aiMetricsEvalLimit = 200

type aiMetricModelView struct {
	Name              string
	IsFreshness       bool
	IsSuperImportant  bool
	Rows              []aiMetricRowView
	Latest            aiMetricRowView
	ChartData         string
	HasMultiplePoints bool
}

type aiMetricRowView struct {
	EvalDate                              string
	EvaluationModel                       string
	CreatedAt                             string
	Training                              string
	Eval                                  string
	MetricsAccuracy                       float64
	MetricsPrecision                      float64
	MetricsRecall                         float64
	MetricsF1                             float64
	MetricsROCAUC                         float64
	MetricsAveragePrecision               float64
	MetricsLogLoss                        float64
	MetricsRPS                            float64
	MetricsWeightedKappa                  float64
	MetricsLogDurationMAE                 float64
	MetricsSuperImportantAveragePrecision float64
	MetricsRelevanceAveragePrecision      float64
	MetricsRecallAt10                     float64
	MetricsRecallAt25                     float64
	MetricsRecallAt50                     float64
	MetricsSuperImportantBonus            float64
	HasMetricsROCAUC                      bool
	HasMetricsWeightedKappa               bool
}

func (h *handler) showAIMetricsPage(w http.ResponseWriter, r *http.Request) {
	user, err := h.store.UserByID(request.UserID(r))
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	if !user.IsAdmin {
		response.HTMLForbidden(w, r)
		return
	}

	modelEvals, err := h.store.ModelEvals(aiMetricsEvalLimit)
	if err != nil {
		response.HTMLServerError(w, r, err)
		return
	}

	view := view.New(h.tpl, r)
	view.Set("models", buildAIMetricModelViews(modelEvals, user.Timezone))
	view.Set("total", len(modelEvals))
	view.Set("menu", "ai_metrics")
	view.Set("user", user)
	view.Set("countUnread", h.store.CountUnreadEntries(user.ID))
	view.Set("countErrorFeeds", h.store.CountUserFeedsWithErrors(user.ID))

	response.HTML(w, r, view.Render("ai_metrics"))
}

func buildAIMetricModelViews(modelEvals model.ModelEvals, userTimezone string) []aiMetricModelView {
	groups := make(map[string]model.ModelEvals)
	for _, modelEval := range modelEvals {
		if modelEval.Model == "Super-important" {
			continue
		}
		groups[modelEval.Model] = append(groups[modelEval.Model], modelEval)
	}

	modelNames := []string{"Relevance", "Urgency", "Freshness"}
	for modelName := range groups {
		if modelName != "Relevance" && modelName != "Urgency" && modelName != "Freshness" {
			modelNames = append(modelNames, modelName)
		}
	}
	sort.Strings(modelNames[3:])

	views := make([]aiMetricModelView, 0, len(groups))
	for _, modelName := range modelNames {
		rows := groups[modelName]
		if len(rows) == 0 {
			continue
		}

		rowViews := make([]aiMetricRowView, 0, len(rows))
		for _, row := range rows {
			rowViews = append(rowViews, buildAIMetricRowView(row, userTimezone))
		}

		views = append(views, aiMetricModelView{
			Name:              modelName,
			IsFreshness:       modelName == "Freshness",
			IsSuperImportant:  modelName == "Super-important",
			Rows:              rowViews,
			Latest:            rowViews[0],
			ChartData:         buildAIMetricChartData(rowViews, modelName),
			HasMultiplePoints: len(rowViews) > 1,
		})
	}

	return views
}

func metricPresent(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

func metricValue(value *float64) float64 {
	if !metricPresent(value) {
		return 0
	}
	return *value
}

func buildAIMetricRowView(row *model.ModelEval, userTimezone string) aiMetricRowView {
	evaluationModel := row.EvaluationModel
	if evaluationModel == "" {
		evaluationModel = "-"
	}

	return aiMetricRowView{
		EvalDate:                              row.EvalDate.Format("2006-01-02"),
		EvaluationModel:                       evaluationModel,
		CreatedAt:                             timezone.Convert(userTimezone, row.CreatedAt).Format("2006-01-02 15:04"),
		Training:                              formatAIMetricCounts(row.Training),
		Eval:                                  formatAIMetricCounts(row.Eval),
		MetricsAccuracy:                       metricValue(row.MetricsAccuracy),
		MetricsPrecision:                      metricValue(row.MetricsPrecision),
		MetricsRecall:                         metricValue(row.MetricsRecall),
		MetricsF1:                             metricValue(row.MetricsF1),
		MetricsROCAUC:                         metricValue(row.MetricsROCAUC),
		MetricsAveragePrecision:               metricValue(row.MetricsAveragePrecision),
		MetricsLogLoss:                        metricValue(row.MetricsLogLoss),
		MetricsRPS:                            metricValue(row.MetricsRPS),
		MetricsWeightedKappa:                  metricValue(row.MetricsWeightedKappa),
		MetricsLogDurationMAE:                 metricValue(row.MetricsLogDurationMAE),
		MetricsSuperImportantAveragePrecision: metricValue(row.MetricsSuperImportantAveragePrecision),
		MetricsRelevanceAveragePrecision:      metricValue(row.MetricsRelevanceAveragePrecision),
		MetricsRecallAt10:                     metricValue(row.MetricsRecallAt10),
		MetricsRecallAt25:                     metricValue(row.MetricsRecallAt25),
		MetricsRecallAt50:                     metricValue(row.MetricsRecallAt50),
		MetricsSuperImportantBonus:            metricValue(row.MetricsSuperImportantBonus),
		HasMetricsROCAUC:                      metricPresent(row.MetricsROCAUC),
		HasMetricsWeightedKappa:               metricPresent(row.MetricsWeightedKappa),
	}
}

func formatAIMetricCounts(counts map[string]int) string {
	if len(counts) == 0 {
		return "-"
	}

	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s: %d", key, counts[key]))
	}

	return strings.Join(parts, ", ")
}

func buildAIMetricChartData(rows []aiMetricRowView, modelName string) string {
	points := make([]map[string]any, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		point := map[string]any{"date": rows[i].EvalDate}
		switch modelName {
		case "Freshness":
			point["rps"] = rows[i].MetricsRPS
			point["f1"] = rows[i].MetricsF1
			if rows[i].HasMetricsWeightedKappa {
				point["weighted_kappa"] = rows[i].MetricsWeightedKappa
			}
			if rows[i].HasMetricsROCAUC {
				point["roc_auc"] = rows[i].MetricsROCAUC
			}
		case "Super-important":
			point["super_important_average_precision"] = rows[i].MetricsSuperImportantAveragePrecision
			point["relevance_average_precision"] = rows[i].MetricsRelevanceAveragePrecision
			point["recall_at_50"] = rows[i].MetricsRecallAt50
		default:
			point["f1"] = rows[i].MetricsF1
			point["roc_auc"] = rows[i].MetricsROCAUC
			point["average_precision"] = rows[i].MetricsAveragePrecision
		}
		points = append(points, point)
	}

	data, err := json.Marshal(points)
	if err != nil {
		return "[]"
	}

	return string(data)
}
