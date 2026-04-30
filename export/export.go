// Package export provides OTLP JSON export and Prometheus metrics for traces.
//
// This package re-exports types and functions from the internal export package:
//
//	import "github.com/qiangli/aperio/export"
//
//	otlp := export.ConvertTrace(t, "my-service")
//	err := export.Send(otlp, export.SendOptions{Endpoint: "http://localhost:4318/v1/traces"})
package export

import (
	iexport "github.com/qiangli/aperio/internal/export"
)

// OTLP types.
type (
	// OTLPExportRequest is the top-level OTLP JSON structure.
	OTLPExportRequest = iexport.OTLPExportRequest

	// ResourceSpans groups spans by resource.
	ResourceSpans = iexport.ResourceSpans

	// Resource describes the entity producing spans.
	Resource = iexport.Resource

	// ScopeSpans groups spans by instrumentation scope.
	ScopeSpans = iexport.ScopeSpans

	// InstrumentationScope identifies the instrumentation library.
	InstrumentationScope = iexport.InstrumentationScope

	// OTLPSpan is the OTLP JSON representation of a span.
	OTLPSpan = iexport.OTLPSpan

	// SpanStatus represents the status of a span.
	SpanStatus = iexport.SpanStatus

	// SpanEvent represents a named event within a span's timeline.
	SpanEvent = iexport.SpanEvent

	// SpanLink connects a span to another span in a different (or same) trace.
	SpanLink = iexport.SpanLink

	// KeyValue is an OTLP attribute key-value pair.
	KeyValue = iexport.KeyValue

	// AnyValue represents an OTLP attribute value.
	AnyValue = iexport.AnyValue
)

// Sender types.
type (
	// SendOptions configures the OTLP HTTP sender.
	SendOptions = iexport.SendOptions

	// SendError provides structured error information from OTLP send failures.
	SendError = iexport.SendError
)

// Batcher types.
type (
	// Batcher accumulates OTLP spans and flushes them in batches.
	Batcher = iexport.Batcher

	// BatcherConfig configures the batcher.
	BatcherConfig = iexport.BatcherConfig
)

// Metrics types.
type (
	// MetricsCollector accumulates trace metrics and exposes them as Prometheus text format.
	MetricsCollector = iexport.MetricsCollector
)

// OTLP SpanKind constants.
const (
	SpanKindUnspecified = iexport.SpanKindUnspecified
	SpanKindInternal    = iexport.SpanKindInternal
	SpanKindServer      = iexport.SpanKindServer
	SpanKindClient      = iexport.SpanKindClient
	SpanKindProducer    = iexport.SpanKindProducer
	SpanKindConsumer    = iexport.SpanKindConsumer
)

// OTLP status code constants.
const (
	StatusCodeUnset = iexport.StatusCodeUnset
	StatusCodeOK    = iexport.StatusCodeOK
	StatusCodeError = iexport.StatusCodeError
)

// Functions.
var (
	// ConvertTrace converts an Aperio Trace to an OTLP export request.
	ConvertTrace = iexport.ConvertTrace

	// FormatJSON serializes an OTLP export request to JSON.
	FormatJSON = iexport.FormatJSON

	// Send posts an OTLP export request to the configured endpoint.
	Send = iexport.Send

	// NewBatcher creates a new span batcher.
	NewBatcher = iexport.NewBatcher

	// NewMetricsCollector creates a new Prometheus metrics collector.
	NewMetricsCollector = iexport.NewMetricsCollector

	// ValidateTraceID checks if a string is a valid OTLP trace ID.
	ValidateTraceID = iexport.ValidateTraceID

	// ValidateSpanID checks if a string is a valid OTLP span ID.
	ValidateSpanID = iexport.ValidateSpanID
)
