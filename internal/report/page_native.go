package report

import (
	"iter"
	"slices"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

type nativeReadError struct{ err error }

// These are the existing reader lookup tables, gathered together while each
// complete original index is decoded. No native bodies survive the visit.
type pageNativeCatalogue struct {
	ready    bool
	ends     map[string]int
	packages map[string][]string
	runBy    *runByOthers
	tests    map[string]bool
}

func (builder *pageBuilder) nativeCatalogue() *pageNativeCatalogue {
	if builder.nativeCatalog == nil {
		builder.nativeCatalog = &pageNativeCatalogue{}
	}
	catalogue := builder.nativeCatalog
	if catalogue.ready {
		return catalogue
	}
	catalogue.ready = true
	catalogue.ends = map[string]int{}
	catalogue.packages = map[string][]string{}
	catalogue.tests = map[string]bool{}
	catalogue.runBy = &runByOthers{programs: map[string][]string{}, keys: map[string]string{}}
	for index := range builder.nativeEntries() {
		for _, object := range index.Objects {
			if object.Location != nil && object.EndLine > object.Location.Line {
				catalogue.ends[placeKey(*object.Location)] = object.EndLine
			}
			if object.Kind == programindex.ObjectPackage || object.Kind == programindex.ObjectModule {
				catalogue.packages[index.Target.ID] = append(catalogue.packages[index.Target.ID], object.Name)
			}
		}
		for path := range groupindex.TestPaths(nil, []programindex.Index{index}) {
			catalogue.tests[path] = true
		}
		if !slices.ContainsFunc(index.Objects, func(object programindex.Object) bool { return object.Unreachable }) {
			continue
		}
		for _, object := range index.Objects {
			key := groupindex.DeclarationKey(object)
			if key == "" || !object.Kind.Callable() {
				continue
			}
			if object.Unreachable {
				catalogue.runBy.keys[subjectKey(index.Target.ID, object.ID)] = key
				continue
			}
			if !slices.Contains(catalogue.runBy.programs[key], index.Target.ID) {
				catalogue.runBy.programs[key] = append(catalogue.runBy.programs[key], index.Target.ID)
			}
		}
	}
	return catalogue
}

func (builder *pageBuilder) nativeFailure() *nativeReadError {
	if builder.nativeErrors == nil {
		builder.nativeErrors = &nativeReadError{}
	}
	return builder.nativeErrors
}

func (builder *pageBuilder) activateNativeScope(targetID string) {
	if builder.nativeScope == targetID {
		return
	}
	builder.nativeScope = targetID
	builder.nativeIndex = nil
	builder.ownUseReader = nil
	builder.callsFrom, builder.namesOf, builder.ownersOf = nil, nil, nil
	builder.typesOf, builder.macros, builder.arities = nil, nil, nil
	builder.fieldsByTarget, builder.dataByTarget = nil, nil
}

// nativeTargets keeps only the current target's complete native body. Switching
// target also releases caches that retain its native relations/source values;
// the ordinary page values and exact cross-target join keys remain unchanged.
func (builder *pageBuilder) nativeTargets(targetID string) []programindex.Index {
	if builder.nativeFailure().err != nil || builder.data == nil || builder.data.ProgramPortfolio == nil {
		return nil
	}
	builder.activateNativeScope(targetID)
	if builder.nativeIndex != nil && builder.nativeIndex.Target.ID == targetID {
		return []programindex.Index{*builder.nativeIndex}
	}
	index, found, err := builder.data.ProgramPortfolio.readTarget(targetID)
	if err != nil {
		builder.nativeFailure().err = err
		return nil
	}
	if !found {
		return nil
	}
	builder.nativeIndex = &index
	return []programindex.Index{index}
}

func (builder *pageBuilder) nativeEntries() iter.Seq[programindex.Index] {
	return func(yield func(programindex.Index) bool) {
		if builder.nativeFailure().err != nil || builder.data == nil || builder.data.ProgramPortfolio == nil {
			return
		}
		for index, err := range builder.data.ProgramPortfolio.indexes() {
			if err != nil {
				builder.nativeFailure().err = err
				return
			}
			if !yield(index) {
				return
			}
		}
	}
}
