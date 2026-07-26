package pavedpath

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestAssessPathPublicationRequiresExplicitSameProcedurePrerequisite(t *testing.T) {
	t.Parallel()

	t.Run("complete structure", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != "" {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("empty prerequisite", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		saved.PrerequisiteEvidenceIDs = nil
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("reused action evidence", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		saved.PrerequisiteEvidenceIDs = []string{"steps"}
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("duplicate action evidence under another id", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		duplicate := evidence["steps"]
		duplicate.ID = "duplicate-action"
		duplicate.Label = "Install and run tool"
		evidence[duplicate.ID] = duplicate
		saved.PrerequisiteEvidenceIDs = []string{duplicate.ID}
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("unrelated neighbouring evidence", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		evidence["unrelated"] = Evidence{
			ID: "unrelated", Role: RoleDocumentedProcedure, Path: "docs/setup.md",
			StartLine: 1, EndLine: 1, Label: "Run tool",
			Excerpt: []string{"First install tool."},
		}
		saved.PrerequisiteEvidenceIDs = []string{"unrelated"}
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("distant same label", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		evidence["distant"] = Evidence{
			ID: "distant", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 100, EndLine: 100, Label: "Run tool",
			Excerpt: []string{"First install tool."},
		}
		saved.PrerequisiteEvidenceIDs = []string{"distant"}
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("bare install action", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		evidence["install"] = Evidence{
			ID: "install", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 12, EndLine: 12, Label: "Install tool",
			Excerpt: []string{"install tool"},
		}
		saved.PrerequisiteEvidenceIDs = []string{"install"}
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("role alone", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		evidence["configuration"] = Evidence{
			ID: "configuration", Role: RoleConfiguration, Path: "README.md",
			StartLine: 11, EndLine: 11, Label: "Run tool",
			Excerpt: []string{"MODE=test"},
		}
		saved.PrerequisiteEvidenceIDs = []string{"configuration"}
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("redacted prerequisite", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		item := evidence["prerequisite"]
		item.Redacted = true
		evidence["prerequisite"] = item
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("negated prerequisite", func(t *testing.T) {
		t.Parallel()
		for _, text := range []string{
			"No prerequisites are required.",
			"Prerequisites: none.",
			"Requirements: none.",
			"Prerequisite: N/A.",
			"Requirements: not applicable.",
		} {
			saved, evidence := publicationStructureFixture()
			item := evidence["prerequisite"]
			item.Excerpt = []string{text}
			item.StartLine = 13
			item.EndLine = 13
			evidence["prerequisite"] = item
			if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
				t.Fatalf("%q: AssessPathPublication().IssueCode = %q", text, issue)
			}
		}
	})

	t.Run("bare requirements label", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		item := evidence["prerequisite"]
		item.Label = "Requirements"
		item.Excerpt = []string{"Tool overview."}
		item.StartLine = 12
		item.EndLine = 12
		evidence["prerequisite"] = item
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("adjacent unrelated requirement", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		evidence["database"] = Evidence{
			ID: "database", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 12, EndLine: 12, Label: "Database setup",
			Excerpt: []string{"Before this database migration, install PostgreSQL."},
		}
		saved.PrerequisiteEvidenceIDs = []string{"database"}
		item := evidence["steps"]
		item.Label = "Run this cache tool"
		evidence["steps"] = item
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("overlapping generic prose is not a procedure identity", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		evidence["database"] = Evidence{
			ID: "database", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 13, EndLine: 13, Label: "Configure this local service",
			Excerpt: []string{"Before this database migration, install PostgreSQL."},
		}
		saved.PrerequisiteEvidenceIDs = []string{"database"}
		item := evidence["steps"]
		item.Label = "Run this local service"
		evidence["steps"] = item
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingPrerequisite {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})
}

func TestAssessPathPublicationRequiresContiguousRepositoryOrder(t *testing.T) {
	t.Parallel()

	t.Run("editorial ordering", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		saved.OrderingBasis = OrderingEditorial
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingActions {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("skipped command", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		item := evidence["steps"]
		item.Commands = []Command{
			{Value: "tool prepare", Basis: CommandExact},
			{Value: "tool configure", Basis: CommandExact},
			{Value: "tool run -o result.txt", Basis: CommandExact},
		}
		evidence["steps"] = item
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingActions {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("overlapping evidence cannot hide skipped command", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		first := evidence["steps"]
		first.ID = "steps-first"
		first.EndLine = 15
		first.Excerpt = []string{
			"tool prepare",
			"tool configure",
			"tool run -o result.txt",
		}
		first.Commands = []Command{
			{Value: "tool prepare", Basis: CommandExact},
			{Value: "tool configure", Basis: CommandExact},
			{Value: "tool run -o result.txt", Basis: CommandExact},
		}
		second := first
		second.ID = "steps-second"
		evidence[first.ID] = first
		evidence[second.ID] = second
		saved.Actions = []Action{
			{EvidenceID: first.ID, Command: "tool prepare"},
			{EvidenceID: second.ID, Command: "tool run -o result.txt"},
		}
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingActions {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("cross procedure", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		evidence["other-steps"] = Evidence{
			ID: "other-steps", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 40, EndLine: 40, Label: "Another procedure",
			Excerpt:  []string{"tool run -o result.txt"},
			Commands: []Command{{Value: "tool run -o result.txt", Basis: CommandExact}},
		}
		saved.Actions[1].EvidenceID = "other-steps"
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingActions {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})

	t.Run("ambiguous command and endpoint", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		saved.Actions[0].Endpoint = "http://127.0.0.1:8080/"
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingActions {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})
}

func TestAssessPathPublicationRequiresResultAfterFinalAction(t *testing.T) {
	t.Parallel()

	t.Run("complete result", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		assessment := AssessPathPublication(saved, evidence)
		if assessment.IssueCode != "" {
			t.Fatalf("AssessPathPublication().IssueCode = %q", assessment.IssueCode)
		}
		want := []PublicationResult{{
			Kind: PublicationResultGeneratedArtifact, Value: "result.txt",
			AfterAction: 2, EvidenceID: "steps", StartOffset: 1, EndOffset: 1,
		}}
		if !reflect.DeepEqual(assessment.Results, want) {
			t.Fatalf("AssessPathPublication().Results = %#v, want %#v", assessment.Results, want)
		}
	})

	t.Run("result before final action", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		saved.Actions = []Action{
			{EvidenceID: "steps", Command: "tool prepare -o result.txt"},
			{EvidenceID: "steps", Command: "tool run"},
		}
		item := evidence["steps"]
		item.Excerpt = []string{"tool prepare -o result.txt", "tool run"}
		item.Commands = []Command{
			{Value: "tool prepare -o result.txt", Basis: CommandExact},
			{Value: "tool run", Basis: CommandExact},
		}
		evidence["steps"] = item
		assessment := AssessPathPublication(saved, evidence)
		if assessment.IssueCode != PublicationIssueMissingResult {
			t.Fatalf("AssessPathPublication().IssueCode = %q", assessment.IssueCode)
		}
		if len(assessment.Results) != 1 || assessment.Results[0].AfterAction != 1 {
			t.Fatalf("AssessPathPublication().Results = %#v", assessment.Results)
		}
	})

	t.Run("generic expected evidence id", func(t *testing.T) {
		t.Parallel()
		saved, evidence := publicationStructureFixture()
		saved.Actions[1].Command = "tool run"
		saved.ExpectedEvidenceIDs = []string{"steps"}
		item := evidence["steps"]
		item.Excerpt = []string{"tool prepare", "tool run"}
		item.Commands[1].Value = "tool run"
		evidence["steps"] = item
		if issue := AssessPathPublication(saved, evidence).IssueCode; issue != PublicationIssueMissingResult {
			t.Fatalf("AssessPathPublication().IssueCode = %q", issue)
		}
	})
}

func TestBuildRecordAssessesPublicationBeforeDuplicateCompression(t *testing.T) {
	t.Parallel()

	bundle := publicationRecordBundle()
	complete := publicationRecordPath("Run the tool")
	incomplete := complete
	incomplete.PrerequisiteEvidenceIDs = nil

	var completeID string
	for _, test := range []struct {
		name          string
		paths         []ProposedPath
		rejectedIndex int
	}{
		{name: "incomplete first", paths: []ProposedPath{incomplete, complete}, rejectedIndex: 0},
		{name: "complete first", paths: []ProposedPath{complete, incomplete}, rejectedIndex: 1},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			record, err := BuildRecord(bundle, Proposal{
				Version: ProposalVersion,
				Paths:   test.paths,
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(record.Paths) != 1 ||
				len(record.Paths[0].PrerequisiteEvidenceIDs) != 1 {
				t.Fatalf("published paths = %#v", record.Paths)
			}
			wantIssues := []Issue{{
				PathIndex: test.rejectedIndex,
				Code:      PublicationIssueMissingPrerequisite,
			}}
			if !reflect.DeepEqual(record.Issues, wantIssues) {
				t.Fatalf("issues = %#v, want %#v", record.Issues, wantIssues)
			}
			if completeID == "" {
				completeID = record.Paths[0].ID
			} else if record.Paths[0].ID != completeID {
				t.Fatalf("complete path ID = %q, want %q", record.Paths[0].ID, completeID)
			}
		})
	}
}

func TestBuildRecordPublicationIssueOrderIsDeterministic(t *testing.T) {
	t.Parallel()

	bundle := publicationRecordBundle()
	missingPrerequisite := publicationRecordPath("Missing prerequisite")
	missingPrerequisite.PrerequisiteEvidenceIDs = nil
	missingActions := publicationRecordPath("Missing actions")
	missingActions.OrderingBasis = OrderingEditorial
	missingResult := ProposedPath{
		Title: "Missing result", Goal: "Run a check with no documented result.",
		PrerequisiteEvidenceIDs: []string{"prerequisite"},
		Actions: []ProposedAction{{
			EvidenceID: "no-result", Instruction: "Run the exact check.",
			Command: "tool inspect",
		}},
		OrderingBasis: OrderingDocumented,
	}
	proposal := Proposal{
		Version: ProposalVersion,
		Paths: []ProposedPath{
			missingPrerequisite,
			missingActions,
			missingResult,
		},
	}
	want := []Issue{
		{PathIndex: 0, Code: PublicationIssueMissingPrerequisite},
		{PathIndex: 1, Code: PublicationIssueMissingActions},
		{PathIndex: 2, Code: PublicationIssueMissingResult},
	}
	for attempt := 0; attempt < 2; attempt++ {
		record, err := BuildRecord(bundle, proposal, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(record.Paths) != 0 {
			t.Fatalf("published paths = %#v", record.Paths)
		}
		if !reflect.DeepEqual(record.Issues, want) {
			t.Fatalf("attempt %d issues = %#v, want %#v", attempt, record.Issues, want)
		}
	}
}

func TestBuildRecordLandmarkCopyPolicyDistinguishesRejectedOnlyAction(t *testing.T) {
	t.Parallel()

	bundle := publicationRecordBundle()
	complete := publicationRecordPath("Complete shared action")
	rejectedShared := complete
	rejectedShared.Title = "Rejected shared action"
	rejectedShared.PrerequisiteEvidenceIDs = nil
	rejectedOnly := ProposedPath{
		Title: "Rejected-only action", Goal: "Inspect without a complete procedure.",
		Actions: []ProposedAction{{
			EvidenceID: "no-result", Instruction: "Run the exact check.",
			Command: "tool inspect",
		}},
		OrderingBasis: OrderingDocumented,
	}
	record, err := BuildRecord(bundle, Proposal{
		Version: ProposalVersion,
		Paths: []ProposedPath{
			rejectedOnly,
			rejectedShared,
			complete,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	copyPolicy := make(map[string]bool, len(record.Landmarks))
	for _, landmark := range record.Landmarks {
		copyPolicy[landmark.Command] = landmark.SafeToCopy
	}
	rejectedOnlyCopy, ok := copyPolicy["tool inspect"]
	if !ok {
		t.Fatalf("rejected-only action landmark is missing: %#v", record.Landmarks)
	}
	if rejectedOnlyCopy {
		t.Fatalf("rejected-only action remained copyable: %#v", record.Landmarks)
	}
	sharedCopy, ok := copyPolicy["tool prepare"]
	if !ok {
		t.Fatalf("shared action landmark is missing: %#v", record.Landmarks)
	}
	if !sharedCopy {
		t.Fatalf("action shared with a complete path lost copyability: %#v", record.Landmarks)
	}
}

func TestDecodeRecordKeepsHistoricalIncompletePathReplayable(t *testing.T) {
	t.Parallel()

	bundle := testBundle()
	record, err := BuildRecord(bundle, Proposal{Version: ProposalVersion}, nil)
	if err != nil {
		t.Fatal(err)
	}
	evidence := make(map[string]Evidence, len(bundle.Evidence))
	for _, item := range bundle.Evidence {
		evidence[item.ID] = item
	}
	historical, code := validateProposedPath(ProposedPath{
		Title: "Build", Goal: "Build the local binary.",
		Actions: []ProposedAction{{
			EvidenceID: "ev-build", Instruction: "Use the repository build target.",
			Command: "make build",
		}},
		OrderingBasis: OrderingEditorial,
	}, evidence, nil)
	if code != "" {
		t.Fatalf("validateProposedPath() code = %q", code)
	}
	record.Paths = []Path{historical}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecord(raw)
	if err != nil {
		t.Fatalf("DecodeRecord() rejected historical incomplete path: %v", err)
	}
	if len(decoded.Paths) != 1 || decoded.Paths[0].ID != record.Paths[0].ID {
		t.Fatalf("decoded historical paths = %#v", decoded.Paths)
	}
}

func publicationRecordBundle() Bundle {
	return Bundle{
		Version: BundleVersion, RepoName: "fixture",
		AllowedPaths: []string{"README.md"},
		Evidence: []Evidence{
			{
				ID: "prerequisite", Role: RoleDocumentedProcedure, Path: "README.md",
				StartLine: 10, EndLine: 11, Label: "Run tool prerequisites",
				Excerpt: []string{
					"First install tool before running this procedure.",
					"tool prepare",
				},
			},
			{
				ID: "steps", Role: RoleDocumentedProcedure, Path: "README.md",
				StartLine: 11, EndLine: 12, Label: "Run tool",
				Excerpt: []string{"tool prepare", "tool run -o result.txt"},
				Commands: []Command{
					{Value: "tool prepare", Basis: CommandExact, SafeToCopy: true},
					{Value: "tool run -o result.txt", Basis: CommandExact, SafeToCopy: true},
				},
			},
			{
				ID: "no-result", Role: RoleDocumentedProcedure, Path: "README.md",
				StartLine: 11, EndLine: 11, Label: "Inspect tool",
				Excerpt: []string{"tool inspect"},
				Commands: []Command{{
					Value: "tool inspect", Basis: CommandExact, SafeToCopy: true,
				}},
			},
		},
	}
}

func publicationRecordPath(title string) ProposedPath {
	return ProposedPath{
		Title: title, Goal: "Run the complete documented procedure.",
		PrerequisiteEvidenceIDs: []string{"prerequisite"},
		Actions: []ProposedAction{
			{
				EvidenceID: "steps", Instruction: "Prepare the exact tool.",
				Command: "tool prepare",
			},
			{
				EvidenceID: "steps", Instruction: "Run the exact tool.",
				Command: "tool run -o result.txt",
			},
		},
		OrderingBasis: OrderingDocumented,
	}
}

func publicationStructureFixture() (Path, map[string]Evidence) {
	evidence := map[string]Evidence{
		"prerequisite": {
			ID: "prerequisite", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 10, EndLine: 13, Label: "Run tool",
			Excerpt: []string{
				"First install tool before running this procedure.",
				"",
				"Run tool",
				"tool prepare",
			},
		},
		"steps": {
			ID: "steps", Role: RoleDocumentedProcedure, Path: "README.md",
			StartLine: 13, EndLine: 14, Label: "Run tool",
			Excerpt: []string{"tool prepare", "tool run -o result.txt"},
			Commands: []Command{
				{Value: "tool prepare", Basis: CommandExact},
				{Value: "tool run -o result.txt", Basis: CommandExact},
			},
		},
	}
	return Path{
		PrerequisiteEvidenceIDs: []string{"prerequisite"},
		Actions: []Action{
			{EvidenceID: "steps", Command: "tool prepare"},
			{EvidenceID: "steps", Command: "tool run -o result.txt"},
		},
		OrderingBasis: OrderingDocumented,
	}, evidence
}
