package telemetry

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// InitTracer wires the global tracer provider. For MVP this returns the global
// no-op-or-configured tracer; OTLP export is configured via env in deploy.
func InitTracer(service string) trace.Tracer {
	return otel.Tracer(service)
}
