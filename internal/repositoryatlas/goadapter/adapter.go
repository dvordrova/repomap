// Package goadapter projects exact local Go facts into the language-neutral
// Repository Atlas contract. It does not read source, run analyzers, or infer
// missing ownership and reachability.
package goadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/token"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/surfacediscovery"
)

const (
	PackageDeclarationEvidenceProvider  = "gofacts"
	PackageDeclarationEvidenceVersion   = "package-declaration-v1"
	PackageDeclarationEvidenceOperation = "package_declaration"
)

type Input struct {
	RepositoryName      string
	Facts               gofacts.Facts
	Catalog             surfacediscovery.TriggerCatalog
	PackageDeclarations map[string]evidence.Location
}

type exactEntrypoint struct {
	entrypoint gofacts.Entrypoint
	anchor     gofacts.EntrypointAnchor
	module     gofacts.ModuleFact
	appUnitID  string
}

func Project(input Input) (repositoryatlas.Atlas, error) {
	if strings.TrimSpace(input.RepositoryName) == "" {
		return repositoryatlas.Atlas{}, fmt.Errorf("repository atlas Go adapter: repository name is required")
	}
	repositoryID := stableID("unit-repository", input.RepositoryName)
	atlas := repositoryatlas.Atlas{
		Version:      repositoryatlas.Version,
		Units:        []repositoryatlas.Unit{{ID: repositoryID, Kind: repositoryatlas.UnitRepository, Name: input.RepositoryName}},
		Entities:     []repositoryatlas.Entity{},
		Observations: []repositoryatlas.Observation{},
		Evidence:     []repositoryatlas.Evidence{},
		Relations:    []repositoryatlas.Relation{},
	}

	modules, err := projectModules(&atlas, repositoryID, input.Facts.Modules)
	if err != nil {
		return repositoryatlas.Atlas{}, err
	}
	if _, err := projectPackages(
		&atlas, input.Facts.Packages, modules, input.PackageDeclarations,
	); err != nil {
		return repositoryatlas.Atlas{}, err
	}
	entrypoints := projectEntrypoints(&atlas, input.Facts.EntrypointPackages, modules)
	projectProcessEntries(&atlas, input.Catalog.Triggers, entrypoints)

	canonical, err := repositoryatlas.Canonical(atlas)
	if err != nil {
		return repositoryatlas.Atlas{}, fmt.Errorf("repository atlas Go adapter: %w", err)
	}
	return canonical, nil
}

func projectModules(
	atlas *repositoryatlas.Atlas,
	repositoryID string,
	values []gofacts.ModuleFact,
) (map[string]gofacts.ModuleFact, error) {
	modules := make(map[string]gofacts.ModuleFact, len(values))
	for _, module := range values {
		if module.ID == "" || module.ModulePath == "" {
			return nil, fmt.Errorf("repository atlas Go adapter: module id and path are required")
		}
		if _, duplicate := modules[module.ID]; duplicate {
			return nil, fmt.Errorf("repository atlas Go adapter: duplicate module id %q", module.ID)
		}
		modules[module.ID] = module
		atlas.Units = append(atlas.Units, repositoryatlas.Unit{
			ID: module.ID, Kind: repositoryatlas.UnitModule,
			ParentID: repositoryID, Name: module.ModulePath,
		})
	}
	return modules, nil
}

func projectPackages(
	atlas *repositoryatlas.Atlas,
	values []gofacts.PackageFact,
	modules map[string]gofacts.ModuleFact,
	declarations map[string]evidence.Location,
) (map[string]gofacts.PackageFact, error) {
	validDeclarations := make(map[string]evidence.Location, len(declarations))
	declarationCounts := make(map[string]int, len(declarations))
	for _, pkg := range values {
		location, ok := declarations[pkg.CanonicalPath]
		if !ok || !exactPackageDeclarationLocation(pkg, location) {
			continue
		}
		validDeclarations[pkg.CanonicalPath] = location
		declarationCounts[packageDeclarationLocationKey(location)]++
	}
	packages := make(map[string]gofacts.PackageFact, len(values))
	for _, pkg := range values {
		if pkg.CanonicalPath == "" || pkg.ModuleID == "" {
			return nil, fmt.Errorf("repository atlas Go adapter: package path and module id are required")
		}
		if _, duplicate := packages[pkg.CanonicalPath]; duplicate {
			return nil, fmt.Errorf("repository atlas Go adapter: duplicate package path %q", pkg.CanonicalPath)
		}
		module, exists := modules[pkg.ModuleID]
		if !exists || module.ModulePath != pkg.ModulePath {
			return nil, fmt.Errorf("repository atlas Go adapter: package %q has inconsistent module ownership", pkg.CanonicalPath)
		}
		unitID := stableID("unit-package", pkg.ModuleID, pkg.CanonicalPath)
		packages[pkg.CanonicalPath] = pkg
		atlas.Units = append(atlas.Units, repositoryatlas.Unit{
			ID: unitID, Kind: repositoryatlas.UnitPackage,
			ParentID: pkg.ModuleID, Name: pkg.CanonicalPath,
		})
		location, hasDeclaration := validDeclarations[pkg.CanonicalPath]
		if hasDeclaration && declarationCounts[packageDeclarationLocationKey(location)] == 1 {
			atlas.Evidence = append(atlas.Evidence, packageDeclarationEvidence(unitID, pkg, location))
		}
	}
	return packages, nil
}

// IsExactPackageDeclarationEvidence recognizes only the adapter-owned package
// declaration fact. It is shared by report projections so presentation and
// Study filtering cannot infer package evidence from a path, basename, or
// package-shaped Unit alone.
func IsExactPackageDeclarationEvidence(
	item repositoryatlas.Evidence,
	unit repositoryatlas.Unit,
	pkg gofacts.PackageFact,
) bool {
	if !IsPersistedPackageDeclarationEvidence(item, unit) ||
		unit.Name != pkg.CanonicalPath || unit.ParentID != pkg.ModuleID ||
		item.Symbol != pkg.Name || !exactPackageDeclarationLocation(pkg, item.Location) {
		return false
	}
	return true
}

// IsPersistedPackageDeclarationEvidence verifies the self-contained canonical
// Atlas representation after producer-owned PackageFact.Files are no longer
// present. It does not infer package membership from a path; it recognizes the
// exact stable evidence record already emitted by this adapter.
func IsPersistedPackageDeclarationEvidence(
	item repositoryatlas.Evidence,
	unit repositoryatlas.Unit,
) bool {
	if unit.Kind != repositoryatlas.UnitPackage || unit.ID == "" || unit.Name == "" || unit.ParentID == "" ||
		unit.ID != stableID("unit-package", unit.ParentID, unit.Name) ||
		item.UnitID != unit.ID || !token.IsIdentifier(item.Symbol) ||
		item.Provenance.Provider != PackageDeclarationEvidenceProvider ||
		item.Provenance.Version != PackageDeclarationEvidenceVersion ||
		item.Provenance.Operation != PackageDeclarationEvidenceOperation ||
		item.Provenance.Detail != "" || item.Provenance.Location != nil ||
		!validPersistedPackageDeclarationLocation(item.Location) {
		return false
	}
	return item.ID == packageDeclarationEvidenceID(unit.ID, item.Symbol, item.Location)
}

func packageDeclarationEvidence(
	unitID string,
	pkg gofacts.PackageFact,
	location evidence.Location,
) repositoryatlas.Evidence {
	return repositoryatlas.Evidence{
		ID:     packageDeclarationEvidenceID(unitID, pkg.Name, location),
		UnitID: unitID, Location: location, Symbol: pkg.Name,
		Provenance: evidence.Provenance{
			Provider:  PackageDeclarationEvidenceProvider,
			Version:   PackageDeclarationEvidenceVersion,
			Operation: PackageDeclarationEvidenceOperation,
		},
	}
}

func packageDeclarationEvidenceID(
	unitID string,
	symbol string,
	location evidence.Location,
) string {
	return stableID(
		"evidence", "package-declaration", unitID, symbol,
		location.Path, strconv.Itoa(location.Line), strconv.Itoa(location.Column),
		PackageDeclarationEvidenceProvider, PackageDeclarationEvidenceVersion,
		PackageDeclarationEvidenceOperation,
	)
}

func validPersistedPackageDeclarationLocation(location evidence.Location) bool {
	cleaned := path.Clean(location.Path)
	return location.Path != "" && cleaned == location.Path && cleaned != "." &&
		!path.IsAbs(location.Path) && !strings.Contains(location.Path, "\\") &&
		!strings.ContainsAny(location.Path, "\x00\r\n") &&
		!strings.HasPrefix(cleaned, "../") && strings.HasSuffix(location.Path, ".go") &&
		location.Line > 0 && location.Column > 0 && location.EndLine == 0 && location.EndColumn == 0
}

func exactPackageDeclarationLocation(pkg gofacts.PackageFact, location evidence.Location) bool {
	if pkg.Name == "" || location.Path == "" || !strings.HasSuffix(location.Path, ".go") ||
		location.Line <= 0 || location.Column <= 0 || location.EndLine != 0 || location.EndColumn != 0 {
		return false
	}
	for _, sourcePath := range pkg.Files {
		if sourcePath == location.Path {
			return true
		}
	}
	return false
}

func packageDeclarationLocationKey(location evidence.Location) string {
	return strings.Join([]string{
		location.Path, strconv.Itoa(location.Line), strconv.Itoa(location.Column),
	}, "\x00")
}

func projectEntrypoints(
	atlas *repositoryatlas.Atlas,
	values []gofacts.Entrypoint,
	modules map[string]gofacts.ModuleFact,
) map[string]exactEntrypoint {
	modulesByOwnership := make(map[string][]gofacts.ModuleFact, len(modules))
	for _, module := range modules {
		key := module.ModulePath + "\x00" + module.ModuleDir
		modulesByOwnership[key] = append(modulesByOwnership[key], module)
	}
	candidates := make(map[string][]exactEntrypoint)
	for _, entrypoint := range values {
		matches := modulesByOwnership[entrypoint.ModulePath+"\x00"+entrypoint.ModuleDir]
		if len(matches) != 1 {
			continue
		}
		module := matches[0]
		for _, anchor := range entrypoint.Anchors {
			if anchor.Version != gofacts.EntrypointAnchorVersion ||
				anchor.Kind != gofacts.EntrypointAnchorGoMain || anchor.Path == "" || anchor.Line <= 0 {
				continue
			}
			candidate := exactEntrypoint{
				entrypoint: entrypoint, anchor: anchor, module: module,
				appUnitID: stableID("unit-app", module.ID, entrypoint.ImportPath),
			}
			key := entrypointKey(entrypoint.ImportPath, anchor.Path, anchor.Line)
			candidates[key] = append(candidates[key], candidate)
		}
	}

	result := make(map[string]exactEntrypoint, len(candidates))
	apps := make(map[string]exactEntrypoint)
	for key, matches := range candidates {
		if len(matches) != 1 {
			continue
		}
		result[key] = matches[0]
		apps[matches[0].appUnitID] = matches[0]
	}
	appIDs := make([]string, 0, len(apps))
	for appID := range apps {
		appIDs = append(appIDs, appID)
	}
	sort.Strings(appIDs)
	for _, appID := range appIDs {
		entrypoint := apps[appID]
		atlas.Units = append(atlas.Units, repositoryatlas.Unit{
			ID: appID, Kind: repositoryatlas.UnitApp,
			ParentID: entrypoint.module.ID, Name: entrypoint.entrypoint.ImportPath,
		})
	}
	return result
}

func projectProcessEntries(
	atlas *repositoryatlas.Atlas,
	values []surfacediscovery.TriggerRecord,
	entrypoints map[string]exactEntrypoint,
) {
	for _, record := range values {
		if record.Kind != "process_entry" || record.ID == "" || record.ProvisionalID ||
			record.Availability != surfacediscovery.AvailabilityAvailable ||
			record.Resolution != "exact" || record.Certainty != "static" {
			continue
		}
		location := record.ProcessEntrypoint.Location
		entrypoint, exists := entrypoints[entrypointKey(record.ProcessEntrypoint.Package, location.Path, location.Line)]
		if !exists || !exactProcessSymbol(record, entrypoint) {
			continue
		}
		sourceEvidence, provenance, ok := exactProcessEvidence(record)
		if !ok {
			continue
		}

		surfaceRef := repositoryatlas.EntityRef{Kind: repositoryatlas.EntitySurface, ID: record.ID}
		operationID := stableID("operation", "application-start", entrypoint.appUnitID, record.ID)
		operationRef := repositoryatlas.EntityRef{Kind: repositoryatlas.EntityOperation, ID: operationID}
		evidenceID := stableID(
			"evidence", entrypoint.appUnitID, sourceEvidence.ID,
			sourceEvidence.Location.Path, strconv.Itoa(sourceEvidence.Location.Line), strconv.Itoa(sourceEvidence.Location.Column),
		)

		atlas.Entities = append(atlas.Entities,
			repositoryatlas.Entity{ID: record.ID, Kind: repositoryatlas.EntitySurface, UnitID: entrypoint.appUnitID},
			repositoryatlas.Entity{ID: operationID, Kind: repositoryatlas.EntityOperation, UnitID: entrypoint.appUnitID},
		)
		atlas.Evidence = append(atlas.Evidence, repositoryatlas.Evidence{
			ID: evidenceID, UnitID: entrypoint.appUnitID,
			Location: evidence.Location{
				Path: sourceEvidence.Location.Path, Line: sourceEvidence.Location.Line,
				Column: sourceEvidence.Location.Column,
			},
			Symbol: record.ProcessEntrypoint.ID,
			Provenance: evidence.Provenance{
				Provider: provenance.Provider, Version: provenance.Version,
				Operation: provenance.Operation,
			},
		})
		atlas.Observations = append(atlas.Observations,
			repositoryatlas.Observation{
				ID: stableID("observation", record.ID, evidenceID), UnitID: entrypoint.appUnitID,
				Subject: surfaceRef, EvidenceRefs: []string{evidenceID},
			},
			repositoryatlas.Observation{
				ID: stableID("observation", operationID, evidenceID), UnitID: entrypoint.appUnitID,
				Subject: operationRef, EvidenceRefs: []string{evidenceID},
			},
		)
		atlas.Relations = append(atlas.Relations, repositoryatlas.Relation{
			ID:     stableID("relation", "exposes", record.ID, operationID),
			UnitID: entrypoint.appUnitID, Kind: repositoryatlas.RelationExposes,
			Source: surfaceRef, Target: operationRef,
			Phase: repositoryatlas.PhaseStartup, Authority: repositoryatlas.AuthorityResolved,
			EvidenceRefs: []string{evidenceID},
		})
	}
}

func exactProcessSymbol(record surfacediscovery.TriggerRecord, entrypoint exactEntrypoint) bool {
	wantID := entrypoint.entrypoint.ImportPath + ".main"
	return record.ProcessEntrypoint.ID == wantID &&
		record.ProcessEntrypoint.Package == entrypoint.entrypoint.ImportPath &&
		record.ProcessEntrypoint.Name == "main" &&
		record.Identity.Name == "main" &&
		record.RegistrationSite == record.ProcessEntrypoint.Location &&
		record.ProcessEntrypoint.Location.Path == entrypoint.anchor.Path &&
		record.ProcessEntrypoint.Location.Line == entrypoint.anchor.Line
}

func exactProcessEvidence(
	record surfacediscovery.TriggerRecord,
) (surfacediscovery.Evidence, surfacediscovery.Provenance, bool) {
	var matchedEvidence []surfacediscovery.Evidence
	for _, candidate := range record.Evidence {
		if candidate.Kind == "process_entry_declaration" && candidate.ID != "" &&
			candidate.Location == record.ProcessEntrypoint.Location {
			matchedEvidence = append(matchedEvidence, candidate)
		}
	}
	var matchedProvenance []surfacediscovery.Provenance
	for _, candidate := range record.Provenance {
		if candidate.Provider == "gofacts" && candidate.Version == "entrypoint-anchor-v1" &&
			candidate.Operation == "build_selected_main_declaration" {
			matchedProvenance = append(matchedProvenance, candidate)
		}
	}
	if len(matchedEvidence) != 1 || len(matchedProvenance) != 1 {
		return surfacediscovery.Evidence{}, surfacediscovery.Provenance{}, false
	}
	return matchedEvidence[0], matchedProvenance[0], true
}

func entrypointKey(packagePath, sourcePath string, line int) string {
	return strings.Join([]string{packagePath, sourcePath, strconv.Itoa(line)}, "\x00")
}

func stableID(prefix string, parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "-" + hex.EncodeToString(digest[:12])
}
