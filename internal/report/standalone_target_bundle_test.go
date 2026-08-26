package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/snapshot"
)

const standaloneBundleRevision = "0123456789abcdef0123456789abcdef01234567"

func TestWriteStandaloneTargetBundleAtomicPublishesCanonicalSelfContainedTargets(t *testing.T) {
	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	runDir := t.TempDir()
	localRoot := filepath.Join(t.TempDir(), "private-workstation")
	ready := preparedStandaloneTargetFixtures(t, container, localRoot)

	if err := WriteStandaloneTargetBundleAtomic(runDir, container, portfolio, ready); err != nil {
		t.Fatalf("WriteStandaloneTargetBundleAtomic: %v", err)
	}
	htmlPath := filepath.Join(runDir, "report.html")
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	identity, found, err := InspectStandaloneTargetBundleHTML(htmlPath)
	if err != nil || !found {
		t.Fatalf("InspectStandaloneTargetBundleHTML = %#v, %t, %v", identity, found, err)
	}
	if identity.Version != StandaloneTargetBundleVersion ||
		identity.DefaultTargetIndex != len(container.Targets)-1 ||
		identity.TargetCount != len(container.Targets) ||
		identity.TargetRunContainerSHA256 != container.SHA256 ||
		identity.TargetPagePortfolioSHA256 != portfolio.SHA256 {
		t.Fatalf("standalone bundle identity = %#v", identity)
	}

	wire := embeddedStandaloneTargetBundle(t, htmlBytes)
	if wire.Version != StandaloneTargetBundleVersion ||
		wire.DefaultTargetIndex != identity.DefaultTargetIndex ||
		len(wire.Targets) != len(container.Targets) {
		t.Fatalf("standalone bundle wire = %#v", wire)
	}
	for index, target := range wire.Targets {
		wantPage := ready[index].prepared.programPage
		if target.TargetID != wantPage.ProgramTarget.ID ||
			target.Language != wantPage.ProgramTarget.Language ||
			target.Kind != wantPage.ProgramTarget.Kind ||
			target.DisplayName != wantPage.ProgramTarget.Name {
			t.Fatalf("standalone target %d = %#v", index, target)
		}
		if target.Href != fmt.Sprintf("?target=%d#/program", index) || len(target.Payload) == 0 {
			t.Fatalf("published standalone target %d = %#v", index, target)
		}
		var payload struct {
			AnalysisTarget   analysistarget.Target `json:"analysis_target"`
			TargetNavigation json.RawMessage       `json:"target_navigation"`
			Warnings         []string              `json:"warnings"`
		}
		if err := json.Unmarshal(target.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.AnalysisTarget.Ref != container.Targets[index].Target.Ref || len(payload.TargetNavigation) != 0 ||
			len(payload.Warnings) != 1 || payload.Warnings[0] != fmt.Sprintf("PAYLOAD_%d", index) {
			t.Fatalf("ready standalone payload %d = %#v", index, payload)
		}
		if bytes.Count(htmlBytes, []byte(payload.Warnings[0])) != 1 {
			t.Fatalf("payload %q appears %d times", payload.Warnings[0], bytes.Count(htmlBytes, []byte(payload.Warnings[0])))
		}
	}
	if bytes.Contains(htmlBytes, []byte(`"artifact_filename"`)) {
		t.Fatal("standalone browser bundle exposes backend-only ProgramIndex artifact filenames")
	}

	for _, unique := range []string{
		`id="rm-standalone-target-bundle"`,
		`id="rm-report-data"`,
		`id="rm-standalone-target-bootstrap"`,
		`id="rm-report-app-css"`,
		`id="rm-report-app-js"`,
	} {
		if count := bytes.Count(htmlBytes, []byte(unique)); count != 1 {
			t.Errorf("asset marker %q count = %d, want 1", unique, count)
		}
	}
	for _, forbiddenAsset := range []string{
		`id="rm-ui-messages-js"`, `id="rm-elkjs"`,
		`id="rm-architecture-canvas-js"`, `id="rm-surface-catalog-js"`,
	} {
		if bytes.Contains(htmlBytes, []byte(forbiddenAsset)) {
			t.Errorf("standalone ProgramPortfolio shell retained legacy asset %q", forbiddenAsset)
		}
	}
	for _, forbidden := range append([]string{localRoot, runDir}, standaloneRunIDs(portfolio)...) {
		if bytes.Contains(htmlBytes, []byte(forbidden)) {
			t.Errorf("standalone bundle retained forbidden local authority %q", forbidden)
		}
	}
}

func TestExactStandaloneTargetBundleProjectionRejectsRewrittenResealedPayload(t *testing.T) {
	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	runDir := t.TempDir()
	ready := preparedStandaloneTargetFixtures(t, container, filepath.Join(t.TempDir(), "private"))
	validated, err := validateStandaloneTargetBundle(container, portfolio, ready)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteStandaloneTargetBundleAtomic(runDir, container, portfolio, ready); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(runDir, "report.html")
	itemAt := func(index int) (standaloneTargetBundleItem, error) {
		return validated.targets[index], nil
	}
	if err := verifyExactStandaloneTargetBundleProjection(
		htmlPath, validated.identity, validated.defaultTarget.repoName, itemAt,
	); err != nil {
		t.Fatalf("verify exact canonical projection: %v", err)
	}

	raw, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(raw, []byte("PAYLOAD_0"), []byte("PAYLOAD_X"), 1)
	if bytes.Equal(tampered, raw) {
		t.Fatal("bundle payload sentinel was not present")
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
		t.Fatalf("self-sealed tampered bundle was not structurally valid: found=%t err=%v", found, err)
	}
	if err := verifyExactStandaloneTargetBundleProjection(
		htmlPath, validated.identity, validated.defaultTarget.repoName, itemAt,
	); err == nil || !strings.Contains(err.Error(), "manifest-derived projection") {
		t.Fatalf("resealed payload authority error = %v", err)
	}
}

func TestStandaloneProgramPortfolioUsesProgramRoutesAndEmbedsItsAssets(t *testing.T) {
	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	ready := preparedStandaloneTargetFixtures(t, container, filepath.Join(t.TempDir(), "private"))
	runDir := t.TempDir()
	if err := WriteStandaloneTargetBundleAtomic(runDir, container, portfolio, ready); err != nil {
		t.Fatal(err)
	}
	htmlBytes, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	wire := embeddedStandaloneTargetBundle(t, htmlBytes)
	for index, item := range wire.Targets {
		if want := fmt.Sprintf("?target=%d#/program", index); item.Href != want {
			t.Errorf("program target %d href = %q, want %q", index, item.Href, want)
		}
	}
	for _, marker := range []string{
		`id="rm-app"`,
		`id="rm-report-app-css"`,
		`id="rm-report-app-js"`,
		`buildPresentationModel(data)`,
	} {
		if !bytes.Contains(htmlBytes, []byte(marker)) {
			t.Errorf("standalone ProgramView bundle is missing %q", marker)
		}
	}
}

func TestStandaloneTargetBootstrapSelectsBeforeMainApplication(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is unavailable")
	}
	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	runDir := t.TempDir()
	ready := preparedStandaloneTargetFixtures(t, container, filepath.Join(t.TempDir(), "private"))
	if err := WriteStandaloneTargetBundleAtomic(runDir, container, portfolio, ready); err != nil {
		t.Fatal(err)
	}
	htmlBytes, err := os.ReadFile(filepath.Join(runDir, "report.html"))
	if err != nil {
		t.Fatal(err)
	}
	bundleJSON := embeddedStandaloneTargetBundleJSON(t, htmlBytes)
	bundlePath := filepath.Join(t.TempDir(), "bundle.json")
	if err := os.WriteFile(bundlePath, bundleJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	bootstrapPath, err := filepath.Abs(filepath.Join("templates", "standalone_target_bootstrap.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs=require("fs"),vm=require("vm");
const bundle=fs.readFileSync(process.argv[4],"utf8");
function execute(search){
  const bundleNode={textContent:bundle},reportNode={textContent:""};
  const document={documentElement:{lang:"en"},title:"",
    getElementById(id){if(id==="rm-standalone-target-bundle")return bundleNode;if(id==="rm-report-data")return reportNode;return null},
    querySelector(){return null},querySelectorAll(){return []}};
  const window={document,location:{search,hash:"#/program",protocol:"file:",pathname:"/bucket/report.html"}};
  const context={window,document,URLSearchParams};
  vm.runInNewContext(fs.readFileSync(process.argv[2],"utf8"),context);
  const data=JSON.parse(reportNode.textContent);
  if(data.standalone_target_error)return {search,error:data.standalone_target_error};
  const items=data.target_navigation.targets.map(item=>({href:item.href}));
  return {search,ref:data.analysis_target.ref,targetID:data.target_navigation.targets[data.target_navigation.current_target_index].target_id,sentinel:data.warnings[0],
    current:data.target_navigation.current_target_index,def:data.target_navigation.default_target_index,
    version:data.target_navigation.version,language:document.documentElement.lang,title:document.title,items};
}
process.stdout.write(JSON.stringify(["","?target=0","?target=00","?target=%31","?target=1","?target=999","?target=0&target=2"].map(execute)));
`
	runnerPath := filepath.Join(t.TempDir(), "standalone-target-bootstrap.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, bootstrapPath, "unused", bundlePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run standalone target bootstrap: %v\n%s", err, output)
	}
	var got []struct {
		Search, Ref, TargetID, Sentinel, Language, Title, Error string
		Current, Def, Version                                   int
		Items                                                   []struct {
			Href string
		}
	}
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatalf("decode standalone target bootstrap: %v\n%s", err, output)
	}
	defaultIndex := len(container.Targets) - 1
	if len(got) != 7 {
		t.Fatalf("bootstrap results = %#v", got)
	}
	for _, result := range got {
		valid := result.Search == "" || result.Search == "?target=0" || result.Search == "?target=1"
		if !valid {
			if result.Error == "" || result.Ref != "" || len(result.Items) != 0 {
				t.Errorf("bootstrap invalid query %q did not fail closed: %#v", result.Search, result)
			}
			continue
		}
		want := defaultIndex
		if result.Search == "?target=0" {
			want = 0
		} else if result.Search == "?target=1" {
			want = 1
		}
		if result.Current != want || result.Def != defaultIndex ||
			result.Version != StandaloneTargetNavigationVersion ||
			result.Ref != container.Targets[want].Target.Ref ||
			result.TargetID != ready[want].prepared.programPage.ProgramTarget.ID ||
			result.Sentinel != fmt.Sprintf("PAYLOAD_%d", want) ||
			result.Language != "en" || !strings.Contains(result.Title, "bundle-fixture") {
			t.Errorf("bootstrap %q = %#v, selected index %d", result.Search, result, want)
		}
		if len(result.Items) != len(container.Targets) {
			t.Fatalf("bootstrap menu %q = %#v", result.Search, result.Items)
		}
		for index, item := range result.Items {
			if item.Href != fmt.Sprintf("?target=%d#/program", index) {
				t.Errorf("bootstrap menu href %q item %d = %q", result.Search, index, item.Href)
			}
		}
	}
	if bytes.Contains(htmlBytes, []byte("report.json")) {
		t.Fatal("standalone target bundle noscript copy references an external report.json")
	}
	if !bytes.Contains(htmlBytes, []byte("Its repository evidence is embedded in this HTML file")) {
		t.Fatal("standalone target bundle noscript copy does not disclose embedded data")
	}
}

func TestStandaloneTargetHrefResolvesForFileAndHostedObjectStorage(t *testing.T) {
	reference, err := url.Parse("?target=2#/program")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ base, want string }{
		{
			base: "file:///Users/test/export/report.html?target=0#study-theme-2",
			want: "file:///Users/test/export/report.html?target=2#/program",
		},
		{
			base: "https://bucket.example.test/maps/report.html?target=0#study-theme-2",
			want: "https://bucket.example.test/maps/report.html?target=2#/program",
		},
	} {
		base, parseErr := url.Parse(test.base)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if got := base.ResolveReference(reference).String(); got != test.want {
			t.Errorf("resolve from %q = %q, want %q", test.base, got, test.want)
		}
	}
}

func TestPrepareStandaloneTargetPinsSourceAndScrubsLocalAuthority(t *testing.T) {
	container, _ := standaloneTargetBundleAuthorityFixture(t)
	localRoot := filepath.Join(t.TempDir(), "secret-run")
	target := container.Targets[0].Target
	payload, err := json.Marshal(map[string]any{
		"format_version":       CurrentFormatVersion,
		"analysis_target":      target,
		"program_portfolio":    standaloneProgramPortfolioFixture(t),
		"repo_name":            "bundle-fixture",
		"captured_revision":    standaloneBundleRevision,
		"captured_input_count": 0,
		"openable_paths":       []string{},
		"github_source_links": map[string]any{
			"repository_url": "https://github.com/example/bundle",
			"revision":       standaloneBundleRevision,
		},
		"warnings": []string{"open " + localRoot + "/orientation.json"},
		"target_navigation": map[string]any{
			"version": TargetNavigationVersion, "targets": []any{map[string]any{"href": "../private-run/report.html#/program"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	htmlBytes := []byte("<html><body>" + reportDataScriptOpen + string(payload) + reportDataScriptClose + "</body></html>")
	prepared, err := prepareStandaloneTargetFromHTML(htmlBytes, []string{localRoot})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{localRoot, "target_navigation", "private-run"} {
		if bytes.Contains(prepared.payload, []byte(forbidden)) {
			t.Errorf("prepared payload retained %q: %s", forbidden, prepared.payload)
		}
	}
	for _, required := range []string{
		`"repository_url":"https://github.com/example/bundle"`,
		`"revision":"` + standaloneBundleRevision + `"`,
		`open [local path]/orientation.json`,
	} {
		if !bytes.Contains(prepared.payload, []byte(required)) {
			t.Errorf("prepared payload lost %q: %s", required, prepared.payload)
		}
	}
}

func TestPrepareStandaloneTargetRestoresManifestValidatedOuterTarget(t *testing.T) {
	container, _ := standaloneTargetBundleAuthorityFixture(t)
	target := container.Targets[0].Target
	payloadData := map[string]any{
		"format_version":       CurrentFormatVersion,
		"program_portfolio":    standaloneProgramPortfolioFixture(t),
		"repo_name":            "bundle-fixture",
		"captured_revision":    standaloneBundleRevision,
		"captured_input_count": 0,
		"openable_paths":       []string{},
		"github_source_links": map[string]any{
			"repository_url": "https://github.com/example/bundle",
			"revision":       standaloneBundleRevision,
		},
	}
	payload, err := json.Marshal(payloadData)
	if err != nil {
		t.Fatal(err)
	}
	htmlBytes := []byte("<html><body>" + reportDataScriptOpen + string(payload) + reportDataScriptClose + "</body></html>")
	prepared, err := prepareStandaloneTargetFromHTMLWithAuthority(htmlBytes, nil, &target)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.analysisTarget == nil || !reflect.DeepEqual(*prepared.analysisTarget, target) {
		t.Fatalf("prepared analysis target = %#v, want %#v", prepared.analysisTarget, target)
	}
	var projected struct {
		AnalysisTarget *analysistarget.Target `json:"analysis_target"`
	}
	if err := json.Unmarshal(prepared.payload, &projected); err != nil {
		t.Fatal(err)
	}
	if projected.AnalysisTarget == nil || !reflect.DeepEqual(*projected.AnalysisTarget, target) {
		t.Fatalf("standalone payload analysis target = %#v, want %#v", projected.AnalysisTarget, target)
	}
	var manifestData ReportData
	if err := json.Unmarshal(payload, &manifestData); err != nil {
		t.Fatal(err)
	}
	manifestTarget := target.Snapshot()
	manifestData.AnalysisTarget = &manifestTarget
	manifestProjection, err := marshalHTMLPayloadWithLocalRoots(
		manifestStandaloneProgramShellPayload(&manifestData, target), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(prepared.payload, manifestProjection) {
		t.Fatalf("prepared no-CubeMap payload does not byte-match manifest replay:\nprepared: %s\nmanifest: %s", prepared.payload, manifestProjection)
	}

	drifted := target.Snapshot()
	drifted.PackageDir = "foreign"
	drifted.Ref = "at-000000000000000000000000"
	payloadData["analysis_target"] = drifted
	payload, err = json.Marshal(payloadData)
	if err != nil {
		t.Fatal(err)
	}
	htmlBytes = []byte("<html><body>" + reportDataScriptOpen + string(payload) + reportDataScriptClose + "</body></html>")
	if _, err := prepareStandaloneTargetFromHTMLWithAuthority(htmlBytes, nil, &target); err == nil ||
		!strings.Contains(err.Error(), "analysis target authority mismatch") {
		t.Fatalf("drifted analysis target error = %v", err)
	}
}

func TestPrepareStandaloneTargetRequiresProgramPortfolioAndRejectsLegacyAliases(t *testing.T) {
	container, _ := standaloneTargetBundleAuthorityFixture(t)
	ready := preparedStandaloneTargetFixtures(t, container, filepath.Join(t.TempDir(), "private"))
	input := ready[0].prepared
	htmlBytes := []byte("<html><body>" + reportDataScriptOpen + string(input.payload) + reportDataScriptClose + "</body></html>")

	if _, err := prepareStandaloneTargetFromHTML(htmlBytes, nil); err != nil {
		t.Fatal(err)
	}

	var incomplete map[string]any
	if err := json.Unmarshal(input.payload, &incomplete); err != nil {
		t.Fatal(err)
	}
	incomplete["program_target"] = map[string]any{"id": "legacy-alias"}
	payload, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	htmlBytes = []byte("<html><body>" + reportDataScriptOpen + string(payload) + reportDataScriptClose + "</body></html>")
	if _, err := prepareStandaloneTargetFromHTML(htmlBytes, nil); err == nil ||
		!strings.Contains(err.Error(), `unknown field "program_target"`) {
		t.Fatalf("legacy program alias error = %v", err)
	}
}

func TestStandaloneTargetBundleRejectsAuthorityDriftAndPreservesExistingHTML(t *testing.T) {
	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	ready := preparedStandaloneTargetFixtures(t, container, filepath.Join(t.TempDir(), "private"))
	runDir := t.TempDir()
	htmlPath := filepath.Join(runDir, "report.html")
	const original = "ORIGINAL_REPORT"
	if err := os.WriteFile(htmlPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func([]PreparedStandaloneTarget) []PreparedStandaloneTarget{
		"missing selected target": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			return values[:len(values)-1]
		},
		"revision drift": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			values = clonePreparedStandaloneTargets(values)
			values[len(values)-1].prepared.revision = strings.Repeat("f", 40)
			return values
		},
		"host drift": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			values = clonePreparedStandaloneTargets(values)
			values[len(values)-1].prepared.host = "GitLab"
			return values
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if err := WriteStandaloneTargetBundleAtomic(runDir, container, portfolio, mutate(ready)); err == nil {
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
}

func TestInspectStandaloneTargetBundleHTMLDistinguishesOrdinaryAndTamperedReports(t *testing.T) {
	ordinaryPath := filepath.Join(t.TempDir(), "report.html")
	ordinaryData, _ := targetNavigationFixture(t)
	ordinary, err := RenderHTMLWithOptions(ordinaryData, RenderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ordinaryPath, ordinary, 0o644); err != nil {
		t.Fatal(err)
	}
	if identity, found, err := InspectStandaloneTargetBundleHTML(ordinaryPath); err != nil || found || identity != (StandaloneTargetBundleIdentity{}) {
		t.Fatalf("ordinary report inspection = %#v, %t, %v", identity, found, err)
	}

	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	runDir := t.TempDir()
	ready := preparedStandaloneTargetFixtures(t, container, filepath.Join(t.TempDir(), "private"))
	if err := WriteStandaloneTargetBundleAtomic(runDir, container, portfolio, ready); err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(runDir, "report.html")
	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatal(err)
	}
	needle := []byte("PAYLOAD_0")
	offset := bytes.Index(htmlBytes, needle)
	if offset < 0 {
		t.Fatal("bundle payload sentinel is absent")
	}
	htmlBytes[offset] = 'X'
	if err := os.WriteFile(htmlPath, htmlBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, found, err := InspectStandaloneTargetBundleHTML(htmlPath); !found || err == nil || !strings.Contains(err.Error(), "seal mismatch") {
		t.Fatalf("tampered report inspection = found %t, error %v", found, err)
	}
}

func TestStandaloneTargetBundleAggregateLimitIsTerminal(t *testing.T) {
	if got, err := standaloneTargetAggregateBytes(MaxStandaloneTargetBundlePayloadBytes-7, 7); err != nil || got != MaxStandaloneTargetBundlePayloadBytes {
		t.Fatalf("exact aggregate bound = %d, %v", got, err)
	}
	if _, err := standaloneTargetAggregateBytes(MaxStandaloneTargetBundlePayloadBytes-7, 8); err == nil {
		t.Fatal("over-limit aggregate was accepted")
	} else {
		var limit *StandaloneTargetBundleResourceLimitError
		if !strings.Contains(err.Error(), fmt.Sprint(MaxStandaloneTargetBundlePayloadBytes)) ||
			!errorAs(err, &limit) || limit.ActualBytes != MaxStandaloneTargetBundlePayloadBytes+1 {
			t.Fatalf("aggregate limit error = %#v", err)
		}
	}
}

func errorAs(err error, target any) bool {
	// Keep the test's import surface small while still checking the concrete
	// terminal resource outcome.
	switch typed := target.(type) {
	case **StandaloneTargetBundleResourceLimitError:
		value, ok := err.(*StandaloneTargetBundleResourceLimitError)
		if ok {
			*typed = value
		}
		return ok
	default:
		return false
	}
}

func standaloneTargetBundleAuthorityFixture(
	t *testing.T,
) (snapshot.TargetRunContainer, snapshot.TargetPagePortfolio) {
	t.Helper()
	fixture := newTargetPageManifestFixture(t)
	container, err := snapshot.BuildTargetRunContainer(fixture.source, snapshot.TargetRunSelection{
		DefaultTargetRef: fixture.helperRef,
		TargetRefs:       []string{fixture.appRef, fixture.helperRef},
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	for index, projection := range container.Targets {
		outcomes = append(outcomes, snapshot.TargetPageOutcome{
			TargetRef: projection.Target.Ref,
			RunID:     fmt.Sprintf("20260811-01010%d-target-%d", index, index),
		})
	}
	portfolio, err := snapshot.BuildTargetPagePortfolio(container, outcomes)
	if err != nil {
		t.Fatal(err)
	}
	return container, portfolio
}

func preparedStandaloneTargetFixtures(
	t *testing.T,
	container snapshot.TargetRunContainer,
	localRoot string,
) []PreparedStandaloneTarget {
	t.Helper()
	result := make([]PreparedStandaloneTarget, 0, len(container.Targets))
	for index, projection := range container.Targets {
		programPortfolio := standaloneProgramPortfolioFixture(t, index)
		defaultEntry, err := programPortfolio.defaultEntry()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := json.Marshal(map[string]any{
			"format_version":       CurrentFormatVersion,
			"analysis_target":      projection.Target,
			"program_portfolio":    programPortfolio,
			"repo_name":            "bundle-fixture",
			"captured_revision":    standaloneBundleRevision,
			"captured_input_count": 0,
			"openable_paths":       []string{},
			"warnings":             []string{fmt.Sprintf("PAYLOAD_%d", index)},
			"github_source_links": map[string]any{
				"repository_url": "https://github.com/example/bundle",
				"revision":       standaloneBundleRevision,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		analysisTarget := projection.Target.Snapshot()
		result = append(result, PreparedStandaloneTarget{prepared: &preparedStandaloneTarget{
			analysisTarget: &analysisTarget,
			programPage: TargetNavigationPage{
				RunID:            fmt.Sprintf("20260811-01010%d-target-%d", index, index),
				ProgramTarget:    defaultEntry.Target.Snapshot(),
				ArtifactFilename: fmt.Sprintf("program-index-%d.json", index),
			},
			payload: payload,
			host:    "GitHub", repositoryURL: "https://github.com/example/bundle",
			revision:   standaloneBundleRevision,
			repoName:   "bundle-fixture",
			localRoots: []string{localRoot},
		}})
	}
	return result
}

func standaloneProgramPortfolioFixture(t *testing.T, ordinal ...int) *ProgramPortfolio {
	t.Helper()
	value := 0
	if len(ordinal) != 0 {
		value = ordinal[0]
	}
	index := reportProgramIndexFixture(t, "go", fmt.Sprintf("executable-%d", value))
	portfolio, err := NewProgramPortfolio(index.Target.ID, []programindex.Index{index})
	if err != nil {
		t.Fatal(err)
	}
	return portfolio
}

func clonePreparedStandaloneTargets(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
	cloned := make([]PreparedStandaloneTarget, len(values))
	for index, value := range values {
		copyValue := *value.prepared
		copyValue.payload = append([]byte(nil), value.prepared.payload...)
		copyValue.localRoots = append([]string(nil), value.prepared.localRoots...)
		cloned[index] = PreparedStandaloneTarget{prepared: &copyValue}
	}
	return cloned
}

func standaloneRunIDs(portfolio snapshot.TargetPagePortfolio) []string {
	result := make([]string, 0, len(portfolio.Targets))
	for _, page := range portfolio.Targets {
		if page.RunID != "" {
			result = append(result, page.RunID)
		}
	}
	return result
}

func embeddedStandaloneTargetBundle(
	t *testing.T,
	htmlBytes []byte,
) standaloneTargetBundleWire {
	t.Helper()
	var wire standaloneTargetBundleWire
	if err := json.Unmarshal(embeddedStandaloneTargetBundleJSON(t, htmlBytes), &wire); err != nil {
		t.Fatalf("decode standalone target bundle wire: %v", err)
	}
	return wire
}

func embeddedStandaloneTargetBundleJSON(t *testing.T, htmlBytes []byte) []byte {
	t.Helper()
	const open = `<script type="application/json" id="rm-standalone-target-bundle">`
	start := bytes.Index(htmlBytes, []byte(open))
	if start < 0 {
		t.Fatal("standalone target bundle script is absent")
	}
	start += len(open)
	end := bytes.Index(htmlBytes[start:], []byte(`</script>`))
	if end < 0 {
		t.Fatal("standalone target bundle script is unterminated")
	}
	return bytes.TrimSpace(htmlBytes[start : start+end])
}
