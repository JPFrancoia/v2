// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import "time"

// ModelEval stores one Feedoscope model evaluation result.
type ModelEval struct {
	ID                                    int64
	EvalDate                              time.Time
	Model                                 string
	EvaluationModel                       string
	Training                              map[string]int
	Eval                                  map[string]int
	MetricsAccuracy                       *float64
	MetricsPrecision                      *float64
	MetricsRecall                         *float64
	MetricsF1                             *float64
	MetricsROCAUC                         *float64
	MetricsAveragePrecision               *float64
	MetricsLogLoss                        *float64
	MetricsRPS                            *float64
	MetricsWeightedKappa                  *float64
	MetricsLogDurationMAE                 *float64
	MetricsSuperImportantAveragePrecision *float64
	MetricsRelevanceAveragePrecision      *float64
	MetricsRecallAt10                     *float64
	MetricsRecallAt25                     *float64
	MetricsRecallAt50                     *float64
	MetricsSuperImportantBonus            *float64
	CreatedAt                             time.Time
}

// ModelEvals represents a collection of model evaluations.
type ModelEvals []*ModelEval
