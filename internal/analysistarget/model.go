// Package analysistarget resolves one exact package target for an analysis.
// It is a local deterministic contract and performs no provider call.
package analysistarget

import (
	"encoding/json"
	"errors"
	"fmt"
)

const Version = 1

type Kind string

const (
	KindExecutablePackage Kind = "executable_package"
	KindLibraryPackage    Kind = "library_package"
)

type RootBoundary string

const (
	RootBoundaryExactPackageMains RootBoundary = "exact_selected_package_mains"
	RootBoundaryExactPublicAPI    RootBoundary = "exact_public_api"
)

// Root is one exact build-selected top-level main declaration. Library API
// roots are intentionally not invented from package facts; their boundary is
// resolved by a later exact public-API collector.
type Root struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// Target is the canonical package identity shared by all analysis stages.
// Ref seals the remaining fields and is safe to bind into run identities.
type Target struct {
	Version      int          `json:"version"`
	Ref          string       `json:"ref"`
	Kind         Kind         `json:"kind"`
	ModuleID     string       `json:"module_id"`
	ModulePath   string       `json:"module_path"`
	ModuleDir    string       `json:"module_dir"`
	PackagePath  string       `json:"package_path"`
	PackageDir   string       `json:"package_dir"`
	RootBoundary RootBoundary `json:"root_boundary"`
	Roots        []Root       `json:"roots"`
}

// Validate verifies both the target shape and its self-sealed Ref.
func (target Target) Validate() error {
	if target.Version != Version || target.ModuleID == "" || target.ModulePath == "" ||
		target.ModuleDir == "" || target.PackagePath == "" || target.PackageDir == "" {
		return fmt.Errorf("analysis target: invalid identity")
	}
	moduleDir, moduleDirErr := canonicalPackageDir(target.ModuleDir)
	packageDir, packageDirErr := canonicalPackageDir(target.PackageDir)
	if moduleDirErr != nil || packageDirErr != nil || target.ModuleDir != moduleDir || target.PackageDir != packageDir {
		return fmt.Errorf("analysis target: non-canonical directory identity")
	}
	switch target.Kind {
	case KindExecutablePackage:
		if target.RootBoundary != RootBoundaryExactPackageMains || len(target.Roots) == 0 {
			return fmt.Errorf("analysis target: executable target requires exact package mains")
		}
	case KindLibraryPackage:
		if target.RootBoundary != RootBoundaryExactPublicAPI || len(target.Roots) != 0 {
			return fmt.Errorf("analysis target: library target requires an unresolved exact public API boundary")
		}
	default:
		return fmt.Errorf("analysis target: invalid kind %q", target.Kind)
	}
	canonical := canonicalRoots(target.Roots)
	if len(canonical) != len(target.Roots) {
		return fmt.Errorf("analysis target: roots are not canonical")
	}
	for index := range canonical {
		validatedRoot, err := canonicalRoot(target.Roots[index].Path, target.Roots[index].Line)
		if err != nil || validatedRoot != target.Roots[index] || canonical[index] != target.Roots[index] {
			return fmt.Errorf("analysis target: roots are not canonical")
		}
	}
	want, err := targetRef(target)
	if err != nil {
		return err
	}
	if target.Ref != want {
		return fmt.Errorf("analysis target: ref binding mismatch")
	}
	return nil
}

// CanonicalJSON returns the stable target bytes used by later artifact
// bindings. It rejects an invalid or drifted self-seal.
func (target Target) CanonicalJSON() ([]byte, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(target)
}

// Snapshot returns an independently owned target value for live handoffs.
func (target Target) Snapshot() Target {
	copyTarget := target
	copyTarget.Roots = append([]Root{}, target.Roots...)
	return copyTarget
}

// Candidate is one exact package the user may select. Key is the durable
// explicit-selection spelling; aliases are only accepted when unambiguous.
type Candidate struct {
	Key            string `json:"key"`
	Target         Target `json:"target"`
	MainModule     bool   `json:"main_module"`
	EntrypointKind string `json:"entrypoint_kind,omitempty"`
}

type ResolutionState string

const (
	ResolutionSelected    ResolutionState = "selected"
	ResolutionAmbiguous   ResolutionState = "ambiguous"
	ResolutionUnavailable ResolutionState = "unavailable"
)

type Resolution struct {
	State      ResolutionState `json:"state"`
	Reason     string          `json:"reason"`
	Selected   *Target         `json:"selected,omitempty"`
	Candidates []Candidate     `json:"candidates"`
}

type Options struct {
	Override string
}

var (
	ErrOverrideNotFound  = errors.New("analysis target override not found")
	ErrOverrideAmbiguous = errors.New("analysis target override is ambiguous")
)
