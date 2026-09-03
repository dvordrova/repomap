package programgrouping

import (
	"fmt"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// countingProvider records how many fit probes the planner asks for.
type countingProvider struct {
	presetProvider
	maxRefs int
	probes  int
}

func (provider *countingProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	provider.probes++
	provider.presetProvider.maxInitialGroupRefs = provider.maxRefs
	return provider.presetProvider.Prepare(prompt, limits)
}

// TestBatchPlanCoversEverySubjectInContiguousPrefixes proves the searched plan
// is still an exact, ordered, disjoint cover, and that the ordinary case where
// everything fits costs a single probe rather than one per subject.
func TestBatchPlanCoversEverySubjectInContiguousPrefixes(t *testing.T) {
	index := groupingTestIndex(t, "python")
	compilation, err := Compile(index)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for _, limit := range []int{0, 1, 2, 3} {
		provider := &countingProvider{maxRefs: limit}
		plan, err := compilation.batchesForProvider(provider)
		if err != nil {
			t.Fatalf("limit %d: batchesForProvider: %v", limit, err)
		}
		covered := make([]string, 0, len(compilation.categorizedRefs))
		for _, item := range plan {
			if limit > 0 && len(item.groupRefs) > limit {
				t.Fatalf("limit %d: batch of %d refs", limit, len(item.groupRefs))
			}
			covered = append(covered, item.groupRefs...)
		}
		if len(covered) != len(compilation.categorizedRefs) {
			t.Fatalf("limit %d: covered %d of %d subjects", limit, len(covered), len(compilation.categorizedRefs))
		}
		for position, ref := range compilation.categorizedRefs {
			if covered[position] != ref {
				t.Fatalf("limit %d: subject %d is %q, want %q", limit, position, covered[position], ref)
			}
		}
		if limit == 0 {
			if len(plan) != 1 {
				t.Fatalf("everything fits but the plan has %d batches", len(plan))
			}
			if provider.probes != 1 {
				t.Fatalf("planning one fitting batch took %d probes, want 1", provider.probes)
			}
		}
	}
}

// TestMergeFailureKeepsTheShardGroups pins that a target survives a merge the
// model gets wrong. The shards' own groups are already validated; consolidating
// them is an improvement, not a precondition.
func TestMergeFailureKeepsTheShardGroups(t *testing.T) {
	index := groupingTestIndex(t, "python")
	provider := &presetProvider{maxInitialGroupRefs: 1}
	provider.respond = func(request groupingRequest) []byte {
		if request.Phase == phaseGrouping {
			ref := request.GroupRefs[0]
			subject := subjectByRef(t, request.Request, ref)
			lane := laneForCategories(subject.Categories)
			_ = lane
			return []byte(fmt.Sprintf(
				`{"assign":[{"ref":%q,"group":%q}],"links":[]}`, ref, "Group "+ref,
			))
		}
		// A consolidation that assigns nothing is refused, and the shards'
		// own groups survive it.
		return []byte(`{"assign":[]}`)
	}

	grouped, diagnostics, err := Run(t.Context(), llm.Executor{
		Enabled: false, BatchConcurrency: 4, BatchController: &llm.BatchController{},
	}, provider, index)
	if err != nil {
		t.Fatalf("a bad merge failed the target: %v", err)
	}
	if len(grouped.Groups) == 0 {
		t.Fatal("the shard groups were lost with the merge")
	}
	var skipped bool
	for _, diagnostic := range diagnostics {
		if diagnostic.Kind == diagnosticMergeSkipped {
			skipped = true
		}
	}
	if !skipped {
		t.Fatalf("the skipped merge was not recorded: %#v", diagnostics)
	}
}
