package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/debugdump"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/themestudy"
)

const (
	atlasStudyD209ReplayRequestFixtureSHA  = "cf42141cda77aabf3db5ab3ae6ba0023bee87218c83af63b011c36ccfdab0563"
	atlasStudyD209ReplayResponseFixtureSHA = "abdeb1c0738bb2fe0457d5e8d3662bcd05b3e1c6beccfd1283cb74102f0655eb"
)

type atlasStudyReplayFixture struct {
	runDir       string
	responsePath string
	requestSHA   string
	responseSHA  string
}

// writeAtlasStudyResponseReplayFixture writes a copied theme Study run with a
// canonical Scout request artifact and a deterministic mock Scout response
// beside it, so the provider-free replay seam can be exercised exactly.
func writeAtlasStudyResponseReplayFixture(t *testing.T, dirName, metadataRunID string) atlasStudyReplayFixture {
	t.Helper()
	parent := t.TempDir()
	runDir := filepath.Join(parent, dirName)
	writer, err := debugdump.NewWriter(parent, dirName, false)
	if err != nil {
		t.Fatalf("create test run: %v", err)
	}
	if err := writer.WriteMetadata(debugdump.RunMeta{
		RunID:   metadataRunID,
		Command: "atlas-first",
	}); err != nil {
		writer.Close()
		t.Fatalf("write metadata: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close test writer: %v", err)
	}

	input := atlasStudyRuntimeInput()
	product, err := atlasstudy.Compile(input)
	if err != nil {
		t.Fatalf("compile substrate: %v", err)
	}
	paths := themeFixtureOpenablePaths(input)
	vocabulary := themestudy.BuildFileVocabulary(paths, 0, func(string) bool { return true })
	seedSpecs := themeSeedSpecsFromInput(input)
	packs, err := themestudy.BuildSeedPacks(
		seedSpecs, 0, 0, 0, 0,
		func(path string, startLine, endLine int) ([]string, error) {
			return []string{"func " + filepath.Base(path) + "() {", "	// fixture body", "}"}, nil
		},
		func(path string) (int, error) { return 3, nil },
	)
	if err != nil {
		t.Fatalf("build seed packs: %v", err)
	}
	request, err := themestudy.CompileScout(
		themestudy.LanguageEnglish, vocabulary, packs,
		themeScoutContext(product, "runtime-fixture"), "",
	)
	if err != nil {
		t.Fatalf("compile scout request: %v", err)
	}
	requestRaw, err := themestudy.EncodeScoutRequest(request)
	if err != nil {
		t.Fatalf("encode scout request: %v", err)
	}
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(runDir, themestudy.ScoutRequestArtifactFilename), requestRaw)

	responseRaw, err := themestudy.MockScoutResponse(request)
	if err != nil {
		t.Fatalf("mock scout response: %v", err)
	}
	responsePath := filepath.Join(parent, "saved-response.json")
	mustWriteAtlasStudyReplayTestFile(t, responsePath, responseRaw)
	return atlasStudyReplayFixture{
		runDir: runDir, responsePath: responsePath,
		requestSHA:  atlasStudyReplaySHA256(requestRaw),
		responseSHA: atlasStudyReplaySHA256(responseRaw),
	}
}

func runAtlasStudyReplayTestCLI(fixture atlasStudyReplayFixture, responsePath, responseSHA string, stdout *bytes.Buffer) error {
	if stdout == nil {
		stdout = &bytes.Buffer{}
	}
	return runThemeStudyResponseReplayCLI([]string{
		"--run-dir", fixture.runDir,
		"--request-sha256", fixture.requestSHA,
		"--response", responsePath,
		"--response-sha256", responseSHA,
	}, stdout)
}

func mustWriteAtlasStudyReplayTestFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func assertAtlasStudyReplayOutputsAbsent(t *testing.T, runDir string) {
	t.Helper()
	for _, name := range []string{themestudy.ScoutResultArtifactFilename, themestudy.ScoutStatusArtifactFilename} {
		if _, err := os.Lstat(filepath.Join(runDir, name)); !os.IsNotExist(err) {
			t.Fatalf("output %s exists or cannot be inspected: %v", name, err)
		}
	}
}

func assertAtlasStudyReplayTestFile(t *testing.T, name string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s changed: got %q want %q", name, got, want)
	}
}

func TestAtlasStudyResponseReplayCLIExactSavedResponse(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	oldResult := []byte("old result must not be read or changed\n")
	oldStatus := []byte("old status must not be read or changed\n")
	// Historical single-stage v7 files in the copy are never read, changed or
	// removed by the theme replay: the retired pipeline's artifacts are
	// outside the theme artifact set.
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_result.v7.json"), oldResult)
	mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_status.v7.json"), oldStatus)

	var stdout bytes.Buffer
	err := runThemeStudyResponseReplayCLI([]string{
		"--run-dir", fixture.runDir,
		"--request-sha256", fixture.requestSHA,
		"--response", fixture.responsePath,
		"--response-sha256", fixture.responseSHA,
	}, &stdout)
	if err != nil {
		t.Fatalf("runThemeStudyResponseReplayCLI: %v", err)
	}
	for _, want := range []string{
		"request_sha256: " + fixture.requestSHA,
		"response_sha256: " + fixture.responseSHA,
		"result_sha256: ",
		"status_sha256: ",
		"candidates: ",
		"provider_calls: 0",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}

	resultRaw, err := os.ReadFile(filepath.Join(fixture.runDir, themestudy.ScoutResultArtifactFilename))
	if err != nil {
		t.Fatalf("read current result: %v", err)
	}
	result, err := themestudy.DecodeScoutResult(resultRaw)
	if err != nil {
		t.Fatalf("decode current result: %v", err)
	}
	if result.Version != themestudy.ScoutResultVersion || len(result.Candidates) == 0 {
		t.Fatalf("theme scout result cardinality = %#v", result)
	}
	statusRaw, err := os.ReadFile(filepath.Join(fixture.runDir, themestudy.ScoutStatusArtifactFilename))
	if err != nil {
		t.Fatalf("read current status: %v", err)
	}
	status, err := themestudy.DecodeScoutStatus(statusRaw)
	if err != nil {
		t.Fatalf("decode current status: %v", err)
	}
	if status.Version != themestudy.ScoutRequestVersion ||
		(status.Status.Accepted == 0 && status.State != string(atlasstudy.ProductStateFailed)) {
		t.Fatalf("status = %+v", status)
	}
	assertAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_result.v7.json"), oldResult)
	assertAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, "atlas_study_status.v7.json"), oldStatus)
}

func TestAtlasStudyResponseReplayCLIRequiresExactHashes(t *testing.T) {
	for _, test := range []struct {
		name        string
		requestSHA  string
		responseSHA string
		want        string
	}{
		{name: "wrong request", requestSHA: strings.Repeat("0", 64), want: "request SHA-256 mismatch"},
		{name: "wrong response", responseSHA: strings.Repeat("0", 64), want: "response SHA-256 mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
			requestSHA := test.requestSHA
			if requestSHA == "" {
				requestSHA = fixture.requestSHA
			}
			responseSHA := test.responseSHA
			if responseSHA == "" {
				responseSHA = fixture.responseSHA
			}
			var stdout bytes.Buffer
			err := runThemeStudyResponseReplayCLI([]string{
				"--run-dir", fixture.runDir,
				"--request-sha256", requestSHA,
				"--response", fixture.responsePath,
				"--response-sha256", responseSHA,
			}, &stdout)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure wrote stdout: %q", stdout.String())
			}
			assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
		})
	}
}

func TestAtlasStudyResponseReplayCLIRejectsUnsafeResponseFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		link := filepath.Join(t.TempDir(), "response-link.json")
		if err := os.Symlink(fixture.responsePath, link); err != nil {
			t.Fatal(err)
		}
		err := runAtlasStudyReplayTestCLI(fixture, link, fixture.responseSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "regular non-symlink") {
			t.Fatalf("symlink error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})

	t.Run("nonregular", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		responseDir := filepath.Join(t.TempDir(), "response-dir")
		if err := os.Mkdir(responseDir, 0o700); err != nil {
			t.Fatal(err)
		}
		err := runAtlasStudyReplayTestCLI(fixture, responseDir, strings.Repeat("0", 64), nil)
		if err == nil || !strings.Contains(err.Error(), "regular non-symlink") {
			t.Fatalf("nonregular error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})

	t.Run("inside target run", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
		inside := filepath.Join(fixture.runDir, "semantic_exchanges", "exact-response.json")
		if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
			t.Fatal(err)
		}
		responseRaw, err := os.ReadFile(fixture.responsePath)
		if err != nil {
			t.Fatal(err)
		}
		mustWriteAtlasStudyReplayTestFile(t, inside, responseRaw)
		err = runAtlasStudyReplayTestCLI(fixture, inside, fixture.responseSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "outside the target run") {
			t.Fatalf("inside-run error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})
}

func TestAtlasStudyResponseReplayCLIMandatorySecretScanDoesNotEcho(t *testing.T) {
	fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
	credential := "Bearer replaycredentialvalue123456789"
	responsePath := filepath.Join(t.TempDir(), "unsafe-response.json")
	mustWriteAtlasStudyReplayTestFile(t, responsePath, []byte(credential))
	restore := secretscan.SetDisabled(true)
	defer restore()

	err := runAtlasStudyReplayTestCLI(fixture, responsePath, atlasStudyReplaySHA256([]byte(credential)), nil)
	if err == nil || !strings.Contains(err.Error(), "bearer_credential") {
		t.Fatalf("secret response error = %v", err)
	}
	if strings.Contains(err.Error(), credential) || strings.Contains(err.Error(), "replaycredentialvalue") {
		t.Fatalf("secret response error echoed credential: %v", err)
	}
	assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
}

func TestAtlasStudyResponseReplayCLIRejectsOriginalAndPreexistingOutputs(t *testing.T) {
	t.Run("original canonical run", func(t *testing.T) {
		fixture := writeAtlasStudyResponseReplayFixture(t, "canonical-run", "canonical-run")
		err := runAtlasStudyReplayTestCLI(fixture, fixture.responsePath, fixture.responseSHA, nil)
		if err == nil || !strings.Contains(err.Error(), "original canonical run") {
			t.Fatalf("original-run error = %v", err)
		}
		assertAtlasStudyReplayOutputsAbsent(t, fixture.runDir)
	})

	for _, output := range []string{themestudy.ScoutResultArtifactFilename, themestudy.ScoutStatusArtifactFilename} {
		t.Run(output, func(t *testing.T) {
			fixture := writeAtlasStudyResponseReplayFixture(t, "copied-review", "original-canonical-run")
			sentinel := []byte("must not overwrite\n")
			mustWriteAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, output), sentinel)
			err := runAtlasStudyReplayTestCLI(fixture, fixture.responsePath, fixture.responseSHA, nil)
			if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
				t.Fatalf("preexisting-output error = %v", err)
			}
			assertAtlasStudyReplayTestFile(t, filepath.Join(fixture.runDir, output), sentinel)
		})
	}
}

// themeFixtureOpenablePaths collects the distinct openable source paths from
// the fixture input's reading targets, mirroring the report's OpenablePaths.
func themeFixtureOpenablePaths(input atlasstudy.Input) []string {
	seen := make(map[string]struct{})
	var paths []string
	for _, target := range input.ReadingTargets {
		path := target.Location.Path
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	return paths
}

// atlasStudyRuntimeInput is the minimal exact Atlas Study substrate the
// provider-free runtime fixture compiles: one app unit, one focused startup
// span with a single allowed reading target, plus configuration and route
// anchors that stay advertised but not selected.
func atlasStudyRuntimeInput() atlasstudy.Input {
	atlas := repositoryatlas.Atlas{
		Version: repositoryatlas.Version,
		Units: []repositoryatlas.Unit{
			{ID: "unit-repository-runtime", Kind: repositoryatlas.UnitRepository, Name: "Runtime fixture"},
			{ID: "unit-module-runtime", Kind: repositoryatlas.UnitModule, ParentID: "unit-repository-runtime", Name: "Runtime module"},
			{ID: "unit-app-runtime", Kind: repositoryatlas.UnitApp, ParentID: "unit-module-runtime", Name: "Runtime app"},
		},
		Entities: []repositoryatlas.Entity{
			{ID: "surface-start-runtime", Kind: repositoryatlas.EntitySurface, UnitID: "unit-app-runtime"},
			{ID: "operation-start-runtime", Kind: repositoryatlas.EntityOperation, UnitID: "unit-app-runtime"},
		},
		Evidence: []repositoryatlas.Evidence{{
			ID: "evidence-start-runtime", UnitID: "unit-app-runtime",
			Location: evidence.Location{Path: "cmd/server/main.go", Line: 20}, Symbol: "RunServer",
			Provenance: evidence.Provenance{Provider: "fixture", Operation: "observe_start"},
		}},
		Relations: []repositoryatlas.Relation{{
			ID: "relation-start-runtime", UnitID: "unit-app-runtime",
			Kind: repositoryatlas.RelationExposes, Phase: repositoryatlas.PhaseStartup,
			Authority:    repositoryatlas.AuthorityResolved,
			Source:       repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: "surface-start-runtime"},
			Target:       repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: "operation-start-runtime"},
			EvidenceRefs: []string{"evidence-start-runtime"},
		}},
	}
	return atlasstudy.Input{
		Atlas: atlas, Language: atlasstudy.LanguageEnglish, Limits: atlasstudy.DefaultLimits(),
		Architecture: atlasstudy.ArchitectureInput{
			Version: 5, Source: "normalized_model", Title: "Runtime anatomy",
			Subsystems: []atlasstudy.Subsystem{{
				ID: "subsystem-core-runtime", Name: "Core",
				Authority:    repositoryatlas.AuthorityInferred,
				ComponentIDs: []string{"component-api-runtime"},
			}},
			Components: []atlasstudy.Component{{
				ID: "component-api-runtime", SubsystemID: "subsystem-core-runtime", Name: "API",
				Authority:        repositoryatlas.AuthorityInferred,
				ReadingTargetIDs: []string{"anchor-config-runtime", "anchor-route-runtime", "anchor-start-runtime"},
			}},
		},
		Surfaces: []atlasstudy.Surface{{
			ID: "surface-start-runtime", UnitID: "unit-app-runtime", Name: "Server entry",
			Kind: "process_entry", Authority: repositoryatlas.AuthorityResolved,
		}},
		ReadingTargets: []atlasstudy.ReadingTarget{
			{ID: "anchor-start-runtime", Owner: atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}, RelatedComponentIDs: []string{"component-api-runtime"}, PrincipalRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}}, Kind: atlasstudy.ReadingTargetEntrypoint, Label: "Server startup", Fact: "Initializes the application shell.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "cmd/server/main.go", Line: 20}, Symbol: "RunServer"},
			{ID: "anchor-config-runtime", Owner: atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}, RelatedComponentIDs: []string{"component-api-runtime"}, PrincipalRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}}, Kind: atlasstudy.ReadingTargetFunction, Label: "Configuration", Fact: "Loads settings.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/config/load.go", Line: 14}, Symbol: "Load"},
			{ID: "anchor-route-runtime", Owner: atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}, RelatedComponentIDs: []string{"component-api-runtime"}, PrincipalRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefComponent, ID: "component-api-runtime"}}, Kind: atlasstudy.ReadingTargetFunction, Label: "Routes", Fact: "Registers handlers.", Authority: repositoryatlas.AuthorityObserved, Location: evidence.Location{Path: "internal/server/routes.go", Line: 31}, Symbol: "RegisterRoutes"},
		},
		ReadingSupports: []atlasstudy.ReadingSupport{{
			ID: "support-start-runtime", TargetID: "anchor-start-runtime",
			PackageBucket: "package-server-runtime", Role: atlasstudy.SupportProcessEntry,
			Authority: repositoryatlas.AuthorityObserved,
		}},
		RouteSpans: []atlasstudy.RouteSpan{{
			ID: "span-start-runtime", Kind: atlasstudy.RouteSpanFocused,
			QuestionEnglish: "Where does this application start?",
			QuestionRussian: "Где запускается это приложение?",
			TargetJob:       atlasstudy.JobFirstContact, LearningStage: atlasstudy.StageOrientation,
			RequiredSupportIDs: []string{"support-start-runtime"},
			AllowedTargetIDs:   []string{"anchor-start-runtime"},
		}},
		Evidence: []atlasstudy.EvidenceFact{{
			ID:          "evidence-start-runtime",
			SubjectRefs: []atlasstudy.CanonicalRef{{Kind: atlasstudy.RefSurface, ID: "surface-start-runtime"}},
			Authority:   repositoryatlas.AuthorityResolved, Fact: "The application exposes a startup surface.",
		}},
		Documents: []atlasstudy.DocumentClaim{{
			ID: "document-purpose-runtime", Label: "Documented purpose",
			Claim:     "The project provides a server for identity workflows.",
			Authority: repositoryatlas.AuthorityObserved,
		}},
	}
}
