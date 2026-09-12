// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package worker // import "miniflux.app/v2/internal/worker"

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"miniflux.app/v2/internal/config"
	"miniflux.app/v2/internal/locale"
	"miniflux.app/v2/internal/metric"
	"miniflux.app/v2/internal/model"
	feedHandler "miniflux.app/v2/internal/reader/handler"
	"miniflux.app/v2/internal/storage"
)

type worker struct {
	id    int
	store *storage.Storage
}

// Run processes feed refresh jobs from the channel until it is closed.
func (w *worker) Run(c <-chan model.Job, wg *sync.WaitGroup) {
	defer wg.Done()

	slog.Debug("Worker started",
		slog.Int("worker_id", w.id),
	)

	for job := range c {
		slog.Debug("Job received by worker",
			slog.Int("worker_id", w.id),
			slog.Int64("user_id", job.UserID),
			slog.Int64("feed_id", job.FeedID),
			slog.String("feed_url", job.FeedURL),
		)

		startTime := time.Now()
		localizedError := w.refreshFeed(job)

		if config.Opts.HasMetricsCollector() {
			status := metric.StatusSuccess
			if localizedError != nil {
				status = metric.StatusError
			}
			metric.BackgroundFeedRefreshDuration.WithLabelValues(status).Observe(time.Since(startTime).Seconds())
		}
	}
}

func (w *worker) refreshFeed(job model.Job) *locale.LocalizedErrorWrapper {
	ctx, span := otel.Tracer("miniflux.app/worker").Start(context.Background(), "miniflux.feed.refresh")
	defer span.End()

	localizedError := feedHandler.RefreshFeed(ctx, w.store, job.UserID, job.FeedID, false)
	if localizedError != nil {
		err := localizedError.Error()
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return localizedError
}
