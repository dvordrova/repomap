package gofacts

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/reporead"
)

const maxPackageDeclarationFileBytes = 16 * 1024 * 1024

type PackageDeclarationKind string

const (
	PackageDeclarationFunc   PackageDeclarationKind = "func"
	PackageDeclarationMethod PackageDeclarationKind = "method"
	PackageDeclarationType   PackageDeclarationKind = "type"
	PackageDeclarationVar    PackageDeclarationKind = "var"
	PackageDeclarationConst  PackageDeclarationKind = "const"
)

// PackageDeclaration is one exact package-owned top-level declaration label
// and, when retained in producer facts, the repository-relative identifier
// position selected by the active Go build. Receiver is the declared base type
// for methods; source, comments, signatures and bodies are deliberately absent.
//
// Path, Line and Column are optional because the target-portfolio catalog
// deliberately strips them before sealing its names-only provider authority.
// Producer Go facts always populate the complete location tuple.
type PackageDeclaration struct {
	Kind     PackageDeclarationKind `json:"kind"`
	Name     string                 `json:"name"`
	Receiver string                 `json:"receiver,omitempty"`
	Path     string                 `json:"path,omitempty"`
	Line     int                    `json:"line,omitempty"`
	Column   int                    `json:"column,omitempty"`
	// ExecutableBody is exact AST authority for a Go func/method body. It is
	// false for assembly/external declarations and every non-callable kind.
	ExecutableBody bool `json:"executable_body,omitempty"`
}

func (declaration PackageDeclaration) Label() string {
	if declaration.Kind == PackageDeclarationMethod {
		return declaration.Receiver + "." + declaration.Name
	}
	return declaration.Name
}

func (declaration PackageDeclaration) ExportedAPI() bool {
	// A method with an exported name remains part of the callable package API
	// even when its receiver type is intentionally opaque: callers can obtain
	// such a value from the package and invoke the exported method on it.
	return ast.IsExported(declaration.Name)
}

func ValidatePackageDeclarations(values []PackageDeclaration) error {
	for index, value := range values {
		if !validDeclarationKind(value.Kind) || !validGoIdentifier(value.Name) || value.Name == "_" {
			return fmt.Errorf("package declaration %d is invalid", index)
		}
		if value.Kind == PackageDeclarationMethod {
			if !validGoIdentifier(value.Receiver) || value.Receiver == "_" {
				return fmt.Errorf("package declaration %d has invalid receiver", index)
			}
		} else if value.Receiver != "" {
			return fmt.Errorf("package declaration %d has an unexpected receiver", index)
		}
		if value.ExecutableBody && value.Kind != PackageDeclarationFunc && value.Kind != PackageDeclarationMethod {
			return fmt.Errorf("package declaration %d has an unexpected executable body", index)
		}
		if !validPackageDeclarationLocation(value) {
			return fmt.Errorf("package declaration %d has an invalid location", index)
		}
		if index > 0 && !packageDeclarationLess(values[index-1], value) {
			return fmt.Errorf("package declarations are not canonical")
		}
	}
	return nil
}

// CanonicalPackageDeclarations returns an independently owned, sorted and
// de-duplicated declaration set.
func CanonicalPackageDeclarations(values []PackageDeclaration) ([]PackageDeclaration, error) {
	result := append([]PackageDeclaration(nil), values...)
	sort.Slice(result, func(i, j int) bool { return packageDeclarationLess(result[i], result[j]) })
	result = compactPackageDeclarations(result)
	if err := ValidatePackageDeclarations(result); err != nil {
		return nil, err
	}
	return result, nil
}

func extractPackageDeclarations(
	reader *reporead.Reader,
	packageDir string,
	pkg goListPackage,
) ([]PackageDeclaration, []string) {
	if reader == nil {
		return nil, []string{fmt.Sprintf("package %s: declaration source reader is unavailable", pkg.ImportPath)}
	}
	goFiles := append([]string(nil), pkg.GoFiles...)
	goFiles = append(goFiles, pkg.CgoFiles...)
	sort.Strings(goFiles)
	goFiles = compactStrings(goFiles)

	declarations := make([]PackageDeclaration, 0)
	for _, goFile := range goFiles {
		if strings.HasSuffix(goFile, "_test.go") {
			continue
		}
		repoPath, err := entrypointSourcePath(packageDir, goFile)
		if err != nil {
			return nil, []string{fmt.Sprintf("package %s: cannot locate declaration source %q: %v", pkg.ImportPath, goFile, err)}
		}
		content, err := reader.ReadFile(repoPath, maxPackageDeclarationFileBytes)
		if err != nil {
			return nil, []string{fmt.Sprintf("package %s: cannot inspect declarations in %s: %v", pkg.ImportPath, repoPath, err)}
		}
		if content.Truncated {
			return nil, []string{fmt.Sprintf("package %s: declaration source %s exceeds %d bytes", pkg.ImportPath, repoPath, maxPackageDeclarationFileBytes)}
		}

		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, repoPath, content.Bytes, parser.SkipObjectResolution)
		if err != nil {
			return nil, []string{fmt.Sprintf("package %s: cannot parse declarations in %s: %v", pkg.ImportPath, repoPath, err)}
		}
		declarations = append(declarations, declarationsFromFile(fileSet, file, repoPath)...)
	}

	declarations, err := CanonicalPackageDeclarations(declarations)
	if err != nil {
		return nil, []string{fmt.Sprintf("package %s: %v", pkg.ImportPath, err)}
	}
	return declarations, nil
}

func declarationsFromFile(fileSet *token.FileSet, file *ast.File, repoPath string) []PackageDeclaration {
	if fileSet == nil || file == nil || !fs.ValidPath(repoPath) {
		return nil
	}
	var result []PackageDeclaration
	for _, declaration := range file.Decls {
		switch value := declaration.(type) {
		case *ast.FuncDecl:
			if value.Name == nil || value.Name.Name == "_" {
				continue
			}
			location := packageDeclarationLocation(fileSet, repoPath, value.Name)
			if value.Recv == nil {
				result = append(result, PackageDeclaration{
					Kind: PackageDeclarationFunc, Name: value.Name.Name,
					Path: location.Path, Line: location.Line, Column: location.Column,
					ExecutableBody: value.Body != nil,
				})
				continue
			}
			receiver := receiverBaseName(value.Recv)
			if receiver != "" {
				result = append(result, PackageDeclaration{
					Kind: PackageDeclarationMethod, Name: value.Name.Name, Receiver: receiver,
					Path: location.Path, Line: location.Line, Column: location.Column,
					ExecutableBody: value.Body != nil,
				})
			}
		case *ast.GenDecl:
			kind, ok := declarationKindForToken(value.Tok)
			if !ok {
				continue
			}
			for _, spec := range value.Specs {
				switch item := spec.(type) {
				case *ast.TypeSpec:
					if item.Name != nil && item.Name.Name != "_" {
						location := packageDeclarationLocation(fileSet, repoPath, item.Name)
						result = append(result, PackageDeclaration{
							Kind: kind, Name: item.Name.Name,
							Path: location.Path, Line: location.Line, Column: location.Column,
						})
					}
				case *ast.ValueSpec:
					for _, name := range item.Names {
						if name != nil && name.Name != "_" {
							location := packageDeclarationLocation(fileSet, repoPath, name)
							result = append(result, PackageDeclaration{
								Kind: kind, Name: name.Name,
								Path: location.Path, Line: location.Line, Column: location.Column,
							})
						}
					}
				}
			}
		}
	}
	return result
}

type declarationLocation struct {
	Path   string
	Line   int
	Column int
}

func packageDeclarationLocation(
	fileSet *token.FileSet,
	repoPath string,
	identifier *ast.Ident,
) declarationLocation {
	if fileSet == nil || identifier == nil || !identifier.Pos().IsValid() {
		return declarationLocation{}
	}
	position := fileSet.PositionFor(identifier.Pos(), false)
	if position.Line <= 0 || position.Column <= 0 {
		return declarationLocation{}
	}
	return declarationLocation{Path: repoPath, Line: position.Line, Column: position.Column}
}

func validPackageDeclarationLocation(value PackageDeclaration) bool {
	if value.Path == "" && value.Line == 0 && value.Column == 0 {
		return true
	}
	return fs.ValidPath(value.Path) && strings.HasSuffix(value.Path, ".go") &&
		value.Line > 0 && value.Column > 0
}

func declarationKindForToken(value token.Token) (PackageDeclarationKind, bool) {
	switch value {
	case token.TYPE:
		return PackageDeclarationType, true
	case token.VAR:
		return PackageDeclarationVar, true
	case token.CONST:
		return PackageDeclarationConst, true
	default:
		return "", false
	}
}

func receiverBaseName(fields *ast.FieldList) string {
	if fields == nil || len(fields.List) != 1 {
		return ""
	}
	return receiverTypeName(fields.List[0].Type)
}

func receiverTypeName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.StarExpr:
		return receiverTypeName(value.X)
	case *ast.ParenExpr:
		return receiverTypeName(value.X)
	case *ast.IndexExpr:
		return receiverTypeName(value.X)
	case *ast.IndexListExpr:
		return receiverTypeName(value.X)
	default:
		return ""
	}
}

func packageDeclarationLess(left, right PackageDeclaration) bool {
	if packageDeclarationKindOrder(left.Kind) != packageDeclarationKindOrder(right.Kind) {
		return packageDeclarationKindOrder(left.Kind) < packageDeclarationKindOrder(right.Kind)
	}
	if left.Receiver != right.Receiver {
		return left.Receiver < right.Receiver
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	return left.Column < right.Column
}

func packageDeclarationKindOrder(kind PackageDeclarationKind) int {
	switch kind {
	case PackageDeclarationFunc:
		return 0
	case PackageDeclarationMethod:
		return 1
	case PackageDeclarationType:
		return 2
	case PackageDeclarationVar:
		return 3
	case PackageDeclarationConst:
		return 4
	default:
		return 5
	}
}

func compactPackageDeclarations(values []PackageDeclaration) []PackageDeclaration {
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func validDeclarationKind(kind PackageDeclarationKind) bool {
	return packageDeclarationKindOrder(kind) < 5
}

func validGoIdentifier(value string) bool {
	return token.IsIdentifier(value)
}
