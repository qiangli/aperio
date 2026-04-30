package export

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/qiangli/aperio/internal/trace"
)

// MetricsCollector accumulates trace metrics and exposes them as Prometheus-compatible
// text format via an HTTP handler. No Prometheus client library is required.
type MetricsCollector struct {
	mu       sync.Mutex
	counters map[string]*counterVec
}

type counterVec struct {
	name   string
	help   string
	values map[string]float64 // key = sorted label pairs string
}

// NewMetricsCollector creates a new metrics collector.
func NewMetricsCollector() *MetricsCollector {
	mc := &MetricsCollector{
		counters: make(map[string]*counterVec),
	}

	// Pre-register all metric names
	mc.counters["aperio_trace_duration_seconds"] = &counterVec{
		name: "aperio_trace_duration_seconds", help: "Total duration of traces in seconds",
		values: make(map[string]float64),
	}
	mc.counters["aperio_llm_call_total"] = &counterVec{
		name: "aperio_llm_call_total", help: "Total number of LLM API calls",
		values: make(map[string]float64),
	}
	mc.counters["aperio_llm_tokens_total"] = &counterVec{
		name: "aperio_llm_tokens_total", help: "Total number of LLM tokens",
		values: make(map[string]float64),
	}
	mc.counters["aperio_tool_call_total"] = &counterVec{
		name: "aperio_tool_call_total", help: "Total number of tool calls",
		values: make(map[string]float64),
	}
	mc.counters["aperio_error_total"] = &counterVec{
		name: "aperio_error_total", help: "Total number of errors",
		values: make(map[string]float64),
	}
	mc.counters["aperio_cost_dollars_total"] = &counterVec{
		name: "aperio_cost_dollars_total", help: "Total cost in US dollars",
		values: make(map[string]float64),
	}

	return mc
}

// RecordTrace extracts metrics from a trace and updates counters.
func (mc *MetricsCollector) RecordTrace(t *trace.Trace) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	agentRole := t.Metadata.AgentRole
	if agentRole == "" {
		agentRole = "default"
	}

	// Trace duration
	var maxDuration float64
	for _, s := range t.Spans {
		if s.Type == trace.SpanProcess {
			dur := s.EndTime.Sub(s.StartTime).Seconds()
			if dur > maxDuration {
				maxDuration = dur
			}
		}
	}
	mc.inc("aperio_trace_duration_seconds", map[string]string{"agent_role": agentRole}, maxDuration)

	// Per-span metrics
	for _, s := range t.Spans {
		switch s.Type {
		case trace.SpanLLMRequest:
			model := s.StringAttr("llm.model")
			if model == "" {
				model = "unknown"
			}
			mc.inc("aperio_llm_call_total", map[string]string{"model": model, "agent_role": agentRole}, 1)

			promptTokens := float64(s.IntAttr("llm.token_count.prompt") + s.IntAttr("llm.token_count.input"))
			completionTokens := float64(s.IntAttr("llm.token_count.completion") + s.IntAttr("llm.token_count.output"))
			if promptTokens > 0 {
				mc.inc("aperio_llm_tokens_total", map[string]string{"model": model, "direction": "input"}, promptTokens)
			}
			if completionTokens > 0 {
				mc.inc("aperio_llm_tokens_total", map[string]string{"model": model, "direction": "output"}, completionTokens)
			}

		case trace.SpanToolCall:
			toolName := s.StringAttr("tool_name")
			if toolName == "" {
				toolName = "unknown"
			}
			mc.inc("aperio_tool_call_total", map[string]string{"tool_name": toolName, "agent_role": agentRole}, 1)

		case trace.SpanExec:
			if s.IntAttr("exit_code") != 0 {
				mc.inc("aperio_error_total", map[string]string{"agent_role": agentRole}, 1)
			}
		}
	}
}

// RecordCost records a cost metric for a model.
func (mc *MetricsCollector) RecordCost(model string, cost float64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.inc("aperio_cost_dollars_total", map[string]string{"model": model}, cost)
}

func (mc *MetricsCollector) inc(name string, labels map[string]string, value float64) {
	cv, ok := mc.counters[name]
	if !ok {
		return
	}
	key := labelsKey(labels)
	cv.values[key] += value
}

func labelsKey(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, labels[k]))
	}
	return strings.Join(parts, ",")
}

// Handler returns an http.Handler that serves the /metrics endpoint
// in Prometheus text exposition format.
func (mc *MetricsCollector) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mc.mu.Lock()
		defer mc.mu.Unlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		// Sort metric names for stable output
		names := make([]string, 0, len(mc.counters))
		for name := range mc.counters {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			cv := mc.counters[name]
			fmt.Fprintf(w, "# HELP %s %s\n", cv.name, cv.help)
			fmt.Fprintf(w, "# TYPE %s counter\n", cv.name)

			// Sort label combinations for stable output
			labelKeys := make([]string, 0, len(cv.values))
			for k := range cv.values {
				labelKeys = append(labelKeys, k)
			}
			sort.Strings(labelKeys)

			for _, lk := range labelKeys {
				v := cv.values[lk]
				if lk != "" {
					fmt.Fprintf(w, "%s{%s} %g\n", cv.name, lk, v)
				} else {
					fmt.Fprintf(w, "%s %g\n", cv.name, v)
				}
			}
		}
	})
}
