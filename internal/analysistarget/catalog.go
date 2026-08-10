package analysistarget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/gofacts"
)

const TargetCatalogVersion = 4

// PackageAPI is one package-qualified, names-only public API group for a
// module-library catalog entry. Exact locations remain in Go facts; the
// catalog binds the complete exported declaration inventory without making a
// package pretend to own the module-level target.
type PackageAPI struct {
	Package      TargetPackage                `json:"package"`
	Declarations []gofacts.PackageDeclaration `json:"declarations"`
}

// TargetCatalogEntry keeps display separate from canonical identity.
// Candidate.Target carries the composite module and canonical package
// identity; DisplayPath is only the flat repository-relative selector label.
type TargetCatalogEntry struct {
	Candidate           Candidate                    `json:"candidate"`
	DisplayPath         string                       `json:"display_path"`
	Symbols             []gofacts.PackageDeclaration `json:"symbols,omitempty"`
	PackageAPIs         []PackageAPI                 `json:"package_apis,omitempty"`
	DeclarationsScanned bool                         `json:"declarations_scanned,omitempty"`
}

// TargetCatalog is the complete exact candidate inventory for one Go-facts
// snapshot. Later semantic selection consumes this inventory without changing
// its local authority.
type TargetCatalog struct {
	Version          int                  `json:"version"`
	Ref              string               `json:"ref"`
	DefaultTargetRef string               `json:"default_target_ref,omitempty"`
	Entries          []TargetCatalogEntry `json:"entries"`
}

// BuildCatalog derives one sealed catalog from the existing exact package and
// entrypoint facts. It adds no quota, provider call, source scan, or package
// ranking.
func BuildCatalog(facts gofacts.Facts) (TargetCatalog, error) {
	candidates, err := Candidates(facts)
	if err != nil {
		return TargetCatalog{}, err
	}

	packages := make(map[string]gofacts.PackageFact, len(facts.Packages))
	for _, pkg := range facts.Packages {
		if pkg.Locality != "" && pkg.Locality != "local" {
			continue
		}
		packages[packageIdentityKey(pkg.ModuleID, pkg.CanonicalPath)] = pkg
	}
	modules := make(map[string]gofacts.ModuleFact, len(facts.Modules))
	for _, module := range facts.Modules {
		modules[module.ID] = module
	}

	entries := make([]TargetCatalogEntry, 0, len(candidates))
	for _, candidate := range candidates {
		module, ok := modules[candidate.Target.ModuleID]
		if !ok {
			return TargetCatalog{}, fmt.Errorf("analysis target catalog: missing exact module facts for %q", candidate.Target.ModuleID)
		}
		if candidate.Target.Kind == KindModuleLibrary {
			packageAPIs, err := catalogPackageAPIs(candidate.Target, packages)
			if err != nil {
				return TargetCatalog{}, err
			}
			entries = append(entries, TargetCatalogEntry{
				Candidate: candidate, DisplayPath: candidate.Target.ModuleDir,
				PackageAPIs: packageAPIs, DeclarationsScanned: true,
			})
			continue
		}

		pkg, ok := packages[packageIdentityKey(candidate.Target.ModuleID, candidate.Target.PackagePath)]
		if !ok {
			return TargetCatalog{}, fmt.Errorf("analysis target catalog: missing exact package facts for %q", candidate.Target.PackagePath)
		}
		displayPath, err := catalogDisplayPath(module.ModuleDir, pkg.ModuleRelativeDir)
		if err != nil {
			return TargetCatalog{}, fmt.Errorf("analysis target catalog: package %q display: %w", pkg.CanonicalPath, err)
		}
		if displayPath != candidate.Target.PackageDir {
			return TargetCatalog{}, fmt.Errorf(
				"analysis target catalog: package %q has inconsistent repository-relative directory %q (module layout gives %q)",
				pkg.CanonicalPath, candidate.Target.PackageDir, displayPath,
			)
		}
		symbols, err := catalogSymbols(pkg.Declarations, candidate.Target.Kind)
		if err != nil {
			return TargetCatalog{}, fmt.Errorf("analysis target catalog: package %q declarations: %w", pkg.CanonicalPath, err)
		}
		entries = append(entries, TargetCatalogEntry{
			Candidate: candidate, DisplayPath: displayPath, Symbols: symbols,
			DeclarationsScanned: pkg.DeclarationsScanned,
		})
	}
	sortCatalogEntries(entries)

	catalog := TargetCatalog{Version: TargetCatalogVersion, Entries: entries}
	if defaultTarget, ok := catalogDefault(entries); ok {
		catalog.DefaultTargetRef = defaultTarget.Ref
	}
	catalog.Ref, err = targetCatalogRef(catalog)
	if err != nil {
		return TargetCatalog{}, err
	}
	if err := catalog.Validate(); err != nil {
		return TargetCatalog{}, err
	}
	return catalog, nil
}

// Validate rejects identity, display, order, default, and seal drift.
func (catalog TargetCatalog) Validate() error {
	if catalog.Version != TargetCatalogVersion || strings.TrimSpace(catalog.Ref) == "" {
		return fmt.Errorf("analysis target catalog: invalid identity")
	}
	seenKeys := make(map[string]struct{}, len(catalog.Entries))
	seenRefs := make(map[string]struct{}, len(catalog.Entries))
	seenPackages := make(map[string]struct{}, len(catalog.Entries))
	seenModuleLibraries := make(map[string]struct{}, len(catalog.Entries))
	for index, entry := range catalog.Entries {
		if err := entry.Candidate.Target.Validate(); err != nil {
			return fmt.Errorf("analysis target catalog: entry %d target: %w", index, err)
		}
		candidate := entry.Candidate
		wantKey := targetCandidateKey(candidate.Target)
		if candidate.Key != wantKey {
			return fmt.Errorf("analysis target catalog: entry %d candidate key mismatch", index)
		}
		if candidate.Target.Kind != KindExecutablePackage && candidate.Target.Kind != KindModuleLibrary {
			return fmt.Errorf("analysis target catalog: entry %d has unsupported v4 target kind %q", index, candidate.Target.Kind)
		}
		if candidate.Target.Kind == KindModuleLibrary && candidate.EntrypointKind != "" {
			return fmt.Errorf("analysis target catalog: entry %d module library has executable kind", index)
		}
		if candidate.MainModule && candidate.Target.ModuleDir != "." {
			return fmt.Errorf("analysis target catalog: entry %d nested target cannot be in the root analysis module", index)
		}
		wantDisplay := candidate.Target.PackageDir
		if candidate.Target.Kind == KindModuleLibrary {
			wantDisplay = candidate.Target.ModuleDir
		}
		if entry.DisplayPath != wantDisplay {
			return fmt.Errorf("analysis target catalog: entry %d display path mismatch", index)
		}
		if err := gofacts.ValidatePackageDeclarations(entry.Symbols); err != nil {
			return fmt.Errorf("analysis target catalog: entry %d symbols: %w", index, err)
		}
		if err := validateNamesOnlyDeclarations(entry.Symbols); err != nil {
			return fmt.Errorf("analysis target catalog: entry %d symbols: %w", index, err)
		}
		if !entry.DeclarationsScanned && len(entry.Symbols) > 0 {
			return fmt.Errorf("analysis target catalog: entry %d has symbols without a complete declaration scan", index)
		}
		if candidate.Target.Kind == KindModuleLibrary {
			if len(entry.Symbols) != 0 || !entry.DeclarationsScanned {
				return fmt.Errorf("analysis target catalog: entry %d module library has invalid flat symbols", index)
			}
			if err := validateCatalogPackageAPIs(candidate.Target, entry.PackageAPIs); err != nil {
				return fmt.Errorf("analysis target catalog: entry %d package APIs: %w", index, err)
			}
		} else if len(entry.PackageAPIs) != 0 {
			return fmt.Errorf("analysis target catalog: entry %d executable has module package APIs", index)
		}
		if index > 0 && !catalogEntryLess(catalog.Entries[index-1], entry) {
			return fmt.Errorf("analysis target catalog: entries are not in canonical order")
		}
		if _, exists := seenKeys[candidate.Key]; exists {
			return fmt.Errorf("analysis target catalog: duplicate candidate key %q", candidate.Key)
		}
		if _, exists := seenRefs[candidate.Target.Ref]; exists {
			return fmt.Errorf("analysis target catalog: duplicate target ref %q", candidate.Target.Ref)
		}
		seenKeys[candidate.Key] = struct{}{}
		seenRefs[candidate.Target.Ref] = struct{}{}
		if candidate.Target.Kind == KindModuleLibrary {
			if _, exists := seenModuleLibraries[candidate.Target.ModuleID]; exists {
				return fmt.Errorf("analysis target catalog: duplicate module-library identity")
			}
			seenModuleLibraries[candidate.Target.ModuleID] = struct{}{}
		} else {
			packageIdentity := packageIdentityKey(candidate.Target.ModuleID, candidate.Target.PackagePath)
			if _, exists := seenPackages[packageIdentity]; exists {
				return fmt.Errorf("analysis target catalog: duplicate module/package identity")
			}
			seenPackages[packageIdentity] = struct{}{}
		}
	}

	wantDefault := ""
	if target, ok := catalogDefault(catalog.Entries); ok {
		wantDefault = target.Ref
	}
	if catalog.DefaultTargetRef != wantDefault {
		return fmt.Errorf("analysis target catalog: default target mismatch")
	}
	wantRef, err := targetCatalogRef(catalog)
	if err != nil {
		return err
	}
	if catalog.Ref != wantRef {
		return fmt.Errorf("analysis target catalog: ref binding mismatch")
	}
	return nil
}

func validateCatalogPackageAPIs(target Target, values []PackageAPI) error {
	if len(values) != len(target.LibraryPackages) {
		return fmt.Errorf("public package inventory mismatch")
	}
	for index, value := range values {
		if value.Package != target.LibraryPackages[index] || len(value.Declarations) == 0 {
			return fmt.Errorf("package %d identity mismatch", index)
		}
		if err := gofacts.ValidatePackageDeclarations(value.Declarations); err != nil {
			return fmt.Errorf("package %d declarations: %w", index, err)
		}
		if err := validateNamesOnlyDeclarations(value.Declarations); err != nil {
			return fmt.Errorf("package %d declarations: %w", index, err)
		}
		for _, declaration := range value.Declarations {
			if !declaration.ExportedAPI() {
				return fmt.Errorf("package %d has non-exported declaration", index)
			}
		}
	}
	return nil
}

func validateNamesOnlyDeclarations(values []gofacts.PackageDeclaration) error {
	for _, value := range values {
		if value.Path != "" || value.Line != 0 || value.Column != 0 || value.ExecutableBody {
			return fmt.Errorf("declaration inventory is not names-only")
		}
	}
	return nil
}

// CanonicalJSON returns the stable catalog bytes after validating its seal.
func (catalog TargetCatalog) CanonicalJSON() ([]byte, error) {
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(catalog)
}

// Snapshot returns an independently owned catalog value for live handoffs.
func (catalog TargetCatalog) Snapshot() TargetCatalog {
	result := catalog
	result.Entries = make([]TargetCatalogEntry, len(catalog.Entries))
	for index, entry := range catalog.Entries {
		result.Entries[index] = entry
		result.Entries[index].Candidate.Target = entry.Candidate.Target.Snapshot()
		result.Entries[index].Symbols = append([]gofacts.PackageDeclaration(nil), entry.Symbols...)
		result.Entries[index].PackageAPIs = snapshotPackageAPIs(entry.PackageAPIs)
	}
	return result
}

func snapshotPackageAPIs(values []PackageAPI) []PackageAPI {
	result := make([]PackageAPI, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Declarations = append([]gofacts.PackageDeclaration(nil), value.Declarations...)
	}
	return result
}

func catalogPackageAPIs(
	target Target,
	packages map[string]gofacts.PackageFact,
) ([]PackageAPI, error) {
	result := make([]PackageAPI, 0, len(target.LibraryPackages))
	for _, targetPackage := range target.LibraryPackages {
		pkg, ok := packages[packageIdentityKey(target.ModuleID, targetPackage.PackagePath)]
		if !ok || pkg.PackageDir != targetPackage.PackageDir || !pkg.DeclarationsScanned {
			return nil, fmt.Errorf("analysis target catalog: incomplete public API facts for %q", targetPackage.PackagePath)
		}
		declarations, err := catalogSymbols(pkg.Declarations, KindLibraryPackage)
		if err != nil {
			return nil, fmt.Errorf("analysis target catalog: package %q declarations: %w", pkg.CanonicalPath, err)
		}
		if len(declarations) == 0 {
			return nil, fmt.Errorf("analysis target catalog: public API package %q has no exported declarations", pkg.CanonicalPath)
		}
		result = append(result, PackageAPI{Package: targetPackage, Declarations: declarations})
	}
	return result, nil
}

func catalogSymbols(values []gofacts.PackageDeclaration, kind Kind) ([]gofacts.PackageDeclaration, error) {
	canonical, err := gofacts.CanonicalPackageDeclarations(values)
	if err != nil {
		return nil, err
	}
	result := make([]gofacts.PackageDeclaration, 0, len(canonical))
	for _, value := range canonical {
		if kind == KindLibraryPackage && !value.ExportedAPI() {
			continue
		}
		// Decision 274's provider authority is intentionally names-only. Exact
		// declaration locations remain in private producer Go facts for local
		// consumers such as Study; they do not alter target-catalog or request
		// identities.
		value.Path, value.Line, value.Column, value.ExecutableBody = "", 0, 0, false
		result = append(result, value)
	}
	result, err = gofacts.CanonicalPackageDeclarations(result)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func catalogDisplayPath(moduleDir, moduleRelativeDir string) (string, error) {
	moduleDir, err := canonicalPackageDir(moduleDir)
	if err != nil {
		return "", err
	}
	moduleRelativeDir, err = canonicalPackageDir(moduleRelativeDir)
	if err != nil {
		return "", err
	}
	switch {
	case moduleDir == "." && moduleRelativeDir == ".":
		return ".", nil
	case moduleDir == ".":
		return moduleRelativeDir, nil
	case moduleRelativeDir == ".":
		return moduleDir, nil
	default:
		return path.Join(moduleDir, moduleRelativeDir), nil
	}
}

func catalogDefault(entries []TargetCatalogEntry) (Target, bool) {
	candidates := make([]Candidate, 0, len(entries))
	for _, entry := range entries {
		candidates = append(candidates, entry.Candidate)
	}
	target, _, ok := autoSelect(candidates)
	return target, ok
}

func sortCatalogEntries(entries []TargetCatalogEntry) {
	sort.Slice(entries, func(i, j int) bool { return catalogEntryLess(entries[i], entries[j]) })
}

func catalogEntryLess(left, right TargetCatalogEntry) bool {
	if left.DisplayPath != right.DisplayPath {
		return left.DisplayPath < right.DisplayPath
	}
	if left.Candidate.Target.Kind != right.Candidate.Target.Kind {
		return left.Candidate.Target.Kind < right.Candidate.Target.Kind
	}
	return left.Candidate.Key < right.Candidate.Key
}

func targetCatalogRef(catalog TargetCatalog) (string, error) {
	identity := catalog
	identity.Ref = ""
	wire, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("analysis target catalog: encode identity: %w", err)
	}
	digest := sha256.Sum256(wire)
	return "atc-" + hex.EncodeToString(digest[:12]), nil
}
