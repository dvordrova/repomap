package runtimeportfolio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestCompactHigherLevelReducePreservesDistinctLargeEvidenceRoles(t *testing.T) {
	const (
		targetCount               = 27
		responsibilitiesPerTarget = 100
	)
	input := oversizedRuntimePortfolioInput(targetCount, responsibilitiesPerTarget)
	compilation := mustCompile(t, input)
	if len(compilation.mapShards) < 2 {
		t.Fatalf("large-evidence regression input produced %d map shard(s), want multiple", len(compilation.mapShards))
	}

	provider := &compactReduceRegressionProvider{}
	outcome, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4,
	}, provider, input)
	if err != nil {
		t.Fatal(err)
	}

	observed := provider.snapshot()
	exactCalls := make([]compactReduceRegressionCall, 0)
	summaryCalls := make([]compactReduceRegressionCall, 0)
	for _, call := range observed {
		if call.payloadBytes == 0 || call.payloadBytes > MaxRequestBytes {
			t.Fatalf("reduce level %d payload bytes = %d", call.level, call.payloadBytes)
		}
		switch call.detailMode {
		case reduceDetailExactEvidence:
			exactCalls = append(exactCalls, call)
		case reduceDetailValidatedSummary:
			summaryCalls = append(summaryCalls, call)
		default:
			t.Fatalf("reduce level %d detail mode = %q", call.level, call.detailMode)
		}
	}
	if len(exactCalls) < 2 {
		t.Fatalf("full-evidence reduce calls = %d, want multiple bounded L1 calls", len(exactCalls))
	}
	for _, call := range exactCalls {
		if call.level != 1 || call.batch.Count != len(exactCalls) ||
			call.evidenceRows == 0 || call.candidateCount == 0 {
			t.Fatalf("full-evidence reduce call = %#v", call)
		}
	}
	if len(summaryCalls) != 1 {
		t.Fatalf("compact reduce calls = %d, want one global L2 call", len(summaryCalls))
	}
	compact := summaryCalls[0]
	if compact.level != 2 || compact.batch != (shardRequest{Ordinal: 1, Count: 1}) ||
		compact.candidateCount != targetCount || compact.evidenceRows != 0 {
		t.Fatalf("compact global reduce call = %#v", compact)
	}
	if bytes.Contains(compact.wire, []byte(`"evidence_refs"`)) ||
		bytes.Contains(compact.wire, []byte(`"evidence_catalog"`)) {
		t.Fatalf("compact reduce wire retained exact evidence fields: %s", compact.wire)
	}

	result := outcome.Value
	if len(result.Roles) != targetCount {
		t.Fatalf("final roles = %d, want %d distinct roles", len(result.Roles), targetCount)
	}
	expectedEvidence := make(map[string]map[string]Evidence, targetCount)
	for _, evidence := range compilation.evidenceByRef {
		if evidence.ProgramTargetID == "" {
			continue
		}
		if expectedEvidence[evidence.ProgramTargetID] == nil {
			expectedEvidence[evidence.ProgramTargetID] = make(map[string]Evidence)
		}
		expectedEvidence[evidence.ProgramTargetID][evidence.ID] = evidence
	}
	observedTargets := make(map[string]struct{}, targetCount)
	for _, role := range result.Roles {
		if len(role.Implementations) != 1 {
			t.Fatalf("role %q implementations = %d, want one", role.Name, len(role.Implementations))
		}
		targetID := role.Implementations[0].ProgramTargetID
		if _, duplicate := observedTargets[targetID]; duplicate {
			t.Fatalf("target %q was merged into or repeated across final roles", targetID)
		}
		observedTargets[targetID] = struct{}{}
		want := expectedEvidence[targetID]
		if len(role.Evidence) != len(want) || len(want) != responsibilitiesPerTarget+1 {
			t.Fatalf("role %q exact evidence = %d/%d", role.Name, len(role.Evidence), len(want))
		}
		for _, evidence := range role.Evidence {
			expected, known := want[evidence.ID]
			if !known || !reflect.DeepEqual(evidence, expected) {
				t.Fatalf("role %q did not restore exact evidence %q", role.Name, evidence.ID)
			}
		}
	}
	if len(observedTargets) != targetCount {
		t.Fatalf("final target coverage = %d/%d", len(observedTargets), targetCount)
	}
	if err := result.ValidateAgainst(input); err != nil {
		t.Fatalf("ValidateAgainst: %v", err)
	}
}

type compactReduceRegressionProvider struct {
	mu    sync.Mutex
	calls []compactReduceRegressionCall
}

type compactReduceRegressionCall struct {
	level          int
	detailMode     reduceDetailMode
	batch          shardRequest
	payloadBytes   int
	candidateCount int
	evidenceRows   int
	wire           []byte
}

type compactReduceRegressionEnvelope struct {
	Response json.RawMessage `json:"response"`
}

func (provider *compactReduceRegressionProvider) State() []byte {
	return []byte(`{"endpoint":"https://provider.test/v1/chat","model":"compact-reduce-regression"}`)
}

func (provider *compactReduceRegressionProvider) Prepare(
	prompt llm.Prompt,
	_ llm.Limits,
) (llm.Prepared, error) {
	user := []byte(prompt.User)
	var phase struct {
		Phase string `json:"phase"`
	}
	if err := json.Unmarshal(user, &phase); err != nil {
		return llm.Prepared{}, err
	}

	var responseRaw []byte
	switch phase.Phase {
	case "map":
		var request mapRequest
		if err := json.Unmarshal(user, &request); err != nil {
			return llm.Prepared{}, err
		}
		roles := make([]responseRole, 0, len(request.Targets))
		for _, target := range request.Targets {
			evidenceRefs := append([]string(nil), target.EvidenceRefs...)
			for _, responsibility := range target.Responsibilities {
				evidenceRefs = append(evidenceRefs, responsibility.EvidenceRefs...)
			}
			evidenceRefs = canonicalStrings(evidenceRefs)
			if len(evidenceRefs) == 0 {
				return llm.Prepared{}, fmt.Errorf("map target %q has no exact evidence", target.Ref)
			}
			roles = append(roles, responseRole{
				Name:       "Distinct runtime " + target.Ref,
				Purpose:    "Runs the independently described repository target " + target.Ref + ".",
				Prominence: ProminenceUnknown, Kind: RoleKindService,
				Requiredness: RequirednessUnknown, Confidence: ConfidenceHigh,
				MappingStatus:   MappingMapped,
				Implementations: []responseImplementation{{TargetRef: target.Ref}},
				EvidenceRefs:    evidenceRefs,
			})
		}
		encoded, err := json.Marshal(response{Roles: roles})
		if err != nil {
			return llm.Prepared{}, err
		}
		responseRaw = encoded
	case "reduce":
		var request reduceRequest
		if err := json.Unmarshal(user, &request); err != nil {
			return llm.Prepared{}, err
		}
		roles := make([]reduceResponseRole, 0, len(request.Candidates))
		for _, candidate := range request.Candidates {
			switch request.DetailMode {
			case reduceDetailExactEvidence:
				if len(candidate.EvidenceRefs) == 0 || candidate.EvidenceCount != 0 ||
					len(candidate.EvidenceKinds) != 0 || len(request.EvidenceCatalog) == 0 {
					return llm.Prepared{}, fmt.Errorf("invalid full-evidence candidate %q", candidate.Ref)
				}
			case reduceDetailValidatedSummary:
				if candidate.EvidenceRefs != nil || candidate.EvidenceCount == 0 ||
					len(candidate.EvidenceKinds) == 0 || request.EvidenceCatalog != nil {
					return llm.Prepared{}, fmt.Errorf("invalid compact candidate %q", candidate.Ref)
				}
			default:
				return llm.Prepared{}, fmt.Errorf("unexpected reduce detail mode %q", request.DetailMode)
			}
			roles = append(roles, reduceResponseRole{
				Name: candidate.Name, Purpose: candidate.Purpose,
				Prominence: candidate.Prominence, Kind: candidate.Kind,
				Requiredness: candidate.Requiredness, Confidence: candidate.Confidence,
				MappingStatus: candidate.MappingStatus,
				CandidateRefs: []string{candidate.Ref},
			})
		}
		if request.DetailMode == reduceDetailValidatedSummary &&
			(bytes.Contains(user, []byte(`"evidence_refs"`)) ||
				bytes.Contains(user, []byte(`"evidence_catalog"`))) {
			return llm.Prepared{}, fmt.Errorf("compact reduce wire contains exact evidence fields")
		}
		encoded, err := json.Marshal(reduceResponse{Roles: roles})
		if err != nil {
			return llm.Prepared{}, err
		}
		responseRaw = encoded
		provider.mu.Lock()
		provider.calls = append(provider.calls, compactReduceRegressionCall{
			level: request.Level, detailMode: request.DetailMode, batch: request.Batch,
			payloadBytes: len(user), candidateCount: len(request.Candidates),
			evidenceRows: len(request.EvidenceCatalog), wire: append([]byte(nil), user...),
		})
		provider.mu.Unlock()
	default:
		return llm.Prepared{}, fmt.Errorf("unexpected runtime phase %q", phase.Phase)
	}

	envelope, err := json.Marshal(compactReduceRegressionEnvelope{Response: responseRaw})
	if err != nil {
		return llm.Prepared{}, err
	}
	return llm.NewPrepared(envelope)
}

func (provider *compactReduceRegressionProvider) Complete(
	_ context.Context,
	prepared llm.Prepared,
) (llm.Completion, error) {
	var envelope compactReduceRegressionEnvelope
	if err := json.Unmarshal(prepared.Bytes(), &envelope); err != nil {
		return llm.Completion{}, err
	}
	return llm.Completion{
		Response: append([]byte(nil), envelope.Response...), FinishReason: llm.FinishStop,
		ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1},
	}, nil
}

func (provider *compactReduceRegressionProvider) snapshot() []compactReduceRegressionCall {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	result := make([]compactReduceRegressionCall, len(provider.calls))
	copy(result, provider.calls)
	return result
}
