// Package workspacepackageselection defines a bounded immutable projection of
// already-selected exact package identities from one authorized workspace
// package graph.
package workspacepackageselection

import (
	"errors"

	"github.com/dvordrova/repomap/internal/workspacegraph"
)

const (
	// MaxRows bounds the ordered candidate and selected package collections.
	MaxRows = 600
	// MaxScalarBytes bounds every consumed exact package identity scalar.
	MaxScalarBytes = 4096
	// MaxAggregateScalarBytes bounds all consumed identity scalars.
	MaxAggregateScalarBytes = 4 * 1024 * 1024
)

var (
	errRawBounds       = errors.New("workspace package selection: candidates exceed bounds")
	errScalarBounds    = errors.New("workspace package selection: scalar exceeds bounds")
	errAggregateBounds = errors.New("workspace package selection: aggregate scalars exceed bounds")
	errUnauthorized    = errors.New("workspace package selection: candidate is unavailable")
)

// Candidate is one caller-selected exact package identity.
type Candidate struct {
	CanonicalPath     string
	Name              string
	ModuleID          string
	ModulePath        string
	PackageDir        string
	ModuleRelativeDir string
}

// Package is one graph-authorized exact package identity. Files and
// presentation fields deliberately remain outside this projection.
type Package struct {
	CanonicalPath     string
	Name              string
	ModuleID          string
	ModulePath        string
	PackageDir        string
	ModuleRelativeDir string
}

// Input is the complete already-constructed authority and ordered selection.
// New performs no discovery, enumeration, traversal, sorting, or source
// access.
type Input struct {
	Graph      workspacegraph.Graph
	Candidates []Candidate
}

// Selection is an immutable ordered projection. Packages returns defensive
// copies and preserves the input's nil versus non-nil empty shape.
type Selection struct {
	packages    []Package
	initialized bool
}

// New validates the complete raw-input budget before graph lookup, hashing,
// map construction, or result allocation. It then authorizes every candidate
// as one exact graph package. Any unavailable or mismatched candidate rejects
// the complete selection.
func New(input Input) (Selection, error) {
	if err := preflight(input.Candidates); err != nil {
		return Selection{}, err
	}
	if input.Candidates == nil {
		return Selection{initialized: true}, nil
	}

	packages := make([]Package, len(input.Candidates))
	authorized := make(map[string]Package, min(len(input.Candidates), MaxRows))
	for index, candidate := range input.Candidates {
		pkg, ok := authorized[candidate.CanonicalPath]
		if !ok {
			graphPackage, found := input.Graph.Package(candidate.CanonicalPath)
			if !found {
				return Selection{}, errUnauthorized
			}
			pkg = Package{
				CanonicalPath:     graphPackage.CanonicalPath,
				Name:              graphPackage.Name,
				ModuleID:          graphPackage.ModuleID,
				ModulePath:        graphPackage.ModulePath,
				PackageDir:        graphPackage.Dir,
				ModuleRelativeDir: graphPackage.ModuleRelativeDir,
			}
			authorized[candidate.CanonicalPath] = pkg
		}
		if !matches(candidate, pkg) {
			return Selection{}, errUnauthorized
		}
		packages[index] = pkg
	}
	return Selection{packages: packages, initialized: true}, nil
}

// Packages returns a defensive copy in exact candidate order.
func (selection Selection) Packages() []Package {
	if !selection.initialized || selection.packages == nil {
		return nil
	}
	packages := make([]Package, len(selection.packages))
	copy(packages, selection.packages)
	return packages
}

func preflight(candidates []Candidate) error {
	if len(candidates) > MaxRows {
		return errRawBounds
	}
	for _, candidate := range candidates {
		if len(candidate.CanonicalPath) > MaxScalarBytes ||
			len(candidate.Name) > MaxScalarBytes ||
			len(candidate.ModuleID) > MaxScalarBytes ||
			len(candidate.ModulePath) > MaxScalarBytes ||
			len(candidate.PackageDir) > MaxScalarBytes ||
			len(candidate.ModuleRelativeDir) > MaxScalarBytes {
			return errScalarBounds
		}
	}

	remaining := MaxAggregateScalarBytes
	for _, candidate := range candidates {
		for _, scalar := range [...]string{
			candidate.CanonicalPath,
			candidate.Name,
			candidate.ModuleID,
			candidate.ModulePath,
			candidate.PackageDir,
			candidate.ModuleRelativeDir,
		} {
			if len(scalar) > remaining {
				return errAggregateBounds
			}
			remaining -= len(scalar)
		}
	}
	return nil
}

func matches(candidate Candidate, pkg Package) bool {
	return candidate.CanonicalPath == pkg.CanonicalPath &&
		candidate.Name == pkg.Name &&
		candidate.ModuleID == pkg.ModuleID &&
		candidate.ModulePath == pkg.ModulePath &&
		candidate.PackageDir == pkg.PackageDir &&
		candidate.ModuleRelativeDir == pkg.ModuleRelativeDir
}
