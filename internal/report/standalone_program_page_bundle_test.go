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
	"github.com/dvordrova/repomap/internal/runtimeportfolio"
	"github.com/dvordrova/repomap/internal/targetoutcome"
)

func TestWriteProgramPageBundleFromArtifactsPublishesOnlyOwnerHTML(t *testing.T) {
	portfolio, ownerRunDir, runDirs := artifactProgramPageBundleFixture(t, 2)
	entriesBefore := make(map[string][]string, len(runDirs))
	for _, runDir := range runDirs {
		entriesBefore[runDir] = directoryTreePaths(t, runDir)
		if _, err := os.Lstat(filepath.Join(runDir, "report.html")); !os.IsNotExist(err) {
			t.Fatalf("backing page unexpectedly published HTML before bundling: %s: %v", runDir, err)
		}
	}

	if err := WriteProgramPageBundleFromArtifactsAtomic(ownerRunDir, portfolio); err != nil {
		t.Fatalf("WriteProgramPageBundleFromArtifactsAtomic: %v", err)
	}
	for _, runDir := range runDirs {
		wantEntries := append([]string(nil), entriesBefore[runDir]...)
		if runDir == ownerRunDir {
			wantEntries = append(wantEntries, "report.html")
			slices.Sort(wantEntries)
		}
		if entriesAfter := directoryTreePaths(t, runDir); !slices.Equal(entriesAfter, wantEntries) {
			t.Fatalf("artifact publication left browser sidecars in %s: before=%v after=%v",
				runDir, entriesBefore[runDir], entriesAfter)
		}
	}
	ownerHTMLPath := filepath.Join(ownerRunDir, "report.html")
	ownerHTML, err := os.ReadFile(ownerHTMLPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, runDir := range runDirs {
		htmlPath := filepath.Join(runDir, "report.html")
		if runDir == ownerRunDir {
			continue
		}
		if _, err := os.Lstat(htmlPath); !os.IsNotExist(err) {
			t.Fatalf("backing page published sibling HTML: %s: %v", htmlPath, err)
		}
	}

	manifest, err := ReadRunManifest(ownerRunDir)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := VerifyStandaloneProgramPageBundleHTML(
		ownerHTMLPath, ownerRunDir, manifest,
	)
	if err != nil {
		t.Fatalf("VerifyStandaloneProgramPageBundleHTML: %v", err)
	}
	if identity.ProgramPagePortfolioSHA256 != portfolio.SHA256 ||
		identity.TargetCount != len(portfolio.Pages) {
		t.Fatalf("artifact-derived bundle identity = %#v", identity)
	}
	transport, err := extractStandaloneBundleTransportV4HTML(ownerHTML)
	if err != nil {
		t.Fatal(err)
	}
	if len(transport.TargetChunks) != len(portfolio.Pages) {
		t.Fatalf("owner HTML target chunk count = %d, want %d", len(transport.TargetChunks), len(portfolio.Pages))
	}
	for index, chunk := range transport.TargetChunks {
		raw, decodeErr := decodeStandaloneBundleTargetChunkV4(chunk.Ref, chunk.Base64)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		payload, decodeErr := DecodeBrowserTargetPayload(raw)
		if decodeErr != nil || payload.Target.ID != portfolio.Pages[index].Target.ID {
			t.Fatalf("owner HTML target payload %d = %#v, %v", index, payload.Target, decodeErr)
		}
	}

	var siblingRunDir string
	siblingIndex := -1
	for index, runDir := range runDirs {
		if runDir != ownerRunDir {
			siblingRunDir = runDir
			siblingIndex = index
			break
		}
	}
	if siblingRunDir == "" {
		t.Fatal("fixture has no sibling run")
	}
	siblingHTMLPath := filepath.Join(siblingRunDir, "report.html")
	if err := os.WriteFile(siblingHTMLPath, []byte("stray sibling report\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyStandaloneProgramPageBundleHTML(
		ownerHTMLPath, ownerRunDir, manifest,
	); err == nil || !strings.Contains(err.Error(), "unexpectedly publishes report.html") {
		t.Fatalf("sibling HTML layout error = %v", err)
	}
	if err := WriteProgramPageBundleFromArtifactsAtomic(ownerRunDir, portfolio); err == nil ||
		!strings.Contains(err.Error(), "unexpectedly publishes report.html") {
		t.Fatalf("republish with sibling HTML error = %v", err)
	}
	if err := os.Remove(siblingHTMLPath); err != nil {
		t.Fatal(err)
	}
	siblingReportPath := filepath.Join(siblingRunDir, "report.json")
	siblingReport, err := os.ReadFile(siblingReportPath)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := []byte(fmt.Sprintf("BACKING_REPORT_%d", siblingIndex))
	tampered := bytes.Replace(siblingReport, sentinel, []byte("BACKING_REPORT_X"), 1)
	if bytes.Equal(tampered, siblingReport) {
		t.Fatal("sibling report sentinel was not present")
	}
	if err := os.WriteFile(siblingReportPath, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyStandaloneProgramPageBundleHTML(
		ownerHTMLPath, ownerRunDir, manifest,
	); err == nil || !strings.Contains(err.Error(), "report sha256 mismatch") {
		t.Fatalf("tampered backing report authority error = %v", err)
	}
	if err := WriteProgramPageBundleFromArtifactsAtomic(ownerRunDir, portfolio); err == nil ||
		!strings.Contains(err.Error(), "report sha256 mismatch") {
		t.Fatalf("republish with tampered backing report error = %v", err)
	}
	unchangedOwnerHTML, err := os.ReadFile(ownerHTMLPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(unchangedOwnerHTML, ownerHTML) {
		t.Fatal("failed artifact-derived republish replaced the verified owner HTML")
	}
}

func TestWriteProgramPageBundleFromArtifactsRejectsSiblingAnalysisRootDriftAtomically(t *testing.T) {
	portfolio, ownerRunDir, runDirs := artifactProgramPageBundleFixture(t, 2)
	siblingRunDir := ""
	for _, runDir := range runDirs {
		if runDir != ownerRunDir {
			siblingRunDir = runDir
			break
		}
	}
	if siblingRunDir == "" {
		t.Fatal("fixture has no sibling run")
	}

	siblingManifest, err := ReadRunManifest(siblingRunDir)
	if err != nil {
		t.Fatal(err)
	}
	siblingManifest.AnalysisRoot = filepath.Join(
		siblingManifest.RepositoryState.Identity, "subtree",
	)
	if err := writeRunManifestAtomic(siblingRunDir, siblingManifest); err != nil {
		t.Fatalf("write valid sibling manifest with a drifted analysis root: %v", err)
	}
	resealedSibling, err := ReadRunManifest(siblingRunDir)
	if err != nil {
		t.Fatal(err)
	}
	if resealedSibling.AnalysisRoot != "/repo/subtree" {
		t.Fatalf("resealed sibling analysis root = %q", resealedSibling.AnalysisRoot)
	}
	if err := resealedSibling.Validate(); err != nil {
		t.Fatalf("resealed sibling manifest is not independently valid: %v", err)
	}

	ownerHTMLPath := filepath.Join(ownerRunDir, "report.html")
	if err := WriteProgramPageBundleFromArtifactsAtomic(ownerRunDir, portfolio); err == nil ||
		!strings.Contains(err.Error(), "authority mismatch") {
		t.Fatalf("sibling analysis-root drift error = %v", err)
	}
	if _, err := os.Lstat(ownerHTMLPath); !os.IsNotExist(err) {
		t.Fatalf("failed artifact-derived publication created owner HTML: %v", err)
	}

	sentinel := []byte("ORIGINAL_OWNER_REPORT\n")
	if err := os.WriteFile(ownerHTMLPath, sentinel, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteProgramPageBundleFromArtifactsAtomic(ownerRunDir, portfolio); err == nil ||
		!strings.Contains(err.Error(), "authority mismatch") {
		t.Fatalf("sibling analysis-root drift republish error = %v", err)
	}
	current, err := os.ReadFile(ownerHTMLPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(current, sentinel) {
		t.Fatal("failed artifact-derived publication replaced owner HTML")
	}
}

func TestWriteProgramPageBundleFromArtifactsAllowsOneAnalyzedPage(t *testing.T) {
	portfolio, ownerRunDir, runDirs := artifactProgramPageBundleFixture(t, 1)
	if len(portfolio.Pages) != 1 || len(runDirs) != 1 {
		t.Fatalf("single analyzed-page fixture = %d pages / %d runs", len(portfolio.Pages), len(runDirs))
	}
	if err := WriteProgramPageBundleFromArtifactsAtomic(ownerRunDir, portfolio); err != nil {
		t.Fatalf("WriteProgramPageBundleFromArtifactsAtomic: %v", err)
	}
	manifest, err := ReadRunManifest(ownerRunDir)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := VerifyStandaloneProgramPageBundleHTML(
		filepath.Join(ownerRunDir, "report.html"), ownerRunDir, manifest,
	)
	if err != nil {
		t.Fatalf("VerifyStandaloneProgramPageBundleHTML: %v", err)
	}
	if identity.ProgramPagePortfolioSHA256 != portfolio.SHA256 ||
		identity.TargetCount != 1 || identity.DefaultTargetIndex != 0 {
		t.Fatalf("single analyzed-page bundle identity = %#v", identity)
	}
	reportRaw, err := os.ReadFile(filepath.Join(ownerRunDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var data ReportData
	if err := json.Unmarshal(reportRaw, &data); err != nil {
		t.Fatal(err)
	}
	if data.TargetOutcomePortfolio == nil || len(data.TargetOutcomePortfolio.Outcomes) != 2 ||
		data.TargetOutcomePortfolio.DefaultSelectedTargetID == "" {
		t.Fatalf("partial selected-target outcomes = %#v", data.TargetOutcomePortfolio)
	}
	defaultFailed := false
	for _, outcome := range data.TargetOutcomePortfolio.Outcomes {
		if outcome.SelectedTargetID == data.TargetOutcomePortfolio.DefaultSelectedTargetID {
			defaultFailed = outcome.State == targetoutcome.StateNotAnalyzed
		}
	}
	if !defaultFailed {
		t.Fatalf("logical selected default was not retained as failed: %#v", data.TargetOutcomePortfolio)
	}
	htmlRaw, err := os.ReadFile(filepath.Join(ownerRunDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	transport, err := extractStandaloneBundleTransportV4HTML(htmlRaw)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := DecodeBrowserRepositoryPayload(transport.RepositoryPayload)
	if err != nil {
		t.Fatal(err)
	}
	if len(transport.Index.Targets) != 2 || len(repository.Targets) != 2 ||
		len(transport.TargetChunks) != 1 {
		t.Fatalf("partial artifact transport = %d index rows / %d repository rows / %d chunks",
			len(transport.Index.Targets), len(repository.Targets), len(transport.TargetChunks))
	}
	failedIndexFound := false
	failedRepositoryFound := false
	for _, row := range transport.Index.Targets {
		if row.TargetID != transport.Index.LogicalDefaultTargetID {
			continue
		}
		failedIndexFound = row.State == standaloneBundleTransportTargetNotAnalyzed &&
			row.ProgramTargetID == "" && row.Chunk == nil
	}
	for _, row := range repository.Targets {
		if row.SelectedTargetID != repository.LogicalDefaultSelectedTargetID {
			continue
		}
		failedRepositoryFound = row.State == "not_analyzed" && row.ProgramTargetID == "" && row.Href == ""
	}
	if !failedIndexFound || !failedRepositoryFound {
		t.Fatalf("failed logical default gained target authority: index=%#v repository=%#v",
			transport.Index.Targets, repository.Targets)
	}
}

func TestWriteStandaloneProgramPageBundleRejectsCrossTargetLocalRootLeak(t *testing.T) {
	portfolio, ready, runDir := standaloneProgramPageBundleFixture(t)
	if len(ready) < 2 || ready[0].prepared == nil || ready[1].prepared == nil ||
		len(ready[0].prepared.localRoots) == 0 {
		t.Fatal("standalone fixture lacks two targets with local roots")
	}
	foreignRoot := ready[0].prepared.localRoots[0]
	ready[1].prepared.target.Target.Name = foreignRoot
	raw, err := EncodeBrowserTargetPayload(ready[1].prepared.target)
	if err != nil {
		t.Fatal(err)
	}
	ready[1].prepared.targetPayload = raw
	if err := WriteStandaloneProgramPageBundleAtomic(runDir, portfolio, ready); err == nil ||
		!strings.Contains(err.Error(), "retained a local path") {
		t.Fatalf("cross-target local-root leak error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(runDir, "report.html")); !os.IsNotExist(err) {
		t.Fatalf("failed local-root publication created report.html: %v", err)
	}
}

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
	transport, err := extractStandaloneBundleTransportV4HTML(htmlBytes)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := DecodeBrowserRepositoryPayload(transport.RepositoryPayload)
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.Targets) != len(portfolio.Pages) ||
		repository.LogicalDefaultSelectedTargetID != portfolio.DefaultTargetID {
		t.Fatalf("standalone repository index = %#v", repository)
	}
	gotLanguages := make([]string, 0, len(transport.TargetChunks))
	for index, chunk := range transport.TargetChunks {
		raw, decodeErr := decodeStandaloneBundleTargetChunkV4(chunk.Ref, chunk.Base64)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		payload, decodeErr := DecodeBrowserTargetPayload(raw)
		if decodeErr != nil {
			t.Fatal(decodeErr)
		}
		if repository.Targets[index].Href != fmt.Sprintf("?target=%d#/program", index) ||
			payload.Target.ID != portfolio.Pages[index].Target.ID {
			t.Fatalf("standalone target %d binding is invalid", index)
		}
		gotLanguages = append(gotLanguages, payload.Target.Language)
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
	if err := verifyExactStandaloneTargetBundleProjection(htmlPath, validated); err != nil {
		t.Fatalf("verify exact neutral projection: %v", err)
	}
}

func TestWriteStandaloneProgramPageBundleAllowsOneAnalyzedPage(t *testing.T) {
	portfolio, ready, _ := standaloneProgramPageBundleFixture(t)
	binding := portfolio.Pages[0]
	singlePortfolio, err := programpage.Build(
		binding.Target.ID,
		[]programpage.Page{binding},
	)
	if err != nil {
		t.Fatal(err)
	}
	var singleReady []PreparedStandaloneTarget
	for _, candidate := range ready {
		if candidate.prepared != nil &&
			candidate.prepared.programPage.ProgramTarget.ID == binding.Target.ID {
			singleReady = append(singleReady, candidate)
		}
	}
	if len(singleReady) != 1 {
		t.Fatalf("prepared single page count = %d", len(singleReady))
	}
	selected := singleReady[0].prepared
	selected.repository.Targets = []BrowserTargetIndexItem{{
		SelectedTargetID: selected.selectedTargetID,
		ProgramTargetID:  selected.target.Target.ID,
		Language:         selected.target.Target.Language,
		Kind:             selected.target.Target.Kind,
		DisplayName:      selected.target.Target.Name,
		State:            "analyzed",
		Href:             "#/program",
	}}
	selected.repository.LogicalDefaultSelectedTargetID = selected.selectedTargetID
	selected.repositoryPayload, err = EncodeBrowserRepositoryPayload(selected.repository)
	if err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(t.TempDir(), binding.RunID)
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteStandaloneProgramPageBundleAtomic(runDir, singlePortfolio, singleReady); err != nil {
		t.Fatalf("WriteStandaloneProgramPageBundleAtomic: %v", err)
	}
	identity, found, err := InspectStandaloneTargetBundleHTML(filepath.Join(runDir, "report.html"))
	if err != nil || !found {
		t.Fatalf("InspectStandaloneTargetBundleHTML = %#v, %t, %v", identity, found, err)
	}
	if identity.ProgramPagePortfolioSHA256 != singlePortfolio.SHA256 ||
		identity.TargetCount != 1 || identity.DefaultTargetIndex != 0 {
		t.Fatalf("one analyzed-page prepared bundle identity = %#v", identity)
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
	tampered := bytes.Replace(raw, []byte("H4sI"), []byte("I4sI"), 1)
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
	if err := verifyExactStandaloneTargetBundleProjection(htmlPath, validated); err == nil ||
		!strings.Contains(err.Error(), "manifest-derived projection") {
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
	neutral.TargetCount = 1
	if !validStandaloneTargetBundleIdentity(neutral) {
		t.Fatal("one analyzed-page neutral standalone authority is invalid")
	}
	legacy.TargetCount = 1
	if validStandaloneTargetBundleIdentity(legacy) {
		t.Fatal("one-page legacy standalone authority became valid")
	}
	mixed := legacy
	mixed.TargetCount = 2
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
	for index, programIndex := range indexes {
		pages = append(pages, programpage.Page{
			Target: programIndex.Target,
			RunID:  fmt.Sprintf("program-page-%d", index),
		})
	}
	portfolio, err := programpage.Build(indexes[1].Target.ID, pages)
	if err != nil {
		t.Fatal(err)
	}
	ready := make([]PreparedStandaloneTarget, 0, len(portfolio.Pages))
	for _, binding := range portfolio.Pages {
		targetPayload := BrowserTargetPayload{
			Version: BrowserTargetPayloadVersion,
			Target: BrowserTarget{
				ID: binding.Target.ID, Language: binding.Target.Language, Kind: binding.Target.Kind,
				Name: binding.Target.Name, Selector: binding.Target.Selector,
			},
			OpenablePaths: []string{},
			Features: BrowserTargetFeatures{Program: BrowserProgramFeature{
				Objects: []BrowserProgramObject{}, Relations: []BrowserProgramRelation{},
			}},
		}
		if binding.Target.Language == "javascript" || binding.Target.Language == "typescript" {
			targetPayload.Features.Surfaces = &BrowserSurfaceFeature{
				Facts: []BrowserJSTSFact{}, Surfaces: []BrowserSurface{},
			}
			targetPayload.Features.CrossSurfacePaths = &BrowserCrossSurfacePathFeature{
				Facts: []BrowserJSTSFact{}, Paths: []BrowserCrossSurfacePath{},
			}
		}
		targetRaw, err := EncodeBrowserTargetPayload(targetPayload)
		if err != nil {
			t.Fatal(err)
		}
		ready = append(ready, PreparedStandaloneTarget{prepared: &preparedStandaloneTarget{
			programPage: TargetNavigationPage{
				RunID: binding.RunID, ProgramTarget: binding.Target.Snapshot(),
				ArtifactFilename: programindex.ArtifactFilename,
			},
			target: targetPayload, targetPayload: targetRaw, selectedTargetID: binding.Target.ID,
			host: "GitHub", repositoryURL: "https://github.com/example/neutral-bundle",
			revision: standaloneBundleRevision, repoName: "neutral-bundle-fixture",
			localRoots: []string{filepath.Join(t.TempDir(), "private")},
		}})
	}
	repository := BrowserRepositoryPayload{
		Version:                        BrowserRepositoryPayloadVersion,
		Repository:                     BrowserRepository{Name: "neutral-bundle-fixture", CapturedRevision: standaloneBundleRevision},
		Source:                         BrowserSource{Kind: "github", RepositoryURL: "https://github.com/example/neutral-bundle"},
		LogicalDefaultSelectedTargetID: portfolio.DefaultTargetID,
		OpenablePaths:                  []string{},
	}
	for index, binding := range portfolio.Pages {
		repository.Targets = append(repository.Targets, BrowserTargetIndexItem{
			SelectedTargetID: binding.Target.ID, ProgramTargetID: binding.Target.ID,
			Language: binding.Target.Language, Kind: binding.Target.Kind, DisplayName: binding.Target.Name,
			State: "analyzed", Href: fmt.Sprintf("?target=%d#/program", index),
		})
	}
	repositoryRaw, err := EncodeBrowserRepositoryPayload(repository)
	if err != nil {
		t.Fatal(err)
	}
	for index := range ready {
		ready[index].prepared.repository = repository
		ready[index].prepared.repositoryPayload = repositoryRaw
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

func directoryTreePaths(t *testing.T, directory string) []string {
	t.Helper()
	paths := make([]string, 0)
	var visit func(string)
	visit = func(relative string) {
		entries, err := os.ReadDir(filepath.Join(directory, relative))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			path := filepath.Join(relative, entry.Name())
			paths = append(paths, filepath.ToSlash(path))
			if entry.IsDir() {
				visit(path)
			}
		}
	}
	visit("")
	slices.Sort(paths)
	return paths
}

func artifactProgramPageBundleFixture(
	t *testing.T,
	targetCount int,
) (programpage.Portfolio, string, []string) {
	t.Helper()
	indexes := []programindex.Index{
		reportProgramIndexFixture(t, "python", "executable"),
		reportProgramIndexFixture(t, "python", "library"),
	}
	if targetCount < 1 || targetCount > len(indexes) {
		t.Fatalf("artifact program-page target count = %d", targetCount)
	}
	indexes = indexes[:targetCount]
	pages := make([]programpage.Page, 0, len(indexes))
	indexByTargetID := make(map[string]programindex.Index, len(indexes))
	for index, programIndex := range indexes {
		pages = append(pages, programpage.Page{
			Target: programIndex.Target,
			RunID:  fmt.Sprintf("artifact-page-%d", index),
		})
		indexByTargetID[programIndex.Target.ID] = programIndex
	}
	portfolio, err := programpage.Build(indexes[len(indexes)-1].Target.ID, pages)
	if err != nil {
		t.Fatal(err)
	}
	portfolioRaw, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	runtimeResult := reportRuntimePortfolioFixture(
		t, portfolio.SHA256, runtimeTargetsForProgramPages(portfolio),
	)
	runtimeRaw, err := runtimeportfolio.Encode(runtimeResult)
	if err != nil {
		t.Fatal(err)
	}
	runtimeView, err := NewRuntimePortfolioView(runtimeResult)
	if err != nil {
		t.Fatal(err)
	}
	targetOutcomes := make([]targetoutcome.Outcome, 0, len(portfolio.Pages))
	defaultSelectedTargetID := ""
	for _, binding := range portfolio.Pages {
		language := targetoutcome.LanguageGroupPython
		scope := targetoutcome.ScopeLibrary
		switch {
		case strings.Contains(binding.Target.Kind, "executable"):
			scope = targetoutcome.ScopeExecutable
		}
		selected, buildErr := targetoutcome.NewSelectedTarget(
			language, scope,
			binding.Target.Name+" "+binding.Target.Kind,
			binding.Target.Selector+":"+binding.Target.Kind,
		)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		outcome, buildErr := targetoutcome.NewAnalyzed(selected, binding.Target, binding.RunID)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		targetOutcomes = append(targetOutcomes, outcome)
		if binding.Target.ID == portfolio.DefaultTargetID {
			defaultSelectedTargetID = selected.ID
		}
	}
	if targetCount == 1 {
		failedSelected, buildErr := targetoutcome.NewSelectedTarget(
			targetoutcome.LanguageGroupPython,
			targetoutcome.ScopeLibrary,
			"failed sibling",
			"python:failed-sibling",
		)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		failedOutcome, buildErr := targetoutcome.NewNotAnalyzed(
			failedSelected,
			targetoutcome.StageProgramAnalysis,
			targetoutcome.ReasonSourceNotAnalyzable,
		)
		if buildErr != nil {
			t.Fatal(buildErr)
		}
		targetOutcomes = append(targetOutcomes, failedOutcome)
		defaultSelectedTargetID = failedSelected.ID
	}
	outcomePortfolio, err := targetoutcome.Build(defaultSelectedTargetID, targetOutcomes)
	if err != nil {
		t.Fatal(err)
	}
	outcomeRaw, err := outcomePortfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	outcomeView, err := NewTargetOutcomePortfolioView(outcomePortfolio, portfolio)
	if err != nil {
		t.Fatal(err)
	}

	runsDir := t.TempDir()
	runDirs := make([]string, 0, len(portfolio.Pages))
	ownerRunDir := ""
	for index, binding := range portfolio.Pages {
		runDir := filepath.Join(runsDir, binding.RunID)
		if err := os.Mkdir(runDir, 0o700); err != nil {
			t.Fatal(err)
		}
		runDirs = append(runDirs, runDir)
		if binding.Target.ID == portfolio.DefaultTargetID {
			ownerRunDir = runDir
		}
		writeTargetPageManifestArtifact(
			t, runDir, programpage.ArtifactFilename, portfolioRaw,
		)
		writeTargetPageManifestArtifact(
			t, runDir, runtimeportfolio.ArtifactFilename, runtimeRaw,
		)
		writeTargetPageManifestArtifact(
			t, runDir, targetoutcome.ArtifactFilename, outcomeRaw,
		)

		programIndex := indexByTargetID[binding.Target.ID]
		indexRaw, err := programindex.Encode(programIndex)
		if err != nil {
			t.Fatal(err)
		}
		writeTargetPageManifestArtifact(
			t, runDir, programindex.ArtifactFilename, indexRaw,
		)
		set, err := programindex.NewArtifactSet(
			programIndex.Target.ID,
			[]programindex.ArtifactSetEntry{{
				TargetID:    programIndex.Target.ID,
				Filename:    programindex.ArtifactFilename,
				IndexSHA256: programIndex.SHA256,
			}},
		)
		if err != nil {
			t.Fatal(err)
		}
		setRaw, err := programindex.EncodeArtifactSet(set)
		if err != nil {
			t.Fatal(err)
		}
		writeTargetPageManifestArtifact(
			t, runDir, programindex.ArtifactSetFilename, setRaw,
		)
		writeReportProgramIndexArtifacts(t, runDir, programIndex)
		_, _, pythonTargetRaw, declarationsRaw := reportPythonDeclarationArtifactsFixture(t, programIndex)
		activityView, activityRaw := reportActivityEntrypointFixture(t, programIndex)
		coreView, coreRaw := reportCoreMapFixture(t, programIndex)
		integrationView, catalogRaw, selectedRaw, usageRaw := reportIntegrationUsageFixture(t, programIndex)
		activityPathView, activityPathRaw := reportActivityPathFixture(
			t, programIndex, activityRaw, selectedRaw, usageRaw,
		)

		programPortfolio, err := NewProgramPortfolio(
			programIndex.Target.ID, []programindex.Index{programIndex},
		)
		if err != nil {
			t.Fatal(err)
		}
		manifest := validRunManifestFixture(t)
		manifest.OpenablePaths = []string{
			"app/__init__.py", "app/main.py", "batch.go", "cmd/app/main.go",
			"scripts/clean.py", "storage/__init__.py", "storage/db.py",
		}
		snapshotRaw := []byte(`{"repo_name":"artifact-bundle-fixture"}`)
		writeTargetPageManifestArtifact(t, runDir, "snapshot.json", snapshotRaw)
		manifest.SnapshotSHA256 = manifestSHA256(snapshotRaw)
		manifest.MaterialInputs.ProgramTargetID,
			manifest.MaterialInputs.ProgramTargetSHA256,
			err = reportProgramTargetMaterial(&binding.Target)
		if err != nil {
			t.Fatal(err)
		}
		manifest.MaterialInputs.ProgramIndexSetSHA256 = manifestSHA256(setRaw)
		manifest.MaterialInputs.PythonTargetCatalogSHA256 = manifestSHA256(pythonTargetRaw)
		manifest.MaterialInputs.DeclaredDependenciesSHA256 = manifestSHA256(declarationsRaw)
		manifest.MaterialInputs.CoreMapSHA256 = manifestSHA256(coreRaw)
		manifest.MaterialInputs.ActivityEntrypointsSHA256 = manifestSHA256(activityRaw)
		manifest.MaterialInputs.DependencyCatalogSHA256 = manifestSHA256(catalogRaw)
		manifest.MaterialInputs.IntegrationDependenciesSHA256 = manifestSHA256(selectedRaw)
		manifest.MaterialInputs.IntegrationUsageSHA256 = manifestSHA256(usageRaw)
		manifest.MaterialInputs.ActivityPathsSHA256 = manifestSHA256(activityPathRaw)
		manifest.MaterialInputs.ProgramPagePortfolioSHA256 = manifestSHA256(portfolioRaw)
		manifest.MaterialInputs.RuntimePortfolioSHA256 = manifestSHA256(runtimeRaw)
		manifest.MaterialInputs.TargetOutcomePortfolioSHA256 = manifestSHA256(outcomeRaw)
		reportRaw, err := json.Marshal(ReportData{
			FormatVersion:          CurrentFormatVersion,
			RepoName:               "artifact-bundle-fixture",
			CapturedRevision:       manifest.RepositoryState.Head,
			CapturedInputCount:     len(manifest.CapturedInputs),
			OpenablePaths:          append([]string(nil), manifest.OpenablePaths...),
			ProgramPortfolio:       programPortfolio,
			CoreMapView:            coreView,
			ActivityEntrypointView: activityView,
			IntegrationUsageView:   integrationView,
			ActivityPathView:       activityPathView,
			RuntimePortfolio:       runtimeView,
			TargetOutcomePortfolio: outcomeView,
			Warnings:               []string{fmt.Sprintf("BACKING_REPORT_%d", index)},
		})
		if err != nil {
			t.Fatal(err)
		}
		manifest.ReportSHA256 = manifestSHA256(reportRaw)
		writeTargetPageManifestArtifact(t, runDir, "report.json", reportRaw)
		if err := writeRunManifestAtomic(runDir, manifest); err != nil {
			t.Fatalf("write backing manifest %d: %v", index, err)
		}
	}
	if ownerRunDir == "" {
		t.Fatal("fixture default run is absent")
	}
	return portfolio, ownerRunDir, runDirs
}
