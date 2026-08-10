// Package analysistarget resolves one exact package target for an analysis.
// It is a local deterministic contract and performs no provider call.
package analysistarget

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const Version = 2

type Kind string

const (
	KindExecutablePackage Kind = "executable_package"
	KindLibraryPackage    Kind = "library_package"
	KindModuleLibrary     Kind = "module_library"
)

type RootBoundary string

const (
	RootBoundaryExactPackageMains RootBoundary = "exact_selected_package_mains"
	RootBoundaryExactPublicAPI    RootBoundary = "exact_public_api"
	RootBoundaryExactModuleAPI    RootBoundary = "exact_module_public_api"
)

// Root is one exact build-selected top-level main declaration. Library API
// roots are intentionally not invented from package facts; their boundary is
// resolved by a later exact public-API collector.
type Root struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// TargetPackage is one exact package identity within the target's owning Go
// module. PackageDir is repository-relative; PackagePath is the canonical Go
// import path. Module-library targets use these values instead of pretending
// that one package at the module root owns the module's public surface.
type TargetPackage struct {
	PackagePath string `json:"package_path"`
	PackageDir  string `json:"package_dir"`
}

// Target is the canonical package identity shared by all analysis stages.
// Ref seals the remaining fields and is safe to bind into run identities.
type Target struct {
	Version         int             `json:"version"`
	Ref             string          `json:"ref"`
	Kind            Kind            `json:"kind"`
	ModuleID        string          `json:"module_id"`
	ModulePath      string          `json:"module_path"`
	ModuleDir       string          `json:"module_dir"`
	PackagePath     string          `json:"package_path,omitempty"`
	PackageDir      string          `json:"package_dir,omitempty"`
	ModulePackages  []TargetPackage `json:"module_packages,omitempty"`
	LibraryPackages []TargetPackage `json:"library_packages,omitempty"`
	RootBoundary    RootBoundary    `json:"root_boundary"`
	Roots           []Root          `json:"roots"`
}

// Validate verifies both the target shape and its self-sealed Ref.
func (target Target) Validate() error {
	if target.Version != Version || strings.TrimSpace(target.ModuleID) == "" ||
		strings.TrimSpace(target.ModulePath) == "" || target.ModuleDir == "" {
		return fmt.Errorf("analysis target: invalid identity")
	}
	moduleDir, moduleDirErr := canonicalPackageDir(target.ModuleDir)
	if moduleDirErr != nil || target.ModuleDir != moduleDir {
		return fmt.Errorf("analysis target: non-canonical directory identity")
	}
	switch target.Kind {
	case KindExecutablePackage:
		if err := validatePackageTargetIdentity(target); err != nil {
			return err
		}
		if target.RootBoundary != RootBoundaryExactPackageMains || len(target.Roots) == 0 ||
			len(target.ModulePackages) != 0 || len(target.LibraryPackages) != 0 {
			return fmt.Errorf("analysis target: executable target requires exact package mains")
		}
	case KindLibraryPackage:
		// Package-library production was superseded by module_library in target
		// catalog v4. The kind remains valid for in-memory consumers while they
		// move atomically to Target v2; no v4 catalog producer emits it.
		if err := validatePackageTargetIdentity(target); err != nil {
			return err
		}
		if target.RootBoundary != RootBoundaryExactPublicAPI || len(target.Roots) != 0 ||
			len(target.ModulePackages) != 0 || len(target.LibraryPackages) != 0 {
			return fmt.Errorf("analysis target: library target requires an unresolved exact public API boundary")
		}
	case KindModuleLibrary:
		if target.PackagePath != "" || target.PackageDir != "" || len(target.Roots) != 0 ||
			target.RootBoundary != RootBoundaryExactModuleAPI {
			return fmt.Errorf("analysis target: module library cannot claim one package identity")
		}
		if err := validateModuleTargetPackages(target); err != nil {
			return err
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
	// These inventories are optional on executable and transitional package
	// targets. Keep their canonical omitted representation instead of turning a
	// nil slice into a non-nil empty slice: target-bearing artifacts use
	// `omitempty`, so a JSON round trip restores the omitted form. Non-empty
	// module-library inventories still receive independently owned backing
	// arrays.
	if len(target.ModulePackages) == 0 {
		copyTarget.ModulePackages = nil
	} else {
		copyTarget.ModulePackages = append([]TargetPackage(nil), target.ModulePackages...)
	}
	if len(target.LibraryPackages) == 0 {
		copyTarget.LibraryPackages = nil
	} else {
		copyTarget.LibraryPackages = append([]TargetPackage(nil), target.LibraryPackages...)
	}
	return copyTarget
}

// DisplayPath returns the repository-relative target label after Validate has
// established the target kind. A module library is displayed by its go.mod
// directory; package targets retain their exact package directory.
func (target Target) DisplayPath() string {
	if target.Kind == KindModuleLibrary {
		return target.ModuleDir
	}
	return target.PackageDir
}

// RootPackages returns independently owned exact package roots after Validate
// has established the target shape. Executables and transitional package
// libraries have one package root; a module library has every API-bearing
// externally importable package root.
func (target Target) RootPackages() []TargetPackage {
	if target.Kind == KindModuleLibrary {
		return append([]TargetPackage(nil), target.LibraryPackages...)
	}
	if target.PackagePath == "" || target.PackageDir == "" {
		return nil
	}
	return []TargetPackage{{PackagePath: target.PackagePath, PackageDir: target.PackageDir}}
}

func validatePackageTargetIdentity(target Target) error {
	if strings.TrimSpace(target.PackagePath) == "" || target.PackageDir == "" {
		return fmt.Errorf("analysis target: package target has no exact package identity")
	}
	packageDir, err := canonicalPackageDir(target.PackageDir)
	if err != nil || target.PackageDir != packageDir || !packageBelongsToModule(target.ModulePath, target.PackagePath) ||
		!directoryBelongsToModule(target.ModuleDir, target.PackageDir) {
		return fmt.Errorf("analysis target: invalid package identity")
	}
	return nil
}

func validateModuleTargetPackages(target Target) error {
	if len(target.ModulePackages) == 0 || len(target.LibraryPackages) == 0 {
		return fmt.Errorf("analysis target: module library requires exact package and public-root identities")
	}
	if err := validateCanonicalTargetPackages(target.ModulePath, target.ModuleDir, target.ModulePackages); err != nil {
		return fmt.Errorf("analysis target: invalid module package inventory: %w", err)
	}
	if err := validateCanonicalTargetPackages(target.ModulePath, target.ModuleDir, target.LibraryPackages); err != nil {
		return fmt.Errorf("analysis target: invalid module public-root inventory: %w", err)
	}
	modulePackages := make(map[TargetPackage]struct{}, len(target.ModulePackages))
	for _, pkg := range target.ModulePackages {
		modulePackages[pkg] = struct{}{}
	}
	for _, pkg := range target.LibraryPackages {
		if _, ok := modulePackages[pkg]; !ok {
			return fmt.Errorf("analysis target: module public root %q is outside module scope", pkg.PackagePath)
		}
		if internalModulePackage(moduleRelativeTargetPackageDir(target.ModuleDir, pkg.PackageDir)) {
			return fmt.Errorf("analysis target: internal package %q cannot be a module public root", pkg.PackagePath)
		}
	}
	return nil
}

func validateCanonicalTargetPackages(modulePath, moduleDir string, values []TargetPackage) error {
	for index, value := range values {
		packageDir, err := canonicalPackageDir(value.PackageDir)
		if err != nil || value.PackageDir != packageDir || strings.TrimSpace(value.PackagePath) == "" ||
			!packageBelongsToModule(modulePath, value.PackagePath) ||
			!directoryBelongsToModule(moduleDir, value.PackageDir) {
			return fmt.Errorf("package %d has invalid identity", index)
		}
		if index > 0 && !targetPackageLess(values[index-1], value) {
			return fmt.Errorf("packages are not in canonical order")
		}
		if index > 0 && (values[index-1].PackagePath == value.PackagePath ||
			values[index-1].PackageDir == value.PackageDir) {
			return fmt.Errorf("packages contain a duplicate path or directory")
		}
	}
	return nil
}

func moduleRelativeTargetPackageDir(moduleDir, packageDir string) string {
	if moduleDir == "." {
		return packageDir
	}
	if packageDir == moduleDir {
		return "."
	}
	return strings.TrimPrefix(packageDir, moduleDir+"/")
}

func canonicalTargetPackages(values []TargetPackage) []TargetPackage {
	result := append([]TargetPackage(nil), values...)
	sort.Slice(result, func(i, j int) bool { return targetPackageLess(result[i], result[j]) })
	compacted := result[:0]
	for _, value := range result {
		if len(compacted) == 0 || compacted[len(compacted)-1] != value {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

func targetPackageLess(left, right TargetPackage) bool {
	if left.PackagePath != right.PackagePath {
		return left.PackagePath < right.PackagePath
	}
	return left.PackageDir < right.PackageDir
}

func packageBelongsToModule(modulePath, packagePath string) bool {
	return packagePath == modulePath || strings.HasPrefix(packagePath, modulePath+"/")
}

func directoryBelongsToModule(moduleDir, packageDir string) bool {
	return moduleDir == "." || packageDir == moduleDir || strings.HasPrefix(packageDir, moduleDir+"/")
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
