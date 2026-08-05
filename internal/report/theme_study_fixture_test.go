package report

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/themestudy"
)

// themeFixtureSource is a synthetic authorized file used by the theme fixture
// reader. It mirrors the atlasStudyReportFixture saved sources.
type themeFixtureSource struct {
	path   string
	lines  []string
	offset int // line number of lines[0] (1-based)
}

// themeFixtureReader returns a themestudy.SourceReader over the fixture's
// saved sources plus synthetic fallback content for any other openable path.
func themeFixtureReader(data *ReportData) themestudy.SourceReader {
	sources := make(map[string]themeFixtureSource)
	for _, snippet := range data.UserSources {
		var lines []string
		if snippet.Content != "" {
			lines = strings.Split(strings.TrimRight(snippet.Content, "\n"), "\n")
		} else {
			for _, line := range snippet.Lines {
				lines = append(lines, line.Text)
			}
		}
		start := snippet.StartLine
		if start < 1 {
			start = 1
		}
		sources[snippet.Path] = themeFixtureSource{path: snippet.Path, lines: lines, offset: start}
	}
	return func(path string, startLine, endLine int) ([]string, error) {
		source, ok := sources[path]
		if !ok {
			// Synthetic fallback: a deterministic single function body so the
			// pack stays bounded and the read path only needs exact refs.
			return []string{"func " + filepath.Base(path) + "() {", "\t// fixture body", "}"}, nil
		}
		start := startLine - source.offset
		if start < 0 {
			start = 0
		}
		end := endLine - source.offset
		if end >= len(source.lines) {
			end = len(source.lines) - 1
		}
		if start > end {
			return []string{}, nil
		}
		return source.lines[start : end+1], nil
	}
}

// writeThemeStudyAcceptedArtifacts compiles the full Decision 213 theme stage
// chain over the fixture report and writes the eight artifacts into runDir:
// Scout request/result/status, source expansion, Adjudication
// request/result/status, study_themes. It is the provider-free fixture the
// accepted-state report tests bind against.
func writeThemeStudyAcceptedArtifacts(t *testing.T, runDir string, data *ReportData) {
	t.Helper()
	product := compileAtlasStudyFixture(t, data)
	_ = product // the local compile is the seed producer; the wire is compiled below
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err != nil {
		t.Fatalf("BuildAtlasStudyInput: %v", err)
	}

	// f* vocabulary: every openable tracked path once (names-only).
	vocabulary := themestudy.BuildFileVocabulary(data.OpenablePaths, 0, func(path string) bool { return true })

	// a* seed packs: one focused seed per reading target, bound to its
	// canonical route span when it is the sole allowed target of one span.
	targetByID := make(map[string]atlasstudy.ReadingTarget, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		targetByID[target.ID] = target
	}
	spanByTarget := make(map[string]string)
	for _, span := range input.RouteSpans {
		if len(span.AllowedTargetIDs) == 1 {
			spanByTarget[span.AllowedTargetIDs[0]] = span.ID
		}
	}
	reader := themeFixtureReader(data)
	var seedSpecs []themestudy.SeedSpec
	for index, target := range input.ReadingTargets {
		spec := themestudy.SeedSpec{
			Ref:        "a" + string(rune('1'+index)),
			Path:       target.Location.Path,
			Line:       target.Location.Line,
			Symbol:     target.Symbol,
			Provenance: "d211_span_reading_target",
			Kind:       "focused",
		}
		if spanID, ok := spanByTarget[target.ID]; ok {
			spec.CanonicalSpanID = spanID
		}
		seedSpecs = append(seedSpecs, spec)
	}
	packs, err := themestudy.BuildSeedPacks(
		seedSpecs, 0, 0, 0, 0, reader, func(path string) (int, error) { return 40, nil },
	)
	if err != nil {
		t.Fatalf("BuildSeedPacks: %v", err)
	}

	scoutRequest, err := themestudy.CompileScout(
		themestudy.LanguageEnglish, vocabulary, packs,
		themestudy.ScoutContext{RepositoryName: data.RepoName},
		"",
	)
	if err != nil {
		t.Fatalf("CompileScout: %v", err)
	}
	writeThemeArtifact(t, runDir, themestudy.ScoutRequestArtifactFilename, func() []byte {
		encoded, encodeErr := themestudy.EncodeScoutRequest(scoutRequest)
		if encodeErr != nil {
			t.Fatalf("EncodeScoutRequest: %v", encodeErr)
		}
		return encoded
	}())

	mock, err := themestudy.MockScoutResponse(scoutRequest)
	if err != nil {
		t.Fatalf("MockScoutResponse: %v", err)
	}
	candidates, scoutStatus, err := themestudy.ValidateScout(
		mock, scoutRequest.AnchorRefs(), scoutRequest.FileRefs(), scoutRequest.CatalogSHA256,
	)
	if err != nil {
		t.Fatalf("ValidateScout: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("mock Scout response produced zero accepted candidates")
	}
	themestudy.AssignCandidateRefs(candidates)
	scoutResult := themestudy.ScoutResult{
		Version: themestudy.ScoutResultVersion, State: scoutStatus.State,
		PromptVersion: themestudy.ScoutPromptVersion, Language: themestudy.LanguageEnglish,
		CatalogSHA256: scoutRequest.CatalogSHA256, WireSHA256: scoutRequest.WireSHA256,
		Candidates: candidates, Status: scoutStatus,
	}
	writeThemeArtifact(t, runDir, themestudy.ScoutResultArtifactFilename, mustEncodeTheme(t, scoutResult))
	writeThemeArtifact(t, runDir, themestudy.ScoutStatusArtifactFilename, mustEncodeTheme(t, themestudy.ScoutStatusRecord{
		Version: themestudy.ScoutRequestVersion, State: scoutStatus.State,
		PromptVersion: themestudy.ScoutPromptVersion, Language: themestudy.LanguageEnglish,
		CatalogSHA256: scoutRequest.CatalogSHA256, Status: scoutStatus,
	}))

	// Contract D: expand exactly the requested f* refs.
	requested := themestudy.RefsForExpansion(candidates)
	expansion, err := themestudy.ExpandFiles(vocabulary.Files, reader, func(path string) (int, error) { return 40, nil })
	if err != nil {
		t.Fatalf("ExpandFiles: %v", err)
	}
	expansion.Requested = requested
	writeThemeArtifact(t, runDir, themestudy.ExpansionArtifactFilename, mustEncodeTheme(t, expansion))

	// Contract E: compile the Adjudication request from accepted candidates.
	anchors := make(map[string]themestudy.AnchorInfo)
	for _, spec := range seedSpecs {
		anchors[spec.Ref] = themestudy.AnchorInfo{
			Path: spec.Path, Symbol: spec.Symbol, Line: spec.Line,
			CanonicalSpanID: spec.CanonicalSpanID,
		}
	}
	adjRequest, err := themestudy.CompileAdjudication(
		themestudy.LanguageEnglish, candidates, expansion, anchors,
	)
	if err != nil {
		t.Fatalf("CompileAdjudication: %v", err)
	}
	writeThemeArtifact(t, runDir, themestudy.AdjudicationRequestArtifactFilename, mustEncodeTheme(t, adjRequest))

	adjMock, err := themestudy.MockAdjudicationResponse(adjRequest)
	if err != nil {
		t.Fatalf("MockAdjudicationResponse: %v", err)
	}
	adjThemes, adjStatus, err := themestudy.ValidateAdjudication(adjMock, candidatesByRef(candidates))
	if err != nil {
		t.Fatalf("ValidateAdjudication: %v", err)
	}
	if len(adjThemes) == 0 {
		t.Fatal("mock Adjudication response produced zero accepted themes")
	}
	adjResult := themestudy.AdjudicationResult{
		Version: themestudy.AdjudicationResultVersion, State: adjStatus.State,
		PromptVersion: themestudy.AdjudicationPromptVersion, Language: themestudy.LanguageEnglish,
		CatalogSHA256: adjRequest.CatalogSHA256, WireSHA256: adjRequest.WireSHA256,
		Themes: adjThemes, Status: adjStatus,
	}
	writeThemeArtifact(t, runDir, themestudy.AdjudicationResultArtifactFilename, mustEncodeTheme(t, adjResult))
	writeThemeArtifact(t, runDir, themestudy.AdjudicationStatusArtifactFilename, mustEncodeTheme(t, themestudy.AdjudicationStatusRecord{
		Version: themestudy.AdjudicationRequestVersion, State: adjStatus.State,
		PromptVersion: themestudy.AdjudicationPromptVersion, Language: themestudy.LanguageEnglish,
		CatalogSHA256: adjRequest.CatalogSHA256, Status: adjStatus,
	}))

	// Contract F: reduce into the published portfolio.
	reduction, err := themestudy.Reduce(themestudy.ReducerInput{
		Themes: adjThemes, Candidates: candidatesByRef(candidates), Anchors: anchors,
	})
	if err != nil {
		t.Fatalf("Reduce: %v", err)
	}
	if len(reduction.Cards) == 0 {
		t.Fatal("reducer produced zero cards")
	}
	themes := themestudy.StudyThemes{
		Version: "v1", ScoutSHA256: scoutRequest.CatalogSHA256,
		AdjSHA256: adjRequest.CatalogSHA256,
		Cards:     reduction.Cards, Omitted: reduction.Omitted,
		Partial: reduction.Partial, Diagnostics: reduction.Diagnostics,
	}
	writeThemeArtifact(t, runDir, themestudy.StudyThemesArtifactFilename, mustEncodeTheme(t, themes))
}

func mustEncodeTheme(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode theme artifact: %v", err)
	}
	return encoded
}

func writeThemeArtifact(t *testing.T, runDir, name string, data []byte) {
	t.Helper()
	path := filepath.Join(runDir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write theme artifact %s: %v", name, err)
	}
}

func candidatesByRef(candidates []themestudy.ScoutCandidate) map[string]*themestudy.ScoutCandidate {
	byRef := make(map[string]*themestudy.ScoutCandidate, len(candidates))
	for index := range candidates {
		byRef[candidates[index].Ref] = &candidates[index]
	}
	return byRef
}

// themeReadStatus helper reads the accepted theme product for a fixture.
func themeReadStatus(t *testing.T, data *ReportData) (*AtlasStudyReportStatus, error) {
	t.Helper()
	runDir := t.TempDir()
	writeThemeStudyAcceptedArtifacts(t, runDir, data)
	status, studyMap, err := readAtlasStudyReportProduct(runDir, data)
	if err != nil {
		return nil, err
	}
	if studyMap != nil {
		t.Fatalf("theme run must not produce a RepositoryStudyMap")
	}
	return status, nil
}
