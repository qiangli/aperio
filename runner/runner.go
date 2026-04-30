// Package runner provides the orchestration layer for recording and replaying
// agent executions with full tracing.
//
// This package re-exports types and functions from the internal runner package:
//
//	import "github.com/qiangli/aperio/runner"
//
//	err := runner.Record(ctx, runner.RecordOptions{
//	    Command:    []string{"python", "agent.py"},
//	    OutputPath: "trace.json",
//	})
package runner

import (
	irunner "github.com/qiangli/aperio/internal/runner"
)

// Core types.
type (
	// RecordOptions configures a recording session.
	RecordOptions = irunner.RecordOptions

	// ReplayOptions configures a replay session.
	ReplayOptions = irunner.ReplayOptions

	// Language represents a supported agent programming language.
	Language = irunner.Language

	// RuntimeInfo holds detected runtime information for a binary.
	RuntimeInfo = irunner.RuntimeInfo
)

// Language constants.
const (
	LangPython  = irunner.LangPython
	LangGo      = irunner.LangGo
	LangNode    = irunner.LangNode
	LangUnknown = irunner.LangUnknown
)

// Functions.
var (
	// Record runs an agent with full tracing and saves the execution trace.
	Record = irunner.Record

	// Replay runs an agent with mocked LLM responses from a recorded trace.
	Replay = irunner.Replay

	// DetectLanguage determines the agent's language from command arguments.
	DetectLanguage = irunner.DetectLanguage

	// DetectRuntime inspects a binary to detect its runtime.
	DetectRuntime = irunner.DetectRuntime
)
