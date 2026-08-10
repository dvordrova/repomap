package surfacediscovery

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/entrycall"
	"golang.org/x/tools/go/ssa"
)

func TestEntrySurfaceCandidatesSplitServeMuxMethodPatternIntoIndependentFacts(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/miniflux-shaped\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

import "net/http"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/users", createUser)
}

func createUser(http.ResponseWriter, *http.Request) {}
`)
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	result, err := Analyze(options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	substrate := result.EntryCallSubstrate
	if substrate == nil {
		t.Fatal("entry-call substrate was not captured")
	}
	if len(substrate.SurfaceCandidates) != 1 {
		t.Fatalf("surface candidates = %#v coverage=%#v, want one exact ServeMux registration", substrate.SurfaceCandidates, substrate.Coverage)
	}
	candidate := substrate.SurfaceCandidates[0]
	method := requireEntrySurfaceFact(t, candidate, entrycall.SurfaceFactToken, "POST", "main.go")
	path := requireEntrySurfaceFact(t, candidate, entrycall.SurfaceFactString, "/v1/users", "main.go")
	handler := requireEntrySurfaceFact(t, candidate, entrycall.SurfaceFactCallable, "createUser", "main.go")
	if method.ID == path.ID || method.Location != path.Location || method.Position != path.Position ||
		method.Label != "argument 1 method" || path.Label != "argument 1 path" {
		t.Fatalf("derived method/path authority = method %#v path %#v", method, path)
	}
	for _, fact := range candidate.Facts {
		if fact.Value == "POST /v1/users" {
			t.Fatalf("ambiguous combined fact remained provider-visible: %#v", candidate)
		}
	}
	if substrate.Coverage.SurfaceCandidatesConsidered != 1 ||
		substrate.Coverage.SurfaceCandidatesIndexed != 1 ||
		substrate.Coverage.SurfaceCandidateFactsConsidered != 4 ||
		substrate.Coverage.SurfaceCandidateFactsIndexed != 4 {
		t.Fatalf("derived fact accounting = %#v, want one candidate and four exact facts", substrate.Coverage)
	}

	compilation, err := entrycall.Compile(substrate.Snapshot())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(compilation.Request.SurfaceCatalog.Candidates) != 1 {
		t.Fatalf("compiled surface catalog = %#v", compilation.Request.SurfaceCatalog)
	}
	requestCandidate := compilation.Request.SurfaceCatalog.Candidates[0]
	factRef := func(kind entrycall.SurfaceFactKind, value string) string {
		t.Helper()
		for _, fact := range requestCandidate.Facts {
			if fact.Kind == kind && fact.Value == value {
				return fact.Ref
			}
		}
		t.Fatalf("request fact kind=%s value=%q not found in %#v", kind, value, requestCandidate)
		return ""
	}
	response := entrycall.Response{
		Version: entrycall.ResultVersion, RequestRef: compilation.Request.RequestRef,
		Entries: make([]entrycall.ResponseEntry, 0, len(compilation.Request.Entries)),
		SurfaceProposals: []entrycall.ResponseSurfaceProposal{{
			CandidateRef: requestCandidate.Ref, KindRef: entrycall.SurfaceKindRefHTTPRoute,
			Bindings: []entrycall.ResponseSurfaceBinding{
				{SlotRef: entrycall.SurfaceSlotRefMethod, FactRef: factRef(entrycall.SurfaceFactToken, "POST")},
				{SlotRef: entrycall.SurfaceSlotRefPath, FactRef: factRef(entrycall.SurfaceFactString, "/v1/users")},
				{SlotRef: entrycall.SurfaceSlotRefHandler, FactRef: factRef(entrycall.SurfaceFactCallable, "createUser")},
			},
		}},
	}
	for _, entry := range compilation.Request.Entries {
		response.Entries = append(response.Entries, entrycall.ResponseEntry{
			RootRef: entry.Ref, FamilyRefs: []string{},
		})
	}
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Marshal response: %v", err)
	}
	reduced, err := entrycall.Reduce(compilation, raw)
	if err != nil {
		t.Fatalf("Reduce independent method/path bindings: %v", err)
	}
	if reduced.SelectedSurfaceCount() != 1 || reduced.RejectedSurfaceCount() != 0 ||
		reduced.SurfaceProposals[0].Method.Text != "POST" ||
		reduced.SurfaceProposals[0].Path.Text != "/v1/users" ||
		reduced.SurfaceProposals[0].Handler.Text != "createUser" ||
		*reduced.SurfaceProposals[0].Method.Location != method.Location ||
		*reduced.SurfaceProposals[0].Path.Location != path.Location ||
		*reduced.SurfaceProposals[0].Handler.Location != handler.Location {
		t.Fatalf("restored ServeMux route = %#v", reduced)
	}
}

func TestEntrySurfaceCombinedHTTPPatternIsStrictAndKeepsSafetyGate(t *testing.T) {
	tests := []struct {
		value      string
		wantMethod string
		wantPath   string
		wantSplit  bool
	}{
		{value: "get /healthz", wantMethod: "get", wantPath: "/healthz", wantSplit: true},
		{value: "PROPFIND /dav"},
		{value: "GET healthz"},
		{value: "GET  /healthz"},
		{value: "GET /health z"},
		{value: "GET\t/healthz"},
		{value: " GET /healthz"},
		{value: "GET /\u00a0healthz"},
	}
	for _, test := range tests {
		method, path, split := splitEntrySurfaceHTTPPattern(test.value)
		if method != test.wantMethod || path != test.wantPath || split != test.wantSplit {
			t.Errorf("splitEntrySurfaceHTTPPattern(%q) = %q, %q, %v; want %q, %q, %v", test.value, method, path, split, test.wantMethod, test.wantPath, test.wantSplit)
		}
	}

	sidecar := newEntryCallSidecar()
	location := entrycall.Location{Path: "routes.go", Line: 7, Column: 2}
	if facts, safe := sidecar.safeDirectCallStringFacts(
		1, "argument 1", "GET /A1b2C3d4E5f6G7h8I9j0KLMNOPQRSTUV", location,
	); safe || len(facts) != 0 || sidecar.surfaceCoverage.UnsafeSurfaceCandidateFactsExcluded != 1 {
		t.Fatalf("derived path bypassed safety gate: facts=%#v safe=%v coverage=%#v", facts, safe, sidecar.surfaceCoverage)
	}
	facts, safe := sidecar.safeDirectCallStringFacts(1, "argument 1", "GET healthz", location)
	if !safe || len(facts) != 1 || facts[0].Kind != entrycall.SurfaceFactString || facts[0].Value != "GET healthz" {
		t.Fatalf("non-pattern ordinary string was rewritten: facts=%#v safe=%v", facts, safe)
	}
}

func TestEntrySurfaceCandidatesCaptureEchoShapedDirectCallWithoutFrameworkSeed(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/echo-shaped\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

type Router struct{}

type Registrar interface { GET(string, func()) }

func (*Router) GET(string, func()) {}

func main() {
	router := &Router{}
	register(router)
	dynamicRegister(router)
}

func register(router *Router) {
	router.GET("/account/:id", getAccount)
}

func dynamicRegister(router Registrar) {
	router.GET("/dynamic", getAccount)
}

func getAccount() {}
`)
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	first, err := Analyze(options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	second, err := Analyze(options)
	if err != nil {
		t.Fatalf("Analyze repeat: %v", err)
	}
	if first.EntryCallSubstrate == nil || second.EntryCallSubstrate == nil {
		t.Fatal("entry-call substrate was not captured")
	}
	if !reflect.DeepEqual(first.EntryCallSubstrate.SurfaceCandidates, second.EntryCallSubstrate.SurfaceCandidates) {
		t.Fatalf("candidate capture is not deterministic:\nfirst  %#v\nsecond %#v", first.EntryCallSubstrate.SurfaceCandidates, second.EntryCallSubstrate.SurfaceCandidates)
	}
	candidate := requireEntrySurfaceCandidateWithFact(t, first.EntryCallSubstrate, "/account/:id")
	if candidate.Form != entrycall.SurfaceCandidateDirectCall || candidate.Sketch != "GET" ||
		candidate.RootNodeID == "" || candidate.Site.Path != "main.go" {
		t.Fatalf("route candidate = %#v", candidate)
	}
	requireEntrySurfaceFact(t, candidate, entrycall.SurfaceFactToken, "GET", "main.go")
	requireEntrySurfaceFact(t, candidate, entrycall.SurfaceFactCallable, "getAccount", "main.go")
	for _, exact := range first.EntryCallSubstrate.SurfaceCandidates {
		for _, fact := range exact.Facts {
			if fact.Value == "/dynamic" {
				t.Fatalf("interface dispatch became an exact direct-call candidate: %#v", exact)
			}
		}
	}
	for _, trigger := range first.Catalog.Triggers {
		if trigger.Kind == "http_route" {
			t.Fatalf("generic candidate mutated deterministic Trigger Catalog: %#v", trigger)
		}
	}
	compilation, err := entrycall.Compile(first.EntryCallSubstrate.Snapshot())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	wire, err := entrycall.ProviderVisibleJSON(compilation)
	if err != nil {
		t.Fatalf("ProviderVisibleJSON: %v", err)
	}
	encoded := string(wire)
	for _, expected := range []string{"GET", "/account/:id", "getAccount"} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("provider request omitted bounded candidate label %q: %s", expected, encoded)
		}
	}
	for _, private := range []string{"example.com/echo-shaped", "main.go", "entry-surface-candidate-"} {
		if strings.Contains(encoded, private) {
			t.Fatalf("provider request leaked private authority %q: %s", private, encoded)
		}
	}
}

func TestEntrySurfaceCandidatesCaptureGenericPathDescriptorWithoutCallable(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/path-descriptor\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

type Controller struct{}

func Router(string, *Controller, string) {}
func Label(string, string) {}
func GET(string, func()) {}

func main() {
	register()
}

func register() {
	Router("/api/signup", &Controller{}, "POST:Signup")
	Label("/not-a-route", "documentation")
	GET("/healthz", healthz)
}

func healthz() {}
`)
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	result, err := Analyze(options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	substrate := result.EntryCallSubstrate
	if substrate == nil {
		t.Fatal("entry-call substrate was not captured")
	}
	if len(substrate.SurfaceCandidates) != 3 {
		t.Fatalf("surface candidates = %#v coverage=%#v, want strong route and two weak descriptors", substrate.SurfaceCandidates, substrate.Coverage)
	}
	descriptor := requireEntrySurfaceCandidateWithFact(t, substrate, "/api/signup")
	if descriptor.Form != entrycall.SurfaceCandidateDirectCall || descriptor.Sketch != "Router" ||
		descriptor.Site.Path != "main.go" {
		t.Fatalf("path descriptor candidate = %#v", descriptor)
	}
	requireEntrySurfaceFact(t, descriptor, entrycall.SurfaceFactString, "POST:Signup", "main.go")
	for _, fact := range descriptor.Facts {
		if fact.Kind == entrycall.SurfaceFactCallable {
			t.Fatalf("controller object became a fabricated callable: %#v", descriptor)
		}
	}

	compilation, err := entrycall.Compile(substrate.Snapshot())
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := len(compilation.Request.SurfaceCatalog.Candidates); got != 3 {
		t.Fatalf("advertised surface candidates = %d, want 3", got)
	}
	first := compilation.Request.SurfaceCatalog.Candidates[0]
	strongFirst := false
	for _, fact := range first.Facts {
		strongFirst = strongFirst || fact.Value == "/healthz"
	}
	if !strongFirst {
		t.Fatalf("weaker path descriptors ranked ahead of callable-complete route: %#v", compilation.Request.SurfaceCatalog.Candidates)
	}
}

func TestEntrySurfaceCandidatesCaptureCobraShapedInitCompositeWithoutFieldAllowlist(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/command-shaped\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

type Descriptor struct {
	Use string
	RunE func() error
}

var serveCommand = &Descriptor{
	Use: "serve [address]",
	RunE: runServe,
}

func main() { _ = serveCommand }

func runServe() error { return nil }
`)
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	result, err := Analyze(options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if result.EntryCallSubstrate == nil {
		t.Fatal("entry-call substrate was not captured")
	}
	candidate := requireEntrySurfaceCandidateWithFact(t, result.EntryCallSubstrate, "serve [address]")
	if candidate.Form != entrycall.SurfaceCandidateKeyedComposite || candidate.Sketch != "Descriptor" ||
		candidate.Site.Path != "main.go" {
		t.Fatalf("command candidate = %#v", candidate)
	}
	identity := requireEntrySurfaceFact(t, candidate, entrycall.SurfaceFactString, "serve [address]", "main.go")
	if identity.Label != "Use" {
		t.Fatalf("identity field label = %q, want Use", identity.Label)
	}
	callback := requireEntrySurfaceFact(t, candidate, entrycall.SurfaceFactCallable, "runServe", "main.go")
	if callback.Label != "RunE" || callback.Location.Line != 15 {
		t.Fatalf("callback fact = %#v, want exact RunE declaration", callback)
	}
}

func TestEntrySurfaceCandidatesRejectSensitiveFactsAndRetainSafeSibling(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/safe-candidates\n\ngo 1.25\n")
	writeFixtureFile(t, filepath.Join(repository, "main.go"), `package main

type Router struct{}
func (*Router) GET(string, func()) {}

func main() {
	router := &Router{}
	router.GET("/safe", handler)
	router.GET("sk-0123456789abcdef", handler)
	router.GET("/A1b2C3d4E5f6G7h8I9j0KLMNOPQRSTUV", handler)
}

func handler() {}
`)
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	result, err := Analyze(options)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	substrate := result.EntryCallSubstrate
	if substrate == nil {
		t.Fatal("entry-call substrate was not captured")
	}
	if len(substrate.SurfaceCandidates) != 1 {
		t.Fatalf("surface candidates = %#v, want only safe sibling", substrate.SurfaceCandidates)
	}
	requireEntrySurfaceCandidateWithFact(t, substrate, "/safe")
	if substrate.Coverage.UnsafeSurfaceCandidateFactsExcluded == 0 {
		t.Fatalf("coverage = %#v, want unsafe fact frontier", substrate.Coverage)
	}
	for _, candidate := range substrate.SurfaceCandidates {
		for _, fact := range candidate.Facts {
			if strings.Contains(fact.Value, "sk-0123456789abcdef") ||
				strings.Contains(fact.Value, "A1b2C3d4E5f6G7h8I9j0KLMNOPQRSTUV") {
				t.Fatalf("sensitive fact retained: %#v", fact)
			}
		}
	}
}

func TestEntrySurfaceCandidateRawReservoirHasDeterministicHardFrontier(t *testing.T) {
	sidecar := newEntryCallSidecar()
	owner := &ssa.Function{}
	for index := 0; index < entrycall.MaxRawSurfaceCandidates+3; index++ {
		site := entrycall.Location{Path: "routes.go", Line: index + 1, Column: 1}
		facts := []entrycall.ExactSurfaceFact{
			exactEntrySurfaceFact(entrycall.SurfaceFactToken, 0, "terminal selector", "GET", site),
			exactEntrySurfaceFact(entrycall.SurfaceFactString, 1, "argument 1", "/route/"+strconv.Itoa(index), site),
			exactEntrySurfaceFact(entrycall.SurfaceFactCallable, 2, "argument 2", "handler", entrycall.Location{Path: "handler.go", Line: 1, Column: 1}),
		}
		sidecar.recordSurfaceCandidate(rawEntrySurfaceCandidate{
			owner: owner, form: entrycall.SurfaceCandidateDirectCall,
			sketch: "GET", site: site, facts: facts,
		}, true)
	}
	if len(sidecar.surfaceCandidates) != entrycall.MaxRawSurfaceCandidates ||
		sidecar.surfaceCoverage.SurfaceCandidateLimitExcluded != 3 ||
		sidecar.surfaceCoverage.SurfaceCandidatesConsidered != entrycall.MaxRawSurfaceCandidates+3 {
		t.Fatalf("reservoir=%d coverage=%#v", len(sidecar.surfaceCandidates), sidecar.surfaceCoverage)
	}
	for _, retained := range sidecar.surfaceCandidates {
		if retained.site.Line > entrycall.MaxRawSurfaceCandidates {
			t.Fatalf("deterministic source prefix did not win bound: %#v", retained)
		}
	}
}

func TestEntrySurfaceCandidateRawReservoirNeverLetsWeakDescriptorEvictStrongCandidate(t *testing.T) {
	sidecar := newEntryCallSidecar()
	owner := &ssa.Function{}
	for index := 0; index < entrycall.MaxRawSurfaceCandidates; index++ {
		site := entrycall.Location{Path: "weak.go", Line: index + 1, Column: 1}
		sidecar.recordSurfaceCandidate(rawEntrySurfaceCandidate{
			owner: owner, form: entrycall.SurfaceCandidateDirectCall, sketch: "Router", site: site,
			facts: []entrycall.ExactSurfaceFact{
				exactEntrySurfaceFact(entrycall.SurfaceFactToken, 0, "terminal selector", "Router", site),
				exactEntrySurfaceFact(entrycall.SurfaceFactString, 1, "argument 1", "/route/"+strconv.Itoa(index), site),
				exactEntrySurfaceFact(entrycall.SurfaceFactString, 2, "argument 2", "GET:Action", site),
			},
		}, true)
	}
	strongSite := entrycall.Location{Path: "zz-strong.go", Line: 1, Column: 1}
	strongHandler := entrycall.Location{Path: "zz-handler.go", Line: 1, Column: 1}
	sidecar.recordSurfaceCandidate(rawEntrySurfaceCandidate{
		owner: owner, form: entrycall.SurfaceCandidateDirectCall, sketch: "GET", site: strongSite,
		facts: []entrycall.ExactSurfaceFact{
			exactEntrySurfaceFact(entrycall.SurfaceFactToken, 0, "terminal selector", "GET", strongSite),
			exactEntrySurfaceFact(entrycall.SurfaceFactString, 1, "argument 1", "/strong", strongSite),
			exactEntrySurfaceFact(entrycall.SurfaceFactCallable, 2, "argument 2", "handler", strongHandler),
		},
	}, true)
	if len(sidecar.surfaceCandidates) != entrycall.MaxRawSurfaceCandidates {
		t.Fatalf("reservoir = %d, want %d", len(sidecar.surfaceCandidates), entrycall.MaxRawSurfaceCandidates)
	}
	retainedStrong := false
	for _, candidate := range sidecar.surfaceCandidates {
		retainedStrong = retainedStrong || candidate.site == strongSite
	}
	if !retainedStrong || sidecar.surfaceCoverage.SurfaceCandidateLimitExcluded != 1 {
		t.Fatalf("strong candidate was evicted by weak descriptors: retained=%v coverage=%#v", retainedStrong, sidecar.surfaceCoverage)
	}
}

func TestEntrySurfaceCandidatesRespectSelectedExecutableAndImportClosure(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/isolation\n\ngo 1.25\n")
	writeTargetScopeFile(t, repository, "cmd/app/main.go", `package main
type Router struct{}
func (*Router) GET(string, func()) {}
func handler() {}
func main() { (&Router{}).GET("/selected", handler) }
`)
	writeTargetScopeFile(t, repository, "cmd/tool/main.go", `package main
type Router struct{}
func (*Router) GET(string, func()) {}
func handler() {}
func main() { (&Router{}).GET("/other", handler) }
`)
	input := Input{
		RepositoryName: "isolation", ModuleDirs: []string{"."},
		Packages: []PackageInput{
			{Path: "example.com/isolation/cmd/app", ModuleDir: "."},
			{Path: "example.com/isolation/cmd/tool", ModuleDir: "."},
		},
		Entrypoints: []EntrypointInput{
			{Package: "example.com/isolation/cmd/app", PackageDir: "cmd/app", ModuleDir: ".", Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/app/main.go", Line: 5}}},
			{Package: "example.com/isolation/cmd/tool", PackageDir: "cmd/tool", ModuleDir: ".", Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/tool/main.go", Line: 5}}},
		},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target:app", Kind: AnalysisTargetExecutablePackage,
			ModuleID: "module:isolation", ModulePath: "example.com/isolation", ModuleDir: ".",
			PackagePath:    "example.com/isolation/cmd/app",
			TargetPackages: []string{"example.com/isolation/cmd/app"},
			Roots:          []AnalysisTargetRootInput{{Path: "cmd/app/main.go", Line: 5}},
		},
	}
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	result, err := AnalyzeWithInput(options, input)
	if err != nil {
		t.Fatalf("AnalyzeWithInput: %v", err)
	}
	substrate := result.EntryCallSubstrate
	if substrate == nil {
		t.Fatal("entry-call substrate was not captured")
	}
	requireEntrySurfaceCandidateWithFact(t, substrate, "/selected")
	for _, candidate := range substrate.SurfaceCandidates {
		for _, fact := range candidate.Facts {
			if fact.Value == "/other" {
				t.Fatalf("off-target executable candidate leaked: %#v", candidate)
			}
		}
	}
	if substrate.Coverage.UnreachableSurfaceCandidatesExcluded == 0 {
		t.Fatalf("coverage = %#v, want off-target candidate accounted as unreachable", substrate.Coverage)
	}
}

func TestEntrySurfaceCandidateReservoirCannotBeExhaustedByOffTargetExecutable(t *testing.T) {
	repository := t.TempDir()
	writeFixtureFile(t, filepath.Join(repository, "go.mod"), "module example.com/reachability\n\ngo 1.25\n")
	writeTargetScopeFile(t, repository, "cmd/z-app/main.go", `package main
type Router struct{}
func (*Router) GET(string, func()) {}
func handler() {}
func main() { (&Router{}).GET("/selected", handler) }
`)
	var flood strings.Builder
	flood.WriteString(`package main
type Router struct{}
func (*Router) GET(string, func()) {}
func handler() {}
func main() {
	router := &Router{}
`)
	offTargetCount := entrycall.MaxRawSurfaceCandidates + 1
	for index := 0; index < offTargetCount; index++ {
		flood.WriteString("\trouter.GET(\"/off/" + strconv.Itoa(index) + "\", handler)\n")
	}
	flood.WriteString("}\n")
	writeTargetScopeFile(t, repository, "cmd/a-tool/main.go", flood.String())

	input := Input{
		RepositoryName: "reachability", ModuleDirs: []string{"."},
		Packages: []PackageInput{
			{Path: "example.com/reachability/cmd/a-tool", ModuleDir: "."},
			{Path: "example.com/reachability/cmd/z-app", ModuleDir: "."},
		},
		Entrypoints: []EntrypointInput{
			{Package: "example.com/reachability/cmd/a-tool", PackageDir: "cmd/a-tool", ModuleDir: ".", Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/a-tool/main.go", Line: 5}}},
			{Package: "example.com/reachability/cmd/z-app", PackageDir: "cmd/z-app", ModuleDir: ".", Anchors: []EntrypointAnchorInput{{Kind: ProcessEntryAnchorGoMain, Path: "cmd/z-app/main.go", Line: 5}}},
		},
		AnalysisTarget: &AnalysisTargetInput{
			TargetRef: "target:z-app", Kind: AnalysisTargetExecutablePackage,
			ModuleID: "module:reachability", ModulePath: "example.com/reachability", ModuleDir: ".",
			PackagePath:    "example.com/reachability/cmd/z-app",
			TargetPackages: []string{"example.com/reachability/cmd/z-app"},
			Roots:          []AnalysisTargetRootInput{{Path: "cmd/z-app/main.go", Line: 5}},
		},
	}
	options := DefaultOptions(repository)
	options.CaptureEntryCallSubstrate = true
	result, err := AnalyzeWithInput(options, input)
	if err != nil {
		t.Fatalf("AnalyzeWithInput: %v", err)
	}
	substrate := result.EntryCallSubstrate
	if substrate == nil {
		t.Fatal("entry-call substrate was not captured")
	}
	requireEntrySurfaceCandidateWithFact(t, substrate, "/selected")
	if len(substrate.SurfaceCandidates) != 1 ||
		substrate.Coverage.SurfaceCandidatesConsidered != offTargetCount+1 ||
		substrate.Coverage.SurfaceCandidatesIndexed != 1 ||
		substrate.Coverage.UnreachableSurfaceCandidatesExcluded != offTargetCount ||
		substrate.Coverage.SurfaceCandidateLimitExcluded != 0 {
		t.Fatalf("target-scoped reservoir/coverage = %d / %#v", len(substrate.SurfaceCandidates), substrate.Coverage)
	}
}

func requireEntrySurfaceCandidateWithFact(
	t *testing.T,
	substrate *entrycall.Substrate,
	value string,
) entrycall.ExactSurfaceCandidate {
	t.Helper()
	if substrate == nil {
		t.Fatal("nil entry-call substrate")
	}
	for _, candidate := range substrate.SurfaceCandidates {
		for _, fact := range candidate.Facts {
			if fact.Value == value {
				return candidate
			}
		}
	}
	t.Fatalf("surface fact value %q not found: candidates=%#v coverage=%#v", value, substrate.SurfaceCandidates, substrate.Coverage)
	return entrycall.ExactSurfaceCandidate{}
}

func requireEntrySurfaceFact(
	t *testing.T,
	candidate entrycall.ExactSurfaceCandidate,
	kind entrycall.SurfaceFactKind,
	value string,
	path string,
) entrycall.ExactSurfaceFact {
	t.Helper()
	for _, fact := range candidate.Facts {
		if fact.Kind == kind && fact.Value == value && fact.Location.Path == path {
			return fact
		}
	}
	t.Fatalf("fact kind=%s value=%q path=%q not found in %#v", kind, value, path, candidate)
	return entrycall.ExactSurfaceFact{}
}
