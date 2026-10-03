package llm

import (
	"bytes"
	"net/http"
	"strings"
)

// HTTPResponse is diagnostic metadata from the last transport attempt only.
// Providers supply only safe diagnostic response headers, never request headers
// or credentials. It is not semantic authority or part of any cache identity.
// A final attempt without an HTTP response leaves this nil, as do cache hits.
// Provider names the service that answered when its client says so (Jev), so
// a refused key can be named; it is empty when the client does not.
type HTTPResponse struct {
	StatusCode int                 `json:"status_code"`
	Provider   string              `json:"provider,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
}

func (response *HTTPResponse) Clone() *HTTPResponse {
	if response == nil {
		return nil
	}
	copy := &HTTPResponse{StatusCode: response.StatusCode, Provider: response.Provider}
	if response.Headers != nil {
		copy.Headers = make(map[string][]string, len(response.Headers))
		for name, values := range response.Headers {
			copy.Headers[name] = append([]string(nil), values...)
		}
	}
	return copy
}

// DiagnosticHTTPResponse keeps a response's status and its diagnostic
// headers only: request and trace identifiers, Retry-After, rate-limit
// headers, Date and Server. Cookies, authorization and every other header
// are dropped.
func DiagnosticHTTPResponse(response *http.Response) *HTTPResponse {
	if response == nil {
		return nil
	}
	diagnostic := &HTTPResponse{StatusCode: response.StatusCode}
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

// CredentialMarker stands in a provider's response where it wrote the
// client's own configured credential back.
const CredentialMarker = "[configured API key]"

// WithoutCredential returns raw with every occurrence of the client's own
// configured credential replaced by CredentialMarker, and raw itself when it
// holds none. A provider client applies it to each response body as it is
// read, before the bytes reach an error, a completion, a cache or a journal:
// a server that echoes the key in a refusal must not put it in the run's
// files (review B5). It looks only for that one configured value; it is not
// a scan of the repository or of anything else.
func WithoutCredential(raw []byte, credential string) []byte {
	if credential == "" || !bytes.Contains(raw, []byte(credential)) {
		return raw
	}
	return bytes.ReplaceAll(raw, []byte(credential), []byte(CredentialMarker))
}

// WithoutCredential is the diagnostics with the configured credential
// replaced in every kept header value.
func (response *HTTPResponse) WithoutCredential(credential string) *HTTPResponse {
	if response == nil || credential == "" {
		return response
	}
	for name, values := range response.Headers {
		for i, value := range values {
			response.Headers[name][i] = strings.ReplaceAll(value, credential, CredentialMarker)
		}
	}
	return response
}
