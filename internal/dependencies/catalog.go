// Package dependencies owns the language-neutral repository dependency model.
package dependencies

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const Version = 1

// Kind describes where a dependency package is implemented relative to the
// selected repository. Language adapters establish the kind from their exact
// package-loading authority; the core does not infer it from path spelling.
type Kind string

const (
	KindWorkspace Kind = "workspace"
	KindStdlib    Kind = "stdlib"
	KindExternal  Kind = "external"
)

type CoverageState string

const (
	CoverageComplete CoverageState = "complete"
	CoveragePartial  CoverageState = "partial"
)

// OmissionReason is a closed explanation for one exact direct import that a
// language adapter observed but could not represent as a typed dependency.
type OmissionReason string

const (
	OmissionImporterIdentityUnavailable OmissionReason = "importer_identity_unavailable"
	OmissionDependencyMetadataMissing   OmissionReason = "dependency_metadata_missing"
	OmissionDependencyLoadUnavailable   OmissionReason = "dependency_load_unavailable"
	OmissionDependencyIdentityMissing   OmissionReason = "dependency_identity_missing"
	OmissionModuleAuthorityMissing      OmissionReason = "module_authority_missing"
)

type Omission struct {
	ImporterRef         string         `json:"importer_ref,omitempty"`
	ImporterPackagePath string         `json:"importer_package_path,omitempty"`
	PackagePath         string         `json:"package_path"`
	Reason              OmissionReason `json:"reason"`
}

// Coverage makes partial language-tool results explicit. Counts are exact
// package-level direct-import uses after adapter deduplication.
type Coverage struct {
	State           CoverageState `json:"state"`
	ImportsObserved int           `json:"imports_observed"`
	ImportsRetained int           `json:"imports_retained"`
	Omissions       []Omission    `json:"omissions"`
}

// Replacement records an exact module replacement without exposing an
// absolute host path. A versioned module replacement has ModulePath; a local
// replacement has Local set and may have RepositoryPath when it is inside the
// selected repository.
type Replacement struct {
	ModulePath     string `json:"module_path,omitempty"`
	ModuleVersion  string `json:"module_version,omitempty"`
	Local          bool   `json:"local,omitempty"`
	RepositoryPath string `json:"repository_path,omitempty"`
}

// Importer is one exact repository package that directly imports a
// dependency. Ref is a stable local identity; provider-facing cubes must map
// it to a request-local short ref before advertising it to a model.
type Importer struct {
	Ref            string `json:"ref"`
	Language       string `json:"language"`
	Name           string `json:"name"`
	ModulePath     string `json:"module_path"`
	PackagePath    string `json:"package_path"`
	RepositoryPath string `json:"repository_path"`
}

// Dependency is one exact directly imported package and the repository
// packages that import it. RepositoryPath is populated only for workspace
// dependencies. ID is a stable local identity, not a model-facing ref.
type Dependency struct {
	ID             string       `json:"id"`
	Language       string       `json:"language"`
	Kind           Kind         `json:"kind"`
	Name           string       `json:"name"`
	ModulePath     string       `json:"module_path,omitempty"`
	ModuleVersion  string       `json:"module_version,omitempty"`
	PackagePath    string       `json:"package_path"`
	RepositoryPath string       `json:"repository_path,omitempty"`
	Replacement    *Replacement `json:"replacement,omitempty"`
	ImporterRefs   []string     `json:"importer_refs"`
}

// Catalog is the canonical dependency handoff shared by language adapters and
// later integration cubes.
type Catalog struct {
	Version      int          `json:"version"`
	Importers    []Importer   `json:"importers"`
	Dependencies []Dependency `json:"dependencies"`
	Coverage     Coverage     `json:"coverage"`
}

// BuildWithOmissions builds a catalog while retaining exact direct imports
// that the adapter observed but could not type without inventing authority.
func BuildWithOmissions(importers []Importer, values []Dependency, omissions []Omission) (Catalog, error) {
	importerByRef := make(map[string]Importer, len(importers))
	for _, importer := range importers {
		var err error
		importer, err = SealImporter(importer)
		if err != nil {
			return Catalog{}, err
		}
		if previous, exists := importerByRef[importer.Ref]; exists && previous != importer {
			return Catalog{}, fmt.Errorf("dependencies: conflicting importer identity %q", importer.Ref)
		}
		importerByRef[importer.Ref] = importer
	}

	dependencyByID := make(map[string]Dependency, len(values))
	for _, value := range values {
		value.ID = dependencyIdentity(value)
		value.ImporterRefs = canonicalStrings(value.ImporterRefs)
		if err := validateDependencyShape(value); err != nil {
			return Catalog{}, err
		}
		for _, ref := range value.ImporterRefs {
			if _, ok := importerByRef[ref]; !ok {
				return Catalog{}, fmt.Errorf("dependencies: dependency %q has unknown importer ref %q", value.ID, ref)
			}
		}
		if previous, exists := dependencyByID[value.ID]; exists {
			if !sameDependencyIdentity(previous, value) {
				return Catalog{}, fmt.Errorf("dependencies: conflicting dependency identity %q", value.ID)
			}
			previous.ImporterRefs = canonicalStrings(append(previous.ImporterRefs, value.ImporterRefs...))
			dependencyByID[value.ID] = previous
			continue
		}
		dependencyByID[value.ID] = snapshotDependency(value)
	}

	catalog := Catalog{
		Version:      Version,
		Importers:    make([]Importer, 0, len(importerByRef)),
		Dependencies: make([]Dependency, 0, len(dependencyByID)),
		Coverage: Coverage{
			State:     CoverageComplete,
			Omissions: canonicalOmissions(omissions),
		},
	}
	for _, importer := range importerByRef {
		catalog.Importers = append(catalog.Importers, importer)
	}
	for _, value := range dependencyByID {
		catalog.Dependencies = append(catalog.Dependencies, value)
		catalog.Coverage.ImportsRetained += len(value.ImporterRefs)
	}
	catalog.Coverage.ImportsObserved = catalog.Coverage.ImportsRetained + len(catalog.Coverage.Omissions)
	if len(catalog.Coverage.Omissions) > 0 {
		catalog.Coverage.State = CoveragePartial
	}
	sortCatalog(&catalog)
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// SealImporter binds an importer to its stable local ref. Provider-facing
// cubes must still replace that ref with a request-local short ref.
func SealImporter(importer Importer) (Importer, error) {
	importer.Ref = importerIdentity(importer)
	if err := validateImporter(importer); err != nil {
		return Importer{}, err
	}
	return importer, nil
}

// Empty returns a valid complete catalog with no dependency rows.
func Empty() Catalog {
	return Catalog{
		Version: Version, Importers: []Importer{}, Dependencies: []Dependency{},
		Coverage: Coverage{State: CoverageComplete, Omissions: []Omission{}},
	}
}

// Validate verifies canonical order, stable identity bindings and exact
// importer-ref resolution.
func (catalog Catalog) Validate() error {
	if catalog.Version != Version {
		return fmt.Errorf("dependencies: unsupported catalog version %d", catalog.Version)
	}
	seenImporters := make(map[string]Importer, len(catalog.Importers))
	for index, importer := range catalog.Importers {
		if err := validateImporter(importer); err != nil {
			return err
		}
		if importer.Ref != importerIdentity(importer) {
			return fmt.Errorf("dependencies: importer ref binding mismatch")
		}
		if _, exists := seenImporters[importer.Ref]; exists {
			return fmt.Errorf("dependencies: duplicate importer ref %q", importer.Ref)
		}
		if index > 0 && !importerLess(catalog.Importers[index-1], importer) {
			return fmt.Errorf("dependencies: importers are not in canonical order")
		}
		seenImporters[importer.Ref] = importer
	}
	seenDependencies := make(map[string]struct{}, len(catalog.Dependencies))
	for index, value := range catalog.Dependencies {
		if err := validateDependencyShape(value); err != nil {
			return err
		}
		if value.ID != dependencyIdentity(value) {
			return fmt.Errorf("dependencies: dependency id binding mismatch")
		}
		if _, exists := seenDependencies[value.ID]; exists {
			return fmt.Errorf("dependencies: duplicate dependency id %q", value.ID)
		}
		if index > 0 && !dependencyLess(catalog.Dependencies[index-1], value) {
			return fmt.Errorf("dependencies: dependencies are not in canonical order")
		}
		for refIndex, ref := range value.ImporterRefs {
			if _, ok := seenImporters[ref]; !ok {
				return fmt.Errorf("dependencies: dependency %q has unknown importer ref %q", value.ID, ref)
			}
			if refIndex > 0 && value.ImporterRefs[refIndex-1] >= ref {
				return fmt.Errorf("dependencies: importer refs are not canonical")
			}
		}
		seenDependencies[value.ID] = struct{}{}
	}
	if err := validateCoverage(catalog.Coverage, seenImporters, catalog.Dependencies); err != nil {
		return err
	}
	return nil
}

// Subset retains only the supplied importer refs and dependencies used by at
// least one of them. Dependency identities remain stable because importer
// membership is not part of dependency identity.
func (catalog Catalog) Subset(importerRefs map[string]struct{}) (Catalog, error) {
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	importers := make([]Importer, 0, len(importerRefs))
	kept := make(map[string]struct{}, len(importerRefs))
	for _, importer := range catalog.Importers {
		if _, ok := importerRefs[importer.Ref]; !ok {
			continue
		}
		importers = append(importers, importer)
		kept[importer.Ref] = struct{}{}
	}
	values := make([]Dependency, 0, len(catalog.Dependencies))
	for _, value := range catalog.Dependencies {
		copyValue := snapshotDependency(value)
		copyValue.ImporterRefs = copyValue.ImporterRefs[:0]
		for _, ref := range value.ImporterRefs {
			if _, ok := kept[ref]; ok {
				copyValue.ImporterRefs = append(copyValue.ImporterRefs, ref)
			}
		}
		if len(copyValue.ImporterRefs) > 0 {
			values = append(values, copyValue)
		}
	}
	omissions := make([]Omission, 0, len(catalog.Coverage.Omissions))
	for _, omission := range catalog.Coverage.Omissions {
		if omission.ImporterRef == "" {
			continue
		}
		if _, ok := kept[omission.ImporterRef]; ok {
			omissions = append(omissions, omission)
		}
	}
	return BuildWithOmissions(importers, values, omissions)
}

func validateCoverage(coverage Coverage, importers map[string]Importer, values []Dependency) error {
	retained := 0
	for _, value := range values {
		retained += len(value.ImporterRefs)
	}
	if coverage.ImportsRetained != retained || coverage.ImportsObserved != retained+len(coverage.Omissions) {
		return fmt.Errorf("dependencies: coverage counts do not match dependency uses")
	}
	if len(coverage.Omissions) == 0 {
		if coverage.State != CoverageComplete {
			return fmt.Errorf("dependencies: complete coverage state mismatch")
		}
	} else if coverage.State != CoveragePartial {
		return fmt.Errorf("dependencies: partial coverage state mismatch")
	}
	for index, omission := range coverage.Omissions {
		if !plainValue(omission.PackagePath) || !validOmissionReason(omission.Reason) ||
			(omission.ImporterPackagePath != "" && !plainValue(omission.ImporterPackagePath)) {
			return fmt.Errorf("dependencies: invalid coverage omission")
		}
		if omission.ImporterRef != "" {
			importer, ok := importers[omission.ImporterRef]
			if !ok || importer.PackagePath != omission.ImporterPackagePath {
				return fmt.Errorf("dependencies: coverage omission has unknown importer ref %q", omission.ImporterRef)
			}
		} else if omission.Reason != OmissionImporterIdentityUnavailable {
			return fmt.Errorf("dependencies: coverage omission requires an importer ref")
		}
		if index > 0 && !omissionLess(coverage.Omissions[index-1], omission) {
			return fmt.Errorf("dependencies: coverage omissions are not canonical")
		}
	}
	return nil
}

func validOmissionReason(reason OmissionReason) bool {
	switch reason {
	case OmissionImporterIdentityUnavailable, OmissionDependencyMetadataMissing,
		OmissionDependencyLoadUnavailable, OmissionDependencyIdentityMissing,
		OmissionModuleAuthorityMissing:
		return true
	default:
		return false
	}
}

func validateImporter(value Importer) error {
	if !plainValue(value.Language) || !plainValue(value.Name) || !plainValue(value.ModulePath) ||
		!plainValue(value.PackagePath) || !repositoryPath(value.RepositoryPath) {
		return fmt.Errorf("dependencies: invalid importer")
	}
	return nil
}

func validateDependencyShape(value Dependency) error {
	if !plainValue(value.Language) || !plainValue(value.Name) || !plainValue(value.PackagePath) ||
		len(value.ImporterRefs) == 0 || (value.ModuleVersion != "" && !plainValue(value.ModuleVersion)) {
		return fmt.Errorf("dependencies: invalid dependency")
	}
	switch value.Kind {
	case KindWorkspace:
		if !plainValue(value.ModulePath) || value.ModuleVersion != "" ||
			!repositoryPath(value.RepositoryPath) || value.Replacement != nil {
			return fmt.Errorf("dependencies: invalid workspace dependency")
		}
	case KindStdlib:
		if value.ModulePath != "" || value.ModuleVersion != "" || value.RepositoryPath != "" || value.Replacement != nil {
			return fmt.Errorf("dependencies: invalid standard-library dependency")
		}
	case KindExternal:
		if !plainValue(value.ModulePath) || value.RepositoryPath != "" {
			return fmt.Errorf("dependencies: invalid external dependency")
		}
	default:
		return fmt.Errorf("dependencies: invalid dependency kind %q", value.Kind)
	}
	if value.Replacement != nil {
		replacement := value.Replacement
		if replacement.Local {
			if replacement.ModulePath != "" || replacement.ModuleVersion != "" ||
				(replacement.RepositoryPath != "" && !repositoryPath(replacement.RepositoryPath)) {
				return fmt.Errorf("dependencies: invalid local module replacement")
			}
		} else if !plainValue(replacement.ModulePath) || replacement.RepositoryPath != "" ||
			(replacement.ModuleVersion != "" && !plainValue(replacement.ModuleVersion)) {
			return fmt.Errorf("dependencies: invalid versioned module replacement")
		}
	}
	return nil
}

func plainValue(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\x00\r\n")
}

func repositoryPath(value string) bool {
	if !plainValue(value) || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "../") || value == ".." ||
		strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == ".." {
			return false
		}
	}
	return true
}

func importerIdentity(value Importer) string {
	return stableID("importer", value.Language, value.ModulePath, value.PackagePath, value.RepositoryPath)
}

func dependencyIdentity(value Dependency) string {
	parts := []string{
		value.Language, string(value.Kind), value.ModulePath, value.ModuleVersion,
		value.PackagePath, value.RepositoryPath,
	}
	if value.Replacement != nil {
		parts = append(parts, value.Replacement.ModulePath, value.Replacement.ModuleVersion,
			fmt.Sprintf("%t", value.Replacement.Local), value.Replacement.RepositoryPath)
	}
	return stableID("dependency", parts...)
}

func stableID(prefix string, values ...string) string {
	hash := sha256.New()
	for _, value := range append([]string{"dependencies-v1", prefix}, values...) {
		_, _ = fmt.Fprintf(hash, "%d:", len(value))
		_, _ = hash.Write([]byte(value))
	}
	return prefix + "-" + hex.EncodeToString(hash.Sum(nil)[:12])
}

func canonicalStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	write := 0
	for _, value := range result {
		if write > 0 && result[write-1] == value {
			continue
		}
		result[write] = value
		write++
	}
	return result[:write]
}

func canonicalOmissions(values []Omission) []Omission {
	result := append([]Omission(nil), values...)
	sort.Slice(result, func(i, j int) bool { return omissionLess(result[i], result[j]) })
	write := 0
	for _, value := range result {
		if write > 0 && result[write-1] == value {
			continue
		}
		result[write] = value
		write++
	}
	return result[:write]
}

func omissionLess(left, right Omission) bool {
	if left.ImporterPackagePath != right.ImporterPackagePath {
		return left.ImporterPackagePath < right.ImporterPackagePath
	}
	if left.PackagePath != right.PackagePath {
		return left.PackagePath < right.PackagePath
	}
	if left.Reason != right.Reason {
		return left.Reason < right.Reason
	}
	return left.ImporterRef < right.ImporterRef
}

func sameDependencyIdentity(left, right Dependency) bool {
	return left.ID == right.ID && left.Language == right.Language && left.Kind == right.Kind &&
		left.Name == right.Name && left.ModulePath == right.ModulePath && left.ModuleVersion == right.ModuleVersion &&
		left.PackagePath == right.PackagePath && left.RepositoryPath == right.RepositoryPath &&
		sameReplacement(left.Replacement, right.Replacement)
}

func sameReplacement(left, right *Replacement) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func snapshotDependency(value Dependency) Dependency {
	result := value
	result.ImporterRefs = append([]string(nil), value.ImporterRefs...)
	if value.Replacement != nil {
		copyReplacement := *value.Replacement
		result.Replacement = &copyReplacement
	}
	return result
}

func sortCatalog(catalog *Catalog) {
	sort.Slice(catalog.Importers, func(i, j int) bool {
		return importerLess(catalog.Importers[i], catalog.Importers[j])
	})
	sort.Slice(catalog.Dependencies, func(i, j int) bool {
		return dependencyLess(catalog.Dependencies[i], catalog.Dependencies[j])
	})
}

func importerLess(left, right Importer) bool {
	if left.RepositoryPath != right.RepositoryPath {
		return left.RepositoryPath < right.RepositoryPath
	}
	if left.PackagePath != right.PackagePath {
		return left.PackagePath < right.PackagePath
	}
	return left.Ref < right.Ref
}

func dependencyLess(left, right Dependency) bool {
	if left.Kind != right.Kind {
		return kindRank(left.Kind) < kindRank(right.Kind)
	}
	if left.PackagePath != right.PackagePath {
		return left.PackagePath < right.PackagePath
	}
	if left.ModulePath != right.ModulePath {
		return left.ModulePath < right.ModulePath
	}
	return left.ID < right.ID
}

func kindRank(kind Kind) int {
	switch kind {
	case KindWorkspace:
		return 0
	case KindStdlib:
		return 1
	case KindExternal:
		return 2
	default:
		return 3
	}
}
