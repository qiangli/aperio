// Package benchmark provides a multi-tool benchmarking framework for AI agents.
//
// This package re-exports types and functions from the internal benchmark package:
//
//	import "github.com/qiangli/aperio/benchmark"
//
//	spec, err := benchmark.ParseSpec("bench.yaml")
//	results, err := benchmark.Run(ctx, spec)
//	comparison := benchmark.Compare(results, benchmark.DefaultPricing())
package benchmark

import (
	ibench "github.com/qiangli/aperio/internal/benchmark"
)

// Spec and config types.
type (
	// BenchmarkSpec defines a benchmark specification loaded from YAML.
	BenchmarkSpec = ibench.BenchmarkSpec

	// TaskSpec defines the task to benchmark.
	TaskSpec = ibench.TaskSpec

	// ToolSpec defines a tool to benchmark.
	ToolSpec = ibench.ToolSpec

	// ValidationCheck defines a validation rule.
	ValidationCheck = ibench.ValidationCheck

	// ValidationResult holds the result of a single validation check.
	ValidationResult = ibench.ValidationResult
)

// Result types.
type (
	// BenchmarkResults holds the results of a benchmark run.
	BenchmarkResults = ibench.BenchmarkResults

	// ToolRunResult holds results for one tool across all runs.
	ToolRunResult = ibench.ToolRunResult

	// RunResult holds the result of a single benchmark run.
	RunResult = ibench.RunResult
)

// Comparison types.
type (
	// ComparisonResult holds N-way comparison results.
	ComparisonResult = ibench.ComparisonResult

	// AggregateScore holds an aggregate ranking score.
	AggregateScore = ibench.AggregateScore

	// ToolSummary holds summary metrics for one tool.
	ToolSummary = ibench.ToolSummary
)

// Metrics types.
type (
	// TraceMetrics holds per-trace metrics extracted from a trace.
	TraceMetrics = ibench.TraceMetrics

	// ModelMetrics holds per-model token and cost metrics.
	ModelMetrics = ibench.ModelMetrics
)

// Pricing types.
type (
	// ModelPrice holds per-token pricing for a model.
	ModelPrice = ibench.ModelPrice

	// PricingTable holds model pricing data.
	PricingTable = ibench.PricingTable
)

// Leaderboard types.
type (
	// Leaderboard holds accumulated benchmark entries.
	Leaderboard = ibench.Leaderboard

	// LeaderboardEntry holds one benchmark entry.
	LeaderboardEntry = ibench.LeaderboardEntry

	// LeaderboardToolResult holds tool results within a leaderboard entry.
	LeaderboardToolResult = ibench.LeaderboardToolResult

	// BestScore tracks the best score for a tool.
	BestScore = ibench.BestScore
)

// Docker types.
type (
	// DockerConfig configures Docker-based benchmark execution.
	DockerConfig = ibench.DockerConfig

	// DockerRunner manages Docker containers for benchmarks.
	DockerRunner = ibench.DockerRunner

	// DockerRunOptions configures a single Docker run.
	DockerRunOptions = ibench.DockerRunOptions
)

// Black-box tracing types.
type (
	// BlackBoxOptions configures black-box benchmark recording.
	BlackBoxOptions = ibench.BlackBoxOptions
)

// Regression types.
type (
	// RegressionConfig configures regression detection.
	RegressionConfig = ibench.RegressionConfig

	// RegressionThresholds defines acceptable regression limits.
	RegressionThresholds = ibench.RegressionThresholds

	// RegressionResult holds the regression check result.
	RegressionResult = ibench.RegressionResult

	// RegressionDetail describes a specific regression.
	RegressionDetail = ibench.RegressionDetail

	// Baseline holds a saved benchmark baseline.
	Baseline = ibench.Baseline

	// TrustFilter configures outlier filtering for pass rates.
	TrustFilter = ibench.TrustFilter

	// BaselineHistory holds historical baselines.
	BaselineHistory = ibench.BaselineHistory

	// HistoricalBaseline holds one historical baseline entry.
	HistoricalBaseline = ibench.HistoricalBaseline
)

// Adapter types.
type (
	// Adapter is the interface for benchmark adapters (e.g., SWE-bench, Exercism).
	Adapter = ibench.Adapter

	// AdapterFilter configures adapter filtering.
	AdapterFilter = ibench.AdapterFilter

	// AdapterOptions configures adapter behavior.
	AdapterOptions = ibench.AdapterOptions

	// AdapterSpec configures which adapter to use.
	AdapterSpec = ibench.AdapterSpec

	// AdapterConstructor is a factory function for adapters.
	AdapterConstructor = ibench.AdapterConstructor
)

// Core functions.
var (
	// Run executes a benchmark.
	Run = ibench.Run

	// ParseSpec loads a benchmark specification from YAML.
	ParseSpec = ibench.ParseSpec

	// ParseTimeout parses a duration string.
	ParseTimeout = ibench.ParseTimeout

	// ExpandCommand expands template variables in command arguments.
	ExpandCommand = ibench.ExpandCommand
)

// Comparison functions.
var (
	// Compare does N-way comparison of benchmark results.
	Compare = ibench.Compare

	// CompareWithPassK does N-way comparison with pass@k metrics.
	CompareWithPassK = ibench.CompareWithPassK

	// CompareTraces compares pre-existing traces.
	CompareTraces = ibench.CompareTraces
)

// Metrics functions.
var (
	// Extract extracts metrics from a trace.
	Extract = ibench.Extract

	// AverageMetrics computes average metrics across multiple traces.
	AverageMetrics = ibench.AverageMetrics
)

// Report functions.
var (
	// FormatText formats a comparison result as text.
	FormatText = ibench.FormatText

	// GenerateJSON writes a comparison result as JSON.
	GenerateJSON = ibench.GenerateJSON

	// GenerateCSV writes a comparison result as CSV.
	GenerateCSV = ibench.GenerateCSV

	// GenerateHTML writes a comparison result as an HTML report.
	GenerateHTML = ibench.GenerateHTML
)

// Pricing functions.
var (
	// DefaultPricing returns built-in model pricing.
	DefaultPricing = ibench.DefaultPricing

	// LoadPricing loads pricing from a YAML file.
	LoadPricing = ibench.LoadPricing

	// MergePricing merges two pricing tables.
	MergePricing = ibench.MergePricing
)

// Leaderboard functions.
var (
	// LoadLeaderboard loads a leaderboard from a file.
	LoadLeaderboard = ibench.LoadLeaderboard

	// SaveLeaderboard saves a leaderboard to a file.
	SaveLeaderboard = ibench.SaveLeaderboard

	// ComparisonToEntry converts comparison results to a leaderboard entry.
	ComparisonToEntry = ibench.ComparisonToEntry

	// FormatLeaderboard formats a leaderboard for display.
	FormatLeaderboard = ibench.FormatLeaderboard
)

// Validation functions.
var (
	// RunChecks runs validation checks against benchmark output.
	RunChecks = ibench.RunChecks

	// ComputePassRate computes the pass rate from validation results.
	ComputePassRate = ibench.ComputePassRate
)

// Pass@k functions.
var (
	// ComputePassAtK computes pass@k from n trials with c successes.
	ComputePassAtK = ibench.ComputePassAtK

	// ComputePassKFromRuns computes pass@k from run results.
	ComputePassKFromRuns = ibench.ComputePassKFromRuns

	// ComputeAllPassK computes pass@k for multiple k values.
	ComputeAllPassK = ibench.ComputeAllPassK
)

// Black-box functions.
var (
	// BlackBoxRecord records an agent execution in black-box mode.
	BlackBoxRecord = ibench.BlackBoxRecord
)

// Docker functions.
var (
	// IsDockerAvailable checks if Docker is available.
	IsDockerAvailable = ibench.IsDockerAvailable

	// PullImage pulls a Docker image.
	PullImage = ibench.PullImage

	// FormatDockerCommand formats Docker command args for display.
	FormatDockerCommand = ibench.FormatDockerCommand
)

// Regression functions.
var (
	// CheckRegression checks for regressions against a baseline.
	CheckRegression = ibench.CheckRegression

	// CheckRegressionWithHistory checks regressions with historical context.
	CheckRegressionWithHistory = ibench.CheckRegressionWithHistory

	// SaveBaseline saves a baseline to a file.
	SaveBaseline = ibench.SaveBaseline

	// LoadBaseline loads a baseline from a file.
	LoadBaseline = ibench.LoadBaseline

	// DefaultThresholds returns default regression thresholds.
	DefaultThresholds = ibench.DefaultThresholds

	// ApplyTrustFilter filters outlier pass rates.
	ApplyTrustFilter = ibench.ApplyTrustFilter

	// AggregatePassRates computes aggregate pass rate.
	AggregatePassRates = ibench.AggregatePassRates

	// AppendBaseline appends a baseline to history.
	AppendBaseline = ibench.AppendBaseline

	// ComputeConfidenceInterval computes a confidence interval.
	ComputeConfidenceInterval = ibench.ComputeConfidenceInterval
)

// Adapter registry functions.
var (
	// RegisterAdapter registers a benchmark adapter.
	RegisterAdapter = ibench.RegisterAdapter

	// GetAdapter retrieves a registered adapter.
	GetAdapter = ibench.GetAdapter

	// RegisteredAdapters returns the names of all registered adapters.
	RegisteredAdapters = ibench.RegisteredAdapters
)
