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
	packageID := testMemberID(MemberPackage, "repo")
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
		{
			name: "duplicate membership",
			proposal: Proposal{Version: ContractVersion, Subsystems: []ProposedSubsystem{{
				Name: "Repository",
				Components: []ProposedComponent{
					{Name: "First", MemberIDs: []MemberID{packageID}},
					{Name: "Second", MemberIDs: []MemberID{packageID}},
				},
			}}},
			diagnostic: "proposal.duplicate_membership",
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
				ID: packageID, Name: "repository package",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/example/repository", "repository.go", 1)},
			},
			{
				ID: testMemberID(MemberFile, "repo-file"), Name: "repository.go", ParentID: &packageID, FlowIDs: []FlowID{flowID},
				Facts: []LocalFact{testLocalFact(FactRepositoryPath, "repository.go", "repository.go", 1)},
			},
			{
				ID: entrypointID, Name: "backup command", FlowIDs: []FlowID{flowID},
				Facts: []LocalFact{testLocalFact(FactDeclaration, "runBackup", "cmd/backup.go", 20)},
			},
			{
				ID: testMemberID(MemberFlow, "backup-flow"), Name: "backup", FlowIDs: []FlowID{flowID},
				Facts: []LocalFact{testLocalFact(FactFlowParticipation, "backup", "cmd/backup.go", 20)},
			},
		},
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
