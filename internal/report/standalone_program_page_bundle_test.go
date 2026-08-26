package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programpage"
)

func TestWriteStandaloneProgramPageBundlePublishesLanguageNeutralPortfolio(t *testing.T) {
	portfolio, ready, runDir := standaloneProgramPageBundleFixture(t)
	if err := WriteStandaloneProgramPageBundleAtomic(runDir, portfolio, ready); err != nil {
		t.Fatalf("WriteStandaloneProgramPageBundleAtomic: %v", err)
	}
	htmlPath := filepath.Join(runDir, "report.html")
	identity, found, err := InspectStandaloneTargetBundleHTML(htmlPath)
	if err != nil || !found {
		t.Fatalf("InspectStandaloneTargetBundleHTML = %#v, %t, %v", identity, found, err)
	}
	defaultIndex := standaloneProgramPageDefaultIndex(t, portfolio)
	if identity.Version != StandaloneTargetBundleVersion ||
		identity.ProgramPagePortfolioSHA256 != portfolio.SHA256 ||
		identity.TargetRunContainerSHA256 != "" || identity.TargetPagePortfolioSHA256 != "" ||
		identity.DefaultTargetIndex != defaultIndex || identity.TargetCount != len(portfolio.Pages) {
		t.Fatalf("standalone program page identity = %#v", identity)
	}

	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	wire := embeddedStandaloneTargetBundle(t, htmlBytes)
	if wire.DefaultTargetIndex != defaultIndex || len(wire.Targets) != len(portfolio.Pages) {
		t.Fatalf("standalone program page wire = %#v", wire)
	}
	gotLanguages := make([]string, 0, len(wire.Targets))
	for index, item := range wire.Targets {
		binding := portfolio.Pages[index]
		if item.TargetID != binding.Target.ID || item.Language != binding.Target.Language ||
			item.Kind != binding.Target.Kind || item.DisplayName != binding.Target.Name ||
			item.Href != fmt.Sprintf("?target=%d#/program", index) {
			t.Fatalf("standalone program page item %d = %#v", index, item)
		}
		gotLanguages = append(gotLanguages, item.Language)
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(item.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if _, found := payload["analysis_target"]; found {
			t.Fatalf("language-neutral payload %d retained Go analysis authority: %s", index, item.Payload)
		}
		if _, found := payload["target_navigation"]; found {
			t.Fatalf("prepared payload %d retained sibling navigation: %s", index, item.Payload)
		}
	}
	slices.Sort(gotLanguages)
	if !slices.Equal(gotLanguages, []string{"go", "python", "typescript"}) {
		t.Fatalf("standalone languages = %v", gotLanguages)
	}
	for _, binding := range portfolio.Pages {
		if bytes.Contains(htmlBytes, []byte(binding.RunID)) {
			t.Errorf("standalone bundle leaked sibling run ID %q", binding.RunID)
		}
	}

	validated, err := validateStandaloneProgramPageBundle(portfolio, ready)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyExactStandaloneTargetBundleProjection(
		htmlPath,
		validated.identity,
		validated.defaultTarget.repoName,
		func(index int) (standaloneTargetBundleItem, error) { return validated.targets[index], nil },
	); err != nil {
		t.Fatalf("verify exact neutral projection: %v", err)
	}
}

func TestStandaloneProgramPageBundleRejectsBindingDriftAtomically(t *testing.T) {
	portfolio, ready, runDir := standaloneProgramPageBundleFixture(t)
	htmlPath := filepath.Join(runDir, "report.html")
	const original = "ORIGINAL_REPORT"
	if err := os.WriteFile(htmlPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func([]PreparedStandaloneTarget) []PreparedStandaloneTarget{
		"missing page": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			return values[:len(values)-1]
		},
		"run mismatch": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			values = clonePreparedStandaloneTargets(values)
			values[0].prepared.programPage.RunID = "foreign-run-1"
			return values
		},
		"target mismatch": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			values = clonePreparedStandaloneTargets(values)
			values[0].prepared.programPage.ProgramTarget = values[1].prepared.programPage.ProgramTarget.Snapshot()
			return values
		},
		"source drift": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			values = clonePreparedStandaloneTargets(values)
			values[0].prepared.repositoryURL = "https://github.com/example/foreign"
			return values
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if err := WriteStandaloneProgramPageBundleAtomic(runDir, portfolio, mutate(ready)); err == nil {
				t.Fatal("authority drift was accepted")
			}
			current, err := os.ReadFile(htmlPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(current) != original {
				t.Fatalf("failed atomic publication replaced report.html: %q", current)
			}
		})
	}

	wrongDestination := filepath.Join(filepath.Dir(runDir), "not-the-default-run")
	if err := os.Mkdir(wrongDestination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteStandaloneProgramPageBundleAtomic(wrongDestination, portfolio, ready); err == nil ||
		!strings.Contains(err.Error(), "destination is not the default run") {
		t.Fatalf("wrong destination error = %v", err)
	}
}

func TestStandaloneProgramPageBundleRejectsResealedPayloadRewrite(t *testing.T) {
	portfolio, ready, runDir := standaloneProgramPageBundleFixture(t)
	validated, err := validateStandaloneProgramPageBundle(portfolio, ready)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteStandaloneProgramPageBundleAtomic(runDir, portfolio, ready); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(runDir, "report.html")
	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte("PROGRAM_PAGE_0"), []byte("PROGRAM_PAGE_X"), 1)
	if bytes.Equal(tampered, raw) {
		t.Fatal("neutral payload sentinel was not present")
	}
	sealStart := bytes.LastIndex(tampered, []byte(standaloneTargetBundleSealPrefix))
	if sealStart < 0 {
		t.Fatal("bundle seal was not present")
	}
	digest := sha256.Sum256(tampered[:sealStart])
	tampered = append(
		append([]byte(nil), tampered[:sealStart]...),
		[]byte(standaloneTargetBundleSealPrefix+hex.EncodeToString(digest[:])+standaloneTargetBundleSealSuffix)...,
	)
	if err := os.WriteFile(htmlPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := InspectStandaloneTargetBundleHTML(htmlPath); err != nil || !found {
		t.Fatalf("self-sealed rewritten bundle was not structurally valid: found=%t err=%v", found, err)
	}
	if err := verifyExactStandaloneTargetBundleProjection(
		htmlPath,
		validated.identity,
		validated.defaultTarget.repoName,
		func(index int) (standaloneTargetBundleItem, error) { return validated.targets[index], nil },
	); err == nil || !strings.Contains(err.Error(), "manifest-derived projection") {
		t.Fatalf("resealed neutral payload authority error = %v", err)
	}
}

func TestStandaloneBundleIdentityAuthoritiesAreMutuallyExclusive(t *testing.T) {
	digest := strings.Repeat("a", 64)
	base := StandaloneTargetBundleIdentity{
		Version: StandaloneTargetBundleVersion, DefaultTargetIndex: 0, TargetCount: 2,
	}
	legacy := base
	legacy.TargetRunContainerSHA256 = digest
	legacy.TargetPagePortfolioSHA256 = digest
	if !validStandaloneTargetBundleIdentity(legacy) {
		t.Fatal("legacy standalone authority became invalid")
	}
	neutral := base
	neutral.ProgramPagePortfolioSHA256 = digest
	if !validStandaloneTargetBundleIdentity(neutral) {
		t.Fatal("neutral standalone authority is invalid")
	}
	mixed := legacy
	mixed.ProgramPagePortfolioSHA256 = digest
	if validStandaloneTargetBundleIdentity(mixed) {
		t.Fatal("standalone identity accepted mixed legacy and neutral authority")
	}
	if validStandaloneTargetBundleIdentity(base) {
		t.Fatal("standalone identity accepted missing authority")
	}
}

func TestPublishedStandaloneProgramPageAuthorityUsesNeutralPortfolio(t *testing.T) {
	fixture := newProgramPageManifestFixture(t)
	runDir, manifest := fixture.run(t)
	identity := StandaloneTargetBundleIdentity{
		Version: StandaloneTargetBundleVersion, ProgramPagePortfolioSHA256: fixture.portfolio.SHA256,
		DefaultTargetIndex: standaloneProgramPageDefaultIndex(t, fixture.portfolio),
		TargetCount:        len(fixture.portfolio.Pages),
	}
	if err := verifyPublishedStandaloneTargetAuthority(runDir, manifest, identity); err != nil {
		t.Fatalf("verify neutral publication authority: %v", err)
	}
	identity.ProgramPagePortfolioSHA256 = strings.Repeat("f", 64)
	if err := verifyPublishedStandaloneTargetAuthority(runDir, manifest, identity); err == nil ||
		!strings.Contains(err.Error(), "does not match") {
		t.Fatalf("drifted neutral publication authority error = %v", err)
	}
}

func standaloneProgramPageBundleFixture(
	t *testing.T,
) (programpage.Portfolio, []PreparedStandaloneTarget, string) {
	t.Helper()
	indexes := []programindex.Index{
		reportProgramIndexFixture(t, "go", "executable"),
		reportProgramIndexFixture(t, "python", "worker"),
		reportProgramIndexFixture(t, "typescript", "application"),
	}
	pages := make([]programpage.Page, 0, len(indexes))
	indexByTargetID := make(map[string]programindex.Index, len(indexes))
	for index, programIndex := range indexes {
		pages = append(pages, programpage.Page{
			Target: programIndex.Target,
			RunID:  fmt.Sprintf("program-page-%d", index),
		})
		indexByTargetID[programIndex.Target.ID] = programIndex
	}
	portfolio, err := programpage.Build(indexes[1].Target.ID, pages)
	if err != nil {
		t.Fatal(err)
	}
	ready := make([]PreparedStandaloneTarget, 0, len(portfolio.Pages))
	for index, binding := range portfolio.Pages {
		programIndex := indexByTargetID[binding.Target.ID]
		programPortfolio, err := NewProgramPortfolio(binding.Target.ID, []programindex.Index{programIndex})
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(map[string]any{
			"format_version":       CurrentFormatVersion,
			"program_portfolio":    programPortfolio,
			"repo_name":            "neutral-bundle-fixture",
			"captured_revision":    standaloneBundleRevision,
			"captured_input_count": 0,
			"openable_paths":       []string{},
			"warnings":             []string{fmt.Sprintf("PROGRAM_PAGE_%d", index)},
			"github_source_links": map[string]any{
				"repository_url": "https://github.com/example/neutral-bundle",
				"revision":       standaloneBundleRevision,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		ready = append(ready, PreparedStandaloneTarget{prepared: &preparedStandaloneTarget{
			programPage: TargetNavigationPage{
				RunID: binding.RunID, ProgramTarget: binding.Target.Snapshot(),
				ArtifactFilename: programindex.ArtifactFilename,
			},
			payload: payload,
			host:    "GitHub", repositoryURL: "https://github.com/example/neutral-bundle",
			revision: standaloneBundleRevision, repoName: "neutral-bundle-fixture",
			localRoots: []string{filepath.Join(t.TempDir(), "private")},
		}})
	}
	slices.Reverse(ready)
	defaultRunID := ""
	for _, binding := range portfolio.Pages {
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaultRunID = binding.RunID
			break
		}
	}
	runDir := filepath.Join(t.TempDir(), defaultRunID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	return portfolio, ready, runDir
}

func standaloneProgramPageDefaultIndex(t *testing.T, portfolio programpage.Portfolio) int {
	t.Helper()
	for index, binding := range portfolio.Pages {
		if binding.Target.ID == portfolio.DefaultTargetID {
			return index
		}
	}
	t.Fatal("program page portfolio default is absent")
	return -1
}
