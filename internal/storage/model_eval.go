// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"miniflux.app/v2/internal/model"
)

const defaultModelEvalLimit = 200

// ModelEvals returns recent Feedoscope model evaluation results.
func (s *Storage) ModelEvals(ctx context.Context, limit int) (model.ModelEvals, error) {
	if limit <= 0 {
		limit = defaultModelEvalLimit
	}

	query := `
		SELECT
			id,
			eval_date,
			model,
			evaluation_model,
			training,
			eval,
			metrics,
			created_at
		FROM model_evals
		ORDER BY eval_date DESC, created_at DESC, model ASC
		LIMIT $1
	`
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf(`store: unable to fetch model evaluations: %v`, err)
	}
	defer rows.Close()

	modelEvals := make(model.ModelEvals, 0)
	for rows.Next() {
		var modelEval model.ModelEval
		var evaluationModel sql.NullString
		var trainingData []byte
		var evalData []byte
		var metricsData []byte

		err := rows.Scan(
			&modelEval.ID,
			&modelEval.EvalDate,
			&modelEval.Model,
			&evaluationModel,
			&trainingData,
			&evalData,
			&metricsData,
			&modelEval.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf(`store: unable to fetch model evaluation row: %v`, err)
		}
		if evaluationModel.Valid {
			modelEval.EvaluationModel = evaluationModel.String
		}

		if err := json.Unmarshal(trainingData, &modelEval.Training); err != nil {
			return nil, fmt.Errorf(`store: unable to parse model evaluation training counts: %v`, err)
		}
		if err := json.Unmarshal(evalData, &modelEval.Eval); err != nil {
			return nil, fmt.Errorf(`store: unable to parse model evaluation eval counts: %v`, err)
		}
		if err := json.Unmarshal(metricsData, &modelEval.Metrics); err != nil {
			return nil, fmt.Errorf(`store: unable to parse model evaluation metrics: %v`, err)
		}

		modelEvals = append(modelEvals, &modelEval)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch model evaluation rows: %v`, err)
	}

	return modelEvals, nil
}
