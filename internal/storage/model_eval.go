// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"miniflux.app/v2/internal/model"
)

const defaultModelEvalLimit = 200

// ModelEvals returns recent Feedoscope model evaluation results.
func (s *Storage) ModelEvals(limit int) (model.ModelEvals, error) {
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
			metrics_accuracy,
			metrics_precision,
			metrics_recall,
			metrics_f1,
			metrics_roc_auc,
			metrics_average_precision,
			metrics_log_loss,
			metrics_rps,
			metrics_weighted_kappa,
			metrics_log_duration_mae,
			metrics_super_important_average_precision,
			metrics_relevance_average_precision,
			metrics_recall_at_10,
			metrics_recall_at_25,
			metrics_recall_at_50,
			metrics_super_important_bonus,
			created_at
		FROM model_evals
		ORDER BY eval_date DESC, created_at DESC, model ASC
		LIMIT $1
	`
	rows, err := s.db.Query(query, limit)
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

		err := rows.Scan(
			&modelEval.ID,
			&modelEval.EvalDate,
			&modelEval.Model,
			&evaluationModel,
			&trainingData,
			&evalData,
			&modelEval.MetricsAccuracy,
			&modelEval.MetricsPrecision,
			&modelEval.MetricsRecall,
			&modelEval.MetricsF1,
			&modelEval.MetricsROCAUC,
			&modelEval.MetricsAveragePrecision,
			&modelEval.MetricsLogLoss,
			&modelEval.MetricsRPS,
			&modelEval.MetricsWeightedKappa,
			&modelEval.MetricsLogDurationMAE,
			&modelEval.MetricsSuperImportantAveragePrecision,
			&modelEval.MetricsRelevanceAveragePrecision,
			&modelEval.MetricsRecallAt10,
			&modelEval.MetricsRecallAt25,
			&modelEval.MetricsRecallAt50,
			&modelEval.MetricsSuperImportantBonus,
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

		modelEvals = append(modelEvals, &modelEval)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(`store: unable to fetch model evaluation rows: %v`, err)
	}

	return modelEvals, nil
}
