package tasklens

import (
	"encoding/json"
	"testing"
)

func TestSourceScopeValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		scope     SourceScope
		wantError bool
	}{
		{
			name: "complete symbol",
			scope: SourceScope{
				ScopeKind:             SourceScopeCompleteEnclosingSymbol,
				ScopeStart:            10,
				ScopeEnd:              30,
				SourceTotalLines:      80,
				NegativeClaimsAllowed: true,
				NegativeEvidenceBasis: NegativeEvidenceCompleteScope,
			},
		},
		{
			name: "truncated source with unknown total",
			scope: SourceScope{
				ScopeKind:             SourceScopePartialWindow,
				ScopeStart:            20,
				ScopeEnd:              40,
				SourceTotalLines:      0,
				Truncated:             true,
				TruncationReason:      "source read stopped at the per-file byte bound",
				NegativeEvidenceBasis: NegativeEvidenceNone,
			},
		},
		{
			name: "matched fragments with independent exact search",
			scope: SourceScope{
				ScopeKind:             SourceScopeMatchedFragments,
				ScopeStart:            5,
				ScopeEnd:              90,
				SourceTotalLines:      120,
				Truncated:             true,
				TruncationReason:      "only exact task-matching fragments were retained",
				NegativeClaimsAllowed: true,
				NegativeEvidenceBasis: NegativeEvidenceExhaustiveExactSearch,
			},
		},
		{
			name: "complete file must span total source",
			scope: SourceScope{
				ScopeKind:             SourceScopeCompleteFile,
				ScopeStart:            1,
				ScopeEnd:              9,
				SourceTotalLines:      10,
				NegativeEvidenceBasis: NegativeEvidenceNone,
			},
			wantError: true,
		},
		{
			name: "known total cannot end before retained lines",
			scope: SourceScope{
				ScopeKind:             SourceScopePartialWindow,
				ScopeStart:            10,
				ScopeEnd:              20,
				SourceTotalLines:      19,
				Truncated:             true,
				TruncationReason:      "bounded window",
				NegativeEvidenceBasis: NegativeEvidenceNone,
			},
			wantError: true,
		},
		{
			name: "partial window cannot allow negative claims",
			scope: SourceScope{
				ScopeKind:                SourceScopePartialWindow,
				ScopeStart:               1,
				ScopeEnd:                 20,
				SourceTotalLines:         100,
				Truncated:                true,
				TruncationReason:         "function exceeds the per-anchor bound",
				TaskMatchesOutsideWindow: true,
				NegativeClaimsAllowed:    true,
				NegativeEvidenceBasis:    NegativeEvidenceExhaustiveExactSearch,
			},
			wantError: true,
		},
		{
			name: "incomplete scope exposes truncation",
			scope: SourceScope{
				ScopeKind:             SourceScopeMatchedFragments,
				ScopeStart:            1,
				ScopeEnd:              20,
				SourceTotalLines:      100,
				NegativeEvidenceBasis: NegativeEvidenceNone,
			},
			wantError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.scope.Validate()
			if test.wantError && err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestSourceScopeValidateClaimRejectsAbsenceFromPartialWindow(t *testing.T) {
	t.Parallel()

	scope := SourceScope{
		ScopeKind:             SourceScopePartialWindow,
		ScopeStart:            1,
		ScopeEnd:              20,
		SourceTotalLines:      0,
		Truncated:             true,
		TruncationReason:      "source total is unknown after a bounded read",
		NegativeEvidenceBasis: NegativeEvidenceNone,
	}
	if err := scope.ValidateClaim(false); err != nil {
		t.Fatalf("ValidateClaim(false) error = %v, want nil", err)
	}
	if err := scope.ValidateClaim(true); err == nil {
		t.Fatal("ValidateClaim(true) error = nil, want absence rejection")
	}
}

func TestDefaultRoleContractValidate(t *testing.T) {
	t.Parallel()

	profiles := []TaskProfile{
		TaskProfileDataTagTransformation,
		TaskProfileErrorStatusMapping,
		TaskProfileNilPanic,
		TaskProfileConfigurationPropagation,
		TaskProfileErrorNormalizationPrivacy,
		TaskProfileExtensionContribution,
		TaskProfileOperationalRelease,
		TaskProfileUnknown,
	}
	for _, profile := range profiles {
		t.Run(string(profile), func(t *testing.T) {
			t.Parallel()
			contract, err := DefaultRoleContract(profile)
			if err != nil {
				t.Fatalf("DefaultRoleContract() error = %v", err)
			}
			if contract.Profile != profile {
				t.Fatalf("Profile = %q, want %q", contract.Profile, profile)
			}
		})
	}
}

func TestEvaluateRoleCoverageHonorsMinimumAnchors(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileExtensionContribution)
	if err != nil {
		t.Fatal(err)
	}
	anchors := []Anchor{
		{ID: "anchor-port", RoleHints: []AnchorRole{RoleExtensionPort}},
		{ID: "anchor-sibling-one", RoleHints: []AnchorRole{RoleRepresentativeImplementation}},
		{ID: "anchor-wiring", RoleHints: []AnchorRole{RoleWiringComposition}},
	}
	coverage, err := EvaluateRoleCoverage(contract, anchors)
	if err != nil {
		t.Fatal(err)
	}
	missing := coverage.MissingKeyRoles()
	if len(missing) != 1 || missing[0] != RoleRepresentativeImplementation {
		t.Fatalf("MissingKeyRoles() = %v, want [%s]", missing, RoleRepresentativeImplementation)
	}
	for _, group := range [][]RoleCoverageItem{coverage.Key, coverage.Supporting, coverage.Optional} {
		for _, item := range group {
			if !item.Represented && item.AnchorIDs == nil {
				t.Fatalf("unrepresented role %s serialized anchor_ids as null", item.Role)
			}
		}
	}

	anchors = append(anchors, Anchor{
		ID:        "anchor-sibling-two",
		RoleHints: []AnchorRole{RoleRepresentativeImplementation},
	})
	coverage, err = EvaluateRoleCoverage(contract, anchors)
	if err != nil {
		t.Fatal(err)
	}
	if missing = coverage.MissingKeyRoles(); len(missing) != 0 {
		t.Fatalf("MissingKeyRoles() = %v, want none", missing)
	}
}

func TestRoleCoverageCannotDropKeyContractRole(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileErrorStatusMapping)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := EvaluateRoleCoverage(contract, []Anchor{})
	if err != nil {
		t.Fatal(err)
	}
	coverage.Key = coverage.Key[1:]
	if err := coverage.ValidateAgainst(contract); err == nil {
		t.Fatal("ValidateAgainst() error = nil, want edited-contract rejection")
	}
}

func TestVerificationFrontierAuthority(t *testing.T) {
	t.Parallel()

	exact := VerificationFrontier{
		DecisiveAnchorID: "anchor-decisive",
		Anchors: []VerificationItem{
			{
				ID:          "verification-test",
				Authority:   VerificationExactExistingTest,
				AnchorID:    "anchor-test",
				Path:        "pkg/handler_test.go",
				Symbol:      "TestHandler",
				Text:        "Focused test observes the status returned by the decisive handler.",
				EvidenceIDs: []string{"evidence-test"},
			},
		},
	}
	if err := exact.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if !exact.HasExactAnchorOrEffect() {
		t.Fatal("HasExactAnchorOrEffect() = false, want true")
	}

	proposed := VerificationFrontier{
		Anchors: []VerificationItem{
			{
				ID:        "verification-proposed",
				Authority: VerificationProposedTestLocation,
				Path:      "pkg/handler_test.go",
				Text:      "A focused regression test could be added here.",
			},
		},
	}
	if err := proposed.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if proposed.HasExactAnchorOrEffect() {
		t.Fatal("proposed test location must not count as exact historical verification")
	}
}

func TestVerificationFrontierRejectsUngroundedExactItem(t *testing.T) {
	t.Parallel()

	frontier := VerificationFrontier{
		Anchors: []VerificationItem{
			{
				ID:        "verification-test",
				Authority: VerificationExactExistingTest,
				Text:      "Claims an exact test without retained evidence.",
			},
		},
	}
	if err := frontier.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want grounding rejection")
	}
}

func TestRelationKindsUseRequiredLabels(t *testing.T) {
	t.Parallel()

	kinds := []RelationKind{
		RelationDirectCall,
		RelationFieldCopy,
		RelationFieldRead,
		RelationFieldWrite,
		RelationErrorCreated,
		RelationErrorMapped,
		RelationErrorExposed,
		RelationValueTransformed,
		RelationTypeNameGenerated,
		RelationConfigApplied,
		RelationScriptInvokes,
		RelationTestExercises,
		RelationFixtureRecords,
		RelationDocumentedUses,
		RelationSharedStateAlias,
		RelationScopeUnknown,
	}
	for _, kind := range kinds {
		if !validRelationKind(kind) {
			t.Fatalf("validRelationKind(%q) = false", kind)
		}
	}
}

func TestEvaluateCheapExit(t *testing.T) {
	t.Parallel()

	base := CheapExitInput{
		AreaIDs:                 []string{"area-handler"},
		MissingKeyRoles:         []AnchorRole{},
		DecisiveRelationKind:    RelationErrorMapped,
		DecisiveRelationSupport: SupportLocallyObserved,
		Verification: VerificationFrontier{
			DecisiveAnchorID: "anchor-decisive",
			Anchors: []VerificationItem{
				{
					ID:          "verification-test",
					Authority:   VerificationExactExistingTest,
					AnchorID:    "anchor-test",
					Text:        "Focused status assertion.",
					EvidenceIDs: []string{"evidence-test"},
				},
			},
		},
	}

	tests := []struct {
		name     string
		mutate   func(*CheapExitInput)
		eligible bool
	}{
		{name: "all gates pass", eligible: true},
		{
			name: "ambiguous area",
			mutate: func(input *CheapExitInput) {
				input.AreaIDs = []string{"area-handler", "area-serializer"}
			},
		},
		{
			name: "missing key role",
			mutate: func(input *CheapExitInput) {
				input.MissingKeyRoles = []AnchorRole{RoleErrorMapping}
			},
		},
		{
			name: "relation is model hypothesis",
			mutate: func(input *CheapExitInput) {
				input.DecisiveRelationSupport = SupportModelHypothesis
			},
		},
		{
			name: "verification is only proposed",
			mutate: func(input *CheapExitInput) {
				input.Verification = VerificationFrontier{
					Anchors: []VerificationItem{
						{
							ID:        "verification-proposed",
							Authority: VerificationProposedTestLocation,
							Text:      "Potential test location.",
						},
					},
				}
			},
		},
		{
			name: "competing hypothesis remains",
			mutate: func(input *CheapExitInput) {
				input.UnresolvedCompetingHypotheses = 1
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := base
			input.AreaIDs = append([]string(nil), base.AreaIDs...)
			input.MissingKeyRoles = append([]AnchorRole(nil), base.MissingKeyRoles...)
			if test.mutate != nil {
				test.mutate(&input)
			}
			decision := EvaluateCheapExit(input)
			if decision.Eligible != test.eligible {
				t.Fatalf("Eligible = %t, want %t; reasons: %v", decision.Eligible, test.eligible, decision.Reasons)
			}
			if len(decision.Gates) != 5 {
				t.Fatalf("len(Gates) = %d, want 5", len(decision.Gates))
			}
			wantRoute := CheapExitRouteSynthesisCall
			if test.eligible {
				wantRoute = CheapExitRouteZeroCall
			}
			if decision.Route != wantRoute {
				t.Fatalf("Route = %q, want %q", decision.Route, wantRoute)
			}
		})
	}
}

func TestContractsJSONRoundTrip(t *testing.T) {
	t.Parallel()

	input := SourceScope{
		ScopeKind:             SourceScopeCompleteFile,
		ScopeStart:            1,
		ScopeEnd:              12,
		SourceTotalLines:      12,
		NegativeClaimsAllowed: true,
		NegativeEvidenceBasis: NegativeEvidenceCompleteScope,
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output SourceScope
	if err := json.Unmarshal(raw, &output); err != nil {
		t.Fatal(err)
	}
	if output != input {
		t.Fatalf("round trip = %#v, want %#v", output, input)
	}
}
