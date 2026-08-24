package report

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/activityentrypoint"
	"github.com/dvordrova/repomap/internal/activitypath"
	"github.com/dvordrova/repomap/internal/coremap"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/dependencydeclaration"
	"github.com/dvordrova/repomap/internal/integrationdependency"
	"github.com/dvordrova/repomap/internal/integrationusage"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/pythontarget"
)

func TestCoreMapViewKeepsModelGroupingSeparateFromExactMemberEvidence(t *testing.T) {
	index := reportProgramIndexFixture(t, "python", "executable")
	view, artifact := reportCoreMapFixture(t, index)
	if view.ProgramTargetID != index.Target.ID || view.ProgramIndexSHA256 != index.SHA256 ||
		!validCubeMapViewSHA256(view.IntegrationUsageSHA256) ||
		len(view.RefinedCore) != 1 || view.RefinedCore[0].Name != "Execution core" {
		t.Fatalf("core map identity/grouping = %#v", view)
	}
	representatives := view.RefinedCore[0].RepresentativeSymbols
	if len(representatives) != 1 || representatives[0].Kind != "function" ||
		representatives[0].Visibility != "public" || representatives[0].Symbol.Name != "start" ||
		representatives[0].Symbol.Location.Path != "app/main.py" {
		t.Fatalf("exact member evidence = %#v", representatives)
	}
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, legacy := range [][]byte{[]byte(`"direct_call_state"`), []byte(`"direct_call_coverage"`)} {
		if bytes.Contains(encoded, legacy) {
			t.Fatalf("CoreMapView leaked legacy Go coverage %s", legacy)
		}
	}
	if bytes.Contains(encoded, []byte(`"exported"`)) || !bytes.Contains(encoded, []byte(`"visibility":"public"`)) {
		t.Fatalf("CoreMapView did not preserve closed ProgramIndex visibility: %s", encoded)
	}
	tampered, err := coremap.Decode(artifact)
	if err != nil {
		t.Fatal(err)
	}
	tampered.Refined[0].Symbols[0].Symbol.Name = "invented-name"
	if _, err := coremap.Encode(tampered); err != nil {
		t.Fatalf("tampered producer shape should remain internally decodable: %v", err)
	}
	if _, err := NewCoreMapView(tampered, index, nil); err == nil ||
		!strings.Contains(err.Error(), "differs from exact ProgramIndex evidence") {
		t.Fatalf("invented exact member error = %v", err)
	}
}

func TestCoreMapViewRetainsProgramRelationFrontier(t *testing.T) {
	index := reportProgramIndexFixtureWithRelationOmissions(t, "go", "executable_package", 1)
	view, _ := reportCoreMapFixture(t, index)
	if view.Coverage.ProgramRelationsOmitted != 1 {
		t.Fatalf("relation frontier = %#v", view.Coverage)
	}
	if err := view.Validate(); err != nil {
		t.Fatalf("retained relation frontier invalidated view: %v", err)
	}
}

func TestCoreMapViewAllowsOneBaselineChild(t *testing.T) {
	child := CoreMapViewBlock{
		ID: "child", Name: "Child", Purpose: "Exact child evidence.",
		Files:                 []CoreMapViewFile{{FileRef: "f-child", Path: "child.py"}},
		RepresentativeSymbols: []CoreMapViewRepresentativeSymbol{}, Children: []CoreMapViewBlock{},
	}
	parent := CoreMapViewBlock{
		ID: "parent", Name: "Parent", Purpose: "Exact parent evidence.",
		Files: []CoreMapViewFile{}, RepresentativeSymbols: []CoreMapViewRepresentativeSymbol{},
		Children: []CoreMapViewBlock{child},
	}
	if err := validateCoreMapViewBlocks(
		[]CoreMapViewBlock{parent}, true, 0, make(map[string]struct{}),
		make(map[string]string), make(map[string]string),
		make(map[string]CoreMapViewRepresentativeSymbol), &coreMapViewCounts{},
	); err != nil {
		t.Fatalf("one producer-supported baseline child: %v", err)
	}
}

func TestCoreMapViewGroupsRequireCompleteRefinedPartition(t *testing.T) {
	refined := map[string]struct{}{"core-a": {}, "core-b": {}, "core-c": {}}
	valid := []CoreMapViewGroup{
		{ID: "group-runtime", Name: "Runtime", Purpose: "Coordinates accepted work.", CoreBlockIDs: []string{"core-a", "core-b"}},
		{ID: "group-storage", Name: "Storage", Purpose: "Persists accepted state.", CoreBlockIDs: []string{"core-c"}},
	}
	if err := validateCoreMapViewGroups(valid, refined); err != nil {
		t.Fatal(err)
	}
	invalid := append([]CoreMapViewGroup(nil), valid...)
	invalid[0].CoreBlockIDs = []string{"core-a"}
	if err := validateCoreMapViewGroups(invalid, refined); err == nil {
		t.Fatal("partial refined group partition was accepted")
	}
	invalid = append([]CoreMapViewGroup(nil), valid...)
	invalid[1].CoreBlockIDs = []string{"core-b", "core-c"}
	if err := validateCoreMapViewGroups(invalid, refined); err == nil {
		t.Fatal("duplicate refined group membership was accepted")
	}
}

func TestReadRunDirDoesNotFallbackWhenPythonCoreMapIsMissing(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "python", "executable")
	writeReportProgramIndexArtifacts(t, runDir, index)
	if err := os.Remove(filepath.Join(runDir, coremap.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"python-fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"python-fixture"}`))

	if _, err := ReadRunDir(runDir); err == nil ||
		!strings.Contains(err.Error(), "requires exact core map, activity entrypoint, integration usage, and activity path authority and no legacy CubeMap authority") {
		t.Fatalf("missing Python CoreMap error = %v", err)
	}
}

func TestVerifyCoreMapProjectionBindsIntegrationUsageAuthority(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "python", "executable")
	writeReportProgramIndexArtifacts(t, runDir, index)
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"python-fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"python-fixture"}`))

	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	reportJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	readArtifact := func(name string) []byte {
		t.Helper()
		raw, readErr := os.ReadFile(filepath.Join(runDir, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		return raw
	}
	coreRaw := readArtifact(coremap.ArtifactFilename)
	usageRaw := readArtifact(integrationusage.ArtifactFilename)
	usageSHA256 := manifestSHA256(usageRaw)
	if data.CoreMapView == nil || data.CoreMapView.IntegrationUsageSHA256 != usageSHA256 {
		t.Fatalf("CoreMap view integration usage authority = %#v, want %q", data.CoreMapView, usageSHA256)
	}

	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&index.Target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramIndexSetSHA256 = manifestSHA256(readArtifact(programindex.ArtifactSetFilename))
	manifest.MaterialInputs.CoreMapSHA256 = manifestSHA256(coreRaw)
	manifest.MaterialInputs.ActivityEntrypointsSHA256 = manifestSHA256(readArtifact(activityentrypoint.ArtifactFilename))
	manifest.MaterialInputs.PythonTargetCatalogSHA256 = manifestSHA256(readArtifact(pythontarget.ArtifactFilename))
	manifest.MaterialInputs.DeclaredDependenciesSHA256 = manifestSHA256(readArtifact(dependencydeclaration.ArtifactFilename))
	manifest.MaterialInputs.DependencyCatalogSHA256 = manifestSHA256(readArtifact(dependencies.ArtifactFilename))
	manifest.MaterialInputs.IntegrationDependenciesSHA256 = manifestSHA256(readArtifact(integrationdependency.ArtifactFilename))
	manifest.MaterialInputs.IntegrationUsageSHA256 = usageSHA256
	manifest.MaterialInputs.ActivityPathsSHA256 = manifestSHA256(readArtifact(activitypath.ArtifactFilename))
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err != nil {
		t.Fatalf("valid authority: %v", err)
	}
	tamperedData := *data
	tamperedView := *data.CoreMapView
	tamperedView.IntegrationUsageSHA256 = strings.Repeat("e", 64)
	tamperedData.CoreMapView = &tamperedView
	tamperedReportJSON, err := json.Marshal(&tamperedData)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyCoreMapProjection(runDir, tamperedReportJSON); err == nil ||
		!strings.Contains(err.Error(), "core map view integration usage authority mismatch") {
		t.Fatalf("foreign CoreMap view integration usage authority error = %v", err)
	}

	decodedCore, err := coremap.Decode(coreRaw)
	if err != nil {
		t.Fatal(err)
	}
	for name, integrationUsageSHA256 := range map[string]string{
		"missing": "",
		"foreign": strings.Repeat("f", 64),
	} {
		t.Run(name, func(t *testing.T) {
			tampered := decodedCore
			tampered.IntegrationUsageSHA256 = integrationUsageSHA256
			tamperedRaw, encodeErr := coremap.Encode(tampered)
			if encodeErr != nil {
				t.Fatal(encodeErr)
			}
			writeReportProgramFile(t, filepath.Join(runDir, coremap.ArtifactFilename), tamperedRaw)
			bound := manifest
			bound.MaterialInputs.CoreMapSHA256 = manifestSHA256(tamperedRaw)
			if verifyErr := bound.VerifyCoreMapProjection(runDir, reportJSON); verifyErr == nil ||
				!strings.Contains(verifyErr.Error(), "core map integration usage authority mismatch") {
				t.Fatalf("CoreMap integration usage authority error = %v", verifyErr)
			}
		})
	}
}

func TestVerifyCoreMapProjectionRejectsMissingUnboundAndChangedArtifacts(t *testing.T) {
	runDir := t.TempDir()
	index := reportProgramIndexFixture(t, "python", "executable")
	writeReportProgramIndexArtifacts(t, runDir, index)
	dependencyRaw, integrationRaw, usageRaw := writeReportPythonSemanticArtifacts(t, runDir, index)
	writeReportProgramFile(t, filepath.Join(runDir, "snapshot.json"), []byte(`{"repo_name":"python-fixture"}`))
	writeReportProgramFile(t, filepath.Join(runDir, "metadata.json"), []byte(`{"repo_name":"python-fixture"}`))
	data, err := ReadRunDir(runDir)
	if err != nil {
		t.Fatal(err)
	}
	reportJSON, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	coreRaw, err := os.ReadFile(filepath.Join(runDir, coremap.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	activityRaw, err := os.ReadFile(filepath.Join(runDir, activityentrypoint.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	targetRaw, err := os.ReadFile(filepath.Join(runDir, pythontarget.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	declarationRaw, err := os.ReadFile(filepath.Join(runDir, dependencydeclaration.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	activityPathRaw, err := os.ReadFile(filepath.Join(runDir, activitypath.ArtifactFilename))
	if err != nil {
		t.Fatal(err)
	}
	manifest := validRunManifestFixture(t)
	manifest.MaterialInputs.ProgramTargetID, manifest.MaterialInputs.ProgramTargetSHA256, err =
		reportProgramTargetMaterial(&index.Target)
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.CoreMapSHA256 = manifestSHA256(coreRaw)
	manifest.MaterialInputs.ActivityEntrypointsSHA256 = manifestSHA256(activityRaw)
	manifest.MaterialInputs.PythonTargetCatalogSHA256 = manifestSHA256(targetRaw)
	manifest.MaterialInputs.DeclaredDependenciesSHA256 = manifestSHA256(declarationRaw)
	manifest.MaterialInputs.DependencyCatalogSHA256 = manifestSHA256(dependencyRaw)
	manifest.MaterialInputs.IntegrationDependenciesSHA256 = manifestSHA256(integrationRaw)
	manifest.MaterialInputs.IntegrationUsageSHA256 = manifestSHA256(usageRaw)
	manifest.MaterialInputs.ActivityPathsSHA256 = manifestSHA256(activityPathRaw)
	setRaw, err := os.ReadFile(filepath.Join(runDir, "program-index-set.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest.MaterialInputs.ProgramIndexSetSHA256 = manifestSHA256(setRaw)
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err != nil {
		t.Fatalf("VerifyCoreMapProjection: %v", err)
	}
	if err := os.Remove(filepath.Join(runDir, dependencydeclaration.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "bound declared dependency artifact is missing") {
		t.Fatalf("missing declared dependency authority error = %v", err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, dependencydeclaration.ArtifactFilename), declarationRaw)
	writeReportProgramFile(t, filepath.Join(runDir, pythontarget.ArtifactFilename), append(targetRaw, '\n'))
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "Python target catalog sha256 mismatch") {
		t.Fatalf("changed Python target authority error = %v", err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, pythontarget.ArtifactFilename), targetRaw)
	partial := manifest
	partial.MaterialInputs.IntegrationUsageSHA256 = ""
	if err := partial.Validate(); err == nil || !strings.Contains(err.Error(), "must be bound together") {
		t.Fatalf("partial Python semantic binding error = %v", err)
	}
	if err := os.Remove(filepath.Join(runDir, integrationusage.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "bound Python semantic artifact is missing") {
		t.Fatalf("missing integration usage error = %v", err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, integrationusage.ArtifactFilename), usageRaw)

	tamperedUsage, err := integrationusage.Decode(usageRaw)
	if err != nil {
		t.Fatal(err)
	}
	tamperedUsage.ProgramIndexSHA256 = strings.Repeat("f", 64)
	tamperedUsageRaw, err := integrationusage.Encode(tamperedUsage)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, integrationusage.ArtifactFilename), tamperedUsageRaw)
	wrongUsage := manifest
	wrongUsage.MaterialInputs.IntegrationUsageSHA256 = manifestSHA256(tamperedUsageRaw)
	if err := wrongUsage.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "core map integration usage authority mismatch") {
		t.Fatalf("cross-run integration usage authority error = %v", err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, integrationusage.ArtifactFilename), usageRaw)

	importer, err := dependencies.SealImporter(dependencies.Importer{
		Language: "python", Name: "app.main", ModulePath: "python-fixture",
		PackagePath: "app.main", RepositoryPath: "app/main.py",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherCatalog, err := dependencies.BuildWithOmissions(
		[]dependencies.Importer{importer},
		[]dependencies.Dependency{{
			Language: "python", Kind: dependencies.KindExternal, Name: "requests",
			ModulePath: "requests", PackagePath: "requests", ImporterRefs: []string{importer.Ref},
		}},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	otherCatalogRaw, err := dependencies.Encode(otherCatalog)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, dependencies.ArtifactFilename), otherCatalogRaw)
	crossRun := manifest
	crossRun.MaterialInputs.DependencyCatalogSHA256 = manifestSHA256(otherCatalogRaw)
	if err := crossRun.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "result authority mismatch") {
		t.Fatalf("cross-run dependency authority error = %v", err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, dependencies.ArtifactFilename), dependencyRaw)
	readmeRaw := []byte(`{"version":1,"files":[{"file_ref":"f2","path":"app/main.py","classifications":[{"class":"target_entry","hypotheses":["README names the application entry."]}]}]}`)
	writeReportProgramFile(t, filepath.Join(runDir, "readme-file-roles.json"), readmeRaw)
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "unbound README file-role artifact") {
		t.Fatalf("unbound README file-role error = %v", err)
	}
	rolesBound := manifest
	rolesBound.MaterialInputs.ReadmeFileRolesSHA256 = manifestSHA256(readmeRaw)
	if err := rolesBound.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "count differs from producer coverage") {
		t.Fatalf("mismatched README file-role authority error = %v", err)
	}
	if err := os.Remove(filepath.Join(runDir, "readme-file-roles.json")); err != nil {
		t.Fatal(err)
	}

	unbound := manifest
	unbound.MaterialInputs.CoreMapSHA256 = ""
	unbound.MaterialInputs.ActivityEntrypointsSHA256 = ""
	unbound.MaterialInputs.PythonTargetCatalogSHA256 = ""
	unbound.MaterialInputs.DeclaredDependenciesSHA256 = ""
	unbound.MaterialInputs.DependencyCatalogSHA256 = ""
	unbound.MaterialInputs.IntegrationDependenciesSHA256 = ""
	unbound.MaterialInputs.IntegrationUsageSHA256 = ""
	unbound.MaterialInputs.ActivityPathsSHA256 = ""
	if err := unbound.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "unbound") {
		t.Fatalf("unbound Python semantic artifact error = %v", err)
	}
	if err := os.Remove(filepath.Join(runDir, pythontarget.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, dependencydeclaration.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, activityentrypoint.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, activitypath.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, dependencies.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, integrationdependency.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(runDir, integrationusage.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := unbound.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "unbound core map artifact") {
		t.Fatalf("unbound CoreMap error = %v", err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, dependencies.ArtifactFilename), dependencyRaw)
	writeReportProgramFile(t, filepath.Join(runDir, pythontarget.ArtifactFilename), targetRaw)
	writeReportProgramFile(t, filepath.Join(runDir, dependencydeclaration.ArtifactFilename), declarationRaw)
	writeReportProgramFile(t, filepath.Join(runDir, activityentrypoint.ArtifactFilename), activityRaw)
	writeReportProgramFile(t, filepath.Join(runDir, integrationdependency.ArtifactFilename), integrationRaw)
	writeReportProgramFile(t, filepath.Join(runDir, integrationusage.ArtifactFilename), usageRaw)
	writeReportProgramFile(t, filepath.Join(runDir, activitypath.ArtifactFilename), activityPathRaw)

	writeReportProgramFile(t, filepath.Join(runDir, coremap.ArtifactFilename), append(coreRaw, '\n'))
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("changed CoreMap error = %v", err)
	}

	if err := os.Remove(filepath.Join(runDir, coremap.ArtifactFilename)); err != nil {
		t.Fatal(err)
	}
	if err := manifest.VerifyCoreMapProjection(runDir, reportJSON); err == nil ||
		!strings.Contains(err.Error(), "bound core map artifact is missing") {
		t.Fatalf("missing bound CoreMap error = %v", err)
	}
}

func writeReportPythonSemanticArtifacts(
	t *testing.T,
	runDir string,
	index programindex.Index,
) ([]byte, []byte, []byte) {
	t.Helper()
	catalog := dependencies.Empty()
	catalogRaw, err := dependencies.Encode(catalog)
	if err != nil {
		t.Fatal(err)
	}
	_, declarations, targetRaw, declarationRaw := reportPythonDeclarationArtifactsFixture(t, index)
	result, err := integrationdependency.RunWithDeclarations(
		context.Background(), llm.Executor{}, nil, catalog, declarations, index.Target,
	)
	if err != nil {
		t.Fatal(err)
	}
	integrationRaw, err := integrationdependency.Encode(result)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := integrationusage.Run(context.Background(), llm.Executor{}, nil, index, result)
	if err != nil {
		t.Fatal(err)
	}
	usageRaw, err := integrationusage.Encode(usage)
	if err != nil {
		t.Fatal(err)
	}
	writeReportProgramFile(t, filepath.Join(runDir, dependencies.ArtifactFilename), catalogRaw)
	writeReportProgramFile(t, filepath.Join(runDir, pythontarget.ArtifactFilename), targetRaw)
	writeReportProgramFile(t, filepath.Join(runDir, dependencydeclaration.ArtifactFilename), declarationRaw)
	writeReportProgramFile(t, filepath.Join(runDir, integrationdependency.ArtifactFilename), integrationRaw)
	writeReportProgramFile(t, filepath.Join(runDir, integrationusage.ArtifactFilename), usageRaw)
	return catalogRaw, integrationRaw, usageRaw
}
