// Package sourcecatalog defines a run-scoped catalog of source files that an
// already validated local analysis authorized. A Catalog is a published scope,
// not an inventory of every file in the repository.
package sourcecatalog

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/freshness"
)

// Input is the minimal source scope needed to construct a Catalog. AllowedPaths
// are relative to AnalysisRoot; CapturedInputs use RepositoryRoot-relative keys.
type Input struct {
	RepositoryRoot string
	AnalysisRoot   string
	AllowedPaths   []string
	CapturedInputs []freshness.CapturedInput
}

// Source is one authorized regular file in a run-scoped Catalog.
type Source struct {
	Path           string // Canonical AnalysisRoot-relative path.
	RepositoryPath string // Canonical RepositoryRoot-relative captured key.
	Kind           freshness.FileKind
	ContentSHA256  string
}

// Catalog is an immutable, run-scoped set of authorized sources.
type Catalog struct {
	analysisRoot string
	paths        []string
	sources      map[string]Source
}

// New validates and indexes one source scope without reading the filesystem.
func New(input Input) (Catalog, error) {
	repositoryRoot, err := validateAbsoluteRoot("repository root", input.RepositoryRoot)
	if err != nil {
		return Catalog{}, err
	}
	analysisRoot, err := validateAbsoluteRoot("analysis root", input.AnalysisRoot)
	if err != nil {
		return Catalog{}, err
	}
	analysisRelative, err := filepath.Rel(repositoryRoot, analysisRoot)
	if err != nil || filepath.IsAbs(analysisRelative) || analysisRelative == ".." ||
		strings.HasPrefix(analysisRelative, ".."+string(filepath.Separator)) {
		return Catalog{}, fmt.Errorf("source catalog: analysis root must be inside repository root")
	}
	analysisPrefix := ""
	if analysisRelative != "." {
		analysisPrefix = filepath.ToSlash(analysisRelative)
		if err := validateRelativePath(analysisPrefix); err != nil {
			return Catalog{}, fmt.Errorf("source catalog: analysis root relative path: %w", err)
		}
	}

	capturedByPath := make(map[string]freshness.CapturedInput, len(input.CapturedInputs))
	for index, captured := range input.CapturedInputs {
		if err := captured.Validate(); err != nil {
			return Catalog{}, fmt.Errorf("source catalog: captured input %d: %w", index, err)
		}
		if _, duplicate := capturedByPath[captured.Path]; duplicate {
			return Catalog{}, fmt.Errorf("source catalog: duplicate captured input path %q", captured.Path)
		}
		capturedByPath[captured.Path] = captured
	}

	paths := make([]string, 0, len(input.AllowedPaths))
	sources := make(map[string]Source, len(input.AllowedPaths))
	repositoryOwners := make(map[string]string, len(input.AllowedPaths))
	for index, allowedPath := range input.AllowedPaths {
		if err := validateRelativePath(allowedPath); err != nil {
			return Catalog{}, fmt.Errorf("source catalog: allowed path %d: %w", index, err)
		}
		if _, duplicate := sources[allowedPath]; duplicate {
			return Catalog{}, fmt.Errorf("source catalog: duplicate allowed path %q", allowedPath)
		}

		repositoryPath := allowedPath
		if analysisPrefix != "" {
			repositoryPath = path.Join(analysisPrefix, allowedPath)
		}
		if owner, duplicate := repositoryOwners[repositoryPath]; duplicate {
			return Catalog{}, fmt.Errorf(
				"source catalog: allowed paths %q and %q map to the same captured input",
				owner, allowedPath,
			)
		}
		repositoryOwners[repositoryPath] = allowedPath

		captured, ok := capturedByPath[repositoryPath]
		if !ok {
			if analysisPrefix != "" {
				if _, alias := capturedByPath[allowedPath]; alias {
					return Catalog{}, fmt.Errorf(
						"source catalog: captured input %q is an analysis-relative alias; want %q",
						allowedPath, repositoryPath,
					)
				}
			}
			return Catalog{}, fmt.Errorf(
				"source catalog: allowed path %q has no captured input %q",
				allowedPath, repositoryPath,
			)
		}
		if captured.Kind != freshness.FileRegular {
			return Catalog{}, fmt.Errorf(
				"source catalog: allowed path %q captured input kind %q is not a regular file",
				allowedPath, captured.Kind,
			)
		}
		if captured.ContentSHA256 == "" {
			return Catalog{}, fmt.Errorf(
				"source catalog: allowed path %q has no captured content SHA-256",
				allowedPath,
			)
		}

		paths = append(paths, allowedPath)
		sources[allowedPath] = Source{
			Path:           allowedPath,
			RepositoryPath: repositoryPath,
			Kind:           captured.Kind,
			ContentSHA256:  captured.ContentSHA256,
		}
	}
	sort.Strings(paths)

	return Catalog{
		analysisRoot: analysisRoot,
		paths:        paths,
		sources:      sources,
	}, nil
}

// AnalysisRoot returns the canonical absolute root against which catalog paths
// are resolved.
func (c Catalog) AnalysisRoot() string {
	return c.analysisRoot
}

// Len returns the number of authorized sources without cloning their paths.
func (c Catalog) Len() int {
	return len(c.paths)
}

// Paths returns the authorized analysis-relative paths in deterministic order.
// The returned slice may be modified by the caller.
func (c Catalog) Paths() []string {
	return append([]string(nil), c.paths...)
}

// Lookup returns one source only when path is already a canonical
// analysis-root-relative slash path in this catalog.
func (c Catalog) Lookup(path string) (Source, bool) {
	if validateRelativePath(path) != nil {
		return Source{}, false
	}
	source, ok := c.sources[path]
	return source, ok
}

func validateAbsoluteRoot(label, value string) (string, error) {
	if value == "" || !filepath.IsAbs(value) || filepath.Clean(value) != value {
		return "", fmt.Errorf("source catalog: %s must be a canonical absolute path", label)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("source catalog: %s must not contain control characters", label)
		}
	}
	return value, nil
}

func validateRelativePath(value string) error {
	if value == "" || value == "." || !fs.ValidPath(value) || path.Clean(value) != value ||
		strings.ContainsRune(value, '\\') {
		return fmt.Errorf("path must be a canonical relative slash path")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("path must not contain control characters")
		}
	}
	return nil
}
