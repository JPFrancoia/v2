// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package ui // import "miniflux.app/v2/internal/ui"

import (
	"encoding/json"
	"fmt"
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
	Rows              []aiMetricRowView
	Latest            aiMetricRowView
	ChartData         string
	HasMultiplePoints bool
}

type aiMetricRowView struct {
	EvalDate                string
	CreatedAt               string
	Training                string
	Eval                    string
	MetricsAccuracy         float64
	MetricsPrecision        float64
	MetricsRecall           float64
	MetricsF1               float64
	MetricsROCAUC           float64
	MetricsAveragePrecision float64
	MetricsLogLoss          float64
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
		groups[modelEval.Model] = append(groups[modelEval.Model], modelEval)
	}

	modelNames := []string{"Relevance", "Urgency"}
	for modelName := range groups {
		if modelName != "Relevance" && modelName != "Urgency" {
			modelNames = append(modelNames, modelName)
		}
	}
	sort.Strings(modelNames[2:])

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
			Rows:              rowViews,
			Latest:            rowViews[0],
			ChartData:         buildAIMetricChartData(rowViews),
			HasMultiplePoints: len(rowViews) > 1,
		})
	}

	return views
}

func buildAIMetricRowView(row *model.ModelEval, userTimezone string) aiMetricRowView {
	return aiMetricRowView{
		EvalDate:                row.EvalDate.Format("2006-01-02"),
		CreatedAt:               timezone.Convert(userTimezone, row.CreatedAt).Format("2006-01-02 15:04"),
		Training:                formatAIMetricCounts(row.Training),
		Eval:                    formatAIMetricCounts(row.Eval),
		MetricsAccuracy:         row.MetricsAccuracy,
		MetricsPrecision:        row.MetricsPrecision,
		MetricsRecall:           row.MetricsRecall,
		MetricsF1:               row.MetricsF1,
		MetricsROCAUC:           row.MetricsROCAUC,
		MetricsAveragePrecision: row.MetricsAveragePrecision,
		MetricsLogLoss:          row.MetricsLogLoss,
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

func buildAIMetricChartData(rows []aiMetricRowView) string {
	points := make([]map[string]any, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		points = append(points, map[string]any{
			"date":              rows[i].EvalDate,
			"f1":                rows[i].MetricsF1,
			"roc_auc":           rows[i].MetricsROCAUC,
			"average_precision": rows[i].MetricsAveragePrecision,
		})
	}

	data, err := json.Marshal(points)
	if err != nil {
		return "[]"
	}

	return string(data)
}
