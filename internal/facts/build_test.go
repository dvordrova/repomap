package facts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

func (s *synthetic) callbackKeyword(ref, from, to, relationRef, patternRef, keyword string) {
	s.callback(ref, from, to, relationRef, patternRef, 0)
	s.relations[len(s.relations)-1].SourceArgument.Keyword = keyword
}

func dynamicKeyword(name, ref string) programindex.PatternArgumentInput {
	return programindex.PatternArgumentInput{Keyword: name, Kind: programindex.PatternDynamic, ObjectRefs: []string{ref}, Resolution: programindex.ResolutionExact, ObjectsObserved: 1}
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
	s.object("payload", programindex.ObjectType, "Payload", "backend/app/settings.py", 12, "settingsmod")
	s.external("fastapi", "fastapi", "FastAPI", programindex.ExternalAuthorityPackage)
	s.external("field", "pydantic", "Field", programindex.ExternalAuthorityPackage)
	s.external("basesettings", "pydantic", "BaseSettings", programindex.ExternalAuthorityPackage)
	s.external("basemodel", "pydantic", "BaseModel", programindex.ExternalAuthorityPackage)
	for _, base := range [][2]string{{"settings", "basesettings"}, {"payload", "basemodel"}} {
		s.relations = append(s.relations, programindex.RelationInput{
			SourceRef: "base-" + base[0], Kind: programindex.RelationImplements, FromRef: base[0], ToRefs: []string{base[1]},
			Resolution: programindex.ResolutionExact, TargetsObserved: 1,
			Witnesses: []programindex.Witness{{Kind: "base_class"}}, WitnessesObserved: 1,
		})
	}
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
	// On a model that is no BaseSettings, env is metadata: nothing reads it.
	s.relate("cfg-payload", programindex.RelationInvokesExternal, "payload", []string{"field"}, loc("backend/app/settings.py", 13),
		pattern("p", programindex.PatternCall, "Field", loc("backend/app/settings.py", 13), nil, keyword("env", "NOT_A_SETTING")))
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

func bindTargetSet(t *testing.T, indexes ...programindex.Index) []programindex.Index {
	t.Helper()
	bound, err := programindex.RebindTargetSet(indexes)
	if err != nil {
		t.Fatalf("bind target set: %v", err)
	}
	return bound
}

func TestBuildTargetsAndEntrypoints(t *testing.T) {
	result := mustBuild(t, Input{Revision: "78714d34ee", Targets: []TargetInput{{Index: backendIndex(t)}}})
	if len(result.Targets) != 1 {
		t.Fatalf("targets = %+v", result.Targets)
	}
	target := result.Targets[0]
	if target.Root != "backend" || target.Language != "python" || target.Anchor.String() != "backend/main.py:14" {
		t.Fatalf("target = %+v", target)
	}
	if target.ID != "t1" {
		t.Fatalf("facts target did not retain ProgramIndex identity: %q", target.ID)
	}
	entrypoint := requireFact(t, result, KindEntrypoint, "main guard", func(fact Fact) bool { return fact.Anchor.String() == "backend/main.py:14" })
	if entrypoint.Symbol != "main" || entrypoint.Key != "main_guard" || entrypoint.TargetID != target.ID {
		t.Fatalf("entrypoint = %+v", entrypoint)
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
	for _, fact := range result.OfKind(KindConfigRead) {
		if fact.Key == "NOT_A_SETTING" {
			t.Fatalf("a BaseModel field's env metadata became a config read: %+v", fact)
		}
	}
}

// TestBuildDynamicExecution proves the stage names the places where the
// program runs code it was handed, using only what the adapter sealed: the
// callee the graph resolves the call to, never the call's word. Python's exec
// is the builtin the adapter witnessed; JavaScript's Function is the
// platform's constructor. A repository function named eval is the
// repository's own code; an eval the graph left unresolved, without the
// adapter's builtin witness, has no known owner; pattern.exec is a RegExp
// method; json.loads cannot construct objects. None of those is reported.
func TestBuildDynamicExecution(t *testing.T) {
	repository := newCorpus(t, map[string]string{
		"svc/field.py": "class Field:\n    def make_step(self):\n        exec(code, {}, {})\n        subprocess.run(cmd)\n        os.system('ls')\n        data = json.loads(raw)\n        eval(text)\n        evaluate(text)\n",
		"svc/ui.tsx":   "const m = pattern.exec(text);\nconst f = new Function('return 1');\n",
	})
	s := newSynthetic(t, "python", "svc", "svc/field.py", "svc/ui.tsx")
	s.object("mod", programindex.ObjectModule, "field", "svc/field.py", 1, "")
	s.object("type", programindex.ObjectType, "Field", "svc/field.py", 1, "mod")
	s.object("step", programindex.ObjectMethod, "make_step", "svc/field.py", 2, "type")
	s.object("evaluate", programindex.ObjectFunction, "eval", "svc/field.py", 9, "mod")
	s.object("ui", programindex.ObjectModule, "ui", "svc/ui.tsx", 1, "")
	s.external("run", "subprocess", "run", programindex.ExternalAuthorityPackage)
	s.external("system", "os", "system", programindex.ExternalAuthorityPackage)
	s.external("loads", "json", "loads", programindex.ExternalAuthorityPackage)
	s.external("regexp", "platform:javascript", "exec", programindex.ExternalAuthorityPlatform)
	s.objects[len(s.objects)-1].External.Receiver = "RegExp"
	s.external("function", "platform:javascript", "Function", programindex.ExternalAuthorityPlatform)
	s.seed("mod", programindex.SeedMainGuard, "svc/field.py", 1)
	s.relate("exec", programindex.RelationCalls, "step", nil, loc("svc/field.py", 3),
		pattern("p", programindex.PatternCall, "exec", loc("svc/field.py", 3), nil, dynamic(1), dynamic(2), dynamic(3)))
	s.relations[len(s.relations)-1].Witnesses = []programindex.Witness{{Kind: "builtin", Detail: "exec"}}
	s.relate("run", programindex.RelationInvokesExternal, "step", []string{"run"}, loc("svc/field.py", 4),
		pattern("p", programindex.PatternCall, "run", loc("svc/field.py", 4), nil, dynamic(1)))
	s.relate("system", programindex.RelationInvokesExternal, "step", []string{"system"}, loc("svc/field.py", 5),
		pattern("p", programindex.PatternCall, "system", loc("svc/field.py", 5), nil, dynamic(1)))
	s.relate("loads", programindex.RelationInvokesExternal, "step", []string{"loads"}, loc("svc/field.py", 6),
		pattern("p", programindex.PatternCall, "loads", loc("svc/field.py", 6), nil, dynamic(1)))
	s.relate("unknown", programindex.RelationCalls, "step", nil, loc("svc/field.py", 7),
		pattern("p", programindex.PatternCall, "eval", loc("svc/field.py", 7), nil, dynamic(1)))
	s.relate("local", programindex.RelationCalls, "step", []string{"evaluate"}, loc("svc/field.py", 8),
		pattern("p", programindex.PatternCall, "eval", loc("svc/field.py", 8), nil, dynamic(1)))
	s.relate("jsexec", programindex.RelationInvokesExternal, "ui", []string{"regexp"}, loc("svc/ui.tsx", 1),
		pattern("p", programindex.PatternCall, "exec", loc("svc/ui.tsx", 1), nil, dynamic(1)))
	s.relate("jsfn", programindex.RelationInvokesExternal, "ui", []string{"function"}, loc("svc/ui.tsx", 2),
		pattern("p", programindex.PatternCall, "Function", loc("svc/ui.tsx", 2), nil, dynamic(1)))
	s.relations[len(s.relations)-1].Invocation = programindex.InvocationConstruct
	result := mustBuild(t, Input{Repository: repository, Targets: []TargetInput{{Index: s.index(), Root: "svc"}}})
	byAnchor := make(map[string]Fact)
	for _, fact := range result.OfKind(KindDynamicExecution) {
		byAnchor[fact.Anchor.String()] = fact
	}
	exec := byAnchor["svc/field.py:3"]
	if exec.Key != "exec" || exec.Symbol != "make_step" || exec.Text != "exec(code, {}, {})" ||
		exec.Resolution != ResolutionExact {
		t.Fatalf("exec = %+v", exec)
	}
	if fn := byAnchor["svc/ui.tsx:2"]; fn.Key != "new Function" || fn.Resolution != ResolutionExact {
		t.Fatalf("new Function = %+v", fn)
	}
	// subprocess.run and os.system start another program, which the
	// reading asks about (runs_program); json.loads reads data; an eval of
	// unknown owner and the repository's own eval are no builtin;
	// pattern.exec matches text. None runs code in this process.
	for _, quiet := range []string{"svc/field.py:4", "svc/field.py:5", "svc/field.py:6", "svc/field.py:7", "svc/field.py:8", "svc/ui.tsx:1"} {
		if fact, found := byAnchor[quiet]; found {
			t.Fatalf("unexpected dynamic execution at %s: %+v", quiet, fact)
		}
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
	// README's "HACK around it" is prose, no comment in code.
	todos := result.OfKind(KindTODO)
	if len(todos) != 2 {
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
}

func TestGoTODOsOnlyComeFromCommentsWithPhysicalAnchors(t *testing.T) {
	source := "package sample\n" +
		"var ctx = context.TODO()\n" +
		"var text = `// TODO not a comment`\n" +
		"var escaped = \"TODO: not work\"\n" +
		"//line fake.go:900\n" +
		"func f() { context.TODO() /* TODO: retry later */ }\n" +
		"/* notes\n * FIXME: preserve this anchor\n */\n"
	repository := newCorpus(t, map[string]string{"sample.go": source})
	result := mustBuild(t, Input{Repository: repository})
	rows := result.OfKind(KindTODO)
	if len(rows) != 2 {
		t.Fatalf("calls or strings became TODOs: %+v", rows)
	}
	for _, expected := range []struct {
		line int
		text string
	}{{6, "retry later"}, {8, "preserve this anchor"}} {
		requireFact(t, result, KindTODO, expected.text, func(f Fact) bool {
			return f.Anchor.Path == "sample.go" && f.Anchor.Line == expected.line && f.Text == expected.text
		})
	}
}

func TestBuildNegatives(t *testing.T) {
	bare := newCorpus(t, map[string]string{"README.md": "# tiny", "src/main.py": "print(1)\n"})
	result := mustBuild(t, Input{Repository: bare})
	names := make(map[string]Fact)
	for _, fact := range result.OfKind(KindNegative) {
		names[fact.Key] = fact
	}
	// A negative earns its place only when it changes what the reader does.
	// A thin README does not: this page is what they would have read instead.
	for _, name := range []string{
		NegativeNoTests, NegativeNoDockerfile, NegativeNoCI,
		NegativeNoLicense, NegativeNoContributing, NegativeNoChangelog, NegativeNoLinter,
	} {
		if _, found := names[name]; !found {
			t.Fatalf("missing negative %s in %+v", name, names)
		}
	}
	if len(names) != 7 {
		t.Fatalf("negatives = %+v", names)
	}

	complete := newCorpus(t, map[string]string{
		"README.md":                strings.Repeat("documentation ", 20),
		"Dockerfile":               "FROM scratch\n",
		".github/workflows/ci.yml": "on: push\n",
		"tests/test_main.py":       "def test_x(): pass\n",
		"LICENSE":                  "MIT\n",
		"docs/CONTRIBUTING.md":     "# how\n",
		"CHANGELOG.md":             "# 1.0\n",
		".golangci.yml":            "linters: {}\n",
	})
	if negatives := mustBuild(t, Input{Repository: complete}).OfKind(KindNegative); len(negatives) != 0 {
		t.Fatalf("complete repository has negatives %+v", negatives)
	}
	missing := newCorpus(t, map[string]string{"src/x.go": "package x\n", "src/x_test.go": "package x\n"})
	withoutReadme := mustBuild(t, Input{Repository: missing})
	for _, fact := range withoutReadme.OfKind(KindNegative) {
		if strings.Contains(fact.Key, "readme") {
			t.Fatalf("a missing README became a negative again: %+v", fact)
		}
	}
	if _, found := findFact(withoutReadme, KindNegative, func(fact Fact) bool { return fact.Key == NegativeNoTests }); found {
		t.Fatalf("_test.go file did not count as a test")
	}
}

// Use the ordinary no-Git filesystem inventory: New with a manually readable
// YAML listing concealed the difference between a missing and unread file.
func TestNegativesUsePathInventoryAndNestedProjectConfiguration(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		".github/workflows/ci.yaml":  "on: push\n",
		"tools/.golangci.yaml":       "linters: {}\n",
		"CHANGELOG/CHANGELOG-3.7.md": "# Changes\n",
	} {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := corpus.Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if _, readable := repository.ID(".github/workflows/ci.yaml"); readable {
		t.Fatal("test must exercise a path outside readable source content")
	}
	for _, row := range mustBuild(t, Input{Repository: repository}).OfKind(KindNegative) {
		if row.Key == NegativeNoCI || row.Key == NegativeNoLinter || row.Key == NegativeNoChangelog {
			t.Fatalf("present project files claimed absent: %+v", row)
		}
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
	if len(todos) != 2 || todos[0].ID == todos[1].ID || todos[0].ID[0] != 'a' || todos[1].ID[0] != 'a' {
		t.Fatalf("facts must get distinct compact artifact IDs: %+v", todos)
	}
}

func TestBuildRejectsDuplicateTargets(t *testing.T) {
	index := backendIndex(t)
	if _, err := Build(Input{Targets: []TargetInput{{Index: index, Root: "backend"}, {Index: index, Root: "backend"}}}); err == nil {
		t.Fatal("duplicate targets were accepted")
	}
}

// TestPackageInitEdgesFollowPythonImport pins that importing a module inside a
// package reaches that package's __init__.py, which is what Python does and
// what python-dotenv's report got wrong.
func TestPackageInitEdgesFollowPythonImport(t *testing.T) {
	edges := make(map[string]map[string]struct{})
	addPackageInitEdges([]string{
		"src/pkg/__init__.py", "src/pkg/sub/__init__.py", "src/pkg/sub/module.py",
		"src/loose/module.py", "src/pkg/asset.txt",
	}, edges)

	if _, ok := edges["src/pkg/sub/module.py"]["src/pkg/sub/__init__.py"]; !ok {
		t.Fatal("a module does not reach its own package init")
	}
	if _, ok := edges["src/pkg/sub/module.py"]["src/pkg/__init__.py"]; !ok {
		t.Fatal("a module does not reach its parent package init")
	}
	if _, ok := edges["src/loose/module.py"]; ok {
		t.Fatal("a directory without __init__.py was treated as a package")
	}
	if _, ok := edges["src/pkg/asset.txt"]; ok {
		t.Fatal("a non-Python file was given package edges")
	}
	if _, ok := edges["src/pkg/sub/__init__.py"]["src/pkg/__init__.py"]; !ok {
		t.Fatal("a package init does not reach its parent package init")
	}
	if _, ok := edges["src/pkg/sub/__init__.py"]["src/pkg/sub/__init__.py"]; ok {
		t.Fatal("a package init reaches itself")
	}
}

// Code in a language no adapter analyses is named with its language and its
// lines, through the ordinary filesystem inventory: Redis's tests live in
// test-redis.tcl, a path the corpus lists but never reads, and "No
// recognized test files" had read as "no tests". An extensionless script
// is named by its interpreter; an analysed language, a document and a
// Python script are not listed.
func TestBuildNamesFilesInLanguagesNoAdapterAnalyses(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"test-redis.tcl":            "proc test {} {\n  puts ok\n}\nputs done",
		"utils/redis-copy.rb":       "require 'redis'\n",
		"utils/redis_init_script":   "#!/bin/sh\necho start\n",
		"utils/tool":                "#!/usr/bin/env python3\nprint(1)\n",
		"redis.c":                   "int main(void) { return 0; }\n",
		"README":                    "Redis\n",
		"design-documents/VM.notes": "notes\n",
	} {
		file := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	repository, err := corpus.Open(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if _, readable := repository.ID("test-redis.tcl"); readable {
		t.Fatal("test must exercise a path outside readable source content")
	}
	var listed []string
	for _, row := range mustBuild(t, Input{Repository: repository}).OfKind(KindUnanalysedFile) {
		listed = append(listed, fmt.Sprintf("%s %s %d", row.Anchor.Path, row.Key, row.Lines))
	}
	if want := []string{"test-redis.tcl Tcl 4", "utils/redis-copy.rb Ruby 1", "utils/redis_init_script Shell 2"}; !slices.Equal(listed, want) {
		t.Fatalf("unanalysed files = %q, want %q", listed, want)
	}
}
