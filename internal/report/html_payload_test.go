package report

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyOrdinaryReportHTMLPayloadBindsCanonicalReport(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	reportJSON := mustReportJSONForHTMLPayloadTest(t, &data)
	htmlBytes, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyOrdinaryReportHTMLPayload(htmlBytes, reportJSON, OrdinaryReportHTMLAuthority{}); err != nil {
		t.Fatalf("verify canonical html payload: %v", err)
	}

	payloadJSON := embeddedReportDataJSON(t, htmlBytes)
	var payload map[string]any
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	payload["repo_name"] = "plausible-but-wrong"
	driftedJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	driftedHTML := bytes.Replace(htmlBytes, payloadJSON, driftedJSON, 1)
	if err := VerifyOrdinaryReportHTMLPayload(driftedHTML, reportJSON, OrdinaryReportHTMLAuthority{}); err == nil ||
		!strings.Contains(err.Error(), "does not match report.json") {
		t.Fatalf("drifted embedded payload error = %v", err)
	}
}

func TestVerifyOrdinaryReportHTMLPayloadRejectsMalformedOrAmbiguousPayload(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	reportJSON := mustReportJSONForHTMLPayloadTest(t, &data)
	htmlBytes, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	payloadJSON := embeddedReportDataJSON(t, htmlBytes)

	unknown := append([]byte(nil), bytes.TrimSuffix(payloadJSON, []byte("}"))...)
	unknown = append(unknown, []byte(`,"unknown":true}`)...)
	unknownHTML := bytes.Replace(htmlBytes, payloadJSON, unknown, 1)
	if err := VerifyOrdinaryReportHTMLPayload(unknownHTML, reportJSON, OrdinaryReportHTMLAuthority{}); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown embedded field error = %v", err)
	}

	duplicated := append(append([]byte{}, htmlBytes...), []byte(reportDataScriptOpen+`{}`+reportDataScriptClose)...)
	if err := VerifyOrdinaryReportHTMLPayload(duplicated, reportJSON, OrdinaryReportHTMLAuthority{}); err == nil ||
		!strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("duplicate embedded payload error = %v", err)
	}
}

func TestVerifyOrdinaryReportHTMLPayloadPreservesValidatedStandaloneRouting(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	artifactsDir := t.TempDir()
	repositoryRoot := t.TempDir()
	analysisRoot := filepath.Join(repositoryRoot, "services", "api")
	data.ArtifactsDir = artifactsDir
	data.standaloneLocalRoots = []string{artifactsDir, analysisRoot, repositoryRoot}
	data.Warnings = []string{
		"artifact " + artifactsDir,
		"source " + analysisRoot,
		"repository " + repositoryRoot,
	}
	links, err := newGitHubSourceLinks(
		"https://github.com/example/project",
		data.CapturedRevision,
		"services/api",
	)
	if err != nil {
		t.Fatal(err)
	}
	data.GitHubSourceLinks = links
	reportJSON := mustReportJSONForHTMLPayloadTest(t, &data)
	htmlBytes, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	authority := OrdinaryReportHTMLAuthority{
		StandaloneSource: &StandaloneSourceAuthority{
			Host:          "GitHub",
			RepositoryURL: "https://github.com/example/project",
		},
		ArtifactsDir:   artifactsDir,
		AnalysisRoot:   analysisRoot,
		RepositoryRoot: repositoryRoot,
	}
	if err := VerifyOrdinaryReportHTMLPayload(htmlBytes, reportJSON, authority); err != nil {
		t.Fatalf("verify standalone source payload: %v", err)
	}
	if err := VerifyOrdinaryReportHTMLPayload(
		htmlBytes, reportJSON, OrdinaryReportHTMLAuthority{},
	); err == nil || !strings.Contains(err.Error(), "manifest authority") {
		t.Fatalf("unbound standalone source error = %v", err)
	}
	drifted := bytes.Replace(
		htmlBytes,
		[]byte("https://github.com/example/project"),
		[]byte("https://github.com/example/another"),
		1,
	)
	if bytes.Equal(drifted, htmlBytes) {
		t.Fatal("standalone source URL was not present in rendered HTML")
	}
	if err := VerifyOrdinaryReportHTMLPayload(drifted, reportJSON, authority); err == nil ||
		!strings.Contains(err.Error(), "manifest authority") {
		t.Fatalf("drifted standalone source error = %v", err)
	}
}

func TestVerifyOrdinaryReportHTMLPayloadAllowsValidatedRenderOnlyNavigation(t *testing.T) {
	data, navigation := targetNavigationFixture(t)
	reportJSON := mustReportJSONForHTMLPayloadTest(t, data)
	htmlBytes, err := RenderHTMLWithOptions(data, RenderOptions{TargetNavigation: navigation})
	if err != nil {
		t.Fatal(err)
	}
	authority := OrdinaryReportHTMLAuthority{TargetNavigation: navigation}
	if err := VerifyOrdinaryReportHTMLPayload(htmlBytes, reportJSON, authority); err != nil {
		t.Fatalf("verify navigable html payload: %v", err)
	}
}

func TestProgramShellPayloadOmitsBackendOnlyProducerDigests(t *testing.T) {
	digest := strings.Repeat("a", 64)
	cube := &CubeMapView{
		SourceIndexSHA256:          digest,
		ExternalCallIndexSHA256:    digest,
		DependencyCatalogSHA256:    digest,
		CoreObjectIndexSHA256:      digest,
		CoreObjectProjectionSHA256: digest,
		ActivitySubstrateSHA256:    digest,
		SurfaceCoreEffects: &CubeMapViewSurfaceCoreEffects{
			AuthoritySHA256: digest,
		},
	}
	core := &CoreMapView{IntegrationUsageSHA256: digest}
	usage := &IntegrationUsageView{
		DependencyCatalogSHA256:       digest,
		IntegrationDependenciesSHA256: digest,
		IntegrationUsageSHA256:        digest,
	}
	paths := &ActivityPathView{
		ActivityEntrypointsSHA256:     digest,
		IntegrationDependenciesSHA256: digest,
		IntegrationUsageSHA256:        digest,
		ActivityPathsSHA256:           digest,
	}
	payload := programShellPayloadForReport(&ReportData{
		CubeMapView:          cube,
		CoreMapView:          core,
		IntegrationUsageView: usage,
		ActivityPathView:     paths,
	}, nil)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"source_index_sha256", "external_call_index_sha256",
		"dependency_catalog_sha256", "core_object_index_sha256",
		"core_object_projection_sha256", "activity_substrate_sha256",
		"authority_sha256", "integration_dependencies_sha256",
		"integration_usage_sha256", "activity_entrypoints_sha256", "activity_paths_sha256",
	} {
		if bytes.Contains(raw, []byte(`"`+field+`"`)) {
			t.Fatalf("browser payload retained backend-only producer digest %q", field)
		}
	}
	if cube.SourceIndexSHA256 != digest || cube.SurfaceCoreEffects.AuthoritySHA256 != digest ||
		core.IntegrationUsageSHA256 != digest || usage.IntegrationUsageSHA256 != digest ||
		paths.ActivityPathsSHA256 != digest {
		t.Fatal("browser projection mutated canonical report authority")
	}
}

func TestProgramShellPayloadDoesNotExposeGoOuterTargetToProgramSemanticBrowser(t *testing.T) {
	target := reportAnalysisTargetForDir(t, ".")
	data := &ReportData{
		AnalysisTarget: target,
		CoreMapView:    &CoreMapView{},
	}
	payload := programShellPayloadForReport(data, nil)
	if payload.AnalysisTarget != nil {
		t.Fatalf("language-neutral browser payload retained Go outer analysis target: %#v", payload.AnalysisTarget)
	}
	if data.AnalysisTarget == nil || data.AnalysisTarget.Ref != target.Ref {
		t.Fatal("browser projection mutated canonical Go page authority")
	}
}

func mustReportJSONForHTMLPayloadTest(t *testing.T, data *ReportData) []byte {
	t.Helper()
	persisted := reportDataForPersistence(data)
	persisted.SourceIDs = nil
	raw, err := json.Marshal(persisted)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
