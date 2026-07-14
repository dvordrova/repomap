package gofacts

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/reporead"
)

const (
	maxEntrypointFilesPerPackage = 256
	maxEntrypointFileBytes       = 2 * 1024 * 1024
	maxEntrypointPackageBytes    = 16 * 1024 * 1024
)

// resolveMainEntrypoints turns package-main candidates from go list into
// actual executable entrypoints. Only build-selected GoFiles are inspected.
func resolveMainEntrypoints(
	reader *reporead.Reader,
	candidates []Entrypoint,
) ([]Entrypoint, []string) {
	if reader == nil {
		return nil, []string{"entrypoint source reader is unavailable"}
	}

	entrypoints := make([]Entrypoint, 0, len(candidates))
	var warnings []string
	for _, candidate := range candidates {
		anchors, candidateWarnings := findMainFunctionAnchors(reader, candidate)
		warnings = append(warnings, candidateWarnings...)
		if len(anchors) == 0 {
			continue
		}

		candidate.Anchors = anchors
		entrypoints = append(entrypoints, candidate)
	}

	return entrypoints, warnings
}

func findMainFunctionAnchors(
	reader *reporead.Reader,
	entrypoint Entrypoint,
) ([]EntrypointAnchor, []string) {
	goFiles := append([]string(nil), entrypoint.GoFiles...)
	sort.Strings(goFiles)
	if len(goFiles) > maxEntrypointFilesPerPackage {
		goFiles = goFiles[:maxEntrypointFilesPerPackage]
	}

	var (
		anchors   []EntrypointAnchor
		warnings  []string
		readBytes int
	)
	for _, goFile := range goFiles {
		if readBytes >= maxEntrypointPackageBytes {
			warnings = append(warnings, fmt.Sprintf(
				"entrypoint package %s: source scan reached %d-byte package limit",
				entrypoint.ImportPath,
				maxEntrypointPackageBytes,
			))
			break
		}

		repoPath, err := entrypointSourcePath(entrypoint.PackageDir, goFile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"entrypoint package %s: skip build-selected source %q: %v",
				entrypoint.ImportPath,
				goFile,
				err,
			))
			continue
		}

		content, err := reader.ReadFile(repoPath, maxEntrypointFileBytes)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"entrypoint package %s: cannot inspect %s: %v",
				entrypoint.ImportPath,
				repoPath,
				err,
			))
			continue
		}
		readBytes += len(content.Bytes)
		if content.Truncated {
			warnings = append(warnings, fmt.Sprintf(
				"entrypoint package %s: skip %s because it exceeds %d bytes",
				entrypoint.ImportPath,
				repoPath,
				maxEntrypointFileBytes,
			))
			continue
		}

		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, repoPath, content.Bytes, parser.SkipObjectResolution)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"entrypoint package %s: cannot parse %s: %v",
				entrypoint.ImportPath,
				repoPath,
				err,
			))
			continue
		}
		if file.Name == nil || file.Name.Name != "main" {
			continue
		}

		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || !isMainFunction(function) {
				continue
			}

			position := fileSet.PositionFor(function.Name.Pos(), false)
			if position.Line <= 0 {
				continue
			}
			anchors = append(anchors, EntrypointAnchor{
				Version: EntrypointAnchorVersion,
				Kind:    EntrypointAnchorGoMain,
				Path:    repoPath,
				Line:    position.Line,
			})
		}
	}

	if len(entrypoint.GoFiles) > maxEntrypointFilesPerPackage {
		warnings = append(warnings, fmt.Sprintf(
			"entrypoint package %s: source scan kept %d of %d build-selected files",
			entrypoint.ImportPath,
			maxEntrypointFilesPerPackage,
			len(entrypoint.GoFiles),
		))
	}

	sort.Slice(anchors, func(i, j int) bool {
		if anchors[i].Path == anchors[j].Path {
			return anchors[i].Line < anchors[j].Line
		}
		return anchors[i].Path < anchors[j].Path
	})
	return anchors, warnings
}

func isMainFunction(function *ast.FuncDecl) bool {
	if function == nil || function.Name == nil || function.Name.Name != "main" || function.Recv != nil {
		return false
	}
	if function.Type == nil || fieldCount(function.Type.Params) != 0 || fieldCount(function.Type.Results) != 0 {
		return false
	}
	return function.Type.TypeParams == nil || fieldCount(function.Type.TypeParams) == 0
}

func fieldCount(fields *ast.FieldList) int {
	if fields == nil {
		return 0
	}
	return len(fields.List)
}

func entrypointSourcePath(packageDir, goFile string) (string, error) {
	goFile = filepath.ToSlash(strings.TrimSpace(goFile))
	if goFile == "" || path.IsAbs(goFile) || path.Clean(goFile) != goFile || path.Base(goFile) != goFile || path.Ext(goFile) != ".go" {
		return "", fmt.Errorf("expected a package-local .go filename")
	}

	packageDir = filepath.ToSlash(strings.TrimSpace(packageDir))
	if packageDir == "" || packageDir == "." {
		return goFile, nil
	}
	if path.IsAbs(packageDir) || path.Clean(packageDir) != packageDir || packageDir == ".." || strings.HasPrefix(packageDir, "../") {
		return "", fmt.Errorf("package directory is not repository-relative")
	}
	return path.Join(packageDir, goFile), nil
}
