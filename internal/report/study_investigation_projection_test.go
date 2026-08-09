package report

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/freshness"
	"github.com/dvordrova/repomap/internal/mechanismstudy"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
	"github.com/dvordrova/repomap/internal/themestudy"
)

const (
	studyInvestigationMainSymbol   = "example.com/repo/cmd/app.main.private-symbol-marker"
	studyInvestigationWorkerSymbol = "example.com/repo/internal/worker.Run"
	studyInvestigationStoreSymbol  = "example.com/repo/internal/store.Save"
)

func TestProjectStudyInvestigationsPublishesOnlyExactPublicAuthority(t *testing.T) {
	themes, canvas, graph, input, openable := studyInvestigationProjectionFixture()
	if err := ProjectStudyInvestigations(themes, canvas, graph, openable, input); err != nil {
		t.Fatal(err)
	}

	prepared := themes.Cards[0].Investigation
	if prepared == nil || prepared.Version != StudyInvestigationVersion ||
		prepared.Outcome != StudyInvestigationOutcomePrepared ||
		!reflect.DeepEqual(prepared.ReadingOrdinals, []int{1}) ||
		prepared.Mechanisms == nil || len(prepared.Mechanisms) != 0 {
		t.Fatalf("prepared investigation = %#v", prepared)
	}

	got := themes.Cards[1].Investigation
	if got == nil || got.ID != "study-investigation-2" ||
		got.Outcome != StudyInvestigationOutcomeMechanism ||
		!reflect.DeepEqual(got.ReadingOrdinals, []int{1, 2, 3}) || len(got.Mechanisms) != 2 {
		t.Fatalf("mechanism investigation = %#v", got)
	}
	mechanism := got.Mechanisms[0]
	if mechanism.ID != "study-investigation-2-mechanism-1" || mechanism.Ordinal != 1 ||
		!reflect.DeepEqual(mechanism.ReadingOrdinals, []int{1, 3}) ||
		len(mechanism.Nodes) != 3 || len(mechanism.Edges) != 2 {
		t.Fatalf("first mechanism = %#v", mechanism)
	}
	if node := mechanism.Nodes[0]; node.ID != "study-investigation-2-mechanism-1-node-1" ||
		node.Label != "app.main" ||
		node.Declaration != (UserCodeLocation{Path: "cmd/app/main.go", Line: 10, Column: 6}) ||
		!reflect.DeepEqual(node.ComponentIDs, []componentmap.ComponentID{"component-entry"}) {
		t.Fatalf("direct declaration node = %#v", node)
	}
	if node := mechanism.Nodes[1]; !reflect.DeepEqual(
		node.ComponentIDs,
		[]componentmap.ComponentID{"component-worker-a", "component-worker-b"},
	) {
		t.Fatalf("plural exact package node = %#v", node)
	}
	if node := mechanism.Nodes[2]; node.ComponentIDs == nil || len(node.ComponentIDs) != 0 {
		t.Fatalf("zero/remainder-only node = %#v", node)
	}
	if edge := mechanism.Edges[0]; edge.FromNodeID != mechanism.Nodes[0].ID ||
		edge.ToNodeID != mechanism.Nodes[1].ID ||
		edge.Invocation != surfacediscovery.DirectCallSynchronous || edge.WitnessCount != 2 ||
		edge.Callsite != (UserCodeLocation{Path: "cmd/app/main.go", Line: 12, Column: 3}) {
		t.Fatalf("first exact edge = %#v", edge)
	}
	if edge := mechanism.Edges[1]; edge.Invocation != surfacediscovery.DirectCallGoroutine {
		t.Fatalf("goroutine edge = %#v", edge)
	}

	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	encoded := string(raw)
	for _, forbidden := range []string{
		"private-symbol-marker", "example.com/repo", `"frontier"`, `"status"`,
		`"issues"`, `"card_ref"`, `"edge_refs"`, `"node_refs"`, `"reading_refs"`,
	} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("public investigation leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestStudyInvestigationPublicationUsesExactEightCardPrefix(t *testing.T) {
	themes := themestudy.StudyThemes{
		Version:  themestudy.StudyThemesVersion,
		Revision: strings.Repeat("a", 40),
		Cards:    make([]themestudy.ThemeCard, 12),
	}
	publication := make([]mechanismstudy.PublicationCard, mechanismstudy.MaxCards)
	projection := &AtlasStudyThemesProjection{Total: 12, Shown: 12, Cards: make([]StudyThemeCard, 12)}
	for index := range themes.Cards {
		ordinal := index + 1
		themes.Cards[index] = themestudy.ThemeCard{
			Ordinal: ordinal, CanonicalID: fmt.Sprintf("theme-%02d", ordinal),
		}
		projection.Cards[index] = StudyThemeCard{Ordinal: ordinal}
		if index < mechanismstudy.MaxCards {
			publication[index] = mechanismstudy.PublicationCard{
				StudyOrdinal: ordinal, StudyCanonicalID: themes.Cards[index].CanonicalID,
				Outcome:         mechanismstudy.OutcomePrepared,
				ReadingOrdinals: []int{}, Mechanisms: []mechanismstudy.PublicationMechanism{},
			}
		}
	}
	if err := validateStudyInvestigationPublicationCards(themes, publication); err != nil {
		t.Fatalf("validate exact prefix: %v", err)
	}
	input, err := StudyInvestigationInputFromPublicationCards(publication)
	if err != nil {
		t.Fatalf("adapt exact prefix: %v", err)
	}
	if err := ProjectStudyInvestigations(projection, nil, nil, nil, input); err != nil {
		t.Fatalf("project exact prefix: %v", err)
	}
	for index, card := range projection.Cards {
		if index < mechanismstudy.MaxCards && card.Investigation == nil {
			t.Fatalf("prefix card %d lacks prepared investigation", index+1)
		}
		if index >= mechanismstudy.MaxCards && card.Investigation != nil {
			t.Fatalf("card %d beyond bounded prefix gained investigation", index+1)
		}
	}
}

func TestStudyInvestigationExactJoinUsesDeclarationBeforePackageFallback(t *testing.T) {
	_, canvas, graph, _, _ := studyInvestigationProjectionFixture()
	location := evidence.Location{Path: "internal/worker/run.go", Line: 22, Column: 1}
	canvas.Components = append(canvas.Components, ArchitectureComponent{
		ID: "component-worker-direct",
		Members: []componentmap.Candidate{studyInvestigationSymbolMember(
			"member-worker-direct",
			studyInvestigationWorkerSymbol,
			location,
		)},
	})

	got := architectureComponentIDsForExactDeclaration(
		canvas,
		graph,
		studyInvestigationWorkerSymbol,
		location,
	)
	if !reflect.DeepEqual(got, []componentmap.ComponentID{"component-worker-direct"}) {
		t.Fatalf("package fallback augmented exact declaration ownership: %#v", got)
	}

	// Neither a structural edge nor a suggestive component name can become a
	// fallback authority when the exact declaration/package joins are absent.
	canvas.StructuralEdges = append(canvas.StructuralEdges, ArchitectureStructuralEdge{
		FromComponentID: "component-entry",
		ToComponentID:   "component-lookalike",
	})
	got = architectureComponentIDsForExactDeclaration(
		canvas,
		graph,
		"example.com/repo/internal/missing.Run",
		evidence.Location{Path: "internal/missing/run.go", Line: 9, Column: 1},
	)
	if got == nil || len(got) != 0 {
		t.Fatalf("non-authoritative structure/name created ownership: %#v", got)
	}
}

func TestStudyInvestigationProjectionIsPermutationStable(t *testing.T) {
	firstThemes, firstCanvas, firstGraph, firstInput, firstOpenable := studyInvestigationProjectionFixture()
	if err := ProjectStudyInvestigations(
		firstThemes,
		firstCanvas,
		firstGraph,
		firstOpenable,
		firstInput,
	); err != nil {
		t.Fatal(err)
	}
	first, err := json.Marshal(firstThemes)
	if err != nil {
		t.Fatal(err)
	}

	secondThemes, secondCanvas, secondGraph, secondInput, secondOpenable := studyInvestigationProjectionFixture()
	slices.Reverse(secondCanvas.Components)
	for index := range secondCanvas.Components {
		slices.Reverse(secondCanvas.Components[index].Members)
		slices.Reverse(secondCanvas.Components[index].SharedMembers)
	}
	slices.Reverse(secondGraph.Packages)
	for index := range secondGraph.Packages {
		slices.Reverse(secondGraph.Packages[index].Files)
	}
	slices.Reverse(secondInput.Cards)
	for index := range secondInput.Cards {
		slices.Reverse(secondInput.Cards[index].ReadingOrdinals)
		slices.Reverse(secondInput.Cards[index].Mechanisms)
		for mechanismIndex := range secondInput.Cards[index].Mechanisms {
			slices.Reverse(secondInput.Cards[index].Mechanisms[mechanismIndex].ReadingOrdinals)
		}
	}
	slices.Reverse(secondOpenable)
	if err := ProjectStudyInvestigations(
		secondThemes,
		secondCanvas,
		secondGraph,
		secondOpenable,
		secondInput,
	); err != nil {
		t.Fatal(err)
	}
	second, err := json.Marshal(secondThemes)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("input permutation changed projection:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestCollectStudyInvestigationSourceLocationsIsExactBoundedAndStable(t *testing.T) {
	_, _, _, input, _ := studyInvestigationProjectionFixture()
	first, err := CollectStudyInvestigationSourceLocations(input)
	if err != nil {
		t.Fatal(err)
	}
	slices.Reverse(input.Cards)
	for index := range input.Cards {
		slices.Reverse(input.Cards[index].Mechanisms)
	}
	second, err := CollectStudyInvestigationSourceLocations(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("preflight changed under permutation:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first) != 10 {
		t.Fatalf("source location count = %d, want 10: %#v", len(first), first)
	}
	for index, location := range first {
		if index > 0 && !studyInvestigationTestLocationLess(first[index-1], location) {
			t.Fatalf("source locations are not unique canonical order: %#v", first)
		}
	}
}

func TestPrepareAuthorizedStudyInvestigationSourceCoverageExtendsExactAuthority(t *testing.T) {
	themes, canvas, graph, input, openable := studyInvestigationProjectionFixture()
	files := map[string]map[int]string{
		"cmd/app/main.go": {
			10: "func main() {}",
			12: "worker.Run()",
		},
		"internal/store/save.go": {
			30: "func Save() {}",
		},
		"internal/worker/run.go": {
			22: "func Run() {}",
			25: "go Save()",
		},
		"prepare.go":  {3: "func Prepare() {}"},
		"z/finish.go": {12: "func finish() {}"},
		"z/middle.go": {
			8:  "func middle() {}",
			10: "finish()",
		},
		"z/start.go": {
			4: "func start() {}",
			6: "defer middle()",
		},
	}
	authority := studyInvestigationSourceAuthority(t, files)
	// Simulate the ordinary pre-mechanism authority: only a pre-existing Study
	// path was captured. The explicit seam must extend it atomically.
	authority.inputs = slices.DeleteFunc(authority.inputs, func(input freshness.CapturedInput) bool {
		return input.Path != "prepare.go"
	})
	data := &ReportData{
		CapturedRevision: authority.repository.Head,
		OpenablePaths:    []string{"prepare.go"},
	}
	if err := PrepareAuthorizedStudyInvestigationSourceCoverage(
		context.Background(),
		data,
		&authority,
		input,
	); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(data.OpenablePaths, openable) {
		t.Fatalf("extended openable paths = %#v, want %#v", data.OpenablePaths, openable)
	}
	if len(authority.inputs) != len(openable) {
		t.Fatalf("captured inputs = %d, want %d", len(authority.inputs), len(openable))
	}
	locations, err := CollectStudyInvestigationSourceLocations(input)
	if err != nil {
		t.Fatal(err)
	}
	targets := overviewSourceTargets(data)
	for _, location := range locations {
		found := false
		for _, target := range targets {
			if target.path != location.Path || target.line != location.Line {
				continue
			}
			resolved, conflict := resolveOverviewSourceSnippet(data.UserSources, target)
			if conflict || !resolved {
				t.Fatalf("source target %s:%d was not exactly hydrated", location.Path, location.Line)
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("source target %s:%d was not authorized", location.Path, location.Line)
		}
	}

	if err := ProjectStudyInvestigations(
		themes,
		canvas,
		graph,
		data.OpenablePaths,
		input,
	); err != nil {
		t.Fatal(err)
	}
	data.AtlasStudy = &AtlasStudyReportStatus{Themes: themes}
	data.studyInvestigationSourceLocations = nil
	before := append([]SourceSnippet(nil), data.UserSources...)
	if err := PrepareAuthorizedSourceCoverage(context.Background(), data, &authority); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, data.UserSources) {
		t.Fatal("later ordinary coverage discarded or changed projected investigation sources")
	}
}

func TestAtlasStudyReplayPreservesBoundarySpanAcrossD246SameLineSource(t *testing.T) {
	data := atlasStudyReportFixture(t)
	grounding := architectureGroundingWithEntryHandoff()
	handoff := &grounding.EntryHandoffs[0]
	handoff.ProcessEntrypoint = ArchitectureAnchorMember{
		ID: "example.com/fixture/cmd/app.main", Package: "example.com/fixture/cmd/app", Name: "main",
		Location: evidence.Location{Path: "cmd/app/main.go", Line: 7, Column: 6},
	}
	handoff.Callee = ArchitectureAnchorMember{
		ID: "example.com/fixture/internal/app.Run", Package: "example.com/fixture/internal/app", Name: "Run",
		Location: evidence.Location{Path: "internal/app/run.go", Line: 11, Column: 6},
	}
	handoff.RepresentativeCallsite = evidence.Location{Path: "cmd/app/main.go", Line: 8, Column: 2}
	handoff.TargetPackage = handoff.Callee.Package
	handoff.ID = architectureEntryHandoffID(*handoff)
	grounding.Coverage.EntryHandoffs.CandidateSetSHA256 =
		architectureEntryHandoffCandidateSetSHA256(grounding.EntryHandoffs)
	data.ArchitectureGrounding = &grounding

	preInput, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("build pre-investigation input: %v", err)
	}
	var targetID, boundarySupportID, boundarySpanID string
	for _, target := range preInput.ReadingTargets {
		if target.Location.Path == handoff.Callee.Location.Path &&
			target.Location.Line == handoff.Callee.Location.Line && target.Symbol == handoff.Callee.ID {
			targetID = target.ID
			break
		}
	}
	for _, support := range preInput.ReadingSupports {
		if support.TargetID == targetID && support.Role == atlasstudy.SupportObservedCallBoundary {
			boundarySupportID = support.ID
			break
		}
	}
	for _, span := range preInput.RouteSpans {
		if span.Kind == atlasstudy.RouteSpanFocused &&
			reflect.DeepEqual(span.RequiredSupportIDs, []string{boundarySupportID}) {
			boundarySpanID = span.ID
			break
		}
	}
	if targetID == "" || boundarySupportID == "" || boundarySpanID == "" {
		t.Fatalf("pre-investigation boundary identity missing: target=%q support=%q span=%q",
			targetID, boundarySupportID, boundarySpanID)
	}

	runDir := t.TempDir()
	writeThemeStudyAcceptedArtifacts(t, runDir, data)
	// Final run hydration restores D246 declaration source actions without
	// their private canonical symbols. The empty-symbol excerpt is the exact
	// same physical declaration as the already saved canonical Study target.
	data.UserSources = append(data.UserSources, atlasStudySourceFixture(
		t, handoff.Callee.Location.Path, handoff.Callee.Location.Line, "", "func Run() {}",
	))
	postInput, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("build final hydrated input: %v", err)
	}
	physicalTargets := 0
	for _, target := range postInput.ReadingTargets {
		if target.Location.Path == handoff.Callee.Location.Path &&
			target.Location.Line == handoff.Callee.Location.Line {
			physicalTargets++
			if target.ID != targetID || target.Symbol != handoff.Callee.ID {
				t.Fatalf("D246 source action displaced canonical target: %#v", target)
			}
		}
	}
	if physicalTargets != 1 {
		t.Fatalf("same declaration produced %d semantic targets", physicalTargets)
	}
	boundarySupportAvailable := false
	for _, support := range postInput.ReadingSupports {
		boundarySupportAvailable = boundarySupportAvailable || support.ID == boundarySupportID
	}
	boundarySpanAvailable := false
	for _, span := range postInput.RouteSpans {
		boundarySpanAvailable = boundarySpanAvailable || span.ID == boundarySpanID
	}
	if !boundarySupportAvailable || !boundarySpanAvailable {
		t.Fatalf("D246 source action removed Scout boundary identity: support=%t span=%t",
			boundarySupportAvailable, boundarySpanAvailable)
	}
	status, studyMap, err := readAtlasStudyReportProduct(runDir, data)
	if err != nil {
		t.Fatalf("replay pre-investigation Scout artifacts: %v", err)
	}
	if status == nil || status.State != atlasstudy.ProductStateAccepted || studyMap != nil {
		t.Fatalf("replayed Study product = %#v / %#v", status, studyMap)
	}
}

func TestThemeStudyHydrationRejectsRevisionMismatch(t *testing.T) {
	data := atlasStudyReportFixture(t)
	runDir := t.TempDir()
	writeThemeStudyAcceptedArtifacts(t, runDir, data)
	data.CapturedRevision = strings.Repeat("f", 40)
	if _, _, err := readAtlasStudyReportProduct(runDir, data); err == nil ||
		!strings.Contains(err.Error(), "does not match captured repository revision") {
		t.Fatalf("cross-revision hydration error = %v", err)
	}
}

func TestProjectStudyInvestigationsRejectsInvalidInputAtomically(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*AtlasStudyThemesProjection, *StudyInvestigationInput, *[]string)
	}{
		{
			name: "unauthorized callsite",
			mutate: func(_ *AtlasStudyThemesProjection, _ *StudyInvestigationInput, openable *[]string) {
				*openable = slices.DeleteFunc(*openable, func(path string) bool { return path == "internal/worker/run.go" })
			},
		},
		{
			name: "one hop is not a mechanism",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				mechanism := &input.Cards[1].Mechanisms[0]
				mechanism.Nodes = mechanism.Nodes[:2]
				mechanism.Edges = mechanism.Edges[:1]
			},
		},
		{
			name: "nonconsecutive edge",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				input.Cards[1].Mechanisms[0].Edges[0].ToNodeOrdinal = 3
			},
		},
		{
			name: "invalid invocation",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				input.Cards[1].Mechanisms[0].Edges[0].Invocation = "possible"
			},
		},
		{
			name: "unknown theme",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				input.Cards[1].ThemeOrdinal = 99
			},
		},
		{
			name: "reading out of range",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				input.Cards[1].ReadingOrdinals = []int{1, 2, 4}
				input.Cards[1].Mechanisms[1].ReadingOrdinals = []int{2, 4}
			},
		},
		{
			name: "card reading union drift",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				input.Cards[1].ReadingOrdinals = []int{1, 3}
			},
		},
		{
			name: "prepared contains a path",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				input.Cards[0].Mechanisms = append(
					[]StudyInvestigationMechanismInput(nil),
					input.Cards[1].Mechanisms[0],
				)
			},
		},
		{
			name: "duplicate exact path",
			mutate: func(_ *AtlasStudyThemesProjection, input *StudyInvestigationInput, _ *[]string) {
				input.Cards[1].Mechanisms = append(
					input.Cards[1].Mechanisms,
					input.Cards[1].Mechanisms[0],
				)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			themes, canvas, graph, input, openable := studyInvestigationProjectionFixture()
			test.mutate(themes, &input, &openable)
			if err := ProjectStudyInvestigations(themes, canvas, graph, openable, input); err == nil {
				t.Fatal("invalid input was accepted")
			}
			for _, card := range themes.Cards {
				if card.Investigation != nil {
					t.Fatalf("failed projection partially mutated theme %d: %#v", card.Ordinal, card.Investigation)
				}
			}
		})
	}
}

func studyInvestigationProjectionFixture() (
	*AtlasStudyThemesProjection,
	*ArchitectureCanvas,
	*RepositoryGraph,
	StudyInvestigationInput,
	[]string,
) {
	themes := &AtlasStudyThemesProjection{
		Total: 2,
		Shown: 2,
		Cards: []StudyThemeCard{
			{
				Ordinal:  1,
				Readings: []StudyThemeReading{{Label: "Preparation root", Path: "prepare.go", Line: 3}},
			},
			{
				Ordinal: 2,
				Readings: []StudyThemeReading{
					{Label: "Main", Path: "cmd/app/main.go", Line: 10},
					{Label: "Second", Path: "z/start.go", Line: 4},
					{Label: "Worker", Path: "internal/worker/run.go", Line: 22},
				},
			},
		},
	}
	mainLocation := evidence.Location{Path: "cmd/app/main.go", Line: 10, Column: 6}
	storeLocation := evidence.Location{Path: "internal/store/save.go", Line: 30, Column: 1}
	workerPackage := componentmap.Candidate{
		ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "member-worker"},
		Facts: []componentmap.LocalFact{{
			Kind:  componentmap.FactDeclaration,
			Value: "example.com/repo/internal/worker",
		}},
	}
	canvas := &ArchitectureCanvas{
		Version: ArchitectureCanvasVersion,
		Components: []ArchitectureComponent{
			{
				ID: "component-entry",
				Members: []componentmap.Candidate{studyInvestigationSymbolMember(
					"member-main",
					studyInvestigationMainSymbol,
					mainLocation,
				)},
			},
			{ID: "component-worker-b", SharedMembers: []componentmap.Candidate{workerPackage}},
			{ID: "component-worker-a", Members: []componentmap.Candidate{workerPackage}},
			{
				ID:   "component-lookalike",
				Name: "store Save internal/store/save.go",
				Members: []componentmap.Candidate{{
					ID: componentmap.MemberID{Kind: componentmap.MemberPackage, Value: "member-wrong"},
					Facts: []componentmap.LocalFact{{
						Kind:  componentmap.FactDeclaration,
						Value: "example.com/repo/internal/not-store",
					}},
				}},
			},
			{
				ID: "local-remainder",
				Members: []componentmap.Candidate{studyInvestigationSymbolMember(
					"member-store",
					studyInvestigationStoreSymbol,
					storeLocation,
				)},
			},
		},
		LocalRemainderComponentID: "local-remainder",
		StructuralEdges: []ArchitectureStructuralEdge{{
			FromComponentID: "component-entry",
			ToComponentID:   "component-lookalike",
		}},
	}
	graph := &RepositoryGraph{
		Packages: []PackageInfo{
			{
				CanonicalPath: "example.com/repo/internal/worker",
				Files:         []string{"internal/worker/extra.go", "internal/worker/run.go"},
			},
			{
				CanonicalPath: "example.com/repo/internal/not-store",
				Files:         []string{"internal/not-store/save.go"},
			},
		},
	}
	first := StudyInvestigationMechanismInput{
		ReadingOrdinals: []int{3, 1},
		Nodes: []StudyInvestigationNodeInput{
			{Label: "app.main", Symbol: studyInvestigationMainSymbol, Location: mainLocation},
			{
				Label:    "worker.Run",
				Symbol:   studyInvestigationWorkerSymbol,
				Location: evidence.Location{Path: "internal/worker/run.go", Line: 22, Column: 1},
			},
			{Label: "store.Save", Symbol: studyInvestigationStoreSymbol, Location: storeLocation},
		},
		Edges: []StudyInvestigationEdgeInput{
			{
				FromNodeOrdinal: 1,
				ToNodeOrdinal:   2,
				Invocation:      surfacediscovery.DirectCallSynchronous,
				WitnessCount:    2,
				Callsite:        evidence.Location{Path: "cmd/app/main.go", Line: 12, Column: 3},
			},
			{
				FromNodeOrdinal: 2,
				ToNodeOrdinal:   3,
				Invocation:      surfacediscovery.DirectCallGoroutine,
				WitnessCount:    1,
				Callsite:        evidence.Location{Path: "internal/worker/run.go", Line: 25, Column: 4},
			},
		},
	}
	second := StudyInvestigationMechanismInput{
		ReadingOrdinals: []int{2},
		Nodes: []StudyInvestigationNodeInput{
			{
				Label:    "z.start",
				Symbol:   "example.com/repo/z.start",
				Location: evidence.Location{Path: "z/start.go", Line: 4, Column: 1},
			},
			{
				Label:    "z.middle",
				Symbol:   "example.com/repo/z.middle",
				Location: evidence.Location{Path: "z/middle.go", Line: 8, Column: 1},
			},
			{
				Label:    "z.finish",
				Symbol:   "example.com/repo/z.finish",
				Location: evidence.Location{Path: "z/finish.go", Line: 12, Column: 1},
			},
		},
		Edges: []StudyInvestigationEdgeInput{
			{
				FromNodeOrdinal: 1,
				ToNodeOrdinal:   2,
				Invocation:      surfacediscovery.DirectCallDeferred,
				WitnessCount:    1,
				Callsite:        evidence.Location{Path: "z/start.go", Line: 6, Column: 2},
			},
			{
				FromNodeOrdinal: 2,
				ToNodeOrdinal:   3,
				Invocation:      surfacediscovery.DirectCallSynchronous,
				WitnessCount:    3,
				Callsite:        evidence.Location{Path: "z/middle.go", Line: 10, Column: 2},
			},
		},
	}
	input := StudyInvestigationInput{Cards: []StudyInvestigationCardInput{
		{ThemeOrdinal: 1, Outcome: StudyInvestigationOutcomePrepared, ReadingOrdinals: []int{1}},
		{
			ThemeOrdinal:    2,
			Outcome:         StudyInvestigationOutcomeMechanism,
			ReadingOrdinals: []int{3, 1, 2},
			Mechanisms:      []StudyInvestigationMechanismInput{second, first},
		},
	}}
	openable := []string{
		"cmd/app/main.go",
		"internal/store/save.go",
		"internal/worker/run.go",
		"prepare.go",
		"z/finish.go",
		"z/middle.go",
		"z/start.go",
	}
	return themes, canvas, graph, input, openable
}

func studyInvestigationSymbolMember(
	memberID string,
	symbol string,
	location evidence.Location,
) componentmap.Candidate {
	return componentmap.Candidate{
		ID: componentmap.MemberID{Kind: componentmap.MemberSymbol, Value: memberID},
		Facts: []componentmap.LocalFact{{
			Kind:     componentmap.FactDeclaration,
			Value:    symbol,
			Location: &location,
		}},
	}
}

func studyInvestigationSourceAuthority(
	t *testing.T,
	files map[string]map[int]string,
) RunAuthority {
	t.Helper()
	repository := t.TempDir()
	paths := make([]string, 0, len(files))
	for sourcePath, replacements := range files {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repository, sourcePath)), 0o755); err != nil {
			t.Fatal(err)
		}
		lines := make([]string, 40)
		for index := range lines {
			lines[index] = fmt.Sprintf("// line %d", index+1)
		}
		for line, text := range replacements {
			lines[line-1] = text
		}
		writeTestFile(t, repository, sourcePath, strings.Join(lines, "\n")+"\n")
		paths = append(paths, sourcePath)
	}
	sort.Strings(paths)
	return captureOverviewSourceAuthority(t, repository, paths)
}

func studyInvestigationTestLocationLess(left, right evidence.Location) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	return left.Column < right.Column
}
