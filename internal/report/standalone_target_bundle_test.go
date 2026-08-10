package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/snapshot"
)

const standaloneBundleRevision = "0123456789abcdef0123456789abcdef01234567"

func TestWriteStandaloneTargetBundleAtomicPublishesCanonicalSelfContainedTargets(t *testing.T) {
	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	runDir := t.TempDir()
	localRoot := filepath.Join(t.TempDir(), "private-workstation")
	ready := preparedStandaloneTargetFixtures(t, container, portfolio, localRoot)

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
		identity.ReadyTargetCount != len(ready) ||
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
		page := portfolio.Targets[index]
		if target.TargetRef != container.Targets[index].Target.Ref ||
			target.Available != (page.State == snapshot.TargetPageReady) {
			t.Fatalf("standalone target %d = %#v", index, target)
		}
		if !target.Available {
			if target.Href != "" || len(target.Payload) != 0 {
				t.Fatalf("unavailable standalone target %d retained route/payload: %#v", index, target)
			}
			continue
		}
		if target.Href != fmt.Sprintf("?target=%d#canvas", index) || len(target.Payload) == 0 {
			t.Fatalf("ready standalone target %d = %#v", index, target)
		}
		var payload struct {
			AnalysisTarget   analysistarget.Target `json:"analysis_target"`
			TargetNavigation json.RawMessage       `json:"target_navigation"`
			Sentinel         string                `json:"bundle_test_sentinel"`
		}
		if err := json.Unmarshal(target.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.AnalysisTarget.Ref != target.TargetRef || len(payload.TargetNavigation) != 0 ||
			payload.Sentinel != fmt.Sprintf("PAYLOAD_%d", index) {
			t.Fatalf("ready standalone payload %d = %#v", index, payload)
		}
		if bytes.Count(htmlBytes, []byte(payload.Sentinel)) != 1 {
			t.Fatalf("payload %q appears %d times", payload.Sentinel, bytes.Count(htmlBytes, []byte(payload.Sentinel)))
		}
	}

	for _, unique := range []string{
		`id="rm-standalone-target-bundle"`,
		`id="rm-report-data"`,
		`id="rm-standalone-target-bootstrap"`,
		`id="rm-ui-messages-js"`,
		`id="rm-elkjs"`,
		`id="rm-architecture-canvas-js"`,
		`id="rm-surface-catalog-js"`,
	} {
		if count := bytes.Count(htmlBytes, []byte(unique)); count != 1 {
			t.Errorf("asset marker %q count = %d, want 1", unique, count)
		}
	}
	for _, forbidden := range append([]string{localRoot, runDir}, standaloneReadyRunIDs(portfolio)...) {
		if bytes.Contains(htmlBytes, []byte(forbidden)) {
			t.Errorf("standalone bundle retained forbidden local authority %q", forbidden)
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
	ready := preparedStandaloneTargetFixtures(t, container, portfolio, filepath.Join(t.TempDir(), "private"))
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
	mainPath, err := filepath.Abs(filepath.Join("templates", "script.js"))
	if err != nil {
		t.Fatal(err)
	}
	runner := `
const fs=require("fs"),vm=require("vm");
const bundle=fs.readFileSync(process.argv[4],"utf8");
class Element {
  constructor(tag){this.tagName=String(tag).toUpperCase();this.className="";this.attributes={};this.children=[];this.textContent="";}
  setAttribute(k,v){this.attributes[k]=String(v)} getAttribute(k){return this.attributes[k]||null}
  appendChild(v){this.children.push(v);return v}
}
function hasClass(node,name){return String(node.className||"").split(/\s+/).includes(name)}
function execute(search){
  const bundleNode={textContent:bundle},reportNode={textContent:""};
  const document={documentElement:{lang:"en"},title:"",createElement:t=>new Element(t),
    getElementById(id){if(id==="rm-standalone-target-bundle")return bundleNode;if(id==="rm-report-data")return reportNode;return null},
    querySelector(){return null},querySelectorAll(){return []}};
  const window={document,location:{search,hash:"#canvas",protocol:"file:",pathname:"/bucket/report.html"},
    __REPOMAP_WORKSPACE_TEST__:{},addEventListener(){},removeEventListener(){}};
  const context={window,document,URLSearchParams,Set,Map,AbortController,Promise};
  vm.runInNewContext(fs.readFileSync(process.argv[2],"utf8"),context);
  vm.runInNewContext(fs.readFileSync(process.argv[3].replace("script.js","ui_messages.js"),"utf8"),context);
  vm.runInNewContext(fs.readFileSync(process.argv[3],"utf8"),context);
  const data=JSON.parse(reportNode.textContent),tabs=new Element("nav");
  window.__REPOMAP_WORKSPACE_TEST__.renderAnalysisTargetMenu(tabs);
  const items=[];
  tabs.children.forEach(group=>group.children[1].children.forEach(row=>{
    const item=row.children[0];items.push({tag:item.tagName,href:item.attributes.href||"",
      active:hasClass(item,"rm-active"),disabled:hasClass(item,"rm-target-link--disabled")});
  }));
  return {search,ref:data.analysis_target.ref,sentinel:data.bundle_test_sentinel,
    current:data.target_navigation.current_target_index,def:data.target_navigation.default_target_index,
    version:data.target_navigation.version,language:document.documentElement.lang,title:document.title,items};
}
process.stdout.write(JSON.stringify(["","?target=0","?target=00","?target=%31","?target=1","?target=999","?target=0&target=2"].map(execute)));
`
	runnerPath := filepath.Join(t.TempDir(), "standalone-target-bootstrap.js")
	if err := os.WriteFile(runnerPath, []byte(runner), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(node, runnerPath, bootstrapPath, mainPath, bundlePath).CombinedOutput()
	if err != nil {
		t.Fatalf("run standalone target bootstrap: %v\n%s", err, output)
	}
	var got []struct {
		Search, Ref, Sentinel, Language, Title string
		Current, Def, Version                  int
		Items                                  []struct {
			Tag, Href        string
			Active, Disabled bool
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
		want := defaultIndex
		if result.Search == "?target=0" {
			want = 0
		}
		if result.Current != want || result.Def != defaultIndex ||
			result.Version != StandaloneTargetNavigationVersion ||
			result.Ref != container.Targets[want].Target.Ref ||
			result.Sentinel != fmt.Sprintf("PAYLOAD_%d", want) ||
			result.Language != "ru" || !strings.Contains(result.Title, "bundle-fixture") {
			t.Errorf("bootstrap %q = %#v, selected index %d", result.Search, result, want)
		}
		if len(result.Items) != len(container.Targets) {
			t.Fatalf("bootstrap menu %q = %#v", result.Search, result.Items)
		}
		for index, item := range result.Items {
			available := portfolio.Targets[index].State == snapshot.TargetPageReady
			if item.Active != (index == want) || item.Disabled == available {
				t.Errorf("bootstrap menu %q item %d = %#v", result.Search, index, item)
			}
			if available && item.Href != fmt.Sprintf("?target=%d#canvas", index) {
				t.Errorf("bootstrap menu href %q item %d = %q", result.Search, index, item.Href)
			}
			if !available && (item.Tag != "SPAN" || item.Href != "") {
				t.Errorf("bootstrap unavailable item %d = %#v", index, item)
			}
		}
	}
	if bytes.Contains(htmlBytes, []byte("report.json")) {
		t.Fatal("standalone target bundle noscript copy references an external report.json")
	}
	if !bytes.Contains(htmlBytes, []byte("Все данные уже встроены в этот HTML-файл")) {
		t.Fatal("standalone target bundle noscript copy does not disclose embedded data")
	}
}

func TestStandaloneTargetHrefResolvesForFileAndHostedObjectStorage(t *testing.T) {
	reference, err := url.Parse("?target=2#canvas")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ base, want string }{
		{
			base: "file:///Users/test/export/report.html?target=0#study-theme-2",
			want: "file:///Users/test/export/report.html?target=2#canvas",
		},
		{
			base: "https://bucket.example.test/maps/report.html?target=0#study-theme-2",
			want: "https://bucket.example.test/maps/report.html?target=2#canvas",
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
		"format_version":  CurrentFormatVersion,
		"analysis_target": target,
		"report_language": "ru",
		"repo_name":       "bundle-fixture",
		"github_source_links": map[string]any{
			"repository_url": "https://github.com/example/bundle",
			"revision":       standaloneBundleRevision,
		},
		"warnings": []string{"open " + localRoot + "/orientation.json"},
		"source_signal": map[string]any{
			"path": "cmd/server/main.go", "line": 3,
			"snippet": "LOCAL_SOURCE_BYTES", "content": "LOCAL_SOURCE_BYTES",
		},
		"target_navigation": map[string]any{
			"version": 2, "targets": []any{map[string]any{"href": "../private-run/report.html#/map"}},
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
	for _, forbidden := range []string{localRoot, "LOCAL_SOURCE_BYTES", "target_navigation", "private-run"} {
		if bytes.Contains(prepared.payload, []byte(forbidden)) {
			t.Errorf("prepared payload retained %q: %s", forbidden, prepared.payload)
		}
	}
	for _, required := range []string{
		`"repository_url":"https://github.com/example/bundle"`,
		`"revision":"` + standaloneBundleRevision + `"`,
		`"path":"cmd/server/main.go"`,
		`open [local path]/orientation.json`,
	} {
		if !bytes.Contains(prepared.payload, []byte(required)) {
			t.Errorf("prepared payload lost %q: %s", required, prepared.payload)
		}
	}
}

func TestStandaloneTargetBundleRejectsAuthorityDriftAndPreservesExistingHTML(t *testing.T) {
	container, portfolio := standaloneTargetBundleAuthorityFixture(t)
	ready := preparedStandaloneTargetFixtures(t, container, portfolio, filepath.Join(t.TempDir(), "private"))
	runDir := t.TempDir()
	htmlPath := filepath.Join(runDir, "report.html")
	const original = "ORIGINAL_REPORT"
	if err := os.WriteFile(htmlPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := map[string]func([]PreparedStandaloneTarget) []PreparedStandaloneTarget{
		"missing ready target": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			return values[:len(values)-1]
		},
		"revision drift": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			values = clonePreparedStandaloneTargets(values)
			values[len(values)-1].prepared.revision = strings.Repeat("f", 40)
			return values
		},
		"language drift": func(values []PreparedStandaloneTarget) []PreparedStandaloneTarget {
			values = clonePreparedStandaloneTargets(values)
			values[len(values)-1].prepared.language = "en"
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
	ordinary, err := RenderHTML(&ReportData{FormatVersion: CurrentFormatVersion, RepoName: "ordinary"})
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
	ready := preparedStandaloneTargetFixtures(t, container, portfolio, filepath.Join(t.TempDir(), "private"))
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
	repository := t.TempDir()
	files := map[string]string{
		"go.mod":             "module example.test/bundle\n\ngo 1.24\n",
		"cmd/server/main.go": "package main\nfunc main() {}\n",
		"cmd/worker/main.go": "package main\nfunc main() {}\n",
		"pkg/client.go":      "package pkg\nfunc Open() {}\n",
	}
	for name, content := range files {
		absolute := filepath.Join(repository, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "--", "go.mod", "cmd/server/main.go", "cmd/worker/main.go", "pkg/client.go"}} {
		command := exec.Command("git", append([]string{"-C", repository}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	deferred, err := snapshot.Build(snapshot.Options{RepoPath: repository, DeferAnalysisTargetResolution: true})
	if err != nil {
		t.Fatal(err)
	}
	if deferred.TargetCatalog == nil || len(deferred.TargetCatalog.Entries) < 3 {
		t.Fatalf("target catalog = %#v, want at least three targets", deferred.TargetCatalog)
	}
	refs := make([]string, 0, len(deferred.TargetCatalog.Entries))
	for _, entry := range deferred.TargetCatalog.Entries {
		refs = append(refs, entry.Candidate.Target.Ref)
	}
	container, err := snapshot.BuildTargetRunContainer(deferred, snapshot.TargetRunSelection{
		DefaultTargetRef: refs[len(refs)-1], TargetRefs: refs,
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make([]snapshot.TargetPageOutcome, 0, len(container.Targets))
	for index, projection := range container.Targets {
		outcome := snapshot.TargetPageOutcome{
			TargetRef: projection.Target.Ref,
			State:     snapshot.TargetPageReady,
			RunID:     fmt.Sprintf("20260811-01010%d-target-%d", index, index),
		}
		if index == 1 {
			outcome.State = snapshot.TargetPageUnavailable
			outcome.RunID = ""
			outcome.UnavailableCode = snapshot.TargetPageUnavailableTargetRunFailed
		}
		outcomes = append(outcomes, outcome)
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
	portfolio snapshot.TargetPagePortfolio,
	localRoot string,
) []PreparedStandaloneTarget {
	t.Helper()
	result := make([]PreparedStandaloneTarget, 0, len(container.Targets))
	for index, projection := range container.Targets {
		if portfolio.Targets[index].State != snapshot.TargetPageReady {
			continue
		}
		payload, err := json.Marshal(map[string]any{
			"format_version":       CurrentFormatVersion,
			"analysis_target":      projection.Target,
			"report_language":      "ru",
			"repo_name":            "bundle-fixture",
			"project_guess":        fmt.Sprintf("target %d", index),
			"bundle_test_sentinel": fmt.Sprintf("PAYLOAD_%d", index),
			"github_source_links": map[string]any{
				"repository_url": "https://github.com/example/bundle",
				"revision":       standaloneBundleRevision,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		result = append(result, PreparedStandaloneTarget{prepared: &preparedStandaloneTarget{
			target: projection.Target.Snapshot(), payload: payload,
			host: "GitHub", repositoryURL: "https://github.com/example/bundle",
			revision: standaloneBundleRevision, language: "ru",
			localizationState: PresentationLocalizationSucceeded,
			repoName:          "bundle-fixture", projectGuess: fmt.Sprintf("target %d", index),
			hasCanvas: index == 0, hasSurfaces: index == len(container.Targets)-1,
			localRoots: []string{localRoot},
		}})
	}
	return result
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

func standaloneReadyRunIDs(portfolio snapshot.TargetPagePortfolio) []string {
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
