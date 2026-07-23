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
		"support_ids must include every evidence ID supplied by every cited relation",
		"task_provided must cite the exact evidence whose kind is task_provided",
		"Every repository evidence ID used by guidance must belong to one of the selected anchors",
		"role_contract and role_coverage were derived before synthesis and are immutable",
		"matched_fragments and partial_window never support an absence claim",
		"verification_frontier is immutable",
		"proposed_test_location and missing_evidence are not historical test evidence",
		"direct_call, field_copy, field_read, field_write",
	} {
		if !strings.Contains(prompt.User, requirement) {
			t.Fatalf("synthesis prompt is missing reducer requirement %q", requirement)
		}
	}
}
