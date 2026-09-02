package facts

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/gitfiles"
	"github.com/dvordrova/repomap/internal/programindex"
)

// synthetic assembles a small sealed ProgramIndex for one extractor.
type synthetic struct {
	t         *testing.T
	language  string
	name      string
	sources   []programindex.TargetSource
	seeds     []programindex.TargetSeedInput
	objects   []programindex.ObjectInput
	relations []programindex.RelationInput
}

func newSynthetic(t *testing.T, language, name string, paths ...string) *synthetic {
	t.Helper()
	result := &synthetic{t: t, language: language, name: name}
	for position, filePath := range paths {
		result.sources = append(result.sources, programindex.TargetSource{FileRef: "f" + itoa(position+1), Path: filePath})
	}
	return result
}

func loc(filePath string, line int) *programindex.Location {
	return &programindex.Location{Path: filePath, Line: line, Column: 1}
}

func (s *synthetic) object(ref string, kind programindex.ObjectKind, name, filePath string, line int, owner string) {
	input := programindex.ObjectInput{SourceRef: ref, Kind: kind, Name: name, Visibility: programindex.VisibilityPublic, OwnerRef: owner}
	if filePath != "" {
		input.Location = loc(filePath, line)
	}
	s.objects = append(s.objects, input)
}

func (s *synthetic) external(ref, packagePath, name string, kind programindex.ExternalAuthorityKind) {
	s.objects = append(s.objects, programindex.ObjectInput{
		SourceRef: ref, Kind: programindex.ObjectExternalSymbol, Name: packagePath + "." + name,
		Visibility: programindex.VisibilityPublic,
		External:   &programindex.ExternalSymbol{AuthorityKind: kind, PackagePath: packagePath, Name: name},
	})
}

func (s *synthetic) seed(ref string, kind programindex.SeedKind, filePath string, line int) {
	s.seeds = append(s.seeds, programindex.TargetSeedInput{ObjectRef: ref, Kind: kind, Location: loc(filePath, line)})
}

func (s *synthetic) relate(ref string, kind programindex.RelationKind, from string, to []string, location *programindex.Location, patterns ...programindex.RelationPatternInput) {
	resolution := programindex.ResolutionUnresolved
	switch len(to) {
	case 0:
	case 1:
		resolution = programindex.ResolutionExact
	default:
		resolution = programindex.ResolutionAlternatives
	}
	observed := len(to)
	if observed == 0 {
		observed = 1
	}
	s.relations = append(s.relations, programindex.RelationInput{
		SourceRef: ref, Kind: kind, FromRef: from, ToRefs: to, Resolution: resolution, Location: location,
		TargetsObserved: observed, Witnesses: []programindex.Witness{{Kind: "test"}}, WitnessesObserved: 1,
		Patterns: patterns, PatternsObserved: len(patterns),
	})
}

func (s *synthetic) callback(ref, from, to, relationRef, patternRef string, position int) {
	s.relations = append(s.relations, programindex.RelationInput{
		SourceRef: ref, Kind: programindex.RelationPassesCallback, FromRef: from, ToRefs: []string{to},
		Resolution: programindex.ResolutionExact, TargetsObserved: 1,
		Witnesses: []programindex.Witness{{Kind: "test"}}, WitnessesObserved: 1,
		Patterns: []programindex.RelationPatternInput{}, PatternsObserved: 0,
		SourceArgument: &programindex.PatternArgumentRefInput{RelationSourceRef: relationRef, PatternSourceRef: patternRef, Position: position},
	})
}

func (s *synthetic) index() programindex.Index {
	s.t.Helper()
	seeds := s.seeds
	if seeds == nil {
		seeds = []programindex.TargetSeedInput{}
	}
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64),
		SourceSHA256:   strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: s.language, Kind: "executable", Name: s.name, Selector: s.language + ":" + s.name,
			Sources: s.sources, AnchorFileRef: s.sources[0].FileRef, Seeds: seeds,
		},
		Objects:   s.objects,
		Relations: s.relations,
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: len(s.objects), RelationsObserved: len(s.relations)},
	})
	if err != nil {
		s.t.Fatalf("programindex.New: %v", err)
	}
	return index
}

func pattern(ref string, form programindex.PatternForm, selector string, location *programindex.Location, origins []string, arguments ...programindex.PatternArgumentInput) programindex.RelationPatternInput {
	input := programindex.RelationPatternInput{
		SourceRef: ref, Form: form, Selector: selector, Location: location,
		ReceiverOriginRefs: origins, ReceiverOriginsObserved: len(origins),
		Arguments: arguments, ArgumentsObserved: len(arguments),
	}
	if len(origins) > 0 {
		input.ReceiverOriginResolution = programindex.ResolutionAlternatives
	}
	return input
}

func literal(position int, value string) programindex.PatternArgumentInput {
	return programindex.PatternArgumentInput{Position: position, Kind: programindex.PatternLiteralString, Value: value}
}

func keyword(name, value string) programindex.PatternArgumentInput {
	return programindex.PatternArgumentInput{Keyword: name, Kind: programindex.PatternLiteralString, Value: value}
}

func dynamic(position int) programindex.PatternArgumentInput {
	return programindex.PatternArgumentInput{Position: position, Kind: programindex.PatternDynamic}
}

// dynamicRef is a dynamic argument that resolves exactly to one object, the
// shape a callback argument has before passes_callback names its target.
func dynamicRef(position int, ref string) programindex.PatternArgumentInput {
	return programindex.PatternArgumentInput{
		Position: position, Kind: programindex.PatternDynamic,
		ObjectRefs: []string{ref}, Resolution: programindex.ResolutionExact, ObjectsObserved: 1,
	}
}

// template builds a string_template argument; an empty part is a hole.
func template(position int, parts ...string) programindex.PatternArgumentInput {
	input := programindex.PatternArgumentInput{Position: position, Kind: programindex.PatternStringTemplate}
	for _, part := range parts {
		if part == "" {
			input.Parts = append(input.Parts, programindex.PatternPartInput{Kind: programindex.PatternPartHole})
			continue
		}
		input.Parts = append(input.Parts, programindex.PatternPartInput{Kind: programindex.PatternPartLiteral, Text: part})
	}
	return input
}

func newCorpus(t *testing.T, files map[string]string) *corpus.Corpus {
	t.Helper()
	root := t.TempDir()
	paths := make([]string, 0, len(files))
	for filePath, content := range files {
		paths = append(paths, filePath)
		full := filepath.Join(root, filepath.FromSlash(filePath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sort.Strings(paths)
	repository, err := corpus.New(context.Background(), root, gitfiles.Listing{Paths: paths, RegularPaths: paths})
	if err != nil {
		t.Fatalf("corpus.New: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return repository
}

func mustBuild(t *testing.T, input Input) Result {
	t.Helper()
	result, err := Build(input)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return result
}

func findFact(result Result, kind Kind, predicate func(Fact) bool) (Fact, bool) {
	for _, fact := range result.OfKind(kind) {
		if predicate(fact) {
			return fact, true
		}
	}
	return Fact{}, false
}

func requireFact(t *testing.T, result Result, kind Kind, description string, predicate func(Fact) bool) Fact {
	t.Helper()
	fact, ok := findFact(result, kind, predicate)
	if !ok {
		t.Fatalf("missing %s fact %s; have %+v", kind, description, result.OfKind(kind))
	}
	return fact
}

func hasDiagnostic(result Result, kind string) bool {
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Kind == kind {
			return true
		}
	}
	return false
}

func backendIndex(t *testing.T) programindex.Index {
	t.Helper()
	s := newSynthetic(t, "python", "main", "backend/main.py", "backend/app/app.py", "backend/app/settings.py")
	s.object("main", programindex.ObjectModule, "main", "backend/main.py", 1, "")
	s.object("appmod", programindex.ObjectModule, "app.app", "backend/app/app.py", 1, "")
	s.object("settingsmod", programindex.ObjectModule, "app.settings", "backend/app/settings.py", 1, "")
	s.object("app", programindex.ObjectVariable, "app", "backend/app/app.py", 15, "appmod")
	s.object("levels", programindex.ObjectFunction, "get_levels_info", "backend/app/app.py", 19, "appmod")
	s.object("level", programindex.ObjectFunction, "get_level", "backend/app/app.py", 60, "appmod")
	s.object("run", programindex.ObjectFunction, "run_level", "backend/app/app.py", 75, "appmod")
	s.object("settings", programindex.ObjectType, "Settings", "backend/app/settings.py", 4, "settingsmod")
	s.external("fastapi", "fastapi", "FastAPI", programindex.ExternalAuthorityPackage)
	s.external("field", "pydantic", "Field", programindex.ExternalAuthorityPackage)
	s.seed("main", programindex.SeedMainGuard, "backend/main.py", 14)
	s.relate("imp-app", programindex.RelationImports, "main", []string{"app"}, loc("backend/main.py", 12))
	s.relate("imp-settings", programindex.RelationImports, "main", []string{"settings"}, loc("backend/main.py", 6))
	s.relate("dec-levels", programindex.RelationDecorates, "levels", nil, loc("backend/app/app.py", 18),
		pattern("p", programindex.PatternDecoratorCall, "get", loc("backend/app/app.py", 18), []string{"fastapi"}, literal(1, "/api/levels")))
	s.relate("dec-level", programindex.RelationDecorates, "level", nil, loc("backend/app/app.py", 59),
		pattern("p", programindex.PatternDecoratorCall, "get", loc("backend/app/app.py", 59), []string{"fastapi"}, literal(1, "/api/level/{level_id}")))
	s.relate("dec-run", programindex.RelationDecorates, "run", nil, loc("backend/app/app.py", 74),
		pattern("p", programindex.PatternDecoratorCall, "post", loc("backend/app/app.py", 74), []string{"fastapi"}, literal(1, "/api/level/run")))
	s.relate("cfg-host", programindex.RelationInvokesExternal, "settings", []string{"field"}, loc("backend/app/settings.py", 5),
		pattern("p", programindex.PatternCall, "Field", loc("backend/app/settings.py", 5), nil, keyword("default", "0.0.0.0"), keyword("env", "APP_HOST")))
	s.relate("cfg-port", programindex.RelationInvokesExternal, "settings", []string{"field"}, loc("backend/app/settings.py", 6),
		pattern("p", programindex.PatternCall, "Field", loc("backend/app/settings.py", 6), nil, programindex.PatternArgumentInput{Keyword: "default", Kind: programindex.PatternDynamic}, keyword("env", "APP_PORT")))
	return s.index()
}

func frontIndex(t *testing.T, calls ...programindex.PatternArgumentInput) programindex.Index {
	t.Helper()
	s := newSynthetic(t, "typescript", "front", "front/package.json", "front/src/index.tsx", "front/src/service/http.ts")
	s.object("index", programindex.ObjectModule, "src/index", "front/src/index.tsx", 1, "")
	s.object("root", programindex.ObjectVariable, "root", "front/src/index.tsx", 7, "index")
	s.object("http", programindex.ObjectModule, "src/service/http", "front/src/service/http.ts", 1, "")
	s.external("axios-get", "axios", "get", programindex.ExternalAuthorityPackage)
	s.external("axios-post", "axios", "post", programindex.ExternalAuthorityPackage)
	s.seed("root", programindex.SeedBoundObject, "front/src/index.tsx", 7)
	s.relate("imp-http", programindex.RelationImports, "index", []string{"http"}, loc("front/src/index.tsx", 2))
	for position, argument := range calls {
		line := 10 + position*10
		function := "fn" + itoa(position)
		s.object(function, programindex.ObjectFunction, "call"+itoa(position), "front/src/service/http.ts", line, "http")
		s.object(function+".response", programindex.ObjectVariable, "call"+itoa(position)+".response", "front/src/service/http.ts", line+2, function)
		selector, callee := "get", "axios-get"
		if argument.Keyword == "post" {
			selector, callee = "post", "axios-post"
			argument.Keyword = ""
		}
		s.relate("call"+itoa(position), programindex.RelationInvokesExternal, function+".response", []string{callee}, loc("front/src/service/http.ts", line+2),
			pattern("p", programindex.PatternCall, selector, loc("front/src/service/http.ts", line+2), nil, argument))
	}
	return s.index()
}

func postCall(path string) programindex.PatternArgumentInput {
	argument := literal(1, path)
	argument.Keyword = "post"
	return argument
}

func TestBuildTargetsAndEntrypoints(t *testing.T) {
	result := mustBuild(t, Input{Revision: "78714d34ee", Targets: []TargetInput{{Index: backendIndex(t), RunID: "run-1"}}})
	if len(result.Targets) != 1 {
		t.Fatalf("targets = %+v", result.Targets)
	}
	target := result.Targets[0]
	if target.Root != "backend" || target.Language != "python" || target.Anchor.String() != "backend/main.py:14" || target.RunID != "run-1" {
		t.Fatalf("target = %+v", target)
	}
	if target.ID != NewTargetID("python", "backend", "") {
		t.Fatalf("target id %q is not derived from language/root/manifest", target.ID)
	}
	entrypoint := requireFact(t, result, KindEntrypoint, "main guard", func(fact Fact) bool { return fact.Anchor.String() == "backend/main.py:14" })
	if entrypoint.Symbol != "main" || entrypoint.Key != "main_guard" || entrypoint.TargetID != target.ID {
		t.Fatalf("entrypoint = %+v", entrypoint)
	}
}

func TestBuildRoutesFromDecorators(t *testing.T) {
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: backendIndex(t), Root: "backend", Manifest: "backend/Pipfile"}}})
	routes := result.OfKind(KindHTTPRoute)
	if len(routes) != 3 {
		t.Fatalf("routes = %+v", routes)
	}
	route := requireFact(t, result, KindHTTPRoute, "GET /api/levels", func(fact Fact) bool { return fact.Path == "/api/levels" })
	if route.Method != "GET" || route.Symbol != "get_levels_info" || route.Anchor.String() != "backend/app/app.py:18" || route.Resolution != ResolutionExact {
		t.Fatalf("route = %+v", route)
	}
	if run, _ := findFact(result, KindHTTPRoute, func(fact Fact) bool { return fact.Method == "POST" }); run.Path != "/api/level/run" || run.Symbol != "run_level" {
		t.Fatalf("post route = %+v", run)
	}
}

func TestBuildConfigReadsFromPatterns(t *testing.T) {
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: backendIndex(t), Root: "backend"}}})
	host := requireFact(t, result, KindConfigRead, "APP_HOST", func(fact Fact) bool { return fact.Key == "APP_HOST" })
	if host.Value != "0.0.0.0" || host.Anchor.String() != "backend/app/settings.py:5" || host.Symbol != "app.settings" || host.Resolution != ResolutionExact {
		t.Fatalf("host = %+v", host)
	}
	port := requireFact(t, result, KindConfigRead, "APP_PORT", func(fact Fact) bool { return fact.Key == "APP_PORT" })
	if port.Value != "" || port.Anchor.Line != 6 {
		t.Fatalf("port = %+v", port)
	}
}

func TestBuildClientCallsAndPortals(t *testing.T) {
	backend := backendIndex(t)
	front := frontIndex(t, literal(1, "/api/levels"), template(1, "/api/level/", ""), postCall("/api/level/run"), literal(1, "/api/nothing"))
	result := mustBuild(t, Input{Targets: []TargetInput{
		{Index: backend, Root: "backend", Manifest: "backend/Pipfile"},
		{Index: front, Root: "front", Manifest: "front/package.json"},
	}})
	calls := result.OfKind(KindHTTPCall)
	if len(calls) != 4 {
		t.Fatalf("calls = %+v", calls)
	}
	templated := requireFact(t, result, KindHTTPCall, "template", func(fact Fact) bool { return fact.Path == "/api/level/{param}" })
	if templated.Resolution != ResolutionPossible || templated.Method != "GET" || templated.Symbol != "call1" || templated.Anchor.String() != "front/src/service/http.ts:22" {
		t.Fatalf("templated call = %+v", templated)
	}
	portals := result.OfKind(KindPortal)
	if len(portals) != 3 {
		t.Fatalf("portals = %+v", portals)
	}
	exact := requireFact(t, result, KindPortal, "GET /api/levels", func(fact Fact) bool { return fact.Path == "/api/levels" })
	if exact.Resolution != ResolutionExact || exact.Anchor.String() != "front/src/service/http.ts:12" || exact.Evidence[0].String() != "backend/app/app.py:18" {
		t.Fatalf("exact portal = %+v", exact)
	}
	if exact.TargetID != result.Targets[1].ID || exact.PeerTargetID != result.Targets[0].ID || exact.Refs[0] != templatedOrExact(result, "/api/levels").ID {
		t.Fatalf("exact portal targets/refs = %+v", exact)
	}
	possible := requireFact(t, result, KindPortal, "GET /api/level/{level_id}", func(fact Fact) bool { return fact.Path == "/api/level/{level_id}" })
	if possible.Resolution != ResolutionPossible || possible.Anchor.Line != 22 || possible.Evidence[0].Line != 59 {
		t.Fatalf("possible portal = %+v", possible)
	}
	if !hasDiagnostic(result, "portal_unmatched") {
		t.Fatalf("expected an unmatched diagnostic, have %+v", result.Diagnostics)
	}
}

func templatedOrExact(result Result, path string) Fact {
	fact, _ := findFact(result, KindHTTPCall, func(fact Fact) bool { return fact.Path == path })
	return fact
}

func TestBuildPortalAmbiguityIsDiagnosed(t *testing.T) {
	s := newSynthetic(t, "python", "api", "api/main.py")
	s.object("main", programindex.ObjectModule, "main", "api/main.py", 1, "")
	s.object("app", programindex.ObjectVariable, "app", "api/main.py", 3, "main")
	s.object("latest", programindex.ObjectFunction, "latest", "api/main.py", 10, "main")
	s.object("byid", programindex.ObjectFunction, "by_id", "api/main.py", 20, "main")
	s.external("flask", "flask", "Flask", programindex.ExternalAuthorityPackage)
	s.seed("main", programindex.SeedMainGuard, "api/main.py", 30)
	s.relate("r1", programindex.RelationDecorates, "latest", nil, loc("api/main.py", 9),
		pattern("p", programindex.PatternDecoratorCall, "get", loc("api/main.py", 9), []string{"flask"}, literal(1, "/api/item/latest")))
	s.relate("r2", programindex.RelationDecorates, "byid", nil, loc("api/main.py", 19),
		pattern("p", programindex.PatternDecoratorCall, "get", loc("api/main.py", 19), []string{"flask"}, literal(1, "/api/item/<int:item_id>")))
	front := frontIndex(t, template(1, "/api/item/", ""))
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "api"}, {Index: front, Root: "front"}}})
	if portals := result.OfKind(KindPortal); len(portals) != 0 {
		t.Fatalf("ambiguous call produced portals %+v", portals)
	}
	if !hasDiagnostic(result, "portal_ambiguous") {
		t.Fatalf("diagnostics = %+v", result.Diagnostics)
	}
}

func TestBuildRouteWithCallbackHandler(t *testing.T) {
	s := newSynthetic(t, "go", "server", "cmd/server/main.go")
	s.object("pkg", programindex.ObjectPackage, "main", "", 0, "")
	s.object("main", programindex.ObjectFunction, "main", "cmd/server/main.go", 10, "pkg")
	s.object("handler", programindex.ObjectFunction, "listArticles", "cmd/server/main.go", 30, "pkg")
	s.external("chi", "github.com/go-chi/chi/v5", "Get", programindex.ExternalAuthorityPackage)
	s.seed("main", programindex.SeedCallable, "cmd/server/main.go", 10)
	s.relate("reg", programindex.RelationInvokesExternal, "main", []string{"chi"}, loc("cmd/server/main.go", 12),
		pattern("p", programindex.PatternCall, "Get", loc("cmd/server/main.go", 12), nil, literal(1, "/articles"), dynamicRef(2, "handler")))
	s.callback("cb", "main", "handler", "reg", "p", 2)
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "cmd/server"}}})
	route := requireFact(t, result, KindHTTPRoute, "GET /articles", func(fact Fact) bool { return fact.Path == "/articles" })
	if route.Method != "GET" || route.Symbol != "listArticles" || route.Anchor.String() != "cmd/server/main.go:12" {
		t.Fatalf("route = %+v", route)
	}
	if dead := result.OfKind(KindDeadModule); len(dead) != 0 {
		t.Fatalf("package object without location produced dead modules %+v", dead)
	}
}

func TestBuildConfigReadsFromCorpusRegex(t *testing.T) {
	repository := newCorpus(t, map[string]string{
		"svc/config.py":   "import os\nDB = os.environ[\"DATABASE_URL\"]\nPORT = os.environ.get('PORT')\n# os.getenv(\"IGNORED\")\n",
		"svc/client.ts":   "const base = process.env.REACT_APP_API;\n",
		"other/tool.py":   "x = os.getenv(\"NOT_IN_TARGET\")\n",
		"svc/notes.md":    "os.environ[\"NOT_SOURCE\"]\n",
		"svc/__init__.py": "",
	})
	s := newSynthetic(t, "python", "svc", "svc/config.py")
	s.object("mod", programindex.ObjectModule, "config", "svc/config.py", 1, "")
	s.object("fn", programindex.ObjectFunction, "load", "svc/config.py", 2, "mod")
	s.seed("mod", programindex.SeedMainGuard, "svc/config.py", 1)
	result := mustBuild(t, Input{Repository: repository, Targets: []TargetInput{{Index: s.index(), Root: "svc"}}})
	keys := make(map[string]Fact)
	for _, fact := range result.OfKind(KindConfigRead) {
		keys[fact.Key] = fact
	}
	for _, unwanted := range []string{"IGNORED", "NOT_IN_TARGET", "NOT_SOURCE"} {
		if _, found := keys[unwanted]; found {
			t.Fatalf("unexpected config key %s in %+v", unwanted, keys)
		}
	}
	database, ok := keys["DATABASE_URL"]
	if !ok || database.Anchor.String() != "svc/config.py:2" || database.Resolution != ResolutionPossible || database.Symbol != "load" {
		t.Fatalf("DATABASE_URL = %+v (%v)", database, ok)
	}
	if port := keys["PORT"]; port.Anchor.Line != 3 {
		t.Fatalf("PORT = %+v", port)
	}
	if api := keys["REACT_APP_API"]; api.Anchor.String() != "svc/client.ts:1" {
		t.Fatalf("REACT_APP_API = %+v", api)
	}
}

func TestBuildRisks(t *testing.T) {
	repository := newCorpus(t, map[string]string{
		"svc/field.py": "class Field:\n    def make_step(self):\n        exec(code, {}, {})\n        subprocess.run(cmd)\n        # os.system('never')\n        os.system('ls')\n        data = json.loads(raw)\n",
		"svc/ui.tsx":   "const m = pattern.exec(text);\nconst f = new Function('return 1');\n<div dangerouslySetInnerHTML={{__html: raw}} />\n",
	})
	s := newSynthetic(t, "python", "svc", "svc/field.py", "svc/ui.tsx")
	s.object("mod", programindex.ObjectModule, "field", "svc/field.py", 1, "")
	s.object("type", programindex.ObjectType, "Field", "svc/field.py", 1, "mod")
	s.object("step", programindex.ObjectMethod, "make_step", "svc/field.py", 2, "type")
	s.object("ui", programindex.ObjectModule, "ui", "svc/ui.tsx", 1, "")
	s.external("run", "subprocess", "run", programindex.ExternalAuthorityPackage)
	s.external("loads", "json", "loads", programindex.ExternalAuthorityPackage)
	s.seed("mod", programindex.SeedMainGuard, "svc/field.py", 1)
	s.relate("exec", programindex.RelationCalls, "step", nil, loc("svc/field.py", 3),
		pattern("p", programindex.PatternCall, "exec", loc("svc/field.py", 3), nil, dynamic(1), dynamic(2), dynamic(3)))
	s.relate("run", programindex.RelationInvokesExternal, "step", []string{"run"}, loc("svc/field.py", 4),
		pattern("p", programindex.PatternCall, "run", loc("svc/field.py", 4), nil, dynamic(1)))
	s.relate("loads", programindex.RelationInvokesExternal, "step", []string{"loads"}, loc("svc/field.py", 7),
		pattern("p", programindex.PatternCall, "loads", loc("svc/field.py", 7), nil, dynamic(1)))
	s.relate("jsexec", programindex.RelationCalls, "ui", nil, loc("svc/ui.tsx", 1),
		pattern("p", programindex.PatternCall, "exec", loc("svc/ui.tsx", 1), nil, dynamic(1)))
	result := mustBuild(t, Input{Repository: repository, Targets: []TargetInput{{Index: s.index(), Root: "svc"}}})
	byAnchor := make(map[string]Fact)
	for _, fact := range result.OfKind(KindRisk) {
		byAnchor[fact.Anchor.String()] = fact
	}
	exec := byAnchor["svc/field.py:3"]
	if exec.Key != "exec" || exec.Symbol != "make_step" || exec.Text != "exec(code, {}, {})" || exec.Resolution != ResolutionExact {
		t.Fatalf("exec = %+v", exec)
	}
	if run := byAnchor["svc/field.py:4"]; run.Key != "subprocess.run" {
		t.Fatalf("subprocess.run = %+v", run)
	}
	if system := byAnchor["svc/field.py:6"]; system.Key != "os.system" || system.Resolution != ResolutionPossible {
		t.Fatalf("os.system = %+v", system)
	}
	for _, clean := range []string{"svc/field.py:5", "svc/field.py:7", "svc/ui.tsx:1"} {
		if fact, found := byAnchor[clean]; found {
			t.Fatalf("unexpected risk at %s: %+v", clean, fact)
		}
	}
	if byAnchor["svc/ui.tsx:2"].Key != "new Function" || byAnchor["svc/ui.tsx:3"].Key != "dangerouslySetInnerHTML" {
		t.Fatalf("javascript risks = %+v", byAnchor)
	}
}

func TestBuildTODOs(t *testing.T) {
	repository := newCorpus(t, map[string]string{
		"svc/app.py":   "x = 1\n# TODO\ny = 2  # FIXME: handle errors\n",
		"README.md":    strings.Repeat("words ", 40) + "\nHACK around it\n",
		"assets/a.bin": "\x00\x01binary TODO",
	})
	s := newSynthetic(t, "python", "svc", "svc/app.py")
	s.object("mod", programindex.ObjectModule, "app", "svc/app.py", 1, "")
	s.seed("mod", programindex.SeedMainGuard, "svc/app.py", 1)
	result := mustBuild(t, Input{Repository: repository, Targets: []TargetInput{{Index: s.index(), Root: "svc"}}})
	todos := result.OfKind(KindTODO)
	if len(todos) != 3 {
		t.Fatalf("todos = %+v", todos)
	}
	bare := requireFact(t, result, KindTODO, "bare TODO", func(fact Fact) bool { return fact.Anchor.String() == "svc/app.py:2" })
	if bare.Text != "TODO" || bare.Key != "TODO" || bare.TargetID != result.Targets[0].ID {
		t.Fatalf("bare todo = %+v", bare)
	}
	fixme := requireFact(t, result, KindTODO, "FIXME", func(fact Fact) bool { return fact.Key == "FIXME" })
	if fixme.Text != "handle errors" {
		t.Fatalf("fixme = %+v", fixme)
	}
	hack := requireFact(t, result, KindTODO, "HACK", func(fact Fact) bool { return fact.Key == "HACK" })
	if hack.TargetID != "" || hack.Text != "around it" {
		t.Fatalf("hack = %+v", hack)
	}
}

func TestBuildDeadModulesAndImports(t *testing.T) {
	s := newSynthetic(t, "typescript", "front", "front/src/index.tsx", "front/src/a.ts", "front/src/b.ts", "front/src/c.ts", "front/src/env.d.ts")
	s.object("index", programindex.ObjectModule, "src/index", "front/src/index.tsx", 1, "")
	s.object("root", programindex.ObjectVariable, "root", "front/src/index.tsx", 7, "index")
	s.object("a", programindex.ObjectModule, "src/a", "front/src/a.ts", 1, "")
	s.object("afn", programindex.ObjectFunction, "helper", "front/src/a.ts", 3, "a")
	s.object("b", programindex.ObjectModule, "src/b", "front/src/b.ts", 1, "")
	s.object("c", programindex.ObjectModule, "src/c", "front/src/c.ts", 1, "")
	s.object("env", programindex.ObjectModule, "src/env", "front/src/env.d.ts", 1, "")
	s.seed("root", programindex.SeedBoundObject, "front/src/index.tsx", 7)
	s.relate("i1", programindex.RelationImports, "index", []string{"afn"}, loc("front/src/index.tsx", 2))
	s.relate("c1", programindex.RelationCalls, "root", []string{"b"}, loc("front/src/index.tsx", 9))
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "front"}}})
	dead := result.OfKind(KindDeadModule)
	if len(dead) != 1 || dead[0].Path != "front/src/c.ts" || dead[0].Anchor.String() != "front/src/c.ts:1" {
		t.Fatalf("dead = %+v", dead)
	}
	imports := result.OfKind(KindImport)
	if len(imports) != 1 || imports[0].Path != "front/src/a.ts" || imports[0].Anchor.String() != "front/src/index.tsx:2" {
		t.Fatalf("imports = %+v", imports)
	}
}

func TestBuildDeadModulesNeedSeeds(t *testing.T) {
	s := newSynthetic(t, "python", "lib", "lib/a.py", "lib/b.py")
	s.object("a", programindex.ObjectModule, "a", "lib/a.py", 1, "")
	s.object("b", programindex.ObjectModule, "b", "lib/b.py", 1, "")
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "lib"}}})
	if dead := result.OfKind(KindDeadModule); len(dead) != 0 {
		t.Fatalf("library without seeds produced dead modules %+v", dead)
	}
	if !hasDiagnostic(result, "dead_module_skipped") {
		t.Fatalf("diagnostics = %+v", result.Diagnostics)
	}
	if result.Targets[0].Anchor.String() != "lib/a.py:1" {
		t.Fatalf("seedless target anchor = %+v", result.Targets[0].Anchor)
	}
}

func TestBuildNegatives(t *testing.T) {
	bare := newCorpus(t, map[string]string{"README.md": "# tiny", "src/main.py": "print(1)\n"})
	result := mustBuild(t, Input{Repository: bare})
	names := make(map[string]Fact)
	for _, fact := range result.OfKind(KindNegative) {
		names[fact.Key] = fact
	}
	for _, name := range []string{NegativeReadmeTooShort, NegativeNoTests, NegativeNoDockerfile, NegativeNoCI} {
		if _, found := names[name]; !found {
			t.Fatalf("missing negative %s in %+v", name, names)
		}
	}
	if readme := names[NegativeReadmeTooShort]; readme.Anchor.String() != "README.md:1" || readme.Text != "README.md is 6 bytes" {
		t.Fatalf("readme negative = %+v", readme)
	}
	if len(names) != 4 {
		t.Fatalf("negatives = %+v", names)
	}

	complete := newCorpus(t, map[string]string{
		"README.md":                strings.Repeat("documentation ", 20),
		"Dockerfile":               "FROM scratch\n",
		".github/workflows/ci.yml": "on: push\n",
		"tests/test_main.py":       "def test_x(): pass\n",
	})
	if negatives := mustBuild(t, Input{Repository: complete}).OfKind(KindNegative); len(negatives) != 0 {
		t.Fatalf("complete repository has negatives %+v", negatives)
	}
	missing := newCorpus(t, map[string]string{"src/x.go": "package x\n", "src/x_test.go": "package x\n"})
	noReadme := mustBuild(t, Input{Repository: missing})
	if _, found := findFact(noReadme, KindNegative, func(fact Fact) bool { return fact.Key == NegativeNoReadme }); !found {
		t.Fatalf("no_readme missing in %+v", noReadme.OfKind(KindNegative))
	}
	if _, found := findFact(noReadme, KindNegative, func(fact Fact) bool { return fact.Key == NegativeNoTests }); found {
		t.Fatalf("_test.go file did not count as a test")
	}
}

func TestBuildManifests(t *testing.T) {
	repository := newCorpus(t, map[string]string{
		"front/package.json": "{\n  \"name\": \"front\",\n  \"dependencies\": {\n    \"axios\": \"1.6.2\",\n    \"react\": \"^18.2.0\"\n  },\n  \"scripts\": {\n    \"start\": \"react-scripts start\",\n    \"build\": \"react-scripts build\"\n  },\n  \"engines\": {\"node\": \">=18\"},\n  \"bin\": \"cli.js\",\n  \"proxy\": \"http://localhost:8080\"\n}\n",
		"backend/Pipfile":    "[[source]]\nname = \"pypi\"\n\n[dev-packages]\npytest = \"*\"\n\n[packages]\nfastapi = \"*\"\npydantic = {extras = [\"dotenv\"], version = \"2.1\"}\n\n[requires]\npython_version = \"3.12\"\n",
		"pyproject.toml":     "[project]\nname = \"tool\"\nrequires-python = \">=3.11\"\ndependencies = [\n  \"httpx>=0.27\",\n  \"click\",\n]\n",
		"requirements.txt":   "flask==3.0.0\nrequests>=2\n# comment\n",
		"go.mod":             "module example.com/app\n\ngo 1.22\n",
	})
	result := mustBuild(t, Input{Repository: repository, TrackedPaths: []string{"backend/.env", "README.md"}})
	rows := make(map[string]Fact)
	for _, fact := range result.OfKind(KindManifest) {
		rows[fact.Anchor.Path+"#"+fact.Key] = fact
	}
	expect := func(path, key, value string, line int) {
		t.Helper()
		fact, ok := rows[path+"#"+key]
		if !ok || fact.Value != value || fact.Anchor.Line != line {
			t.Fatalf("%s %s = %+v (found %v), want value %q line %d", path, key, fact, ok, value, line)
		}
	}
	expect("front/package.json", "scripts.start", "react-scripts start", 8)
	expect("front/package.json", "scripts.build", "react-scripts build", 9)
	expect("front/package.json", "proxy", "http://localhost:8080", 13)
	expect("front/package.json", "engines.node", ">=18", 11)
	expect("front/package.json", "bin", "cli.js", 12)
	expect("front/package.json", "dependency.axios", "1.6.2", 4)
	if _, found := rows["front/package.json#dependency.react"]; found {
		t.Fatal("range dependency was reported as pinned")
	}
	expect("backend/Pipfile", "packages.fastapi", "*", 8)
	expect("backend/Pipfile", "packages.pydantic", "2.1", 9)
	expect("backend/Pipfile", "dev-packages.pytest", "*", 5)
	expect("backend/Pipfile", "requires.python_version", "3.12", 12)
	expect("pyproject.toml", "project.name", "tool", 2)
	expect("pyproject.toml", "project.requires-python", ">=3.11", 3)
	expect("pyproject.toml", "project.dependencies.httpx", "httpx>=0.27", 5)
	expect("pyproject.toml", "project.dependencies.click", "click", 6)
	expect("requirements.txt", "requirements.flask", "3.0.0", 1)
	expect("go.mod", "module", "example.com/app", 1)
	expect("go.mod", "go", "1.22", 3)
	expect("backend/.env", "env_file", "backend/.env", 1)
	if _, found := rows["README.md#env_file"]; found {
		t.Fatal("non-env tracked path reported as env file")
	}
}

func TestBuildDependencies(t *testing.T) {
	importer, err := dependencies.SealImporter(dependencies.Importer{Language: "typescript", Name: "front", ModulePath: "front", PackagePath: "front", RepositoryPath: "front"})
	if err != nil {
		t.Fatalf("importer: %v", err)
	}
	catalog, err := dependencies.BuildWithOmissions(
		[]dependencies.Importer{importer},
		[]dependencies.Dependency{
			{Language: "typescript", Kind: dependencies.KindExternal, Name: "axios", ModulePath: "axios", ModuleVersion: "1.6.2", PackagePath: "axios", ImporterRefs: []string{importer.Ref}},
			{Language: "typescript", Kind: dependencies.KindWorkspace, Name: "shared", ModulePath: "shared", PackagePath: "shared", RepositoryPath: "shared", ImporterRefs: []string{importer.Ref}},
		}, nil)
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	s := newSynthetic(t, "typescript", "front", "front/src/index.tsx")
	s.object("index", programindex.ObjectModule, "src/index", "front/src/index.tsx", 1, "")
	s.external("axios", "axios", "default", programindex.ExternalAuthorityPackage)
	s.relate("imp", programindex.RelationImports, "index", []string{"axios"}, loc("front/src/index.tsx", 3))
	result := mustBuild(t, Input{Targets: []TargetInput{{Index: s.index(), Root: "front", Dependencies: &catalog}}})
	rows := result.OfKind(KindDependency)
	if len(rows) != 1 || rows[0].Key != "axios" || rows[0].Value != "1.6.2" || rows[0].Anchor.String() != "front/src/index.tsx:3" {
		t.Fatalf("dependencies = %+v", rows)
	}
}

func TestBuildIsDeterministicAndRoundTrips(t *testing.T) {
	repository := newCorpus(t, map[string]string{"README.md": "# x", "svc/app.py": "# TODO one\n# TODO one\n"})
	build := func() Result {
		return mustBuild(t, Input{Revision: "abc1234", Repository: repository, Targets: []TargetInput{{Index: backendIndex(t), Root: "backend"}}})
	}
	first, second := build(), build()
	if first.SHA256 != second.SHA256 {
		t.Fatalf("two builds differ: %s vs %s", first.SHA256, second.SHA256)
	}
	encoded, err := Encode(first)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	decoded, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if decoded.SHA256 != first.SHA256 {
		t.Fatal("round trip changed the digest")
	}
	todos := first.OfKind(KindTODO)
	if len(todos) != 2 || todos[0].ID == todos[1].ID || !strings.HasPrefix(todos[1].ID, todos[0].ID) {
		t.Fatalf("identical lines must get ordinal ids: %+v", todos)
	}
}

func TestBuildRejectsDuplicateTargets(t *testing.T) {
	index := backendIndex(t)
	if _, err := Build(Input{Targets: []TargetInput{{Index: index, Root: "backend"}, {Index: index, Root: "backend"}}}); err == nil {
		t.Fatal("duplicate targets were accepted")
	}
}
