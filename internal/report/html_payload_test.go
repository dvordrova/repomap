package report

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrdinaryHTMLUsesOneTypedV4BrowserTransport(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	reportJSON := mustReportJSONForHTMLPayloadTest(t, &data)
	htmlBytes, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(htmlBytes, []byte(`id="rm-report-data"`)) {
		t.Fatal("ordinary HTML retained the legacy report data payload")
	}
	for _, id := range []string{
		`id="rm-bundle-index"`, `id="rm-repository-payload"`, `id="rm-target-chunk-0"`,
		`id="rm-report-loader-js"`, `id="rm-report-app-js"`, `id="rm-report-app-boot-js"`,
	} {
		if bytes.Count(htmlBytes, []byte(id)) != 1 {
			t.Errorf("script %s count is not one", id)
		}
	}
	transport, err := extractStandaloneBundleTransportV4HTML(htmlBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(transport.Index.Targets) != 1 || len(transport.TargetChunks) != 1 ||
		transport.Index.Targets[0].ProgramTargetID == "" {
		t.Fatalf("ordinary transport = %#v", transport.Index)
	}
	if _, err := DecodeBrowserRepositoryPayload(transport.RepositoryPayload); err != nil {
		t.Fatalf("decode repository payload: %v", err)
	}
	targetRaw, err := decodeStandaloneBundleTargetChunkV4(
		transport.TargetChunks[0].Ref, transport.TargetChunks[0].Base64,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeBrowserTargetPayload(targetRaw); err != nil {
		t.Fatalf("decode target payload: %v", err)
	}
	if err := VerifyOrdinaryReportHTMLPayload(
		htmlBytes, reportJSON, OrdinaryReportHTMLAuthority{},
	); err != nil {
		t.Fatalf("verify ordinary typed payload: %v", err)
	}
	assertHTMLScriptOrder(t, htmlBytes, []string{
		"rm-bundle-index", "rm-report-loader-js", "rm-system-canvas-graph-js",
		"rm-report-app-js", "rm-report-app-boot-js",
	})
}

func TestVerifyOrdinaryHTMLRejectsUnknownCorruptAndDuplicateTransport(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	reportJSON := mustReportJSONForHTMLPayloadTest(t, &data)
	htmlBytes, err := RenderHTMLWithOptions(&data, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := extractStandaloneBundleTransportV4HTML(htmlBytes)
	if err != nil {
		t.Fatal(err)
	}
	targetRaw, err := decodeStandaloneBundleTargetChunkV4(
		transport.TargetChunks[0].Ref, transport.TargetChunks[0].Base64,
	)
	if err != nil {
		t.Fatal(err)
	}
	unknownRepository := append([]byte(nil), transport.RepositoryPayload[:len(transport.RepositoryPayload)-1]...)
	unknownRepository = append(unknownRepository, []byte(`,"unknown":true}`)...)
	unknownTransport, err := prepareStandaloneBundleTransportV4(standaloneBundleTransportInputV4{
		RepositoryPayload:      unknownRepository,
		LogicalDefaultTargetID: transport.Index.LogicalDefaultTargetID,
		Targets: []standaloneBundleTransportTargetInputV4{{
			TargetID:        transport.Index.Targets[0].TargetID,
			ProgramTargetID: transport.Index.Targets[0].ProgramTargetID,
			State:           standaloneBundleTransportTargetAnalyzed, Payload: targetRaw,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	originalSection, err := standaloneBundleTransportHTMLSectionV4(transport)
	if err != nil {
		t.Fatal(err)
	}
	unknownSection, err := standaloneBundleTransportHTMLSectionV4(unknownTransport)
	if err != nil {
		t.Fatal(err)
	}
	unknownHTML := bytes.Replace(htmlBytes, originalSection, unknownSection, 1)
	if err := VerifyOrdinaryReportHTMLPayload(
		unknownHTML, reportJSON, OrdinaryReportHTMLAuthority{},
	); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown repository field error = %v", err)
	}

	corrupt := bytes.Replace(htmlBytes, []byte(transport.TargetChunks[0].Base64[:8]), []byte("AAAAAAAA"), 1)
	if err := VerifyOrdinaryReportHTMLPayload(
		corrupt, reportJSON, OrdinaryReportHTMLAuthority{},
	); err == nil {
		t.Fatal("corrupt target chunk was accepted")
	}
	duplicate := append(append([]byte(nil), htmlBytes...), transport.IndexJSON...)
	duplicate = append(duplicate, []byte(`<script type="application/json" id="rm-bundle-index">{}</script>`)...)
	if err := VerifyOrdinaryReportHTMLPayload(
		duplicate, reportJSON, OrdinaryReportHTMLAuthority{},
	); err == nil {
		t.Fatal("duplicate bundle index was accepted")
	}
}

func TestVerifyOrdinaryHTMLBindsSourceNavigationAndScrubsLocalRoots(t *testing.T) {
	data, navigation := targetNavigationFixture(t)
	artifactsDir := t.TempDir()
	repositoryRoot := t.TempDir()
	analysisRoot := filepath.Join(repositoryRoot, "services", "api")
	data.ArtifactsDir = artifactsDir
	data.standaloneLocalRoots = []string{artifactsDir, analysisRoot, repositoryRoot}
	data.Warnings = []string{"artifact " + artifactsDir + "/private.json"}
	links, err := newGitHubSourceLinks(
		"https://github.com/example/project", data.CapturedRevision, "services/api",
	)
	if err != nil {
		t.Fatal(err)
	}
	data.GitHubSourceLinks = links
	reportJSON := mustReportJSONForHTMLPayloadTest(t, data)
	htmlBytes, err := RenderHTMLWithOptions(data, RenderOptions{TargetNavigation: navigation})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(htmlBytes, []byte(artifactsDir)) {
		t.Fatal("ordinary browser payload retained an authorized local root")
	}
	transport, err := extractStandaloneBundleTransportV4HTML(htmlBytes)
	if err != nil {
		t.Fatal(err)
	}
	repository, err := DecodeBrowserRepositoryPayload(transport.RepositoryPayload)
	if err != nil {
		t.Fatal(err)
	}
	if transport.Index.LogicalDefaultTargetID != repository.LogicalDefaultSelectedTargetID {
		t.Fatalf("ordinary transport logical default = %q, repository = %q",
			transport.Index.LogicalDefaultTargetID, repository.LogicalDefaultSelectedTargetID)
	}
	if transport.Index.LogicalDefaultTargetID == transport.Index.Targets[0].TargetID {
		t.Fatal("fixture did not preserve a logical default outside the current one-chunk directory")
	}
	authority := OrdinaryReportHTMLAuthority{
		TargetNavigation: navigation,
		StandaloneSource: &StandaloneSourceAuthority{
			Host: "GitHub", RepositoryURL: "https://github.com/example/project",
		},
		ArtifactsDir: artifactsDir, AnalysisRoot: analysisRoot, RepositoryRoot: repositoryRoot,
	}
	if err := VerifyOrdinaryReportHTMLPayload(htmlBytes, reportJSON, authority); err != nil {
		t.Fatalf("verify source/navigation binding: %v", err)
	}
	wrong := authority
	wrong.TargetNavigation = nil
	if err := VerifyOrdinaryReportHTMLPayload(htmlBytes, reportJSON, wrong); err == nil {
		t.Fatal("unbound navigation was accepted")
	}
}

func TestBrowserHTMLEncoderChecksLocalPathsBeforeJSONEscaping(t *testing.T) {
	data := reportProgramShellDataFixture(t, "fixture")
	payload, err := ProjectBrowserTargetPayload(&data)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "private<&>")
	payload.Target.Name = root + "/target"
	if _, err := encodeBrowserTargetPayloadForHTML(payload, []string{root}); err == nil ||
		!strings.Contains(err.Error(), "retained a local path") {
		t.Fatalf("escaped local path error = %v", err)
	}

	repository, err := ProjectBrowserRepositoryPayload(&data, nil)
	if err != nil {
		t.Fatal(err)
	}
	repository.Warnings = []string{"inspect " + root + "/warning.json"}
	raw, err := encodeBrowserRepositoryPayloadForHTML(repository, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(root)) || bytes.Contains(raw, []byte(`\u003c`)) {
		t.Fatalf("escaped repository warning retained a local root: %s", raw)
	}
}

func assertHTMLScriptOrder(t *testing.T, htmlBytes []byte, ids []string) {
	t.Helper()
	previous := -1
	for _, id := range ids {
		position := bytes.Index(htmlBytes, []byte(`id="`+id+`"`))
		if position < 0 || position <= previous {
			t.Fatalf("script order %q is invalid", ids)
		}
		previous = position
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
