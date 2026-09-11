package run

import (
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestAccessRefusalStopsTheRunThroughTheConsoleClock(t *testing.T) {
	root := newRunOutput(nil)
	var cause error
	root.abort = func(err error) { cause = err }
	child := newRunOutput(nil)
	child.consoleClock = root
	for _, status := range []int{400, 429, 500} {
		if _, refused := accessRefusal(debugdump.SemanticFailureReceipt{Stage: "atlas_symbols", HTTPResponse: &llm.HTTPResponse{StatusCode: status}}); refused {
			t.Fatalf("HTTP %d is not an access or balance refusal", status)
		}
	}
	if _, refused := accessRefusal(debugdump.SemanticFailureReceipt{Stage: "atlas_symbols"}); refused {
		t.Fatal("a failure without an HTTP response is not an access refusal")
	}
	balance, refused := accessRefusal(debugdump.SemanticFailureReceipt{Stage: "report_translation", HTTPResponse: &llm.HTTPResponse{StatusCode: 402}})
	if !refused || !strings.Contains(balance.Error(), "HTTP 402") || !strings.Contains(balance.Error(), "report_translation") {
		t.Fatalf("balance refusal not named: %v", balance)
	}
	if !child.abortRun(balance) || cause == nil || cause != balance {
		t.Fatalf("child console did not stop the run through its clock: %v", cause)
	}
	credentials, refused := accessRefusal(debugdump.SemanticFailureReceipt{Stage: "orientation", HTTPResponse: &llm.HTTPResponse{StatusCode: 401}})
	if !refused || !strings.Contains(credentials.Error(), "credentials") {
		t.Fatalf("credential refusal not named: %v", credentials)
	}
	if newRunOutput(nil).abortRun(credentials) {
		t.Fatal("a console without a root abort claimed to stop the run")
	}
}
