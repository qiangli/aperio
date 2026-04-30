package export

// attrMapping maps Aperio attribute names to OTEL semantic convention names.
var attrMapping = map[string]string{
	"llm.token_count.prompt":     "gen_ai.usage.input_tokens",
	"llm.token_count.completion": "gen_ai.usage.output_tokens",
	"llm.token_count.total":      "gen_ai.usage.total_tokens",
	"llm.model":                  "gen_ai.request.model",
	"tool_name":                  "gen_ai.tool.name",
	"http.method":                "http.request.method",
	"http.url":                   "url.full",
	"http.status_code":           "http.response.status_code",
	"command":                    "process.command",
	"path":                       "file.path",
	"context_type":               "aperio.context.type",
	"token_estimate":             "aperio.context.token_estimate",
	"similarity_scores":          "aperio.vector.similarity_scores",
	"result_count":               "aperio.retrieval.result_count",
	"query":                      "aperio.vector.query",
	"source":                     "aperio.retrieval.source",
}

// mapAttributeName returns the OTEL semantic convention name for an Aperio attribute,
// or the original name if no mapping exists.
func mapAttributeName(name string) string {
	if mapped, ok := attrMapping[name]; ok {
		return mapped
	}
	return name
}
