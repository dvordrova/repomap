// Package workspaceentrypoint defines a bounded immutable index of exact Go
// entrypoint declarations over one authorized workspace package graph.
package workspaceentrypoint

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspacegraph"
)

const (
	// MaxRawRows bounds both the outer entrypoint collection and the aggregate
	// number of exact anchors consumed from it.
	MaxRawRows = 1000
	// MaxScalarBytes is the encoded-byte limit for every consumed scalar.
	MaxScalarBytes = 4096
	// MaxAggregateScalarBytes bounds all consumed scalars in one construction.
	MaxAggregateScalarBytes = 4 * 1024 * 1024
)

var (
	errRawBounds       = errors.New("workspace entrypoint index: raw facts exceed bounds")
	errScalarBounds    = errors.New("workspace entrypoint index: scalar fact exceeds bounds")
	errAggregateBounds = errors.New("workspace entrypoint index: aggregate scalar facts exceed bounds")
)

// Input is the complete already-collected authority used to construct an
// Index. New performs no source reads, discovery, command execution, or
// provider calls.
type Input struct {
	GoFacts gofacts.Facts
	Graph   workspacegraph.Graph
}

// Entry is one exact build-selected Go main declaration. Package, Path, and
// Line form its identity. Openable is true only when the graph's exact package
// file is also a member of the authorized source catalog.
type Entry struct {
	Kind     string
	Package  string
	Path     string
	Symbol   string
	Line     int
	Openable bool
}

type entryKey struct {
	pkg  string
	path string
	line int
}

type rawEntry struct {
	modulePath        string
	importPath        string
	packageDir        string
	moduleRelativeDir string
	moduleDir         string
	anchorVersion     int
	anchorKind        string
	anchorPath        string
	anchorLine        int
}

// Index is an immutable deterministic projection. Its slices and lookup map
// are private; Entries returns a defensive copy.
type Index struct {
	entries     []Entry
	entryLookup map[entryKey]int
	initialized bool
}

// New constructs an Index after completely preflighting raw collection and
// scalar budgets. Malformed, outside, unknown-package, and unsupported rows
// are omitted. Exact duplicates collapse; conflicting rows with the same
// package/path/line identity reject the complete index.
func New(input Input) (Index, error) {
	rawCount, err := preflight(input.GoFacts.EntrypointPackages)
	if err != nil {
		return Index{}, err
	}

	packages := input.Graph.Packages()
	if packages == nil {
		return Index{}, fmt.Errorf("workspace entrypoint index: package graph is unavailable")
	}
	packageLookup := make(map[string]workspacegraph.Package, min(len(packages), MaxRawRows))
	for _, pkg := range packages {
		packageLookup[pkg.CanonicalPath] = pkg
	}

	rawByIdentity := make(map[entryKey]rawEntry, min(rawCount, MaxRawRows))
	accepted := make([]Entry, 0, min(rawCount, MaxRawRows))
	for _, entrypoint := range input.GoFacts.EntrypointPackages {
		for _, anchor := range entrypoint.Anchors {
			raw := rawEntry{
				modulePath:        entrypoint.ModulePath,
				importPath:        entrypoint.ImportPath,
				packageDir:        entrypoint.PackageDir,
				moduleRelativeDir: entrypoint.ModuleRelativeDir,
				moduleDir:         entrypoint.ModuleDir,
				anchorVersion:     anchor.Version,
				anchorKind:        string(anchor.Kind),
				anchorPath:        anchor.Path,
				anchorLine:        anchor.Line,
			}
			if !validImportIdentity(raw.importPath) ||
				!validFilePath(raw.anchorPath) ||
				raw.anchorLine <= 0 {
				continue
			}

			key := entryKey{pkg: raw.importPath, path: raw.anchorPath, line: raw.anchorLine}
			if existing, duplicate := rawByIdentity[key]; duplicate {
				if existing != raw {
					return Index{}, fmt.Errorf("workspace entrypoint index: conflicting exact identity")
				}
				continue
			}
			rawByIdentity[key] = raw

			pkg, ok := packageLookup[raw.importPath]
			if !ok ||
				pkg.Name != "main" ||
				raw.modulePath != pkg.ModulePath ||
				raw.packageDir != pkg.Dir ||
				raw.moduleRelativeDir != pkg.ModuleRelativeDir {
				continue
			}
			if _, ok := input.Graph.Module(pkg.ModuleID, pkg.ModulePath, raw.moduleDir); !ok {
				continue
			}
			if raw.anchorVersion != gofacts.EntrypointAnchorVersion ||
				raw.anchorKind != string(gofacts.EntrypointAnchorGoMain) {
				continue
			}

			file, ok := packageFile(pkg, raw.anchorPath)
			if !ok {
				continue
			}
			accepted = append(accepted, Entry{
				Kind:     raw.anchorKind,
				Package:  raw.importPath,
				Path:     raw.anchorPath,
				Symbol:   "main",
				Line:     raw.anchorLine,
				Openable: file.Openable,
			})
		}
	}

	sort.Slice(accepted, func(i, j int) bool {
		left, right := accepted[i], accepted[j]
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Symbol < right.Symbol
	})
	entryLookup := make(map[entryKey]int, min(len(accepted), MaxRawRows))
	for index, entry := range accepted {
		entryLookup[entryKey{pkg: entry.Package, path: entry.Path, line: entry.Line}] = index
	}
	return Index{
		entries:     accepted,
		entryLookup: entryLookup,
		initialized: true,
	}, nil
}

// Entries returns a defensive copy in deterministic package/path/line order.
func (index Index) Entries() []Entry {
	if !index.initialized || index.entries == nil {
		return nil
	}
	return append([]Entry(nil), index.entries...)
}

// Lookup returns one exact declaration without normalizing caller-controlled
// input or accepting an oversized lookup key.
func (index Index) Lookup(packagePath, filePath string, line int) (Entry, bool) {
	if !index.initialized ||
		!queryBounded(packagePath, filePath) ||
		!validImportIdentity(packagePath) ||
		!validFilePath(filePath) ||
		line <= 0 {
		return Entry{}, false
	}
	entryIndex, ok := index.entryLookup[entryKey{pkg: packagePath, path: filePath, line: line}]
	if !ok {
		return Entry{}, false
	}
	return index.entries[entryIndex], true
}

func preflight(entrypoints []gofacts.Entrypoint) (int, error) {
	if len(entrypoints) > MaxRawRows {
		return 0, errRawBounds
	}
	rawCount := 0
	for _, entrypoint := range entrypoints {
		if len(entrypoint.Anchors) > MaxRawRows-rawCount {
			return 0, errRawBounds
		}
		rawCount += len(entrypoint.Anchors)
	}

	budget := scalarBudget{remaining: MaxAggregateScalarBytes}
	for _, entrypoint := range entrypoints {
		if err := budget.consumeText(
			entrypoint.ModulePath,
			entrypoint.ImportPath,
			entrypoint.PackageDir,
			entrypoint.ModuleRelativeDir,
			entrypoint.ModuleDir,
		); err != nil {
			return 0, err
		}
		for _, anchor := range entrypoint.Anchors {
			if err := budget.consumeText(string(anchor.Kind), anchor.Path); err != nil {
				return 0, err
			}
			if err := budget.consumeInt(anchor.Version); err != nil {
				return 0, err
			}
			if err := budget.consumeInt(anchor.Line); err != nil {
				return 0, err
			}
		}
	}
	return rawCount, nil
}

type scalarBudget struct {
	remaining int
}

func (budget *scalarBudget) consumeText(values ...string) error {
	for _, value := range values {
		if len(value) > MaxScalarBytes {
			return errScalarBounds
		}
		if len(value) > budget.remaining {
			return errAggregateBounds
		}
		budget.remaining -= len(value)
	}
	return nil
}

func (budget *scalarBudget) consumeInt(value int) error {
	size := decimalBytes(value)
	if size > MaxScalarBytes {
		return errScalarBounds
	}
	if size > budget.remaining {
		return errAggregateBounds
	}
	budget.remaining -= size
	return nil
}

func decimalBytes(value int) int {
	size := 1
	var magnitude uint
	if value < 0 {
		size++
		magnitude = uint(-(value + 1))
		magnitude++
	} else {
		magnitude = uint(value)
	}
	for magnitude >= 10 {
		magnitude /= 10
		size++
	}
	return size
}

func packageFile(pkg workspacegraph.Package, filePath string) (workspacegraph.File, bool) {
	for _, file := range pkg.Files {
		if file.Path == filePath {
			return file, true
		}
	}
	return workspacegraph.File{}, false
}

func validImportIdentity(value string) bool {
	if !validIdentityText(value) ||
		strings.HasPrefix(value, "/") ||
		strings.HasSuffix(value, "/") ||
		strings.Contains(value, "//") ||
		strings.ContainsRune(value, '\\') {
		return false
	}
	for _, element := range strings.Split(value, "/") {
		if element == "" || element == "." || element == ".." {
			return false
		}
	}
	return true
}

func validIdentityText(value string) bool {
	if value == "" || len(value) > MaxScalarBytes || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func validFilePath(value string) bool {
	if value == "" ||
		value == "." ||
		len(value) > MaxScalarBytes ||
		!utf8.ValidString(value) ||
		!fs.ValidPath(value) ||
		path.Clean(value) != value ||
		strings.ContainsRune(value, '\\') {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func queryBounded(values ...string) bool {
	total := 0
	for _, value := range values {
		if len(value) > MaxScalarBytes ||
			len(value) > MaxAggregateScalarBytes-total {
			return false
		}
		total += len(value)
	}
	return true
}
