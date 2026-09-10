package places

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestCallableBindingsKeepAnonymousHandlersAndQualifiedCalls(t *testing.T) {
	loc := &programindex.Location{Path: "app/main.go", Line: 8, Column: 2}
	index := programindex.Index{Target: programindex.Target{ID: "app", Language: "go"}, Objects: []programindex.Object{
		{ID: "factory", Name: "Install", Kind: programindex.ObjectFunction, Location: loc},
		{ID: "handler", Name: "Install$1", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: loc.Path, Line: 9, Column: 3}},
		{ID: "incidental", Name: "helper$1", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: loc.Path, Line: 20, Column: 3}},
		{ID: "timeout", Name: "WithTimeout", Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{PackagePath: "context", Name: "WithTimeout"}},
	}, Relations: []programindex.Relation{
		{FromID: "factory", ToIDs: []string{"handler"}, Kind: programindex.RelationPassesCallback, Invocation: "callable_binding:field", Resolution: programindex.ResolutionExact, Witnesses: []programindex.Witness{
			{Detail: "company.Worker.Execute", Location: loc},
			{Kind: "callable_receiver_field", Detail: "Name = \"refresh\"", Location: &programindex.Location{Path: loc.Path, Line: 7, Column: 2}},
			{Kind: "interface_field_assignment", Detail: "observed receiver assignment", Location: &programindex.Location{Path: loc.Path, Line: 6, Column: 2}},
		}},
		{FromID: "handler", ToIDs: []string{"timeout"}, Kind: programindex.RelationInvokesExternal, Patterns: []programindex.RelationPattern{{Selector: "WithTimeout", Location: loc}}},
		{FromID: "handler", ToIDs: []string{"incidental"}, Kind: programindex.RelationCalls, Resolution: programindex.ResolutionExact, Invocation: "synchronous", Witnesses: []programindex.Witness{{Detail: "app/main.go:10", Location: loc}}},
		{FromID: "handler", Kind: programindex.RelationCalls, Resolution: programindex.ResolutionUnresolved, Invocation: "declared_interface_dispatch:synchronous", Witnesses: []programindex.Witness{{Detail: "company.Store.Put func(value string) error", Location: loc}}},
		{FromID: "handler", ToIDs: []string{"factory"}, Kind: programindex.RelationCalls, Resolution: programindex.ResolutionAlternatives, Witnesses: []programindex.Witness{
			{Detail: "company.Worker.Run via field Facade.worker", Location: loc},
			{Kind: "interface_field_assignment", Detail: "observed receiver assignment for Worker.Run", Location: &programindex.Location{Path: "app/factory.go", Line: 12, Column: 4}},
		}},
	}}
	target := TargetInput{Index: index, Root: "app"}
	b := builder{input: Input{Targets: []TargetInput{target, target}}, files: map[string]*fileState{}, byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
	b.collectObjects(target)
	decls := b.files[loc.Path].decls
	if len(decls) != 2 {
		t.Fatalf("callback declaration lost or incidental closure promoted: %+v", decls)
	}
	bindings := b.symbolBindings()
	for _, id := range []string{"factory", "handler"} {
		got := bindings[b.symbolOf[id]]
		if len(got) != 1 || got[0].To != "Install$1" || got[0].Detail != "company.Worker.Execute" || got[0].Path != loc.Path || got[0].Line != 8 {
			t.Fatalf("binding lost/doubled on %s: %+v", id, got)
		}
		if len(got[0].Evidence) != 2 || got[0].Evidence[0].Label != "Name = \"refresh\"" || got[0].Evidence[0].LineNo != 7 || got[0].Evidence[1].LineNo != 6 {
			t.Fatalf("receiver metadata lost or became another binding: %+v", got)
		}
	}
	calls := b.symbolCalls()[b.symbolOf["handler"]]
	if len(calls) != 4 {
		t.Fatalf("patternless calls lost or target copies doubled: %+v", calls)
	}
	var external, unresolved, fieldAssignment bool
	for _, call := range calls {
		external = external || call.Name == "context.WithTimeout"
		unresolved = unresolved || call.Detail == "company.Store.Put func(value string) error" && call.Resolution == "unresolved" && call.Name == ""
		if call.Name == "Install" {
			fieldAssignment = call.Line == loc.Line && call.Resolution == "alternatives" && len(call.Evidence) == 1 && call.Evidence[0].Path == "app/factory.go" && call.Evidence[0].LineNo == 12
		}
	}
	if !external || !unresolved || !fieldAssignment {
		t.Fatalf("qualified external or unresolved dispatch evidence lost: %+v", calls)
	}
	callers := b.symbolCallers()[b.symbolOf["factory"]]
	if len(callers) != 1 || callers[0].ObjectID != "handler" || callers[0].PlaceID != b.symbolOf["handler"] || callers[0].Name != "Install$1" || callers[0].Path != loc.Path || callers[0].Resolution != "alternatives" {
		t.Fatalf("incoming caller not bound to native target: %+v", callers)
	}
}

func TestDocumentSectionsKeepCommandsLinksAndExactLines(t *testing.T) {
	text := "Repository guide\r\n\r\n# Development\r\nFirst paragraph.\r\n\r\nLater instructions [testing](docs/testing.md#integration).\r\n\r\n```sh\r\n# This is a shell comment, not a section\r\nmake test-unit\r\nmake test-integration\r\n```\r\n\r\n## More tests\r\n~~~sh\r\n## Another shell comment\r\nmake test-e2e\r\n~~~\r\n[testing]: docs/testing.md\r\n"
	sections := documentSections("CONTRIBUTING.md", text)
	if len(sections) != 3 || sections[1].LineNo != 3 || sections[2].LineNo != 14 {
		t.Fatalf("fenced examples split or section anchors moved: %+v", sections)
	}
	var joined strings.Builder
	for _, section := range sections {
		if section.Kind != atlas.PlaceDocument || section.File != nil || section.Parent != "" || len(section.TargetIDs) != 0 {
			t.Fatal("documentation acquired code or target ownership")
		}
		if section.Document.EndLine != section.LineNo+strings.Count(strings.TrimSuffix(section.Document.Text, "\n"), "\n") {
			t.Fatal("incorrect source extent")
		}
		joined.WriteString(section.Document.Text)
	}
	if joined.String() != text {
		t.Fatal("documentation bytes were dropped, duplicated or reformatted")
	}
	graph := atlas.Graph{Version: atlas.GraphVersion, Places: sections, Seeds: []string{}, Edges: []atlas.Edge{}}
	atlas.SortPlaces(graph.Places)
	encoded, err := atlas.EncodeGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := atlas.DecodeGraph(encoded)
	if err != nil || len(loaded.Places) != len(sections) {
		t.Fatalf("documentation did not survive graph persistence: %v", err)
	}
}

func TestTypeMembersFollowNativeOwnershipAcrossFiles(t *testing.T) {
	location := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 1}
	}
	index := programindex.Index{Target: programindex.Target{ID: "target", Language: "go"}, Objects: []programindex.Object{
		{ID: "type-a", Name: "Ticket", Kind: programindex.ObjectType, Location: location("a/type.go", 1)},
		{ID: "type-b", Name: "Ticket", Kind: programindex.ObjectType, Location: location("b/type.go", 1)},
		{ID: "renew-a", Name: "Renew", Kind: programindex.ObjectMethod, OwnerID: "type-a", Location: location("a/methods.go", 4), Signature: "func() error"},
		{ID: "renew-b", Name: "Renew", Kind: programindex.ObjectMethod, OwnerID: "type-b", Location: location("b/type.go", 9)},
		{ID: "similar-name", Name: "Ticket.Release", Kind: programindex.ObjectFunction, Location: location("a/type.go", 12)},
	}}
	target := TargetInput{Index: index, Root: "."}
	b := builder{input: Input{Targets: []TargetInput{target, target}}, files: map[string]*fileState{}, byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
	b.collectObjects(target)
	b.collectObjects(target)
	b.docs = map[string][]claims.Claim{"a/methods.go": {{Line: 3, Text: "Renew extends the ticket's validity. Pending jobs remain available."}}}
	b.files["a/methods.go"].decls[0].Doc = b.docstringFor("a/methods.go", 4, b.files["a/methods.go"].decls)
	b.collectSymbols()
	for _, place := range b.symbols {
		if place.Symbol.Decl.Kind != "type" {
			continue
		}
		members := place.Symbol.Members
		if len(members) != 1 {
			t.Fatalf("lost, duplicated or guessed ownership: %+v", place)
		}
		if place.Path == "a/type.go" && (members[0].Path != "a/methods.go" || members[0].Decl.Doc != "Renew extends the ticket's validity. Pending jobs remain available." || members[0].Decl.LineNo != 4) {
			t.Fatalf("cross-file member context lost: %+v", members)
		}
		if place.Path == "b/type.go" && members[0].Decl.ObjectID != "renew-b" {
			t.Fatal("same-named types were conflated")
		}
	}
	if b.files["a/methods.go"].decls[0].Doc != "Renew extends the ticket's validity." {
		t.Fatal("type context changed ordinary callable evidence")
	}
	// Owned fields remain with their class, including types beyond the
	// description candidate budget. Do not flood the file with loose fields.
	var objects []programindex.Object
	for i := 0; i < MaxSymbolCandidates+2; i++ {
		id := fmt.Sprintf("class-%d", i)
		objects = append(objects,
			programindex.Object{ID: id, Name: id, Kind: programindex.ObjectType, Location: location("models.py", i*3+1)},
			programindex.Object{ID: id + "-field", Name: "items", Kind: programindex.ObjectVariable, OwnerID: id, ContainerID: id, Signature: "items: list[Point]", Location: location("models.py", i*3+2)},
			programindex.Object{ID: id + "-copy", Name: "items", Kind: programindex.ObjectVariable, OwnerID: id, ContainerID: id, Signature: "items: list[Point]", Location: location("models.py", i*3+2)},
		)
	}
	python := TargetInput{Index: programindex.Index{Target: programindex.Target{ID: "python", Language: "python"}, Objects: objects}, Root: "."}
	b = builder{input: Input{Targets: []TargetInput{python}}, files: map[string]*fileState{}, byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
	b.collectObjects(python)
	duplicate := python
	duplicate.Index = python.Index.Snapshot()
	duplicate.Index.Target.ID = "other-python-target"
	for i := range duplicate.Index.Objects {
		object := &duplicate.Index.Objects[i]
		object.ID = "0-" + object.ID
		if object.OwnerID != "" {
			object.OwnerID = "0-" + object.OwnerID
		}
		if object.ContainerID != "" {
			object.ContainerID = "0-" + object.ContainerID
		}
	}
	b.input.Targets = append(b.input.Targets, duplicate)
	b.collectObjects(duplicate)
	if len(b.byID) != len(duplicate.Index.Objects) {
		t.Fatal("native object lookup retained the previous complete target")
	}
	b.collectSymbols()
	if len(b.symbols) != MaxSymbolCandidates+2 || len(b.files["models.py"].decls) != MaxSymbolCandidates+2 {
		t.Fatal("type evidence was ranked out or fields leaked into the file's declarations")
	}
	for _, symbol := range b.symbols {
		if len(symbol.Symbol.Members) != 1 || symbol.Symbol.Members[0].Decl.Signature != "items: list[Point]" {
			t.Fatalf("class field was omitted or duplicated across target copies: %+v", symbol)
		}
		if got, want := symbol.Symbol.Members[0].Decl.ObjectID, "0-"+symbol.Symbol.Decl.Name+"-copy"; got != want {
			t.Fatalf("class field representative = %s, want original native-ID ordering %s", got, want)
		}
	}
}

func TestFixturePlaces(t *testing.T) {
	fixture := filepath.Join(repositoryRoot(t), "testdata", "acceptance", "python-tutorial-game")
	repository := materializeFixture(t, fixture)
	backend := decodeIndex(t, fixture, "backend-program-index.json")
	front := decodeIndex(t, fixture, "front-program-index.json")
	docLine := firstDeclarationLine(t, backend, "backend/app/robot.py")
	input := Input{
		Revision:   strings.Repeat("a", 40),
		Repository: repository,
		Targets: []TargetInput{
			{Index: backend, Dependencies: decodeCatalog(t, fixture, "backend-dependency-catalog.json"), Root: "backend"},
			{Index: front, Dependencies: decodeCatalog(t, fixture, "front-dependency-catalog.json"), Root: "front"},
		},
		Claims: claims.Result{Claims: []claims.Claim{
			{ID: "c1", Source: claims.SourceDocstring, Path: "backend/app/robot.py", Line: docLine - 1, Text: "Levels are loaded here. More words follow."},
			{ID: "c2", Source: claims.SourceDocstring, Path: "backend/main.py", Line: 1, Text: "Backend entry module."},
		}},
	}
	factInput := facts.Input{Revision: input.Revision, Repository: repository, TrackedPaths: []string{"backend/.env"}}
	for _, target := range input.Targets {
		factInput.Targets = append(factInput.Targets, facts.TargetInput{Index: target.Index, Dependencies: target.Dependencies, Root: target.Root})
	}
	var err error
	input.Facts, err = facts.Build(factInput)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	firstEncoded, err := atlas.EncodeGraph(first)
	if err != nil {
		t.Fatal(err)
	}
	// Native HTTP facts keep their actual declarations after target release;
	// local execution remains a source call, not a runtime boundary.
	wantOwners := map[string]string{
		"bnd:backend/app/app.py:18:http_server":        "sym:backend/app/app.py:19:get_levels_info",
		"bnd:backend/app/app.py:59:http_server":        "sym:backend/app/app.py:60:get_level",
		"bnd:backend/app/app.py:74:http_server":        "sym:backend/app/app.py:75:run_level",
		"bnd:front/src/service/http.ts:12:http_client": "sym:front/src/service/http.ts:10:getLevels",
		"bnd:front/src/service/http.ts:21:http_client": "sym:front/src/service/http.ts:19:getLevel",
		"bnd:front/src/service/http.ts:34:http_client": "sym:front/src/service/http.ts:30:runLevel",
	}
	dynamicFacts := make(map[string]bool)
	for _, fact := range input.Facts.OfKind(facts.KindDynamicExecution) {
		dynamicFacts[fact.ID] = true
	}
	for _, place := range first.Places {
		if place.Boundary == nil {
			continue
		}
		if dynamicFacts[place.Boundary.FactID] {
			t.Fatalf("local code execution became an external runtime boundary: %+v", place)
		}
		if owner, expected := wantOwners[place.ID]; expected {
			if place.Boundary.SubjectID != owner {
				t.Fatalf("native boundary owner changed: %+v", place)
			}
			delete(wantOwners, place.ID)
		}
	}
	if len(wantOwners) != 0 {
		t.Fatalf("native fixture boundaries missing: %+v", wantOwners)
	}
	// Keep identical canonical bytes for eager and lazy target storage below.
	if got := fmt.Sprintf("%x", sha256.Sum256(firstEncoded)); got != "da210da7ad934396463e9c246507fc9d190d8c1997620387e930823cd95a7ed2" {
		t.Fatalf("saved mixed fixture graph changed: %s", got)
	}
	lazy := input
	lazy.Targets = append([]TargetInput(nil), input.Targets...)
	loads := make([]int, len(input.Targets))
	for i, original := range input.Targets {
		original := original
		lazy.Targets[i].Index = programindex.Index{Target: original.Index.Target}
		lazy.Targets[i].ReadIndex = func() (programindex.Index, error) {
			loads[i]++
			return original.Index, nil
		}
	}
	second, err := Build(lazy)
	if err != nil {
		t.Fatal(err)
	}
	secondEncoded, err := atlas.EncodeGraph(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstEncoded, secondEncoded) {
		t.Fatalf("Build is not deterministic")
	}
	for i, count := range loads {
		if count != 1 {
			t.Fatalf("target %d loaded %d times, want one shared facts and external-boundary pass", i, count)
		}
	}
	if err := atlas.Validate(atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}); err != nil {
		t.Fatal(err)
	}
	places := make(map[string]atlas.Place)
	for _, place := range first.Places {
		places[place.ID] = place
	}
	// The runnable seed lives below the module declaration; manifest values
	// live outside code files. Both must remain exact observations for Learn.
	var mainGuard, startScript, pythonVersion, excludedReference bool
	for _, fact := range input.Facts.Facts {
		if fact.Kind != facts.KindEntrypoint && fact.Kind != facts.KindManifest {
			continue
		}
		place, found := places["fact:"+fact.ID]
		if fact.Anchor != nil && fact.Anchor.Path == "backend/.env" {
			excludedReference = true
			if found {
				t.Fatal("excluded configuration file became provider evidence")
			}
			continue
		}
		if fact.Anchor == nil || fact.Anchor.Line < 1 {
			continue
		}
		if !found || place.SourceFact == nil || place.Path != fact.Anchor.Path || place.LineNo != fact.Anchor.Line || place.Column != fact.Anchor.Column || place.SourceFact.Key != fact.Key || place.SourceFact.Value != fact.Value || place.SourceFact.ObjectID != fact.ObjectID {
			t.Fatalf("launch/manifest observation lost its exact source: %+v => %+v", fact, place)
		}
		if fact.Key == "main_guard" && place.Path == "backend/main.py" {
			mainGuard = place.LineNo == 14 && place.SourceFact.ObjectID != "" && place.SourceFact.Language == "python" && place.SourceFact.Root == "backend"
		}
		startScript = startScript || fact.Key == "scripts.start" && place.Path == "front/package.json" && fact.Value == "react-scripts start"
		pythonVersion = pythonVersion || fact.Key == "requires.python_version" && place.Path == "backend/Pipfile" && fact.Value == "3.12"
	}
	if !mainGuard || !startScript || !pythonVersion || !excludedReference {
		t.Fatalf("missing acceptance evidence: guard=%v script=%v python=%v excluded=%v", mainGuard, startScript, pythonVersion, excludedReference)
	}
	encoded, err := atlas.EncodeGraph(first)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := atlas.DecodeGraph(encoded)
	if err != nil {
		t.Fatalf("saved graph changed launch evidence: %v", err)
	}
	reencoded, err := atlas.EncodeGraph(restored)
	if err != nil || !bytes.Equal(encoded, reencoded) {
		t.Fatalf("launch evidence changed on a saved graph round trip: %v", err)
	}
	for _, entry := range repository.Entries() {
		if filepath.Ext(entry.Path) != ".md" {
			continue
		}
		var sections []atlas.Place
		for _, place := range first.Places {
			if place.Path == entry.Path && place.Document != nil {
				sections = append(sections, place)
			}
		}
		sort.Slice(sections, func(i, j int) bool { return sections[i].LineNo < sections[j].LineNo })
		var text strings.Builder
		for _, section := range sections {
			text.WriteString(section.Document.Text)
		}
		content, err := repository.ReadFileAll(entry.ID)
		if err != nil || text.String() != string(content.Bytes) {
			t.Fatalf("Build did not retain complete Markdown %s: %v", entry.Path, err)
		}
	}
	// The adapter already indexed this exported constant. Losing it here left
	// an empty file row, causing the model to describe its whole directory.
	instructions := places[atlas.FileID("front/src/utils/instructions.ts")]
	if instructions.File == nil || len(instructions.File.Decls) != 1 {
		t.Fatalf("instructions.ts lost its module-level declaration: %+v", instructions.File)
	}
	decl := instructions.File.Decls[0]
	if decl.Name != "instructions" || decl.LineNo != 1 || decl.ObjectID == "" || !decl.Exported {
		t.Fatalf("instructions declaration lost its source binding: %+v", decl)
	}
	for _, decl := range places[atlas.FileID("front/src/service/http.ts")].File.Decls {
		if decl.Name == "getLevel.error" || decl.Name == "getLevels.response" {
			t.Fatalf("function local promoted to a file declaration: %+v", decl)
		}
	}
	root, ok := places[atlas.DirectoryID(".")]
	if !ok || root.Directory.FileCount == 0 || len(root.Directory.Dirs) != 2 {
		t.Fatalf("root: %+v", root)
	}
	if root.Directory.Readme != "python-tutorial-game" {
		t.Fatalf("root README line: %q", root.Directory.Readme)
	}
	for _, path := range []string{"backend", "front"} {
		place, ok := places[atlas.DirectoryID(path)]
		if !ok {
			t.Fatalf("directory %s is missing", path)
		}
		if place.Parent != atlas.DirectoryID(".") {
			t.Fatalf("directory %s parent %q", path, place.Parent)
		}
	}
	// backend holds main.py, so it is the top box; backend/app sits beneath it.
	if top := places[atlas.DirectoryID("backend")]; !top.Directory.TopBox {
		t.Fatalf("backend is not a top box: %+v", top.Directory)
	}
	if app := places[atlas.DirectoryID("backend/app")]; app.Directory.TopBox {
		t.Fatalf("backend/app is a top box under backend")
	}
	levels := places[atlas.FileID("backend/app/robot.py")]
	if levels.File == nil || len(levels.File.Decls) == 0 {
		t.Fatalf("robot.py has no declarations")
	}
	documented := false
	for _, decl := range levels.File.Decls {
		if decl.LineNo == docLine && decl.Doc == "Levels are loaded here." {
			documented = true
		}
		if decl.Name == "self" || strings.Contains(decl.Name, "$") || decl.Name == "call result" {
			t.Fatalf("levels.py lists %q as a declaration", decl.Name)
		}
	}
	if !documented {
		t.Fatalf("the docstring above line %d was not attached: %+v", docLine, levels.File.Decls)
	}
	if main := places[atlas.FileID("backend/main.py")]; main.File.Doc != "Backend entry module." || main.Given != "Backend entry module." {
		t.Fatalf("module docstring: doc %q given %q", main.File.Doc, main.Given)
	}
	qualified := false
	for _, place := range first.Places {
		if place.Kind != atlas.PlaceFile || !strings.HasPrefix(place.Path, "front/") {
			continue
		}
		for _, decl := range place.File.Decls {
			if decl.Kind == "method" && strings.Contains(decl.Name, ".") {
				qualified = true
			}
		}
	}
	if !qualified {
		t.Fatal("no qualified TypeScript method name survived")
	}
	pythonMethod := false
	for _, decl := range placesFile(first, "backend/app/robot.py").Decls {
		if decl.Kind == "method" && strings.Contains(decl.Name, ".") {
			pythonMethod = true
		}
	}
	if !pythonMethod {
		t.Fatal("Python methods are not shown as Type.name")
	}
	if len(first.Seeds) != 2 {
		t.Fatalf("seeds: %v", first.Seeds)
	}
	unreached := 0
	for _, place := range first.Places {
		if place.Kind == atlas.PlaceFile && place.Depth > 0 && len(place.File.Callers) == 0 {
			unreached++
		}
		if place.Given == "" {
			t.Fatalf("%s has no fallback line", place.ID)
		}
	}
	if len(first.Edges) == 0 {
		t.Fatal("no edges")
	}
	symbols := 0
	perFile := make(map[string]int)
	for _, place := range first.Places {
		if place.Kind != atlas.PlaceSymbol {
			continue
		}
		symbols++
		perFile[place.Parent]++
		wantCandidate := !places[place.Parent].File.Generated && (place.Symbol.Rank <= MaxSymbolCandidates || place.Symbol.Decl.Kind == "function" || place.Symbol.Decl.Kind == "method")
		if place.Symbol.Candidate != wantCandidate || place.Symbol.Rank < 1 || place.Given == "" {
			t.Fatalf("symbol place %s: %+v", place.ID, place.Symbol)
		}
		if _, ok := places[place.Parent]; !ok {
			t.Fatalf("symbol %s has no file place", place.ID)
		}
	}
	if symbols == 0 {
		t.Fatal("no symbol places")
	}
	for fileID, count := range perFile {
		if count > len(places[fileID].File.Decls) {
			t.Fatalf("%s has %d candidates", fileID, count)
		}
	}
	for _, place := range first.Places {
		if place.Path == "front/src/react-app-env.d.ts" && !place.File.Generated {
			t.Fatal("a .d.ts file was not marked generated")
		}
	}
}

// Per-target indexes assign different object IDs to the same source callable.
// Its observations must meet at one symbol place, including generated callees;
// an unrelated callable with the same name must remain separate.
func TestSymbolContextJoinsTargetCopiesBySourceAndKeepsGeneratedCalls(t *testing.T) {
	location := func(path string, line int) *programindex.Location {
		return &programindex.Location{Path: path, Line: line, Column: 6}
	}
	makeTarget := func(suffix string, line int) TargetInput {
		return TargetInput{Root: "app", Index: programindex.Index{
			Target: programindex.Target{ID: suffix, Language: "go"},
			Objects: []programindex.Object{
				{ID: "send-" + suffix, Name: "Send", Kind: programindex.ObjectFunction, Location: location("app/client.go", 10)},
				{ID: "generated-" + suffix, Name: "Submit", Kind: programindex.ObjectFunction, Location: location("app/generated.go", 20)},
				{ID: "unrelated-" + suffix, Name: "Submit", Kind: programindex.ObjectFunction, Location: location("other/client.go", 20)},
				{ID: "transport-" + suffix, Name: "Invoke", Kind: programindex.ObjectExternalSymbol, External: &programindex.ExternalSymbol{PackagePath: "company/transport", Name: "Invoke"}},
			},
			Relations: []programindex.Relation{
				{FromID: "send-" + suffix, ToIDs: []string{"generated-" + suffix}, Kind: programindex.RelationCalls, Invocation: "synchronous", Resolution: programindex.ResolutionExact,
					Witnesses: []programindex.Witness{{Location: location("app/client.go", line)}}},
				{FromID: "generated-" + suffix, ToIDs: []string{"transport-" + suffix}, Kind: programindex.RelationInvokesExternal,
					Patterns: []programindex.RelationPattern{{Selector: "Invoke", Location: location("app/generated.go", 21)}}},
			},
		}}
	}
	a, duplicate, extra := makeTarget("a", 11), makeTarget("b", 11), makeTarget("c", 12)
	b := builder{input: Input{Targets: []TargetInput{a, duplicate, extra}}, files: map[string]*fileState{}, byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
	for _, target := range b.input.Targets {
		b.collectObjects(target)
	}
	b.files["app/generated.go"].generated = true
	b.collectSymbols()
	byPlace := make(map[string]atlas.Place)
	for _, p := range b.symbols {
		byPlace[p.ID] = p
	}
	if len(byPlace) != 3 {
		t.Fatalf("target copies became different places: %+v", byPlace)
	}
	sender := byPlace[atlas.SymbolID("app/client.go", 10, "Send")]
	generatedID := atlas.SymbolID("app/generated.go", 20, "Submit")
	if len(sender.Symbol.Calls) != 2 {
		t.Fatalf("duplicate call witnesses or lost sibling-index call: %+v", sender.Symbol.Calls)
	}
	for _, call := range sender.Symbol.Calls {
		if len(call.CalleeIDs) != 1 || call.CalleeIDs[0] != generatedID || call.Resolution != "exact" {
			t.Fatalf("callee guessed by name or lost source identity: %+v", call)
		}
	}
	generated := byPlace[generatedID].Symbol
	if generated.Candidate || len(generated.Calls) != 1 || generated.Calls[0].Name != "transport.Invoke" || len(generated.CalledBy) != 2 {
		t.Fatalf("generated callable lost, duplicated or promoted to a description: %+v", generated)
	}
	for _, caller := range generated.CalledBy {
		if caller.PlaceID != sender.ID {
			t.Fatalf("caller target copy did not resolve to source: %+v", caller)
		}
	}
	graph := atlas.Graph{Version: atlas.GraphVersion, Revision: "fixture", Places: append([]atlas.Place(nil), b.symbols...)}
	for path, state := range b.files {
		graph.Places = append(graph.Places, atlas.Place{ID: atlas.FileID(path), Kind: atlas.PlaceFile, Path: path, File: &atlas.FileFacts{Decls: state.decls, Generated: state.generated}})
	}
	atlas.SortPlaces(graph.Places)
	if _, err := atlas.EncodeGraph(graph); err != nil {
		t.Fatalf("shared source links do not form a valid graph: %v", err)
	}
	sender.Symbol.Calls[0].CalleeIDs = []string{"sym:absent.go:1:Submit"}
	if _, err := atlas.EncodeGraph(graph); err == nil {
		t.Fatal("a dangling callee link passed graph validation")
	}
}

func TestDispatchViewsKeepPossibleReceiversWithoutInventingExtraCalls(t *testing.T) {
	type observation struct {
		resolution programindex.Resolution
		callee     string
		column     int
		detail     string
		evidence   bool
	}
	unresolved := observation{resolution: programindex.ResolutionUnresolved, column: 12, detail: "Port.Send via field Client.Port"}
	possible := unresolved
	possible.resolution, possible.callee = programindex.ResolutionAlternatives, "SendA"
	otherReceiver := possible
	otherReceiver.callee = "SendB"
	otherSite := unresolved
	otherSite.column = 30
	otherDetail := unresolved
	otherDetail.detail = "OtherPort.Send via field Client.Other"
	withEvidence := unresolved
	withEvidence.evidence = true
	exact := possible
	exact.resolution = programindex.ResolutionExact
	missingColumn, unknownColumn := possible, unresolved
	missingColumn.column, unknownColumn.column = 0, 0
	repeatedCall := possible
	repeatedCall.column = 30
	alpha, zulu := unresolved, unresolved
	alpha.detail, alpha.column = "Alpha.Send", 50
	zulu.detail, zulu.column = "Zulu.Send", 2
	for _, tc := range []struct {
		name         string
		observations []observation
		calls        int
		unresolved   int
	}{
		{"same dispatch in two views", []observation{unresolved, possible}, 1, 0},
		{"all possible receivers", []observation{unresolved, possible, otherReceiver}, 2, 0},
		{"different column", []observation{otherSite, possible}, 2, 1},
		{"different receiver detail", []observation{otherDetail, possible}, 2, 1},
		{"independent evidence", []observation{withEvidence, possible}, 2, 1},
		{"exact view cannot erase uncertainty", []observation{unresolved, exact}, 2, 1},
		{"no precise source position", []observation{unknownColumn, missingColumn}, 2, 1},
		{"same function called twice on one line", []observation{unresolved, possible, repeatedCall}, 2, 0},
		{"local column does not reorder evidence", []observation{zulu, alpha}, 2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := builder{files: map[string]*fileState{}, byID: map[string]programindex.Object{}, fileOf: map[string]string{}, targetOf: map[string]map[string]struct{}{}}
			for i, observed := range tc.observations {
				suffix := fmt.Sprint(i)
				location := &programindex.Location{Path: "app/client.go", Line: 11, Column: observed.column}
				relation := programindex.Relation{FromID: "caller" + suffix, Kind: programindex.RelationCalls, Resolution: observed.resolution, Invocation: "interface_invoke:synchronous",
					Witnesses: []programindex.Witness{{Kind: "dispatch", Detail: observed.detail, Location: location}}}
				if observed.callee != "" {
					relation.ToIDs = []string{observed.callee + suffix}
				}
				if observed.evidence {
					relation.Witnesses = append(relation.Witnesses, programindex.Witness{Kind: "interface_field_assignment", Detail: "a separate receiver observation", Location: location})
				}
				target := TargetInput{Root: "app", Index: programindex.Index{Target: programindex.Target{ID: suffix, Language: "go"}, Objects: []programindex.Object{
					{ID: "caller" + suffix, Name: "Run", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "app/client.go", Line: 10, Column: 6}},
					{ID: "SendA" + suffix, Name: "SendA", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "app/store.go", Line: 20, Column: 6}},
					{ID: "SendB" + suffix, Name: "SendB", Kind: programindex.ObjectFunction, Location: &programindex.Location{Path: "app/other.go", Line: 30, Column: 6}},
				}, Relations: []programindex.Relation{relation}}}
				b.input.Targets = append(b.input.Targets, target)
				b.collectObjects(target)
			}
			calls := b.symbolCalls()[atlas.SymbolID("app/client.go", 10, "Run")]
			unknown := 0
			for _, call := range calls {
				if call.Resolution == string(programindex.ResolutionUnresolved) {
					unknown++
				}
				if call.Name != "" && tc.name != "exact view cannot erase uncertainty" && call.Resolution != string(programindex.ResolutionAlternatives) {
					t.Fatalf("possible receiver became exact: %+v", call)
				}
			}
			if len(calls) != tc.calls || unknown != tc.unresolved {
				t.Fatalf("call-site identity or open frontier lost: %+v", calls)
			}
			if tc.name == "local column does not reorder evidence" && calls[0].Detail != "Alpha.Send" {
				t.Fatalf("local position changed provider evidence order: %+v", calls)
			}
		})
	}
}

func atlasTestIndexTarget(t *testing.T, name string) programindex.Index {
	t.Helper()
	index, err := programindex.New(programindex.Input{
		ScenarioSHA256: strings.Repeat("a", 64), SourceSHA256: strings.Repeat("b", 64),
		Target: programindex.TargetInput{
			Language: "go", Kind: "executable", Name: name, Selector: name,
			Sources: []programindex.TargetSource{{FileRef: "f1", Path: name + "/main.go"}}, AnchorFileRef: "f1",
		},
		Objects: []programindex.ObjectInput{{
			SourceRef: "o", Kind: programindex.ObjectFunction, Name: "F", Visibility: programindex.VisibilityPublic,
			Location: &programindex.Location{Path: name + "/main.go", Line: 1, Column: 1},
		}},
		Relations: []programindex.RelationInput{},
		Coverage:  programindex.CoverageInput{Measured: true, ObjectsObserved: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func placesFile(graph atlas.Graph, path string) *atlas.FileFacts {
	for _, place := range graph.Places {
		if place.ID == atlas.FileID(path) {
			return place.File
		}
	}
	return &atlas.FileFacts{}
}

func firstDeclarationLine(t *testing.T, index programindex.Index, path string) int {
	t.Helper()
	line := 0
	for _, object := range index.Objects {
		if object.Location == nil || object.Location.Path != path ||
			(object.Kind != programindex.ObjectFunction && object.Kind != programindex.ObjectMethod) ||
			strings.Contains(object.Name, "$") {
			continue
		}
		if line == 0 || object.Location.Line < line {
			line = object.Location.Line
		}
	}
	if line < 3 {
		t.Fatalf("no function in %s after line 2", path)
	}
	return line
}

func TestFilesUnderAnotherTargetRootBelongToIt(t *testing.T) {
	tool := atlasTestIndexTarget(t, "tool")
	lib := atlasTestIndexTarget(t, "lib")
	toolID, libID := tool.Target.ID, lib.Target.ID
	b := &builder{files: map[string]*fileState{
		"cmd/tool/main.go": {targets: map[string]struct{}{toolID: {}}},
		"lib/x.go":         {targets: map[string]struct{}{toolID: {}, libID: {}}},
		"shared/y.go":      {targets: map[string]struct{}{toolID: {}, libID: {}}},
	}}
	b.input.Targets = []TargetInput{{Index: tool, Root: "cmd/tool"}, {Index: lib, Root: "lib"}}
	b.claimByRoot()
	if _, still := b.files["lib/x.go"].targets[tool.Target.ID]; still {
		t.Fatal("lib/x.go is still the tool's")
	}
	if _, kept := b.files["lib/x.go"].targets[libID]; !kept {
		t.Fatal("lib/x.go lost its indexed owner")
	}
	if len(b.files["shared/y.go"].targets) != 2 {
		t.Fatal("a shared file lost a target")
	}
}

func TestSameRootTargetsKeepTheirIndexedFilesInEitherOrder(t *testing.T) {
	exe, lib, nested := atlasTestIndexTarget(t, "tool"), atlasTestIndexTarget(t, "library"), atlasTestIndexTarget(t, "nested")
	inputs := []TargetInput{{Index: exe, Root: "tools/check"}, {Index: lib, Root: "tools/check"}, {Index: nested, Root: "tools/check/child"}}
	for _, order := range [][]TargetInput{inputs, {inputs[2], inputs[1], inputs[0]}} {
		b := &builder{input: Input{Targets: order}, files: map[string]*fileState{
			"tools/check/main.go":      {targets: map[string]struct{}{exe.Target.ID: {}}},
			"tools/check/cmd/run.go":   {targets: map[string]struct{}{exe.Target.ID: {}, lib.Target.ID: {}}},
			"tools/check/child/run.go": {targets: map[string]struct{}{exe.Target.ID: {}, lib.Target.ID: {}, nested.Target.ID: {}}},
		}}
		b.claimByRoot()
		shared := b.files["tools/check/cmd/run.go"].targets
		if len(shared) != 2 {
			t.Fatalf("same-root library lost code: %v", shared)
		}
		if len(b.files["tools/check/main.go"].targets) != 1 {
			t.Fatal("a library acquired the executable's main file")
		}
		child := b.files["tools/check/child/run.go"].targets
		if _, ok := child[nested.Target.ID]; !ok || len(child) != 1 {
			t.Fatalf("nested package lost ownership: %v", child)
		}
	}
}

func TestCleanTextDropsControlCharacters(t *testing.T) {
	if got := cleanText("a\nb\tc\x00d"); got != "a b c d" {
		t.Fatalf("cleanText: %q", got)
	}
	if got := cleanText("plain text"); got != "plain text" {
		t.Fatalf("cleanText left plain text alone: %q", got)
	}
}

func TestShortSignature(t *testing.T) {
	python := "read(path: str='some/path/data.json') -> list[Point]"
	if languageSignature(python, "python") != python {
		t.Fatal("Go path shortening changed a Python declaration literal")
	}
	cases := map[string]string{
		"func(entries []github.com/dvordrova/repomap/internal/corpus.Entry) (map[string]github.com/x/y/z.T, error)": "func(entries []corpus.Entry) (map[string]z.T, error)",
		"func(a int) string":                             "func(a int) string",
		"func(*golang.org/x/tools/go/ssa.Function) bool": "func(*ssa.Function) bool",
	}
	for signature, want := range cases {
		if got := shortSignature(signature); got != want {
			t.Errorf("%q -> %q, want %q", signature, got, want)
		}
	}
}

func TestGeneratedMarkers(t *testing.T) {
	late := strings.Repeat("// license line\n", 19) + "// Code generated by protoc-gen-go. DO NOT EDIT.\npackage x\n"
	if !generatedByMarker([]byte(late)) {
		t.Fatal("a marker on line 20 was missed")
	}
	tooLate := strings.Repeat("// license line\n", 40) + "// Code generated. DO NOT EDIT.\n"
	if generatedByMarker([]byte(tooLate)) {
		t.Fatal("a marker past the header window counted")
	}
	if generatedByMarker([]byte("package x\n// this file is not generated at all\n")) {
		t.Fatal("prose mentioning generation counted")
	}
	for _, name := range []string{"pkg/zz_generated.deepcopy.go", "api/v1/types.pb.go", "web/dist/app.min.js"} {
		if !generatedByName(name) {
			t.Errorf("%s not generated by name", name)
		}
	}
}

func TestReadableLineAndFirstSentence(t *testing.T) {
	if got := readableLine("# repomap"); got != "repomap" {
		t.Errorf("heading: %q", got)
	}
	if got := readableLine("[![build](https://x/badge.svg)](https://x)"); got != "" {
		t.Errorf("badge: %q", got)
	}
	if got := readableLine("See the [guide](docs/guide.md) for **details**."); got != "See the guide for details." {
		t.Errorf("link: %q", got)
	}
	if got := firstSentence("Package x does things. It also does more."); got != "Package x does things." {
		t.Errorf("first sentence: %q", got)
	}
	if got := firstSentence("Supports v1.2 and later. Also v2."); got != "Supports v1.2 and later." {
		t.Errorf("version: %q", got)
	}
	if got := goPackageComment("// Package facts owns the fact layer.\n// More.\npackage facts\n"); got != "Package facts owns the fact layer." {
		t.Errorf("package comment: %q", got)
	}
}

func decodeIndex(t *testing.T, fixture, name string) programindex.Index {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixture, "artifacts", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	index, err := programindex.Decode(raw)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return index
}

func decodeCatalog(t *testing.T, fixture, name string) *dependencies.Catalog {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(fixture, "artifacts", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	catalog, err := dependencies.Decode(raw)
	if err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return &catalog
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
}

func materializeFixture(t *testing.T, fixture string) *corpus.Corpus {
	t.Helper()
	for _, name := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR"} {
		t.Setenv(name, "")
		os.Unsetenv(name)
	}
	destination := filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	skip := map[string]struct{}{"expected.json": {}, "artifacts": {}, "REVISION": {}}
	err := filepath.WalkDir(fixture, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(fixture, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		// node_modules is tracked for the TypeScript compiler; places never
		// reads it, and copying it is most of the test's time.
		if entry.IsDir() && entry.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if _, skipped := skip[strings.SplitN(filepath.ToSlash(relative), "/", 2)[0]]; skipped {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	git := func(args ...string) {
		command := exec.Command("git", args...)
		command.Dir = destination
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "--quiet")
	git("add", "--all", "--")
	git("-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "--message", "fixture")
	repository, err := corpus.Open(t.Context(), destination)
	if err != nil {
		t.Fatalf("open fixture corpus: %v", err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return repository
}
