package componentmap

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
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
					{Name: "CLI", MemberIDs: []MemberID{
						testMemberID(MemberPackage, "cmd"),
						testMemberID(MemberEntrypoint, "backup-command"),
					}},
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
					{Name: "Renamed CLI", MemberIDs: []MemberID{
						testMemberID(MemberEntrypoint, "backup-command"),
						testMemberID(MemberPackage, "cmd"),
					}},
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
	if !reflect.DeepEqual(first.ConceptualMemberships, second.ConceptualMemberships) {
		t.Fatalf("canonical conceptual memberships changed after reorder:\nfirst:  %#v\nsecond: %#v", first.ConceptualMemberships, second.ConceptualMemberships)
	}
}

func TestApplyRejectsUnknownMemberWithoutChangingLocalEvidence(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Storage",
			Components: []ProposedComponent{{
				Name: "Repository",
				MemberIDs: []MemberID{
					bundle.Candidates[1].ID,
					testMemberID(MemberFile, "invented-by-provider"),
				},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if !result.Fallback || result.ValidationOutcome != ValidationRejected || result.FallbackReason != FallbackRejectedUnknownMember {
		t.Fatalf("result = %#v, want fatal unknown-member rejection", result)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.unknown_member_id") {
		t.Fatalf("diagnostics = %#v, want unknown member diagnostic", result.Diagnostics)
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
			if !result.Fallback || result.FallbackReason != FallbackRejectedMalformed {
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

// Decision 227: a unit may participate in several components when the
// components express different conceptual roles (participation, not
// exclusive ownership). Sharing the same exact member set under different
// names/descriptions is accepted.
func TestApplyAcceptsCrossCuttingParticipation(t *testing.T) {
	t.Parallel()

	bundle := candidateBundleWithPackages(3)
	ids := candidateIDs(bundle.Candidates)
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Casdoor-like",
			Components: []ProposedComponent{
				{Name: "Server", MemberIDs: ids[:1]},
				{Name: "Certificates", MemberIDs: ids[:1]},
				{Name: "Lifecycle providers", MemberIDs: ids[:1]},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback || result.ValidationOutcome == ValidationRejected {
		t.Fatalf("cross-cutting participation was rejected: %#v", result.Diagnostics)
	}
	if len(result.Subsystems[0].Components) != 3 {
		t.Fatalf("components = %d, want 3 (all roles preserved)", len(result.Subsystems[0].Components))
	}
}

// Decision 227 + Decision 229 D7 D4: an exact twin — same name, description,
// member set AND anchor set — is a literal copy with no added knowledge. It
// is skipped item-scope (equivalent collision affects only its equivalence
// class); the valid first component and unrelated components publish.
func TestApplyRejectsExactTwinComponents(t *testing.T) {
	t.Parallel()

	bundle := candidateBundleWithPackages(3)
	ids := candidateIDs(bundle.Candidates)
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Repository",
			Components: []ProposedComponent{
				{Name: "Same role", Description: "same responsibility", MemberIDs: ids[:2]},
				{Name: "Same role", Description: "same responsibility", MemberIDs: ids[:2]},
				{Name: "Unrelated", Description: "a different role", MemberIDs: ids[2:]},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("exact twin must publish accepted_partial with valid siblings: %#v", result.Diagnostics)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.duplicate_component_identity") {
		t.Fatalf("diagnostics = %#v, want duplicate_component_identity counted item-scope", result.Diagnostics)
	}
	// The twin is skipped; the valid first component and the unrelated
	// component publish (2 components, not 3, not 1).
	if len(result.Subsystems[0].Components) != 2 {
		t.Fatalf("components = %d, want 2 (twin skipped item-scope, valid siblings publish)", len(result.Subsystems[0].Components))
	}
}

func TestPackageLandscapeIgnoresProviderGroundingClaim(t *testing.T) {
	t.Parallel()

	bundle := candidateBundleWithPackages(2)
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Packages",
			Components: []ProposedComponent{{
				Name:      "Provider claimed grounded",
				MemberIDs: candidateIDs(bundle.Candidates),
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAccepted ||
		result.Source != SourceValidatedModel || len(result.Normalizations) != 0 ||
		!result.Subsystems[0].Components[0].Hypothesis {
		t.Fatalf("package landscape trusted provider operational status: %#v", result)
	}
}

func TestApplyAcceptsManyToManyConceptualMembership(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	packageID := testMemberID(MemberPackage, "repo")
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Repository",
			Components: []ProposedComponent{
				{Name: "First", MemberIDs: candidateIDs(bundle.Candidates)},
				{Name: "Repeated", MemberIDs: []MemberID{packageID}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback || result.ValidationOutcome == ValidationRejected {
		t.Fatalf("many-to-many placement result = %#v", result)
	}
	count := 0
	for _, membership := range result.ConceptualMemberships {
		if membership.MemberID == packageID {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("shared member relation count = %d, want 2: %#v", count, result.ConceptualMemberships)
	}
}

// Decision 229 D7 D1: a repeated member within one component normalizes
// locally — the first occurrence wins, the component survives.
func TestApplyRejectsDuplicateMemberWithinOneComponent(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	memberID := bundle.Candidates[0].ID
	otherID := bundle.Candidates[1].ID
	result, err := Apply(bundle, Proposal{
		Version: ProposalVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Repository",
			Components: []ProposedComponent{{
				Name: "Repeated", MemberIDs: []MemberID{memberID, memberID, otherID},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("duplicate member must normalize locally (accepted_partial), not reject: %#v", result.Diagnostics)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.duplicate_member_id") {
		t.Fatalf("diagnostics = %#v, want duplicate_member_id counted", result.Diagnostics)
	}
	// The component survives with the first occurrence plus the distinct
	// sibling member.
	if len(result.Subsystems[0].Components[0].Members) != 2 {
		t.Fatalf("component members = %d, want 2 (duplicate normalized locally)", len(result.Subsystems[0].Components[0].Members))
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

func TestCanonicalBuildsStableLocalLandscapeWithoutFallbackState(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	first, err := Canonical(bundle)
	if err != nil {
		t.Fatalf("Canonical(first) error = %v", err)
	}
	second, err := Canonical(bundle)
	if err != nil {
		t.Fatalf("Canonical(second) error = %v", err)
	}

	if first.Fallback || first.FallbackReason != "" {
		t.Fatalf("canonical fallback = %v (%q), want primary local landscape", first.Fallback, first.FallbackReason)
	}
	if first.Source != SourceLocalPackages || first.ValidationOutcome != ValidationAccepted {
		t.Fatalf("canonical source/outcome = %q/%q", first.Source, first.ValidationOutcome)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("canonical landscape is not deterministic:\nfirst:  %#v\nsecond: %#v", first, second)
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

func TestApplyBoundsManyToManyMembershipIndependentlyFromCandidates(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	memberID := bundle.Candidates[0].ID
	subsystems := make([]ProposedSubsystem, maxConceptualMembershipsPerMember+1)
	for index := range subsystems {
		subsystems[index] = ProposedSubsystem{
			Name: fmt.Sprintf("Cross-cut %d", index),
			Components: []ProposedComponent{{
				Name: fmt.Sprintf("Participation %d", index), MemberIDs: []MemberID{memberID},
			}},
		}
	}
	result, err := Apply(bundle, Proposal{
		Version: ProposalVersion, Subsystems: subsystems,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback || !hasLandscapeDiagnostic(result.Diagnostics, "proposal.member_participation_limit_exceeded") {
		t.Fatalf("per-member conceptual bound did not fail closed: %#v", result)
	}

	large := landscapeTestBundle()
	large.Candidates = nil
	large.BehaviorAnchors = nil
	large.Relations = nil
	large.AnchorBindings = nil
	for index := 0; index < maxCandidates; index++ {
		id := MemberID{Kind: MemberFile, Value: fmt.Sprintf("bounded-member-%03d", index)}
		large.Candidates = append(large.Candidates, Candidate{
			ID: id, Role: CandidateRoleConceptualMember, Name: fmt.Sprintf("member-%03d.go", index),
			Facts: []LocalFact{testLocalFact(FactRepositoryPath, fmt.Sprintf("member-%03d.go", index), fmt.Sprintf("member-%03d.go", index), 1)},
		})
	}
	components := make([]ProposedComponent, 5)
	for index := range components {
		members := make([]MemberID, len(large.Candidates))
		for candidateIndex, candidate := range large.Candidates {
			members[candidateIndex] = candidate.ID
		}
		components[index] = ProposedComponent{Name: fmt.Sprintf("Cross-cut %d", index), MemberIDs: members}
	}
	components[len(components)-1].MemberIDs = components[len(components)-1].MemberIDs[:1]
	result, err = Apply(large, Proposal{
		Version:    ProposalVersion,
		Subsystems: []ProposedSubsystem{{Name: "Repository", Components: components}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback || !hasLandscapeDiagnostic(result.Diagnostics, "proposal.membership_limit_exceeded") {
		t.Fatalf("total conceptual membership bound did not fail closed: %#v", result.Diagnostics)
	}
}

func TestApplyRejectsRawParticipationLimitBeforeComponentNormalization(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	memberID := bundle.Candidates[0].ID
	components := make([]ProposedComponent, maxConceptualMembershipsPerMember+1)
	for index := range components {
		components[index] = ProposedComponent{
			Name:      fmt.Sprintf("Cross-cut %d", index),
			MemberIDs: []MemberID{memberID},
		}
	}
	result, err := Apply(bundle, Proposal{
		Version: ProposalVersion,
		Subsystems: []ProposedSubsystem{{
			Name:       "Repository",
			Components: components,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Fallback || result.ValidationOutcome != ValidationRejected ||
		!hasLandscapeDiagnostic(result.Diagnostics, "proposal.member_participation_limit_exceeded") {
		t.Fatalf("raw per-member limit was hidden by component normalization: %#v", result)
	}
	if len(result.Normalizations) != 0 {
		t.Fatalf("rejected raw membership unexpectedly recorded normalization: %#v", result.Normalizations)
	}
}

func TestApplyBoundsExcessComponentsAndPreservesRemainder(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.Candidates = nil
	bundle.BehaviorAnchors = nil
	bundle.Relations = nil
	bundle.AnchorBindings = nil
	components := make([]ProposedComponent, maxComponents+1)
	for index := range components {
		id := testMemberID(MemberFile, fmt.Sprintf("member-%02d", index))
		bundle.Candidates = append(bundle.Candidates, Candidate{
			ID: id, Role: CandidateRoleConceptualMember, Name: fmt.Sprintf("member-%02d.go", index),
			Facts: []LocalFact{testLocalFact(FactRepositoryPath, fmt.Sprintf("member-%02d.go", index), fmt.Sprintf("member-%02d.go", index), 1)},
		})
		components[index] = ProposedComponent{
			Name:      fmt.Sprintf("Responsibility %02d", index),
			MemberIDs: []MemberID{id},
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
	if result.ValidationOutcome != ValidationAcceptedNormalized || result.Source != SourceNormalizedModel {
		t.Fatalf("validation outcome = %q source = %q", result.ValidationOutcome, result.Source)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.normalized_components_per_subsystem") {
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

func TestCandidateBundleNumericLimitBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		kind       CandidateBundleLimitKind
		limit      int
		makeBundle func(int) CandidateBundle
	}{
		{
			name: "candidates", kind: CandidateBundleLimitCandidates,
			limit: maxCandidates, makeBundle: candidateBundleWithPackages,
		},
		{
			name: "relations", kind: CandidateBundleLimitRelations,
			limit: maxRelations, makeBundle: candidateBundleWithRelations,
		},
		{
			name: "behavior anchors", kind: CandidateBundleLimitBehaviorAnchors,
			limit: maxBehaviorAnchors, makeBundle: candidateBundleWithBehaviorAnchors,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.makeBundle(test.limit).Validate(); err != nil {
				t.Fatalf("Validate(N=%d) error = %v", test.limit, err)
			}
			err := test.makeBundle(test.limit + 1).Validate()
			var limitErr *CandidateBundleLimitError
			if !errors.As(err, &limitErr) {
				t.Fatalf("Validate(N+1=%d) error = %v, want CandidateBundleLimitError", test.limit+1, err)
			}
			if limitErr.Kind != test.kind || limitErr.Observed != test.limit+1 || limitErr.Limit != test.limit {
				t.Fatalf("limit error = %#v", limitErr)
			}
		})
	}
}

func TestCandidateBundleRemainingNumericLimitsAreTyped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		kind     CandidateBundleLimitKind
		observed int
		limit    int
		mutate   func(*CandidateBundle)
	}{
		{
			name: "flows", kind: CandidateBundleLimitFlows,
			observed: maxFlows + 1, limit: maxFlows,
			mutate: func(bundle *CandidateBundle) { bundle.Flows = make([]Flow, maxFlows+1) },
		},
		{
			name: "anchor bindings", kind: CandidateBundleLimitAnchorBindings,
			observed: maxAnchorBindings + 1, limit: maxAnchorBindings,
			mutate: func(bundle *CandidateBundle) {
				bundle.AnchorBindings = make([]FlowAnchorBinding, maxAnchorBindings+1)
			},
		},
		{
			name: "research findings", kind: CandidateBundleLimitResearchFindings,
			observed: maxResearchFindings + 1, limit: maxResearchFindings,
			mutate: func(bundle *CandidateBundle) {
				bundle.ResearchFindings = make([]ResearchInterpretation, maxResearchFindings+1)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			bundle := candidateBundleWithPackages(1)
			test.mutate(&bundle)
			err := bundle.Validate()
			var limitErr *CandidateBundleLimitError
			if !errors.As(err, &limitErr) {
				t.Fatalf("Validate() error = %v, want CandidateBundleLimitError", err)
			}
			if limitErr.Kind != test.kind || limitErr.Observed != test.observed || limitErr.Limit != test.limit {
				t.Fatalf("limit error = %#v", limitErr)
			}
		})
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

func TestApplyAcceptsExactPartialCoverageWithDeterministicLocalRemainder(t *testing.T) {
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
	if result.Fallback || result.ValidationOutcome != ValidationAcceptedPartial || result.Source != SourcePartialModel {
		t.Fatalf("partial proposal result = %#v, want accepted exact partial landscape", result)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.partial_member_coverage") {
		t.Fatalf("diagnostics = %#v, want partial-coverage diagnostic", result.Diagnostics)
	}
	if got := landscapeMemberCount(result); got != len(bundle.Candidates) {
		t.Fatalf("local fallback members = %d, want all %d exact candidates", got, len(bundle.Candidates))
	}
	wantRemainder := candidateIDsExcept(bundle, testMemberID(MemberPackage, "repo"))
	sortMemberIDs(wantRemainder)
	if !reflect.DeepEqual(result.LocalRemainderMemberIDs, wantRemainder) {
		t.Fatalf("local remainder = %#v, want %#v", result.LocalRemainderMemberIDs, wantRemainder)
	}
	if len(result.ConceptualMemberships) != 1 || result.ConceptualMemberships[0].MemberID != testMemberID(MemberPackage, "repo") {
		t.Fatalf("provider conceptual memberships include local remainder: %#v", result.ConceptualMemberships)
	}
	tampered := result
	tampered.ConceptualMemberships = append(tampered.ConceptualMemberships, ConceptualMembership{
		ComponentID: tampered.Subsystems[len(tampered.Subsystems)-1].Components[0].ID,
		MemberID:    tampered.LocalRemainderMemberIDs[0],
	})
	if err := tampered.Validate(bundle); err == nil || !strings.Contains(err.Error(), "conceptual membership relation") {
		t.Fatalf("Validate(remainder promoted to provider relation) error = %v", err)
	}

	reorderedBundle := landscapeTestBundle()
	for left, right := 0, len(reorderedBundle.Candidates)-1; left < right; left, right = left+1, right-1 {
		reorderedBundle.Candidates[left], reorderedBundle.Candidates[right] = reorderedBundle.Candidates[right], reorderedBundle.Candidates[left]
	}
	reordered, err := Apply(reorderedBundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Storage",
			Components: []ProposedComponent{{
				Name: "Repository", MemberIDs: []MemberID{testMemberID(MemberPackage, "repo")},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reordered.LocalRemainderMemberIDs, result.LocalRemainderMemberIDs) ||
		reordered.Subsystems[len(reordered.Subsystems)-1].ID != result.Subsystems[len(result.Subsystems)-1].ID {
		t.Fatalf("partial remainder changed with candidate order:\nfirst=%#v\nsecond=%#v", result, reordered)
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

func TestProcessEntryFallbackSeparatesExecutableRoles(t *testing.T) {
	t.Parallel()

	appPackage := MemberID{Kind: MemberPackage, Value: "app-package"}
	appFile := MemberID{Kind: MemberFile, Value: "app-file"}
	appSymbol := MemberID{Kind: MemberSymbol, Value: "app-symbol"}
	toolPackage := MemberID{Kind: MemberPackage, Value: "tool-package"}
	toolFile := MemberID{Kind: MemberFile, Value: "tool-file"}
	toolSymbol := MemberID{Kind: MemberSymbol, Value: "tool-symbol"}
	servicePackage := MemberID{Kind: MemberPackage, Value: "service-package"}
	serviceFile := MemberID{Kind: MemberFile, Value: "service-file"}
	serviceSymbol := MemberID{Kind: MemberSymbol, Value: "service-symbol"}
	testPackage := MemberID{Kind: MemberPackage, Value: "test-package"}
	testFile := MemberID{Kind: MemberFile, Value: "test-file"}
	testSymbol := MemberID{Kind: MemberSymbol, Value: "test-symbol"}
	known := map[MemberID]Candidate{
		appPackage: {ID: appPackage, Role: CandidateRoleConceptualMember, Name: "cmd/project"},
		appFile:    {ID: appFile, Role: CandidateRoleConceptualMember, Name: "cmd/project/main.go", ParentID: &appPackage},
		appSymbol: {
			ID: appSymbol, Role: CandidateRoleConceptualMember, Name: "example.com/project/v2/app.main", ParentID: &appFile,
			Facts: []LocalFact{
				{Kind: FactDeclaration, Value: "example.com/project/v2/app.main"},
				{Kind: FactExecutableRole, Value: "primary_application"},
			},
		},
		servicePackage: {ID: servicePackage, Role: CandidateRoleConceptualMember, Name: "cmd/server"},
		serviceFile:    {ID: serviceFile, Role: CandidateRoleConceptualMember, Name: "cmd/server/main.go", ParentID: &servicePackage, Participations: []FlowParticipation{{FlowID: "serve"}}},
		serviceSymbol: {
			ID: serviceSymbol, Role: CandidateRoleConceptualMember, Name: "example.com/project/cmd/server.main", ParentID: &serviceFile,
			Facts: []LocalFact{
				{Kind: FactDeclaration, Value: "example.com/project/cmd/server.main"},
				{Kind: FactExecutableRole, Value: "secondary_service"},
			},
		},
		toolPackage: {ID: toolPackage, Role: CandidateRoleConceptualMember, Name: "cmd/inspect"},
		toolFile:    {ID: toolFile, Role: CandidateRoleConceptualMember, Name: "cmd/inspect/main.go", ParentID: &toolPackage},
		toolSymbol: {
			ID: toolSymbol, Role: CandidateRoleConceptualMember, Name: "example.com/project/cmd/inspect.main", ParentID: &toolFile,
			Facts: []LocalFact{
				{Kind: FactDeclaration, Value: "example.com/project/cmd/inspect.main"},
				{Kind: FactExecutableRole, Value: "tooling"},
			},
		},
		testPackage: {ID: testPackage, Role: CandidateRoleConceptualMember, Name: "cmd/helper"},
		testFile:    {ID: testFile, Role: CandidateRoleConceptualMember, Name: "cmd/helper/main.go", ParentID: &testPackage},
		testSymbol: {
			ID: testSymbol, Role: CandidateRoleConceptualMember, Name: "example.com/project/cmd/helper.main", ParentID: &testFile,
			Facts: []LocalFact{
				{Kind: FactDeclaration, Value: "example.com/project/cmd/helper.main"},
				{Kind: FactExecutableRole, Value: "test_or_helper"},
			},
		},
	}
	components := processEntryLocalComponents([]BehaviorAnchor{
		{ID: "app-entry", Kind: AnchorProcessEntry, ProofMode: AnchorProofProcessEntry, MemberIDs: []MemberID{appSymbol}},
		{ID: "service-entry", Kind: AnchorProcessEntry, ProofMode: AnchorProofProcessEntry, MemberIDs: []MemberID{serviceSymbol}},
		{ID: "tool-entry", Kind: AnchorProcessEntry, ProofMode: AnchorProofProcessEntry, MemberIDs: []MemberID{toolSymbol}},
		{ID: "test-entry", Kind: AnchorProcessEntry, ProofMode: AnchorProofProcessEntry, MemberIDs: []MemberID{testSymbol}},
	}, known, make(map[MemberID]struct{}))
	if len(components) != 4 || components[0].Name != "Primary application" ||
		components[1].Name != "Secondary services" || components[2].Name != "Tool entrypoints" ||
		components[3].Name != "Test and helper entrypoints" {
		t.Fatalf("process-entry components = %#v", components)
	}
	for _, member := range components[0].Members {
		if strings.Contains(member.Name, "server") || strings.Contains(member.Name, "dev/") {
			t.Fatalf("Primary application includes non-primary member %q", member.Name)
		}
	}
}

func TestModuleBaseNameIgnoresSemanticImportVersion(t *testing.T) {
	t.Parallel()

	for modulePath, want := range map[string]string{
		"github.com/example/project":      "project",
		"github.com/example/project/v2":   "project",
		"github.com/example/project/v10":  "project",
		"github.com/example/project/view": "view",
	} {
		if got := moduleBaseName(modulePath); got != want {
			t.Errorf("moduleBaseName(%q) = %q, want %q", modulePath, got, want)
		}
	}
}

func TestApplyRetainsOmittedProcessEntryInExactLocalRemainder(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.GroundingMode = GroundingMixed
	entrypointID := testMemberID(MemberEntrypoint, "backup-command")
	bundle.BehaviorAnchors = []BehaviorAnchor{{
		ID: "process", Kind: AnchorProcessEntry, Label: "process entry",
		ProofMode: AnchorProofProcessEntry,
		Location:  evidence.Location{Path: "cmd/backup.go", Line: 20, Column: 1},
		Scenario:  ScenarioContext{ID: "go:test", Name: "test build"},
		Producer: evidence.Provenance{
			Provider: "fixture", Version: "v1", Operation: "process_entry",
		},
		Certainty: evidence.CertaintyStatic, MemberIDs: []MemberID{entrypointID},
		Limitations: []string{"execution is not observed"},
	}}
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Runtime",
			Components: []ProposedComponent{{
				Name:      "Command package",
				MemberIDs: []MemberID{testMemberID(MemberPackage, "cmd")},
				AnchorIDs: []string{"process"},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAcceptedPartial ||
		!hasLandscapeDiagnostic(result.Diagnostics, "proposal.partial_member_coverage") {
		t.Fatalf("process-entry omission result = %#v", result)
	}
	if !slices.Contains(result.LocalRemainderMemberIDs, entrypointID) {
		t.Fatalf("process entry %q missing from exact local remainder %#v", entrypointID.key(), result.LocalRemainderMemberIDs)
	}
	if got := landscapeMemberCount(result); got != len(bundle.Candidates) {
		t.Fatalf("landscape members = %d, want all %d exact candidates", got, len(bundle.Candidates))
	}
}

func TestApplyDerivesKnownPackageOnlyComponentHypothesisLocally(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.GroundingMode = GroundingMixed
	bundle.BehaviorAnchors = []BehaviorAnchor{{
		ID: "process", Kind: AnchorProcessEntry, Label: "process entry",
		ProofMode:   AnchorProofProcessEntry,
		Location:    evidence.Location{Path: "cmd/backup.go", Line: 20, Column: 1},
		Scenario:    ScenarioContext{ID: "go:test", Name: "test build"},
		Producer:    evidence.Provenance{Provider: "fixture", Version: "v1", Operation: "process_entry"},
		Certainty:   evidence.CertaintyStatic,
		MemberIDs:   []MemberID{testMemberID(MemberEntrypoint, "backup-command")},
		Limitations: []string{"execution is not observed"},
	}}
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Runtime",
			Components: []ProposedComponent{
				{
					Name:      "Command",
					MemberIDs: []MemberID{testMemberID(MemberEntrypoint, "backup-command")},
					AnchorIDs: []string{"process"},
				},
				{
					Name:      "Repository support",
					MemberIDs: []MemberID{testMemberID(MemberPackage, "repo")},
				},
				{
					Name: "Other exact members", MemberIDs: candidateIDsExcept(
						bundle,
						testMemberID(MemberEntrypoint, "backup-command"),
						testMemberID(MemberPackage, "repo"),
					),
					Hypothesis: true,
				},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAccepted ||
		result.Source != SourceValidatedModel || len(result.Normalizations) != 0 {
		t.Fatalf("locally resolved result = %#v", result)
	}
	var found bool
	for _, subsystem := range result.Subsystems {
		for _, component := range subsystem.Components {
			if component.Name == "Repository support" {
				found = component.Hypothesis && len(component.AnchorIDs) == 0
			}
		}
	}
	if !found {
		t.Fatal("package-only component was not retained as an explicit hypothesis")
	}
}

func TestDeclarationFamilyAnchorIsStaticContextNotOperationalGrounding(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.GroundingMode = GroundingMixed
	family := declarationFamilyTestAnchor(testMemberID(MemberPackage, "repo"))
	bundle.BehaviorAnchors = []BehaviorAnchor{family}
	proposal := Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Runtime",
			Components: []ProposedComponent{{
				Name: "All repository responsibilities", MemberIDs: candidateIDs(bundle.Candidates),
				AnchorIDs: []string{family.ID},
			}},
		}},
	}

	derived, err := Apply(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if derived.Fallback || derived.ValidationOutcome != ValidationAccepted ||
		!derived.Subsystems[0].Components[0].Hypothesis {
		t.Fatalf("family-only proposal was not locally derived as hypothetical: %#v", derived)
	}

	proposal.Subsystems[0].Components[0].Hypothesis = true
	hypothesis, err := Apply(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	component := hypothesis.Subsystems[0].Components[0]
	if hypothesis.Fallback || component.Hypothesis != true ||
		!reflect.DeepEqual(component.AnchorIDs, []string{family.ID}) {
		t.Fatalf("explicit family-context hypothesis = %#v", hypothesis)
	}

	bundle.BehaviorAnchors[0].ProofMode = AnchorProofCallTarget
	proposal.Subsystems[0].Components[0].Hypothesis = false
	partialProof, err := Apply(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if partialProof.Fallback || !partialProof.Subsystems[0].Components[0].Hypothesis {
		t.Fatalf("partially scoped call-target proof grounded the whole component: %#v", partialProof)
	}

	bundle.BehaviorAnchors[0].MemberIDs = candidateIDs(bundle.Candidates)
	operational, err := Apply(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if operational.Fallback || operational.Subsystems[0].Components[0].Hypothesis {
		t.Fatalf("call-target grounded proposal = %#v", operational)
	}
}

func TestDeclarationFamilyPackageContextNormalizesAndLocalDerivationStaysHypothetical(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	bundle.GroundingMode = GroundingMixed
	entrypointID := testMemberID(MemberEntrypoint, "backup-command")
	process := bundle.BehaviorAnchors[0]
	process.ID = "process"
	family := declarationFamilyTestAnchor(testMemberID(MemberPackage, "repo"))
	bundle.BehaviorAnchors = []BehaviorAnchor{process, family}
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Runtime",
			Components: []ProposedComponent{
				{Name: "Command", MemberIDs: []MemberID{entrypointID}, AnchorIDs: []string{process.ID}},
				{Name: "Repository family", MemberIDs: []MemberID{testMemberID(MemberPackage, "repo")}, AnchorIDs: []string{family.ID}},
				{
					Name: "Other exact members", MemberIDs: candidateIDsExcept(
						bundle, entrypointID, testMemberID(MemberPackage, "repo"),
					),
					Hypothesis: true,
				},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAccepted || len(result.Normalizations) != 0 {
		t.Fatalf("family-context local resolution = %#v", result)
	}
	var normalizedFamily bool
	for _, subsystem := range result.Subsystems {
		for _, component := range subsystem.Components {
			if component.Name == "Repository family" {
				normalizedFamily = component.Hypothesis &&
					reflect.DeepEqual(component.AnchorIDs, []string{family.ID})
			}
		}
	}
	if !normalizedFamily {
		t.Fatal("declaration-family context was lost or treated as operational")
	}

	localBundle := landscapeTestBundle()
	localBundle.GroundingMode = GroundingMixed
	localFamily := declarationFamilyTestAnchor(testMemberID(MemberPackage, "repo"))
	localBundle.BehaviorAnchors = []BehaviorAnchor{localFamily}
	local, err := Canonical(localBundle)
	if err != nil {
		t.Fatal(err)
	}
	var familyComponent *Component
	for subsystemIndex := range local.Subsystems {
		for componentIndex := range local.Subsystems[subsystemIndex].Components {
			component := &local.Subsystems[subsystemIndex].Components[componentIndex]
			if reflect.DeepEqual(component.AnchorIDs, []string{localFamily.ID}) {
				familyComponent = component
			}
		}
	}
	if familyComponent == nil || !familyComponent.Hypothesis {
		t.Fatalf("local family context = %#v", local)
	}
	familyComponent.Hypothesis = false
	if err := local.Validate(localBundle); err == nil || !strings.Contains(err.Error(), "operational anchor") {
		t.Fatalf("family-only non-hypothesis landscape validation = %v", err)
	}
}

func declarationFamilyTestAnchor(memberID MemberID) BehaviorAnchor {
	return BehaviorAnchor{
		ID: "declaration-family", Kind: AnchorExtensionFamily,
		ProofMode: AnchorProofDeclarationFamily,
		Label:     "Exact declaration family",
		Location:  evidence.Location{Path: "repository.go", Line: 1, Column: 1},
		Scenario:  ScenarioContext{ID: "go:test", Name: "test build"},
		Producer: evidence.Provenance{
			Provider: "fixture", Version: "v1", Operation: "declaration_family",
		},
		Certainty: evidence.CertaintyStatic, MemberIDs: []MemberID{memberID},
		Limitations: []string{"Static declaration family; runtime use is not observed."},
	}
}

// Decision 229 D7: a component referencing an unknown member ref is rejected
// item-scope — the valid sibling component still publishes (accepted_partial);
// the exact reason is counted. Whole-stage rejection only when zero
// independently valid items remain.
func TestApplyItemScopeUnknownMemberKeepsValidSiblings(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Storage",
			Components: []ProposedComponent{
				{
					Name:      "Repository",
					MemberIDs: []MemberID{bundle.Candidates[1].ID},
				},
				{
					Name:      "Invented",
					MemberIDs: []MemberID{testMemberID(MemberFile, "invented-by-provider")},
				},
			},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("valid sibling must publish accepted_partial after item-scope drop: %#v", result.Diagnostics)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.unknown_member_id") {
		t.Fatalf("diagnostics = %#v, want unknown member diagnostic counted", result.Diagnostics)
	}
	if len(result.Subsystems[0].Components) != 1 {
		t.Fatalf("components = %d, want 1 (invented component dropped item-scope)", len(result.Subsystems[0].Components))
	}
}

// Decision 229 D7: incompatible response-local ID reuse (duplicate member
// within one component) rejects only the dependent duplicate — the component
// survives with the first occurrence.
func TestApplyItemScopeDuplicateMemberKeepsComponent(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	memberID := bundle.Candidates[0].ID
	otherID := bundle.Candidates[1].ID
	result, err := Apply(bundle, Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Storage",
			Components: []ProposedComponent{{
				Name:      "Repository",
				MemberIDs: []MemberID{memberID, memberID, otherID},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if result.Fallback || result.ValidationOutcome != ValidationAcceptedPartial {
		t.Fatalf("duplicate member must normalize locally (accepted_partial): %#v", result.Diagnostics)
	}
	if !hasLandscapeDiagnostic(result.Diagnostics, "proposal.duplicate_member_id") {
		t.Fatalf("diagnostics = %#v, want duplicate member diagnostic counted", result.Diagnostics)
	}
	if len(result.Subsystems[0].Components[0].Members) != 2 {
		t.Fatalf("component members = %d, want 2 (duplicate normalized locally)", len(result.Subsystems[0].Components[0].Members))
	}
}

// Decision 229 D7: adding unrelated exact evidence cannot remove prior
// published facts — the same valid proposal over the same bundle publishes
// the identical component set on every replay.
func TestApplyReplayIsIdempotentAndCounted(t *testing.T) {
	t.Parallel()

	bundle := landscapeTestBundle()
	proposal := Proposal{
		Version: ContractVersion,
		Subsystems: []ProposedSubsystem{{
			Name: "Storage",
			Components: []ProposedComponent{{
				Name:      "Repository",
				MemberIDs: []MemberID{bundle.Candidates[1].ID, bundle.Candidates[0].ID},
			}},
		}},
	}
	first, err := Apply(bundle, proposal)
	if err != nil {
		t.Fatalf("Apply(first) error = %v", err)
	}
	second, err := Apply(bundle, proposal)
	if err != nil {
		t.Fatalf("Apply(second) error = %v", err)
	}
	if len(first.Subsystems) != len(second.Subsystems) {
		t.Fatalf("replay changed subsystem count: %d vs %d", len(first.Subsystems), len(second.Subsystems))
	}
	for index := range first.Subsystems {
		if len(first.Subsystems[index].Components) != len(second.Subsystems[index].Components) {
			t.Fatalf("replay changed component count at subsystem %d", index)
		}
	}
}

func TestApplyDoesNotNormalizeUnknownOrNonPackageUngroundedComponent(t *testing.T) {

	tests := []struct {
		name        string
		memberID    MemberID
		wantPartial bool
	}{
		{name: "unknown package", memberID: testMemberID(MemberPackage, "unknown")},
		{name: "known file", memberID: testMemberID(MemberFile, "repo-file"), wantPartial: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bundle := landscapeTestBundle()
			bundle.GroundingMode = GroundingMixed
			bundle.BehaviorAnchors = []BehaviorAnchor{{
				ID: "process", Kind: AnchorProcessEntry, Label: "process entry",
				ProofMode:   AnchorProofProcessEntry,
				Location:    evidence.Location{Path: "cmd/backup.go", Line: 20, Column: 1},
				Scenario:    ScenarioContext{ID: "go:test", Name: "test build"},
				Producer:    evidence.Provenance{Provider: "fixture", Version: "v1", Operation: "process_entry"},
				Certainty:   evidence.CertaintyStatic,
				MemberIDs:   []MemberID{testMemberID(MemberEntrypoint, "backup-command")},
				Limitations: []string{"execution is not observed"},
			}}
			result, err := Apply(bundle, Proposal{
				Version: ContractVersion,
				Subsystems: []ProposedSubsystem{{
					Name:       "Runtime",
					Components: []ProposedComponent{{Name: "Unsupported", MemberIDs: []MemberID{test.memberID}}},
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if test.wantPartial {
				if result.Fallback || result.ValidationOutcome != ValidationAcceptedPartial ||
					!result.Subsystems[0].Components[0].Hypothesis {
					t.Fatalf("locally derived known-file partial result = %#v", result)
				}
			} else if !result.Fallback || result.ValidationOutcome != ValidationRejected || len(result.Normalizations) != 0 {
				t.Fatalf("unknown identity result = %#v", result)
			}
		})
	}
}

func candidateBundleWithPackages(count int) CandidateBundle {
	bundle := CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeLibraryFramework,
		GroundingMode: GroundingPackages,
		Candidates:    make([]Candidate, 0, count),
	}
	for index := 0; index < count; index++ {
		packagePath := fmt.Sprintf("example.com/project/package-%03d", index)
		bundle.Candidates = append(bundle.Candidates, Candidate{
			ID:   MemberID{Kind: MemberPackage, Value: fmt.Sprintf("package-%03d", index)},
			Role: CandidateRoleConceptualMember,
			Name: packagePath,
			Facts: []LocalFact{{
				Kind: FactDeclaration, Value: packagePath, Certainty: evidence.CertaintyStatic,
				Provenance: []evidence.Provenance{{
					Provider: "fixture", Version: "v1", Operation: "package_declaration",
				}},
			}},
		})
	}
	return bundle
}

func candidateBundleWithRelations(count int) CandidateBundle {
	// Thirty-three exact package members provide 1,056 distinct directed
	// witnesses, enough to exercise the 1,024-relation boundary without
	// manufacturing duplicate relation identities.
	bundle := candidateBundleWithPackages(33)
	bundle.Relations = make([]LocalRelation, 0, count)
	for from := 0; from < len(bundle.Candidates) && len(bundle.Relations) < count; from++ {
		for to := 0; to < len(bundle.Candidates) && len(bundle.Relations) < count; to++ {
			if from == to {
				continue
			}
			bundle.Relations = append(bundle.Relations, LocalRelation{
				ID:        fmt.Sprintf("relation-%04d", len(bundle.Relations)),
				From:      bundle.Candidates[from].ID,
				To:        bundle.Candidates[to].ID,
				Kind:      StructuralRelationPackageImport,
				Certainty: evidence.CertaintyStatic,
				Provenance: []evidence.Provenance{{
					Provider: "fixture", Version: "v1", Operation: "package_import",
				}},
				Scenarios: []ScenarioContext{{ID: "go-default", Name: "Default Go build"}},
			})
		}
	}
	return bundle
}

func candidateBundleWithBehaviorAnchors(count int) CandidateBundle {
	bundle := candidateBundleWithPackages(1)
	bundle.BehaviorAnchors = make([]BehaviorAnchor, 0, count)
	for index := 0; index < count; index++ {
		bundle.BehaviorAnchors = append(bundle.BehaviorAnchors, BehaviorAnchor{
			ID: fmt.Sprintf("anchor-%03d", index), Kind: AnchorUnresolvedFrontier,
			ProofMode:   AnchorProofDeclarationFamily,
			Label:       fmt.Sprintf("Exact local anchor %d", index),
			Location:    evidence.Location{Path: "package/file.go", Line: index + 1, Column: 1},
			Scenario:    ScenarioContext{ID: "go-default", Name: "Default Go build"},
			Producer:    evidence.Provenance{Provider: "fixture", Version: "v1", Operation: "local_anchor"},
			Certainty:   evidence.CertaintyStatic,
			MemberIDs:   []MemberID{bundle.Candidates[0].ID},
			Limitations: []string{"Static fixture evidence; runtime execution is not observed."},
		})
	}
	return bundle
}

func landscapeTestBundle() CandidateBundle {
	packageID := testMemberID(MemberPackage, "repo")
	commandPackageID := testMemberID(MemberPackage, "cmd")
	entrypointID := testMemberID(MemberEntrypoint, "backup-command")
	flowID := FlowID("backup")
	return CandidateBundle{
		Version: ContractVersion, RepositoryArchetype: ArchetypeApplication, GroundingMode: GroundingPackages,
		BehaviorAnchors: []BehaviorAnchor{{
			ID: "run-backup", Kind: AnchorProcessEntry, Label: "backup process entry",
			ProofMode: AnchorProofProcessEntry,
			Location:  evidence.Location{Path: "cmd/backup.go", Line: 20, Column: 1},
			Scenario:  ScenarioContext{ID: "go:test", Name: "test build"},
			Producer: evidence.Provenance{
				Provider: "fixture", Version: "v1", Operation: "classify_process_entry",
			},
			Certainty: evidence.CertaintyStatic, MemberIDs: []MemberID{entrypointID},
			Limitations: []string{"Static fixture evidence; runtime execution is not observed."},
		}},
		Flows: []Flow{{
			ID: flowID, Name: "Backup",
			Facts: []LocalFact{testLocalFact(FactDeclaration, "saved flowproof v2", "cmd/backup.go", 20)},
		}},
		Candidates: []Candidate{
			{
				ID: commandPackageID, Role: CandidateRoleConceptualMember, Name: "command package",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/example/cmd", "cmd/main.go", 1)},
			},
			{
				ID: packageID, Role: CandidateRoleConceptualMember, Name: "repository package",
				Facts: []LocalFact{testLocalFact(FactDeclaration, "github.com/example/repository", "repository.go", 1)},
			},
			{
				ID: testMemberID(MemberFile, "repo-file"), Role: CandidateRoleConceptualMember,
				Name: "repository.go", ParentID: &packageID,
				Participations: []FlowParticipation{testFlowParticipation(flowID, "repository.go", 1)},
				Facts:          []LocalFact{testLocalFact(FactRepositoryPath, "repository.go", "repository.go", 1)},
			},
			{
				ID: entrypointID, Role: CandidateRoleConceptualMember,
				Name: "backup command", ParentID: &commandPackageID,
				Participations: []FlowParticipation{testFlowParticipation(flowID, "cmd/backup.go", 20)},
				Facts:          []LocalFact{testLocalFact(FactDeclaration, "runBackup", "cmd/backup.go", 20)},
			},
			{
				ID: testMemberID(MemberFlow, "backup-flow"), Role: CandidateRoleConceptualMember, Name: "backup",
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

func candidateIDsExcept(bundle CandidateBundle, excluded ...MemberID) []MemberID {
	excludedSet := make(map[MemberID]struct{}, len(excluded))
	for _, memberID := range excluded {
		excludedSet[memberID] = struct{}{}
	}
	result := make([]MemberID, 0, len(bundle.Candidates)-len(excludedSet))
	for _, candidate := range bundle.Candidates {
		if candidate.Role != CandidateRoleConceptualMember {
			continue
		}
		if _, skip := excludedSet[candidate.ID]; skip {
			continue
		}
		result = append(result, candidate.ID)
	}
	return result
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
