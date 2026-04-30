// Package proxy provides the MITM HTTPS proxy for recording and replaying LLM API calls.
//
// This package re-exports types and functions from the internal proxy package:
//
//	import "github.com/qiangli/aperio/proxy"
//
//	p, err := proxy.New(proxy.Options{Mode: proxy.ModeRecord})
package proxy

import (
	iproxy "github.com/qiangli/aperio/internal/proxy"
)

// Mode constants.
type Mode = iproxy.Mode

const (
	ModeRecord = iproxy.ModeRecord
	ModeReplay = iproxy.ModeReplay
)

// Core types.
type (
	// Proxy wraps a goproxy server with recording/replay capabilities.
	Proxy = iproxy.Proxy

	// Options configures the proxy.
	Options = iproxy.Options
)

// Redaction types.
type (
	// Redactor applies redaction rules to strings, JSON bodies, and headers.
	Redactor = iproxy.Redactor

	// RedactionRule defines a single redaction pattern.
	RedactionRule = iproxy.RedactionRule

	// RedactionConfig defines the complete redaction configuration.
	RedactionConfig = iproxy.RedactionConfig
)

// Constructor functions.
var (
	// New creates a new proxy instance.
	New = iproxy.New

	// NewRedactor creates a redactor from a configuration.
	NewRedactor = iproxy.NewRedactor

	// LoadRedactionConfig loads redaction config from a YAML file.
	LoadRedactionConfig = iproxy.LoadRedactionConfig
)
