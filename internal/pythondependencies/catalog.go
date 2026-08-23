// Package pythondependencies projects exact Python ProgramIndex import facts
// into the shared language-neutral dependency catalog.
package pythondependencies

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

const (
	WitnessStdlibImport       = "python_stdlib_import"
	WitnessStdlibFromImport   = "python_stdlib_from_import"
	WitnessExternalImport     = "python_external_import"
	WitnessExternalFromImport = "python_external_from_import"
)

// Build derives one complete target-scoped catalog. It accepts only exact
// import targets whose stdlib/external kind was established by the isolated
// Python adapter. Dynamic, wildcard, ambiguous, or otherwise unresolved
// imports remain typed omissions, so callers cannot mistake them for absence.
func Build(index programindex.Index) (dependencies.Catalog, error) {
	if err := index.Validate(); err != nil {
		return dependencies.Catalog{}, fmt.Errorf("Python dependencies: program index: %w", err)
	}
	if index.Target.Language != "python" {
		return dependencies.Catalog{}, fmt.Errorf("Python dependencies: program target language is %q", index.Target.Language)
	}
	if index.Coverage.RelationsOmitted != 0 || index.Coverage.ObjectsOmitted != 0 ||
		index.Coverage.WitnessesOmitted != 0 {
		return dependencies.Catalog{}, fmt.Errorf(
			"Python dependencies: ProgramIndex coverage is incomplete (%d objects, %d relations, and %d witnesses omitted)",
			index.Coverage.ObjectsOmitted, index.Coverage.RelationsOmitted,
			index.Coverage.WitnessesOmitted,
		)
	}

	objects := make(map[string]programindex.Object, len(index.Objects))
	for _, object := range index.Objects {
		objects[object.ID] = object
	}
	resolver := objectResolver{objects: objects}
	importersByKey := make(map[string]dependencies.Importer)
	uses := make([]dependencyUse, 0)
	omissions := make([]dependencies.Omission, 0)
	for _, relation := range index.Relations {
		if relation.Kind != programindex.RelationImports {
			continue
		}
		// A variable-driven importlib.import_module call is an exact dynamic
		// handoff fact, but it does not name a direct dependency. The
		// ProgramIndex keeps that unresolved relation for mechanism and path
		// analysis; projecting its callable name as a missing package would
		// incorrectly make every plugin registry look like incomplete package
		// authority.
		if dynamicImportFrontier(relation) {
			continue
		}
		importerObject, importerErr := resolver.moduleFor(relation.FromID)
		if importerErr != nil {
			omissions = append(omissions, dependencies.Omission{
				PackagePath: omissionPackagePath(relation),
				Reason:      dependencies.OmissionImporterIdentityUnavailable,
			})
			continue
		}
		importer, err := importerFromObject(importerObject)
		if err != nil {
			return dependencies.Catalog{}, err
		}
		importerKey := importerIdentityKey(importer)
		importersByKey[importerKey] = importer

		if relation.Resolution != programindex.ResolutionExact || len(relation.ToIDs) == 0 || relation.TargetsOmitted != 0 {
			omissions = append(omissions, dependencies.Omission{
				ImporterPackagePath: importer.PackagePath,
				PackagePath:         omissionPackagePath(relation),
				Reason:              dependencies.OmissionDependencyIdentityMissing,
			})
			continue
		}
		for _, targetID := range relation.ToIDs {
			target, ok := objects[targetID]
			if !ok {
				return dependencies.Catalog{}, fmt.Errorf("Python dependencies: import relation cites unknown target %q", targetID)
			}
			value, err := dependencyFromImport(relation, target, resolver)
			if err != nil {
				omissions = append(omissions, dependencies.Omission{
					ImporterPackagePath: importer.PackagePath,
					PackagePath:         omissionPackagePath(relation),
					Reason:              dependencies.OmissionDependencyMetadataMissing,
				})
				continue
			}
			uses = append(uses, dependencyUse{importerKey: importerKey, value: value})
		}
	}

	importerKeys := make([]string, 0, len(importersByKey))
	for key := range importersByKey {
		importerKeys = append(importerKeys, key)
	}
	sort.Strings(importerKeys)
	importers := make([]dependencies.Importer, 0, len(importerKeys))
	refsByKey := make(map[string]string, len(importerKeys))
	for _, key := range importerKeys {
		sealed, err := dependencies.SealImporter(importersByKey[key])
		if err != nil {
			return dependencies.Catalog{}, fmt.Errorf("Python dependencies: importer: %w", err)
		}
		importers = append(importers, sealed)
		refsByKey[key] = sealed.Ref
	}
	for position := range uses {
		uses[position].value.ImporterRefs = []string{refsByKey[uses[position].importerKey]}
	}
	for position := range omissions {
		if omissions[position].ImporterPackagePath == "" {
			continue
		}
		for _, importer := range importers {
			if importer.PackagePath == omissions[position].ImporterPackagePath {
				omissions[position].ImporterRef = importer.Ref
				break
			}
		}
		if omissions[position].ImporterRef == "" {
			return dependencies.Catalog{}, fmt.Errorf(
				"Python dependencies: omission importer %q has no exact ref",
				omissions[position].ImporterPackagePath,
			)
		}
	}
	values := make([]dependencies.Dependency, len(uses))
	for position, use := range uses {
		values[position] = use.value
	}
	catalog, err := dependencies.BuildWithOmissions(importers, values, omissions)
	if err != nil {
		return dependencies.Catalog{}, fmt.Errorf("Python dependencies: seal catalog: %w", err)
	}
	return catalog, nil
}

func dynamicImportFrontier(relation programindex.Relation) bool {
	if relation.Resolution != programindex.ResolutionUnresolved ||
		len(relation.ToIDs) != 0 ||
		len(relation.Witnesses) != 1 || relation.WitnessesObserved != 1 {
		return false
	}
	witness := relation.Witnesses[0]
	return witness.Kind == "dynamic_import" && witness.Detail == "importlib.import_module"
}

type dependencyUse struct {
	importerKey string
	value       dependencies.Dependency
}

type objectResolver struct {
	objects map[string]programindex.Object
}

func (resolver objectResolver) moduleFor(objectID string) (programindex.Object, error) {
	seen := make(map[string]struct{})
	for objectID != "" {
		if _, duplicate := seen[objectID]; duplicate {
			return programindex.Object{}, fmt.Errorf("Python dependencies: object containment contains a cycle")
		}
		seen[objectID] = struct{}{}
		object, ok := resolver.objects[objectID]
		if !ok {
			return programindex.Object{}, fmt.Errorf("Python dependencies: unknown object %q", objectID)
		}
		if object.Kind == programindex.ObjectModule || object.Kind == programindex.ObjectPackage {
			return object, nil
		}
		if object.ContainerID != "" {
			objectID = object.ContainerID
		} else {
			objectID = object.OwnerID
		}
	}
	return programindex.Object{}, fmt.Errorf("Python dependencies: object has no exact module container")
}

func importerFromObject(object programindex.Object) (dependencies.Importer, error) {
	if object.Location == nil {
		return dependencies.Importer{}, fmt.Errorf("Python dependencies: importer module %q has no source location", object.Name)
	}
	repositoryPath := path.Dir(object.Location.Path)
	modulePath := firstPythonNamePart(object.Name)
	if modulePath == "" {
		return dependencies.Importer{}, fmt.Errorf("Python dependencies: importer module has no exact name")
	}
	return dependencies.Importer{
		Language: "python", Name: object.Name, ModulePath: modulePath,
		PackagePath: object.Name, RepositoryPath: repositoryPath,
	}, nil
}

func dependencyFromImport(
	relation programindex.Relation,
	target programindex.Object,
	resolver objectResolver,
) (dependencies.Dependency, error) {
	if target.Kind != programindex.ObjectExternalSymbol {
		module, err := resolver.moduleFor(target.ID)
		if err != nil || module.Location == nil {
			return dependencies.Dependency{}, fmt.Errorf("workspace import target has no exact module")
		}
		return dependencies.Dependency{
			Language: "python", Kind: dependencies.KindWorkspace, Name: module.Name,
			ModulePath: firstPythonNamePart(module.Name), PackagePath: module.Name,
			RepositoryPath: path.Dir(module.Location.Path),
		}, nil
	}

	kind, packagePath, ok := externalImportAuthority(relation)
	if !ok {
		return dependencies.Dependency{}, fmt.Errorf("external import has no adapter-owned kind")
	}
	value := dependencies.Dependency{
		Language: "python", Kind: kind, Name: packagePath, PackagePath: packagePath,
	}
	if kind == dependencies.KindExternal {
		value.ModulePath = firstPythonNamePart(packagePath)
	}
	return value, nil
}

func externalImportAuthority(relation programindex.Relation) (dependencies.Kind, string, bool) {
	var selectedKind dependencies.Kind
	selectedPath := ""
	for _, witness := range relation.Witnesses {
		kind := dependencies.Kind("")
		fromImport := false
		switch witness.Kind {
		case WitnessStdlibImport:
			kind = dependencies.KindStdlib
		case WitnessStdlibFromImport:
			kind, fromImport = dependencies.KindStdlib, true
		case WitnessExternalImport:
			kind = dependencies.KindExternal
		case WitnessExternalFromImport:
			kind, fromImport = dependencies.KindExternal, true
		default:
			continue
		}
		packagePath := canonicalPythonImport(witness.Detail, fromImport)
		if packagePath == "" || selectedKind != "" && (selectedKind != kind || selectedPath != packagePath) {
			return "", "", false
		}
		selectedKind, selectedPath = kind, packagePath
	}
	return selectedKind, selectedPath, selectedKind != "" && selectedPath != ""
}

func canonicalPythonImport(value string, fromImport bool) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return ""
	}
	parts := strings.Split(value, ".")
	if fromImport {
		if len(parts) < 2 {
			return ""
		}
		parts = parts[:len(parts)-1]
	}
	for _, part := range parts {
		if part == "" {
			return ""
		}
		for position, character := range part {
			if character == '_' || character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
				position > 0 && character >= '0' && character <= '9' {
				continue
			}
			return ""
		}
	}
	return strings.Join(parts, ".")
}

func omissionPackagePath(relation programindex.Relation) string {
	for _, witness := range relation.Witnesses {
		if value := strings.TrimSpace(witness.Detail); value != "" && !strings.ContainsAny(value, "\x00\r\n") {
			return value
		}
	}
	return "unresolved_import"
}

func firstPythonNamePart(value string) string {
	if position := strings.IndexByte(value, '.'); position >= 0 {
		return value[:position]
	}
	return value
}

func importerIdentityKey(value dependencies.Importer) string {
	return value.Language + "\x00" + value.ModulePath + "\x00" + value.PackagePath + "\x00" + value.RepositoryPath
}
