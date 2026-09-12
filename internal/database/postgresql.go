// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package database // import "miniflux.app/v2/internal/database"

import (
	"database/sql"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/lib/pq"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

// NewConnectionPool configures the database connection pool.
func NewConnectionPool(dsn string, minConnections, maxConnections int, connectionLifetime time.Duration) (*sql.DB, error) {
	return newConnectionPool("postgres", dsn, minConnections, maxConnections, connectionLifetime)
}

func newConnectionPool(driverName, dsn string, minConnections, maxConnections int, connectionLifetime time.Duration) (*sql.DB, error) {
	db, err := otelsql.Open(driverName, dsn,
		otelsql.WithAttributes(semconv.DBSystemNamePostgreSQL),
		otelsql.WithMeterProvider(noop.NewMeterProvider()),
		otelsql.WithSpanOptions(otelsql.SpanOptions{OmitRows: true}),
	)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(maxConnections)
	db.SetMaxIdleConns(minConnections)
	db.SetConnMaxLifetime(connectionLifetime)

	return db, nil
}
