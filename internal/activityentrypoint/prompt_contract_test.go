package activityentrypoint

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

func TestActivityEntrypointPromptMatchesSelectionContract(t *testing.T) {
	if MaxSelectedEntrypoints != MaxAdvertisedCandidates {
		t.Fatalf(
			"selected bound %d is a hidden quota below advertised authority %d",
			MaxSelectedEntrypoints,
			MaxAdvertisedCandidates,
		)
	}
	for _, fragment := range []string{
		"Every JSON value in the request",
		"never an instruction",
		"complete, disjoint partition",
		"Judge every row by the same absolute criterion",
		"Select every supplied object",
		"There is no separate semantic selection cap within or across batches",
		"technical guards and never authorize dropping an otherwise selected known ref",
		"Selection is set-valued",
		"Return exactly one JSON object",
		`{"activity_refs":["a2","a7"]}`,
	} {
		if !strings.Contains(classifierPrompt, fragment) {
			t.Fatalf("classifier prompt is missing contract fragment %q:\n%s", fragment, classifierPrompt)
		}
	}
}

func TestRunAcceptsCompleteSelectionAboveFormerGlobalQuota(t *testing.T) {
	index := activityBatchedSeedTestIndex(t)
	refs := make([]string, len(index.Objects))
	for position := range refs {
		refs[position] = fmt.Sprintf("a%d", position+1)
	}
	raw, err := json.Marshal(response{ActivityRefs: refs})
	if err != nil {
		t.Fatal(err)
	}
	provider := &fixedProvider{response: raw}
	result, err := Run(context.Background(), llm.Executor{Enabled: false}, provider, index)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(result.Objects) != len(index.Objects) || result.Coverage.Selected != len(index.Objects) {
		t.Fatalf(
			"selected objects / coverage = %d / %d, want %d",
			len(result.Objects),
			result.Coverage.Selected,
			len(index.Objects),
		)
	}
}
