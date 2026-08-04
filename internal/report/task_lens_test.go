package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/tasklens"
)

func TestReadTaskInvestigationProjectsValidatedArtifactsWithoutOpaqueEvidenceIDs(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	bundle, attempt, pack, status := taskInvestigationArtifactFixture(t)
	writeTaskInvestigationArtifacts(t, runDir, bundle, attempt, pack, status)

	workspace, warning := readTaskInvestigation(runDir, "")
	if warning != "" {
		t.Fatalf("readTaskInvestigation warning = %q", warning)
	}
	if workspace == nil || workspace.TaskID != pack.ID || workspace.Repository != pack.Repository.Identity ||
		workspace.Task != bundle.Task.Text || !workspace.Sufficient || len(workspace.Anchors) != 3 {
		t.Fatalf("task workspace = %#v", workspace)
	}
	areaIndexes := append([]int(nil), workspace.LikelyAreas[0].AnchorIndexes...)
	slices.Sort(areaIndexes)
	if !slices.Equal(areaIndexes, []int{0, 1, 2}) {
		t.Fatalf("likely area anchor indexes = %#v", workspace.LikelyAreas[0].AnchorIndexes)
	}
	if len(workspace.EvidenceJoins) != 1 || workspace.EvidenceJoins[0].LeftAnchor != 0 ||
		workspace.EvidenceJoins[0].RightAnchor != 1 ||
		!slices.Equal(workspace.EvidenceJoins[0].SupportAnchorIndexes, []int{0, 1}) {
		t.Fatalf("evidence joins = %#v", workspace.EvidenceJoins)
	}
	if len(workspace.WorkingHypothesis) != 1 ||
		!slices.Equal(workspace.WorkingHypothesis[0].SupportAnchorIndexes, []int{0, 1}) {
		t.Fatalf("working hypothesis = %#v", workspace.WorkingHypothesis)
	}
	if len(workspace.Verify.Steps) != 1 ||
		!slices.Equal(workspace.Verify.Steps[0].SupportAnchorIndexes, []int{2}) {
		t.Fatalf("verification citations = %#v", workspace.Verify.Steps)
	}
	if !slices.Contains(workspace.Interpretation.FoundTerms, "Validate") ||
		!slices.Contains(workspace.Interpretation.UserProvidedOnly, "reported failure") ||
		len(workspace.Warnings) != 0 {
		t.Fatalf("task truth boundary = %#v, warnings = %#v", workspace.Interpretation, workspace.Warnings)
	}
	for _, anchor := range workspace.Anchors {
		if err := anchor.Source.Validate(); err != nil {
			t.Fatalf("source projection for %s: %v", anchor.Path, err)
		}
		if anchor.Source.Revision != status.CapturedRevision || len(anchor.Source.RelatedEvidenceIDs) != 0 {
			t.Fatalf("source authority leaked or lost binding: %#v", anchor.Source)
		}
	}

	projected, err := json.Marshal(workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, opaque := range []string{
		pack.Anchors[0].ID,
		pack.Anchors[0].EvidenceIDs[0],
		pack.EvidenceJoins[0].ID,
		pack.EvidenceJoins[0].RelationID,
	} {
		if bytesContain(projected, opaque) {
			t.Fatalf("user projection contains opaque reducer id %q: %s", opaque, projected)
		}
	}
}

func TestReadRunDirMakesTaskAnchorsOpenableAndUsesCanonicalRepositoryIdentity(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	bundle, attempt, pack, status := taskInvestigationArtifactFixture(t)
	writeTaskInvestigationArtifacts(t, runDir, bundle, attempt, pack, status)
	if err := os.WriteFile(filepath.Join(runDir, "snapshot.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	if data.TaskInvestigation == nil || data.RepoName != bundle.Repository.Identity {
		t.Fatalf("task report identity = %q, task = %#v", data.RepoName, data.TaskInvestigation)
	}
	for _, expected := range bundle.AllowedPaths {
		if !slices.Contains(data.OpenablePaths, expected) {
			t.Fatalf("task anchor %q is not openable: %#v", expected, data.OpenablePaths)
		}
	}
}

func TestReadTaskInvestigationRejectsStatusThatIsNotBoundToSavedPack(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	bundle, attempt, pack, status := taskInvestigationArtifactFixture(t)
	writeTaskInvestigationArtifacts(t, runDir, bundle, attempt, pack, status)
	statusRaw, err := os.ReadFile(filepath.Join(runDir, tasklens.StatusFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(statusRaw, &status); err != nil {
		t.Fatal(err)
	}
	status.PackSHA256 = strings.Repeat("0", 64)
	if err := os.WriteFile(
		filepath.Join(runDir, tasklens.StatusFile),
		taskInvestigationJSON(t, status),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	workspace, warning := readTaskInvestigation(runDir, "")
	if workspace != nil || !strings.Contains(warning, "saved status does not match") {
		t.Fatalf("workspace = %#v, warning = %q", workspace, warning)
	}
}

func TestReadTaskInvestigationRejectsRetrievalTraceProjectionDrift(t *testing.T) {
	t.Parallel()

	runDir := t.TempDir()
	bundle, attempt, pack, status := taskInvestigationArtifactFixture(t)
	writeTaskInvestigationArtifacts(t, runDir, bundle, attempt, pack, status)

	tracePath := filepath.Join(runDir, tasklens.TraceJSONFile)
	var trace tasklens.RetrievalTrace
	if err := json.Unmarshal(taskInvestigationReadFile(t, tracePath), &trace); err != nil {
		t.Fatal(err)
	}
	if len(trace.TaskTerms) == 0 {
		t.Fatal("fixture retrieval trace has no task terms")
	}
	trace.TaskTerms[0].Text = "structurally-valid-projection-drift"
	traceRaw := taskInvestigationJSON(t, trace)
	traceMarkdown, err := tasklens.RenderRetrievalTraceMarkdown(trace)
	if err != nil {
		t.Fatal(err)
	}
	traceMarkdownRaw := []byte(traceMarkdown)
	if err := os.WriteFile(tracePath, traceRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir, tasklens.TraceMarkdownFile), traceMarkdownRaw, 0o600); err != nil {
		t.Fatal(err)
	}

	statusPath := filepath.Join(runDir, tasklens.StatusFile)
	if err := json.Unmarshal(taskInvestigationReadFile(t, statusPath), &status); err != nil {
		t.Fatal(err)
	}
	status.RetrievalTraceSHA256 = tasklens.SHA256(traceRaw)
	status.RetrievalTraceMarkdownSHA256 = tasklens.SHA256(traceMarkdownRaw)
	if err := os.WriteFile(statusPath, taskInvestigationJSON(t, status), 0o600); err != nil {
		t.Fatal(err)
	}

	workspace, warning := readTaskInvestigation(runDir, "")
	if workspace != nil || !strings.Contains(warning, "task terms differ from bundle") {
		t.Fatalf("workspace = %#v, warning = %q", workspace, warning)
	}
}

func TestBindTaskInvestigationRevisionRebindsSourcePresentationIdentity(t *testing.T) {
	t.Parallel()

	bundle, attempt, pack, status := taskInvestigationArtifactFixture(t)
	workspace, err := projectTaskInvestigation(bundle, attempt, pack, status)
	if err != nil {
		t.Fatal(err)
	}
	before := workspace.Anchors[0].Source.PresentationSHA256
	bindTaskInvestigationRevision(workspace, "captured-authority-revision")
	if workspace.CapturedRevision != "captured-authority-revision" ||
		workspace.Anchors[0].Source.Revision != "captured-authority-revision" ||
		workspace.Anchors[0].Source.PresentationSHA256 == before {
		t.Fatalf("task source revision was not rebound: %#v", workspace.Anchors[0].Source)
	}
	if err := workspace.Anchors[0].Source.Validate(); err != nil {
		t.Fatalf("rebound source: %v", err)
	}
}

func TestTaskInvestigationPackSufficientRequiresGroundedContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		attemptState string
		mutate       func(*tasklens.Pack)
		want         bool
	}{
		{name: "accepted grounded pack", attemptState: "accepted", want: true},
		{name: "accepted with rejections", attemptState: "accepted_with_rejections", want: true},
		{
			name: "missing reproduce authority", attemptState: "accepted", want: false,
			mutate: func(pack *tasklens.Pack) {
				pack.ReproduceOrObserve[0].Authority = tasklens.AuthorityMissing
				pack.ReproduceOrObserve[0].EvidenceIDs = nil
			},
		},
		{
			name: "sparse local skip remains partial", attemptState: "skipped_insufficient_evidence", want: false,
			mutate: func(pack *tasklens.Pack) { pack.Locality = tasklens.LocalityLocalExact },
		},
		{
			name: "offline local exact remains partial without cheap exit", attemptState: "skipped_offline", want: false,
			mutate: func(pack *tasklens.Pack) { pack.Locality = tasklens.LocalityLocalExact },
		},
		{
			name: "offline cross file remains partial", attemptState: "skipped_offline", want: false,
		},
		{
			name: "broad accepted remains partial", attemptState: "accepted", want: false,
			mutate: func(pack *tasklens.Pack) { pack.Locality = tasklens.LocalityBroadDynamic },
		},
		{
			name: "generic one-anchor plausible hypothesis remains partial", attemptState: "accepted", want: false,
			mutate: func(pack *tasklens.Pack) {
				pack.WorkingHypothesis[0].Status = tasklens.HypothesisPlausible
				pack.WorkingHypothesis[0].Text = "A relationship involving the retained evidence for Validate is plausible, but runtime sequence and causality are not established."
				pack.WorkingHypothesis[0].SupportIDs = pack.Anchors[1].EvidenceIDs
				pack.WorkingHypothesis[0].RelationIDs = nil
			},
		},
		{
			name: "shared-state alias does not ground unrelated verification", attemptState: "accepted", want: false,
			mutate: func(pack *tasklens.Pack) {
				pack.Anchors[2].Symbol = "TestUnrelatedSecurity"
				pack.Anchors[2].Path = "internal/unrelated_test.go"
				pack.Anchors[2].Excerpt = []tasklens.SourceLine{{Line: 30, Text: "func TestUnrelatedSecurity(t *testing.T) {}"}}
				for index := range pack.EvidenceJoins {
					pack.EvidenceJoins[index].Kind = string(tasklens.RelationSharedStateAlias)
				}
			},
		},
		{
			name: "unknown-scope relation does not ground unrelated verification", attemptState: "accepted", want: false,
			mutate: func(pack *tasklens.Pack) {
				pack.Anchors[2].Symbol = "TestUnrelatedSecurity"
				pack.Anchors[2].Path = "internal/unrelated_test.go"
				pack.Anchors[2].Excerpt = []tasklens.SourceLine{{Line: 30, Text: "func TestUnrelatedSecurity(t *testing.T) {}"}}
				for index := range pack.EvidenceJoins {
					pack.EvidenceJoins[index].Kind = string(tasklens.RelationScopeUnknown)
				}
			},
		},
		{
			name: "task-provided verification remains partial", attemptState: "accepted", want: false,
			mutate: func(pack *tasklens.Pack) {
				pack.Verify.Steps = []tasklens.Guidance{{
					Text:        pack.ReproduceOrObserve[0].Text,
					Authority:   tasklens.AuthorityTaskProvided,
					EvidenceIDs: append([]string(nil), pack.ReproduceOrObserve[0].EvidenceIDs...),
				}}
			},
		},
		{
			name: "verification guidance outside exact frontier remains partial", attemptState: "accepted", want: false,
			mutate: func(pack *tasklens.Pack) {
				pack.Verify.Steps = []tasklens.Guidance{{
					Text:        "Inspect the retained validation implementation.",
					Authority:   tasklens.AuthorityRepositoryObservation,
					EvidenceIDs: append([]string(nil), pack.Anchors[1].EvidenceIDs...),
				}}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, pack, _ := taskInvestigationArtifactFixture(t)
			if test.mutate != nil {
				test.mutate(&pack)
			}
			if got := TaskInvestigationPackSufficient(pack, test.attemptState); got != test.want {
				t.Fatalf("TaskInvestigationPackSufficient() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestTaskInvestigationConfigurationPackRejectsVisibleAuxiliaryAnchor(t *testing.T) {
	t.Parallel()

	contract, err := tasklens.DefaultRoleContract(tasklens.TaskProfileConfigurationPropagation)
	if err != nil {
		t.Fatal(err)
	}
	sourceID := tasklens.OpaqueID("anchor", "config-source")
	copyID := tasklens.OpaqueID("anchor", "config-copy")
	destinationID := tasklens.OpaqueID("anchor", "config-destination")
	testID := tasklens.OpaqueID("anchor", "config-test")
	auxiliaryID := tasklens.OpaqueID("anchor", "auxiliary")
	decisiveID := tasklens.OpaqueID("relation", "config-copy-destination")
	testRelationID := tasklens.OpaqueID("relation", "config-test-copy")
	evidence := func(anchorID string) string {
		return tasklens.OpaqueID("evidence", anchorID)
	}
	coverage := tasklens.RoleCoverage{
		Profile: tasklens.TaskProfileConfigurationPropagation,
		Key: []tasklens.RoleCoverageItem{
			{Role: tasklens.RoleConfigurationSource, MinimumAnchors: 1, AnchorIDs: []string{sourceID}, Represented: true},
			{Role: tasklens.RoleConfigurationCopy, MinimumAnchors: 1, AnchorIDs: []string{copyID}, Represented: true},
			{Role: tasklens.RoleEffectiveDestination, MinimumAnchors: 1, AnchorIDs: []string{destinationID}, Represented: true},
		},
		Supporting: []tasklens.RoleCoverageItem{
			{Role: tasklens.RoleVerificationAnchor, MinimumAnchors: 1, AnchorIDs: []string{testID}, Represented: true},
		},
		Optional: []tasklens.RoleCoverageItem{
			{Role: tasklens.RoleDocumentationContract, MinimumAnchors: 1, AnchorIDs: []string{}, Represented: false},
		},
	}
	pack := tasklens.Pack{
		Locality:     tasklens.LocalityBoundedCrossFile,
		Profile:      tasklens.TaskProfileConfigurationPropagation,
		RoleContract: contract,
		RoleCoverage: coverage,
		Interpretation: tasklens.Interpretation{
			FoundTerms: []string{"Enabled"},
		},
		Anchors: []tasklens.InvestigationAnchor{
			{ID: sourceID, Path: "config.go", Symbol: "Config", Role: tasklens.RoleConfigurationSource, EvidenceIDs: []string{evidence(sourceID)}},
			{ID: copyID, Path: "options.go", Symbol: "CopyConfig", Role: tasklens.RoleConfigurationCopy, EvidenceIDs: []string{evidence(copyID)}},
			{ID: destinationID, Path: "server.go", Symbol: "Server", Role: tasklens.RoleEffectiveDestination, EvidenceIDs: []string{evidence(destinationID)}},
			{ID: testID, Path: "server_test.go", Symbol: "TestConfig", Role: tasklens.RoleVerificationAnchor, EvidenceIDs: []string{evidence(testID)}},
		},
		EvidenceJoins: []tasklens.EvidenceJoin{
			{
				LeftID: copyID, RightID: destinationID, RelationID: decisiveID,
				Kind: string(tasklens.RelationConfigApplied), SupportType: tasklens.SupportLocallyObserved,
				SupportIDs: []string{evidence(copyID), evidence(destinationID)},
			},
			{
				LeftID: testID, RightID: copyID, RelationID: testRelationID,
				Kind: string(tasklens.RelationTestExercises), SupportType: tasklens.SupportLocallyObserved,
				SupportIDs: []string{evidence(testID), evidence(copyID)},
			},
		},
		DecisiveRelationID: decisiveID,
		VerificationFrontier: tasklens.VerificationFrontier{
			DecisiveAnchorID: copyID,
			Anchors: []tasklens.VerificationItem{{
				ID:        tasklens.OpaqueID("verification", "config-test"),
				Authority: tasklens.VerificationExactExistingTest, AnchorID: testID,
				Path: "server_test.go", Symbol: "TestConfig",
				EvidenceIDs: []string{evidence(testID)}, Text: "Exact configuration test.",
			}},
		},
		WorkingHypothesis: []tasklens.HypothesisClause{{
			Status: tasklens.HypothesisSupported, RelationIDs: []string{decisiveID},
			SupportIDs: []string{evidence(copyID), evidence(destinationID)},
		}},
		ReproduceOrObserve: []tasklens.Guidance{{
			Authority:   tasklens.AuthorityRepositoryObservation,
			EvidenceIDs: []string{evidence(sourceID)},
		}},
		Verify: tasklens.Verification{Steps: []tasklens.Guidance{{
			Authority:   tasklens.AuthorityRepositoryTest,
			EvidenceIDs: []string{evidence(testID)},
		}}},
	}
	if !TaskInvestigationPackSufficient(pack, "accepted") {
		t.Fatal("grounded configuration pack should be sufficient before the adversarial anchor")
	}
	pack.Anchors = append(pack.Anchors, tasklens.InvestigationAnchor{
		ID: auxiliaryID, Path: "auxiliary.go", Symbol: "Auxiliary",
		Role:        tasklens.RoleRepresentativeImplementation,
		EvidenceIDs: []string{evidence(auxiliaryID)},
	})
	if TaskInvestigationPackSufficient(pack, "accepted") {
		t.Fatal("generic Auxiliary outside all task-relevance lanes remained sufficient")
	}
}

func TestTaskInvestigationActionableExtensionRejectsWeakLocalJoins(t *testing.T) {
	t.Parallel()

	const (
		entryID    = "anchor-entry"
		verifyID   = "anchor-verify"
		evidenceID = "evidence-verify"
	)
	base := tasklens.Pack{
		Anchors: []tasklens.InvestigationAnchor{
			{ID: entryID, Role: tasklens.RoleIntegrationBoundary},
			{ID: verifyID, Role: tasklens.RoleVerificationAnchor, EvidenceIDs: []string{evidenceID}},
		},
		LikelyAreas: []tasklens.Area{{Label: "entry"}, {Label: "verification"}},
		Verify: tasklens.Verification{Steps: []tasklens.Guidance{{
			Authority:   tasklens.AuthorityRepositoryTest,
			EvidenceIDs: []string{evidenceID},
		}}},
	}
	tests := []struct {
		name string
		kind string
		want bool
	}{
		{name: "direct call", kind: string(tasklens.RelationDirectCall), want: true},
		{name: "shared state alias", kind: string(tasklens.RelationSharedStateAlias)},
		{name: "scope unknown", kind: string(tasklens.RelationScopeUnknown)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pack := base
			pack.EvidenceJoins = []tasklens.EvidenceJoin{{
				LeftID: entryID, RightID: verifyID,
				Kind: test.kind, SupportType: tasklens.SupportLocallyObserved,
			}}
			if got := taskInvestigationHasActionableExtensionEvidence(pack); got != test.want {
				t.Fatalf("taskInvestigationHasActionableExtensionEvidence() = %t, want %t", got, test.want)
			}
		})
	}
}

func taskInvestigationArtifactFixture(
	t *testing.T,
) (tasklens.Bundle, tasklens.Attempt, tasklens.Pack, tasklens.Status) {
	t.Helper()
	repository := tasklens.Repository{
		Identity: "example.test/fuego", DisplayName: "task-labelled-checkout",
		Revision: "base-revision", TreeHash: "base-tree",
		StateSHA256: strings.Repeat("a", 64), IdentitySource: "remote",
	}
	taskText := "The request fails when `Validate` sees the `reported failure`; verify the request path."
	taskEvidenceID := tasklens.OpaqueID("evidence", "task", tasklens.SHA256([]byte(taskText)))
	paths := []string{"internal/api.go", "internal/service.go", "internal/service_test.go"}
	symbols := []string{"Handle", "Validate", "TestValidate"}
	excerpts := [][]tasklens.SourceLine{
		{{Line: 10, Text: "func Handle() {", Highlight: true}, {Line: 11, Text: "\tValidate()"}, {Line: 12, Text: "}"}},
		{{Line: 20, Text: "func Validate() error {", Highlight: true}, {Line: 21, Text: "\treturn nil"}, {Line: 22, Text: "}"}},
		{{Line: 30, Text: "func TestValidate(t *testing.T) {", Highlight: true}, {Line: 31, Text: "\t_ = Validate()"}, {Line: 32, Text: "}"}},
	}
	anchorIDs := make([]string, len(paths))
	evidenceIDs := make([]string, len(paths))
	for index := range paths {
		start, end := excerpts[index][0].Line, excerpts[index][len(excerpts[index])-1].Line
		excerptSHA := tasklens.SourceExcerptSHA256(excerpts[index])
		anchorIDs[index] = tasklens.OpaqueID(
			"anchor", paths[index], symbols[index], fmt.Sprintf("%d", start), fmt.Sprintf("%d", end), excerptSHA,
		)
		evidenceIDs[index] = tasklens.OpaqueID(
			"evidence", repository.StateSHA256, paths[index], fmt.Sprintf("%d", start), fmt.Sprintf("%d", end), excerptSHA,
		)
	}
	anchors := make([]tasklens.Anchor, 0, len(paths))
	evidence := []tasklens.Evidence{{
		ID: taskEvidenceID, Kind: tasklens.EvidenceTaskProvided,
		Summary: "Symptom or requested outcome supplied by the task; not repository truth.",
	}}
	for index := range paths {
		anchors = append(anchors, tasklens.Anchor{
			ID: anchorIDs[index], Path: paths[index], Symbol: symbols[index], Package: "internal",
			StartLine: excerpts[index][0].Line, EndLine: excerpts[index][len(excerpts[index])-1].Line,
			Excerpt: excerpts[index], EvidenceIDs: []string{evidenceIDs[index]},
			Scope: tasklens.SourceScope{
				ScopeKind:             tasklens.SourceScopeCompleteEnclosingSymbol,
				ScopeStart:            excerpts[index][0].Line,
				ScopeEnd:              excerpts[index][len(excerpts[index])-1].Line,
				SourceTotalLines:      32,
				NegativeClaimsAllowed: true,
				NegativeEvidenceBasis: tasklens.NegativeEvidenceCompleteScope,
			},
		})
		evidence = append(evidence, tasklens.Evidence{
			ID: evidenceIDs[index], Kind: tasklens.EvidenceRepositoryFact,
			Path: paths[index], StartLine: excerpts[index][0].Line,
			EndLine:  excerpts[index][len(excerpts[index])-1].Line,
			AnchorID: anchorIDs[index],
			Summary: fmt.Sprintf(
				"Exact repository source excerpt for %s at %s:%d-%d.",
				symbols[index], paths[index], excerpts[index][0].Line, excerpts[index][len(excerpts[index])-1].Line,
			),
		})
	}
	kindHint, observableHint := tasklens.GroundedTaskClassification(taskText)
	bundle := tasklens.Bundle{
		Version: tasklens.BundleVersion,
		ID: tasklens.OpaqueID(
			"task", repository.Identity, repository.Revision, repository.StateSHA256, tasklens.SHA256([]byte(taskText)),
		),
		Repository: repository,
		Task: tasklens.Task{
			Text: taskText, EvidenceID: taskEvidenceID,
		},
		KindHint: kindHint, ObservableHint: observableHint,
		Anchors: anchors, Evidence: evidence, AllowedPaths: paths,
		StagesSkipped: tasklens.CanonicalStagesSkipped(),
		Budgets: tasklens.Budgets{
			InitialCandidates: 7, CandidateItemsFound: 7,
			RetainedAnchors: 3, AnchorItemsFound: 3, EvidenceFilesConsidered: 7, ReadFiles: 3,
			ReadBytes: 256, GoplsQueries: 1, FrontierExpansions: 1, LocalWallMillis: 42,
		},
		Metrics: tasklens.RetrievalMetrics{TrackedFiles: 7, ASTParses: 3, EvidenceFilesRead: 3},
	}
	roles := make([]tasklens.AnchorRole, len(bundle.Anchors))
	for index := range bundle.Anchors {
		bundle.Anchors[index].RoleHints = tasklens.GroundedAnchorRoles(
			bundle.Anchors[index], bundle.KindHint, bundle.Task.Text,
		)
		roles[index] = bundle.Anchors[index].RoleHints[0]
	}
	bundle.Terms = tasklens.GroundedTaskTerms(taskText, bundle.Anchors)
	bundle.Budgets.RetainedSourceBytes = tasklens.RetainedSourceByteCount(bundle.Anchors)
	bundle.Relations = tasklens.GroundedRelations(bundle.Anchors, bundle.Terms)
	bundle.Locality = tasklens.GroundedLocality(
		taskText, bundle.Terms, bundle.Anchors, bundle.Relations,
	)
	bundle.Metrics.RelationsRetained = len(bundle.Relations)
	if err := tasklens.GroundV01Contract(&bundle); err != nil {
		t.Fatal(err)
	}
	var directRelation tasklens.Relation
	for _, relation := range bundle.Relations {
		if relation.LeftID == anchorIDs[0] && relation.RightID == anchorIDs[1] &&
			relation.Kind == string(tasklens.RelationDirectCall) {
			directRelation = relation
			break
		}
	}
	if directRelation.ID == "" {
		t.Fatal("fixture lacks deterministic Handle -> Validate relation")
	}
	relationID := directRelation.ID
	relationSupport := directRelation.EvidenceIDs
	bundleSHA, err := tasklens.BundleHash(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal := tasklens.Proposal{
		Version: tasklens.ProposalVersion,
		Interpretation: tasklens.ProposedInterpretation{
			Restatement: "Trace validation from the request boundary.",
			Kind:        tasklens.TaskBug, Observable: "The request path validates without the reported failure.",
		},
		Areas: []tasklens.ProposedArea{{
			Label: "Request validation", Why: "The selected anchors bound the request validation path.",
			TargetIDs: func() []string {
				ids := append([]string(nil), anchorIDs...)
				slices.Sort(ids)
				return ids
			}(),
		}},
		Joins: []tasklens.ProposedJoin{{
			LeftID: anchorIDs[0], RightID: anchorIDs[1], RelationID: relationID,
			Kind: string(tasklens.RelationDirectCall), SupportType: tasklens.SupportLocallyObserved,
			SupportIDs:  relationSupport,
			Explanation: "The retained left-anchor excerpt contains an unqualified call expression named by the retained right anchor.",
			Scope:       "A direct call expression is present in the retained caller excerpt; this does not prove runtime reachability, order, or callee behavior.",
		}},
		Hypothesis: []tasklens.ProposedClause{{
			Status:     tasklens.HypothesisSupported,
			Text:       "The retained left-anchor excerpt contains an unqualified call expression named by the retained right anchor.",
			SupportIDs: relationSupport, RelationIDs: []string{relationID},
		}},
		ReproduceOrObserve: []tasklens.ProposedGuidance{{
			Text: "Use the task-provided failing request.", Authority: tasklens.AuthorityTaskProvided,
			EvidenceIDs: []string{taskEvidenceID},
		}},
		Verify: tasklens.ProposedVerification{
			Effect: "The request returns the intended result.",
			Steps: []tasklens.ProposedGuidance{{
				Text: "Inspect the focused validation test.", Authority: tasklens.AuthorityRepositoryTest,
				EvidenceIDs: []string{evidenceIDs[2]},
			}},
		},
		NextProbes: []tasklens.ProposedProbe{{
			Action:    tasklens.ProbeResolveReference,
			AnchorIDs: []string{anchorIDs[1]}, Text: "Resolve callers outside the retained source window.",
		}},
	}
	for index, anchorID := range anchorIDs {
		proposal.Anchors = append(proposal.Anchors, tasklens.ProposedAnchor{
			AnchorID: anchorID, Role: roles[index],
			Why: "This exact retained anchor is relevant to request validation.",
		})
	}
	pack, reductionWarnings, err := tasklens.ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(reductionWarnings) != 0 {
		t.Fatalf("fixture proposal was unexpectedly reduced: %#v", reductionWarnings)
	}
	responseRaw, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	attempt := tasklens.Attempt{
		Version: tasklens.AttemptVersion, BundleSHA256: bundleSHA,
		PromptVersion: tasklens.PromptVersion, State: "accepted",
		ResponseSHA256: tasklens.SHA256(responseRaw), RawResponse: string(responseRaw),
		Provider: tasklens.ProviderMetrics{
			Calls: 1, TransportAttempts: 1, RequestBytes: 128,
			ResponseBytes: len(responseRaw), InputTokens: 30, OutputTokens: 20, LatencyMillis: 12,
		},
	}
	stablePrompt, err := tasklens.StablePromptJSON(bundle)
	if err != nil {
		t.Fatal(err)
	}
	attempt.PromptSHA256 = tasklens.SHA256(stablePrompt)
	status := tasklens.Status{
		Version: tasklens.StatusVersion, State: "accepted", Sufficient: true,
		TaskID: pack.ID, BundleSHA256: bundleSHA,
		CapturedRevision: repository.Revision, TreeHash: repository.TreeHash,
		Locality: pack.Locality, StagesSkipped: pack.StagesSkipped,
		Provider: attempt.Provider, Budgets: pack.Budgets, CheapExit: pack.CheapExit,
	}
	return bundle, attempt, pack, status
}

func writeTaskInvestigationArtifacts(
	t *testing.T,
	runDir string,
	bundle tasklens.Bundle,
	attempt tasklens.Attempt,
	pack tasklens.Pack,
	status tasklens.Status,
) {
	t.Helper()
	bundleRaw := taskInvestigationJSON(t, bundle)
	attemptRaw := taskInvestigationJSON(t, attempt)
	packRaw := taskInvestigationJSON(t, pack)
	trace, err := tasklens.GroundedSelectedRetrievalTrace(bundle)
	if err != nil {
		t.Fatal(err)
	}
	traceRaw := taskInvestigationJSON(t, trace)
	traceMarkdown, err := tasklens.RenderRetrievalTraceMarkdown(trace)
	if err != nil {
		t.Fatal(err)
	}
	traceMarkdownRaw := []byte(traceMarkdown)
	status.AttemptSHA256 = tasklens.SHA256(attemptRaw)
	status.PackSHA256 = tasklens.SHA256(packRaw)
	status.RetrievalTraceSHA256 = tasklens.SHA256(traceRaw)
	status.RetrievalTraceMarkdownSHA256 = tasklens.SHA256(traceMarkdownRaw)
	statusRaw := taskInvestigationJSON(t, status)
	for name, raw := range map[string][]byte{
		tasklens.BundleFile: bundleRaw, tasklens.AttemptFile: attemptRaw,
		tasklens.PackFile: packRaw, tasklens.StatusFile: statusRaw,
		tasklens.TraceJSONFile: traceRaw, tasklens.TraceMarkdownFile: traceMarkdownRaw,
	} {
		if err := os.WriteFile(filepath.Join(runDir, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func taskInvestigationJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(raw, '\n')
}

func taskInvestigationReadFile(t *testing.T, filePath string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func bytesContain(raw []byte, value string) bool {
	return strings.Contains(string(raw), value)
}
