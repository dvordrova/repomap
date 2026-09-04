package gofacts

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/dvordrova/repomap/internal/reporead"
)

func TestExtractPackageDeclarationsFailsPackageAtomically(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "valid.go"), []byte("package api\nfunc Exported() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := reporead.New(repo)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	declarations, warnings, _ := extractPackageDeclarations(reader, ".", goListPackage{
		ImportPath: "example.com/api", GoFiles: []string{"valid.go", "missing.go"},
	})
	if len(declarations) != 0 || len(warnings) != 1 {
		t.Fatalf("atomic result = %#v warnings=%#v", declarations, warnings)
	}
}

func TestPackageDeclarationsDistinguishCallableBodiesAndOpaqueExportedReceivers(t *testing.T) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "api.go", `package api
func Assembly()
func NewClient() {}
type hidden struct{}
func (*hidden) Exported() {}
func (*hidden) private() {}
`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	declarations, err := CanonicalPackageDeclarations(declarationsFromFile(fileSet, file, "api.go"))
	if err != nil {
		t.Fatal(err)
	}
	byLabel := make(map[string]PackageDeclaration, len(declarations))
	for _, declaration := range declarations {
		byLabel[declaration.Label()] = declaration
	}
	if byLabel["Assembly"].ExecutableBody || !byLabel["Assembly"].ExportedAPI() {
		t.Fatalf("bodyless exported func = %#v", byLabel["Assembly"])
	}
	if !byLabel["NewClient"].ExecutableBody || !byLabel["NewClient"].ExportedAPI() ||
		!byLabel["hidden.Exported"].ExecutableBody || !byLabel["hidden.Exported"].ExportedAPI() {
		t.Fatalf("callable public API = %#v", declarations)
	}
	if byLabel["hidden.private"].ExportedAPI() {
		t.Fatalf("unexported method entered public API: %#v", byLabel["hidden.private"])
	}
}

func TestCanonicalPackageDeclarationsIsPermutationStableAndStrict(t *testing.T) {
	input := []PackageDeclaration{
		{Kind: PackageDeclarationType, Name: "API", Path: "api.go", Line: 9, Column: 6},
		{Kind: PackageDeclarationFunc, Name: "Open", Path: "api.go", Line: 3, Column: 6},
		{Kind: PackageDeclarationType, Name: "API", Path: "api.go", Line: 9, Column: 6},
	}
	got, err := CanonicalPackageDeclarations(input)
	if err != nil {
		t.Fatal(err)
	}
	want := []PackageDeclaration{
		{Kind: PackageDeclarationFunc, Name: "Open", Path: "api.go", Line: 3, Column: 6},
		{Kind: PackageDeclarationType, Name: "API", Path: "api.go", Line: 9, Column: 6},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("canonical = %#v, want %#v", got, want)
	}
	if _, err := CanonicalPackageDeclarations([]PackageDeclaration{{Kind: PackageDeclarationMethod, Name: "Open"}}); err == nil {
		t.Fatal("receiver-less method was accepted")
	}
	for name, declaration := range map[string]PackageDeclaration{
		"partial path": {Kind: PackageDeclarationFunc, Name: "Open", Path: "api.go"},
		"partial line": {Kind: PackageDeclarationFunc, Name: "Open", Line: 3, Column: 6},
		"unsafe path":  {Kind: PackageDeclarationFunc, Name: "Open", Path: "../api.go", Line: 3, Column: 6},
		"non-Go path":  {Kind: PackageDeclarationFunc, Name: "Open", Path: "api.txt", Line: 3, Column: 6},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := CanonicalPackageDeclarations([]PackageDeclaration{declaration}); err == nil {
				t.Fatalf("invalid location was accepted: %#v", declaration)
			}
		})
	}
}

func main() {}
