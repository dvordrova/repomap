package pavedpath

import (
	"encoding/json"
	"testing"
)

func TestBuildRecordKeepsExactCommandsAndRejectsInventedPathIndependently(t *testing.T) {
	bundle := testBundle()
	proposal := Proposal{Version: ProposalVersion, Paths: []ProposedPath{
		{
			Title: "Inspect the CLI", Goal: "Inspect the supported flags and their documented output.",
			PrerequisiteEvidenceIDs: []string{"ev-help-prerequisite"},
			Actions: []ProposedAction{{
				EvidenceID: "ev-help", Instruction: "Ask the CLI for its documented flags.",
				Command: "./repomap --help",
			}},
			RelatedStudyDirectionIDs: []string{"study-cli"}, OrderingBasis: OrderingDocumented,
		},
		{
			Title: "Invented", Goal: "This path must be rejected.",
			Actions:       []ProposedAction{{EvidenceID: "ev-build", Instruction: "Do something else.", Command: "make magic"}},
			OrderingBasis: OrderingEditorial,
		},
	}}
	record, err := BuildRecord(bundle, proposal, []string{"study-cli"})
	if err != nil {
		t.Fatalf("BuildRecord() error = %v", err)
	}
	if len(record.Paths) != 1 || record.Paths[0].Actions[0].Command != "./repomap --help" {
		t.Fatalf("paths = %#v", record.Paths)
	}
	if len(record.Issues) != 1 || record.Issues[0].Code != "command_not_in_evidence" {
		t.Fatalf("issues = %#v", record.Issues)
	}
	if len(record.Landmarks) == 0 {
		t.Fatal("expected exact landmarks")
	}
}

func TestBuildRecordRejectsBannerOnlyMakeTarget(t *testing.T) {
	bundle := testBundle()
	bundle.Evidence = append(bundle.Evidence, Evidence{
		ID: "ev-banner", Role: RoleBuildTarget, Path: "Makefile", StartLine: 1, EndLine: 3,
		Label: "all target", Excerpt: []string{"all:", "\t@echo build tool", ""}, Target: "all",
		Commands: []Command{{Value: "make all", Basis: CommandStructural}},
	})
	proposal := Proposal{Version: ProposalVersion, Paths: []ProposedPath{{
		Title: "Build the project", Goal: "Compile the project.",
		Actions: []ProposedAction{{
			EvidenceID: "ev-banner", Instruction: "Use the all target.", Command: "make all",
		}},
		OrderingBasis: OrderingEditorial,
	}}}
	record, err := BuildRecord(bundle, proposal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 0 || len(record.Issues) != 1 ||
		record.Issues[0].Code != "non_substantive_structural_command" {
		t.Fatalf("banner-only target was published: %#v", record)
	}
	for _, landmark := range record.Landmarks {
		if landmark.EvidenceID == "ev-banner" {
			t.Fatalf("banner-only target became a landmark: %#v", landmark)
		}
	}
}

func TestBundleRejectsCredentialBearingSavedText(t *testing.T) {
	bundle := testBundle()
	bundle.Evidence[0].Excerpt = []string{"API_KEY=actual-secret"}
	if err := bundle.Validate(); err == nil {
		t.Fatal("Validate() accepted credential-bearing excerpt")
	}
}

func TestRecordRoundTripRevalidatesIdentityAndEvidence(t *testing.T) {
	bundle := testBundle()
	record, err := BuildRecord(bundle, Proposal{Version: ProposalVersion}, nil)
	if err != nil {
		t.Fatalf("BuildRecord() error = %v", err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecord(raw)
	if err != nil {
		t.Fatalf("DecodeRecord() error = %v", err)
	}
	if len(decoded.Landmarks) != 2 {
		t.Fatalf("landmarks = %d", len(decoded.Landmarks))
	}
}

func TestRecordRejectsTamperedCopyAuthorization(t *testing.T) {
	bundle := testBundle()
	bundle.Evidence[1].Commands[0].SafeToCopy = false
	record, err := BuildRecord(bundle, Proposal{Version: ProposalVersion, Paths: []ProposedPath{{
		Title: "Inspect", Goal: "Inspect the documented CLI output.",
		PrerequisiteEvidenceIDs: []string{"ev-help-prerequisite"},
		Actions: []ProposedAction{{
			EvidenceID: "ev-help", Instruction: "Inspect the documented flags.",
			Command: "./repomap --help",
		}},
		OrderingBasis: OrderingDocumented,
	}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	record.Paths[0].Actions[0].SafeToCopy = true
	if err := record.Validate(); err == nil {
		t.Fatal("Validate() accepted tampered copy authorization")
	}
}

func TestRecordRejectsLandmarkWithCommandAndEndpoint(t *testing.T) {
	record, err := BuildRecord(testBundle(), Proposal{Version: ProposalVersion}, nil)
	if err != nil {
		t.Fatal(err)
	}
	record.Landmarks[0].Endpoint = "http://127.0.0.1:9999/"
	record.Landmarks[0].ID = stableID(
		"landmark",
		record.Landmarks[0].EvidenceID,
		record.Landmarks[0].Command,
		record.Landmarks[0].Endpoint,
	)
	if err := record.Validate(); err == nil {
		t.Fatal("Validate() accepted a command landmark with an arbitrary endpoint")
	}
}

func TestBuildRecordScopedRejectsEvidenceOutsideEditorBundle(t *testing.T) {
	record, err := BuildRecordScoped(
		testBundle(),
		Proposal{Version: ProposalVersion, Paths: []ProposedPath{{
			Title: "Inspect", Goal: "Inspect the CLI flags.",
			Actions: []ProposedAction{{
				EvidenceID: "ev-help", Instruction: "Inspect documented flags.", Command: "./repomap --help",
			}},
			OrderingBasis: OrderingEditorial,
		}}},
		nil,
		[]string{"ev-build"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 0 || len(record.Issues) != 1 ||
		record.Issues[0].Code != "evidence_outside_editor_bundle" {
		t.Fatalf("record = %#v", record)
	}
}

func TestDecodeProposalRejectsUnknownFields(t *testing.T) {
	_, err := DecodeProposal([]byte(`{"version":1,"paths":[],"surprise":true}`))
	if err == nil {
		t.Fatal("DecodeProposal() accepted unknown field")
	}
}

func TestBuildRecordRejectsCommandSmuggledThroughProse(t *testing.T) {
	bundle := testBundle()
	proposal := Proposal{Version: ProposalVersion, Paths: []ProposedPath{{
		Title: "Inspect the CLI", Goal: "Learn the supported flags.",
		Actions: []ProposedAction{{
			EvidenceID: "ev-help", Instruction: "Run rm -rf work before inspecting flags.",
			Command: "./repomap --help",
		}},
		OrderingBasis: OrderingEditorial,
	}}}
	record, err := BuildRecord(bundle, proposal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 0 || len(record.Issues) != 1 || record.Issues[0].Code != "action_evidence_invalid" {
		t.Fatalf("command-like prose was published: %#v", record)
	}
}

func TestBuildRecordAllowsInstructionToRepeatItsExactCommand(t *testing.T) {
	bundle := testBundle()
	proposal := Proposal{Version: ProposalVersion, Paths: []ProposedPath{{
		Title: "Inspect", Goal: "Inspect the documented CLI output.",
		PrerequisiteEvidenceIDs: []string{"ev-help-prerequisite"},
		Actions: []ProposedAction{{
			EvidenceID: "ev-help", Instruction: "Run ./repomap --help.",
			Command: "./repomap --help",
		}},
		OrderingBasis: OrderingDocumented,
	}}}
	record, err := BuildRecord(bundle, proposal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 1 || len(record.Issues) != 0 {
		t.Fatalf("exact command repetition was rejected: %#v", record)
	}
}

func TestBuildRecordAllowsProductNameInOperationalProse(t *testing.T) {
	bundle := testBundle()
	proposal := Proposal{Version: ProposalVersion, Paths: []ProposedPath{{
		Title: "Inspect the repomap binary", Goal: "Display the repomap CLI help output.",
		PrerequisiteEvidenceIDs: []string{"ev-help-prerequisite"},
		Actions: []ProposedAction{{
			EvidenceID: "ev-help", Instruction: "Ask repomap to display its documented flags.",
			Command: "./repomap --help",
		}},
		OrderingBasis: OrderingDocumented,
	}}}
	record, err := BuildRecord(bundle, proposal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 1 || len(record.Issues) != 0 {
		t.Fatalf("ordinary product prose was rejected: %#v", record)
	}
}

func TestBuildRecordRejectsUnprovenOrderingAtPublication(t *testing.T) {
	bundle := testBundle()
	proposal := Proposal{Version: ProposalVersion, Paths: []ProposedPath{{
		Title: "Build and inspect", Goal: "Prepare a binary and inspect its flags.",
		PrerequisiteEvidenceIDs: []string{
			"ev-build-prerequisite",
			"ev-help-prerequisite",
		},
		Actions: []ProposedAction{
			{EvidenceID: "ev-build", Instruction: "Use the repository build target.", Command: "make build"},
			{EvidenceID: "ev-help", Instruction: "Inspect the documented flags.", Command: "./repomap --help"},
		},
		OrderingBasis: OrderingDocumented,
	}}}
	record, err := BuildRecord(bundle, proposal, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Paths) != 0 || len(record.Issues) != 1 ||
		record.Issues[0].Code != PublicationIssueMissingActions {
		t.Fatalf("unproven cross-file order was published: %#v", record)
	}
}

func TestBundleRejectsCopyAuthorizationFromRedactedEvidence(t *testing.T) {
	bundle := testBundle()
	bundle.Evidence[0].Redacted = true
	if err := bundle.Validate(); err == nil {
		t.Fatal("redacted evidence retained copy authorization")
	}
}

func testBundle() Bundle {
	return Bundle{
		Version: BundleVersion, RepoName: "repomap",
		AllowedPaths: []string{"Makefile", "README.md"},
		Evidence: []Evidence{
			{
				ID: "ev-build", Role: RoleBuildTarget, Path: "Makefile", StartLine: 8, EndLine: 9,
				Label: "build target", Excerpt: []string{"build:", "\tgo build -o repomap ./cmd/repomap"},
				Target: "build", Commands: []Command{{Value: "make build", Basis: CommandStructural, SafeToCopy: true}},
			},
			{
				ID: "ev-help", Role: RoleDocumentedProcedure, Path: "README.md", StartLine: 20, EndLine: 21,
				Label: "inspect CLI flags",
				Excerpt: []string{
					"$ ./repomap --help",
					"Usage: repomap [flags]",
				},
				Commands: []Command{{Value: "./repomap --help", Basis: CommandExact, SafeToCopy: true}},
			},
			{
				ID: "ev-build-prerequisite", Role: RoleDocumentedProcedure, Path: "Makefile",
				StartLine: 7, EndLine: 8, Label: "build prerequisites",
				Excerpt: []string{
					"Before running make build, install the required Go toolchain.",
					"build:",
				},
			},
			{
				ID: "ev-help-prerequisite", Role: RoleDocumentedProcedure, Path: "README.md",
				StartLine: 19, EndLine: 20, Label: "CLI prerequisites",
				Excerpt: []string{
					"Before running this procedure, install repomap.",
					"$ ./repomap --help",
				},
			},
		},
	}
}
