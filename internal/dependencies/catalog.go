// Package dependencies owns the language-neutral repository dependency model.
package dependencies

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const Version = 2

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
// dependency. Ref is its compact catalog-local identity.
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
// dependencies. ID is its compact catalog-local identity.
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
	// EffectImporterRefs are the importers that import the package for the
	// effect of importing it (Go's `import _ "github.com/go-sql-driver/mysql"`
	// in one of the importer's files, whatever else they use of it): a driver
	// or a plugin registering itself. A subset of ImporterRefs; an adapter
	// that cannot tell leaves it empty.
	EffectImporterRefs []string `json:"effect_importer_refs,omitempty"`
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
	importerAlias := make(map[string]string, len(importers)*2)
	for _, importer := range importers {
		inputRef := importer.Ref
		var err error
		importer, err = SealImporter(importer)
		if err != nil {
			return Catalog{}, err
		}
		if previous, exists := importerByRef[importer.Ref]; exists && previous != importer {
			return Catalog{}, fmt.Errorf("dependencies: conflicting importer identity %q", importer.Ref)
		}
		importerByRef[importer.Ref] = importer
		for _, alias := range []string{inputRef, importer.Ref} {
			if alias == "" {
				continue
			}
			if previous, exists := importerAlias[alias]; exists && previous != importer.Ref {
				return Catalog{}, fmt.Errorf("dependencies: conflicting importer ref %q", alias)
			}
			importerAlias[alias] = importer.Ref
		}
	}
	orderedImporters := make([]Importer, 0, len(importerByRef))
	for _, importer := range importerByRef {
		orderedImporters = append(orderedImporters, importer)
	}
	sort.Slice(orderedImporters, func(i, j int) bool { return importerLess(orderedImporters[i], orderedImporters[j]) })
	compactImporterRef := make(map[string]string, len(orderedImporters))
	for position := range orderedImporters {
		identity := orderedImporters[position].Ref
		compact := "i" + strconv.Itoa(position+1)
		orderedImporters[position].Ref = compact
		compactImporterRef[identity] = compact
	}

	dependencyByID := make(map[string]Dependency, len(values))
	for _, value := range values {
		for position, ref := range value.ImporterRefs {
			identity, ok := importerAlias[ref]
			if !ok {
				return Catalog{}, fmt.Errorf("dependencies: dependency has unknown importer ref %q", ref)
			}
			value.ImporterRefs[position] = identity
		}
		value.ImporterRefs = canonicalStrings(value.ImporterRefs)
		for position, ref := range value.EffectImporterRefs {
			identity, ok := importerAlias[ref]
			if !ok {
				return Catalog{}, fmt.Errorf("dependencies: dependency has unknown effect importer ref %q", ref)
			}
			value.EffectImporterRefs[position] = identity
		}
		value.EffectImporterRefs = canonicalStrings(value.EffectImporterRefs)
		value.ID = dependencyIdentity(value)
		if err := validateDependencyShape(value); err != nil {
			return Catalog{}, err
		}
		if previous, exists := dependencyByID[value.ID]; exists {
			if !sameDependencyIdentity(previous, value) {
				return Catalog{}, fmt.Errorf("dependencies: conflicting dependency identity %q", value.ID)
			}
			previous.ImporterRefs = canonicalStrings(append(previous.ImporterRefs, value.ImporterRefs...))
			if len(value.EffectImporterRefs) > 0 {
				previous.EffectImporterRefs = canonicalStrings(append(previous.EffectImporterRefs, value.EffectImporterRefs...))
			}
			dependencyByID[value.ID] = previous
			continue
		}
		dependencyByID[value.ID] = snapshotDependency(value)
	}

	catalog := Catalog{
		Version:      Version,
		Importers:    orderedImporters,
		Dependencies: make([]Dependency, 0, len(dependencyByID)),
		Coverage: Coverage{
			State:     CoverageComplete,
			Omissions: canonicalOmissions(omissions),
		},
	}
	for _, value := range dependencyByID {
		for position, ref := range value.ImporterRefs {
			value.ImporterRefs[position] = compactImporterRef[ref]
		}
		value.ImporterRefs = canonicalCompactIDs(value.ImporterRefs, "i")
		for position, ref := range value.EffectImporterRefs {
			value.EffectImporterRefs[position] = compactImporterRef[ref]
		}
		if len(value.EffectImporterRefs) > 0 {
			value.EffectImporterRefs = canonicalCompactIDs(value.EffectImporterRefs, "i")
		}
		catalog.Dependencies = append(catalog.Dependencies, value)
	}
	sort.Slice(catalog.Dependencies, func(i, j int) bool { return dependencyLess(catalog.Dependencies[i], catalog.Dependencies[j]) })
	for position := range catalog.Dependencies {
		catalog.Dependencies[position].ID = "d" + strconv.Itoa(position+1)
		catalog.Coverage.ImportsRetained += len(catalog.Dependencies[position].ImporterRefs)
	}
	for position := range catalog.Coverage.Omissions {
		ref := catalog.Coverage.Omissions[position].ImporterRef
		if ref == "" {
			continue
		}
		identity, ok := importerAlias[ref]
		if !ok {
			return Catalog{}, fmt.Errorf("dependencies: coverage omission has unknown importer ref %q", ref)
		}
		catalog.Coverage.Omissions[position].ImporterRef = compactImporterRef[identity]
	}
	catalog.Coverage.Omissions = canonicalOmissions(catalog.Coverage.Omissions)
	catalog.Coverage.ImportsObserved = catalog.Coverage.ImportsRetained + len(catalog.Coverage.Omissions)
	if len(catalog.Coverage.Omissions) > 0 {
		catalog.Coverage.State = CoveragePartial
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

// SealImporter supplies a construction-only identity so language adapters can
// join dependency uses before the complete catalog assigns i* ordinals. The
// value never survives BuildWithOmissions.
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

// Validate verifies canonical order, compact identity bindings and exact
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
		if importer.Ref != "i"+strconv.Itoa(index+1) {
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
		if value.ID != "d"+strconv.Itoa(index+1) {
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
			if !compactID(ref, "i") || refIndex > 0 && !compactIDLess(value.ImporterRefs[refIndex-1], ref, "i") {
				return fmt.Errorf("dependencies: importer refs are not canonical")
			}
		}
		for refIndex, ref := range value.EffectImporterRefs {
			if !slices.Contains(value.ImporterRefs, ref) || refIndex > 0 && !compactIDLess(value.EffectImporterRefs[refIndex-1], ref, "i") {
				return fmt.Errorf("dependencies: dependency %q has an effect importer ref %q that is no canonical importer of it", value.ID, ref)
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
// least one of them, then seals a new self-contained catalog. Its i*/d*
// ordinals are local to that result; semantic joins use the typed fields.
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
		copyValue.EffectImporterRefs = nil
		for _, ref := range value.EffectImporterRefs {
			if _, ok := kept[ref]; ok {
				copyValue.EffectImporterRefs = append(copyValue.EffectImporterRefs, ref)
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

func canonicalCompactIDs(values []string, prefix string) []string {
	result := append([]string(nil), values...)
	sort.Slice(result, func(i, j int) bool { return compactIDLess(result[i], result[j], prefix) })
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

func compactID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	ordinal, err := strconv.Atoi(value[len(prefix):])
	return err == nil && ordinal > 0 && prefix+strconv.Itoa(ordinal) == value
}

func compactIDLess(left, right, prefix string) bool {
	leftOrdinal, leftErr := strconv.Atoi(strings.TrimPrefix(left, prefix))
	rightOrdinal, rightErr := strconv.Atoi(strings.TrimPrefix(right, prefix))
	if leftErr == nil && rightErr == nil && leftOrdinal != rightOrdinal {
		return leftOrdinal < rightOrdinal
	}
	return left < right
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
	if left.ImporterRef != right.ImporterRef {
		return compactIDLess(left.ImporterRef, right.ImporterRef, "i")
	}
	return false
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
	if len(value.EffectImporterRefs) > 0 {
		result.EffectImporterRefs = append([]string(nil), value.EffectImporterRefs...)
	}
	if value.Replacement != nil {
		copyReplacement := *value.Replacement
		result.Replacement = &copyReplacement
	}
	return result
}

func importerLess(left, right Importer) bool {
	if left.RepositoryPath != right.RepositoryPath {
		return left.RepositoryPath < right.RepositoryPath
	}
	if left.PackagePath != right.PackagePath {
		return left.PackagePath < right.PackagePath
	}
	if left.Language != right.Language {
		return left.Language < right.Language
	}
	if left.ModulePath != right.ModulePath {
		return left.ModulePath < right.ModulePath
	}
	if left.Name != right.Name {
		return left.Name < right.Name
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
	if left.ModuleVersion != right.ModuleVersion {
		return left.ModuleVersion < right.ModuleVersion
	}
	if left.RepositoryPath != right.RepositoryPath {
		return left.RepositoryPath < right.RepositoryPath
	}
	if left.Language != right.Language {
		return left.Language < right.Language
	}
	if left.Name != right.Name {
		return left.Name < right.Name
	}
	leftReplacement, rightReplacement := replacementOrderKey(left.Replacement), replacementOrderKey(right.Replacement)
	if leftReplacement != rightReplacement {
		return leftReplacement < rightReplacement
	}
	return left.ID < right.ID
}

func replacementOrderKey(value *Replacement) string {
	if value == nil {
		return ""
	}
	return strings.Join([]string{value.ModulePath, value.ModuleVersion, strconv.FormatBool(value.Local), value.RepositoryPath}, "\x00")
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
