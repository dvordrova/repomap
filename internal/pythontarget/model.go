// Package pythontarget discovers exact Python analysis targets without
// importing or executing repository code.
package pythontarget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/dvordrova/repomap/internal/corpus"
)

const (
	CatalogVersion        = 3
	TargetVersion         = 3
	TargetIdentityVersion = 3
	// AdvisoryCatalogBytes is a diagnostic usual size for the complete sealed
	// in-memory target catalog. Crossing it never narrows or rejects targets.
	AdvisoryCatalogBytes = 64 << 20
)

type Kind string

const (
	KindExecutable Kind = "executable"
	KindLibrary    Kind = "library"
)

type RootKind string

const (
	RootCallable        RootKind = "callable"
	RootModule          RootKind = "module"
	RootModuleExecution RootKind = "module_execution"
	RootMainGuard       RootKind = "main_guard"
	RootScriptFile      RootKind = "script_file"
	RootBoundObject     RootKind = "bound_object"
)

type BasisKind string

const (
	BasisPEP621Script        BasisKind = "pep621_project_script"
	BasisPEP621GUIScript     BasisKind = "pep621_project_gui_script"
	BasisPoetryScript        BasisKind = "poetry_script"
	BasisSetupCFGScript      BasisKind = "setup_cfg_console_script"
	BasisSetupCFGGUIScript   BasisKind = "setup_cfg_gui_script"
	BasisSetupPYScript       BasisKind = "setup_py_console_script"
	BasisSetupPYGUIScript    BasisKind = "setup_py_gui_script"
	BasisPackageMain         BasisKind = "package_main"
	BasisNameMainGuard       BasisKind = "name_main_guard"
	BasisPythonShebang       BasisKind = "python_shebang"
	BasisModuleExecutionView BasisKind = "module_execution_view"
	BasisImportPackage       BasisKind = "import_package"
)

type Coverage string

const (
	CoverageComplete Coverage = "complete"
	CoveragePartial  Coverage = "partial"
)

type OmissionKind string

const (
	OmissionDynamicSetup      OmissionKind = "dynamic_setup_py"
	OmissionSourceSyntax      OmissionKind = "source_syntax"
	OmissionUnresolvedRoot    OmissionKind = "unresolved_root"
	OmissionAmbiguousRoot     OmissionKind = "ambiguous_root"
	OmissionUnsupportedLaunch OmissionKind = "unsupported_launch"
)

// Root is an exact local Python execution root. Module is an import name for
// declarative callable/module roots and a project-local source identity for
// exact file execution roots. Qualname is populated for exact callable and
// object bindings.
type Root struct {
	Kind     RootKind `json:"kind"`
	Module   string   `json:"module"`
	Qualname string   `json:"qualname,omitempty"`
	Path     string   `json:"path"`
	Line     int      `json:"line"`
}

// Package is one first-party top-level import package or module in a library
// target. Dir and Path are repository-relative; namespace packages have no
// __init__.py Path and are marked explicitly.
type Package struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	Path      string `json:"path,omitempty"`
	Namespace bool   `json:"namespace,omitempty"`
}

// Module is one Python source file in the owning project's exact inventory.
// Importable distinguishes package/module resolution from a runnable file
// whose authority is its exact path and top-level __name__ guard.
type Module struct {
	FileID     corpus.FileID `json:"file_id"`
	Name       string        `json:"name"`
	Path       string        `json:"path"`
	Importable bool          `json:"importable,omitempty"`
	Package    bool          `json:"package,omitempty"`
}

// ModuleScope is the sealed, language-local inventory from which a selected
// exact file can be projected into a framework-neutral module-execution view.
// Scopes are resolver authority, not native target hypotheses: their presence
// never advertises every Python module to the model.
type ModuleScope struct {
	Ref         string   `json:"ref"`
	ProjectDir  string   `json:"project_dir"`
	SourceRoots []string `json:"source_roots"`
	Modules     []Module `json:"modules"`
}

// Basis is one exact local fact that establishes a target. Some targets are
// advertised natively; module-execution views are instead materialized only
// after another cube selects their exact file. Label is a closed script or
// symbol name, never confidence, prose, or a copied shell command.
type Basis struct {
	FileID corpus.FileID `json:"file_id"`
	Kind   BasisKind     `json:"kind"`
	Path   string        `json:"path"`
	Line   int           `json:"line,omitempty"`
	Label  string        `json:"label,omitempty"`
}

// Omission is a typed reason why local facts could not establish
// an exact target. It makes partial discovery explicit without promoting a
// guess into the catalog.
type Omission struct {
	Kind  OmissionKind `json:"kind"`
	Path  string       `json:"path"`
	Line  int          `json:"line,omitempty"`
	Label string       `json:"label,omitempty"`
}

// Target is a sealed Python target. Executables have exact Roots; libraries
// have an exact discovered top-level Packages inventory.
type Target struct {
	Version       int             `json:"version"`
	Ref           string          `json:"ref"`
	IdentityRef   string          `json:"identity_ref"`
	Kind          Kind            `json:"kind"`
	Selector      string          `json:"selector"`
	DisplayName   string          `json:"display_name"`
	ProjectDir    string          `json:"project_dir"`
	ScopeRef      string          `json:"scope_ref,omitempty"`
	SourceRoots   []string        `json:"source_roots"`
	SourceRefs    []corpus.FileID `json:"source_refs"`
	AnchorFileRef corpus.FileID   `json:"anchor_file_ref"`
	Modules       []Module        `json:"modules"`
	Roots         []Root          `json:"roots,omitempty"`
	Packages      []Package       `json:"packages,omitempty"`
	Basis         []Basis         `json:"basis"`
}

// Catalog is the complete canonical Python target and module-scope inventory
// produced by one discovery pass. Ref seals every target, scope, and ordering.
type Catalog struct {
	Version      int           `json:"version"`
	Ref          string        `json:"ref"`
	Coverage     Coverage      `json:"coverage"`
	Entries      []Target      `json:"entries"`
	ModuleScopes []ModuleScope `json:"module_scopes"`
	Omissions    []Omission    `json:"omissions,omitempty"`
}

// Snapshot returns a fully consumer-owned copy of one exact Python target.
func (target Target) Snapshot() Target {
	copyTarget := target
	copyTarget.SourceRoots = append([]string(nil), target.SourceRoots...)
	copyTarget.SourceRefs = append([]corpus.FileID(nil), target.SourceRefs...)
	copyTarget.Modules = append([]Module(nil), target.Modules...)
	copyTarget.Roots = append([]Root(nil), target.Roots...)
	copyTarget.Packages = append([]Package(nil), target.Packages...)
	copyTarget.Basis = append([]Basis(nil), target.Basis...)
	return copyTarget
}

func (target Target) Validate() error {
	if target.Version != TargetVersion || !strings.HasPrefix(target.Ref, "pyt-") ||
		!strings.HasPrefix(target.IdentityRef, "pyti-") {
		return fmt.Errorf("python target: invalid identity")
	}
	if !validLabel(target.Selector) || !validLabel(target.DisplayName) {
		return fmt.Errorf("python target: invalid selector or display name")
	}
	if hasRootKind(target.Roots, RootModuleExecution) {
		if !strings.HasPrefix(target.ScopeRef, "pys-") {
			return fmt.Errorf("python target: module-execution view has no sealed scope")
		}
	} else if target.ScopeRef != "" {
		return fmt.Errorf("python target: native target unexpectedly cites a resolver scope")
	}
	if err := validateRepoDir(target.ProjectDir); err != nil {
		return fmt.Errorf("python target: project directory: %w", err)
	}
	if len(target.SourceRoots) == 0 ||
		!sort.StringsAreSorted(target.SourceRoots) || hasDuplicateStrings(target.SourceRoots) {
		return fmt.Errorf("python target: source roots are not canonical")
	}
	for _, sourceRoot := range target.SourceRoots {
		if err := validateRepoDir(sourceRoot); err != nil {
			return fmt.Errorf("python target: source root %q: %w", sourceRoot, err)
		}
		if !pathWithin(target.ProjectDir, sourceRoot) {
			return fmt.Errorf("python target: source root %q escapes project", sourceRoot)
		}
	}
	if len(target.Modules) == 0 {
		return fmt.Errorf("python target: invalid module inventory")
	}
	if err := validateModules(target.Modules); err != nil {
		return err
	}
	for _, module := range target.Modules {
		if !pathWithin(target.ProjectDir, module.Path) {
			return fmt.Errorf("python target: module %q is outside project scope", module.Name)
		}
		if module.Importable && longestContainingPath(target.SourceRoots, module.Path) == "" {
			return fmt.Errorf("python target: module %q is outside source roots", module.Name)
		}
	}
	if len(target.Basis) == 0 {
		return fmt.Errorf("python target: invalid basis inventory")
	}
	if err := validateBasis(target.Basis); err != nil {
		return err
	}
	if err := validateSourceRefs(target.SourceRefs); err != nil {
		return err
	}
	if !sameFileIDs(target.SourceRefs, sourceRefsForTarget(target)) {
		return fmt.Errorf("python target: source refs are not derived from exact target sources")
	}
	if !validCorpusFileID(target.AnchorFileRef) || !containsFileID(target.SourceRefs, target.AnchorFileRef) ||
		target.AnchorFileRef != anchorFileRefForTarget(target) {
		return fmt.Errorf("python target: anchor file ref is not the canonical target anchor")
	}
	wantIdentity, err := targetIdentityRef(target)
	if err != nil {
		return err
	}
	if target.IdentityRef != wantIdentity {
		return fmt.Errorf("python target: identity ref binding mismatch")
	}

	switch target.Kind {
	case KindExecutable:
		if len(target.Roots) == 0 || len(target.Packages) != 0 {
			return fmt.Errorf("python target: executable requires exact roots and no package inventory")
		}
		if err := validateRoots(target.Roots); err != nil {
			return err
		}
		if err := validateRootsAgainstModules(target.Roots, target.Modules); err != nil {
			return err
		}
	case KindLibrary:
		if len(target.Roots) != 0 || len(target.Packages) == 0 {
			return fmt.Errorf("python target: library requires packages and no execution roots")
		}
		if err := validatePackages(target.Packages); err != nil {
			return err
		}
		for _, pkg := range target.Packages {
			if longestContainingPath(target.SourceRoots, pkg.Dir) == "" {
				return fmt.Errorf("python target: package %q is outside source roots", pkg.Name)
			}
		}
	default:
		return fmt.Errorf("python target: invalid kind %q", target.Kind)
	}

	want, err := targetRef(target)
	if err != nil {
		return err
	}
	if target.Ref != want {
		return fmt.Errorf("python target: ref binding mismatch")
	}
	return nil
}

func (catalog Catalog) Validate() error {
	if catalog.Version != CatalogVersion || !strings.HasPrefix(catalog.Ref, "pytc-") {
		return fmt.Errorf("python target catalog: invalid identity")
	}
	if (len(catalog.Omissions) == 0 && catalog.Coverage != CoverageComplete) ||
		(len(catalog.Omissions) > 0 && catalog.Coverage != CoveragePartial) {
		return fmt.Errorf("python target catalog: coverage does not match omissions")
	}
	if err := validateOmissions(catalog.Omissions); err != nil {
		return err
	}
	if err := validateModuleScopes(catalog.ModuleScopes); err != nil {
		return err
	}
	seenRefs := make(map[string]struct{}, len(catalog.Entries))
	seenSelectors := make(map[string]struct{}, len(catalog.Entries))
	for index, target := range catalog.Entries {
		if err := target.Validate(); err != nil {
			return fmt.Errorf("python target catalog: entry %d: %w", index, err)
		}
		if index > 0 && !targetLess(catalog.Entries[index-1], target) {
			return fmt.Errorf("python target catalog: entries are not in canonical order")
		}
		if _, exists := seenRefs[target.Ref]; exists {
			return fmt.Errorf("python target catalog: duplicate target ref %q", target.Ref)
		}
		if _, exists := seenSelectors[target.Selector]; exists {
			return fmt.Errorf("python target catalog: duplicate selector %q", target.Selector)
		}
		seenRefs[target.Ref] = struct{}{}
		seenSelectors[target.Selector] = struct{}{}
		if !catalogScopeOwnsTarget(catalog.ModuleScopes, target) {
			return fmt.Errorf("python target catalog: entry %d is outside sealed module scopes", index)
		}
	}
	want, err := catalogRef(catalog)
	if err != nil {
		return err
	}
	if catalog.Ref != want {
		return fmt.Errorf("python target catalog: ref binding mismatch")
	}
	return nil
}

func (catalog Catalog) Snapshot() Catalog {
	copyCatalog := catalog
	copyCatalog.Omissions = append([]Omission(nil), catalog.Omissions...)
	copyCatalog.ModuleScopes = cloneModuleScopes(catalog.ModuleScopes)
	copyCatalog.Entries = make([]Target, len(catalog.Entries))
	for index, target := range catalog.Entries {
		copyCatalog.Entries[index] = target.Snapshot()
	}
	return copyCatalog
}

// OwnsTarget reports whether target is either an exact native catalog entry
// or the unique framework-neutral module-execution view derivable from one of
// the catalog's sealed module scopes. It never accepts an independently
// constructed semantic target merely because its fields look plausible.
func (catalog Catalog) OwnsTarget(target Target) bool {
	if catalog.Validate() != nil || target.Validate() != nil {
		return false
	}
	if target.ScopeRef == "" {
		for _, entry := range catalog.Entries {
			if entry.Ref == target.Ref {
				return true
			}
		}
		return false
	}
	if !resolverOnlyModuleExecutionTarget(target) || !catalogScopeOwnsTarget(catalog.ModuleScopes, target) {
		return false
	}
	for _, scope := range catalog.ModuleScopes {
		if scope.Ref != target.ScopeRef {
			continue
		}
		for _, module := range scope.Modules {
			if module.FileID == target.AnchorFileRef {
				want, err := newModuleExecutionTarget(scope, module)
				return err == nil && want.Ref == target.Ref
			}
		}
	}
	return false
}

// ResolveSelector restores one exact catalog-owned target without requiring
// repository source access. Native selectors resolve to their sealed catalog
// entry; module-execution selectors are deterministically rebuilt from the
// catalog's sealed module scope. Paths and display names are never aliases.
func (catalog Catalog) ResolveSelector(selector string) (Target, bool, error) {
	if err := catalog.Validate(); err != nil {
		return Target{}, false, err
	}
	for _, entry := range catalog.Entries {
		if entry.Selector == selector {
			return cloneFileResolverTarget(entry), true, nil
		}
	}
	const prefix = "python:module-execution:"
	if !strings.HasPrefix(selector, prefix) {
		return Target{}, false, nil
	}
	remainder := strings.TrimPrefix(selector, prefix)
	scopeKey, fileKey, ok := strings.Cut(remainder, ":")
	if !ok || scopeKey == "" || fileKey == "" {
		return Target{}, false, nil
	}
	fileRef := corpus.FileID(fileKey)
	for _, scope := range catalog.ModuleScopes {
		if strings.TrimPrefix(scope.Ref, "pys-") != scopeKey {
			continue
		}
		for _, module := range scope.Modules {
			if module.FileID != fileRef {
				continue
			}
			target, err := newModuleExecutionTarget(scope, module)
			if err != nil {
				return Target{}, false, err
			}
			if target.Selector != selector || !catalog.OwnsTarget(target) {
				return Target{}, false, nil
			}
			return cloneFileResolverTarget(target), true, nil
		}
	}
	return Target{}, false, nil
}

// NewCatalog canonicalizes and seals adapter-owned Python target facts into
// the same persisted catalog consumed by the rest of the pipeline. Callers
// provide exact local facts; this constructor derives every target and catalog
// identity instead of accepting copied refs or anchors as authority.
func NewCatalog(entries []Target, omissions []Omission) (Catalog, error) {
	ownedEntries := make([]Target, 0, len(entries))
	for _, input := range entries {
		target := input
		target.SourceRoots = append([]string(nil), input.SourceRoots...)
		target.SourceRefs = append([]corpus.FileID(nil), input.SourceRefs...)
		target.Modules = append([]Module(nil), input.Modules...)
		target.Roots = append([]Root(nil), input.Roots...)
		target.Packages = append([]Package(nil), input.Packages...)
		target.Basis = append([]Basis(nil), input.Basis...)
		canonicalizeTarget(&target)
		sealed, err := sealTarget(target)
		if err != nil {
			return Catalog{}, err
		}
		ownedEntries = append(ownedEntries, sealed)
	}
	scopes, err := moduleScopesFromTargets(ownedEntries)
	if err != nil {
		return Catalog{}, err
	}
	return sealCatalog(ownedEntries, scopes, append([]Omission(nil), omissions...))
}

func newCatalogWithModuleScopes(entries []Target, scopes []ModuleScope, omissions []Omission) (Catalog, error) {
	ownedEntries := make([]Target, 0, len(entries))
	for _, input := range entries {
		target := cloneTarget(input)
		canonicalizeTarget(&target)
		sealed, err := sealTarget(target)
		if err != nil {
			return Catalog{}, err
		}
		ownedEntries = append(ownedEntries, sealed)
	}
	return sealCatalog(ownedEntries, cloneModuleScopes(scopes), append([]Omission(nil), omissions...))
}

func validateModuleScopes(values []ModuleScope) error {
	seenRefs := make(map[string]struct{}, len(values))
	seenProjects := make(map[string]struct{}, len(values))
	for index, scope := range values {
		if index > 0 && !moduleScopeLess(values[index-1], scope) {
			return fmt.Errorf("python target catalog: module scopes are not canonical")
		}
		if err := validateRepoDir(scope.ProjectDir); err != nil {
			return fmt.Errorf("python target catalog: module scope %d project: %w", index, err)
		}
		if len(scope.SourceRoots) == 0 ||
			!sort.StringsAreSorted(scope.SourceRoots) || hasDuplicateStrings(scope.SourceRoots) {
			return fmt.Errorf("python target catalog: module scope %d source roots are not canonical", index)
		}
		for _, sourceRoot := range scope.SourceRoots {
			if err := validateRepoDir(sourceRoot); err != nil || !pathWithin(scope.ProjectDir, sourceRoot) {
				return fmt.Errorf("python target catalog: module scope %d has invalid source root %q", index, sourceRoot)
			}
		}
		if len(scope.Modules) == 0 {
			return fmt.Errorf("python target catalog: module scope %d has invalid module inventory", index)
		}
		if err := validateModules(scope.Modules); err != nil {
			return fmt.Errorf("python target catalog: module scope %d: %w", index, err)
		}
		for _, module := range scope.Modules {
			if !pathWithin(scope.ProjectDir, module.Path) ||
				module.Importable && longestContainingPath(scope.SourceRoots, module.Path) == "" {
				return fmt.Errorf("python target catalog: module scope %d does not own %q", index, module.Path)
			}
		}
		wantRef, err := moduleScopeRef(scope)
		if err != nil {
			return err
		}
		if scope.Ref != wantRef {
			return fmt.Errorf("python target catalog: module scope %d identity mismatch", index)
		}
		if _, duplicate := seenRefs[scope.Ref]; duplicate {
			return fmt.Errorf("python target catalog: duplicate module scope ref %q", scope.Ref)
		}
		if _, duplicate := seenProjects[scope.ProjectDir]; duplicate {
			return fmt.Errorf("python target catalog: duplicate project module scope %q", scope.ProjectDir)
		}
		seenRefs[scope.Ref] = struct{}{}
		seenProjects[scope.ProjectDir] = struct{}{}
	}
	return nil
}

func validateModules(values []Module) error {
	seenPaths := make(map[string]Module, len(values))
	seenImportNames := make(map[string]string)
	seenFileIDs := make(map[corpus.FileID]string, len(values))
	for index, value := range values {
		if !validCorpusFileID(value.FileID) {
			return fmt.Errorf("python target: module %d has invalid file ID", index)
		}
		if (value.Importable && !validModule(value.Name)) || (!value.Importable && !validSourceIdentity(value.Name)) {
			return fmt.Errorf("python target: module %d has invalid source identity", index)
		}
		if err := validateRepoFile(value.Path); err != nil {
			return fmt.Errorf("python target: module %d: %w", index, err)
		}
		if index > 0 && !moduleLess(values[index-1], value) {
			return fmt.Errorf("python target: modules are not canonical")
		}
		if previous, exists := seenPaths[value.Path]; exists && previous != value {
			return fmt.Errorf("python target: source %q has conflicting identities", value.Path)
		}
		if previous, exists := seenFileIDs[value.FileID]; exists && previous != value.Path {
			return fmt.Errorf("python target: file ID %q maps to conflicting paths", value.FileID)
		}
		if previous, exists := seenImportNames[value.Name]; value.Importable && exists && previous != value.Path {
			return fmt.Errorf("python target: import name %q is ambiguous", value.Name)
		}
		if value.Package && !value.Importable {
			return fmt.Errorf("python target: module %d non-importable source cannot be a package", index)
		}
		if value.Importable && value.Package != strings.HasSuffix(value.Path, "/__init__.py") &&
			!(value.Package && value.Path == "__init__.py") {
			return fmt.Errorf("python target: module %d package marker mismatch", index)
		}
		seenPaths[value.Path] = value
		seenFileIDs[value.FileID] = value.Path
		if value.Importable {
			seenImportNames[value.Name] = value.Path
		}
	}
	return nil
}

func validateRootsAgainstModules(roots []Root, modules []Module) error {
	byPath := make(map[string]Module, len(modules))
	for _, module := range modules {
		byPath[module.Path] = module
	}
	for index, root := range roots {
		module, ok := byPath[root.Path]
		if !ok {
			return fmt.Errorf("python target: root %d is outside module inventory", index)
		}
		if root.Kind == RootModule {
			if !module.Importable {
				return fmt.Errorf("python target: module root %d is not importable", index)
			}
			if module.Name != root.Module && module.Name != root.Module+".__main__" {
				return fmt.Errorf("python target: module root %d does not match source identity", index)
			}
		} else if root.Kind == RootModuleExecution {
			if module.Name != root.Module {
				return fmt.Errorf("python target: module-execution root %d does not match source identity", index)
			}
		} else if root.Kind == RootCallable || root.Kind == RootBoundObject {
			if !module.Importable || module.Name != root.Module {
				return fmt.Errorf("python target: semantic root %d does not match import identity", index)
			}
		} else if module.Name != root.Module {
			return fmt.Errorf("python target: root %d does not match source identity", index)
		}
	}
	return nil
}

func validateOmissions(values []Omission) error {
	for index, value := range values {
		switch value.Kind {
		case OmissionDynamicSetup, OmissionSourceSyntax, OmissionUnresolvedRoot,
			OmissionAmbiguousRoot, OmissionUnsupportedLaunch:
		default:
			return fmt.Errorf("python target catalog: omission %d has invalid kind %q", index, value.Kind)
		}
		if err := validateRepoFile(value.Path); err != nil {
			return fmt.Errorf("python target catalog: omission %d: %w", index, err)
		}
		if value.Line < 0 || (value.Label != "" && !validLabel(value.Label)) {
			return fmt.Errorf("python target catalog: omission %d has invalid detail", index)
		}
		if index > 0 && !omissionLess(values[index-1], value) {
			return fmt.Errorf("python target catalog: omissions are not canonical")
		}
	}
	return nil
}

func validateRoots(values []Root) error {
	for index, value := range values {
		if !validSourceIdentity(value.Module) || value.Line <= 0 {
			return fmt.Errorf("python target: root %d has invalid source identity or line", index)
		}
		if err := validateRepoFile(value.Path); err != nil {
			return fmt.Errorf("python target: root %d: %w", index, err)
		}
		switch value.Kind {
		case RootCallable, RootBoundObject:
			if !validQualname(value.Qualname) {
				return fmt.Errorf("python target: semantic root %d has invalid qualname", index)
			}
		case RootModule, RootModuleExecution, RootMainGuard, RootScriptFile:
			if value.Qualname != "" {
				return fmt.Errorf("python target: non-callable root %d has a qualname", index)
			}
		default:
			return fmt.Errorf("python target: root %d has invalid kind %q", index, value.Kind)
		}
		if index > 0 && !rootLess(values[index-1], value) {
			return fmt.Errorf("python target: roots are not canonical")
		}
	}
	return nil
}

func validatePackages(values []Package) error {
	for index, value := range values {
		if !validModulePart(value.Name) {
			return fmt.Errorf("python target: package %d has invalid import name", index)
		}
		if err := validateRepoDir(value.Dir); err != nil {
			return fmt.Errorf("python target: package %d directory: %w", index, err)
		}
		if value.Namespace {
			if value.Path != "" {
				return fmt.Errorf("python target: namespace package %d has an init path", index)
			}
		} else if err := validateRepoFile(value.Path); err != nil {
			return fmt.Errorf("python target: package %d path: %w", index, err)
		}
		if index > 0 && !packageLess(values[index-1], value) {
			return fmt.Errorf("python target: packages are not canonical")
		}
	}
	return nil
}

func validateBasis(values []Basis) error {
	for index, value := range values {
		if !validCorpusFileID(value.FileID) {
			return fmt.Errorf("python target: basis %d has invalid file ID", index)
		}
		switch value.Kind {
		case BasisPEP621Script, BasisPEP621GUIScript, BasisPoetryScript,
			BasisSetupCFGScript, BasisSetupCFGGUIScript,
			BasisSetupPYScript, BasisSetupPYGUIScript, BasisPackageMain, BasisNameMainGuard, BasisPythonShebang,
			BasisModuleExecutionView, BasisImportPackage:
		default:
			return fmt.Errorf("python target: basis %d has invalid kind %q", index, value.Kind)
		}
		if err := validateRepoFile(value.Path); err != nil {
			return fmt.Errorf("python target: basis %d: %w", index, err)
		}
		if value.Line < 0 {
			return fmt.Errorf("python target: basis %d has invalid line", index)
		}
		if value.Label != "" && !validLabel(value.Label) {
			return fmt.Errorf("python target: basis %d has invalid label", index)
		}
		if index > 0 && !basisLess(values[index-1], value) {
			return fmt.Errorf("python target: basis is not canonical")
		}
	}
	return nil
}

func validLabel(value string) bool {
	return value != "" && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func validateSourceRefs(values []corpus.FileID) error {
	if len(values) == 0 {
		return fmt.Errorf("python target: invalid source ref inventory")
	}
	for index, value := range values {
		if !validCorpusFileID(value) {
			return fmt.Errorf("python target: source ref %d has invalid file ID", index)
		}
		if index > 0 && !corpusFileIDLess(values[index-1], value) {
			return fmt.Errorf("python target: source refs are not canonical")
		}
	}
	return nil
}

func targetRef(target Target) (string, error) {
	identity := target
	identity.Ref = ""
	wire, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("python target: encode identity: %w", err)
	}
	digest := sha256.Sum256(wire)
	return "pyt-" + hex.EncodeToString(digest[:12]), nil
}

// targetSemanticIdentity intentionally excludes presentation, catalog
// provenance, and the current repository inventory. It is the exact merge key
// for independently discovered evidence about the same target shape.
type targetSemanticIdentity struct {
	Version     int       `json:"version"`
	Kind        Kind      `json:"kind"`
	ProjectDir  string    `json:"project_dir"`
	ScopeRef    string    `json:"scope_ref,omitempty"`
	SourceRoots []string  `json:"source_roots"`
	Roots       []Root    `json:"roots,omitempty"`
	Packages    []Package `json:"packages,omitempty"`
}

func targetIdentityRef(target Target) (string, error) {
	identity := targetSemanticIdentity{
		Version:     TargetIdentityVersion,
		Kind:        target.Kind,
		ProjectDir:  target.ProjectDir,
		ScopeRef:    target.ScopeRef,
		SourceRoots: target.SourceRoots,
		Roots:       target.Roots,
		Packages:    target.Packages,
	}
	wire, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("python target: encode semantic identity: %w", err)
	}
	digest := sha256.Sum256(wire)
	return "pyti-" + hex.EncodeToString(digest[:12]), nil
}

func catalogRef(catalog Catalog) (string, error) {
	identity := catalog
	identity.Ref = ""
	wire, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("python target catalog: encode identity: %w", err)
	}
	digest := sha256.Sum256(wire)
	return "pytc-" + hex.EncodeToString(digest[:12]), nil
}

func moduleScopeRef(scope ModuleScope) (string, error) {
	identity := scope
	identity.Ref = ""
	wire, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("python target module scope: encode identity: %w", err)
	}
	digest := sha256.Sum256(wire)
	return "pys-" + hex.EncodeToString(digest[:12]), nil
}

func sealTarget(target Target) (Target, error) {
	var err error
	target.SourceRefs = sourceRefsForTarget(target)
	target.AnchorFileRef = anchorFileRefForTarget(target)
	target.IdentityRef, err = targetIdentityRef(target)
	if err != nil {
		return Target{}, err
	}
	target.Ref, err = targetRef(target)
	if err != nil {
		return Target{}, err
	}
	if err := target.Validate(); err != nil {
		return Target{}, err
	}
	return target, nil
}

func sealCatalog(entries []Target, scopes []ModuleScope, omissions []Omission) (Catalog, error) {
	sort.Slice(entries, func(i, j int) bool { return targetLess(entries[i], entries[j]) })
	sealedScopes, err := sealModuleScopes(scopes)
	if err != nil {
		return Catalog{}, err
	}
	sort.Slice(omissions, func(i, j int) bool { return omissionLess(omissions[i], omissions[j]) })
	omissions = compactOmissions(omissions)
	coverage := CoverageComplete
	if len(omissions) > 0 {
		coverage = CoveragePartial
	}
	catalog := Catalog{
		Version: CatalogVersion, Coverage: coverage, Entries: entries,
		ModuleScopes: sealedScopes, Omissions: omissions,
	}
	catalog.Ref, err = catalogRef(catalog)
	if err != nil {
		return Catalog{}, err
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func sealModuleScopes(values []ModuleScope) ([]ModuleScope, error) {
	owned := cloneModuleScopes(values)
	for index := range owned {
		canonicalizeModuleScope(&owned[index])
		ref, err := moduleScopeRef(owned[index])
		if err != nil {
			return nil, err
		}
		owned[index].Ref = ref
	}
	sort.Slice(owned, func(i, j int) bool { return moduleScopeLess(owned[i], owned[j]) })
	if err := validateModuleScopes(owned); err != nil {
		return nil, err
	}
	return owned, nil
}

func canonicalizeModuleScope(scope *ModuleScope) {
	sort.Strings(scope.SourceRoots)
	scope.SourceRoots = compactStrings(scope.SourceRoots)
	sort.Slice(scope.Modules, func(i, j int) bool { return moduleLess(scope.Modules[i], scope.Modules[j]) })
}

func moduleScopeLess(left, right ModuleScope) bool {
	if left.ProjectDir != right.ProjectDir {
		return left.ProjectDir < right.ProjectDir
	}
	return left.Ref < right.Ref
}

func moduleScopesFromTargets(targets []Target) ([]ModuleScope, error) {
	byProject := make(map[string]ModuleScope)
	for _, target := range targets {
		scope := ModuleScope{
			ProjectDir: target.ProjectDir, SourceRoots: cloneStrings(target.SourceRoots),
			Modules: cloneModules(target.Modules),
		}
		if previous, exists := byProject[target.ProjectDir]; exists {
			if !sameStrings(previous.SourceRoots, scope.SourceRoots) || !sameModules(previous.Modules, scope.Modules) {
				return nil, fmt.Errorf("python target catalog: project %q has conflicting module scopes", target.ProjectDir)
			}
			continue
		}
		byProject[target.ProjectDir] = scope
	}
	result := make([]ModuleScope, 0, len(byProject))
	for _, scope := range byProject {
		result = append(result, scope)
	}
	return result, nil
}

func cloneModuleScopes(values []ModuleScope) []ModuleScope {
	if values == nil {
		return nil
	}
	result := make([]ModuleScope, len(values))
	for index, scope := range values {
		result[index] = scope
		result[index].SourceRoots = cloneStrings(scope.SourceRoots)
		result[index].Modules = cloneModules(scope.Modules)
	}
	return result
}

func cloneTarget(input Target) Target {
	target := input
	target.SourceRoots = cloneStrings(input.SourceRoots)
	target.SourceRefs = append([]corpus.FileID(nil), input.SourceRefs...)
	target.Modules = cloneModules(input.Modules)
	target.Roots = append([]Root(nil), input.Roots...)
	target.Packages = append([]Package(nil), input.Packages...)
	target.Basis = append([]Basis(nil), input.Basis...)
	return target
}

func catalogScopeOwnsTarget(scopes []ModuleScope, target Target) bool {
	for _, scope := range scopes {
		if scope.ProjectDir == target.ProjectDir && sameStrings(scope.SourceRoots, target.SourceRoots) &&
			sameModules(scope.Modules, target.Modules) {
			if target.ScopeRef == "" || target.ScopeRef == scope.Ref {
				return true
			}
		}
	}
	return false
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameModules(left, right []Module) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func hasRootKind(roots []Root, kind RootKind) bool {
	for _, root := range roots {
		if root.Kind == kind {
			return true
		}
	}
	return false
}

func omissionLess(left, right Omission) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	return left.Label < right.Label
}

func compactOmissions(values []Omission) []Omission {
	if len(values) == 0 {
		return nil
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func targetLess(left, right Target) bool {
	if left.Selector != right.Selector {
		return left.Selector < right.Selector
	}
	return left.Ref < right.Ref
}

func rootLess(left, right Root) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	if left.Module != right.Module {
		return left.Module < right.Module
	}
	if left.Qualname != right.Qualname {
		return left.Qualname < right.Qualname
	}
	return left.Kind < right.Kind
}

func packageLess(left, right Package) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.Dir != right.Dir {
		return left.Dir < right.Dir
	}
	return left.Path < right.Path
}

func moduleLess(left, right Module) bool {
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Importable != right.Importable {
		return !left.Importable && right.Importable
	}
	return !left.Package && right.Package
}

func basisLess(left, right Basis) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.Line != right.Line {
		return left.Line < right.Line
	}
	return left.Label < right.Label
}

func sourceRefsForTarget(target Target) []corpus.FileID {
	values := make([]corpus.FileID, 0, len(target.Roots))
	switch target.Kind {
	case KindExecutable:
		for _, root := range target.Roots {
			values = append(values, moduleFileIDByPath(target.Modules, root.Path))
		}
	case KindLibrary:
		values = append(values, anchorFileRefForTarget(target))
	}
	sort.Slice(values, func(i, j int) bool { return corpusFileIDLess(values[i], values[j]) })
	return compactFileIDs(values)
}

func anchorFileRefForTarget(target Target) corpus.FileID {
	switch target.Kind {
	case KindExecutable:
		if len(target.Roots) > 0 {
			return moduleFileIDByPath(target.Modules, target.Roots[0].Path)
		}
	case KindLibrary:
		for _, basis := range target.Basis {
			if basis.Kind == BasisImportPackage && validCorpusFileID(basis.FileID) {
				return basis.FileID
			}
		}
		if len(target.Packages) > 0 {
			return packageFileID(target.Modules, target.Packages[0])
		}
	}
	return ""
}

func packageFileID(modules []Module, pkg Package) corpus.FileID {
	if pkg.Path != "" {
		return moduleFileIDByPath(modules, pkg.Path)
	}
	for _, module := range modules {
		if strings.Split(module.Name, ".")[0] == pkg.Name {
			return module.FileID
		}
	}
	return ""
}

func moduleFileIDByPath(modules []Module, filePath string) corpus.FileID {
	for _, module := range modules {
		if module.Path == filePath {
			return module.FileID
		}
	}
	return ""
}

func compactFileIDs(values []corpus.FileID) []corpus.FileID {
	if len(values) == 0 {
		return nil
	}
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func sameFileIDs(left, right []corpus.FileID) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func containsFileID(values []corpus.FileID, expected corpus.FileID) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func validCorpusFileID(value corpus.FileID) bool {
	wire := string(value)
	if len(wire) < 2 || wire[0] != 'f' || wire[1] == '0' {
		return false
	}
	for _, character := range wire[1:] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func corpusFileIDLess(left, right corpus.FileID) bool {
	if !validCorpusFileID(left) || !validCorpusFileID(right) {
		return string(left) < string(right)
	}
	leftDigits := string(left)[1:]
	rightDigits := string(right)[1:]
	if len(leftDigits) != len(rightDigits) {
		return len(leftDigits) < len(rightDigits)
	}
	return leftDigits < rightDigits
}

func validateRepoDir(value string) error {
	if value == "." {
		return nil
	}
	return validateRepoPath(value, false)
}

func validateRepoFile(value string) error {
	return validateRepoPath(value, true)
}

func validateRepoPath(value string, requireFile bool) error {
	if value == "" || strings.ContainsRune(value, '\x00') || strings.Contains(value, "\\") ||
		strings.HasPrefix(value, "/") || path.Clean(value) != value || value == ".." ||
		strings.HasPrefix(value, "../") {
		return fmt.Errorf("non-canonical repository-relative path %q", value)
	}
	if requireFile && value == "." {
		return fmt.Errorf("path is not a file")
	}
	return nil
}

func validModule(value string) bool {
	if value == "" {
		return false
	}
	for _, part := range strings.Split(value, ".") {
		if !validModulePart(part) {
			return false
		}
	}
	return true
}

func validQualname(value string) bool { return validModule(value) }

func validSourceIdentity(value string) bool {
	return value != "" && strings.TrimSpace(value) == value &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func validModulePart(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if index == 0 {
			if char != '_' && !unicode.IsLetter(char) {
				return false
			}
			continue
		}
		if char != '_' && !unicode.IsLetter(char) && !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func hasDuplicateStrings(values []string) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] == values[index] {
			return true
		}
	}
	return false
}
