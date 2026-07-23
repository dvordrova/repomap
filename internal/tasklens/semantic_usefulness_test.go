package tasklens

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestPlausibleHypothesisPreservesOnlyExplicitEvidenceBoundSemantics(t *testing.T) {
	repo := newTaskLensTestRepo(t, "semantic-hypothesis")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and ReadEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	copyConfig := anchorBySymbol(bundle.Anchors, "CopyConfig")
	readEnabled := anchorBySymbol(bundle.Anchors, "ReadEnabled")
	if copyConfig.ID == "" || readEnabled.ID == "" {
		t.Fatalf("required anchors missing: %#v", bundle.Anchors)
	}
	selected := map[string]struct{}{copyConfig.ID: {}, readEnabled.ID: {}}
	supportIDs := append(append([]string(nil), copyConfig.EvidenceIDs...), readEnabled.EvidenceIDs...)
	index := newBundleIndex(bundle)
	groundedText := "CopyConfig may leave Enabled unset before ReadEnabled observes it. Missing evidence: an exact assignment or call connecting CopyConfig to ReadEnabled."

	tests := []struct {
		name         string
		status       HypothesisStatus
		text         string
		wantPreserve bool
	}{
		{
			name:         "explicit grounded plausible hypothesis",
			status:       HypothesisPlausible,
			text:         groundedText,
			wantPreserve: true,
		},
		{
			name:   "plausible status alone does not name missing evidence",
			status: HypothesisPlausible,
			text:   "CopyConfig may copy Enabled before ReadEnabled observes it.",
		},
		{
			name:   "generic missing evidence phrase is insufficient",
			status: HypothesisPlausible,
			text:   "CopyConfig may affect ReadEnabled. Missing evidence: more context.",
		},
		{
			name:   "plausible text with an uncited symbol",
			status: HypothesisPlausible,
			text:   "CopyConfig may route state through ImaginaryHandler before ReadEnabled observes it. Missing evidence: an exact call connecting CopyConfig to ReadEnabled.",
		},
		{
			name:   "supported fact lane remains local",
			status: HypothesisSupported,
			text:   groundedText,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clause, clauseErr := buildClause(ProposedClause{
				Status: test.status, Text: test.text, SupportIDs: supportIDs,
			}, selected, index)
			if clauseErr != nil {
				t.Fatal(clauseErr)
			}
			if got := clause.Text == test.text; got != test.wantPreserve {
				t.Fatalf("text preserved = %t, want %t: %q", got, test.wantPreserve, clause.Text)
			}
			if !test.wantPreserve && strings.Contains(clause.Text, "ImaginaryHandler") {
				t.Fatalf("uncited model semantics escaped local reconstruction: %q", clause.Text)
			}
		})
	}
}

func TestAbsenceClaimsRequireFileOrExhaustiveAuthority(t *testing.T) {
	t.Parallel()

	const (
		anchorID   = "anchor-handler"
		evidenceID = "evidence-handler"
	)
	tests := []struct {
		name      string
		scope     SourceScope
		wantError bool
	}{
		{
			name: "complete symbol is insufficient for unstructured absence",
			scope: SourceScope{
				ScopeKind: SourceScopeCompleteEnclosingSymbol, ScopeStart: 1, ScopeEnd: 5,
				SourceTotalLines: 10, NegativeClaimsAllowed: true,
				NegativeEvidenceBasis: NegativeEvidenceCompleteScope,
			},
			wantError: true,
		},
		{
			name: "complete file",
			scope: SourceScope{
				ScopeKind: SourceScopeCompleteFile, ScopeStart: 1, ScopeEnd: 10,
				SourceTotalLines: 10, NegativeClaimsAllowed: true,
				NegativeEvidenceBasis: NegativeEvidenceCompleteScope,
			},
		},
		{
			name: "exhaustive bounded search",
			scope: SourceScope{
				ScopeKind: SourceScopeMatchedFragments, ScopeStart: 1, ScopeEnd: 5,
				SourceTotalLines: 10, Truncated: true, TruncationReason: "bounded exact search",
				NegativeClaimsAllowed: true, NegativeEvidenceBasis: NegativeEvidenceExhaustiveExactSearch,
			},
		},
		{
			name: "deterministic manifest",
			scope: SourceScope{
				ScopeKind: SourceScopeMatchedFragments, ScopeStart: 1, ScopeEnd: 5,
				SourceTotalLines: 10, Truncated: true, TruncationReason: "manifest projection",
				NegativeClaimsAllowed: true, NegativeEvidenceBasis: NegativeEvidenceDeterministicManifest,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			index := bundleIndex{
				anchors: map[string]Anchor{anchorID: {ID: anchorID, Symbol: "Handler", Scope: test.scope}},
				evidence: map[string]Evidence{evidenceID: {
					ID: evidenceID, Kind: EvidenceRepositoryFact, AnchorID: anchorID,
				}},
				paths: map[string]struct{}{}, relations: map[string]Relation{},
			}
			_, err := buildClause(ProposedClause{
				Status: HypothesisSupported, Text: "Handler has no guard for the request.",
				SupportIDs: []string{evidenceID},
			}, map[string]struct{}{anchorID: {}}, index)
			if (err != nil) != test.wantError {
				t.Fatalf("buildClause() error = %v, want error %t", err, test.wantError)
			}
		})
	}
}

func TestEpistemicNonGuaranteesAreNotAbsenceClaims(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "does not prove", text: "The bounded source does not prove runtime reachability."},
		{name: "not established", text: "Runtime order is not established by retained evidence."},
		{name: "missing evidence label", text: "Missing evidence: an exact call between Handler and Serve."},
		{name: "missing test", text: "No test exists for Handler.", want: true},
		{name: "not implemented", text: "The handler is not implemented.", want: true},
		{name: "fails to copy", text: "CopyConfig fails to copy Enabled.", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := containsAbsenceClaim(test.text); got != test.want {
				t.Fatalf("containsAbsenceClaim(%q) = %t, want %t", test.text, got, test.want)
			}
		})
	}
}

func TestLocalProposalSelectsTaskSpecificAnchorBeyondFirstEight(t *testing.T) {
	anchors := make([]Anchor, 0, 9)
	for index := 0; index < 8; index++ {
		anchors = append(anchors, Anchor{
			ID:     "anchor-unrelated-" + strconv.Itoa(index),
			Path:   "unrelated" + strconv.Itoa(index) + "_test.go",
			Symbol: "TestUnrelated" + strconv.Itoa(index), Score: 200,
			RoleHints: []AnchorRole{RoleVerificationAnchor},
		})
	}
	anchors = append(anchors, Anchor{
		ID: "anchor-task-specific", Path: "feature_test.go",
		Symbol: "TestFeatureSetting", Score: 20,
		RoleHints: []AnchorRole{RoleVerificationAnchor},
	})
	bundle := Bundle{
		Anchors: anchors,
		Terms:   []Term{{Text: "FeatureSetting", Normalized: "featuresetting", Weight: 16}},
	}
	for _, anchor := range localProposalAnchors(bundle) {
		if anchor.ID == "anchor-task-specific" {
			return
		}
	}
	t.Fatal("task-specific anchor was crowded out by the first eight retained anchors")
}

func TestRankedLocalProposalRelationsReservesDecisiveRelation(t *testing.T) {
	t.Parallel()

	bundle := Bundle{
		Anchors: []Anchor{
			{ID: "anchor-decisive-left", Score: 0},
			{ID: "anchor-decisive-right", Score: 0},
			{ID: "anchor-high-left", Score: 1_000},
			{ID: "anchor-high-right", Score: 1_000},
		},
		Relations: []Relation{
			{ID: "relation-decisive", LeftID: "anchor-decisive-left", RightID: "anchor-decisive-right", Kind: relationKindDirectCall},
			{ID: "relation-high-score", LeftID: "anchor-high-left", RightID: "anchor-high-right", Kind: relationKindDirectCall},
		},
		DecisiveRelationID: "relation-decisive",
	}
	selected := map[string]struct{}{
		"anchor-decisive-left": {}, "anchor-decisive-right": {},
		"anchor-high-left": {}, "anchor-high-right": {},
	}
	ranked := rankedLocalProposalRelations(bundle, selected)
	if len(ranked) != 2 || ranked[0].ID != bundle.DecisiveRelationID {
		t.Fatalf("ranked relations = %#v, want decisive relation first", ranked)
	}
}

func TestLocalProposalAnchorsReservesDecisiveEndpointsBeyondPerFilePreference(t *testing.T) {
	t.Parallel()

	anchors := make([]Anchor, 0, 10)
	for index := 0; index < 10; index++ {
		anchor := Anchor{
			ID: "anchor-" + strconv.Itoa(index), Path: "mechanism.go",
			Symbol: "Symbol" + strconv.Itoa(index), Score: 100 - index,
		}
		if index < 4 {
			anchor.RoleHints = []AnchorRole{RoleRepresentativeImplementation}
		}
		anchors = append(anchors, anchor)
	}
	bundle := Bundle{
		Anchors: anchors,
		RoleContract: RoleContract{Key: []RoleRequirement{{
			Role: RoleRepresentativeImplementation, MinimumAnchors: 4,
		}}},
		Relations: []Relation{{
			ID: "relation-decisive", LeftID: "anchor-6", RightID: "anchor-7",
			Kind: relationKindDirectCall,
		}},
		DecisiveRelationID: "relation-decisive",
	}
	selected := localProposalAnchors(bundle)
	selectedIDs := make(map[string]struct{}, len(selected))
	for _, anchor := range selected {
		selectedIDs[anchor.ID] = struct{}{}
	}
	for _, endpoint := range []string{"anchor-6", "anchor-7"} {
		if _, ok := selectedIDs[endpoint]; !ok {
			t.Fatalf("decisive endpoint %q was not reserved: %#v", endpoint, selected)
		}
	}
}

func TestLocalAndReducedProposalsOmitGenericAuxiliaryAnchor(t *testing.T) {
	repo := newTaskLensTestRepo(t, "auxiliary-anchor")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The generic fixture document is retained by collection, but its weak term
	// overlap and optional role do not make it part of this investigation. It
	// models the generic Auxiliary anchor without making the fixture task-specific.
	auxiliary := anchorBySymbol(bundle.Anchors, "Fixture")
	if auxiliary.ID == "" {
		t.Fatalf("fixture did not retain generic auxiliary anchor: %#v", bundle.Anchors)
	}
	if _, relevant := taskRelevantBundleAnchorIDs(bundle)[auxiliary.ID]; relevant {
		t.Fatal("generic Auxiliary unexpectedly acquired a local task-relevance reason")
	}

	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if proposedAnchorByID(proposal.Anchors, auxiliary.ID) {
		t.Fatalf("local proposal retained generic Auxiliary: %#v", proposal.Anchors)
	}
	if len(proposal.Anchors) >= MaxVisibleAnchors {
		t.Fatalf("fixture left no room for adversarial model anchor: %d", len(proposal.Anchors))
	}
	proposal.Anchors = append(proposal.Anchors, ProposedAnchor{
		AnchorID: auxiliary.ID,
		Role:     auxiliary.RoleHints[0],
		Why:      "Auxiliary may help with configuration.",
	})
	proposal.Areas[0].TargetIDs = append(proposal.Areas[0].TargetIDs, auxiliary.ID)

	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "was omitted") {
		t.Fatalf("reducer warnings = %#v", warnings)
	}
	for _, anchor := range pack.Anchors {
		if anchor.ID == auxiliary.ID {
			t.Fatalf("reduced pack retained generic Auxiliary: %#v", pack.Anchors)
		}
	}
}

func proposedAnchorByID(anchors []ProposedAnchor, id string) bool {
	for _, anchor := range anchors {
		if anchor.AnchorID == id {
			return true
		}
	}
	return false
}

func TestExtensionPlanCannotBecomeTaskProvidedReproduction(t *testing.T) {
	repo := newTaskLensTestRepo(t, "extension-guidance")
	const task = "Extend middleware support with Gin examples and documentation."
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo, TaskText: task,
	})
	if err != nil {
		t.Fatal(err)
	}
	if TaskProvidesConcreteReproductionOrObservation(task) {
		t.Fatal("plan-only extension was classified as a concrete task observation")
	}
	if TaskProvidesConcreteReproductionOrObservation(
		"Add Gin support and provide a reproduction and verification plan.",
	) {
		t.Fatal("requested reproduction plan was treated as an already supplied observation")
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.ReproduceOrObserve[0].Authority == AuthorityTaskProvided {
		t.Fatalf("plan-only extension received task-provided reproduction: %#v", proposal.ReproduceOrObserve)
	}
	proposal.ReproduceOrObserve[0] = ProposedGuidance{
		Text: "Use the requested examples as reproduction.", Authority: AuthorityTaskProvided,
		EvidenceIDs: []string{bundle.Task.EvidenceID},
	}
	if _, err := BuildPack(bundle, proposal); err == nil ||
		!strings.Contains(err.Error(), "task-provided guidance") {
		t.Fatalf("model-promoted task reproduction error = %v", err)
	}
}

func TestModelHypothesisJoinPreservesOnlyGroundedSemantics(t *testing.T) {
	repo := newTaskLensTestRepo(t, "semantic-join")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and ReadEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	copyConfig := anchorBySymbol(bundle.Anchors, "CopyConfig")
	readEnabled := anchorBySymbol(bundle.Anchors, "ReadEnabled")
	if copyConfig.ID == "" || readEnabled.ID == "" {
		t.Fatalf("required anchors missing: %#v", bundle.Anchors)
	}
	selected := map[string]struct{}{copyConfig.ID: {}, readEnabled.ID: {}}
	supportIDs := append(append([]string(nil), copyConfig.EvidenceIDs...), readEnabled.EvidenceIDs...)
	index := newBundleIndex(bundle)
	wantExplanation := "CopyConfig may leave Enabled unset before ReadEnabled observes it."
	join, err := buildJoin(ProposedJoin{
		LeftID: copyConfig.ID, RightID: readEnabled.ID, Kind: "possible_copy_path",
		SupportType: SupportModelHypothesis, SupportIDs: supportIDs,
		Explanation: wantExplanation, Scope: "This is a proposed relationship.",
	}, selected, index)
	if err != nil {
		t.Fatal(err)
	}
	if join.SupportType != SupportModelHypothesis || join.Kind != "model_hypothesis" ||
		join.Explanation != wantExplanation {
		t.Fatalf("model-hypothesis join = %#v", join)
	}
	if _, err := buildJoin(ProposedJoin{
		LeftID: copyConfig.ID, RightID: readEnabled.ID, Kind: "possible_copy_path",
		SupportType: SupportModelHypothesis, SupportIDs: supportIDs,
		Explanation: "CopyConfig may route through ImaginaryHandler before ReadEnabled observes it.",
		Scope:       "This is a proposed relationship.",
	}, selected, index); err == nil {
		t.Fatal("model-hypothesis join with an uncited symbol was accepted")
	}

	if len(bundle.Relations) == 0 {
		t.Fatal("fixture did not produce a locally observed relation")
	}
	relation := bundle.Relations[0]
	selected = map[string]struct{}{relation.LeftID: {}, relation.RightID: {}}
	join, err = buildJoin(ProposedJoin{
		LeftID: relation.LeftID, RightID: relation.RightID,
		RelationID: relation.ID, Kind: relation.Kind,
		SupportType: SupportLocallyObserved, SupportIDs: relation.EvidenceIDs,
		Explanation: "The model says this definitely causes the failure.",
		Scope:       "The model says this proves runtime order.",
	}, selected, index)
	if err != nil {
		t.Fatal(err)
	}
	if join.Explanation != localRelationExplanation(relation.Kind) || join.Scope != relation.Scope ||
		strings.Contains(join.Explanation, "definitely") || strings.Contains(join.Scope, "proves") {
		t.Fatalf("locally observed join retained model prose: %#v", join)
	}
}

func TestReducerKeepsGroundedSemanticsAndReconstructsSpecificGuidance(t *testing.T) {
	const task = "The Enabled configuration is silently ignored.\n\nA minimal reproduction constructs Config{Enabled: true}, calls CopyConfig, and observes ReadEnabled."
	repo := newTaskLensTestRepo(t, "semantic-guidance")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       task,
	})
	if err != nil {
		t.Fatal(err)
	}
	copyConfig := anchorBySymbol(bundle.Anchors, "CopyConfig")
	readEnabled := anchorBySymbol(bundle.Anchors, "ReadEnabled")
	if copyConfig.ID == "" || readEnabled.ID == "" {
		t.Fatalf("required anchors missing: %#v", bundle.Anchors)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	selected := make(map[string]struct{}, len(proposal.Anchors))
	for _, anchor := range proposal.Anchors {
		selected[anchor.AnchorID] = struct{}{}
	}
	if _, ok := selected[copyConfig.ID]; !ok {
		t.Fatalf("CopyConfig is not visible in local proposal: %#v", proposal.Anchors)
	}
	if _, ok := selected[readEnabled.ID]; !ok {
		t.Fatalf("ReadEnabled is not visible in local proposal: %#v", proposal.Anchors)
	}
	if len(proposal.Verify.Steps) == 0 || proposal.Verify.Steps[0].Authority != AuthorityRepositoryTest {
		t.Fatalf("fixture did not retain a repository test: %#v", proposal.Verify.Steps)
	}

	groundedHypothesis := "CopyConfig may leave Enabled unset before ReadEnabled observes it. Missing evidence: an exact assignment or call connecting CopyConfig to ReadEnabled."
	proposal.Hypothesis = []ProposedClause{{
		Status: HypothesisPlausible, Text: groundedHypothesis,
		SupportIDs: append(append([]string(nil), copyConfig.EvidenceIDs...), readEnabled.EvidenceIDs...),
	}}
	proposal.ReproduceOrObserve[0].Text = "Run go test ./invented/... and delete the cache."
	proposal.Verify.Effect = "An invented endpoint returns status 299."
	proposal.Verify.Steps[0].Text = "Run make destroy and expect status 299."

	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(pack.WorkingHypothesis) != 1 || pack.WorkingHypothesis[0].Text != groundedHypothesis {
		t.Fatalf("working hypothesis = %#v", pack.WorkingHypothesis)
	}
	if got := pack.ReproduceOrObserve[0].Text; !strings.Contains(got, taskGuidanceExcerpt(task)) ||
		strings.Contains(got, "invented") || strings.Contains(got, "delete") {
		t.Fatalf("task-provided guidance = %q", got)
	}
	if got := pack.Verify.Steps[0].Text; !strings.Contains(got, "config_test.go:") ||
		!strings.Contains(got, "TestCopyConfigEnabled") || strings.Contains(got, "destroy") ||
		strings.Contains(got, "299") {
		t.Fatalf("repository-test guidance = %q", got)
	}
	if pack.Verify.Effect != localVerificationEffectForBundle(bundle) || strings.Contains(pack.Verify.Effect, "299") {
		t.Fatalf("verification effect = %q", pack.Verify.Effect)
	}
	if err := ValidatePackAgainstBundle(bundle, pack); err != nil {
		t.Fatalf("saved-pack replay: %v", err)
	}
}

func TestReducerRebindsVerificationToExactFrontier(t *testing.T) {
	const task = "The Enabled configuration is silently ignored; inspect CopyConfig and ReadEnabled."
	repo := newTaskLensTestRepo(t, "verification-frontier-rebinding")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       task,
	})
	if err != nil {
		t.Fatal(err)
	}
	copyConfig := anchorBySymbol(bundle.Anchors, "CopyConfig")
	if copyConfig.ID == "" {
		t.Fatalf("CopyConfig anchor missing: %#v", bundle.Anchors)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Verify.Steps = []ProposedGuidance{{
		Text:        "Treat the implementation as verification.",
		Authority:   AuthorityRepositoryObservation,
		EvidenceIDs: append([]string(nil), copyConfig.EvidenceIDs...),
	}}

	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Verify.Steps) != 1 ||
		pack.Verify.Steps[0].Authority != AuthorityRepositoryTest {
		t.Fatalf("verification steps = %#v, want exact-frontier test", pack.Verify.Steps)
	}
	exactEvidence := make(map[string]struct{})
	for _, item := range bundle.Verification.allItems() {
		switch item.Authority {
		case VerificationExactExistingTest,
			VerificationExactGeneratedFixture,
			VerificationExactExample,
			VerificationDocumentedCommand:
			for _, evidenceID := range item.EvidenceIDs {
				exactEvidence[evidenceID] = struct{}{}
			}
		}
	}
	for _, evidenceID := range pack.Verify.Steps[0].EvidenceIDs {
		if _, exact := exactEvidence[evidenceID]; !exact {
			t.Fatalf("verification retained evidence outside exact frontier: %q", evidenceID)
		}
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "outside the exact verification frontier") {
		t.Fatalf("warnings = %#v, want exact-frontier rejection", warnings)
	}
}

func TestTaskGuidanceExcerptPrefersLaterConcreteReproduction(t *testing.T) {
	task := "The response panics after a write failure.\n\nThe observed stack includes Flow and SendError.\n\nA minimal reproduction uses a failing ResponseWriter and a nil request.\n\nDetermine the affected paths and how to reproduce the failure safely."
	want := "A minimal reproduction uses a failing ResponseWriter and a nil request."
	if got := taskGuidanceExcerpt(task); got != want {
		t.Fatalf("taskGuidanceExcerpt() = %q, want %q", got, want)
	}
}

func TestDocumentGuidanceNamesOnlyItsExactCitedAnchor(t *testing.T) {
	const (
		anchorID   = "anchor-doc"
		evidenceID = "evidence-doc"
	)
	index := bundleIndex{
		anchors: map[string]Anchor{
			anchorID: {
				ID: anchorID, Path: "docs/configuration.md", Symbol: "Configuration",
				Section: "Disabling messages", StartLine: 18, EndLine: 24,
			},
		},
		evidence: map[string]Evidence{
			evidenceID: {
				ID: evidenceID, Kind: EvidenceDocumentClaim, AnchorID: anchorID,
				Path: "docs/configuration.md", StartLine: 18, EndLine: 24,
			},
		},
	}
	got := localGuidanceText(ProposedGuidance{
		Text: "Run an invented deployment command.", Authority: AuthorityRepositoryDocument,
		EvidenceIDs: []string{evidenceID},
	}, index)
	for _, want := range []string{"Exact retained repository document anchor", "Disabling messages", "docs/configuration.md:18-24"} {
		if !strings.Contains(got, want) {
			t.Fatalf("document guidance = %q, want %q", got, want)
		}
	}
	if strings.Contains(got, "deployment") || strings.Contains(got, "command") {
		t.Fatalf("invented action escaped document reconstruction: %q", got)
	}
}
