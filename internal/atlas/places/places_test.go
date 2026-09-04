package places

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

func TestFixturePlaces(t *testing.T) {
	fixture := filepath.Join(repositoryRoot(t), "testdata", "acceptance", "python-tutorial-game")
	repository := materializeFixture(t, fixture)
	backend := decodeIndex(t, fixture, "backend-program-index.json")
	front := decodeIndex(t, fixture, "front-program-index.json")
	docLine := firstDeclarationLine(t, backend, "backend/app/robot.py")
	input := Input{
		Revision:   "abc",
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
	first, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.SHA256 != second.SHA256 {
		t.Fatalf("Build is not deterministic")
	}
	if err := atlas.Validate(atlas.Atlas{Version: atlas.Version, Targets: []atlas.Target{}, Joints: []atlas.Joint{}, Diagnostics: []atlas.Diagnostic{}}); err != nil {
		t.Fatal(err)
	}
	places := make(map[string]atlas.Place)
	for _, place := range first.Places {
		places[place.ID] = place
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
	for _, place := range first.Places {
		if place.Path == "front/src/react-app-env.d.ts" && !place.File.Generated {
			t.Fatal("a .d.ts file was not marked generated")
		}
	}
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

func TestShortSignature(t *testing.T) {
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
