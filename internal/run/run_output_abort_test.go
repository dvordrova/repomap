package run

import (
	"bytes"
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

// The executor's hook stops the run from the executing call, names the
// provider and the status, and announces the stop once: the observer's later
// notice for the same refusal changes nothing.
func TestAnExecutorsAccessRefusalStopsTheRunOnce(t *testing.T) {
	var console bytes.Buffer
	root := newRunOutput(&console)
	var causes []error
	root.abort = func(err error) { causes = append(causes, err) }
	executor := root.stopOnAccessRefusal(llm.Executor{})
	if executor.AccessRefused == nil {
		t.Fatal("the executor has no access refusal hook")
	}
	executor.AccessRefused(llm.ProviderFailure{HTTPStatus: 402}, &llm.HTTPResponse{StatusCode: 402, Provider: "DeepSeek"})
	if len(causes) != 1 || !strings.Contains(causes[0].Error(), "DeepSeek refused for balance: HTTP 402;") {
		t.Fatalf("causes %v", causes)
	}
	later, _ := accessRefusal(debugdump.SemanticFailureReceipt{Stage: "atlas_symbols", HTTPResponse: &llm.HTTPResponse{StatusCode: 402}})
	if root.abortRun(later) || len(causes) != 1 {
		t.Fatalf("a second refusal stopped the run again: %v", causes)
	}
	if strings.Count(console.String(), "Stopping the run") != 1 {
		t.Fatalf("the stop was announced %d times:\n%s", strings.Count(console.String(), "Stopping the run"), console.String())
	}
	if nilSafe := (*runOutput)(nil).stopOnAccessRefusal(llm.Executor{}); nilSafe.AccessRefused != nil {
		t.Fatal("a nil console installed a hook")
	}
}
