package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

type testResponseAdapter struct {
	*testProvider
	accepted        int
	unwrapped       int
	invalidEnvelope bool
}

func (adapter *testResponseAdapter) AdaptResponse(request, response []byte) (AdaptedResponse, error) {
	adapter.unwrapped++
	if len(request) == 0 {
		return AdaptedResponse{}, errors.New("adapter did not receive the exact request")
	}
	if adapter.invalidEnvelope {
		return AdaptedResponse{}, errors.New("invalid domain envelope")
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		return AdaptedResponse{}, err
	}
	return AdaptedResponse{Domain: envelope.Result, Accept: func([]string) { adapter.accepted++ }}, nil
}

func TestResponseAdjunctKeepsExactBytesAndRevalidatesCache(t *testing.T) {
	provider := &testResponseAdapter{testProvider: baseTestProvider()}
	raw := []byte(`{"result":{"value":"ok"},"adjunct":{"text":"source-backed"}}`)
	provider.responses = [][]byte{raw}
	executor := Executor{RootDir: t.TempDir(), Enabled: true}
	call := baseTestCall("adjunct", "input")
	for i := 0; i < 2; i++ {
		outcome, err := ExecuteJSON(t.Context(), executor, provider, call)
		if err != nil {
			t.Fatal(err)
		}
		if outcome.Value.Value != "ok" || outcome.Cached != (i == 1) || !bytes.Equal(outcome.Response, raw) {
			t.Fatalf("lost result, raw response or cache: %+v", outcome)
		}
	}
	if provider.completeCalls != 1 || provider.accepted != 2 || provider.unwrapped != 2 {
		t.Fatalf("calls=%d accepted=%d decoded=%d", provider.completeCalls, provider.accepted, provider.unwrapped)
	}
	provider.invalidEnvelope = true
	if _, err := ExecuteJSON(t.Context(), executor, provider, call); err == nil {
		t.Fatal("cached envelope bypassed current validation")
	}
	if provider.accepted != 2 {
		t.Fatal("refused metadata was collected")
	}
}

func TestRejectedDomainNeverCollectsAnAcceptedAdjunct(t *testing.T) {
	provider := &testResponseAdapter{testProvider: baseTestProvider()}
	provider.responses = [][]byte{[]byte(`{"result":{"value":""},"adjunct":{}}`)}
	if _, err := ExecuteJSON(t.Context(), Executor{}, provider, baseTestCall("adjunct", "input")); err == nil {
		t.Fatal("domain validation bypassed")
	}
	if provider.accepted != 0 {
		t.Fatal("adjunct from rejected domain became knowledge")
	}
}
