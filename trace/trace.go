// Package trace provides the core data model and operations for execution traces.
//
// This package re-exports types and functions from the internal trace package,
// making them available for use by external Go projects:
//
//	import "github.com/qiangli/aperio/trace"
//
//	t, err := trace.ReadTrace("my-trace.json")
//	graph := trace.BuildGraph(t)
package trace

import (
	itrace "github.com/qiangli/aperio/internal/trace"
)

// Core data model types.
type (
	// SpanType represents the kind of span in an execution trace.
	SpanType = itrace.SpanType

	// Span represents a single unit of work in an execution trace.
	Span = itrace.Span

	// Metadata holds contextual information about the trace.
	Metadata = itrace.Metadata

	// Trace represents a complete execution trace of an agent session.
	Trace = itrace.Trace
)

// SpanType constants.
const (
	SpanFunction         = itrace.SpanFunction
	SpanLLMRequest       = itrace.SpanLLMRequest
	SpanLLMResponse      = itrace.SpanLLMResponse
	SpanToolCall         = itrace.SpanToolCall
	SpanToolResult       = itrace.SpanToolResult
	SpanUserInput        = itrace.SpanUserInput
	SpanAgentOutput      = itrace.SpanAgentOutput
	SpanExec             = itrace.SpanExec
	SpanFSRead           = itrace.SpanFSRead
	SpanFSWrite          = itrace.SpanFSWrite
	SpanNetIO            = itrace.SpanNetIO
	SpanDBQuery          = itrace.SpanDBQuery
	SpanProcess          = itrace.SpanProcess
	SpanGoroutine        = itrace.SpanGoroutine
	SpanGC               = itrace.SpanGC
	SpanMemoryRetrieval  = itrace.SpanMemoryRetrieval
	SpanContextInjection = itrace.SpanContextInjection
	SpanVectorSearch     = itrace.SpanVectorSearch
)

// Graph types.
type (
	// TraceGraph is the explicit graph representation of an execution trace.
	TraceGraph = itrace.TraceGraph

	// GraphNode represents a span projected into graph form.
	GraphNode = itrace.GraphNode

	// GraphEdge represents a parent-child relationship in the trace graph.
	GraphEdge = itrace.GraphEdge

	// GraphStats holds summary statistics about the graph.
	GraphStats = itrace.GraphStats
)

// Diff types.
type (
	// Difference represents a divergence between two traces.
	Difference = itrace.Difference
)

// Filter types.
type (
	// FilterConfig controls which function spans to keep.
	FilterConfig = itrace.FilterConfig
)

// Enrich types.
type (
	// ContentRedactor is an interface for redacting string content during enrichment.
	ContentRedactor = itrace.ContentRedactor

	// EnrichOptions configures enrichment behavior.
	EnrichOptions = itrace.EnrichOptions
)

// Classify types.
type (
	// ExecCategory represents the classification of a shell command.
	ExecCategory = itrace.ExecCategory

	// ClassifiedCommand holds the classification result for a single simple command.
	ClassifiedCommand = itrace.ClassifiedCommand
)

// ExecCategory constants.
const (
	ExecCatGit            = itrace.ExecCatGit
	ExecCatFilesystem     = itrace.ExecCatFilesystem
	ExecCatEditor         = itrace.ExecCatEditor
	ExecCatNetwork        = itrace.ExecCatNetwork
	ExecCatPackageManager = itrace.ExecCatPackageManager
	ExecCatBuild          = itrace.ExecCatBuild
	ExecCatRuntime        = itrace.ExecCatRuntime
	ExecCatDocker         = itrace.ExecCatDocker
	ExecCatTest           = itrace.ExecCatTest
	ExecCatLint           = itrace.ExecCatLint
	ExecCatEnv            = itrace.ExecCatEnv
	ExecCatOther          = itrace.ExecCatOther
)

// Stream types.
type (
	// StreamWriter writes spans incrementally as NDJSON.
	StreamWriter = itrace.StreamWriter
)

// I/O functions.
var (
	// ReadTrace reads a trace from the given path.
	// It auto-detects gzip-compressed files (.gz) and encrypted files.
	ReadTrace = itrace.ReadTrace

	// WriteTrace writes a trace to the given path atomically.
	// If the path ends in .gz, the trace is gzip-compressed.
	WriteTrace = itrace.WriteTrace

	// ReadTraceEncrypted reads an AES-256-GCM encrypted trace file.
	ReadTraceEncrypted = itrace.ReadTraceEncrypted

	// WriteTraceEncrypted writes a trace encrypted with AES-256-GCM.
	WriteTraceEncrypted = itrace.WriteTraceEncrypted
)

// Graph functions.
var (
	// BuildGraph converts a flat Trace into an explicit TraceGraph.
	BuildGraph = itrace.BuildGraph

	// ReadGraph reads a graph from a JSON file.
	ReadGraph = itrace.ReadGraph

	// WriteGraph writes a graph to a JSON file.
	WriteGraph = itrace.WriteGraph
)

// Merge and processing functions.
var (
	// Merge combines function spans with API spans into a unified trace.
	Merge = itrace.Merge

	// MergeTraces combines multiple correlated traces into a single DAG.
	MergeTraces = itrace.MergeTraces

	// Enrich processes raw LLM_REQUEST spans and extracts semantic spans.
	Enrich = itrace.Enrich

	// Filter removes function spans that don't match the filter criteria.
	Filter = itrace.Filter

	// ClassifyAndAnnotate classifies exec commands and annotates spans.
	ClassifyAndAnnotate = itrace.ClassifyAndAnnotate

	// ClassifyExecCommand classifies a shell command string.
	ClassifyExecCommand = itrace.ClassifyExecCommand

	// ParseAndClassify parses a compound shell command and classifies each part.
	ParseAndClassify = itrace.ParseAndClassify
)

// Diff functions.
var (
	// Diff compares two traces and returns the differences.
	Diff = itrace.Diff

	// FormatDiff formats differences for display.
	FormatDiff = itrace.FormatDiff
)

// Stream functions.
var (
	// NewStreamWriter creates a new NDJSON stream writer.
	NewStreamWriter = itrace.NewStreamWriter

	// ReadStreamTrace reads an NDJSON stream trace file.
	ReadStreamTrace = itrace.ReadStreamTrace
)
