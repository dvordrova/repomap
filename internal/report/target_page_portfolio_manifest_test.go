package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/snapshot"
)

func TestRunManifestVerifiesTargetPagePortfolioArtifacts(t *testing.T) {
	fixture := newTargetPageManifestFixture(t)

	t.Run("container only is valid", func(t *testing.T) {
		runDir, manifest := fixture.run(t, false)
		if err := manifest.Validate(); err != nil {
			t.Fatalf("container-only manifest: %v", err)
		}
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err != nil {
			t.Fatalf("VerifyTargetPagePortfolioArtifacts: %v", err)
		}
	})

	t.Run("portfolio binds exact current target page", func(t *testing.T) {
		runDir, manifest := fixture.run(t, true)
		if err := manifest.Validate(); err != nil {
			t.Fatalf("portfolio-bound manifest: %v", err)
		}
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err != nil {
			t.Fatalf("VerifyTargetPagePortfolioArtifacts: %v", err)
		}
	})

	t.Run("manifest digest rejects changed portfolio bytes", func(t *testing.T) {
		runDir, manifest := fixture.run(t, true)
		changed := append(append([]byte(nil), fixture.portfolioRaw...), '\n')
		writeTargetPageManifestArtifact(t, runDir, snapshot.TargetPagePortfolioArtifactFilename, changed)
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("changed portfolio error = %v", err)
		}
	})

	t.Run("self seal rejects tamper even when manifest digest follows bytes", func(t *testing.T) {
		runDir, manifest := fixture.run(t, true)
		tampered := bytes.Replace(fixture.portfolioRaw, []byte("run-app-1"), []byte("run-app-2"), 1)
		writeTargetPageManifestArtifact(t, runDir, snapshot.TargetPagePortfolioArtifactFilename, tampered)
		manifest.MaterialInputs.TargetPagePortfolioSHA256 = manifestSHA256(tampered)
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "seal binding mismatch") {
			t.Fatalf("resealed-manifest tamper error = %v", err)
		}
	})

	t.Run("missing bound portfolio is rejected", func(t *testing.T) {
		runDir, manifest := fixture.run(t, true)
		if err := os.Remove(filepath.Join(runDir, snapshot.TargetPagePortfolioArtifactFilename)); err != nil {
			t.Fatal(err)
		}
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "sha256 mismatch") {
			t.Fatalf("missing portfolio error = %v", err)
		}
	})

	t.Run("present unbound portfolio is rejected", func(t *testing.T) {
		runDir, manifest := fixture.run(t, true)
		manifest.MaterialInputs.TargetPagePortfolioSHA256 = ""
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "unbound target page artifact") {
			t.Fatalf("unbound portfolio error = %v", err)
		}
	})

	t.Run("present unbound container is rejected", func(t *testing.T) {
		runDir, manifest := fixture.run(t, false)
		manifest.MaterialInputs.TargetRunContainerSHA256 = ""
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "unbound target page artifact") {
			t.Fatalf("unbound container error = %v", err)
		}
	})

	t.Run("portfolio field without container field is invalid", func(t *testing.T) {
		_, manifest := fixture.run(t, true)
		manifest.MaterialInputs.TargetRunContainerSHA256 = ""
		if err := manifest.Validate(); err == nil ||
			!strings.Contains(err.Error(), "portfolio binding is invalid") {
			t.Fatalf("portfolio without container manifest error = %v", err)
		}
	})

	t.Run("portfolio for another container is rejected", func(t *testing.T) {
		runDir, manifest := fixture.run(t, true)
		writeTargetPageManifestArtifact(
			t, runDir, snapshot.TargetRunContainerArtifactFilename, fixture.singleContainerRaw,
		)
		manifest.MaterialInputs.TargetRunContainerSHA256 = manifestSHA256(fixture.singleContainerRaw)
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "container binding mismatch") {
			t.Fatalf("cross-container portfolio error = %v", err)
		}
	})

	t.Run("current manifest target must own this exact run", func(t *testing.T) {
		runDir, manifest := fixture.run(t, true)
		manifest.MaterialInputs.AnalysisTargetRef = fixture.helperRef
		manifest.MaterialInputs.AnalysisTargetSHA256 = fixture.targetSHA256(t, fixture.helperRef)
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "no exact published target page") {
			t.Fatalf("other sibling run error = %v", err)
		}

		manifest.MaterialInputs.AnalysisTargetRef = "at-000000000000000000000000"
		manifest.MaterialInputs.AnalysisTargetSHA256 = strings.Repeat("0", 64)
		if err := manifest.VerifyTargetPagePortfolioArtifacts(runDir); err == nil ||
			!strings.Contains(err.Error(), "no exact published target page") {
			t.Fatalf("unselected current target error = %v", err)
		}
	})
}

type targetPageManifestFixture struct {
	container          snapshot.TargetRunContainer
	containerRaw       []byte
	singleContainerRaw []byte
	portfolioRaw       []byte
	appRef             string
	helperRef          string
}

func newTargetPageManifestFixture(t *testing.T) targetPageManifestFixture {
	t.Helper()
	const modulePath = "example.com/target-pages"
	entrypoint := func(dir string, line int) gofacts.Entrypoint {
		return gofacts.Entrypoint{
			ModulePath: modulePath, ImportPath: modulePath + "/" + dir,
			Dir: dir, PackageDir: dir, ModuleRelativeDir: dir, ModuleDir: ".",
			Kind: "cli", GoFiles: []string{"main.go"},
			Anchors: []gofacts.EntrypointAnchor{{
				Version: gofacts.EntrypointAnchorVersion, Kind: gofacts.EntrypointAnchorGoMain,
				Path: dir + "/main.go", Line: line,
			}},
		}
	}
	facts := gofacts.Facts{
		Modules: []gofacts.ModuleFact{{
			ID: "module-root", ModulePath: modulePath, ModuleDir: ".", GoMod: "go.mod",
			Main: true, DisplayName: "target-pages", PackagesCount: 2, RetainedPackagesCount: 2,
			Coverage: gofacts.ModuleCoverage{
				State: gofacts.CoverageComplete, PackagesDiscovered: 2, PackagesRetained: 2,
			},
		}},
		Packages: []gofacts.PackageFact{
			{
				CanonicalPath: modulePath + "/cmd/app", Name: "main", ModuleID: "module-root",
				ModulePath: modulePath, PackageDir: "cmd/app", ModuleRelativeDir: "cmd/app",
				DisplayPath: "cmd/app", Locality: "local", Files: []string{"cmd/app/main.go"},
			},
			{
				CanonicalPath: modulePath + "/cmd/helper", Name: "main", ModuleID: "module-root",
				ModulePath: modulePath, PackageDir: "cmd/helper", ModuleRelativeDir: "cmd/helper",
				DisplayPath: "cmd/helper", Locality: "local", Files: []string{"cmd/helper/main.go"},
			},
		},
		EntrypointPackages: []gofacts.Entrypoint{entrypoint("cmd/app", 10), entrypoint("cmd/helper", 12)},
		Coverage: gofacts.Coverage{
			State: gofacts.CoverageComplete, ModulesDiscovered: 1, ModulesAvailable: 1,
			PackagesDiscovered: 2, PackagesRetained: 2,
		},
	}
	facts.Modules[0].EntrypointPackages = append([]gofacts.Entrypoint(nil), facts.EntrypointPackages...)
	catalog, err := analysistarget.BuildCatalog(facts)
	if err != nil {
		t.Fatal(err)
	}
	refByDir := make(map[string]string, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		refByDir[entry.DisplayPath] = entry.Candidate.Target.Ref
	}
	appRef, helperRef := refByDir["cmd/app"], refByDir["cmd/helper"]
	if appRef == "" || helperRef == "" {
		t.Fatalf("target catalog refs = %#v", refByDir)
	}
	source := snapshot.Snapshot{
		RepoName: "target-pages", GoFacts: &facts, TargetCatalog: &catalog,
		FilteredFiles: []string{"cmd/app/main.go", "cmd/helper/main.go", "go.mod"},
	}
	container, err := snapshot.BuildTargetRunContainer(source, snapshot.TargetRunSelection{
		DefaultTargetRef: appRef, TargetRefs: []string{helperRef, appRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	containerRaw, err := container.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	singleContainer, err := snapshot.BuildTargetRunContainer(source, snapshot.TargetRunSelection{
		DefaultTargetRef: appRef, TargetRefs: []string{appRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	singleContainerRaw, err := singleContainer.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, []snapshot.TargetPageOutcome{
		{TargetRef: appRef, RunID: "run-app-1"},
		{TargetRef: helperRef, RunID: "run-helper-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	portfolioRaw, err := portfolio.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return targetPageManifestFixture{
		container: container, containerRaw: containerRaw, singleContainerRaw: singleContainerRaw,
		portfolioRaw: portfolioRaw, appRef: appRef, helperRef: helperRef,
	}
}

func (fixture targetPageManifestFixture) run(t *testing.T, withPortfolio bool) (string, RunManifest) {
	t.Helper()
	runDir := filepath.Join(t.TempDir(), "run-app-1")
	if err := os.Mkdir(runDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeTargetPageManifestArtifact(
		t, runDir, snapshot.TargetRunContainerArtifactFilename, fixture.containerRaw,
	)
	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.AnalysisTargetRef = fixture.appRef
	manifest.MaterialInputs.AnalysisTargetSHA256 = fixture.targetSHA256(t, fixture.appRef)
	manifest.MaterialInputs.TargetRunContainerSHA256 = manifestSHA256(fixture.containerRaw)
	if withPortfolio {
		writeTargetPageManifestArtifact(
			t, runDir, snapshot.TargetPagePortfolioArtifactFilename, fixture.portfolioRaw,
		)
		manifest.MaterialInputs.TargetPagePortfolioSHA256 = manifestSHA256(fixture.portfolioRaw)
	}
	return runDir, manifest
}

func (fixture targetPageManifestFixture) targetSHA256(t *testing.T, targetRef string) string {
	t.Helper()
	for _, projection := range fixture.container.Targets {
		if projection.Target.Ref != targetRef {
			continue
		}
		canonical, err := projection.Target.CanonicalJSON()
		if err != nil {
			t.Fatal(err)
		}
		return manifestSHA256(canonical)
	}
	t.Fatalf("target ref is absent from fixture container")
	return ""
}

func writeTargetPageManifestArtifact(t *testing.T, runDir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(runDir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
