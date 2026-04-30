// Package viewer provides an interactive HTTP-based trace visualization server.
//
// This package re-exports types and functions from the internal viewer package:
//
//	import "github.com/qiangli/aperio/viewer"
//
//	url, err := viewer.Serve(viewer.Options{Port: 8080, TraceFile: "trace.json"})
package viewer

import (
	iviewer "github.com/qiangli/aperio/internal/viewer"
)

// Types.
type (
	// Options configures the viewer server.
	Options = iviewer.Options

	// DiffOptions configures the diff viewer server.
	DiffOptions = iviewer.DiffOptions
)

// Functions.
var (
	// Serve starts the viewer HTTP server and returns the URL.
	Serve = iviewer.Serve

	// ServeDiff starts the side-by-side diff viewer and returns the URL.
	ServeDiff = iviewer.ServeDiff
)
