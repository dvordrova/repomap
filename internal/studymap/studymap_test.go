package studymap

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/artifactrole"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
	"github.com/dvordrova/repomap/internal/sourcewindowfacts"
)

func TestAnchorSourceUnionPreservesGoAndValidatesExactNonGoSource(t *testing.T) {
	t.Parallel()

	goBundle, _ := studyMapFixture(t)
	goAnchor := goBundle.Anchors[0]
	legacyRaw, err := json.Marshal(struct {
		ID           string                         `json:"id"`
		Path         string                         `json:"path"`
		Symbol       string                         `json:"symbol"`
		Line         int                            `json:"line"`
		Role         artifactrole.Role              `json:"role"`
		Statement    string                         `json:"statement"`
		Capabilities []semanticdiscovery.Capability `json:"capabilities,omitempty"`
		AreaIDs      []string                       `json:"area_ids,omitempty"`
		Function     sourcewindowfacts.Function     `json:"function"`
	}{
		ID: goAnchor.ID, Path: goAnchor.Path, Symbol: goAnchor.Symbol,
		Line: goAnchor.Line, Role: goAnchor.Role, Statement: goAnchor.Statement,
		Capabilities: goAnchor.Capabilities, AreaIDs: goAnchor.AreaIDs, Function: goAnchor.Function,
	})
	if err != nil {
		t.Fatal(err)
	}
	currentRaw, err := json.Marshal(goAnchor)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(currentRaw, legacyRaw) {
		t.Fatalf("Go anchor JSON changed:\n got %s\nwant %s", currentRaw, legacyRaw)
	}
	if _, err := BundleHash(goBundle); err != nil {
		t.Fatalf("existing Go bundle: %v", err)
	}

	valid := exactSourceBundleForTest("src/service.py", "run", 12, []string{"def run() -> None:"})
	hash, err := BundleHash(valid)
	if err != nil || hash == "" {
		t.Fatalf("exact Python bundle hash = %q, error = %v", hash, err)
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBundle(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Anchors[0].ExactSource == nil || decoded.Anchors[0].ExactSource.Language != "python" {
		t.Fatalf("decoded exact source = %#v", decoded.Anchors[0].ExactSource)
	}
	goExact := *valid.Anchors[0].ExactSource
	goExact.Path = "src/service.go"
	goExact.Language = "go"
	if err := goExact.Validate(); err == nil {
		t.Fatal("ExactSource accepted Go data")
	}

	tests := []struct {
		name   string
		mutate func(*Anchor)
	}{
		{name: "neither arm", mutate: func(anchor *Anchor) { anchor.ExactSource = nil }},
		{name: "both arms", mutate: func(anchor *Anchor) {
			anchor.Function = testFunction(t, "src/service.go", "run", 12)
		}},
		{name: "path mismatch", mutate: func(anchor *Anchor) { anchor.ExactSource.Path = "src/other.py" }},
		{name: "symbol mismatch", mutate: func(anchor *Anchor) { anchor.ExactSource.Symbol = "other" }},
		{name: "line mismatch", mutate: func(anchor *Anchor) { anchor.ExactSource.Line++ }},
		{name: "line outside source", mutate: func(anchor *Anchor) {
			anchor.Line = 13
			anchor.ExactSource.Line = 13
		}},
		{name: "malformed source", mutate: func(anchor *Anchor) {
			anchor.ExactSource.Lines[0] = "def run():\n    pass"
		}},
		{name: "hash mismatch", mutate: func(anchor *Anchor) { anchor.ExactSource.ContentSHA256 = strings.Repeat("0", 64) }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			bundle := exactSourceBundleForTest("src/service.py", "run", 12, []string{"def run() -> None:"})
			test.mutate(&bundle.Anchors[0])
			if _, err := BundleHash(bundle); err == nil {
				t.Fatal("BundleHash accepted invalid source union")
			}
		})
	}
}

func TestPromptBundleRetainsOpaqueAnchorsWithoutSourceBodies(t *testing.T) {
	t.Parallel()

	bundle, _ := studyMapFixture(t)
	prompt := bundle.PromptBundle()
	if len(prompt.Anchors) != len(bundle.Anchors) || prompt.Anchors[0].ID == "" {
		t.Fatalf("prompt anchors = %#v", prompt.Anchors)
	}
	raw, err := json.Marshal(prompt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "value := 1") || strings.Contains(string(raw), "content_sha256") ||
		strings.Contains(string(raw), "observations") {
		t.Fatalf("model-visible Study Map bundle contains source-function internals: %s", raw)
	}
}

func TestBuildRecordRetainsOneAreaShapeForSmallLibrary(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	proposal.ShapeAreaIDs = proposal.ShapeAreaIDs[:1]
	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record.ShapeAreaIDs, proposal.ShapeAreaIDs) {
		t.Fatalf("shape areas = %v, want %v", record.ShapeAreaIDs, proposal.ShapeAreaIDs)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestBuildRecordSelectsBoundedDiverseEditorialDirections(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Directions) < MinDirections || len(record.Directions) > MaxDirections {
		t.Fatalf("selected %d directions, want %d-%d", len(record.Directions), MinDirections, MaxDirections)
	}
	if record.Reduction.Proposed != 8 || record.Reduction.Validated != 8 || record.Reduction.Selected != len(record.Directions) {
		t.Fatalf("reduction = %#v", record.Reduction)
	}
	for _, direction := range record.Directions {
		if len(direction.AnchorIDs) != 3 || len(direction.ReadingAnchors) != 3 {
			t.Fatalf("direction %q is not bounded: %#v", direction.Question, direction)
		}
	}
	for left := range record.Directions {
		for right := left + 1; right < len(record.Directions); right++ {
			overlap := stringSetJaccard(record.Directions[left].AnchorIDs, record.Directions[right].AnchorIDs)
			if overlap >= 0.8 || overlap >= 0.6 && record.Directions[left].LearningStage == record.Directions[right].LearningStage {
				t.Fatalf("near-duplicate directions survived selection: %q / %q (anchor overlap %.2f)", record.Directions[left].Question, record.Directions[right].Question, overlap)
			}
		}
	}
	if err := record.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildRecordRejectsUnverifiableCandidatesLocally(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*Bundle, *Candidate)
		wantCode string
	}{
		{
			name: "unknown anchor",
			mutate: func(_ *Bundle, candidate *Candidate) {
				candidate.AnchorIDs[0] = "fact-unknown"
			},
			wantCode: "unknown_anchor_id",
		},
		{
			name: "fixture only",
			mutate: func(bundle *Bundle, candidate *Candidate) {
				selected := make(map[string]struct{}, len(candidate.AnchorIDs))
				for _, anchorID := range candidate.AnchorIDs {
					selected[anchorID] = struct{}{}
				}
				for index := range bundle.Anchors {
					if _, ok := selected[bundle.Anchors[index].ID]; ok {
						bundle.Anchors[index].Role = artifactrole.RoleFixture
					}
				}
			},
			wantCode: "strong_production_anchor_missing",
		},
		{
			name: "runtime order copy",
			mutate: func(_ *Bundle, candidate *Candidate) {
				candidate.LearningOutcome = "The reader sees the execution order before the output is returned."
			},
			wantCode: "learning_outcome_missing",
		},
		{
			name: "Russian runtime order copy",
			mutate: func(_ *Bundle, candidate *Candidate) {
				candidate.LearningOutcome = "Читатель видит порядок выполнения до возврата результата."
			},
			wantCode: "learning_outcome_missing",
		},
		{
			name: "reading anchor outside selection",
			mutate: func(_ *Bundle, candidate *Candidate) {
				candidate.ReadingAnchors[0].AnchorID = "fact-6"
			},
			wantCode: "reading_anchor_not_selected",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			bundle, proposal := studyMapFixture(t)
			test.mutate(&bundle, &proposal.Candidates[0])
			record, err := BuildRecord(bundle, proposal)
			if err != nil {
				t.Fatal(err)
			}
			if len(record.Directions) < MinDirections || !hasReductionIssue(record.Reduction, test.wantCode) {
				t.Fatalf("directions/issues = %d/%#v, want %q", len(record.Directions), record.Reduction.Issues, test.wantCode)
			}
		})
	}
}

func TestBuildRecordAttachesOnlyOverlappingCanonicalMechanism(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	bundle.Mechanisms = []Mechanism{
		{ID: "mechanism-routing", Question: "How does routing work?", Title: "How routing works", AnchorIDs: []string{"fact-1"}, Paths: []string{"internal/part1/work.go"}},
		{ID: "mechanism-output", Question: "How is output written?", Title: "How output is written", AnchorIDs: []string{"fact-6"}, Paths: []string{"internal/part6/work.go"}},
	}
	proposal.Candidates = proposal.Candidates[:4]
	proposal.Candidates[0].MechanismID = "mechanism-routing"
	proposal.Candidates[1].MechanismID = "mechanism-output"
	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Directions) != 4 {
		t.Fatalf("directions = %d", len(record.Directions))
	}
	byQuestion := make(map[string]Direction, len(record.Directions))
	for _, direction := range record.Directions {
		byQuestion[direction.Question] = direction
	}
	if got := byQuestion[proposal.Candidates[0].Question].MechanismID; got != "mechanism-routing" {
		t.Fatalf("overlapping mechanism = %q", got)
	}
	if got := byQuestion[proposal.Candidates[1].Question].MechanismID; got != "" {
		t.Fatalf("non-overlapping mechanism was retained: %q", got)
	}
}

func TestBuildRecordAcceptsOneDirectionAndRecordRejectsEmptySelection(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	proposal.Candidates = proposal.Candidates[:1]
	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Directions) != 1 {
		t.Fatalf("directions = %d", len(record.Directions))
	}
	record.Directions = nil
	if err := record.Validate(); err == nil ||
		!strings.Contains(err.Error(), "invalid canonical selection") {
		t.Fatalf("Record.Validate() error = %v", err)
	}
}

func TestBuildRecordCanonicalizesProviderReadingLabels(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	proposal.Candidates = proposal.Candidates[:1]
	proposal.Candidates[0].ReadingAnchors = append(
		[]ReadingAnchor(nil),
		proposal.Candidates[0].ReadingAnchors...,
	)
	proposal.Candidates[0].ReadingAnchors[0].Label = "С чего начать"
	proposal.Candidates[0].ReadingAnchors[1].Label = "Затем изучите"
	proposal.Candidates[0].ReadingAnchors[2].Label = "Связанная реализация"

	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	got := record.Directions[0].ReadingAnchors
	want := []string{"Start here", "Then inspect", "Related implementation"}
	for index := range want {
		if got[index].Label != want[index] {
			t.Fatalf("reading label %d = %q, want %q", index, got[index].Label, want[index])
		}
	}

	proposal.Candidates[0].ReadingAnchors[0].Label = "Platform-specific"
	if _, err := BuildRecord(bundle, proposal); err == nil {
		t.Fatal("BuildRecord accepted an unknown provider reading label")
	}
}

func TestCanonicalProviderReadingLabelAcceptsOnlyOwnedTranslations(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"Start here":             "Start here",
		"С чего начать":          "Start here",
		"Then inspect":           "Then inspect",
		"Затем изучите":          "Then inspect",
		"Related implementation": "Related implementation",
		"Связанная реализация":   "Related implementation",
		"Public boundary":        "Public boundary",
		"Публичная граница":      "Public boundary",
		"Core data type":         "Core data type",
		"Основной тип данных":    "Core data type",
	}
	for input, want := range tests {
		got, ok := canonicalProviderReadingLabel(input)
		if !ok || got != want {
			t.Fatalf(
				"canonicalProviderReadingLabel(%q) = %q, %t; want %q, true",
				input,
				got,
				ok,
				want,
			)
		}
	}
	for _, input := range []string{
		"start here",
		"START HERE",
		"с чего начать",
		"Core operation",
		"Data transformation",
		"Integration point",
		"Platform-specific",
	} {
		if got, ok := canonicalProviderReadingLabel(input); ok {
			t.Fatalf(
				"canonicalProviderReadingLabel(%q) = %q, true; want rejection",
				input,
				got,
			)
		}
	}
}

func TestRuntimeOrderDetectionCoversEnglishAndRussian(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"Then the system executes the next operation.",
		"The reader sees the execution order.",
		"Затем система выполняет следующую операцию.",
		"Система сначала открывает файл, потом записывает данные.",
		"Читатель видит порядок выполнения.",
		"Это доказанная последовательность операций.",
	} {
		if !impliesRuntimeOrder(value) {
			t.Fatalf("impliesRuntimeOrder(%q) = false", value)
		}
	}
	for _, value := range []string{
		"How does this code preserve the execution order?",
		"Как код сохраняет порядок выполнения?",
		"В каком порядке выполняются вызовы?",
		"Какова последовательность операций в этом коде?",
	} {
		if !asksForRuntimeOrder(value) {
			t.Fatalf("asksForRuntimeOrder(%q) = false", value)
		}
	}
}

func TestDecodeRecordRevalidatesBriefAndCanonicalDirection(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	bundle.Mechanisms = []Mechanism{{
		ID: "mechanism-other", Question: "How does unrelated work happen?",
		Title: "Unrelated work", AnchorIDs: []string{"fact-8"},
		Paths: []string{"internal/part8/work.go"},
	}}
	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("brief support", func(t *testing.T) {
		tampered := record
		tampered.Brief.WhatItIs.SupportIDs = []string{"fact-unknown"}
		raw, marshalErr := json.Marshal(tampered)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, decodeErr := DecodeRecord(raw); decodeErr == nil ||
			!strings.Contains(decodeErr.Error(), "saved repository brief is invalid") {
			t.Fatalf("DecodeRecord() error = %v", decodeErr)
		}
	})

	t.Run("non-overlapping mechanism", func(t *testing.T) {
		tampered := record
		tampered.Directions = append([]Direction(nil), record.Directions...)
		tampered.Directions[0].MechanismID = "mechanism-other"
		raw, marshalErr := json.Marshal(tampered)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, decodeErr := DecodeRecord(raw); decodeErr == nil ||
			!strings.Contains(decodeErr.Error(), "saved direction is not canonical") {
			t.Fatalf("DecodeRecord() error = %v", decodeErr)
		}
	})
}

func TestDecodeRecordRejectsHashTampering(t *testing.T) {
	t.Parallel()

	bundle, proposal := studyMapFixture(t)
	record, err := BuildRecord(bundle, proposal)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), `"repo_name":"fixture"`, `"repo_name":"different"`, 1)
	if _, err := DecodeRecord([]byte(tampered)); err == nil || !strings.Contains(err.Error(), "bundle hash mismatch") {
		t.Fatalf("DecodeRecord(tampered) error = %v", err)
	}
}

func studyMapFixture(t *testing.T) (Bundle, Proposal) {
	t.Helper()

	areas := []Area{
		{ID: "area-api", Name: "API", Responsibility: "Public entry points."},
		{ID: "area-core", Name: "Core", Responsibility: "Central implementation."},
		{ID: "area-output", Name: "Output", Responsibility: "Visible results."},
		{ID: "area-extension", Name: "Extensions", Responsibility: "Integration surface."},
	}
	anchors := make([]Anchor, 0, 8)
	allowed := []string{"README.md"}
	for index := 0; index < 8; index++ {
		filePath := fmt.Sprintf("internal/part%d/work.go", index+1)
		symbol := fmt.Sprintf("work%d", index+1)
		function := testFunction(t, filePath, symbol, 10+index*20)
		anchors = append(anchors, Anchor{
			ID: fmt.Sprintf("fact-%d", index+1), Path: filePath, Symbol: symbol,
			Line: function.StartLine + 1, Role: artifactrole.RoleProductionCore,
			Statement: symbol + " provides a bounded implementation anchor.",
			AreaIDs:   []string{areas[index%len(areas)].ID}, Function: function,
		})
		allowed = append(allowed, filePath)
	}
	bundle := Bundle{
		Version: BundleVersion, RepoName: "fixture",
		DocumentedPurpose:  "Fixture demonstrates a source-backed repository reading guide.",
		RepositoryTypeHint: RepositoryLibrary,
		Areas:              areas, Anchors: anchors,
		Documents:    []Document{{ID: "doc-readme", Path: "README.md", Label: "README", Excerpt: "Fixture overview."}},
		AllowedPaths: allowed,
	}
	questions := []string{
		"How should callers enter this library?",
		"How does the core process work?",
		"Where does the visible output originate?",
		"How are extensions connected to core code?",
		"How can maintainers inspect internal state?",
		"Where is repository configuration interpreted?",
		"How should contributors approach the implementation?",
		"Which public types define the domain model?",
	}
	stages := []LearningStage{
		StageOrientation, StageCentralOperation, StageCoreModel, StageIntegration,
		StageOperations, StageOperations, StageContribution, StageCoreModel,
	}
	jobs := []TargetJob{
		JobFirstContact, JobUseOperate, JobMaintain, JobIntegrate,
		JobMaintain, JobUseOperate, JobContribute, JobIntegrate,
	}
	proposal := Proposal{
		Version: ProposalVersion, RepositoryType: RepositoryLibrary,
		Brief: Brief{
			WhatItIs:              BriefStatement{Text: "Fixture is a source-backed library.", SupportIDs: []string{"doc-readme"}},
			Problem:               BriefStatement{Text: "It demonstrates bounded repository orientation.", SupportIDs: []string{"doc-readme"}},
			MainInput:             BriefStatement{Text: "A public library call.", SupportIDs: []string{"fact-1"}},
			CentralResponsibility: BriefStatement{Text: "It performs central work.", SupportIDs: []string{"fact-2"}},
			ObservableResult:      BriefStatement{Text: "It exposes a visible result.", SupportIDs: []string{"fact-3"}},
		},
		ShapeAreaIDs: []string{"area-api", "area-core", "area-output", "area-extension"},
	}
	for index, question := range questions {
		ids := []string{
			anchors[index%len(anchors)].ID,
			anchors[(index+1)%len(anchors)].ID,
			anchors[(index+2)%len(anchors)].ID,
		}
		reading := []ReadingAnchor{
			{AnchorID: ids[0], Label: "Start here", WhatToLookFor: "Inspect the public or central declaration."},
			{AnchorID: ids[1], Label: "Then inspect", WhatToLookFor: "Compare the related bounded implementation."},
			{AnchorID: ids[2], Label: "Related implementation", WhatToLookFor: "Inspect the nearby data and calls."},
		}
		proposal.Candidates = append(proposal.Candidates, Candidate{
			Question: question, WhyItMatters: "This question identifies a useful repository responsibility.",
			LearningOutcome: "The reader can name the relevant code and responsibility.",
			TargetJob:       jobs[index], LearningStage: stages[index], AnchorIDs: ids,
			DocumentIDs: []string{"doc-readme"}, AreaIDs: []string{areas[index%len(areas)].ID},
			ReadingAnchors: reading, Confidence: "high", SearchQueries: []string{question},
		})
	}
	return bundle, proposal
}

func testFunction(t *testing.T, filePath, symbol string, startLine int) sourcewindowfacts.Function {
	t.Helper()
	window, err := sourcewindowfacts.NewWindow("window-"+symbol, filePath, startLine, []string{
		"func " + symbol + "() int {",
		"\tvalue := 1",
		"\treturn value",
		"}",
	})
	if err != nil {
		t.Fatal(err)
	}
	function, err := sourcewindowfacts.ExtractGoFunction(window, symbol)
	if err != nil {
		t.Fatal(err)
	}
	return function
}

func exactSourceBundleForTest(
	filePath string,
	symbol string,
	line int,
	lines []string,
) Bundle {
	raw, _ := json.Marshal(lines)
	digest := sha256.Sum256(raw)
	exact := &ExactSource{
		Path: filePath, Language: "python", Symbol: symbol, Line: line,
		StartLine: line, EndLine: line + len(lines) - 1,
		Lines: append([]string(nil), lines...), ContentSHA256: hex.EncodeToString(digest[:]),
	}
	return Bundle{
		Version: BundleVersion, RepoName: "exact-source-fixture",
		Anchors: []Anchor{{
			ID: "fact-exact", Path: filePath, Symbol: symbol, Line: line,
			Role: artifactrole.RoleProductionCore, Statement: "An exact declaration is available for study.",
			ExactSource: exact,
		}},
		AllowedPaths: []string{filePath},
	}
}

func hasReductionIssue(reduction Reduction, code string) bool {
	for _, issue := range reduction.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
