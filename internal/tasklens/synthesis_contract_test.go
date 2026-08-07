package tasklens

import (
	"context"
	"strings"
	"testing"
)

func TestBuildSynthesisPromptStatesReducerEvidenceRequirements(t *testing.T) {
	repo := newTaskLensTestRepo(t, "prompt-evidence-contract")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}

	prompt, err := BuildSynthesisPrompt(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, requirement := range []string{
		// Phase 5 prompt cleanup: relation evidence closure is completed by
		// the backend — the prompt no longer requires the model to union
		// every evidence ID itself.
		"the backend completes its support_ids from the exact local relation evidence",
		"task_provided must cite the exact evidence whose kind is task_provided",
		"Every repository evidence ID used by guidance must belong to one of the selected anchors",
		"matched_fragments and partial_window never support an absence claim",
		"verification_frontier is immutable",
		"proposed_test_location and missing_evidence are not historical test evidence",
		"direct_call, field_copy, field_read, field_write",
		// Phase 5 prompt cleanup: roles are backend-restored, never copied.
		"The backend restores mechanically derivable details: anchor roles",
	} {
		if !strings.Contains(prompt.User, requirement) {
			t.Fatalf("synthesis prompt is missing reducer requirement %q", requirement)
		}
	}
	for _, removed := range []string{
		"role_contract and role_coverage were derived before synthesis",
		"Every selected anchor role must be copied exactly",
		"support_ids must include every evidence ID supplied by every cited relation",
		`"version": 1`,
		`"observable_or_outcome"`,
		`"effect_to_observe"`,
		`"role": "copy exactly one role`,
		`literal form "Missing evidence: ..."`,
	} {
		if strings.Contains(prompt.User, removed) {
			t.Fatalf("synthesis prompt still delegates backend-owned work %q", removed)
		}
	}
}
