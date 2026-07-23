package tasklens

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGoAnchorRetainsCompleteFunctionWithinByteBudget(t *testing.T) {
	t.Parallel()

	var source strings.Builder
	source.WriteString("package fixture\n\n")
	source.WriteString("func TransformWidget(input int) int {\n")
	source.WriteString("\ttotal := input\n")
	for range 48 {
		source.WriteString("\ttotal++\n")
	}
	source.WriteString("\treturn total\n")
	source.WriteString("}\n")

	file := completeCollectedFile("transform.go", source.String())
	anchors := goAnchors(file, extractTerms("TransformWidget must transform the widget value."))
	anchor := anchorCandidateBySymbol(anchors, "TransformWidget")
	if anchor.anchor.ID == "" {
		t.Fatalf("TransformWidget anchor was not generated: %#v", anchors)
	}
	if len(anchor.anchor.Excerpt) <= 36 {
		t.Fatalf("complete function regressed to a fixed short window: %d lines", len(anchor.anchor.Excerpt))
	}
	if anchor.anchor.StartLine != 3 || anchor.anchor.EndLine != len(file.lines) {
		t.Fatalf(
			"function bounds = %d-%d, want 3-%d",
			anchor.anchor.StartLine,
			anchor.anchor.EndLine,
			len(file.lines),
		)
	}
	if anchor.anchor.Scope.ScopeKind != SourceScopeCompleteEnclosingSymbol ||
		anchor.anchor.Scope.Truncated ||
		anchor.anchor.Scope.SourceTotalLines != len(file.lines) ||
		!anchor.anchor.Scope.NegativeClaimsAllowed {
		t.Fatalf("complete function scope = %#v", anchor.anchor.Scope)
	}
	if err := anchor.anchor.Scope.Validate(); err != nil {
		t.Fatalf("complete function scope Validate() error = %v", err)
	}
}

func TestGoAnchorRetainsAllDistantTaskStatementsAndNeighborhoods(t *testing.T) {
	t.Parallel()

	var source strings.Builder
	source.WriteString("package fixture\n\n")
	source.WriteString("func HugeTransform(input int) int {\n")
	source.WriteString("\ttotal := input\n")
	source.WriteString("\tfirst := MatchToken(total)\n")
	for range 3200 {
		source.WriteString("\ttotal += 1\n")
	}
	source.WriteString("\tif MatchToken(first) {\n")
	source.WriteString("\t\ttotal = ApplyMatch(total)\n")
	source.WriteString("\t}\n")
	source.WriteString("\treturn total\n")
	source.WriteString("}\n")

	file := completeCollectedFile("transform.go", source.String())
	if sourceRangeBytes(file.lines, 3, len(file.lines)) <= maxCompleteSymbolBytes {
		t.Fatalf("fixture must exceed the complete-symbol bound")
	}
	terms := extractTerms("Trace `MatchToken` assignments and branch behavior.")
	anchor := anchorCandidateBySymbol(goAnchors(file, terms), "HugeTransform").anchor
	if anchor.ID == "" {
		t.Fatal("HugeTransform fragment anchor was not generated")
	}
	if anchor.Scope.ScopeKind != SourceScopeMatchedFragments ||
		!anchor.Scope.Truncated || anchor.Scope.TaskMatchesOutsideWindow ||
		anchor.Scope.NegativeClaimsAllowed ||
		anchor.Scope.TruncationReason != "oversized_symbol_fragment_retention" {
		t.Fatalf("oversized function scope = %#v", anchor.Scope)
	}
	if err := anchor.Scope.Validate(); err != nil {
		t.Fatalf("fragment scope Validate() error = %v", err)
	}
	if err := anchor.Scope.ValidateClaim(true); err == nil {
		t.Fatal("matched fragments unexpectedly authorized an absence claim")
	}

	retained := retainedSourceLineNumbers(anchor.Excerpt)
	for _, line := range taskMatchingLines(file.lines, 3, len(file.lines), terms) {
		if _, exists := retained[line]; !exists {
			t.Fatalf("task-matching statement line %d was omitted", line)
		}
	}
	if !excerptContains(anchor.Excerpt, "total = ApplyMatch(total)") {
		t.Fatal("assignment/branch neighborhood around the distant match was omitted")
	}
	if !sourceLinesHaveGap(anchor.Excerpt) {
		t.Fatal("adversarial fixture did not exercise distant matched fragments")
	}
}

func TestGoAnchorDowngradesFragmentsWhenTaskMatchesExceedByteCap(t *testing.T) {
	t.Parallel()

	var source strings.Builder
	source.WriteString("package fixture\n\n")
	source.WriteString("func HugeTransform(input int) int {\n")
	for range 2400 {
		source.WriteString("\tinput += MatchToken(input)\n")
	}
	source.WriteString("\treturn input\n")
	source.WriteString("}\n")

	file := completeCollectedFile("transform.go", source.String())
	terms := extractTerms("Inspect `MatchToken`.")
	anchor := anchorCandidateBySymbol(goAnchors(file, terms), "HugeTransform").anchor
	if anchor.ID == "" {
		t.Fatal("HugeTransform partial anchor was not generated")
	}
	if anchor.Scope.ScopeKind != SourceScopePartialWindow ||
		!anchor.Scope.TaskMatchesOutsideWindow || anchor.Scope.NegativeClaimsAllowed ||
		anchor.Scope.TruncationReason != "per_anchor_fragment_byte_or_line_limit" {
		t.Fatalf("capped function scope = %#v", anchor.Scope)
	}
	if anchorExcerptBytes(anchor) > maxCompleteSymbolBytes {
		t.Fatalf("fragment bytes = %d, bound = %d", anchorExcerptBytes(anchor), maxCompleteSymbolBytes)
	}
	if err := anchor.Scope.ValidateClaim(true); err == nil {
		t.Fatal("capped fragments unexpectedly authorized an absence claim")
	}
}

func TestOversizedOperationalAndDocumentFilesRetainDistantStructuralFragments(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		path       string
		source     string
		structures []string
	}{
		{
			name: "shell functions",
			path: "scripts/release.sh",
			source: "#!/usr/bin/env bash\n" +
				"preamble() {\n" + strings.Repeat("  # bounded filler\n", 4300) + "}\n" +
				"first_stage() {\n  printf 'release_marker first'\n}\n" +
				"padding() {\n" + strings.Repeat("  # distant filler\n", 4300) + "}\n" +
				"second_stage() {\n  printf 'release_marker second'\n}\n",
			structures: []string{"first_stage() {", "second_stage() {"},
		},
		{
			name: "make targets",
			path: "Makefile",
			source: "padding-a:\n" + strings.Repeat("\t@true # bounded filler\n", 3300) +
				"first-stage:\n\t@echo release_marker-first\n" +
				"padding-b:\n" + strings.Repeat("\t@true # distant filler\n", 3300) +
				"second-stage:\n\t@echo release_marker-second\n",
			structures: []string{"first-stage:", "second-stage:"},
		},
		{
			name: "document headings",
			path: "README.md",
			source: "# Padding A\n" + strings.Repeat("bounded filler\n", 4400) +
				"# First stage\nrelease_marker first\n" +
				"# Padding B\n" + strings.Repeat("distant filler\n", 4400) +
				"# Second stage\nrelease_marker second\n",
			structures: []string{"# First stage", "# Second stage"},
		},
		{
			name: "config sections",
			path: "config.yaml",
			source: "padding_a:\n" + strings.Repeat("  filler: bounded\n", 3800) +
				"first_stage:\n  value: release_marker-first\n" +
				"padding_b:\n" + strings.Repeat("  filler: distant\n", 3800) +
				"second_stage:\n  value: release_marker-second\n",
			structures: []string{"first_stage:", "second_stage:"},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			file := completeCollectedFile(test.path, test.source)
			if sourceRangeBytes(file.lines, 1, len(file.lines)) <= maxCompleteFileBytes {
				t.Fatalf("fixture must exceed the complete-file bound")
			}
			terms := extractTerms("Inspect `release_marker`.")
			anchors := lineAnchors(file, terms)
			if len(anchors) != 1 {
				t.Fatalf("fragment anchor count = %d, want 1", len(anchors))
			}
			anchor := anchors[0].anchor
			if anchor.Scope.ScopeKind != SourceScopeMatchedFragments ||
				!anchor.Scope.Truncated || anchor.Scope.TaskMatchesOutsideWindow ||
				anchor.Scope.NegativeClaimsAllowed {
				t.Fatalf("oversized file scope = %#v", anchor.Scope)
			}
			retained := retainedSourceLineNumbers(anchor.Excerpt)
			for _, line := range taskMatchingLines(file.lines, 1, len(file.lines), terms) {
				if _, exists := retained[line]; !exists {
					t.Fatalf("task-matching section line %d was omitted", line)
				}
			}
			for _, structure := range test.structures {
				if !excerptContains(anchor.Excerpt, structure) {
					t.Fatalf("structural fragment %q was omitted", structure)
				}
			}
			if !sourceLinesHaveGap(anchor.Excerpt) {
				t.Fatal("adversarial fixture did not exercise distant structural fragments")
			}
			if err := anchor.Scope.ValidateClaim(true); err == nil {
				t.Fatal("matched file fragments unexpectedly authorized an absence claim")
			}
		})
	}
}

func TestOperationalAnchorRetainsCompleteSmallFileWithLateMatch(t *testing.T) {
	t.Parallel()

	source := `#!/usr/bin/env bash
set -euo pipefail

root_dir="$(pwd)"
module="${1:-core}"
version="${2:-v0.0.0}"

prepare_workspace() {
  cd "$root_dir"
  printf '%s\n' "$module"
}

prepare_workspace

# The decisive safety check is intentionally late in this small file.
if git rev-parse "$version" >/dev/null 2>&1; then
  printf 'release tag already exists: %s\n' "$version"
  exit 1
fi

printf 'release tag is available: %s\n' "$version"
`
	file := completeCollectedFile("scripts/release.sh", source)
	anchors := lineAnchors(file, extractTerms("The release script must detect an existing tag before release."))
	if len(anchors) != 1 {
		t.Fatalf("len(lineAnchors) = %d, want 1 complete-file anchor", len(anchors))
	}
	anchor := anchors[0].anchor
	if anchor.StartLine != 1 || anchor.EndLine != len(file.lines) ||
		len(anchor.Excerpt) != len(file.lines) {
		t.Fatalf("operational file was clipped: anchor=%d-%d total=%d", anchor.StartLine, anchor.EndLine, len(file.lines))
	}
	if anchor.Scope.ScopeKind != SourceScopeCompleteFile || anchor.Scope.Truncated ||
		anchor.Scope.SourceTotalLines != len(file.lines) ||
		anchor.Scope.TaskMatchesOutsideWindow || !anchor.Scope.NegativeClaimsAllowed {
		t.Fatalf("complete operational scope = %#v", anchor.Scope)
	}
	lateMatchRetained := false
	for _, line := range anchor.Excerpt {
		if line.Line > len(file.lines)/2 && strings.Contains(line.Text, "release tag is available") {
			lateMatchRetained = true
			break
		}
	}
	if !lateMatchRetained {
		t.Fatal("late task match was not retained")
	}
}

func TestCollectRetainsLateMatchBeyondInitialFilePrefix(t *testing.T) {
	repo := newTaskLensTestRepo(t, "late-operational-match")
	scriptPath := filepath.Join(repo, "scripts", "release.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "#!/usr/bin/env bash\n" + strings.Repeat("# bounded filler\n", 5000) +
		"late_safety_guard() { printf 'tag conflict'; }\n"
	if len(source) <= maxCollectedFileBytes {
		t.Fatalf("fixture is only %d bytes", len(source))
	}
	if err := os.WriteFile(scriptPath, []byte(source), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "scripts/release.sh")
	runGitTest(t, repo, "commit", "--quiet", "-m", "add long release script")

	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "Inspect the release script late_safety_guard before creating a tag.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Budgets.SourceScanBytes <= maxCollectedFileBytes || bundle.Budgets.SourceScanLimitBound {
		t.Fatalf("source scan budget = %#v", bundle.Budgets)
	}
	for _, anchor := range bundle.Anchors {
		if anchor.Path != "scripts/release.sh" {
			continue
		}
		for _, line := range anchor.Excerpt {
			if strings.Contains(line.Text, "late_safety_guard") {
				if line.Line <= 4000 || anchor.Scope.SourceTotalLines == 0 {
					t.Fatalf("late anchor scope = %#v", anchor.Scope)
				}
				return
			}
		}
	}
	t.Fatalf("late script match was not retained: %#v", bundle.Anchors)
}

func TestCollectVerifiesDiscontiguousMatchedFragments(t *testing.T) {
	repo := newTaskLensTestRepo(t, "verified-fragments")
	scriptPath := filepath.Join(repo, "scripts", "release.sh")
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "#!/usr/bin/env bash\n" +
		"padding_a() {\n" + strings.Repeat("  # bounded filler\n", 4000) + "}\n" +
		"first_stage() {\n  printf 'unique_release_marker first'\n}\n" +
		"padding_b() {\n" + strings.Repeat("  # distant filler\n", 4000) + "}\n" +
		"second_stage() {\n  printf 'unique_release_marker second'\n}\n"
	if err := os.WriteFile(scriptPath, []byte(source), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, repo, "add", "scripts/release.sh")
	runGitTest(t, repo, "commit", "--quiet", "-m", "add fragmented release script")

	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "Inspect `unique_release_marker` in the release script.",
	})
	if err != nil {
		t.Fatal(err)
	}
	anchor := Anchor{}
	for _, candidate := range bundle.Anchors {
		if candidate.Path == "scripts/release.sh" &&
			candidate.Scope.ScopeKind == SourceScopeMatchedFragments {
			anchor = candidate
			break
		}
	}
	if anchor.ID == "" || !sourceLinesHaveGap(anchor.Excerpt) {
		t.Fatalf("discontiguous fragment anchor was not retained: %#v", bundle.Anchors)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("bundle Validate() error = %v", err)
	}
	if err := VerifyBundleSources(repo, bundle); err != nil {
		t.Fatalf("VerifyBundleSources() error = %v", err)
	}
}

func TestPartialAnchorRejectsAbsenceClaimDuringReduction(t *testing.T) {
	t.Parallel()

	const (
		anchorID   = "anchor-partial"
		evidenceID = "evidence-partial"
	)
	index := bundleIndex{
		anchors: map[string]Anchor{
			anchorID: {
				ID:     anchorID,
				Path:   "scripts/release.sh",
				Symbol: "release.sh:40",
				Scope: SourceScope{
					ScopeKind:                SourceScopePartialWindow,
					ScopeStart:               32,
					ScopeEnd:                 48,
					SourceTotalLines:         0,
					Truncated:                true,
					TruncationReason:         "file_read_byte_limit",
					TaskMatchesOutsideWindow: true,
					NegativeClaimsAllowed:    false,
					NegativeEvidenceBasis:    NegativeEvidenceNone,
				},
			},
		},
		evidence: map[string]Evidence{
			evidenceID: {
				ID:       evidenceID,
				Kind:     EvidenceRepositoryFact,
				AnchorID: anchorID,
			},
		},
		paths: map[string]struct{}{"scripts/release.sh": {}},
	}
	selected := map[string]struct{}{anchorID: {}}
	_, err := buildClause(ProposedClause{
		Status:     HypothesisSupported,
		Text:       "The release script lacks a tag conflict check.",
		SupportIDs: []string{evidenceID},
	}, selected, index)
	if err == nil || !strings.Contains(err.Error(), "absence claim exceeds retained source scope") {
		t.Fatalf("partial-source absence claim error = %v", err)
	}
}

func TestCompletionPassStopsAfterTwoMissingKeyRoles(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileErrorStatusMapping)
	if err != nil {
		t.Fatal(err)
	}
	candidates := []anchorCandidate{
		{
			anchor: Anchor{ID: "anchor-symptom", RoleHints: []AnchorRole{RoleSymptomSite}},
		},
		{
			anchor: Anchor{ID: "anchor-created", RoleHints: []AnchorRole{RoleErrorCreation}},
		},
		{
			anchor: Anchor{ID: "anchor-mapped", RoleHints: []AnchorRole{RoleErrorMapping}},
		},
	}
	anchors, expansions := completeMissingRoleAnchors(nil, candidates, nil, contract)
	if expansions != MaxFrontierExpansions {
		t.Fatalf("completion expansions = %d, want %d", expansions, MaxFrontierExpansions)
	}
	if len(anchors) != MaxFrontierExpansions {
		t.Fatalf("retained completion anchors = %d, want %d", len(anchors), MaxFrontierExpansions)
	}
	coverage, err := EvaluateRoleCoverage(contract, anchors)
	if err != nil {
		t.Fatal(err)
	}
	missing := coverage.MissingKeyRoles()
	if len(missing) != 1 || missing[0] != RoleSymptomSite {
		t.Fatalf("missing roles after bounded completion = %v, want [%s]", missing, RoleSymptomSite)
	}
}

func TestCompletionReplacesFillerAtFullAnchorBudget(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileErrorStatusMapping)
	if err != nil {
		t.Fatal(err)
	}
	anchors := []Anchor{
		{ID: "creation", Path: "errors.go", Symbol: "StatusError", RoleHints: []AnchorRole{RoleErrorCreation}},
		{ID: "mapping", Path: "serialize.go", Symbol: "MapError", RoleHints: []AnchorRole{RoleErrorMapping}},
	}
	for index := len(anchors); index < MaxRetainedAnchors; index++ {
		anchors = append(anchors, Anchor{
			ID:     OpaqueID("anchor", "filler", string(rune('a'+index))),
			Path:   filepath.Join("fillers", string(rune('a'+index))+".go"),
			Symbol: "Filler",
		})
	}
	candidates := []anchorCandidate{{
		anchor: Anchor{
			ID:        "symptom",
			Path:      "response.go",
			Symbol:    "Send",
			RoleHints: []AnchorRole{RoleSymptomSite},
		},
		score: 100,
	}}

	completed, expansions := completeMissingRoleAnchors(anchors, candidates, nil, contract)
	if expansions != 1 || len(completed) != MaxRetainedAnchors || !containsAnchorID(completed, "symptom") {
		t.Fatalf("completion = expansions:%d anchors:%d symptom:%t", expansions, len(completed), containsAnchorID(completed, "symptom"))
	}
	if candidates[0].stage != RetrievalStageCompletion1 {
		t.Fatalf("completion stage = %q, want %q", candidates[0].stage, RetrievalStageCompletion1)
	}
	coverage, err := EvaluateRoleCoverage(contract, completed)
	if err != nil {
		t.Fatal(err)
	}
	if missing := coverage.MissingKeyRoles(); len(missing) != 0 {
		t.Fatalf("missing key roles after replacement = %v", missing)
	}
}

func TestCompletionRecomputesCoverageAfterMultiRoleCandidate(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileErrorStatusMapping)
	if err != nil {
		t.Fatal(err)
	}
	anchors := []Anchor{{
		ID: "mapping", Path: "serialize.go", Symbol: "MapError", RoleHints: []AnchorRole{RoleErrorMapping},
	}}
	candidates := []anchorCandidate{{
		anchor: Anchor{
			ID: "handoff", Path: "response.go", Symbol: "SendError",
			RoleHints: []AnchorRole{RoleSymptomSite, RoleErrorCreation},
		},
		score: 100,
	}}

	completed, expansions := completeMissingRoleAnchors(anchors, candidates, nil, contract)
	if expansions != 1 {
		t.Fatalf("completion expansions = %d, want 1", expansions)
	}
	coverage, err := EvaluateRoleCoverage(contract, completed)
	if err != nil {
		t.Fatal(err)
	}
	if missing := coverage.MissingKeyRoles(); len(missing) != 0 {
		t.Fatalf("multi-role completion left missing roles: %v", missing)
	}
}

func TestCompletionRetainsExactParallelTransformationBranch(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileDataTagTransformation)
	if err != nil {
		t.Fatal(err)
	}
	entry := semanticAnchor("entry", "pkg/transform.go", "Register", RolePublicOrCLIEntry)
	entry.Package = "fixture"
	entry.Score = 100
	entry.Excerpt = []SourceLine{{
		Line: 1,
		Text: "func Register() { Weak(); ParseScalar(); ParseCollection() }",
	}}
	weak := semanticAnchor("weak", "pkg/transform.go", "Weak", RoleTransformation)
	weak.Package = "fixture"
	weak.Score = 1
	weak.Excerpt = []SourceLine{{Line: 2, Text: "func Weak() {}"}}
	output := semanticAnchor("output", "pkg/testdata/generated.json", "generated", RoleGeneratedOutput)
	scalar := semanticAnchor("scalar", "pkg/helpers.go", "ParseScalar", RoleTransformation)
	scalar.Package = "fixture"
	scalar.Score = 200
	scalar.Excerpt = []SourceLine{{Line: 3, Text: "func ParseScalar() {}"}}
	collection := semanticAnchor("collection", "pkg/helpers.go", "ParseCollection", RoleTransformation)
	collection.Package = "fixture"
	collection.Score = 150
	collection.Excerpt = []SourceLine{{Line: 4, Text: "func ParseCollection() {}"}}
	candidates := []anchorCandidate{
		{anchor: scalar, score: scalar.Score},
		{anchor: collection, score: collection.Score},
	}

	completed, expansions := completeMissingRoleAnchors(
		[]Anchor{entry, weak, output},
		candidates,
		nil,
		contract,
	)
	if expansions != MaxFrontierExpansions {
		t.Fatalf("completion expansions = %d, want %d", expansions, MaxFrontierExpansions)
	}
	for _, id := range []string{scalar.ID, collection.ID} {
		if !containsAnchorID(completed, id) {
			t.Fatalf("parallel transformation branch %q was not retained: %#v", id, completed)
		}
	}
	if candidates[0].stage != RetrievalStageCompletion1 ||
		candidates[1].stage != RetrievalStageCompletion2 {
		t.Fatalf("completion stages = %q, %q", candidates[0].stage, candidates[1].stage)
	}
}

func TestVerificationProbeAddsExactDecisiveTest(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileDataTagTransformation)
	if err != nil {
		t.Fatal(err)
	}
	entry := semanticAnchor("entry", "pkg/entry.go", "Register", RolePublicOrCLIEntry)
	entry.Package = "fixture"
	entry.Excerpt = []SourceLine{{Line: 1, Text: "func Register() { Transform() }"}}
	transform := semanticAnchor("transform", "pkg/transform.go", "Transform", RoleTransformation)
	transform.Package = "fixture"
	transform.Excerpt = []SourceLine{{Line: 1, Text: "func Transform() {}"}}
	output := semanticAnchor("output", "pkg/generated.json", "generated", RoleGeneratedOutput)
	anchors := []Anchor{entry, transform, output}
	exactTest := semanticAnchor("exact-test", "pkg/transform_test.go", "TestTransform", RoleVerificationAnchor)
	exactTest.Package = "fixture"
	exactTest.Excerpt = []SourceLine{{Line: 1, Text: "func TestTransform(t *testing.T) { Transform() }"}}
	candidates := []anchorCandidate{{anchor: exactTest, score: 100}}

	completed := completeVerificationAnchor(anchors, candidates, nil, contract)
	if !containsAnchorID(completed, exactTest.ID) {
		t.Fatalf("verification probe did not retain exact test: %#v", completed)
	}
	if candidates[0].stage != RetrievalStageVerification {
		t.Fatalf("verification stage = %q, want %q", candidates[0].stage, RetrievalStageVerification)
	}
}

func TestTraceUsesRecordedStageAndExactCandidateScore(t *testing.T) {
	t.Parallel()

	contract, err := DefaultRoleContract(TaskProfileNilPanic)
	if err != nil {
		t.Fatal(err)
	}
	anchor := Anchor{
		ID: "candidate", Path: "validation.go", Symbol: "validate",
		RoleHints: []AnchorRole{RoleUnsafeOperation}, Score: 37,
	}
	candidate := anchorCandidate{
		anchor: anchor,
		score:  37,
		terms:  []string{"term-validation"},
		stage:  RetrievalStageCompletion1,
	}
	bundle := Bundle{
		Profile:      TaskProfileNilPanic,
		RoleContract: contract,
		Anchors:      []Anchor{anchor},
	}

	trace := buildRetrievalTrace(bundle, []anchorCandidate{candidate})
	if len(trace.CandidatesBeforeRanking) != 1 || len(trace.SelectedAnchors) != 1 {
		t.Fatalf("trace candidate projection = %#v", trace)
	}
	got := trace.CandidatesBeforeRanking[0]
	if got.Stage != RetrievalStageCompletion1 || got.Score != candidate.score {
		t.Fatalf("trace candidate stage/score = %q/%d, want %q/%d", got.Stage, got.Score, RetrievalStageCompletion1, candidate.score)
	}
	sum := 0
	for _, component := range got.ScoreComponents {
		sum += component.Value
	}
	if sum != got.Score {
		t.Fatalf("trace component sum = %d, want score %d", sum, got.Score)
	}
	if !strings.Contains(trace.SelectedAnchors[0].Reason, "completion expansion 1") {
		t.Fatalf("trace selection reason = %q", trace.SelectedAnchors[0].Reason)
	}
}

func TestZeroCallBundleReplayIsDeterministicAndTamperStrict(t *testing.T) {
	// Collect creates an isolated git fixture; do not run this test in parallel
	// with process-global git environment mutation tests.
	repo := newTaskLensTestRepo(t, "zero-call-replay")
	bundle, err := Collect(context.Background(), CollectOptions{
		RepositoryPath: repo,
		TaskText:       "The Enabled configuration is ignored; inspect CopyConfig and TestCopyConfigEnabled.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bundle.CheapExit.Eligible || bundle.CheapExit.Route != CheapExitRouteZeroCall {
		t.Fatalf("fixture did not qualify for deterministic zero-call: %#v", bundle.CheapExit)
	}

	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	var replay Bundle
	if err := json.Unmarshal(raw, &replay); err != nil {
		t.Fatal(err)
	}
	if err := replay.Validate(); err != nil {
		t.Fatalf("replayed zero-call bundle Validate() error = %v", err)
	}
	if !reflect.DeepEqual(replay.CheapExit, bundle.CheapExit) {
		t.Fatalf("replayed cheap-exit decision changed:\n got %#v\nwant %#v", replay.CheapExit, bundle.CheapExit)
	}

	replay.CheapExit.Gates[0].Passed = false
	replay.CheapExit.Gates[0].Reason = "tampered but plausible local decision"
	replay.CheapExit.Eligible = false
	replay.CheapExit.Route = CheapExitRouteSynthesisCall
	replay.CheapExit.Reasons = []string{"tampered but plausible local decision"}
	if err := replay.Validate(); err == nil ||
		!strings.Contains(err.Error(), "cheap-exit decision does not match deterministic gates") {
		t.Fatalf("tampered replay error = %v", err)
	}
}

func completeCollectedFile(filePath, source string) collectedFile {
	content := []byte(source)
	lines := splitSourceLines(content)
	return collectedFile{
		candidate: candidate{
			path:      filePath,
			termHits:  map[string]struct{}{},
			grepLines: []int{},
		},
		content:          content,
		lines:            lines,
		sourceTotalLines: len(lines),
	}
}

func anchorCandidateBySymbol(candidates []anchorCandidate, symbol string) anchorCandidate {
	for _, candidate := range candidates {
		if candidate.anchor.Symbol == symbol {
			return candidate
		}
	}
	return anchorCandidate{}
}

func retainedSourceLineNumbers(lines []SourceLine) map[int]struct{} {
	result := make(map[int]struct{}, len(lines))
	for _, line := range lines {
		result[line.Line] = struct{}{}
	}
	return result
}

func excerptContains(lines []SourceLine, text string) bool {
	for _, line := range lines {
		if strings.Contains(line.Text, text) {
			return true
		}
	}
	return false
}

func sourceLinesHaveGap(lines []SourceLine) bool {
	for index := 1; index < len(lines); index++ {
		if lines[index].Line > lines[index-1].Line+1 {
			return true
		}
	}
	return false
}
