package facts

import (
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

// addDependencies lists the external packages of a target from its catalog,
// anchored to the first import that names the package when the index has it.
func (b *builder) addDependencies(target *targetContext) {
	if target.input.Dependencies == nil {
		return
	}
	for _, dependency := range target.input.Dependencies.Dependencies {
		if dependency.Kind != dependencies.KindExternal || dependency.Name == "" {
			continue
		}
		anchor := target.firstImport(dependency.PackagePath)
		b.add(target.root, Fact{
			Kind:     KindDependency,
			TargetID: target.target.ID,
			Anchor:   anchor,
			Key:      dependency.Name,
			Value:    dependency.ModuleVersion,
		}, dependency.Name)
	}
}

// firstImport finds the earliest import relation whose external target lives
// in the package, ordered by file and line.
func (target *targetContext) firstImport(packagePath string) *Anchor {
	var best *Anchor
	for _, relation := range target.input.Index.Relations {
		if relation.Kind != programindex.RelationImports {
			continue
		}
		if !target.importsPackage(relation, packagePath) {
			continue
		}
		location := relation.Location
		if location == nil {
			location = target.location(relation.FromID)
		}
		if location == nil {
			continue
		}
		candidate := &Anchor{Path: location.Path, Line: location.Line}
		if best == nil || candidate.Path < best.Path || candidate.Path == best.Path && candidate.Line < best.Line {
			best = candidate
		}
	}
	return best
}

func (target *targetContext) importsPackage(relation programindex.Relation, packagePath string) bool {
	for _, id := range relation.ToIDs {
		object, ok := target.object(id)
		if !ok || object.Kind != programindex.ObjectExternalSymbol || object.External == nil {
			continue
		}
		if _, matches := packageMatches(object.External.PackagePath, packagePath); matches {
			return true
		}
	}
	return false
}
