// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package observability // import "miniflux.app/v2/internal/observability"

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"

	"miniflux.app/v2/internal/version"
)

// InitTracer configures the global OpenTelemetry trace provider.
func InitTracer(ctx context.Context, endpoint string) (*sdktrace.TracerProvider, error) {
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf(`observability: invalid OTEL endpoint: %w`, err)
	}
	if endpointURL.Host == "" || (endpointURL.Scheme != "http" && endpointURL.Scheme != "https") {
		return nil, fmt.Errorf(`observability: invalid OTEL endpoint %q`, endpoint)
	}
	endpointURL.Path = strings.TrimRight(endpointURL.Path, "/") + "/v1/traces"
	endpointURL.RawPath = ""

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("miniflux"),
			semconv.ServiceVersion(version.Version),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf(`observability: unable to create OTEL resource: %w`, err)
	}

	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(endpointURL.String()))
	if err != nil {
		return nil, fmt.Errorf(`observability: unable to create OTEL exporter: %w`, err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	otel.SetTracerProvider(provider)

	return provider, nil
}
