package deepseek

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestHTTPResponseDiagnosticsKeepsOnlyDiagnosticHeaders(t *testing.T) {
	allowed := []string{
		"Request-Id", "Requestid", "X-Request-Id", "X-Correlation-Id",
		"Traceparent", "Tracestate", "X-Amzn-Trace-Id", "X-B3-Traceid", "Cf-Ray",
		"Retry-After", "Date", "Server", "RateLimit-Limit", "X-RateLimit-Remaining-Tokens",
		"X-Proxy-Request-Id", "Vendor-Trace-Id", "Vendor-Correlation-Id",
	}
	response := &http.Response{StatusCode: 500, Header: make(http.Header)}
	want := make(map[string][]string)
	for _, name := range allowed {
		response.Header.Add(name, "first")
		response.Header.Add(name, "second")
		want[http.CanonicalHeaderKey(name)] = []string{"first", "second"}
	}
	for _, name := range []string{
		"Authorization", "Proxy-Authorization", "Cookie", "Set-Cookie", "X-API-Key",
		"Api-Key", "X-Auth-Token", "X-Access-Token", "Content-Type", "Location", "X-Arbitrary",
	} {
		response.Header.Set(name, "secret-or-unrelated")
	}
	got := httpResponseDiagnostics(response)
	if got.StatusCode != 500 || !reflect.DeepEqual(got.Headers, want) {
		t.Fatalf("diagnostic headers differ from allowlist: %#v", got)
	}
	response.Header["X-Request-Id"][0] = "mutated"
	if got.Headers["X-Request-Id"][0] != "first" {
		t.Fatal("diagnostic aliases live response headers")
	}
	if httpResponseDiagnostics(nil) != nil {
		t.Fatal("invented HTTP response for absent transport response")
	}
}

func TestLLMProviderDiagnosticsDescribeOnlyLastHTTPAttempt(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(fmt.Sprintf("success=%v", success), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				attempts := 0
				client := llmProviderHandlerClient(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
					attempts++
					writer.Header().Set("X-Request-Id", fmt.Sprintf("attempt-%d", attempts))
					writer.Header().Set("Vendor-Trace-Id", "trace")
					writer.Header().Set("X-RateLimit-Remaining", "27")
					writer.Header().Set("Authorization", "secret")
					writer.Header().Set("Set-Cookie", "secret")
					if attempts == 1 {
						writer.Header().Set("First-Request-Id", "must-not-survive")
					}
					if success && attempts == 2 {
						_, _ = writer.Write(llmProviderResponse("stop", `{"ok":true}`, nil))
						return
					}
					writer.WriteHeader(http.StatusInternalServerError)
					_, _ = writer.Write([]byte("<html>non-JSON provider failure</html>"))
				}))
				prepared, _ := llm.NewPrepared([]byte(`{"request":true}`))
				completion, err := client.Complete(t.Context(), prepared)
				wantStatus, wantAttempts := 500, maxRetries+1
				if success {
					wantStatus, wantAttempts = 200, 2
				}
				if (err == nil) != success || attempts != wantAttempts || completion.Metrics.Attempts != wantAttempts {
					t.Fatalf("attempts=%d completion=%#v err=%v", attempts, completion, err)
				}
				if completion.HTTPResponse == nil || completion.HTTPResponse.StatusCode != wantStatus {
					t.Fatalf("missing actual HTTP status: %#v", completion.HTTPResponse)
				}
				headers := completion.HTTPResponse.Headers
				if !reflect.DeepEqual(headers["X-Request-Id"], []string{fmt.Sprintf("attempt-%d", wantAttempts)}) ||
					headers["Vendor-Trace-Id"][0] != "trace" || headers["X-Ratelimit-Remaining"][0] != "27" ||
					headers["First-Request-Id"] != nil || headers["Authorization"] != nil || headers["Set-Cookie"] != nil {
					t.Fatalf("earlier attempt or secret headers leaked: %#v", headers)
				}
			})
		})
	}
}

type diagnosticRoundTripper func(*http.Request) (*http.Response, error)

func (transport diagnosticRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestLLMProviderDoesNotCarryOldHTTPResponseIntoNetworkFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := llmProviderHandlerClient(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.Header().Set("X-Request-Id", "old-http-response")
			writer.WriteHeader(http.StatusInternalServerError)
		}))
		baseTransport := client.HTTPClient.Transport
		attempts := 0
		client.HTTPClient.Transport = diagnosticRoundTripper(func(request *http.Request) (*http.Response, error) {
			attempts++
			if attempts == 1 {
				return baseTransport.RoundTrip(request)
			}
			return nil, errors.New("local transport failure")
		})
		prepared, _ := llm.NewPrepared([]byte(`{"request":true}`))
		completion, err := client.Complete(t.Context(), prepared)
		if err == nil || attempts != maxRetries+1 || completion.HTTPResponse != nil {
			t.Fatalf("previous HTTP response attributed to final network failure: %#v / %v", completion, err)
		}
	})
}
