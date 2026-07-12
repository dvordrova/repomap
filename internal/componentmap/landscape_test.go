package componentmap

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/evidence"
)

func TestApplyKeepsComponentIdentityAcrossRenameAndReorder(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	first, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{
			{
				Name: "Storage",
				Components: []ProposedComponent{
					{Name: "Repository", MemberIDs: []MemberID{testMemberID(MemberPackage, "repo"), testMemberID(MemberFile, "repo-file")}},
					{Name: "Backup", MemberIDs: []MemberID{testMemberID(MemberFlow, "backup-flow")}},
				},
			},
			{
				Name: "Interface",
				Components: []ProposedComponent{
					{Name: "CLI", MemberIDs: []MemberID{testMemberID(MemberEntrypoint, "backup-command")}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply(first) error = %v", err)
	}

	second, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{
			{
				Name: "Renamed interface",
				Components: []ProposedComponent{
					{Name: "Renamed CLI", MemberIDs: []MemberID{testMemberID(MemberEntrypoint, "backup-command")}},
				},
			},
			{
				Name: "Renamed storage",
				Components: []ProposedComponent{
					{Name: "Renamed backup", MemberIDs: []MemberID{testMemberID(MemberFlow, "backup-flow")}},
					{Name: "Renamed repository", MemberIDs: []MemberID{testMemberID(MemberFile, "repo-file"), testMemberID(MemberPackage, "repo")}},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply(second) error = %v", err)
	}

	firstIDs := componentIDByMembers(first)
	secondIDs := componentIDByMembers(second)
	if !reflect.DeepEqual(firstIDs, secondIDs) {
		t.Fatalf("component ids changed after rename/reorder:\nfirst:  %#v\nsecond: %#v", firstIDs, secondIDs)
	}
	if firstIDs["file:repo-file,package:repo"] == "" {
		t.Fatalf("component id is missing for the reordered two-member component: %#v", firstIDs)
	}
}

func TestApplyDropsUnknownMemberWithoutChangingLocalEvidence(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	wanted := bundle.Candidates[1]
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Storage",
			Components: []ProposedComponent{{
				Name: "Repository",
				MemberIDs: []MemberID{
					wanted.ID,
					testMemberID(MemberFile, "invented-by-provider"),
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback {
		t.Fatal("Apply() used fallback for a component with one surviving exact member")
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.unknown_member_dropped") {
		t.Fatalf("diagnostics = %#v, want unknown member diagnostic", result.Diagnostics)
	}
	got := result.Subsystems[0].Components[0].Members
	if len(got) != 1 || !reflect.DeepEqual(got[0], wanted) {
		t.Fatalf("accepted members = %#v, want unchanged candidate %#v", got, wanted)
	}
}

func TestApplyFallsBackForInvalidOrEmptyProposal(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	tests := []struct {
		name       string
		proposal   Proposal
		diagnostic string
	}{
		{
			name:       "empty proposal",
			proposal:   Proposal{Version: ContractVersion},
			diagnostic: "proposal.invalid_subsystem_count",
		},
		{
			name: "empty membership",
			proposal: Proposal{Version: ContractVersion, Subsystems: []ProposedSubsystem{{
				Name: "Repository", Components: []ProposedComponent{{Name: "Empty"}},
			}}},
			diagnostic: "proposal.invalid_component",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := Apply(bundle, test.proposal)
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if !result.Fallback || result.FallbackReason != "proposal_invalid_or_empty" {
				t.Fatalf("fallback = %v (%q), want explicit invalid proposal fallback", result.Fallback, result.FallbackReason)
			}
			if !hasLandscapeDiagnostic(result.Diagnostics, test.diagnostic) {
				t.Fatalf("diagnostics = %#v, want %q", result.Diagnostics, test.diagnostic)
			}
			if got := landscapeMemberCount(result); got != len(bundle.Candidates) {
				t.Fatalf("fallback members = %d, want all %d exact candidates", got, len(bundle.Candidates))
			}
		})
	}
}

func TestApplyKeepsFirstDuplicateMembershipAndPreservesRemainder(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	packageID := testMemberID(MemberPackage, "repo")
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Repository",
			Components: []ProposedComponent{
				{Name: "First", MemberIDs: []MemberID{packageID}},
				{Name: "Repeated", MemberIDs: []MemberID{packageID}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback {
		t.Fatalf("duplicate placement caused fallback: %#v", result.Diagnostics)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.duplicate_membership_dropped") ||
		!hasLandscapeDiagnostic(result.Diagnostics, "proposal.empty_membership_dropped") {
		t.Fatalf("diagnostics = %#v, want duplicate and empty-component repairs", result.Diagnostics)
	}
	if got := landscapeMemberCount(result); got != len(bundle.Candidates) {
		t.Fatalf("landscape members = %d, want all %d exact candidates", got, len(bundle.Candidates))
	}
}

func TestDeterministicDoesNotReportProviderFailure(t *testing.T) {
	t.Parallel()

	result, err := Deterministic(landscapeTestBundle(), FallbackModelDisabled)
	if err != nil {
		t.Fatalf("Deterministic() error = %v", err)
	}
	if !result.Fallback || result.FallbackReason != FallbackModelDisabled {
		t.Fatalf("fallback = %v (%q), want explicit model-disabled result", result.Fallback, result.FallbackReason)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, intentionally disabled model is not provider failure", result.Diagnostics)
	}
}

func TestDeterministicSupportsPathOnlyFactsAndNoStructuralRelations(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.Relations = nil
	bundle.Candidates[0].Facts[0].Location = &evidence.Location{Path: "cmd"}
	result, err := Deterministic(bundle, FallbackProviderUnconfigured)
	if err != nil {
		t.Fatalf("Deterministic() error = %v", err)
	}
	if result.Relations != nil {
		t.Fatalf("relations = %#v, want nil to preserve the saved contract", result.Relations)
	}
}

func TestApplyBoundsAndValidatesProposedMemberIDs(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	tests := []struct {
		name       string
		components []ProposedComponent
		memberIDs  []MemberID
		diagnostic string
	}{
		{
			name:       "too many references",
			memberIDs:  make([]MemberID, maxCandidates+1),
			diagnostic: "proposal.invalid_members",
		},
		{
			name:       "malformed opaque id",
			memberIDs:  []MemberID{{Kind: MemberFile, Value: "bad\nmember"}},
			diagnostic: "proposal.invalid_member_id",
		},
	}
	for index := range tests[0].memberIDs {
		tests[0].memberIDs[index] = MemberID{Kind: MemberFile, Value: "unknown"}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := test.components
			if components == nil {
				components = []ProposedComponent{{Name: "Invalid", MemberIDs: test.memberIDs}}
			}
			result, err := Apply(bundle, Proposal{
				Version: ContractVersion,
				Subsystems: []ProposedSubsystem{{
					Name:       "Invalid",
					Components: components,
				}},
			})
			if err != nil {
				t.Fatalf("Apply() error = %v", err)
			}
			if !result.Fallback || !hasLandscapeDiagnostic(result.Diagnostics, test.diagnostic) {
				t.Fatalf("result = %#v, want bounded fallback diagnostic %q", result, test.diagnostic)
			}
			for _, diagnostic := range result.Diagnostics {
				if diagnostic.Member != nil {
					t.Fatalf("malformed or oversized proposal leaked member into diagnostics: %#v", diagnostic)
				}
			}
		})
	}
}

func TestApplyBoundsExcessComponentsAndPreservesRemainder(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	packageID := testMemberID(MemberPackage, "repo")
	components := make([]ProposedComponent, maxComponents+1)
	for index := range components {
		components[index] = ProposedComponent{
			Name:      "Repeated placement",
			MemberIDs: []MemberID{packageID},
		}
	}
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name:       "Repository",
			Components: components,
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback {
		t.Fatalf("excess components caused fallback: %#v", result.Diagnostics)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.excess_components_dropped") {
		t.Fatalf("diagnostics = %#v, want bounded component repair", result.Diagnostics)
	}
	if got := landscapeMemberCount(result); got != len(bundle.Candidates) {
		t.Fatalf("landscape members = %d, want all %d exact candidates", got, len(bundle.Candidates))
	}
}

func TestBundleRequiresWitnessedFlowParticipation(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.Candidates[2].Participations[0].Evidence.Kind = FactRepositoryPath
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "flow-participation fact") {
		t.Fatalf("Validate() error = %v, want missing participation witness", err)
	}

	bundle = landscapeTestBundle()
	bundle.Candidates[2].Participations[0].Evidence.Certainty = evidence.CertaintyHypothesis
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "not locally grounded") {
		t.Fatalf("Validate() error = %v, want tentative participation rejection", err)
	}
}

func TestLandscapePreservesTypedStructuralRelations(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	result, err := Deterministic(bundle, FallbackProviderUnconfigured)
	if err != nil {
		t.Fatalf("Deterministic() error = %v", err)
	}
	if !reflect.DeepEqual(result.Relations, bundle.Relations) {
		t.Fatalf("relations changed:\nresult=%#v\nbundle=%#v", result.Relations, bundle.Relations)
	}
	result.Relations[0].To = result.Relations[0].From
	if err := result.Validate(bundle); err == nil || !strings.Contains(err.Error(), "changed local structural relations") {
		t.Fatalf("Validate(mutated relation) error = %v", err)
	}
}

func TestBundleRejectsConflictingScenarioDefinitions(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	conflict := bundle.Relations[0]
	conflict.ID = "repo-imports-cmd"
	conflict.From, conflict.To = conflict.To, conflict.From
	conflict.Scenarios = []ScenarioContext{{
		ID: "go-default", Name: "Different build",
		Build: evidence.BuildContext{GOOS: "linux", GOARCH: "amd64"},
	}}
	bundle.Relations = append(bundle.Relations, conflict)
	if err := bundle.Validate(); err == nil || !strings.Contains(err.Error(), "conflicting definitions") {
		t.Fatalf("Validate() error = %v, want scenario identity conflict", err)
	}
}

func TestApplyPreservesCandidatesOmittedByProposal(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Storage",
			Components: []ProposedComponent{{
				Name:      "Repository",
				MemberIDs: []MemberID{testMemberID(MemberPackage, "repo")},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback {
		t.Fatal("Apply() replaced a usable partial proposal with the full fallback")
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.omitted_members_preserved") {
		t.Fatalf("diagnostics = %#v, want omitted-members diagnostic", result.Diagnostics)
	}
	if got := landscapeMemberCount(result); got != len(bundle.Candidates) {
		t.Fatalf("landscape members = %d, want all %d exact candidates", got, len(bundle.Candidates))
	}
	last := result.Subsystems[len(result.Subsystems)-1]
	if last.Name != "Unassigned local evidence" || len(last.Components) != 1 {
		t.Fatalf("deterministic remainder = %#v", last)
	}
}

func TestFallbackIsDeterministicAcrossCandidateOrder(t *testing.T) {
	t.Parallel()

	firstBundle := landscapeTestBundle()
	secondBundle := landscapeTestBundle()
	for left, right := 0, len(secondBundle.Candidates)-1; left < right; left, right = left+1, right-1 {
		secondBundle.Candidates[left], secondBundle.Candidates[right] = secondBundle.Candidates[right], secondBundle.Candidates[left]
	}

	invalid := Proposal{Version: ContractVersion}
	first, err := Apply(firstBundle, invalid)
	if err != nil {
		t.Fatalf("Apply(first) error = %v", err)
	}
	second, err := Apply(secondBundle, invalid)
	if err != nil {
		t.Fatalf("Apply(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("fallback depends on candidate order:\nfirst:  %#v\nsecond: %#v", first, second)
	}
}

func landscapeTestBundle() CandidateBundle {
	packageID := testMemberID(MemberPackage, "repo")
	commandPackageID := testMemberID(MemberPackage, "cmd")
	entrypointID := testMemberID(MemberEntrypoint, "backup-command")
	flowID := FlowID("backup")
	return CandidateBundle{
		Version: ContractVersion,
		Flows: []Flow{{
			ID: flowID, Name: "Backup",
			Facts: []LocalFact{testLocalFact(FactDeclaration, "saved flowproof v2", "cmd/backup.go", 20)},
		}},
		Candidates: []Candidate{
			{
				ID: commandPackageID, Name: "command package",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/example/cmd", "cmd/main.go", 1)},
			},
			{
				ID: packageID, Name: "repository package",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/example/repository", "repository.go", 1)},
			},
			{
				ID: testMemberID(MemberFile, "repo-file"), Name: "repository.go", ParentID: &packageID,
				Participations: []FlowParticipation{testFlowParticipation(flowID, "repository.go", 1)},
				Facts:          []LocalFact{testLocalFact(FactRepositoryPath, "repository.go", "repository.go", 1)},
			},
			{
				ID: entrypointID, Name: "backup command", ParentID: &commandPackageID,
				Participations: []FlowParticipation{testFlowParticipation(flowID, "cmd/backup.go", 20)},
				Facts:          []LocalFact{testLocalFact(FactDeclaration, "runBackup", "cmd/backup.go", 20)},
			},
			{
				ID: testMemberID(MemberFlow, "backup-flow"), Name: "backup",
				Participations: []FlowParticipation{testFlowParticipation(flowID, "cmd/backup.go", 20)},
				Facts:          []LocalFact{testLocalFact(FactFlowParticipation, "backup", "cmd/backup.go", 20)},
			},
		},
		Relations: []LocalRelation{{
			ID: "cmd-imports-repo", From: commandPackageID, To: packageID,
			Kind: StructuralRelationPackageImport, Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider: "go_list", Version: "fixture-v1", Operation: "list_package_imports",
			}},
			Scenarios: []ScenarioContext{{
				ID: "go-default", Name: "Default Go build",
				Build: evidence.BuildContext{GOOS: "darwin", GOARCH: "amd64"},
			}},
		}},
		AnchorBindings: []FlowAnchorBinding{{
			FlowID: flowID, AnchorID: "run-backup", MemberID: entrypointID,
			Location:  &evidence.Location{Path: "cmd/backup.go", Line: 20, Column: 1},
			Certainty: evidence.CertaintyStatic,
			Provenance: []evidence.Provenance{{
				Provider: "fixture", Version: "v1", Operation: "bind_flow_anchor",
			}},
		}},
	}
}

func testFlowParticipation(flowID FlowID, path string, line int) FlowParticipation {
	return FlowParticipation{
		FlowID:   flowID,
		Evidence: testLocalFact(FactFlowParticipation, string(flowID), path, line),
	}
}

func testMemberID(kind MemberKind, value string) MemberID {
	return MemberID{Kind: kind, Value: value}
}

func testLocalFact(kind FactKind, value, path string, line int) LocalFact {
	return LocalFact{
		Kind: kind, Value: value,
		Location:  &evidence.Location{Path: path, Line: line, Column: 1},
		Certainty: evidence.CertaintyStatic,
		Provenance: []evidence.Provenance{{
			Provider: "fixture", Version: "v1", Operation: "local_fact",
		}},
	}
}

func componentIDByMembers(landscape Landscape) map[string]ComponentID {
	result := make(map[string]ComponentID)
	for _, subsystem := range landscape.Subsystems {
		for _, component := range subsystem.Components {
			keys := make([]string, len(component.Members))
			for index, member := range component.Members {
				keys[index] = string(member.ID.Kind) + ":" + member.ID.Value
			}
			sort.Strings(keys)
			result[strings.Join(keys, ",")] = component.ID
		}
	}
	return result
}

func hasLandscapeDiagnostic(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func landscapeMemberCount(landscape Landscape) int {
	total := 0
	for _, subsystem := range landscape.Subsystems {
		for _, component := range subsystem.Components {
			total += len(component.Members)
		}
	}
	return total
}
