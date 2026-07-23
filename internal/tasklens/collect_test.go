package tasklens

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/freshness"
)

func TestParseTaskTextExtractsPromptSafeSection(t *testing.T) {
	raw := []byte("# Episode\n\n## Frozen source state\nsecret metadata\n\n## Prompt-safe task\n\nFind `Enabled` and verify it.\n\n## Benchmark rules\nignore\n")
	got, err := ParseTaskText(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "Find `Enabled` and verify it." {
		t.Fatalf("task = %q", got)
	}
}

func TestCompoundTaskTermsKeepSearchableIdentifierFragments(t *testing.T) {
	terms := extractTerms("Inspect `netHttpContext.SerializeError`, `Response[[]ModelA]`, and generic/slice handling.")
	weights := make(map[string]int, len(terms))
	for _, term := range terms {
		weights[term.Normalized] = term.Weight
	}
	for _, want := range []string{"serializeerror", "response", "modela", "generic", "slice"} {
		if weights[want] < 8 {
			t.Fatalf("fragment %q weight = %d, terms = %#v", want, weights[want], terms)
		}
	}
	for _, term := range terms {
		if strings.ContainsAny(term.Text, "[](){}<>*") && usableGrepTerm(term) {
			t.Fatalf("syntactic compound became a grep query: %#v", term)
		}
	}
}

func TestTaskTermsPreferRepeatedMechanismVocabularyOverSentenceCapitalization(t *testing.T) {
	tests := []struct {
		name string
		task string
		want []string
	}{
		{
			name: "group state",
			task: "Create a parent route group with middleware A, then a child group with middleware B. " +
				"The child group middleware changes with route order. Find why group middleware state is aliased.",
			want: []string{"group", "middleware", "route", "child"},
		},
		{
			name: "schema naming",
			task: "OpenAPI component names collide for a generic slice schema. " +
				"Trace the component naming path and preserve the schema type name.",
			want: []string{"openapi", "component", "schema", "name"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			terms := extractTerms(test.task)
			weights := make(map[string]int, len(terms))
			for _, term := range terms {
				weights[term.Normalized] = term.Weight
			}
			for _, want := range test.want {
				if weights[want] == 0 {
					t.Fatalf("mechanism term %q was displaced: %#v", want, terms)
				}
			}
			if weights["the"] != 0 || weights["create"] != 0 {
				t.Fatalf("sentence scaffolding was promoted: %#v", terms)
			}
		})
	}
}

func TestRankGrepMatchesPrefersDeclarationsAndConcentratedFiles(t *testing.T) {
	matches := make([]grepMatch, 0, 24)
	for index := 0; index < 14; index++ {
		matches = append(matches, grepMatch{
			path: "noise" + strconv.Itoa(index) + ".go",
			line: 20,
			text: "// Register a general repository object.",
		})
	}
	matches = append(matches,
		grepMatch{path: "router.go", line: 25, text: "func Register(router *Router) {}"},
		grepMatch{path: "router.go", line: 40, text: "Register(router)"},
		grepMatch{path: "router.go", line: 55, text: "func registerChild(router *Router) {}"},
		grepMatch{path: "router_test.go", line: 10, text: "func TestRegisterChild(t *testing.T) {}"},
		grepMatch{path: "router_test.go", line: 20, text: "Register(router)"},
	)

	rankGrepMatches(matches, "register")
	paths := make(map[string]struct{})
	for _, match := range matches {
		if len(paths) == 12 {
			break
		}
		paths[match.path] = struct{}{}
	}
	for _, want := range []string{"router.go", "router_test.go"} {
		if _, ok := paths[want]; !ok {
			t.Fatalf("bounded grep paths omit %q: %#v", want, matches)
		}
	}
}

func TestRepositoryStateSHANormalizesCleanDirtySlice(t *testing.T) {
	state := freshness.RepositoryState{
		Version:  freshness.RepositoryStateVersion,
		Identity: "/repo", Head: strings.Repeat("a", 40), Dirty: []freshness.DirtyFile{},
	}
	want, err := RepositoryStateSHA(state)
	if err != nil {
		t.Fatal(err)
	}
	state.Dirty = nil
	got, err := RepositoryStateSHA(state)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("clean repository state differs for nil/empty dirty slices: %s != %s", got, want)
	}
}

func TestModuleIndexCannotStarveTaskEvidenceBudget(t *testing.T) {
	repo := newTaskLensTestRepo(t, "many-modules")
	for index := 0; index < MaxReadFiles+3; index++ {
		directory := filepath.Join(repo, "module"+strconv.Itoa(index))
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		content := "module example.com/module" + strconv.Itoa(index) + "\n\ngo 1.24\n"
		if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "--quiet", "-m", "add modules")

	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and ReadEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Metrics.ManifestFilesRead > MaxManifestFiles || bundle.Metrics.EvidenceFilesRead == 0 ||
		len(bundle.Anchors) == 0 {
		t.Fatalf("manifest indexing starved task evidence: metrics=%#v anchors=%d", bundle.Metrics, len(bundle.Anchors))
	}
	if bundle.Budgets.ReadFiles > MaxReadFiles {
		t.Fatalf("unified file budget exceeded: %#v", bundle.Budgets)
	}
}

func TestRelationsRequireDirectionalExcerptEvidence(t *testing.T) {
	duplicateDeclarations := []Anchor{
		{ID: "anchor-left", Path: "left.go", Symbol: "Validate", Excerpt: []SourceLine{{Line: 1, Text: "func Validate() error { return nil }"}}},
		{ID: "anchor-right", Path: "right.go", Symbol: "Validate", Excerpt: []SourceLine{{Line: 1, Text: "func Validate() error { return nil }"}}},
	}
	for _, relation := range collectRelations(duplicateDeclarations, nil) {
		if relation.Kind == relationKindExactIdentifier {
			t.Fatalf("duplicate declarations created a false exact relation: %#v", relation)
		}
	}

	production := Anchor{ID: "anchor-production", Path: "handler.go", Package: "fixture", Symbol: "Handle", Excerpt: []SourceLine{{Line: 1, Text: "func Handle() {}"}}}
	test := Anchor{ID: "anchor-test", Path: "handler_test.go", Package: "fixture", Symbol: "TestHandleReference", Excerpt: []SourceLine{{Line: 1, Text: "var _ = Handle"}}}
	relations := collectRelations([]Anchor{production, test}, nil)
	if len(relations) != 1 || relations[0].Kind != relationKindTestReference {
		t.Fatalf("test reference relation = %#v", relations)
	}
}

func TestTestExercisesRequiresExactGoPackageBinding(t *testing.T) {
	t.Parallel()

	alpha := Anchor{
		ID: "anchor-alpha-handler", Path: "alpha/handler.go", Package: "alpha", Symbol: "Handler",
		Excerpt:     []SourceLine{{Line: 1, Text: "func Handler() {}"}},
		EvidenceIDs: []string{"evidence-alpha-handler"},
	}
	beta := Anchor{
		ID: "anchor-beta-handler", Path: "beta/handler.go", Package: "beta", Symbol: "Handler",
		Excerpt:     []SourceLine{{Line: 1, Text: "func Handler() {}"}},
		EvidenceIDs: []string{"evidence-beta-handler"},
	}
	qualified := Anchor{
		ID: "anchor-qualified-test", Path: "consumer/handler_test.go", Package: "consumer_test",
		Symbol: "TestHandler", EvidenceIDs: []string{"evidence-qualified-test"},
		Excerpt: []SourceLine{{Line: 1, Text: "func TestHandler() { alpha.Handler() }"}},
	}
	relations := collectRelations([]Anchor{alpha, beta, qualified}, nil)
	foundAlpha := false
	for _, relation := range relations {
		if relation.Kind != relationKindTestReference {
			continue
		}
		if relation.LeftID == beta.ID || relation.RightID == beta.ID {
			t.Fatalf("selector for package alpha bound to duplicate beta.Handler: %#v", relation)
		}
		if relation.LeftID == alpha.ID || relation.RightID == alpha.ID {
			foundAlpha = true
		}
	}
	if !foundAlpha {
		t.Fatalf("qualified alpha.Handler did not produce test_exercises: %#v", relations)
	}
	if !hasRelationKind(relations, relationKindExactIdentifier) {
		t.Fatalf("mismatched beta.Handler did not remain weak exact evidence: %#v", relations)
	}

	external := Anchor{
		ID: "anchor-external-test", Path: "alpha/handler_external_test.go", Package: "alpha_test",
		Symbol: "TestHandler", EvidenceIDs: []string{"evidence-external-test"},
		Excerpt: []SourceLine{{Line: 1, Text: "var _ = Handler"}},
	}
	externalRelations := collectRelations([]Anchor{alpha, external}, nil)
	for _, relation := range externalRelations {
		if relation.Kind == relationKindTestReference {
			t.Fatalf("unqualified cross-package reference became test_exercises: %#v", relation)
		}
	}
	if !hasRelationKind(externalRelations, relationKindExactIdentifier) {
		t.Fatalf("unqualified cross-package reference did not remain weak exact evidence: %#v", externalRelations)
	}

	samePackage := external
	samePackage.ID = "anchor-same-package-test"
	samePackage.Package = "alpha"
	if relations := collectRelations([]Anchor{alpha, samePackage}, nil); !hasRelationKind(relations, relationKindTestReference) {
		t.Fatalf("unqualified same-package reference lost test_exercises: %#v", relations)
	}
}

func TestRepositoryObservationGuidanceQuotesExactBoundedSource(t *testing.T) {
	repo := newTaskLensTestRepo(t, "repository-observation")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var sourceEvidence string
	for _, anchor := range bundle.Anchors {
		if !isDocumentPath(anchor.Path) && !isTestPath(anchor.Path) && len(anchor.EvidenceIDs) > 0 {
			sourceEvidence = anchor.EvidenceIDs[0]
			break
		}
	}
	if sourceEvidence == "" {
		t.Fatal("fixture has no retained source evidence")
	}
	proposal.ReproduceOrObserve = []ProposedGuidance{{
		Text:      "Observe the exact retained configuration assignment.",
		Authority: AuthorityRepositoryObservation, EvidenceIDs: []string{sourceEvidence},
	}}
	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	got := pack.ReproduceOrObserve[0].Text
	if !strings.Contains(got, "Exact retained repository observation (not executed):") ||
		!strings.Contains(got, ".go:") {
		t.Fatalf("repository observation guidance = %q", got)
	}
}

func TestSelectFilesReservesSourceAndTestsBeforeGeneratedSnapshots(t *testing.T) {
	terms := []Term{
		{ID: "term-send", Weight: 16},
		{ID: "term-context", Weight: 10},
		{ID: "term-flow", Weight: 16},
	}
	candidates := []candidate{
		{path: "testdata/generated.golden.json", score: 300, termHits: map[string]struct{}{"term-send": {}, "term-context": {}, "term-flow": {}}, isTest: true},
		{path: "serialization.go", score: 200, termHits: map[string]struct{}{"term-send": {}}},
		{path: "ctx.go", score: 180, termHits: map[string]struct{}{"term-context": {}}},
		{path: "adapter.go", score: 170, termHits: map[string]struct{}{"term-send": {}}},
		{path: "serve.go", score: 90, termHits: map[string]struct{}{"term-flow": {}}},
		{path: "serialization_test.go", score: 160, termHits: map[string]struct{}{"term-send": {}}, isTest: true},
		{path: "serve_test.go", score: 80, termHits: map[string]struct{}{"term-flow": {}}, isTest: true},
		{path: "ctx_test.go", score: 70, termHits: map[string]struct{}{"term-context": {}}, isTest: true},
	}
	for index := 0; index < 8; index++ {
		candidates = append(candidates, candidate{
			path: "duplicate" + strconv.Itoa(index) + ".go", score: 60 - index,
			termHits: map[string]struct{}{"term-send": {}},
		})
	}
	selected := selectFiles(candidates, terms, TaskBug)
	positions := make(map[string]int, len(selected))
	for index, item := range selected {
		positions[item.path] = index
	}
	for _, want := range []string{"serialization.go", "ctx.go", "serve.go", "serialization_test.go", "serve_test.go", "ctx_test.go"} {
		if _, ok := positions[want]; !ok {
			t.Fatalf("selected files omit %q: %#v", want, selected)
		}
	}
	if generated, ok := positions["testdata/generated.golden.json"]; ok && generated < 6 {
		t.Fatalf("generated snapshot was read before source/test reservations: %#v", selected)
	}
}

func TestSelectFilesForDataTransformationPrefersTaskNamedPathAndTest(t *testing.T) {
	terms := []Term{
		{ID: "term-openapi", Text: "OpenAPI", Normalized: "openapi", Weight: 10},
		{ID: "term-parameter", Text: "parameter", Normalized: "parameter", Weight: 8},
		{ID: "term-registration", Text: "registration", Normalized: "registration", Weight: 8},
		{ID: "term-example", Text: "example", Normalized: "example", Weight: 8},
	}
	candidates := []candidate{
		{path: "schema_customizer.go", score: 220, termHits: map[string]struct{}{}},
		{path: "openapi_description.go", score: 210, termHits: map[string]struct{}{"term-openapi": {}}},
		{path: "openapi.go", score: 150, termHits: map[string]struct{}{"term-openapi": {}, "term-parameter": {}}},
		{path: "option.go", score: 140, termHits: map[string]struct{}{"term-example": {}}},
		{path: "openapi_test.go", score: 230, termHits: map[string]struct{}{"term-openapi": {}}, isTest: true},
		{path: "parameter_registration_test.go", score: 160, termHits: map[string]struct{}{"term-parameter": {}, "term-registration": {}}, isTest: true},
		{path: "testdata/openapi.golden.json", score: 200, termHits: map[string]struct{}{"term-openapi": {}}, isTest: true},
	}

	selected := selectFilesForProfile(
		candidates,
		terms,
		TaskBug,
		TaskProfileDataTagTransformation,
	)
	if len(selected) < 4 {
		t.Fatalf("selected files = %#v", selected)
	}
	want := []string{
		"openapi.go",
		"option.go",
		"parameter_registration_test.go",
		"testdata/openapi.golden.json",
	}
	for index, path := range want {
		if selected[index].path != path {
			t.Fatalf("selected[%d] = %q, want %q; all = %#v", index, selected[index].path, path, selected)
		}
	}
}

func TestSelectFileAnchorsKeepsTaskConceptsAndReferencedHelpers(t *testing.T) {
	terms := []Term{
		{ID: "term-openapi", Text: "OpenAPI", Normalized: "openapi", Weight: 10},
		{ID: "term-generic", Text: "generic", Normalized: "generic", Weight: 8},
		{ID: "term-slice", Text: "slice", Normalized: "slice", Weight: 8},
	}
	candidates := []anchorCandidate{
		{anchor: Anchor{Path: "schema.go", Symbol: "Register", Excerpt: []SourceLine{{Line: 1, Text: "func Register() { NormalizeName() }"}}}, score: 100, terms: []string{"term-openapi"}},
		{anchor: Anchor{Path: "schema.go", Symbol: "OpenAPI", Excerpt: []SourceLine{{Line: 2, Text: "type OpenAPI struct{}"}}}, score: 95, terms: []string{"term-openapi"}},
	}
	for index := 0; index < 7; index++ {
		candidates = append(candidates, anchorCandidate{
			anchor: Anchor{Path: "schema.go", Symbol: "Common" + strconv.Itoa(index), Excerpt: []SourceLine{{Line: index + 3, Text: "func Common() { useOpenAPI() }"}}},
			score:  90 - index, terms: []string{"term-openapi"},
		})
	}
	candidates = append(candidates,
		anchorCandidate{anchor: Anchor{Path: "schema.go", Symbol: "ParseGeneric", Excerpt: []SourceLine{{Line: 20, Text: "func ParseGeneric() { /* generic slice */ }"}}}, score: 20, terms: []string{"term-generic", "term-slice"}},
		anchorCandidate{anchor: Anchor{Path: "schema.go", Symbol: "NormalizeName", Excerpt: []SourceLine{{Line: 21, Text: "func NormalizeName() {}"}}}, score: 10},
	)
	selected := selectFileAnchors(candidates, terms)
	symbols := make(map[string]struct{}, len(selected))
	for _, item := range selected {
		symbols[item.anchor.Symbol] = struct{}{}
	}
	for _, want := range []string{"Register", "ParseGeneric", "NormalizeName"} {
		if _, ok := symbols[want]; !ok {
			t.Fatalf("selected declarations omit %q: %#v", want, selected)
		}
	}
}

func TestGoAnchorDiversityCountsTaskTermsMatchedBySymbol(t *testing.T) {
	terms := []Term{
		{ID: "term-openapi", Text: "OpenAPI", Normalized: "openapi", Weight: 10},
		{ID: "term-schema", Text: "schema", Normalized: "schema", Weight: 6},
		{ID: "term-slice", Text: "slice", Normalized: "slice", Weight: 8},
		{ID: "term-type", Text: "type", Normalized: "type", Weight: 3},
		{ID: "term-name", Text: "name", Normalized: "name", Weight: 6},
	}
	content := `package fixture

type OpenAPI struct{}

func OpenAPIAlpha() {}
func OpenAPIBeta() {}
func OpenAPIGamma() {}
func OpenAPIDelta() {}
func OpenAPIEpsilon() {}
func OpenAPIZeta() {}
func OpenAPIEta() {}
func OpenAPITheta() {}

func SchemaTagFromType(openapi *OpenAPI, value any) string {
	return dive(openapi, value)
}

func dive(openapi *OpenAPI, value any) string {
	// A slice component is reduced to its element before naming.
	return transformTypeName(openapi.getOrCreateSchema(value))
}

func transformTypeName(value string) string { return value }

func (openapi *OpenAPI) getOrCreateSchema(value any) string { return "" }
`
	file := collectedFile{
		candidate: candidate{path: "schema.go", score: 90},
		content:   []byte(content),
		lines:     strings.Split(content, "\n"),
	}
	candidates := goAnchors(file, terms)
	selected := selectFileAnchors(candidates, terms)
	symbols := make(map[string]struct{}, len(selected))
	for _, item := range selected {
		symbols[item.anchor.Symbol] = struct{}{}
	}
	for _, want := range []string{"SchemaTagFromType", "dive", "transformTypeName", "OpenAPI.getOrCreateSchema"} {
		if _, ok := symbols[want]; !ok {
			t.Fatalf("symbol-matched task mechanism omits %q: %#v", want, selected)
		}
	}
}

func TestGoAnchorsAdmitsZeroTaskScoreExactCallTarget(t *testing.T) {
	t.Parallel()

	const source = `package fixture

func RegisterTag(value string) string {
	return decodeAtom(value)
}

func decodeAtom(value string) string { return value }

func ignoredHelper(value string) string {
	_ = "decodeAtom(value)"
	return value
}
`
	terms := []Term{{
		ID: "term-register-tag", Text: "RegisterTag", Normalized: "registertag", Weight: 12,
	}}
	candidates := goAnchors(completeCollectedFile("tag.go", source), terms)
	if len(candidates) != 2 {
		t.Fatalf("Go candidate ledger = %#v, want RegisterTag and its exact call target", candidates)
	}
	register := anchorCandidateBySymbol(candidates, "RegisterTag")
	decode := anchorCandidateBySymbol(candidates, "decodeAtom")
	if register.anchor.ID == "" || decode.anchor.ID == "" {
		t.Fatalf("exact call target was not admitted: %#v", candidates)
	}
	if decode.score != 0 || len(decode.terms) != 0 {
		t.Fatalf("zero-task-score target gained task evidence: %#v", decode)
	}
	if ignored := anchorCandidateBySymbol(candidates, "ignoredHelper"); ignored.anchor.ID != "" {
		t.Fatalf("unreferenced zero-score declaration was admitted: %#v", ignored)
	}
	relations := collectRelations([]Anchor{register.anchor, decode.anchor}, terms)
	if !hasRelationKind(relations, relationKindDirectCall) {
		t.Fatalf("admitted target did not form an exact direct call: %#v", relations)
	}
	for index := range candidates {
		switch candidates[index].anchor.Symbol {
		case "RegisterTag":
			candidates[index].anchor.RoleHints = []AnchorRole{RolePublicOrCLIEntry}
		case "decodeAtom":
			candidates[index].anchor.RoleHints = []AnchorRole{RoleTransformation}
		}
	}
	contract := RoleContract{
		Profile: TaskProfileDataTagTransformation,
		Key: []RoleRequirement{
			{Role: RolePublicOrCLIEntry, MinimumAnchors: 1},
			{Role: RoleTransformation, MinimumAnchors: 1},
		},
	}
	completed, expansions := completeMissingRoleAnchors(
		[]Anchor{candidates[0].anchor},
		candidates,
		terms,
		contract,
	)
	if expansions != 1 || !containsAnchorID(completed, decode.anchor.ID) {
		t.Fatalf("bounded completion did not select zero-score target: expansions=%d anchors=%#v", expansions, completed)
	}
}

func TestSplitSourceLinesDoesNotInventTerminalEOFLine(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty file", raw: "", want: nil},
		{name: "terminal newline", raw: "one\n", want: []string{"one"}},
		{name: "physical blank line", raw: "one\n\n", want: []string{"one", ""}},
		{name: "CRLF terminal newline", raw: "one\r\ntwo\r\n", want: []string{"one", "two"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := splitSourceLines([]byte(test.raw)); !slices.Equal(got, test.want) {
				t.Fatalf("splitSourceLines(%q) = %#v, want %#v", test.raw, got, test.want)
			}
		})
	}
}

func TestCollectIsBoundedAndTaskTextStaysSeparateFromRepositoryTruth(t *testing.T) {
	repo := newTaskLensTestRepo(t, "task-labelled-checkout")
	runGitTest(t, repo, "remote", "add", "origin", "https://github.com/remote/should-not-win.git")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is silently ignored. Locate how Enabled is copied and identify a verification test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Repository.Identity != "example.com/tasklensfixture" || bundle.Repository.DisplayName != "task-labelled-checkout" {
		t.Fatalf("repository = %#v", bundle.Repository)
	}
	if bundle.Locality != LocalityLocalExact || bundle.KindHint != TaskConfiguration {
		t.Fatalf("classification = %q / %q", bundle.KindHint, bundle.Locality)
	}
	if bundle.Budgets.InitialCandidates > MaxInitialCandidates || bundle.Budgets.RetainedAnchors > MaxRetainedAnchors ||
		bundle.Budgets.ReadFiles > MaxReadFiles || bundle.Budgets.ReadBytes > MaxReadBytes || bundle.Budgets.GoplsQueries > MaxGoplsQueries {
		t.Fatalf("budgets exceeded: %#v", bundle.Budgets)
	}
	if len(bundle.StagesSkipped) == 0 || !contains(bundle.StagesSkipped, "generic_orientation") {
		t.Fatalf("stages skipped = %#v", bundle.StagesSkipped)
	}
	if bundle.Task.EvidenceID == "" || bundle.Evidence[0].Kind != EvidenceTaskProvided {
		t.Fatalf("task evidence = %#v", bundle.Evidence)
	}
	unknown := Term{Text: "ImaginarySetting", Normalized: "imaginarysetting"}
	found := false
	for _, term := range bundle.Terms {
		if term.Normalized == unknown.Normalized {
			found = term.Found
		}
	}
	if found {
		t.Fatal("user-only term was promoted to repository evidence")
	}
}

func TestClassifyLocalityUsesGroundedContractEvidence(t *testing.T) {
	anchors := func(role AnchorRole, exactTerm string, count int) []Anchor {
		result := make([]Anchor, 0, count)
		result = append(result, Anchor{
			Path: "contract.go", Symbol: exactTerm,
			Excerpt:   []SourceLine{{Line: 1, Text: "type " + exactTerm + " struct{}"}},
			RoleHints: []AnchorRole{role},
		})
		for index := 1; index < count; index++ {
			result = append(result, Anchor{
				Path: "other" + strconv.Itoa(index) + ".go", Symbol: "Other" + strconv.Itoa(index),
				Excerpt:   []SourceLine{{Line: 1, Text: "func Other" + strconv.Itoa(index) + "() {}"}},
				RoleHints: []AnchorRole{RoleRepresentativeImplementation},
			})
		}
		return result
	}
	strongTerm := func(text string) []Term {
		return []Term{{Text: text, Normalized: strings.ToLower(text), Found: true, Weight: 16}}
	}

	tests := []struct {
		name      string
		task      string
		terms     []Term
		anchors   []Anchor
		relations []Relation
		want      Locality
	}{
		{
			name:    "configuration contract ignores retained breadth",
			task:    "The configuration option DisableMessages is ignored.",
			terms:   strongTerm("DisableMessages"),
			anchors: anchors(RoleConfigurationSource, "DisableMessages", MaxRetainedAnchors),
			want:    LocalityLocalExact,
		},
		{
			name:    "documentation compatibility contract ignores retained breadth",
			task:    "The documentation dependency no longer renders OpenAPI.",
			terms:   strongTerm("OpenAPI"),
			anchors: anchors(RoleDocumentationContract, "OpenAPI", MaxRetainedAnchors),
			want:    LocalityLocalExact,
		},
		{
			name:  "unrelated strong term does not make configuration local",
			task:  "The configuration option DisableMessages is ignored.",
			terms: strongTerm("DisableMessages"),
			anchors: append(
				anchors(RoleConfigurationSource, "Settings", MaxRetainedAnchors-1),
				Anchor{
					Path: "unrelated.go", Symbol: "DisableMessages",
					Excerpt:   []SourceLine{{Line: 1, Text: "func DisableMessages() {}"}},
					RoleHints: []AnchorRole{RoleRepresentativeImplementation},
				},
			),
			want: LocalityBoundedCrossFile,
		},
		{
			name:      "weak role evidence remains bounded",
			task:      "The configuration option setting is ignored.",
			terms:     []Term{{Text: "setting", Normalized: "setting", Found: true, Weight: 3}},
			anchors:   anchors(RoleConfigurationSource, "setting", MaxRetainedAnchors),
			relations: []Relation{{Kind: relationKindExactIdentifier}},
			want:      LocalityBoundedCrossFile,
		},
		{
			name:    "sparse ungrounded task stays broad",
			task:    "Inspect an unknown behavior.",
			terms:   []Term{{Text: "unknown", Normalized: "unknown", Weight: 3}},
			anchors: anchors(RoleRepresentativeImplementation, "Other", 2),
			want:    LocalityBroadDynamic,
		},
		{
			name:    "extension classification still wins",
			task:    "Add support for a new integration.",
			terms:   strongTerm("Integration"),
			anchors: anchors(RoleConfigurationSource, "Integration", MaxRetainedAnchors),
			want:    LocalityExtension,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyLocality(test.task, test.terms, test.anchors, test.relations); got != test.want {
				t.Fatalf("classifyLocality() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestClassifyTaskKindUsesWordAndPhraseBoundaries(t *testing.T) {
	tests := []struct {
		task string
		want TaskKind
	}{
		{task: "Implement vanilla parsing.", want: TaskFeature},
		{task: "Implement manuscript parsing.", want: TaskFeature},
		{task: "Support optional fields in the decoder.", want: TaskUnknown},
		{task: "The option is silently ignored.", want: TaskConfiguration},
		{task: "A nil request causes a panic.", want: TaskBug},
	}
	for _, test := range tests {
		if got := classifyTaskKind(test.task); got != test.want {
			t.Errorf("classifyTaskKind(%q) = %q, want %q", test.task, got, test.want)
		}
	}
}

func TestCollectDoesNotDereferenceTrackedSymlinkEvidence(t *testing.T) {
	repo := newTaskLensTestRepo(t, "tracked-symlink")
	secretPath := filepath.Join(repo, ".ignored-secret.go")
	if err := os.WriteFile(secretPath, []byte("package fixture\n\nfunc LeakedSecret() string { return \"do-not-read\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".ignored-secret.go", filepath.Join(repo, "Leaked.go")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	runGitTest(t, repo, "add", "Leaked.go")
	runGitTest(t, repo, "commit", "--quiet", "-m", "track evidence-looking symlink")

	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect Leaked and CopyConfig.",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "LeakedSecret") || strings.Contains(string(raw), "do-not-read") ||
		contains(bundle.AllowedPaths, "Leaked.go") {
		t.Fatalf("tracked symlink target became model-visible: %s", raw)
	}
}

func TestCollectDoesNotDereferenceTrackedPathThroughSymlinkDirectory(t *testing.T) {
	repo := newTaskLensTestRepo(t, "tracked-symlink-directory")
	trackedDir := filepath.Join(repo, "tracked")
	if err := os.MkdirAll(trackedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(trackedDir, "Leaked.go"),
		[]byte("package fixture\n\nfunc HarmlessTracked() {}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "tracked/Leaked.go")
	runGitTest(t, repo, "commit", "--quiet", "-m", "track nested source")

	secretDir := filepath.Join(repo, ".ignored-secret-dir")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(secretDir, "Leaked.go"),
		[]byte("package fixture\n\nfunc LeakedThroughDirectory() string { return \"do-not-read-directory\" }\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(trackedDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".ignored-secret-dir", trackedDir); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and Leaked.",
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(bundle.PromptBundle())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "LeakedThroughDirectory") ||
		strings.Contains(string(raw), "do-not-read-directory") ||
		contains(bundle.AllowedPaths, "tracked/Leaked.go") {
		t.Fatalf("tracked path through symlink directory became model-visible: %s", raw)
	}
}

func TestIsolatedGitCommandDisablesRepositoryFSMonitor(t *testing.T) {
	repo := newTaskLensTestRepo(t, "fsmonitor")
	marker := filepath.Join(t.TempDir(), "fsmonitor-ran")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	if err := os.WriteFile(
		hook,
		[]byte("#!/bin/sh\nprintf ran > \""+marker+"\"\n"),
		0o700,
	); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "config", "--local", "core.fsmonitor", hook)
	command := isolatedGitCommand(context.Background(), repo, "status", "--porcelain=v1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("safe git status: %v: %s", err, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("repository fsmonitor executed; marker stat error = %v", err)
	}
}

func TestSparseBoundedEvidenceProducesAReplayablePartialPack(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "sparse")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"go.mod":  "module example.com/sparse\n\ngo 1.24\n",
		"solo.go": "package sparse\n\nfunc Solo() int { return 1 }\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitTest(t, repo, "init", "--quiet")
	runGitTest(t, repo, "config", "user.email", "tasklens@example.com")
	runGitTest(t, repo, "config", "user.name", "Task Lens Test")
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "--quiet", "-m", "fixture")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo, TaskText: "Inspect Solo and identify what remains unresolved.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Anchors) == 0 || len(bundle.Anchors) >= PreferredMinVisibleAnchors ||
		bundle.Locality != LocalityBroadDynamic {
		t.Fatalf("sparse bundle = anchors:%d locality:%q", len(bundle.Anchors), bundle.Locality)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Anchors) != len(bundle.Anchors) || len(pack.NextProbes) == 0 {
		t.Fatalf("sparse pack = %#v", pack)
	}
	if err := ValidatePackAgainstBundle(bundle, pack); err != nil {
		t.Fatalf("sparse pack replay: %v", err)
	}
}

func TestUnrelatedTaskProducesReplayableZeroEvidencePartial(t *testing.T) {
	repo := newTaskLensTestRepo(t, "zero-evidence")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "Investigate ZqxvAbsentMechanism and QplmMissingBoundary.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Anchors) != 0 || !slices.Equal(bundle.AllowedPaths, []string{"go.mod"}) ||
		len(bundle.Evidence) != 1 || bundle.Locality != LocalityBroadDynamic {
		t.Fatalf("zero-evidence bundle = anchors:%d paths:%#v evidence:%#v locality:%q", len(bundle.Anchors), bundle.AllowedPaths, bundle.Evidence, bundle.Locality)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.Anchors) != 0 || len(pack.LikelyAreas) != 0 ||
		len(pack.NextProbes) != 1 || pack.NextProbes[0].Action != ProbeSearchTaskTerms {
		t.Fatalf("zero-evidence pack = %#v", pack)
	}
}

func TestBundleValidateRejectsInvalidOrUnboundedModules(t *testing.T) {
	repo := newTaskLensTestRepo(t, "module-contract")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}

	invalid := bundle
	invalid.Modules = append([]Module(nil), bundle.Modules...)
	invalid.Modules[0].Dir = "../outside"
	if err := invalid.Validate(); err == nil || !strings.Contains(err.Error(), "invalid module index entry") {
		t.Fatalf("invalid module error = %v", err)
	}

	unbounded := bundle
	unbounded.Modules = make([]Module, 0, MaxModules+1)
	for index := 0; index <= MaxModules; index++ {
		dir := "module-" + strconv.Itoa(index)
		modulePath := "example.com/fixture/" + dir
		unbounded.Modules = append(unbounded.Modules, Module{
			ID: OpaqueID("module", modulePath, dir), Path: modulePath, Dir: dir,
		})
	}
	if err := unbounded.Validate(); err == nil || !strings.Contains(err.Error(), "module index is outside bounds") {
		t.Fatalf("unbounded module error = %v", err)
	}

	badBudget := bundle
	badBudget.Budgets.RetainedAnchors--
	if err := badBudget.Validate(); err == nil || !strings.Contains(err.Error(), "budget accounting is inconsistent") {
		t.Fatalf("retained-anchor accounting error = %v", err)
	}
	badBudget = bundle
	badBudget.Budgets.ReadFiles = 0
	if err := badBudget.Validate(); err == nil || !strings.Contains(err.Error(), "budget accounting is inconsistent") {
		t.Fatalf("read-file accounting error = %v", err)
	}
	badBudget = bundle
	badBudget.Budgets.CandidateLimitBound = !badBudget.Budgets.CandidateLimitBound
	if err := badBudget.Validate(); err == nil || !strings.Contains(err.Error(), "budget accounting is inconsistent") {
		t.Fatalf("bound-flag accounting error = %v", err)
	}
}

func TestCollectIgnoresAmbientAlternateGitIdentity(t *testing.T) {
	wanted := newTaskLensTestRepo(t, "wanted")
	other := newTaskLensTestRepo(t, "other")
	wantedHead := gitTestOutput(t, wanted, "rev-parse", "HEAD")
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git", "index"))
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: wanted,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Repository.Revision != wantedHead {
		t.Fatalf("captured revision = %q, want %q", bundle.Repository.Revision, wantedHead)
	}
}

func TestCollectIdentityIgnoresNestedModuleAndFallsThroughUnnormalizableRemote(t *testing.T) {
	repo := newTaskLensTestRepo(t, "nested-module")
	runGitTest(t, repo, "rm", "--quiet", "go.mod")
	if err := os.MkdirAll(filepath.Join(repo, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(repo, "tools", "go.mod"),
		[]byte("module example.com/tasklensfixture/tools\n\ngo 1.24\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(repo, "package.json"),
		[]byte("{\n  \"name\": \"manifest-fallback\"\n}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "--quiet", "-m", "use nested module and manifest")
	runGitTest(t, repo, "remote", "add", "origin", "../local-mirror")

	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Repository.Identity != "manifest-fallback" || bundle.Repository.IdentitySource != "manifest" {
		t.Fatalf("repository identity = %#v, want tracked manifest fallback", bundle.Repository)
	}
	if len(bundle.Modules) != 1 || bundle.Modules[0].Dir != "tools" {
		t.Fatalf("module index = %#v, want retained nested module without identity authority", bundle.Modules)
	}
}

func TestCollectIdentityIgnoresAmbientGitConfigOverrides(t *testing.T) {
	repo := newTaskLensTestRepo(t, "ambient-config")
	runGitTest(t, repo, "rm", "--quiet", "go.mod")
	if err := os.WriteFile(
		filepath.Join(repo, "package.json"),
		[]byte("{\n  \"name\": \"manifest-should-not-win\"\n}\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "package.json")
	runGitTest(t, repo, "commit", "--quiet", "-m", "replace root module with manifest")
	runGitTest(t, repo, "remote", "add", "origin", "https://github.com/wanted/repository.git")

	ambientConfig := filepath.Join(t.TempDir(), "ambient.gitconfig")
	if err := os.WriteFile(
		ambientConfig,
		[]byte("[core]\n\tbare = true\n\tworktree = /definitely/not/the/requested/repository\n[remote \"origin\"]\n\turl = https://example.invalid/ambient/repository.git\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG", ambientConfig)
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "remote.origin.url")
	t.Setenv("GIT_CONFIG_VALUE_0", "https://example.invalid/injected/repository.git")
	t.Setenv("GIT_CONFIG_KEY_1", "core.bare")
	t.Setenv("GIT_CONFIG_VALUE_1", "true")
	t.Setenv("GIT_CONFIG_PARAMETERS", "'remote.origin.url'='https://example.invalid/parameters/repository.git'")
	t.Setenv("GIT_CONFIG_GLOBAL", ambientConfig)
	t.Setenv("GIT_CONFIG_SYSTEM", ambientConfig)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Repository.Identity != "github.com/wanted/repository" || bundle.Repository.IdentitySource != "remote" {
		t.Fatalf("repository identity = %#v, want repository-local origin", bundle.Repository)
	}
}

func TestIsolatedGitEnvironmentDropsAmbientConfigOverrides(t *testing.T) {
	environment := []string{
		"PATH=/usr/bin", "GIT_DIR=/tmp/other.git", "GIT_CONFIG=/tmp/config",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=remote.origin.url",
		"GIT_CONFIG_VALUE_0=https://example.invalid/repository.git",
		"GIT_CONFIG_PARAMETERS='remote.origin.url'='https://example.invalid/parameters.git'",
		"GIT_CONFIG_SYSTEM=/tmp/system", "GIT_CONFIG_GLOBAL=/tmp/global", "GIT_CONFIG_NOSYSTEM=1",
	}
	got := isolatedGitEnvironment(environment)
	joined := strings.Join(got, "\n")
	for _, forbidden := range []string{
		"GIT_DIR=/tmp/other.git", "GIT_CONFIG=/tmp/config", "GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=", "GIT_CONFIG_VALUE_0=", "GIT_CONFIG_PARAMETERS=",
		"GIT_CONFIG_SYSTEM=/tmp/system", "GIT_CONFIG_GLOBAL=/tmp/global",
	} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("isolated environment retained %q: %q", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "PATH=/usr/bin") || !strings.Contains(joined, "GIT_OPTIONAL_LOCKS=0") ||
		!strings.Contains(joined, "GIT_CONFIG_NOSYSTEM=1") ||
		!strings.Contains(joined, "GIT_CONFIG_SYSTEM="+os.DevNull) ||
		!strings.Contains(joined, "GIT_CONFIG_GLOBAL="+os.DevNull) {
		t.Fatalf("isolated environment lost required entries: %q", joined)
	}
}

func TestPromptBundleDoesNotLeakCheckoutBasename(t *testing.T) {
	root := t.TempDir()
	source := newTaskLensTestRepoIn(t, filepath.Join(root, "source"))
	first := filepath.Join(root, "nil-panic-task")
	second := filepath.Join(root, "unrelated-display-name")
	runGitTest(t, root, "clone", "--quiet", source, first)
	runGitTest(t, root, "clone", "--quiet", source, second)
	task := "The Enabled configuration is ignored; inspect CopyConfig and its test."
	a, err := Collect(context.Background(), CollectOptions{RepositoryPath: first, TaskText: task})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Collect(context.Background(), CollectOptions{RepositoryPath: second, TaskText: task})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.PromptBundle(), b.PromptBundle()) {
		aJSON, _ := json.MarshalIndent(a.PromptBundle(), "", "  ")
		bJSON, _ := json.MarshalIndent(b.PromptBundle(), "", "  ")
		t.Fatalf("semantic bundles differ:\n%s\n---\n%s", aJSON, bJSON)
	}
	encoded, _ := json.Marshal(a.PromptBundle())
	if strings.Contains(string(encoded), "nil-panic-task") || strings.Contains(string(encoded), "unrelated-display-name") {
		t.Fatalf("provider bundle leaked display basename: %s", encoded)
	}
}

func TestBuildSynthesisPromptScansTheCompletePromptBundle(t *testing.T) {
	repo := newTaskLensTestRepo(t, "prompt-secret")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	// A retained module fact and its source path are model-visible even when no
	// anchor cites that file, so this exercises the final transport boundary.
	secretDir := "sk-0123456789abcdef"
	modulePath := "example.com/fixture/secret"
	bundle.Modules = append(bundle.Modules, Module{
		ID: OpaqueID("module", modulePath, secretDir), Path: modulePath,
		Dir: secretDir, SourcePath: secretDir + "/go.mod",
	})
	bundle.AllowedPaths = append(bundle.AllowedPaths, secretDir+"/go.mod")
	sort.Strings(bundle.AllowedPaths)
	bundle.Budgets.ReadFiles++
	bundle.Budgets.ReadBytes++
	bundle.Metrics.TrackedFiles++
	bundle.Metrics.ModuleFilesFound++
	bundle.Metrics.ModuleFilesRead++
	bundle.Metrics.ModuleBytesRead++
	bundle.Metrics.ManifestFilesRead++
	bundle.Metrics.ManifestBytesRead++
	if _, err := BuildSynthesisPrompt(bundle); err == nil || !strings.Contains(err.Error(), "secret key") {
		t.Fatalf("prompt-bundle secret error = %v", err)
	}
}

func TestBundleValidateRequiresTaskEvidenceAndRepositoryGroundedTerms(t *testing.T) {
	repo := newTaskLensTestRepo(t, "task-evidence-boundary")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The ImaginarySetting is ignored; inspect Enabled and CopyConfig.",
	})
	if err != nil {
		t.Fatal(err)
	}

	missingTaskEvidence := bundle
	missingTaskEvidence.Evidence = append([]Evidence(nil), bundle.Evidence...)
	for index, item := range missingTaskEvidence.Evidence {
		if item.ID == bundle.Task.EvidenceID {
			missingTaskEvidence.Evidence[index] = Evidence{
				ID: OpaqueID("evidence", "replacement"), Kind: EvidenceRepositoryFact,
				Path: bundle.AllowedPaths[0], StartLine: 1, EndLine: 1,
				Summary: "Unlinked repository evidence used to preserve the collection bound.",
			}
			break
		}
	}
	if err := missingTaskEvidence.Validate(); err == nil || !strings.Contains(err.Error(), "exact task-provided evidence") {
		t.Fatalf("missing task evidence error = %v", err)
	}

	falseRepositoryGrounding := bundle
	falseRepositoryGrounding.Terms = append([]Term(nil), bundle.Terms...)
	termIndex := -1
	for index := range falseRepositoryGrounding.Terms {
		if falseRepositoryGrounding.Terms[index].Normalized == "imaginarysetting" {
			termIndex = index
			break
		}
	}
	if termIndex < 0 {
		t.Fatalf("fixture did not retain ImaginarySetting: %#v", bundle.Terms)
	}
	falseRepositoryGrounding.Terms[termIndex].Found = true
	falseRepositoryGrounding.Terms[termIndex].EvidenceIDs = []string{bundle.Task.EvidenceID}
	if err := falseRepositoryGrounding.Validate(); err == nil || !strings.Contains(err.Error(), "deterministic grounding") {
		t.Fatalf("task-only term grounding error = %v", err)
	}
}

func TestBuildPackRejectsTaskOnlySupportedCausalClaimAndInventedRelation(t *testing.T) {
	repo := newTaskLensTestRepo(t, "reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Hypothesis = []ProposedClause{{
		Status: HypothesisSupported, Text: "This causes the runtime failure.",
		SupportIDs: []string{bundle.Task.EvidenceID},
	}}
	if _, err := BuildPack(bundle, proposal); err == nil || !strings.Contains(err.Error(), "exceeds local evidence") {
		t.Fatalf("BuildPack task-only causal claim error = %v", err)
	}
	proposal, _ = LocalProposal(bundle)
	proposal.Joins = []ProposedJoin{{
		LeftID: proposal.Anchors[0].AnchorID, RightID: proposal.Anchors[1].AnchorID,
		RelationID: "relation-invented", Kind: "calls", SupportType: SupportLocallyObserved,
		SupportIDs:  bundle.Anchors[0].EvidenceIDs,
		Explanation: "The left anchor calls the right anchor.",
		Scope:       "The bounded source does not prove runtime reachability.",
	}}
	if _, err := BuildPack(bundle, proposal); err == nil || !strings.Contains(err.Error(), "matching local relation") {
		t.Fatalf("BuildPack invented relation error = %v", err)
	}
	proposal, _ = LocalProposal(bundle)
	proposal.Hypothesis = []ProposedClause{{
		Status:     HypothesisSupported,
		Text:       "ImaginaryHelper is present in the retained implementation.",
		SupportIDs: append([]string(nil), bundle.Anchors[0].EvidenceIDs...),
	}}
	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pack.WorkingHypothesis[0].Text, "ImaginaryHelper") ||
		pack.WorkingHypothesis[0].Text != supportedClauseText(proposal.Hypothesis[0].SupportIDs, nil, newBundleIndex(bundle)) {
		t.Fatalf("invented supported prose was not reconstructed: %#v", pack.WorkingHypothesis)
	}
	proposal, _ = LocalProposal(bundle)
	proposal.Joins = []ProposedJoin{{
		LeftID: proposal.Anchors[0].AnchorID, RightID: proposal.Anchors[1].AnchorID,
		Kind: "possible_dependency", SupportType: SupportModelHypothesis,
		SupportIDs:  append([]string(nil), bundle.Anchors[0].EvidenceIDs...),
		Explanation: "These anchors may be related.",
		Scope:       "This remains a model hypothesis.",
	}}
	if _, err := BuildPack(bundle, proposal); err == nil || !strings.Contains(err.Error(), "both endpoints") {
		t.Fatalf("BuildPack unrelated model-join support error = %v", err)
	}
	proposal, _ = LocalProposal(bundle)
	proposal.Interpretation.Observable = "The generated invented/schema.proto changes."
	if _, err := BuildPack(bundle, proposal); err == nil || !strings.Contains(err.Error(), "invalid proposed interpretation") {
		t.Fatalf("BuildPack invented path error = %v", err)
	}
}

func TestValidateProposalHeaderGroundsSlashListAgainstTask(t *testing.T) {
	index := bundleIndex{
		paths:    map[string]struct{}{},
		taskText: "The stack includes one of `SendJSON` / `SendXML` / `SendYAML` / `SendHTML` paths.",
	}
	tests := []struct {
		name        string
		restatement string
		wantErr     bool
	}{
		{
			name:        "task-grounded compact slash list",
			restatement: "Investigate SendJSON/SendXML/SendYAML/SendHTML after the write failure.",
		},
		{
			name:        "invented repository path",
			restatement: "Investigate generated/invented/schema.proto after the write failure.",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proposal := Proposal{Interpretation: ProposedInterpretation{
				Restatement: tt.restatement,
				Kind:        TaskBug,
				Observable:  "The service panics after a socket write failure.",
			}}
			err := validateProposalHeader(proposal, index)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateProposalHeader() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReduceProposalDropsInvalidJoinAndKeepsUsefulPack(t *testing.T) {
	repo := newTaskLensTestRepo(t, "partial-reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Joins = append(proposal.Joins, ProposedJoin{
		LeftID: proposal.Anchors[0].AnchorID, RightID: proposal.Anchors[1].AnchorID,
		Kind: "test_usage", SupportType: SupportDocument,
		SupportIDs:  bundle.Anchors[0].EvidenceIDs,
		Explanation: "The exact source and test are nearby.",
		Scope:       "This does not prove runtime execution.",
	})
	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "document support lacks document evidence") {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(pack.Anchors) < MinVisibleAnchors || len(pack.WorkingHypothesis) == 0 ||
		len(pack.EvidenceJoins) != len(proposal.Joins)-1 {
		t.Fatalf("reduced pack = %#v", pack)
	}
}

func TestReduceProposalNormalizesKnownAnchorPresentation(t *testing.T) {
	repo := newTaskLensTestRepo(t, "anchor-presentation-reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	anchor := anchorByID(bundle.Anchors, proposal.Anchors[0].AnchorID)
	unsupportedRole := RoleGeneratedOutput
	for _, candidate := range []AnchorRole{RoleGeneratedOutput, RoleIntegrationBoundary, RoleErrorMapping} {
		if !slices.Contains(anchor.RoleHints, candidate) {
			unsupportedRole = candidate
			break
		}
	}

	roleMismatch := proposal
	roleMismatch.Anchors = append([]ProposedAnchor(nil), proposal.Anchors...)
	roleMismatch.Anchors[0].Role = unsupportedRole
	roleMismatch.Anchors[0].Why = "Group/Use is an ungrounded presentation label."
	if _, err := BuildPack(bundle, roleMismatch); err == nil || !strings.Contains(err.Error(), "proposed anchor is invalid") {
		t.Fatalf("strict BuildPack role error = %v", err)
	}
	pack, warnings, err := ReduceProposal(bundle, roleMismatch)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "locally allowed role") {
		t.Fatalf("role normalization warnings = %#v", warnings)
	}
	if pack.Anchors[0].Role != anchor.RoleHints[0] || pack.Anchors[0].Why != localAnchorWhy() {
		t.Fatalf("normalized anchor = %#v", pack.Anchors[0])
	}

	invalidWhy := proposal
	invalidWhy.Anchors = append([]ProposedAnchor(nil), proposal.Anchors...)
	invalidWhy.Anchors[0].Why = "The Group/Use presentation is not a repository path."
	pack, warnings, err = ReduceProposal(bundle, invalidWhy)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "explanation was replaced") ||
		pack.Anchors[0].Why != localAnchorWhy() {
		t.Fatalf("explanation normalization: warnings=%#v anchor=%#v", warnings, pack.Anchors[0])
	}

	unknown := proposal
	unknown.Anchors = append([]ProposedAnchor(nil), proposal.Anchors...)
	unknown.Anchors[0].AnchorID = "anchor-unknown"
	if _, _, err := ReduceProposal(bundle, unknown); err == nil || !strings.Contains(err.Error(), "unknown anchor id") {
		t.Fatalf("unknown anchor error = %v", err)
	}
	duplicate := proposal
	duplicate.Anchors = append([]ProposedAnchor(nil), proposal.Anchors...)
	duplicate.Anchors[1] = duplicate.Anchors[0]
	if _, _, err := ReduceProposal(bundle, duplicate); err == nil || !strings.Contains(err.Error(), "repeats an anchor id") {
		t.Fatalf("duplicate anchor error = %v", err)
	}
}

func TestReduceProposalNormalizesLikelyAreaPresentation(t *testing.T) {
	repo := newTaskLensTestRepo(t, "area-presentation-reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	local, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	minimumVisible := min(PreferredMinVisibleAnchors, len(bundle.Anchors))
	if len(local.Anchors) <= minimumVisible {
		t.Fatalf("fixture lacks an unselected supplied anchor: %#v", local.Anchors)
	}
	proposal := local
	proposal.Anchors = append([]ProposedAnchor(nil), local.Anchors[:minimumVisible]...)
	selected := make(map[string]struct{}, len(proposal.Anchors))
	var selectedIDs []string
	for _, anchor := range proposal.Anchors {
		selected[anchor.AnchorID] = struct{}{}
		selectedIDs = append(selectedIDs, anchor.AnchorID)
	}
	unselectedID := local.Anchors[minimumVisible].AnchorID
	proposal.Joins = nil
	fallback, _ := localFallbackHypothesis(bundle, selected, newBundleIndex(bundle))
	proposal.Hypothesis = []ProposedClause{fallback}
	proposal.ReproduceOrObserve = []ProposedGuidance{{
		Text: "Use the task observation.", Authority: AuthorityTaskProvided,
		EvidenceIDs: []string{bundle.Task.EvidenceID},
	}}
	proposal.Verify.Steps = []ProposedGuidance{{Text: "Obtain repository verification.", Authority: AuthorityMissing}}
	proposal.NextProbes = []ProposedProbe{{
		Action: ProbeInspectSymbol, AnchorIDs: []string{selectedIDs[0]}, Text: "Inspect the selected anchor.",
	}}

	strict := proposal
	strict.Areas = []ProposedArea{{
		Label: "Selected area", Why: "Contains a selected and an unselected supplied anchor.",
		TargetIDs: []string{selectedIDs[0], unselectedID},
	}}
	if _, err := BuildPack(bundle, strict); err == nil || !strings.Contains(err.Error(), "area target is not a visible anchor") {
		t.Fatalf("strict BuildPack area target error = %v", err)
	}

	proposal.Areas = []ProposedArea{
		{
			Label: "Group/Use", Why: "",
			TargetIDs: []string{selectedIDs[0], selectedIDs[0], unselectedID, "anchor-unknown"},
		},
		{Label: "Empty after filtering", Why: "Only unselected targets.", TargetIDs: []string{unselectedID, "anchor-unknown"}},
		{Label: "Area two", Why: "Selected bounded area.", TargetIDs: []string{selectedIDs[1]}},
		{Label: "Area three", Why: "Selected bounded area.", TargetIDs: []string{selectedIDs[2]}},
		{Label: "Area four", Why: "Beyond the presentation bound.", TargetIDs: []string{selectedIDs[0]}},
	}
	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	warningText := strings.Join(warnings, "\n")
	for _, want := range []string{"filtered to unique selected anchors", "did not retain a selected anchor", "replaced with local wording", "presentation bound"} {
		if !strings.Contains(warningText, want) {
			t.Fatalf("area warnings %q omit %q", warningText, want)
		}
	}
	if len(pack.LikelyAreas) != 3 || !slices.Equal(pack.LikelyAreas[0].TargetIDs, []string{selectedIDs[0]}) {
		t.Fatalf("normalized likely areas = %#v", pack.LikelyAreas)
	}
	for _, area := range pack.LikelyAreas {
		if len(area.TargetIDs) == 0 || slices.Contains(area.TargetIDs, unselectedID) ||
			slices.Contains(area.TargetIDs, "anchor-unknown") {
			t.Fatalf("area retained a filtered target: %#v", area)
		}
	}

	missingAreas := proposal
	missingAreas.Areas = nil
	if _, err := BuildPack(bundle, missingAreas); err == nil || !strings.Contains(err.Error(), "likely area count") {
		t.Fatalf("strict BuildPack missing-area error = %v", err)
	}
	pack, warnings, err = ReduceProposal(bundle, missingAreas)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.LikelyAreas) == 0 || !strings.Contains(strings.Join(warnings, "\n"), "deterministic local likely area") {
		t.Fatalf("missing-area fallback: warnings=%#v areas=%#v", warnings, pack.LikelyAreas)
	}

	unknownAnchor := proposal
	unknownAnchor.Anchors = append([]ProposedAnchor(nil), proposal.Anchors...)
	unknownAnchor.Anchors[0].AnchorID = "anchor-unknown"
	if _, _, err := ReduceProposal(bundle, unknownAnchor); err == nil || !strings.Contains(err.Error(), "unknown anchor id") {
		t.Fatalf("unknown selected anchor error = %v", err)
	}
}

func TestReduceProposalCompletesExactVisibleRelationEvidence(t *testing.T) {
	repo := newTaskLensTestRepo(t, "relation-evidence-reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and TestCopyConfigEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	selected := make(map[string]struct{}, len(proposal.Anchors))
	for _, anchor := range proposal.Anchors {
		selected[anchor.AnchorID] = struct{}{}
	}
	relations := rankedLocalProposalRelations(bundle, selected)
	if len(relations) == 0 || len(relations[0].EvidenceIDs) < 2 {
		t.Fatalf("fixture lacks a visible relation with endpoint evidence: %#v", relations)
	}
	relation := relations[0]
	proposal.Hypothesis = []ProposedClause{{
		Status: HypothesisSupported,
		Text:   "The model presents this exact local relation as a supported fact.",
		SupportIDs: []string{
			relation.EvidenceIDs[0],
		},
		RelationIDs: []string{relation.ID},
	}}
	if _, err := BuildPack(bundle, proposal); err == nil || !strings.Contains(err.Error(), "relation evidence is not included") {
		t.Fatalf("strict BuildPack relation evidence error = %v", err)
	}
	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(pack.WorkingHypothesis[0].SupportIDs, relation.EvidenceIDs) ||
		!strings.Contains(strings.Join(warnings, "\n"), "exact local relation evidence") {
		t.Fatalf("completed relation clause: warnings=%#v clause=%#v", warnings, pack.WorkingHypothesis[0])
	}

	unknownRelation := proposal
	unknownRelation.Hypothesis = []ProposedClause{{
		Status: HypothesisSupported, Text: "An unknown relation must not become local evidence.",
		SupportIDs: []string{relation.EvidenceIDs[0]}, RelationIDs: []string{"relation-unknown"},
	}}
	if _, err := BuildPack(bundle, unknownRelation); err == nil || !strings.Contains(err.Error(), "unknown relation id") {
		t.Fatalf("strict BuildPack unknown relation error = %v", err)
	}
	pack, warnings, err = ReduceProposal(bundle, unknownRelation)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "unknown relation id") ||
		len(pack.WorkingHypothesis) == 0 || slices.Contains(pack.WorkingHypothesis[0].RelationIDs, "relation-unknown") {
		t.Fatalf("unknown relation fallback: warnings=%#v clauses=%#v", warnings, pack.WorkingHypothesis)
	}

	outsideVisible := proposal
	outsideVisible.Anchors = nil
	var targetIDs []string
	for _, proposed := range proposal.Anchors {
		if proposed.AnchorID == relation.RightID {
			continue
		}
		outsideVisible.Anchors = append(outsideVisible.Anchors, proposed)
		targetIDs = append(targetIDs, proposed.AnchorID)
	}
	minimumVisible := min(PreferredMinVisibleAnchors, len(bundle.Anchors))
	if len(outsideVisible.Anchors) < minimumVisible {
		t.Fatalf("fixture cannot exclude one relation endpoint: anchors=%#v", proposal.Anchors)
	}
	outsideVisible.Areas = []ProposedArea{{
		Label: "Visible retained anchors", Why: "Groups only the selected retained anchors.", TargetIDs: targetIDs,
	}}
	outsideVisible.Joins = nil
	outsideVisible.Hypothesis = []ProposedClause{{
		Status: HypothesisSupported, Text: "An out-of-view relation must not become visible evidence.",
		SupportIDs:  append([]string(nil), anchorByID(bundle.Anchors, relation.LeftID).EvidenceIDs...),
		RelationIDs: []string{relation.ID},
	}}
	pack, warnings, err = ReduceProposal(bundle, outsideVisible)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "relation is outside visible anchors") {
		t.Fatalf("out-of-view relation warnings = %#v", warnings)
	}
	for _, clause := range pack.WorkingHypothesis {
		if slices.Contains(clause.RelationIDs, relation.ID) {
			t.Fatalf("out-of-view relation survived reduction: %#v", pack.WorkingHypothesis)
		}
	}
}

func TestReduceProposalRestoresRequiredSectionsFromLocalEvidence(t *testing.T) {
	repo := newTaskLensTestRepo(t, "required-section-reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.ReproduceOrObserve = []ProposedGuidance{{
		Text: "Treat an unknown item as a reproduction.", Authority: AuthorityRepositoryObservation,
		EvidenceIDs: []string{"evidence-unknown"},
	}}
	proposal.Verify.Steps = []ProposedGuidance{{
		Text: "Treat task prose as repository verification.", Authority: AuthorityTaskProvided,
	}}
	proposal.NextProbes = []ProposedProbe{{
		Action: ProbeInspectSymbol, AnchorIDs: []string{"anchor-unknown"}, Text: "Inspect an unknown anchor.",
	}}
	if _, err := BuildPack(bundle, proposal); err == nil {
		t.Fatal("strict BuildPack accepted invalid required sections")
	}

	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	warningText := strings.Join(warnings, "\n")
	for _, want := range []string{"bounded local reproduction", "repository-owned verification", "bounded local next probe"} {
		if !strings.Contains(warningText, want) {
			t.Fatalf("fallback warnings %q omit %q", warningText, want)
		}
	}
	if len(pack.ReproduceOrObserve) == 0 || len(pack.Verify.Steps) == 0 || len(pack.NextProbes) == 0 {
		t.Fatalf("required sections were not restored: %#v", pack)
	}
	if pack.Verify.Steps[0].Authority == AuthorityTaskProvided {
		t.Fatalf("task-provided prose became verification authority: %#v", pack.Verify.Steps)
	}
	visible := make(map[string]struct{}, len(pack.Anchors))
	for _, anchor := range pack.Anchors {
		visible[anchor.ID] = struct{}{}
	}
	for _, anchorID := range pack.NextProbes[0].AnchorIDs {
		if _, ok := visible[anchorID]; !ok {
			t.Fatalf("fallback probe references a non-visible anchor: %#v", pack.NextProbes[0])
		}
	}
}

func TestReduceProposalDeduplicatesGuidanceAndSupplementsSelectedTest(t *testing.T) {
	repo := newTaskLensTestRepo(t, "guidance-quality-reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.ReproduceOrObserve = nil
	for index := 0; index < MaxGuidanceSteps+1; index++ {
		proposal.ReproduceOrObserve = append(proposal.ReproduceOrObserve, ProposedGuidance{
			Text:      "Task-provided reproduction wording " + strconv.Itoa(index) + ".",
			Authority: AuthorityTaskProvided, EvidenceIDs: []string{bundle.Task.EvidenceID},
		})
	}
	proposal.Verify.Steps = []ProposedGuidance{
		{Text: "Missing verification one.", Authority: AuthorityMissing},
		{Text: "Missing verification two.", Authority: AuthorityMissing},
		{Text: "Task verification one.", Authority: AuthorityTaskProvided, EvidenceIDs: []string{bundle.Task.EvidenceID}},
		{Text: "Task verification two.", Authority: AuthorityTaskProvided, EvidenceIDs: []string{bundle.Task.EvidenceID}},
	}

	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	warningText := strings.Join(warnings, "\n")
	for _, want := range []string{"duplicates the same locally authoritative guidance", "selected repository test or example"} {
		if !strings.Contains(warningText, want) {
			t.Fatalf("guidance warnings %q omit %q", warningText, want)
		}
	}
	if len(pack.ReproduceOrObserve) != 1 {
		t.Fatalf("canonical reproduction guidance was not deduplicated: %#v", pack.ReproduceOrObserve)
	}
	if len(pack.ReproduceOrObserve) > MaxGuidanceSteps || len(pack.Verify.Steps) > MaxGuidanceSteps {
		t.Fatalf("guidance bound exceeded: reproduce=%d verify=%d", len(pack.ReproduceOrObserve), len(pack.Verify.Steps))
	}
	authorities := make(map[GuidanceAuthority]int)
	for _, guidance := range pack.Verify.Steps {
		authorities[guidance.Authority]++
	}
	if authorities[AuthorityMissing] != 1 || authorities[AuthorityTaskProvided] != 1 ||
		authorities[AuthorityRepositoryTest] != 1 || len(pack.Verify.Steps) != 3 {
		t.Fatalf("verification guidance was not deduplicated and supplemented: %#v", pack.Verify.Steps)
	}
}

func TestReduceProposalDoesNotSupplementVerificationWithoutSelectedTest(t *testing.T) {
	repo := newTaskLensTestRepo(t, "guidance-no-test-reducer")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Anchors = nil
	selected := make(map[string]struct{})
	for _, anchor := range bundle.Anchors {
		if isTestPath(anchor.Path) || len(proposal.Anchors) == MaxVisibleAnchors {
			continue
		}
		proposal.Anchors = append(proposal.Anchors, ProposedAnchor{
			AnchorID: anchor.ID, Role: anchor.RoleHints[0], Why: localAnchorWhy(),
		})
		selected[anchor.ID] = struct{}{}
	}
	minimumVisible := min(PreferredMinVisibleAnchors, len(bundle.Anchors))
	if len(proposal.Anchors) < minimumVisible {
		t.Fatalf("fixture lacks enough non-test anchors: %#v", bundle.Anchors)
	}
	var targetIDs []string
	for _, anchor := range proposal.Anchors {
		targetIDs = append(targetIDs, anchor.AnchorID)
	}
	proposal.Areas = []ProposedArea{{
		Label: "Selected source anchors", Why: "Groups only selected source anchors.", TargetIDs: targetIDs,
	}}
	proposal.Joins = nil
	proposal.Hypothesis = nil
	fallback, _ := localFallbackHypothesis(bundle, selected, newBundleIndex(bundle))
	proposal.Hypothesis = []ProposedClause{fallback}
	proposal.ReproduceOrObserve = []ProposedGuidance{{
		Text: "Use the task observation.", Authority: AuthorityTaskProvided,
		EvidenceIDs: []string{bundle.Task.EvidenceID},
	}}
	proposal.Verify.Steps = []ProposedGuidance{
		{Text: "Missing verification one.", Authority: AuthorityMissing},
		{Text: "Missing verification two.", Authority: AuthorityMissing},
		{Text: "Use only task prose.", Authority: AuthorityTaskProvided, EvidenceIDs: []string{bundle.Task.EvidenceID}},
	}
	proposal.NextProbes = []ProposedProbe{{
		Action: ProbeInspectSymbol, AnchorIDs: []string{proposal.Anchors[0].AnchorID}, Text: "Inspect the selected anchor.",
	}}

	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(warnings, "\n"), "selected repository test or example") {
		t.Fatalf("reducer claimed a test supplement without a selected test: %#v", warnings)
	}
	if len(pack.Verify.Steps) != 2 {
		t.Fatalf("low-authority verification was not deduplicated: %#v", pack.Verify.Steps)
	}
	for _, guidance := range pack.Verify.Steps {
		if guidance.Authority == AuthorityRepositoryTest {
			t.Fatalf("unselected test became verification authority: %#v", pack.Verify.Steps)
		}
	}
}

func TestCollectRecordsTypedDirectCallAndReducerPreservesItsKind(t *testing.T) {
	repo := newTaskLensTestRepo(t, "direct-call")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and TestCopyConfigEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	caller := anchorBySymbol(bundle.Anchors, "TestCopyConfigEnabled")
	target := anchorBySymbol(bundle.Anchors, "CopyConfig")
	found, parseErr := parsedAnchorCalls(caller, target)
	if !found {
		t.Fatalf("actual retained test anchor did not expose CopyConfig call: err=%v excerpt=%#v", parseErr, caller.Excerpt)
	}
	var direct Relation
	for _, relation := range bundle.Relations {
		left := anchorByID(bundle.Anchors, relation.LeftID)
		right := anchorByID(bundle.Anchors, relation.RightID)
		if relation.Kind == string(RelationDirectCall) && left.Symbol == "TestCopyConfigEnabled" && right.Symbol == "CopyConfig" {
			direct = relation
			break
		}
	}
	if direct.ID == "" {
		t.Fatalf("direct TestCopyConfigEnabled -> CopyConfig relation not found; anchors=%#v relations=%#v", bundle.Anchors, bundle.Relations)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Anchors = ensureSelectedAnchors(proposal.Anchors, direct.LeftID, direct.RightID)
	proposal.Joins = []ProposedJoin{{
		LeftID: direct.LeftID, RightID: direct.RightID, RelationID: direct.ID,
		Kind: direct.Kind, SupportType: SupportLocallyObserved,
		SupportIDs:  []string{direct.EvidenceIDs[0]},
		Explanation: "The caller delegates validation and returns an error.",
		Scope:       "This guarantees the callee executes on every request.",
	}}
	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(pack.EvidenceJoins) != 1 || pack.EvidenceJoins[0].Explanation != localRelationExplanation(direct.Kind) ||
		pack.EvidenceJoins[0].Scope != direct.Scope || !reflect.DeepEqual(pack.EvidenceJoins[0].SupportIDs, direct.EvidenceIDs) {
		t.Fatalf("local relation presentation was not reconstructed: %#v", pack.EvidenceJoins)
	}
	pack.EvidenceJoins[0].Scope = "This guarantees runtime execution."
	if err := ValidatePackAgainstBundle(bundle, pack); err == nil {
		t.Fatal("reducer replay accepted a mutated local scope")
	}
	proposal, err = LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Anchors = ensureSelectedAnchors(proposal.Anchors, direct.LeftID, direct.RightID)
	proposal.Joins = []ProposedJoin{{
		LeftID: direct.LeftID, RightID: direct.RightID, RelationID: direct.ID,
		Kind: "exact_identifier_reference", SupportType: SupportLocallyObserved,
		SupportIDs:  append([]string(nil), direct.EvidenceIDs...),
		Explanation: "The caller directly calls the callee in the retained excerpt.",
		Scope:       direct.Scope,
	}}
	_, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "changes the local relation kind") {
		t.Fatalf("warnings = %#v", warnings)
	}
	proposal, err = LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Anchors = ensureSelectedAnchors(proposal.Anchors, direct.LeftID, direct.RightID)
	proposal.Hypothesis = []ProposedClause{{
		Status:      HypothesisSupported,
		Text:        "TestCopyConfigEnabled calls CopyConfig and therefore causes the configuration update.",
		SupportIDs:  append([]string(nil), direct.EvidenceIDs...),
		RelationIDs: []string{direct.ID},
	}}
	pack, err = BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if pack.WorkingHypothesis[0].Text != localRelationExplanation(direct.Kind) ||
		strings.Contains(pack.WorkingHypothesis[0].Text, "causes") {
		t.Fatalf("supported causal prose was not reconstructed: %#v", pack.WorkingHypothesis)
	}
}

func TestAnchorCallsUsesParsedCallExpressions(t *testing.T) {
	anchor := Anchor{Path: "fixture.go", Package: "fixture", Excerpt: []SourceLine{
		{Line: 1, Text: "func caller() {"},
		{Line: 2, Text: "\t// Target() in a comment must not count."},
		{Line: 3, Text: "\t_ = `Target()`"},
		{Line: 4, Text: "\tTarget()"},
		{Line: 5, Text: "}"},
	}}
	target := Anchor{Path: "fixture.go", Package: "fixture", Symbol: "Target", Excerpt: []SourceLine{{Line: 10, Text: "func Target() {}"}}}
	if !anchorCalls(anchor, target) {
		t.Fatal("parsed direct call was not detected")
	}
	anchor.Excerpt = anchor.Excerpt[:3]
	if anchorCalls(anchor, target) {
		t.Fatal("comment or string was promoted to a direct call")
	}
	methodTarget := target
	methodTarget.Symbol = "Receiver.Target"
	methodTarget.Excerpt[0].Text = "func (Receiver) Target() {}"
	selectorCaller := Anchor{Path: "fixture.go", Package: "fixture", Excerpt: []SourceLine{
		{Line: 1, Text: "func caller(value Receiver) { value.Target() }"},
	}}
	if anchorCalls(selectorCaller, methodTarget) {
		t.Fatal("unresolved selector name was promoted to an exact endpoint relation")
	}
	shadowedCaller := Anchor{Path: "fixture.go", Package: "fixture", Excerpt: []SourceLine{
		{Line: 1, Text: "func caller(Target func()) { Target() }"},
	}}
	if anchorCalls(shadowedCaller, target) {
		t.Fatal("a locally shadowed function value was promoted to a package-function relation")
	}
}

func TestCollectRelationsRequiresTheSameGoPackageDirectoryForDirectCalls(t *testing.T) {
	caller := Anchor{
		ID: "anchor-caller", Path: "one/caller.go", Package: "fixture", Symbol: "Caller",
		Excerpt: []SourceLine{
			{Line: 1, Text: "func Caller() {"},
			{Line: 2, Text: "\tTarget()"},
			{Line: 3, Text: "}"},
		},
		EvidenceIDs: []string{"evidence-caller"},
	}
	target := Anchor{
		ID: "anchor-target", Path: "one/target.go", Package: "fixture", Symbol: "Target",
		Excerpt:     []SourceLine{{Line: 10, Text: "func Target() {}"}},
		EvidenceIDs: []string{"evidence-target"},
	}
	if relations := collectRelations([]Anchor{caller, target}, nil); !hasRelationKind(relations, relationKindDirectCall) {
		t.Fatalf("same-directory call was not retained: %#v", relations)
	}

	target.Path = "two/target.go"
	if anchorCalls(caller, target) {
		t.Fatal("same package clause in another directory was treated as the same Go package")
	}
	if relations := collectRelations([]Anchor{caller, target}, nil); hasRelationKind(relations, relationKindDirectCall) {
		t.Fatalf("cross-directory package-name match became a direct call: %#v", relations)
	}
}

func TestCollectRelationsDoesNotPromoteIdentifierSubstrings(t *testing.T) {
	run := Anchor{
		ID: "anchor-run", Path: "run.go", Package: "fixture", Symbol: "Run",
		Excerpt: []SourceLine{{Line: 1, Text: "func Run() {}"}}, EvidenceIDs: []string{"evidence-run"},
	}
	runtime := Anchor{
		ID: "anchor-runtime", Path: "runtime.go", Package: "fixture", Symbol: "Runtime",
		Excerpt:     []SourceLine{{Line: 1, Text: "func Runtime() string { return \"runtime settings\" }"}},
		EvidenceIDs: []string{"evidence-runtime"},
	}
	if relations := collectRelations([]Anchor{run, runtime}, nil); len(relations) != 0 {
		t.Fatalf("identifier substring became a local relation: %#v", relations)
	}
}

func TestSharedConfigurationRelationRequiresPrimaryTaskField(t *testing.T) {
	t.Parallel()

	terms := extractTerms("OpenAPIConfig drops DisableMessages while applying configuration.")
	source := Anchor{
		Path: "engine.go", Symbol: "Engine.read", RoleHints: []AnchorRole{RoleEffectiveDestination},
		Excerpt: []SourceLine{{Line: 1, Text: "func (e Engine) read() { _ = e.OpenAPI; _ = e.DisableMessages }"}},
	}
	target := Anchor{
		Path: "engine.go", Symbol: "Config", RoleHints: []AnchorRole{RoleConfigurationSource},
		Excerpt: []SourceLine{{Line: 1, Text: "type Config struct { OpenAPI bool; DisableMessages bool }"}},
	}
	if got := classifySharedTermRelation(source, target, "openapi", terms); got != relationKindSharedTaskTerm {
		t.Fatalf("generic OpenAPI relation = %q, want %q", got, relationKindSharedTaskTerm)
	}
	if got := classifySharedTermRelation(source, target, "disablemessages", terms); got != string(RelationFieldRead) {
		t.Fatalf("primary configuration field relation = %q, want %q", got, RelationFieldRead)
	}
}

func TestErrorMappingDoesNotBindGenericInterfaceMethodToConcreteReceiver(t *testing.T) {
	t.Parallel()

	source := Anchor{
		Path: "serialization.go", Symbol: "SendError", RoleHints: []AnchorRole{RoleErrorMapping},
		Excerpt: []SourceLine{{Line: 1, Text: "func SendError(err ErrorWithStatus) { _ = err.StatusCode() }"}},
	}
	target := Anchor{
		Path: "errors.go", Symbol: "NotAcceptableError.StatusCode", RoleHints: []AnchorRole{RoleErrorCreation},
		Excerpt: []SourceLine{{Line: 1, Text: "func (NotAcceptableError) StatusCode() int { return 406 }"}},
	}
	if got := classifyReferencedRelation(source, target, nil); got != relationKindExactIdentifier {
		t.Fatalf("generic interface method relation = %q, want %q", got, relationKindExactIdentifier)
	}

	typed := source
	typed.Excerpt = []SourceLine{{
		Line: 1,
		Text: "func SendError(err error) { var target NotAcceptableError; _ = errors.As(err, &target) }",
	}}
	if got := classifyReferencedRelation(typed, target, nil); got != string(RelationErrorMapped) {
		t.Fatalf("typed errors.As relation = %q, want %q", got, RelationErrorMapped)
	}
}

func TestBundleValidateRecomputesLocalRelationAuthority(t *testing.T) {
	repo := newTaskLensTestRepo(t, "relation-authority")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "Inspect the direct CopyConfig reference in TestCopyConfigEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Relations) == 0 {
		t.Fatal("fixture did not produce a local relation")
	}
	tampered := bundle
	tampered.Relations = append([]Relation(nil), bundle.Relations...)
	tampered.Relations[0].EvidenceIDs = tampered.Relations[0].EvidenceIDs[:1]
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "does not match retained syntax") {
		t.Fatalf("tampered relation evidence error = %v", err)
	}
	tampered = bundle
	tampered.Relations = append([]Relation(nil), bundle.Relations...)
	tampered.Relations[0].ID = OpaqueID("relation", "forged")
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "does not match retained syntax") {
		t.Fatalf("tampered relation id error = %v", err)
	}
	tampered = bundle
	tampered.Relations = append([]Relation(nil), bundle.Relations...)
	tampered.Relations[0].Scope = "This forged scope claims runtime causality."
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "does not match retained syntax") {
		t.Fatalf("tampered relation scope error = %v", err)
	}
	directIndex := -1
	for index, relation := range bundle.Relations {
		if relation.Kind == relationKindDirectCall {
			directIndex = index
			break
		}
	}
	if directIndex < 0 {
		t.Fatalf("fixture did not produce a direct relation: %#v", bundle.Relations)
	}
	tampered = bundle
	tampered.Relations = append([]Relation(nil), bundle.Relations...)
	tampered.Relations[directIndex].Kind = relationKindExactIdentifier
	tampered.Relations[directIndex].Scope = relationScope(relationKindExactIdentifier)
	key := tampered.Relations[directIndex].LeftID + "\x00" + tampered.Relations[directIndex].RightID + "\x00" + relationKindExactIdentifier
	tampered.Relations[directIndex].ID = OpaqueID("relation", key)
	if err := tampered.Validate(); err == nil || !strings.Contains(err.Error(), "does not match retained syntax") {
		t.Fatalf("downgraded relation kind error = %v", err)
	}
}

func TestLocalProposalAcceptsNestedAreasAndTaskGroundedUnretainedPaths(t *testing.T) {
	repo := newTaskLensTestRepo(t, "nested-local-proposal")
	if err := os.MkdirAll(filepath.Join(repo, "internal", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(repo, "internal", "foo", "bar.go"),
		[]byte("package foo\n\nfunc Bar() bool { return true }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "internal/foo/bar.go")
	runGitTest(t, repo, "commit", "--quiet", "-m", "add nested anchor")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "Inspect Bar in internal/foo and preserve the requested missing/config.yaml outcome.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Verify.Effect = "Observe the requested missing/config.yaml outcome."
	if _, err := BuildPack(bundle, proposal); err != nil {
		t.Fatalf("deterministic nested fallback was rejected: %v", err)
	}
}

func TestVerificationEffectIsReconstructedFromTheTaskObservable(t *testing.T) {
	repo := newTaskLensTestRepo(t, "verification-effect")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and its test.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.Verify.Effect = "An invented HTTP endpoint returns status 299."
	want := localVerificationEffectForBundle(bundle)

	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Verify.Effect != want || strings.Contains(pack.Verify.Effect, "299") {
		t.Fatalf("verification effect = %q, want %q", pack.Verify.Effect, want)
	}

	proposal.Verify.Effect = ""
	reduced, _, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if reduced.Verify.Effect != want {
		t.Fatalf("reduced verification effect = %q, want %q", reduced.Verify.Effect, want)
	}
}

func TestReducerRejectsSupportedClauseThatNamesAnUnsupportedAnchor(t *testing.T) {
	repo := newTaskLensTestRepo(t, "unsupported-symbol")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and TestCopyConfigEnabled.",
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
	proposal.Hypothesis = []ProposedClause{
		{
			Status: HypothesisSupported,
			Text:   "CopyConfig and ReadEnabled are both shown by the retained source.",
			// Deliberately omit ReadEnabled's evidence.
			SupportIDs: append([]string(nil), copyConfig.EvidenceIDs...),
		},
		{
			Status:     HypothesisPlausible,
			Text:       "The retained configuration anchors are a bounded starting point.",
			SupportIDs: append([]string(nil), copyConfig.EvidenceIDs...),
		},
	}
	pack, warnings, err := ReduceProposal(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "prose was replaced") ||
		!strings.Contains(warnings[1], "prose was replaced") {
		t.Fatalf("warnings = %#v", warnings)
	}
	if len(pack.WorkingHypothesis) != 2 || pack.WorkingHypothesis[0].Status != HypothesisSupported ||
		strings.Contains(pack.WorkingHypothesis[0].Text, "ReadEnabled") ||
		pack.WorkingHypothesis[1].Status != HypothesisPlausible {
		t.Fatalf("locally reconstructed hypothesis = %#v", pack.WorkingHypothesis)
	}
}

func TestValidatePackAgainstBundleReplaysRelationAuthority(t *testing.T) {
	repo := newTaskLensTestRepo(t, "pack-replay")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and TestCopyConfigEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePackAgainstBundle(bundle, pack); err != nil {
		t.Fatalf("valid pack replay: %v", err)
	}
	if len(pack.EvidenceJoins) == 0 {
		t.Fatal("fixture did not produce a local relation")
	}
	pack.EvidenceJoins[0].Kind = "invented_relation_kind"
	if err := ValidatePackAgainstBundle(bundle, pack); err == nil || !strings.Contains(err.Error(), "changes the local relation kind") {
		t.Fatalf("tampered relation kind error = %v", err)
	}
}

func TestCanonicalHypothesisGroundsNestedFileFallbackSymbol(t *testing.T) {
	const (
		anchorID   = "anchor-json"
		evidenceID = "evidence-json"
		filePath   = "examples/petstore/lib/testdata/doc/openapi.golden.json"
	)
	index := bundleIndex{
		anchors: map[string]Anchor{
			anchorID: {
				ID: anchorID, Path: filePath,
				Symbol: "openapi.golden.json:1156", StartLine: 1148,
			},
		},
		evidence: map[string]Evidence{
			evidenceID: {ID: evidenceID, Kind: EvidenceRepositoryFact, AnchorID: anchorID},
		},
		paths: map[string]struct{}{filePath: {}},
	}

	selected := map[string]struct{}{anchorID: {}}
	clause, err := buildClause(ProposedClause{
		Status: HypothesisPlausible, Text: "The exact retained schema evidence is a bounded lead.",
		SupportIDs: []string{evidenceID},
	}, selected, index)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(clause.Text, filePath+":1156") {
		t.Fatalf("canonical plausible clause = %q, want exact allowed path and line", clause.Text)
	}
	if unknownPathInText(clause.Text, index.paths) {
		t.Fatalf("canonical plausible clause contains an ungrounded path: %q", clause.Text)
	}
	replayed, err := buildClause(ProposedClause{
		Status: clause.Status, Text: clause.Text,
		SupportIDs: clause.SupportIDs, RelationIDs: clause.RelationIDs,
	}, selected, index)
	if err != nil {
		t.Fatalf("replay canonical plausible clause: %v", err)
	}
	if !reflect.DeepEqual(replayed, clause) {
		t.Fatalf("replayed clause = %#v, want %#v", replayed, clause)
	}
}

func TestBuildPackRejectsGuidanceOutsideItsCitedAuthority(t *testing.T) {
	repo := newTaskLensTestRepo(t, "guidance-authority")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and TestCopyConfigEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{
		"invented command":              "Run `go test ./invented/...` and inspect the result.",
		"uncited symbol":                "Call ReadEnabled and inspect the result.",
		"invented plain English action": "Disable the configuration, restart the service, send the request twice, and expect a failure.",
	} {
		t.Run(name, func(t *testing.T) {
			proposal, proposalErr := LocalProposal(bundle)
			if proposalErr != nil {
				t.Fatal(proposalErr)
			}
			proposal.ReproduceOrObserve[0].Text = text
			pack, buildErr := BuildPack(bundle, proposal)
			if buildErr != nil {
				t.Fatal(buildErr)
			}
			if pack.ReproduceOrObserve[0].Text == text ||
				strings.Contains(pack.ReproduceOrObserve[0].Text, "invented") ||
				strings.Contains(pack.ReproduceOrObserve[0].Text, "restart") {
				t.Fatalf("invented guidance was not reconstructed: %#v", pack.ReproduceOrObserve)
			}
		})
	}
	proposal, err := LocalProposal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	proposal.ReproduceOrObserve[0] = ProposedGuidance{
		Text: "Run `go test ./...` to obtain the missing evidence.", Authority: AuthorityMissing,
	}
	pack, err := BuildPack(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(pack.ReproduceOrObserve[0].Text, "go test") ||
		pack.ReproduceOrObserve[0].Text != localGuidanceText(proposal.ReproduceOrObserve[0], newBundleIndex(bundle)) {
		t.Fatalf("missing-evidence guidance was not reconstructed: %#v", pack.ReproduceOrObserve)
	}
}

func anchorByID(anchors []Anchor, id string) Anchor {
	for _, anchor := range anchors {
		if anchor.ID == id {
			return anchor
		}
	}
	return Anchor{}
}

func hasRelationKind(relations []Relation, kind string) bool {
	for _, relation := range relations {
		if relation.Kind == kind {
			return true
		}
	}
	return false
}

func anchorBySymbol(anchors []Anchor, symbol string) Anchor {
	for _, anchor := range anchors {
		if anchor.Symbol == symbol {
			return anchor
		}
	}
	return Anchor{}
}

func ensureSelectedAnchors(anchors []ProposedAnchor, ids ...string) []ProposedAnchor {
	seen := make(map[string]struct{}, len(anchors))
	for _, anchor := range anchors {
		seen[anchor.AnchorID] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			anchors = append(anchors, ProposedAnchor{
				AnchorID: id, Role: RoleRepresentativeImplementation, Why: "Required to test the retained direct relation.",
			})
		}
	}
	return anchors
}

func newTaskLensTestRepo(t *testing.T, name string) string {
	t.Helper()
	return newTaskLensTestRepoIn(t, filepath.Join(t.TempDir(), name))
}

func newTaskLensTestRepoIn(t *testing.T, repo string) string {
	t.Helper()
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"go.mod": "module example.com/tasklensfixture\n\ngo 1.24\n",
		"config.go": `package fixture

type Config struct { Enabled bool }
type Engine struct { Config Config }

func CopyConfig(engine *Engine, config Config) {
	engine.Config.Enabled = config.Enabled
}

func ReadEnabled(engine *Engine) bool {
	return engine.Config.Enabled
}
`,
		"config_test.go": `package fixture

import "testing"

func TestCopyConfigEnabled(t *testing.T) {
	engine := &Engine{}
	CopyConfig(engine, Config{Enabled: true})
	if !ReadEnabled(engine) { t.Fatal("Enabled was not copied") }
}
`,
		"README.md": "# Fixture\n\nEnabled controls the documented feature.\n",
	}
	for name, content := range files {
		filePath := filepath.Join(repo, name)
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitTest(t, repo, "init", "--quiet")
	runGitTest(t, repo, "config", "user.email", "tasklens@example.com")
	runGitTest(t, repo, "config", "user.name", "Task Lens Test")
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "--quiet", "-m", "fixture")
	return repo
}

func runGitTest(t *testing.T, directory string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
}

func gitTestOutput(t *testing.T, directory string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
