package facts

import (
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// reachabilityKinds are the relation kinds along which execution or loading
// can move from one file to another.
var reachabilityKinds = map[programindex.RelationKind]struct{}{
	programindex.RelationImports:        {},
	programindex.RelationCalls:          {},
	programindex.RelationContains:       {},
	programindex.RelationDecorates:      {},
	programindex.RelationPassesCallback: {},
	programindex.RelationExecutes:       {},
	programindex.RelationSources:        {},
	programindex.RelationReads:          {},
	programindex.RelationWrites:         {},
}

// addReachability emits file-level import facts and marks every target file
// that no entrypoint seed can reach as dead.
func (b *builder) addReachability(target *targetContext) {
	edges := make(map[string]map[string]struct{})
	for _, relation := range target.input.Index.Relations {
		if _, ok := reachabilityKinds[relation.Kind]; !ok {
			continue
		}
		from := target.filePath(relation.FromID)
		if from == "" {
			continue
		}
		for _, id := range relation.ToIDs {
			to := target.filePath(id)
			if to == "" || to == from {
				continue
			}
			if edges[from] == nil {
				edges[from] = make(map[string]struct{})
			}
			edges[from][to] = struct{}{}
			if relation.Kind == programindex.RelationImports {
				b.addImport(target, relation, to)
			}
		}
	}
	addPackageInitEdges(target.files(), edges)
	roots := seedFiles(target)
	if len(roots) == 0 {
		b.diagnose("dead_module_skipped", target.target.Name+": no entrypoint seeds")
		return
	}
	reached := reach(roots, edges)
	for _, filePath := range target.files() {
		if _, ok := reached[filePath]; ok || isDeclarationFile(filePath) {
			continue
		}
		b.add(target.root, Fact{
			Kind:     KindDeadModule,
			TargetID: target.target.ID,
			Anchor:   &Anchor{Path: filePath, Line: 1},
			Path:     filePath,
		}, filePath)
	}
}

func (b *builder) addImport(target *targetContext, relation programindex.Relation, imported string) {
	location := relation.Location
	if location == nil {
		location = target.location(relation.FromID)
	}
	if location == nil {
		return
	}
	anchor := Anchor{Path: location.Path, Line: location.Line}
	if !b.once(strings.Join([]string{string(KindImport), anchor.String(), imported}, "\x00")) {
		return
	}
	b.add(target.root, Fact{
		Kind:     KindImport,
		TargetID: target.target.ID,
		Anchor:   &anchor,
		Path:     imported,
	}, imported)
}

func seedFiles(target *targetContext) []string {
	set := make(map[string]struct{})
	for _, seed := range target.input.Index.Target.Seeds {
		if seed.Location != nil {
			set[seed.Location.Path] = struct{}{}
		} else if filePath := target.filePath(seed.ObjectID); filePath != "" {
			set[filePath] = struct{}{}
		}
	}
	result := make([]string, 0, len(set))
	for filePath := range set {
		result = append(result, filePath)
	}
	sort.Strings(result)
	return result
}

func reach(roots []string, edges map[string]map[string]struct{}) map[string]struct{} {
	reached := make(map[string]struct{}, len(roots))
	queue := append([]string(nil), roots...)
	for _, root := range roots {
		reached[root] = struct{}{}
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for next := range edges[current] {
			if _, seen := reached[next]; !seen {
				reached[next] = struct{}{}
				queue = append(queue, next)
			}
		}
	}
	return reached
}

// addPackageInitEdges records what Python does when a module is imported:
// importing `pkg.sub.module` executes `pkg/__init__.py` and
// `pkg/sub/__init__.py` first. Without those edges a package's `__init__.py`
// is reachable only when something imports the package by name, and
// python-dotenv's was reported as dead code while `python -m dotenv` could
// not run without it.
func addPackageInitEdges(targetFiles []string, edges map[string]map[string]struct{}) {
	files := make(map[string]struct{}, len(targetFiles))
	for _, filePath := range targetFiles {
		files[filePath] = struct{}{}
	}
	for filePath := range files {
		if !strings.HasSuffix(filePath, ".py") {
			continue
		}
		for directory := path.Dir(filePath); directory != "." && directory != "/" && directory != ""; directory = path.Dir(directory) {
			initPath := path.Join(directory, "__init__.py")
			if initPath == filePath {
				continue
			}
			if _, present := files[initPath]; !present {
				// A directory without __init__.py is not a package, and
				// nothing above it is either.
				break
			}
			if edges[filePath] == nil {
				edges[filePath] = make(map[string]struct{})
			}
			edges[filePath][initPath] = struct{}{}
		}
	}
}

// isDeclarationFile excludes TypeScript ambient declarations: they are never
// imported at runtime, so unreachability says nothing about them.
func isDeclarationFile(filePath string) bool {
	return strings.HasSuffix(filePath, ".d.ts")
}
