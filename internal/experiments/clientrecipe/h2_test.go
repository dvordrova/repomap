package clientrecipe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestH2SynthesisIsCopyOnlyAndRequestBound(t *testing.T) {
	h1 := goldenH1(t)
	before, err := EncodeH1(h1)
	if err != nil {
		t.Fatal(err)
	}
	provider := &recordingH2Provider{}
	result, err := BuildH2(t.Context(), h1, provider)
	if err != nil {
		t.Fatal(err)
	}
	if provider.calls != 1 {
		t.Fatalf("provider calls = %d, want 1", provider.calls)
	}
	if !strings.Contains(provider.prompt, "Copy does not decide order, membership, completeness") {
		t.Fatal("embedded H2 prompt lost its non-authority instruction")
	}
	var request H2ProviderRequest
	if err := json.Unmarshal(provider.request, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Steps) != 6 || len(request.Examples) != 4 {
		t.Fatalf("H2 request catalog = %d steps / %d examples", len(request.Steps), len(request.Examples))
	}
	for _, forbidden := range []string{
		`internal/`, `.go`, `"authority_id"`, `"h1_sha256"`, `"h0_sha256"`,
		`program-object-`, `program-relation-`, `h1-instance-`, h1.SHA256, h1.H0SHA256,
	} {
		if bytes.Contains(provider.request, []byte(forbidden)) {
			t.Fatalf("H2 provider request leaked %q", forbidden)
		}
	}
	if err := result.ValidateAgainst(h1); err != nil {
		t.Fatal(err)
	}
	after, err := EncodeH1(h1)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("H2 mutated structural H1 authority")
	}
	raw, err := EncodeH2(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"roles"`, `"evidence"`, `"complete"`, `"missing"`, `"best"`, `"path"`} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("H2 result acquired structural field %s", forbidden)
		}
	}
	decoded, err := DecodeH2(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, result) {
		t.Fatal("H2 canonical round trip changed result")
	}
	assertExperimentGolden(t, "06-h2-enrichment.json", raw)
}

func TestH2RejectsIncompleteOrUntrustedCopy(t *testing.T) {
	h1 := goldenH1(t)
	tests := []struct {
		name   string
		mutate func(*h2ProviderResponse)
	}{
		{name: "wrong digest", mutate: func(value *h2ProviderResponse) { value.RequestDigest = strings.Repeat("0", 64) }},
		{name: "missing step", mutate: func(value *h2ProviderResponse) { value.Steps = value.Steps[:5] }},
		{name: "duplicate step", mutate: func(value *h2ProviderResponse) { value.Steps[5].Ref = value.Steps[0].Ref }},
		{name: "unknown example", mutate: func(value *h2ProviderResponse) { value.Examples[3].Ref = "e9" }},
		{name: "html", mutate: func(value *h2ProviderResponse) { value.Steps[0].Title = "<b>Configure</b>" }},
		{name: "markdown", mutate: func(value *h2ProviderResponse) { value.Steps[0].Title = "**Configure**" }},
		{name: "url", mutate: func(value *h2ProviderResponse) { value.Examples[0].Summary = "See https://example.com" }},
		{name: "source path", mutate: func(value *h2ProviderResponse) { value.Steps[0].Purpose = "Read internal/client.go" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &recordingH2Provider{mutate: test.mutate}
			if _, err := BuildH2(t.Context(), h1, provider); err == nil {
				t.Fatal("untrusted H2 response was accepted")
			}
		})
	}

	errorProvider := SynthesisProviderFunc(func(context.Context, string, []byte) ([]byte, error) {
		return nil, errors.New("provider unavailable")
	})
	if _, err := BuildH2(t.Context(), h1, errorProvider); err == nil {
		t.Fatal("provider failure became a partial H2 result")
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := BuildH2(canceled, h1, &recordingH2Provider{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled H2 error = %v", err)
	}
	if _, err := BuildH2(t.Context(), h1, nil); err == nil {
		t.Fatal("nil provider silently selected a fallback")
	}
}

func TestH2RejectsTamperedBindings(t *testing.T) {
	h1 := goldenH1(t)
	result, err := BuildH2(t.Context(), h1, &recordingH2Provider{})
	if err != nil {
		t.Fatal(err)
	}
	mutated := result
	mutated.H1SHA256 = strings.Repeat("a", 64)
	mutated.SHA256 = h2Digest(mutated)
	if err := mutated.ValidateAgainst(h1); err == nil {
		t.Fatal("H2 accepted another H1 binding")
	}
	mutated = result
	mutated.RequestDigest = strings.Repeat("b", 64)
	mutated.SHA256 = h2Digest(mutated)
	if err := mutated.ValidateAgainst(h1); err == nil {
		t.Fatal("H2 accepted another request binding")
	}
}

type recordingH2Provider struct {
	calls   int
	prompt  string
	request []byte
	mutate  func(*h2ProviderResponse)
}

func (provider *recordingH2Provider) Synthesize(_ context.Context, prompt string, requestRaw []byte) ([]byte, error) {
	provider.calls++
	provider.prompt = prompt
	provider.request = append([]byte(nil), requestRaw...)
	var request H2ProviderRequest
	if err := json.Unmarshal(requestRaw, &request); err != nil {
		return nil, err
	}
	response := h2ProviderResponse{
		Version: H2Version, RequestDigest: request.RequestDigest,
		Steps: []H2StepCopy{
			{Ref: "s1", Title: "Shape client configuration", Purpose: "Translate application settings into a focused client configuration before construction."},
			{Ref: "s2", Title: "Build the local boundary", Purpose: "Keep SDK construction behind a repository-owned wrapper that the application can depend on."},
			{Ref: "s3", Title: "Define the consumer contract", Purpose: "Expose the smallest application-facing interface and call it from the real production operation."},
			{Ref: "s4", Title: "Wire the live service graph", Purpose: "Construct the client at startup and pass the local boundary into the service that uses it."},
			{Ref: "s5", Title: "Prove the integration", Purpose: "Follow the repository's existing unit or integration verification shape for observable behavior."},
			{Ref: "s6", Title: "Observe and contain failure", Purpose: "Record client attempts and failures, then apply the repository's common timeout or retry policy."},
		},
		Examples: []H2ExampleCopy{
			{Ref: "e1", Summary: "Complete integration-tested boundary with configuration, wiring, operations, and failure visibility."},
			{Ref: "e2", Summary: "Complete unit-tested boundary with an application interface, retries, metrics, and failure logging."},
			{Ref: "e3", Summary: "Production-reachable boundary that still lacks verification, observability, and a failure policy."},
			{Ref: "e4", Summary: "Complete unit-tested boundary with explicit configuration, timeout-aware retries, metrics, and logging."},
		},
	}
	if provider.mutate != nil {
		provider.mutate(&response)
	}
	return json.Marshal(response)
}

func goldenH1(t *testing.T) H1Result {
	t.Helper()
	value, err := DecodeH1(readExperimentFile(t, filepath.Join(experimentRoot(t), "golden", "03-h1-structural.json")))
	if err != nil {
		t.Fatal(err)
	}
	return value
}
