package llm

// HTTPResponse is diagnostic metadata from the last transport attempt only.
// Providers supply only safe diagnostic response headers, never request headers
// or credentials. It is not semantic authority or part of any cache identity.
// A final attempt without an HTTP response leaves this nil, as do cache hits.
type HTTPResponse struct {
	StatusCode int                 `json:"status_code"`
	Headers    map[string][]string `json:"headers,omitempty"`
}

func (response *HTTPResponse) Clone() *HTTPResponse {
	if response == nil {
		return nil
	}
	copy := &HTTPResponse{StatusCode: response.StatusCode}
	if response.Headers != nil {
		copy.Headers = make(map[string][]string, len(response.Headers))
		for name, values := range response.Headers {
			copy.Headers[name] = append([]string(nil), values...)
		}
	}
	return copy
}
