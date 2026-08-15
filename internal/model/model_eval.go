// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package model // import "miniflux.app/v2/internal/model"

import "time"

// ModelEval stores one Feedoscope model evaluation result.
type ModelEval struct {
	ID              int64
	EvalDate        time.Time
	Model           string
	EvaluationModel string
	Training        map[string]int
	Eval            map[string]int
	Metrics         map[string]float64
	CreatedAt       time.Time
}

// ModelEvals represents a collection of model evaluations.
type ModelEvals []*ModelEval
