package deepseek

import (
	"net/http"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

func httpResponseDiagnostics(response *http.Response) *llm.HTTPResponse {
	if response == nil {
		return nil
	}
	diagnostic := &llm.HTTPResponse{StatusCode: response.StatusCode}
	for name, values := range response.Header {
		if !diagnosticResponseHeader(name) {
			continue
		}
		if diagnostic.Headers == nil {
			diagnostic.Headers = make(map[string][]string)
		}
		key := http.CanonicalHeaderKey(name)
		diagnostic.Headers[key] = append(diagnostic.Headers[key], values...)
	}
	return diagnostic
}

func diagnosticResponseHeader(name string) bool {
	name = strings.ToLower(name)
	switch name {
	case "request-id", "requestid", "x-request-id", "x-correlation-id",
		"traceparent", "tracestate", "x-amzn-trace-id", "x-b3-traceid",
		"cf-ray", "retry-after", "date", "server":
		return true
	}
	return strings.HasPrefix(name, "ratelimit-") || strings.HasPrefix(name, "x-ratelimit-") ||
		strings.HasSuffix(name, "-request-id") || strings.HasSuffix(name, "-trace-id") ||
		strings.HasSuffix(name, "-correlation-id")
}
