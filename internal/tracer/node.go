package tracer

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qiangli/aperio/internal/trace"
)

//go:embed scripts/node_tracer.js
var nodeTracerScript embed.FS

// NodeTracer instruments Node.js/TypeScript agents via --require injection.
type NodeTracer struct {
	tmpDir     string
	binaryMode bool // when true, use NODE_OPTIONS instead of rewriting args
}

func NewNodeTracer() *NodeTracer {
	return &NodeTracer{}
}

// SetBinaryMode enables binary agent mode, which uses NODE_OPTIONS env var
// instead of rewriting command args. This works for installed binaries
// (e.g., claude, copilot) that have shebangs or are shell wrappers.
func (nt *NodeTracer) SetBinaryMode(enabled bool) {
	nt.binaryMode = enabled
}

func (nt *NodeTracer) Setup(ctx context.Context, cfg Config, args []string) ([]string, []string, error) {
	tmpDir, err := os.MkdirTemp("", "aperio-node-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp dir: %w", err)
	}
	nt.tmpDir = tmpDir

	// Write the tracer script
	tracerContent, err := nodeTracerScript.ReadFile("scripts/node_tracer.js")
	if err != nil {
		return nil, nil, fmt.Errorf("read embedded tracer: %w", err)
	}

	tracerPath := filepath.Join(tmpDir, "aperio_node_tracer.js")
	if err := os.WriteFile(tracerPath, tracerContent, 0644); err != nil {
		return nil, nil, fmt.Errorf("write tracer script: %w", err)
	}

	// Build environment variables
	env := []string{
		fmt.Sprintf("APERIO_TRACE_OUTPUT=%s", cfg.TraceOutputPath),
		fmt.Sprintf("APERIO_TRACE_DIR=%s", cfg.WorkingDir),
	}

	if len(cfg.ExcludePatterns) > 0 {
		env = append(env, fmt.Sprintf("APERIO_EXCLUDE=%s", strings.Join(cfg.ExcludePatterns, ",")))
	}

	var newArgs []string
	if nt.binaryMode {
		// Binary mode: use NODE_OPTIONS to inject --require without changing args.
		// This works for installed binaries with shebangs (#!/usr/bin/env node)
		// and shell wrappers that eventually exec node.
		existingOpts := os.Getenv("NODE_OPTIONS")
		nodeOpts := fmt.Sprintf("--require %s", tracerPath)
		if existingOpts != "" {
			nodeOpts = existingOpts + " " + nodeOpts
		}
		env = append(env, fmt.Sprintf("NODE_OPTIONS=%s", nodeOpts))
		newArgs = args // keep original args unchanged
	} else {
		// Source mode: inject --require directly into the command args
		newArgs = injectRequire(args, tracerPath)
	}

	return env, newArgs, nil
}

// injectRequire adds --require <tracer> to Node.js-like commands.
func injectRequire(args []string, tracerPath string) []string {
	if len(args) == 0 {
		return args
	}

	exe := filepath.Base(args[0])

	switch {
	case exe == "node" || exe == "nodejs":
		// node --require /path/to/tracer.js <script> [args...]
		newArgs := []string{args[0], "--require", tracerPath}
		newArgs = append(newArgs, args[1:]...)
		return newArgs

	case exe == "npx":
		// npx uses node under the hood; inject via NODE_OPTIONS
		// (handled via env var instead)
		return args

	case exe == "tsx" || exe == "ts-node":
		// tsx/ts-node support --require
		newArgs := []string{args[0], "--require", tracerPath}
		newArgs = append(newArgs, args[1:]...)
		return newArgs

	case exe == "bun":
		// bun doesn't support --require; use preload
		newArgs := []string{args[0], "--preload", tracerPath}
		newArgs = append(newArgs, args[1:]...)
		return newArgs

	default:
		return args
	}
}

// nodeSpan is the JSON structure written by node_tracer.js.
type nodeSpan struct {
	ID         string         `json:"id"`
	ParentID   string         `json:"parent_id"`
	Type       string         `json:"type"`
	Name       string         `json:"name"`
	StartTime  string         `json:"start_time"`
	EndTime    string         `json:"end_time"`
	Attributes map[string]any `json:"attributes"`
}

func (nt *NodeTracer) Collect(cfg Config) ([]*trace.Span, error) {
	data, err := os.ReadFile(cfg.TraceOutputPath)
	if err != nil {
		return nil, fmt.Errorf("read node trace output: %w", err)
	}

	var nodeSpans []nodeSpan
	if err := json.Unmarshal(data, &nodeSpans); err != nil {
		return nil, fmt.Errorf("unmarshal node trace: %w", err)
	}

	spans := make([]*trace.Span, 0, len(nodeSpans))
	for _, ns := range nodeSpans {
		startTime, _ := time.Parse(time.RFC3339Nano, ns.StartTime)
		endTime, _ := time.Parse(time.RFC3339Nano, ns.EndTime)

		spans = append(spans, &trace.Span{
			ID:         ns.ID,
			ParentID:   ns.ParentID,
			Type:       trace.SpanType(ns.Type),
			Name:       ns.Name,
			StartTime:  startTime,
			EndTime:    endTime,
			Attributes: ns.Attributes,
		})
	}

	return spans, nil
}

func (nt *NodeTracer) Cleanup() error {
	if nt.tmpDir != "" {
		return os.RemoveAll(nt.tmpDir)
	}
	return nil
}
