package jstsproject

import (
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/programindex"
)

// An entry module's re-exports are followed: `export * from "./b"` gives b's
// exports, `export { c } from "./c"` gives c alone, and an exported class
// gives its methods but a #private one. What the entry module does not
// re-export, or a test file exports, is no part of the package's API.
func TestLibraryExportsFollowTheEntryModulesReExports(t *testing.T) {
	at := func(file string, line int) Location { return Location{FileRef: file, Line: line, Column: 1} }
	result := Result{
		Project: Project{PackageEntryFileRefs: []string{"index"}},
		Files:   []File{{FileRef: "index"}, {FileRef: "b"}, {FileRef: "c"}, {FileRef: "spec", Test: true}},
		Declarations: []Declaration{
			{Ref: "b.run", Kind: "function", Name: "run", Exported: true, Location: at("b", 1)},
			{Ref: "b.Box", Kind: "type", Name: "Box", Exported: true, Location: at("b", 2)},
			{Ref: "b.Box.open", Kind: "method", Name: "open", OwnerRef: "b.Box", Location: at("b", 3)},
			{Ref: "b.Box.#seal", Kind: "method", Name: "#seal", OwnerRef: "b.Box", Location: at("b", 4)},
			{Ref: "c.c", Kind: "function", Name: "c", Exported: true, Location: at("c", 1)},
			{Ref: "c.d", Kind: "function", Name: "d", Exported: true, Location: at("c", 2)},
			{Ref: "spec.helper", Kind: "function", Name: "helper", Exported: true, Location: at("spec", 1)},
		},
		Exports: []Export{
			{Ref: "e1", Name: "*", ResolvedFileRef: "b", Resolution: "exact", Location: at("index", 1)},
			{Ref: "e2", Name: "c", ResolvedFileRef: "c", Resolution: "unresolved", Location: at("index", 2)},
		},
	}
	declarations := map[string]Declaration{}
	for _, declaration := range result.Declarations {
		declarations[declaration.Ref] = declaration
	}
	exports, basis := libraryExports(result, declarations)
	var got []string
	for _, export := range exports {
		got = append(got, export.ObjectRef)
	}
	slices.Sort(got)
	if basis != programindex.ExportsEntryModules || !slices.Equal(got, []string{"b.Box.open", "b.run", "c.c"}) {
		t.Fatalf("exports %v by %q", got, basis)
	}
	// Without entry modules, every module's exports count, a test's none.
	result.Project.PackageEntryFileRefs = nil
	exports, basis = libraryExports(result, declarations)
	got = got[:0]
	for _, export := range exports {
		got = append(got, export.ObjectRef)
	}
	slices.Sort(got)
	if basis != programindex.ExportsVisibility || !slices.Equal(got, []string{"b.Box.open", "b.run", "c.c", "c.d"}) {
		t.Fatalf("exports without entries %v by %q", got, basis)
	}
}
