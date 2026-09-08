package documentationreduce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/bits"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/readmetargetscout"
)

// Count bytes and row appearances, not elapsed time. Searching the entire
// remaining tail can use few Prepare calls while repeatedly encoding almost
// the whole reservoir when only three or seven rows fit.
func TestPackingPreparationWorkAndMaximalDisjointWindows(t *testing.T) {
	for _, phase := range []string{"source", "merge"} {
		for _, size := range []struct{ count, capacity int }{
			{64, 64}, {128, 128}, {256, 3}, {512, 3}, {256, 7}, {512, 7},
		} {
			t.Run(fmt.Sprintf("%s/%d_capacity_%d", phase, size.count, size.capacity), func(t *testing.T) {
				units, candidates, authority := packingEvidence(size.count)
				measure := &packingWorkProvider{documentationPresetProvider: &documentationPresetProvider{}}
				// Derive the fake provider envelope from its complete serialized
				// request. Uniform-width source identities make each window of this
				// length fit exactly; this is not a production row-count budget.
				var err error
				if phase == "source" {
					_, err = sourceUnitsFit(measure, units[:size.capacity])
				} else {
					_, err = mergeCandidatesFit(measure, candidates[:size.capacity], 1)
				}
				if err != nil {
					t.Fatal(err)
				}
				envelope := measure.preparedBytes
				measure = &packingWorkProvider{documentationPresetProvider: &documentationPresetProvider{}}
				if phase == "source" {
					_, err = sourceUnitsFit(measure, units[:1])
				} else {
					_, err = mergeCandidatesFit(measure, candidates[:1], 1)
				}
				if err != nil {
					t.Fatal(err)
				}
				singletonBytes := measure.preparedBytes
				provider := &packingWorkProvider{documentationPresetProvider: &documentationPresetProvider{maximumPreparedBytes: envelope}}
				if phase == "source" {
					got, err := packSourceBatches(provider, units)
					if err != nil {
						t.Fatal(err)
					}
					var plan []sourceBatch
					for start := 0; start < len(units); start += size.capacity {
						plan = append(plan, sourceBatch{units: units[start:min(start+size.capacity, len(units))]})
					}
					want, err := materializeSourcePlan(plan)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatal("source plan lost evidence, order, local refs, parts, or exact materialized bytes")
					}
				} else {
					got, err := packMergeBatches(provider, candidates, 1, authority)
					if err != nil {
						t.Fatal(err)
					}
					var plan []mergeBatch
					for start := 0; start < len(candidates); start += size.capacity {
						plan = append(plan, mergeBatch{candidates: candidates[start:min(start+size.capacity, len(candidates))]})
					}
					want, err := materializeMergePlan(plan, 1, authority)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Fatal("merge plan lost evidence, order, local refs, authority, or exact materialized bytes")
					}
				}
				if provider.sourceCalls != 0 || provider.mergeCalls != 0 {
					t.Fatal("packing called Complete")
				}
				maxRows := size.count * (bits.Len(uint(size.capacity)) + 3)
				t.Logf("Prepare=%d row_appearances=%d user_bytes=%d prepared_bytes=%d max_rows=%d", provider.calls, provider.rows, provider.userBytes, provider.preparedBytes, maxRows)
				if provider.rows > maxRows || provider.preparedBytes > maxRows*singletonBytes {
					t.Fatalf("packing re-encoded too much evidence: rows=%d (bound %d), prepared bytes=%d (bound %d)", provider.rows, maxRows, provider.preparedBytes, maxRows*singletonBytes)
				}
			})
		}
	}
}

func TestPackingPreservesPreparationErrors(t *testing.T) {
	units, candidates, authority := packingEvidence(3)
	for _, cause := range []error{
		context.Canceled,
		errors.New("provider configuration failed"),
		llm.NewResourceLimitError(llm.ResourceLimitError{Kind: llm.ResourceLimitContextTokens, Limit: 10}),
	} {
		for _, phase := range []string{"source", "merge"} {
			t.Run(phase+"/"+cause.Error(), func(t *testing.T) {
				provider := &packingWorkProvider{documentationPresetProvider: &documentationPresetProvider{}, err: cause}
				var err error
				if phase == "source" {
					_, err = packSourceBatches(provider, units)
				} else {
					_, err = packMergeBatches(provider, candidates, 1, authority)
				}
				if !errors.Is(err, cause) || provider.calls != 1 {
					t.Fatalf("preparation error = %v after %d calls, want original terminal error", err, provider.calls)
				}
			})
		}
	}
}

func TestPackingRejectsIndivisibleMergeCandidateAtAnyPosition(t *testing.T) {
	for _, position := range []int{0, 1} {
		t.Run(fmt.Sprint(position), func(t *testing.T) {
			_, candidates, authority := packingEvidence(2)
			measure := &packingWorkProvider{documentationPresetProvider: &documentationPresetProvider{}}
			if fits, err := mergeCandidatesFit(measure, candidates[:1], 1); err != nil || !fits {
				t.Fatalf("singleton envelope: fits=%v err=%v", fits, err)
			}
			provider := &packingWorkProvider{documentationPresetProvider: &documentationPresetProvider{maximumPreparedBytes: measure.preparedBytes}}
			candidates[position].overview = strings.Repeat(candidates[position].overview, 8)
			plan, err := packMergeBatches(provider, candidates, 1, authority)
			if err == nil || !strings.Contains(err.Error(), "indivisible merge candidate") || len(plan) != 0 {
				t.Fatalf("oversized candidate entered a partial plan: plan=%d err=%v", len(plan), err)
			}
			if provider.sourceCalls != 0 || provider.mergeCalls != 0 {
				t.Fatal("rejected packing called Complete")
			}
		})
	}
}

func packingEvidence(count int) ([]documentUnit, []normalizedReduction, map[string]documentAuthority) {
	var units []documentUnit
	var candidates []normalizedReduction
	authority := make(map[string]documentAuthority)
	for index := range count {
		ref := fmt.Sprintf("d%04d", index+1)
		path := fmt.Sprintf("docs/%04d/README.md", index+1)
		text := fmt.Sprintf("Original source %04d: ", index+1) + strings.Repeat("evidence α with \"quotes\". ", 16)
		units = append(units, documentUnit{ref: ref, path: path, kind: readmetargetscout.GuidanceReadme, content: text})
		candidates = append(candidates, normalizedReduction{overview: text, sources: []responseSource{{Ref: ref, Claims: []string{text}, Concepts: []string{"Evidence"}}}})
		authority[ref] = documentAuthority{path: path, kind: readmetargetscout.GuidanceReadme}
	}
	return units, candidates, authority
}

type packingWorkProvider struct {
	*documentationPresetProvider
	calls, rows, userBytes, preparedBytes int
	err                                   error
}

func (provider *packingWorkProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	provider.calls++
	if provider.err != nil {
		return llm.Prepared{}, provider.err
	}
	var request struct {
		Documents  []json.RawMessage `json:"documents"`
		Candidates []json.RawMessage `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(prompt.User), &request); err != nil {
		return llm.Prepared{}, err
	}
	provider.rows += len(request.Documents) + len(request.Candidates)
	provider.userBytes += len(prompt.User)
	wire, err := json.Marshal(documentationPresetPrepared{System: prompt.System, User: prompt.User})
	if err != nil {
		return llm.Prepared{}, err
	}
	provider.preparedBytes += len(wire)
	return provider.documentationPresetProvider.Prepare(prompt, limits)
}
