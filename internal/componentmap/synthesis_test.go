package componentmap

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

func TestSavedCaddyArchitectureProposalReplaysWithoutFallback(t *testing.T) {
	t.Parallel()

	bundle, response := caddyArchitectureReplayFixture(t)
	result, err := RecordSynthesisResponse(
		bundle, "caddy-saved-run", "openai-compatible/bearer", "deepseek-v4-flash", 0, response,
	)
	if err != nil {
		t.Fatalf("RecordSynthesisResponse() error = %v", err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome == ValidationRejected {
		t.Fatalf("saved Caddy response selected fallback: %#v", result.Landscape)
	}
	if result.Landscape.Source != SourceValidatedModel || len(result.Landscape.Subsystems) != 6 {
		t.Fatalf("source/subsystems = %q/%d", result.Landscape.Source, len(result.Landscape.Subsystems))
	}
	componentCount := 0
	for _, subsystem := range result.Landscape.Subsystems {
		componentCount += len(subsystem.Components)
		for _, component := range subsystem.Components {
			if !component.Hypothesis && len(component.AnchorIDs) == 0 {
				t.Fatalf("component %q has no supplied anchor", component.Name)
			}
		}
	}
	if componentCount != 12 {
		t.Fatalf("nested components = %d, want 12", componentCount)
	}
	wantSubsystems := []string{"Core", "Config", "Admin", "HTTP", "Security", "Entry"}
	for index, want := range wantSubsystems {
		if result.Landscape.Subsystems[index].Name != want {
			t.Fatalf("subsystem[%d] = %q, want %q", index, result.Landscape.Subsystems[index].Name, want)
		}
	}
	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplaySynthesis(bundle, "caddy-saved-run", saved)
	if err != nil {
		t.Fatalf("ReplaySynthesis() error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatal("saved Caddy response did not replay deterministically")
	}
	legacyRecord, err := json.Marshal(map[string]any{
		"version": 1, "repository_revision": "caddy-saved-run", "cache_key": "legacy", "request_sha256": strings.Repeat("a", 64),
		"call": map[string]any{
			"metadata": map[string]any{
				"prompt_version": "architecture-grounding-v3", "profile": "openai-compatible/bearer",
				"model": "deepseek-v4-flash", "input_bytes": 119270, "latency_ms": 12009,
				"validation_warnings": []map[string]string{{"code": "proposal.excess_primary_pillars", "message": "grounded architecture exceeds eight primary pillars"}},
				"fallback_reason":     "proposal_invalid_or_empty",
			},
			"response_state": "captured", "response_bytes": len(response), "response": response,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyReplayed, err := ReplayLegacyCapturedSynthesis(bundle, legacyRecord)
	if err != nil {
		t.Fatalf("ReplayLegacyCapturedSynthesis() error = %v", err)
	}
	if legacyReplayed.Fallback || len(legacyReplayed.Subsystems) != 6 {
		t.Fatalf("legacy captured response replay = %#v", legacyReplayed)
	}
	var legacyProposal map[string]any
	if err := json.Unmarshal(response, &legacyProposal); err != nil {
		t.Fatal(err)
	}
	legacyProposal["version"] = 2
	legacyResponse, err := json.Marshal(legacyProposal)
	if err != nil {
		t.Fatal(err)
	}
	legacyV2Record, err := json.Marshal(map[string]any{
		"version": 1, "repository_revision": "caddy-saved-run", "cache_key": "legacy", "request_sha256": strings.Repeat("a", 64),
		"call": map[string]any{
			"metadata": map[string]any{
				"prompt_version": "component-landscape-v2", "profile": "openai-compatible/bearer",
				"model": "deepseek-v4-flash", "input_bytes": 45271, "latency_ms": 10964,
			},
			"response_state": "captured", "response_bytes": len(legacyResponse), "response": legacyResponse,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyV2Replayed, err := ReplayLegacyCapturedSynthesis(bundle, legacyV2Record)
	if err != nil {
		t.Fatalf("ReplayLegacyCapturedSynthesis(v2) error = %v", err)
	}
	if legacyV2Replayed.Fallback || len(legacyV2Replayed.Subsystems) != 6 {
		t.Fatalf("legacy v2 captured response replay = %#v", legacyV2Replayed)
	}
}

func TestSavedEtcdSizedArchitectureParityGate(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/etcd_architecture_parity_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Version     int             `json:"version"`
		Bundle      CandidateBundle `json:"bundle"`
		SavedRecord json.RawMessage `json:"saved_record"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Version != 1 {
		t.Fatalf("fixture version = %d", fixture.Version)
	}
	var saved SynthesisRecord
	if err := json.Unmarshal(fixture.SavedRecord, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Call == nil || saved.Call.ResponseState != ResponseCaptured ||
		saved.Call.Metadata.OutputTokens <= 0 || saved.Call.ResponseBytes != len(saved.Call.Response) {
		t.Fatalf("saved etcd call is incomplete: %#v", saved.Call)
	}
	request, requestJSON, err := BuildSynthesisRequest(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := BuildSynthesisPrompt(fixture.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponseForLanguage(
		fixture.Bundle, "etcd-parity-v1", saved.Call.Metadata.Profile,
		saved.Call.Metadata.Model, "en", 0, saved.Call.Response,
	)
	if err != nil {
		t.Fatalf("saved etcd response did not complete against its bounded shape: %v", err)
	}
	if result.Landscape.Fallback ||
		(result.Landscape.ValidationOutcome != ValidationAccepted &&
			result.Landscape.ValidationOutcome != ValidationAcceptedNormalized) {
		t.Fatalf("saved etcd response was not accepted: %#v", result.Landscape)
	}
	wantMembers := make(map[string]Candidate, len(fixture.Bundle.Candidates))
	for _, candidate := range fixture.Bundle.Candidates {
		wantMembers[candidate.ID.key()] = candidate
	}
	gotMembers := make(map[string][]Candidate, len(wantMembers))
	for _, subsystem := range result.Landscape.Subsystems {
		for _, component := range subsystem.Components {
			for _, member := range component.Members {
				gotMembers[member.ID.key()] = append(gotMembers[member.ID.key()], member)
			}
		}
	}
	if len(gotMembers) != len(wantMembers) {
		t.Fatalf("accepted member cardinality = %d, want %d", len(gotMembers), len(wantMembers))
	}
	for member, want := range wantMembers {
		if len(gotMembers[member]) != 1 {
			t.Fatalf("accepted membership %q count = %d, want exactly one", member, len(gotMembers[member]))
		}
		if !reflect.DeepEqual(gotMembers[member][0], want) {
			t.Fatalf("accepted model grouping changed exact local candidate %q", member)
		}
	}
	if !reflect.DeepEqual(result.Landscape.Relations, fixture.Bundle.Relations) {
		t.Fatal("model grouping changed exact local relations")
	}
	if !reflect.DeepEqual(result.Landscape.AnchorBindings, fixture.Bundle.AnchorBindings) {
		t.Fatal("model grouping changed exact local anchor bindings")
	}
	t.Logf(
		"etcd parity: candidates=%d members=%d anchors=%d request_json=%d prompt_bytes=%d output_tokens=%d response_state=%s",
		len(fixture.Bundle.Candidates), len(gotMembers), len(fixture.Bundle.BehaviorAnchors),
		len(requestJSON), synthesisPromptSize(prompt), saved.Call.Metadata.OutputTokens,
		saved.Call.ResponseState,
	)
	if request.Version != SynthesisRequestVersion {
		t.Fatalf("request version = %d", request.Version)
	}
}

func TestRejectedCaddyProposalUsesAnchorFirstFallback(t *testing.T) {
	t.Parallel()

	bundle, response := caddyArchitectureReplayFixture(t)
	var proposal Proposal
	if err := json.Unmarshal(response, &proposal); err != nil {
		t.Fatal(err)
	}
	proposal.Subsystems[0].Components[0].AnchorIDs = []string{"anchor-invented"}
	response, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "caddy-saved-run", "test", "test", 0, response)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || result.Landscape.Source != SourceLocalAnchors || result.Landscape.Level != 3 ||
		result.Landscape.FallbackReason != FallbackRejectedUnknownAnchor {
		t.Fatalf("fallback ladder result = %#v", result.Landscape)
	}
	for _, subsystem := range result.Landscape.Subsystems {
		if subsystem.Name == "Packages" || subsystem.Name == "Files" || subsystem.Name == "Symbols" {
			t.Fatalf("anchor-first fallback exposed raw kind group %q", subsystem.Name)
		}
	}
}

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

	defaultLanguageKey, err := SynthesisCacheKeyForProvider(
		"revision-a", firstBundle, "deepseek-compatible", "deepseek-v4-flash",
	)
	if err != nil {
		t.Fatal(err)
	}
	englishKey, err := SynthesisCacheKeyForProviderAndLanguage(
		"revision-a", firstBundle, "deepseek-compatible", "deepseek-v4-flash", "en",
	)
	if err != nil {
		t.Fatal(err)
	}
	russianKey, err := SynthesisCacheKeyForProviderAndLanguage(
		"revision-a", firstBundle, "deepseek-compatible", "deepseek-v4-flash", "ru",
	)
	if err != nil {
		t.Fatal(err)
	}
	if defaultLanguageKey != englishKey {
		t.Fatalf("default/English cache keys = %q/%q, want path compatibility", defaultLanguageKey, englishKey)
	}
	if russianKey == englishKey {
		t.Fatalf("Russian cache key %q reused English identity", russianKey)
	}
}

func TestBuildSynthesisPromptLanguageKeepsEnglishBytesAndScopesRussianProse(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	canonicalEnglish, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	explicitEnglish, err := BuildSynthesisPromptForLanguage(bundle, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(canonicalEnglish, explicitEnglish) {
		t.Fatalf("explicit English prompt changed canonical bytes:\ncanonical=%#v\nexplicit=%#v", canonicalEnglish, explicitEnglish)
	}

	russian, err := BuildSynthesisPromptForLanguage(bundle, "ru")
	if err != nil {
		t.Fatal(err)
	}
	if russian.Version != canonicalEnglish.Version || russian.User != canonicalEnglish.User {
		t.Fatalf("Russian prompt changed versioned facts request: %#v", russian)
	}
	if russian.System == canonicalEnglish.System ||
		!strings.Contains(russian.System, "name and description prose in Russian") ||
		!strings.Contains(russian.System, "Preserve technical identifiers") {
		t.Fatalf("Russian prompt has no narrow prose instruction: %q", russian.System)
	}
	if strings.Contains(canonicalEnglish.System, "prose in Russian") {
		t.Fatal("canonical English prompt changed")
	}
	if _, err := BuildSynthesisPromptForLanguage(bundle, "fr"); err == nil {
		t.Fatal("unsupported synthesis language was accepted")
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
		metadata.OutputLanguage != "en" ||
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

	legacyUnknownLanguage := bytes.Replace(saved, []byte(`,"output_language":"en"`), nil, 1)
	if bytes.Equal(legacyUnknownLanguage, saved) {
		t.Fatalf("saved record did not contain explicit output language: %s", saved)
	}
	legacyReplayed, err := ReplaySynthesis(bundle, "revision-a", legacyUnknownLanguage)
	if err != nil {
		t.Fatalf("ReplaySynthesis() rejected historical language-unknown record: %v", err)
	}
	if !reflect.DeepEqual(legacyReplayed, result.Landscape) {
		t.Fatalf("historical language-unknown replay differs:\nrecorded: %#v\nreplayed: %#v", result.Landscape, legacyReplayed)
	}
}

func TestRussianSynthesisRecordBindsLanguagePromptAndReplays(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	response := validSynthesisProposalJSON(t, bundle)
	result, err := RecordSynthesisResponseForLanguage(
		bundle,
		"revision-ru",
		"deepseek-compatible",
		"deepseek-v4-flash",
		"ru",
		2*time.Second,
		response,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Call == nil || result.Record.Call.Metadata.OutputLanguage != "ru" {
		t.Fatalf("Russian record metadata = %#v", result.Record.Call)
	}
	russianPrompt, err := BuildSynthesisPromptForLanguage(bundle, "ru")
	if err != nil {
		t.Fatal(err)
	}
	englishPrompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.Call.Metadata.InputBytes != synthesisPromptSize(russianPrompt) ||
		result.Record.Call.Metadata.InputBytes == synthesisPromptSize(englishPrompt) {
		t.Fatalf("Russian prompt identity was not recorded: %#v", result.Record.Call.Metadata)
	}

	saved, err := json.Marshal(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := ReplaySynthesis(bundle, "revision-ru", saved)
	if err != nil {
		t.Fatalf("ReplaySynthesis(Russian) error = %v", err)
	}
	if !reflect.DeepEqual(replayed, result.Landscape) {
		t.Fatal("Russian synthesis record did not replay deterministically")
	}

	tampered := result.Record
	call := *tampered.Call
	call.Metadata.OutputLanguage = "en"
	tampered.Call = &call
	tamperedJSON, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReplaySynthesis(bundle, "revision-ru", tamperedJSON); err == nil {
		t.Fatal("replay accepted a Russian record relabeled as English")
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := RecordSynthesisResponse(bundle, "revision-a", "local-openai", "weak-model", time.Second, test.response)
			if err != nil {
				t.Fatalf("RecordSynthesisResponse() error = %v", err)
			}
			if !result.Landscape.Fallback || result.Landscape.FallbackReason != FallbackRejectedMalformed {
				t.Fatalf("fallback = %v (%q), want invalid proposal fallback", result.Landscape.Fallback, result.Landscape.FallbackReason)
			}
			if !hasLandscapeDiagnostic(result.Landscape.Diagnostics, test.diagnostic) {
				t.Fatalf("diagnostics = %#v, want %q", result.Landscape.Diagnostics, test.diagnostic)
			}
			if result.Record.Call.ResponseState != test.state || result.Record.Call.Metadata.FallbackReason != FallbackRejectedMalformed {
				t.Fatalf("saved call = %#v", result.Record.Call)
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

func TestSynthesisResponseUsesSharedEnvelopeAndResourceLimitIsTerminal(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := validSynthesisProposalJSON(t, bundle)
	response := append(bytes.Repeat([]byte(" "), (256<<10)+1), proposal...)
	result, err := RecordSynthesisResponse(
		bundle, "revision-a", "test", "test", time.Millisecond, response,
	)
	if err != nil {
		t.Fatalf("response above former stage cap rejected: %v", err)
	}
	if result.Landscape.Fallback || result.Record.Call.ResponseState != ResponseCaptured {
		t.Fatalf("response above former stage cap used fallback/omission: %#v", result)
	}

	oversize := bytes.Repeat([]byte("x"), maxSynthesisResponseBytes+1)
	limited, err := RecordSynthesisResponse(
		bundle, "revision-a", "test", "test", time.Millisecond, oversize,
	)
	var limitErr *modelresearch.ResourceLimitError
	if !errors.As(err, &limitErr) ||
		limitErr.Kind != modelresearch.ResourceLimitResponseBytes ||
		limitErr.Limit != maxSynthesisResponseBytes ||
		limitErr.Observed != len(oversize) ||
		limited.Landscape.Fallback || limited.Record.Call != nil {
		t.Fatalf("terminal response limit = result %#v, error %#v", limited, limitErr)
	}

	_, err = ReplaySynthesis(
		bundle,
		"revision-a",
		bytes.Repeat([]byte("x"), maxSynthesisRecordBytes+1),
	)
	limitErr = nil
	if !errors.As(err, &limitErr) || limitErr.Kind != modelresearch.ResourceLimitRecordBytes {
		t.Fatalf("record limit error = %#v, want typed record resource limit", err)
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

func TestGroundedSynthesisNormalizesPackageOnlyComponentButRequiresOtherGrounding(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.GroundingMode = GroundingMixed
	bundle.BehaviorAnchors = []BehaviorAnchor{{
		ID: "process", Kind: AnchorProcessEntry, Label: "process entry",
		Location: evidence.Location{Path: "cmd/main.go", Line: 10, Column: 1},
		Scenario: ScenarioContext{ID: "go:test", Name: "test build"},
		Producer: evidence.Provenance{
			Provider: "test", Version: "v1", Operation: "fixture",
			Location: &evidence.Location{Path: "cmd/main.go", Line: 10, Column: 1},
		},
		Certainty:   evidence.CertaintyStatic,
		MemberIDs:   []MemberID{bundle.Candidates[0].ID},
		Limitations: []string{"Static test evidence; execution is not observed."},
	}}

	proposal := Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Runtime",
			Components: []ProposedComponent{{
				Name: "Process", MemberIDs: []MemberID{bundle.Candidates[0].ID},
			}},
		}},
	}
	raw, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	result, err := RecordSynthesisResponse(bundle, "revision-grounded", "test", "test", time.Millisecond, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || result.Landscape.ValidationOutcome != ValidationAcceptedNormalized ||
		!hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.normalized_package_only_hypothesis") {
		t.Fatalf("package-only proposal = %#v", result.Landscape)
	}

	proposal.Subsystems[0].Components[0] = ProposedComponent{
		Name: "Source file", MemberIDs: []MemberID{bundle.Candidates[2].ID},
	}
	raw, err = json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	result, err = RecordSynthesisResponse(bundle, "revision-grounded", "test", "test", time.Millisecond, raw)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Landscape.Fallback || !hasLandscapeDiagnostic(result.Landscape.Diagnostics, "proposal.ungrounded_primary_component") {
		t.Fatalf("ungrounded non-package proposal = %#v", result.Landscape)
	}

	proposal.Subsystems[0].Components[0] = ProposedComponent{
		Name: "Process", MemberIDs: []MemberID{bundle.Candidates[0].ID},
	}
	proposal.Subsystems[0].Components[0].AnchorIDs = []string{"process"}
	raw, err = json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	result, err = RecordSynthesisResponse(bundle, "revision-grounded", "test", "test", time.Millisecond, raw)
	if err != nil {
		t.Fatal(err)
	}
	if result.Landscape.Fallback || !reflect.DeepEqual(result.Landscape.Subsystems[0].Components[0].AnchorIDs, []string{"process"}) {
		t.Fatalf("grounded proposal = %#v", result.Landscape)
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

func caddyArchitectureReplayFixture(t *testing.T) (CandidateBundle, []byte) {
	t.Helper()

	response, err := os.ReadFile("testdata/caddy_architecture_proposal.json")
	if err != nil {
		t.Fatal(err)
	}
	var proposal Proposal
	if err := json.Unmarshal(response, &proposal); err != nil {
		t.Fatal(err)
	}
	anchorKinds := map[string]BehaviorAnchorKind{
		"anchor-13712c6ead2b942f11dcffac": AnchorRegistryLookup,
		"anchor-15a7795600d479a868920444": AnchorAdminControlPlane,
		"anchor-19ecb1abd8774376a314e8c5": AnchorProcessEntry,
		"anchor-1cb683cf6bdf9da982a3a2cb": AnchorCommandDispatch,
		"anchor-1cbc8e5d96c6a3d4ee8cfc55": AnchorConfigApply,
		"anchor-472447da3f3b81394c5d1b51": AnchorRequestDispatchRoot,
		"anchor-5b79058ae0786d830a6cd1d5": AnchorExtensionFamily,
		"anchor-a020c474f76db85ae85d9357": AnchorConfigIngress,
		"anchor-b95e592bce5b179ca6607eca": AnchorLifecycleInterface,
		"anchor-c068a8bd8b4edb1b113b3b4a": AnchorLifecycleStart,
		"anchor-e55d283d2f5bd2fd480082ec": AnchorConfigAdapter,
		"anchor-e97c73cefeeab8629977e8e2": AnchorRegistryWrite,
		"anchor-fa09229b7ce874a630de6544": AnchorSecurityBoundary,
	}
	candidates := make(map[MemberID]Candidate)
	anchorMembers := make(map[string]map[MemberID]struct{})
	for _, subsystem := range proposal.Subsystems {
		for _, component := range subsystem.Components {
			for index, memberID := range component.MemberIDs {
				if _, exists := candidates[memberID]; !exists {
					path := fmt.Sprintf("fixture/%s/%02d.go", subsystem.Name, index+1)
					candidates[memberID] = Candidate{
						ID: memberID, Name: memberID.Value,
						Facts: []LocalFact{testLocalFact(FactDeclaration, memberID.Value, path, 1)},
					}
				}
				for _, anchorID := range component.AnchorIDs {
					if anchorMembers[anchorID] == nil {
						anchorMembers[anchorID] = make(map[MemberID]struct{})
					}
					anchorMembers[anchorID][memberID] = struct{}{}
				}
			}
		}
	}
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeModularPlatformServer, GroundingMode: GroundingMixed,
	}
	for _, candidate := range candidates {
		bundle.Candidates = append(bundle.Candidates, candidate)
	}
	sortCandidates(bundle.Candidates)
	for anchorID, kind := range anchorKinds {
		memberIDs := make([]MemberID, 0, len(anchorMembers[anchorID]))
		for memberID := range anchorMembers[anchorID] {
			memberIDs = append(memberIDs, memberID)
		}
		sort.Slice(memberIDs, func(i, j int) bool { return memberIDs[i].key() < memberIDs[j].key() })
		location := evidence.Location{Path: "fixture/anchors.go", Line: len(bundle.BehaviorAnchors) + 1, Column: 1}
		bundle.BehaviorAnchors = append(bundle.BehaviorAnchors, BehaviorAnchor{
			ID: anchorID, Kind: kind, Label: string(kind), Location: location,
			Scenario: ScenarioContext{ID: "go:fixture", Name: "saved Caddy replay"},
			Producer: evidence.Provenance{
				Provider: "saved_caddy_run", Version: "20260712-184001", Operation: "replay_architecture_anchor", Location: &location,
			},
			Certainty: evidence.CertaintyStatic, MemberIDs: memberIDs,
			Limitations: []string{"Saved deterministic fixture evidence; runtime execution is not implied."},
		})
	}
	sort.Slice(bundle.BehaviorAnchors, func(i, j int) bool { return bundle.BehaviorAnchors[i].ID < bundle.BehaviorAnchors[j].ID })
	return bundle, response
}
