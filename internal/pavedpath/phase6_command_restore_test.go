package pavedpath

import (
	"testing"
)

// Phase 6 prompt cleanup: the model selects evidence IDs and writes editorial
// instruction; the backend restores the exact command and safe-to-copy
// metadata from the selected evidence.
func TestRestorePrimaryCommand(t *testing.T) {
	t.Parallel()
	item := Evidence{
		ID: "prep", Role: RoleDocumentedProcedure, Path: "README.md",
		StartLine: 10, EndLine: 10, Label: "Prepare",
		Excerpt:  []string{"make setup"},
		Commands: []Command{{Value: "make setup", Basis: CommandExact, SafeToCopy: true}},
	}
	command, restored := restorePrimaryCommand(item)
	if !restored || command.Value != "make setup" || !command.SafeToCopy {
		t.Fatalf("single substantive command not restored: %#v restored=%v", command, restored)
	}
	// Ambiguous evidence (multiple substantive commands) is not restored.
	item.Commands = append(item.Commands, Command{Value: "make test", Basis: CommandExact, SafeToCopy: true})
	if _, restored := restorePrimaryCommand(item); restored {
		t.Fatalf("ambiguous evidence must not auto-restore")
	}
	// No substantive command: not restored.
	item.Commands = nil
	if _, restored := restorePrimaryCommand(item); restored {
		t.Fatalf("missing command must not be restored")
	}
}

// Phase 6: validateProposedPath restores the exact command/endpoint from the
// evidence when the model omits them, and still validates explicit echoes.
func TestValidateProposedPathRestoresBackendOwnedValues(t *testing.T) {
	t.Parallel()
	evidence := map[string]Evidence{
		"prep": {
			ID: "prep", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 10, EndLine: 10, Label: "Prepare",
			Excerpt:  []string{"make setup"},
			Commands: []Command{{Value: "make setup", Basis: CommandExact, SafeToCopy: true}},
		},
		"run": {
			ID: "run", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 12, EndLine: 12, Label: "Run",
			Excerpt:  []string{"make test"},
			Commands: []Command{{Value: "make test", Basis: CommandExact, SafeToCopy: false}},
		},
	}
	proposed := ProposedPath{
		Title: "Build and test", Goal: "Build and verify the project.",
		Actions: []ProposedAction{
			{EvidenceID: "prep", Instruction: "Prepare the workspace."},
			{EvidenceID: "run", Instruction: "Run the test suite."},
		},
		OrderingBasis: OrderingEditorial,
	}
	built, code := validateProposedPath(proposed, evidence, nil)
	if code != "" {
		t.Fatalf("validateProposedPath code = %q", code)
	}
	if len(built.Actions) != 2 {
		t.Fatalf("actions = %#v", built.Actions)
	}
	if built.Actions[0].Command != "make setup" || !built.Actions[0].SafeToCopy {
		t.Fatalf("first command not restored: %#v", built.Actions[0])
	}
	if built.Actions[1].Command != "make test" || built.Actions[1].SafeToCopy {
		t.Fatalf("second command not restored with safe_to_copy: %#v", built.Actions[1])
	}
	// Explicit echoed command must match the evidence exactly.
	proposed.Actions[0].Command = "make teast"
	if _, code := validateProposedPath(proposed, evidence, nil); code != "command_not_in_evidence" {
		t.Fatalf("explicit mismatched command code = %q, want command_not_in_evidence", code)
	}
	proposed.Actions[0].Command = "make setup"
	if built, code := validateProposedPath(proposed, evidence, nil); code != "" || built.Actions[0].Command != "make setup" {
		t.Fatalf("explicit exact command: code=%q built=%#v", code, built.Actions[0])
	}
	// Endpoint restoration.
	evidence["api"] = Evidence{
		ID: "api", Role: RoleDocumentedProcedure, Path: "README.md",
		StartLine: 20, EndLine: 20, Label: "API",
		Excerpt:  []string{"curl /api/v1/health"},
		Endpoint: "/api/v1/health",
	}
	proposed.Actions = []ProposedAction{{EvidenceID: "api", Instruction: "Check health."}}
	built, code = validateProposedPath(proposed, evidence, nil)
	if code != "" {
		t.Fatalf("endpoint path code = %q", code)
	}
	if built.Actions[0].Endpoint != "/api/v1/health" {
		t.Fatalf("endpoint not restored: %#v", built.Actions[0])
	}
}
