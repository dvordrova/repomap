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

const TargetCatalogVersion = 3

// TargetCatalogEntry keeps display separate from canonical identity.
// Candidate.Target carries the composite module and canonical package
// identity; DisplayPath is only the flat repository-relative selector label.
type TargetCatalogEntry struct {
	Candidate           Candidate                    `json:"candidate"`
	DisplayPath         string                       `json:"display_path"`
	Symbols             []gofacts.PackageDeclaration `json:"symbols,omitempty"`
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
		pkg, ok := packages[packageIdentityKey(candidate.Target.ModuleID, candidate.Target.PackagePath)]
		if !ok {
			return TargetCatalog{}, fmt.Errorf("analysis target catalog: missing exact package facts for %q", candidate.Target.PackagePath)
		}
		module, ok := modules[candidate.Target.ModuleID]
		if !ok {
			return TargetCatalog{}, fmt.Errorf("analysis target catalog: missing exact module facts for %q", candidate.Target.ModuleID)
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
	for index, entry := range catalog.Entries {
		if err := entry.Candidate.Target.Validate(); err != nil {
			return fmt.Errorf("analysis target catalog: entry %d target: %w", index, err)
		}
		candidate := entry.Candidate
		wantKey := candidateKey(candidate.Target.ModulePath, candidate.Target.ModuleDir, candidate.Target.PackagePath)
		if candidate.Key != wantKey {
			return fmt.Errorf("analysis target catalog: entry %d candidate key mismatch", index)
		}
		if candidate.Target.Kind == KindLibraryPackage && candidate.EntrypointKind != "" {
			return fmt.Errorf("analysis target catalog: entry %d library has executable kind", index)
		}
		if candidate.MainModule && candidate.Target.ModuleDir != "." {
			return fmt.Errorf("analysis target catalog: entry %d nested package cannot be in the root analysis module", index)
		}
		if entry.DisplayPath != candidate.Target.PackageDir {
			return fmt.Errorf("analysis target catalog: entry %d display path mismatch", index)
		}
		if err := gofacts.ValidatePackageDeclarations(entry.Symbols); err != nil {
			return fmt.Errorf("analysis target catalog: entry %d symbols: %w", index, err)
		}
		if !entry.DeclarationsScanned && len(entry.Symbols) > 0 {
			return fmt.Errorf("analysis target catalog: entry %d has symbols without a complete declaration scan", index)
		}
		if candidate.Target.Kind == KindLibraryPackage {
			for _, symbol := range entry.Symbols {
				if !symbol.ExportedAPI() {
					return fmt.Errorf("analysis target catalog: entry %d library has non-exported symbol", index)
				}
			}
		}
		if candidate.Target.ModuleDir != "." &&
			candidate.Target.PackageDir != candidate.Target.ModuleDir &&
			!strings.HasPrefix(candidate.Target.PackageDir, candidate.Target.ModuleDir+"/") {
			return fmt.Errorf("analysis target catalog: entry %d package is outside its module directory", index)
		}
		if index > 0 && !catalogEntryLess(catalog.Entries[index-1], entry) {
			return fmt.Errorf("analysis target catalog: entries are not in canonical order")
		}
		packageIdentity := packageIdentityKey(candidate.Target.ModuleID, candidate.Target.PackagePath)
		if _, exists := seenKeys[candidate.Key]; exists {
			return fmt.Errorf("analysis target catalog: duplicate candidate key %q", candidate.Key)
		}
		if _, exists := seenRefs[candidate.Target.Ref]; exists {
			return fmt.Errorf("analysis target catalog: duplicate target ref %q", candidate.Target.Ref)
		}
		if _, exists := seenPackages[packageIdentity]; exists {
			return fmt.Errorf("analysis target catalog: duplicate module/package identity")
		}
		seenKeys[candidate.Key] = struct{}{}
		seenRefs[candidate.Target.Ref] = struct{}{}
		seenPackages[packageIdentity] = struct{}{}
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
	}
	return result
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
