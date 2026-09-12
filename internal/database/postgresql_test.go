// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestConnectionPoolCreatesCorrelatedSQLSpanWithoutArguments verifies trace correlation and argument privacy.
func TestConnectionPoolCreatesCorrelatedSQLSpanWithoutArguments(t *testing.T) {
	db, recorder, provider := newTracingTestDatabase(t, "miniflux-otel-exec-test", &tracingTestDriver{})

	ctx, parent := provider.Tracer("test").Start(context.Background(), "request")
	parentID := parent.SpanContext().SpanID()
	if _, err := db.ExecContext(ctx, "SELECT $1", "secret-value"); err != nil {
		t.Fatal(err)
	}
	parent.End()

	var sqlSpan sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() != "request" {
			sqlSpan = span
			break
		}
	}
	if sqlSpan == nil {
		t.Fatal("database span was not recorded")
	}
	if sqlSpan.Parent().SpanID() != parentID {
		t.Fatalf("database span parent = %s, want %s", sqlSpan.Parent().SpanID(), parentID)
	}

	attributes := make(map[string]string)
	for _, attr := range sqlSpan.Attributes() {
		value := fmt.Sprint(attr.Value.AsInterface())
		attributes[string(attr.Key)] = value
		if strings.Contains(value, "secret-value") {
			t.Fatalf("database span contains a bound argument: %s", value)
		}
	}
	if attributes["db.system.name"] != "postgresql" {
		t.Fatalf("db.system.name = %q, want postgresql", attributes["db.system.name"])
	}
	if attributes["db.query.text"] != "SELECT $1" {
		t.Fatalf("db.query.text = %q, want SELECT $1", attributes["db.query.text"])
	}
}

// TestConnectionPoolOmitsRowsSpans verifies that row iteration does not create extra spans.
func TestConnectionPoolOmitsRowsSpans(t *testing.T) {
	db, recorder, provider := newTracingTestDatabase(t, "miniflux-otel-query-test", &tracingTestDriver{})

	ctx, parent := provider.Tracer("test").Start(context.Background(), "request")
	rows, err := db.QueryContext(ctx, "SELECT $1", "secret-value")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var value string
	if !rows.Next() {
		t.Fatal("query returned no rows")
	}
	if err := rows.Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "result" {
		t.Fatalf("query value = %q, want result", value)
	}
	if rows.Next() {
		t.Fatal("query returned an unexpected second row")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	parent.End()

	databaseSpans := 0
	for _, span := range recorder.Ended() {
		if span.Name() != "request" {
			databaseSpans++
		}
	}
	if databaseSpans != 1 {
		t.Fatalf("database span count = %d, want 1", databaseSpans)
	}
}

// TestConnectionPoolRollsBackCanceledTransaction verifies transaction cancellation reaches the driver.
func TestConnectionPoolRollsBackCanceledTransaction(t *testing.T) {
	testDriver := &tracingTestDriver{
		blockExec:   true,
		execStarted: make(chan struct{}),
		rolledBack:  make(chan struct{}),
	}
	db, _, _ := newTracingTestDatabase(t, "miniflux-otel-cancel-test", testDriver)

	ctx, cancel := context.WithCancel(context.Background())
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	execResult := make(chan error, 1)
	go func() {
		_, err := tx.ExecContext(ctx, "SELECT $1", "secret-value")
		execResult <- err
	}()

	select {
	case <-testDriver.execStarted:
	case <-time.After(time.Second):
		t.Fatal("transaction execution did not start")
	}
	cancel()

	select {
	case err := <-execResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("transaction error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("transaction execution did not stop after cancellation")
	}

	select {
	case <-testDriver.rolledBack:
	case <-time.After(time.Second):
		t.Fatal("transaction was not rolled back after cancellation")
	}
}

func newTracingTestDatabase(t *testing.T, driverName string, testDriver *tracingTestDriver) (*sql.DB, *tracetest.SpanRecorder, *sdktrace.TracerProvider) {
	t.Helper()

	driverName = fmt.Sprintf("%s-%d", driverName, tracingTestDriverID.Add(1))
	sql.Register(driverName, testDriver)
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		provider.Shutdown(context.Background())
	})

	db, err := newConnectionPool(driverName, "", 1, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	return db, recorder, provider
}

var tracingTestDriverID atomic.Uint64

type tracingTestDriver struct {
	blockExec   bool
	execStarted chan struct{}
	rolledBack  chan struct{}
}

func (d *tracingTestDriver) Open(string) (driver.Conn, error) {
	return &tracingTestConn{driver: d}, nil
}

type tracingTestConn struct {
	driver *tracingTestDriver
}

func (c *tracingTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *tracingTestConn) Close() error                        { return nil }
func (c *tracingTestConn) Begin() (driver.Tx, error) {
	return &tracingTestTx{rolledBack: c.driver.rolledBack}, nil
}
func (c *tracingTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}
func (c *tracingTestConn) ExecContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	if c.driver.blockExec {
		close(c.driver.execStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return driver.RowsAffected(1), nil
}
func (c *tracingTestConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &tracingTestRows{}, nil
}

type tracingTestRows struct {
	returned bool
}

func (*tracingTestRows) Columns() []string { return []string{"value"} }
func (*tracingTestRows) Close() error      { return nil }
func (r *tracingTestRows) Next(values []driver.Value) error {
	if r.returned {
		return io.EOF
	}
	r.returned = true
	values[0] = "result"
	return nil
}

type tracingTestTx struct {
	rolledBack   chan struct{}
	rollbackOnce sync.Once
}

func (*tracingTestTx) Commit() error { return nil }
func (tx *tracingTestTx) Rollback() error {
	if tx.rolledBack != nil {
		tx.rollbackOnce.Do(func() { close(tx.rolledBack) })
	}
	return nil
}
