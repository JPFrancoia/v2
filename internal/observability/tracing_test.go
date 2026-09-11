// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestInitTracerRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"", "collector:4318", "ftp://collector:4318"} {
		if _, err := InitTracer(context.Background(), endpoint); err == nil {
			t.Errorf("Expected endpoint %q to be rejected", endpoint)
		}
	}
}

func TestInitTracerExportsSpans(t *testing.T) {
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "")
	t.Setenv("OTEL_SERVICE_NAME", "")

	requestReceived := make(chan struct{}, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/otel/v1/traces" {
			t.Errorf("Expected trace path /otel/v1/traces, got %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Unable to read trace request: %v", err)
		}
		if len(body) == 0 {
			t.Error("Expected a non-empty trace request")
		}
		requestReceived <- struct{}{}
	}))
	defer collector.Close()

	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
	})

	provider, err := InitTracer(context.Background(), collector.URL+"/otel/")
	if err != nil {
		t.Fatalf("Unexpected tracer initialization error: %v", err)
	}

	_, span := otel.Tracer("miniflux-test").Start(context.Background(), "test-span")
	span.End()

	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("Unexpected tracer shutdown error: %v", err)
	}

	select {
	case <-requestReceived:
	default:
		t.Fatal("Expected the collector to receive a trace request")
	}
}
