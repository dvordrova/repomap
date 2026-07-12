package componentmap

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildSynthesisRequestIsBoundedAndPresentationNeutral(t *testing.T) {
	t.Parallel()

	firstBundle := landscapeTestBundle()
	secondBundle := landscapeTestBundle()
	for left, right := 0, len(secondBundle.Candidates)-1; left < right; left, right = left+1, right-1 {
		secondBundle.Candidates[left], secondBundle.Candidates[right] = secondBundle.Candidates[right], secondBundle.Candidates[left]
	}

	request, firstJSON, err := BuildSynthesisRequest(firstBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest(first) error = %v", err)
	}
	_, secondJSON, err := BuildSynthesisRequest(secondBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisRequest(second) error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatal("synthesis request depends on top-level candidate order")
	}
	if request.Version != SynthesisRequestVersion || request.ContractVersion != ContractVersion || request.PromptVersion != SynthesisPromptVersion {
		t.Fatalf("request versions = %#v", request)
	}
	if len(firstJSON) > maxSynthesisRequestBytes {
		t.Fatalf("request bytes = %d, limit %d", len(firstJSON), maxSynthesisRequestBytes)
	}
	encoded := string(firstJSON)
	for _, forbidden := range []string{"file_tree", "coordinates", "styles", "command package", "repository package"} {
		if strings.Contains(encoded, forbidden) {
			t.Errorf("provider request leaked forbidden presentation/raw field %q: %s", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, "cmd-imports-repo") || !strings.Contains(encoded, "flow_anchor_bindings") {
		t.Fatalf("request omitted exact local relations/bindings: %s", encoded)
	}
	prompt, err := BuildSynthesisPrompt(firstBundle)
	if err != nil {
		t.Fatalf("BuildSynthesisPrompt() error = %v", err)
	}
	if prompt.Version != SynthesisPromptVersion || !strings.Contains(prompt.User, encoded) {
		t.Fatalf("prompt is not bound to the versioned request: %#v", prompt)
	}
	for _, required := range []string{"opaque", "member_ids", "Do not return edges", "coordinates", "provenance"} {
		if !strings.Contains(prompt.System, required) {
			t.Errorf("synthesis instruction misses %q", required)
		}
	}

	firstKey, err := SynthesisCacheKey("revision-a", firstBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(first) error = %v", err)
	}
	secondKey, err := SynthesisCacheKey("revision-a", secondBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(second) error = %v", err)
	}
	changedKey, err := SynthesisCacheKey("revision-b", firstBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(changed) error = %v", err)
	}
	if firstKey != secondKey || firstKey == changedKey {
		t.Fatalf("cache keys = %q, %q, changed %q", firstKey, secondKey, changedKey)
	}
	changedBundle := landscapeTestBundle()
	changedBundle.Candidates[0].Facts[0].Value += " changed"
	changedRequestKey, err := SynthesisCacheKey("revision-a", changedBundle)
	if err != nil {
		t.Fatalf("SynthesisCacheKey(changed request) error = %v", err)
	}
	if changedRequestKey == firstKey {
		t.Fatalf("cache key %q did not bind the exact synthesis request", firstKey)
	}
}

func TestBuildSynthesisRequestRejectsObviousCredentialWithoutEcho(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	secret := "company-secret-value-12345"
	bundle.Candidates[0].Facts[0].Value = `api_key="` + secret + `"`
	_, _, err := BuildSynthesisRequest(bundle)
	if err == nil {
		t.Fatal("BuildSynthesisRequest() accepted an obvious credential")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("BuildSynthesisRequest() error echoed the credential-like value")
	}
}

func TestRecordSynthesisResponseReplaysDeterministically(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	response := validSynthesisProposalJSON(t, bundle)
	result, err := RecordSynthesisResponse(
		bundle,
		"revision-a",
		"deepseek-compatible",
		"deepseek-v4-flash",
		1450*time.Millisecond,
		response,
	)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if result.Landscape.Fallback || result.Record.Call == nil {
		t.Fatalf("result = %#v, want one successful represented call", result)
	}
	metadata := result.Record.Call.Metadata
	if metadata.PromptVersion != SynthesisPromptVersion ||
		metadata.Profile != "deepseek-compatible" ||
		metadata.Model != "deepseek-v4-flash" ||
		metadata.InputBytes <= 0 || metadata.LatencyMillis != 1450 ||
		len(metadata.ValidationWarnings) != 0 || metadata.FallbackReason != "" {
		t.Fatalf("metadata = %#v", metadata)
	}

	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatalf("json.Marshal(record) error = %v", err)
	}
	if !bytes.Contains(saved, []byte(`"call"`)) || bytes.Contains(saved, []byte(`"calls"`)) {
		t.Fatalf("record does not represent a singular call: %s", saved)
	}
	replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
	if err != nil {
		t.Fatalf("ReplaySynthesis() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatalf("replayed landscape differs:\nrecorded: %#v\nreplayed: %#v", result.Landscape, replayed)
	}
}

func TestSynthesisResponseExtractionKeepsWeakModelFormattingRecoverable(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	tests := []struct {
		name     string
		response []byte
		warning  string
	}{
		{
			name:     "markdown fence",
			response: []byte("Here is the map:\n```json\n" + string(proposal) + "\n```\n"),
			warning:  "response.fenced_json_extracted",
		},
		{
			name:     "surrounding prose",
			response: []byte("The bounded proposal follows.\n" + string(proposal) + "\nEnd of proposal."),
			warning:  "response.embedded_json_extracted",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, test.response)
			if err != nil {
				t.Fatalf("RecordSynthesisResponse() error = %v", err)
			}
			if result.Landscape.Fallback {
				t.Fatalf("formatted valid response used fallback: %#v", result.Landscape.Diagnostics)
			}
			if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, test.warning) {
				t.Fatalf("diagnostics = %#v, want %q", result.Landscape.Diagnostics, test.warning)
			}
			if !reflect.DeepEqual(result.Record.Call.Metadata.ValidationWarnings, result.Landscape.Diagnostics) {
				t.Fatalf("saved warnings do not match local validation: %#v", result.Record.Call.Metadata)
			}
			saved, err := json.Marshal(result.Record)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
			if err != nil {
				t.Fatalf("ReplaySynthesis() error = %v", err)
			}
			if !reflect.DeepEqual(replayed, result.Landscape) {
				t.Fatal("formatted response did not replay deterministically")
			}
		})
	}
}

func TestInvalidSynthesisOutputFallsBackAndReplays(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	tests := []struct {
		name       string
		response   []byte
		diagnostic string
		state      ResponseState
	}{
		{name: "junk", response: []byte("not json at all"), diagnostic: "response.no_json", state: ResponseCaptured},
		{name: "ambiguous objects", response: append(append(append([]byte(nil), proposal...), '\n'), proposal...), diagnostic: "response.ambiguous_json", state: ResponseCaptured},
		{name: "invalid proposal type", response: []byte(`{"version":2,"subsystems":"not-an-array"}`), diagnostic: "response.invalid_proposal", state: ResponseCaptured},
		{name: "oversize", response: bytes.Repeat([]byte("x"), maxSynthesisResponseBytes+1), diagnostic: "response.too_large", state: ResponseOversize},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, test.response)
			if err != nil {
				t.Fatalf("RecordSynthesisResponse() error = %v", err)
			}
			if !result.Landscape.Fallback || result.Landscape.FallbackReason != FallbackProposalInvalid {
				t.Fatalf("fallback = %v (%q), want invalid proposal fallback", result.Landscape.Fallback, result.Landscape.FallbackReason)
			}
			if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, test.diagnostic) {
				t.Fatalf("diagnostics = %#v, want %q", result.Landscape.Diagnostics, test.diagnostic)
			}
			if result.Record.Call.ResponseState != test.state || result.Record.Call.Metadata.FallbackReason != FallbackProposalInvalid {
				t.Fatalf("saved call = %#v", result.Record.Call)
			}
			if test.state == ResponseOversize && (len(result.Record.Call.Response) != 0 || result.Record.Call.ResponseBytes != len(test.response)) {
				t.Fatalf("oversize response was not bounded: %#v", result.Record.Call)
			}
			saved, err := json.Marshal(result.Record)
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
			if err != nil {
				t.Fatalf("ReplaySynthesis() error = %v", err)
			}
			if !reflect.DeepEqual(replayed, result.Landscape) {
				t.Fatal("invalid response fallback did not replay deterministically")
			}
		})
	}
}

func TestUnknownSynthesisResponseFieldsAreIgnoredWithWarning(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	response := append([]byte(nil), proposal[:len(proposal)-1]...)
	response = append(response, []byte(`,"commentary":{"note":"harmless"}}`)...)
	result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, response)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if result.Landscape.Fallback {
		t.Fatalf("harmless unknown field caused fallback: %#v", result.Landscape.Diagnostics)
	}
	if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, "response.unknown_fields_ignored") {
		t.Fatalf("diagnostics = %#v, want unknown-field warning", result.Landscape.Diagnostics)
	}
}

func TestSensitiveSynthesisResponseIsNotSavedOrEchoed(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	secret := "company-secret-value-12345"
	var proposal Proposal
	if err := json.Unmarshal(validSynthesisProposalJSON(t, bundle), &proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Subsystems[0].Description = `api_key="` + secret + `"`
	response, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, response)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if result.Record.Call.ResponseState != ResponseSensitiveOmitted || len(result.Record.Call.Response) != 0 {
		t.Fatalf("sensitive response was retained: %#v", result.Record.Call)
	}
	if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, "response.sensitive_omitted") {
		t.Fatalf("diagnostics = %#v, want sensitive response warning", result.Landscape.Diagnostics)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(saved, []byte(secret)) {
		t.Fatal("saved synthesis record exposed the credential-like value")
	}
	replayed, err := ReplaySynthesis(bundle, "revision-a", saved)
	if err != nil {
		if strings.Contains(err.Error(), secret) {
			t.Fatal("replay error echoed the credential-like value")
		}
		t.Fatalf("ReplaySynthesis() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatal("sensitive response fallback did not replay deterministically")
	}
}

func TestReplaySynthesisRejectsPluralCallHistory(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	result, err := RecordSynthesisResponse(
		bundle, "revision-a", "deepseek-compatible", "deepseek-v4-flash", time.Second,
		validSynthesisProposalJSON(t, bundle),
	)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(saved, &object); err != nil {
		t.Fatal(err)
	}
	object["calls"] = []any{object["call"], object["call"]}
	saved, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaySynthesis(bundle, "revision-a", saved); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ReplaySynthesis() error = %v, want plural call history rejection", err)
	}
}

func validSynthesisProposalJSON(t *testing.T, bundle CandidateBundle) []byte {
	t.Helper()

	memberIDs := make([]MemberID, len(bundle.Candidates))
	for index, candidate := range bundle.Candidates {
		memberIDs[index] = candidate.ID
	}
	encoded, err := json.Marshal(Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Repository",
			Components: []ProposedComponent{{
				Name: "Local architecture", Description: "Conceptual grouping over exact local candidates.", MemberIDs: memberIDs,
			}},
		}},
	})
	if err != nil {
		t.Fatalf("json.Marshal(proposal) error = %v", err)
	}
	return encoded
}
