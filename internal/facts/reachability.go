package facts

import (
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/programindex"
)

// A file is dead when no entrypoint of any target reaches it through the
// relations the program graph records between files. A file that declares
// only types and values has no code to run and is never judged: the graph
// carries no edge for naming a type in a signature.

// addReachability emits this target's file-level import facts and records its
// file edges, seeds and judgeable files for the repository-wide verdict.
func (b *builder) addReachability(target *targetContext) {
	for _, relation := range target.input.Index.Relations {
		from := target.filePath(relation.FromID)
		if from == "" {
			continue
		}
		for _, id := range relation.ToIDs {
			to := target.filePath(id)
			if to == "" || to == from {
				continue
			}
			b.reach.edge(from, to)
			if relation.Kind == programindex.RelationImports {
				b.addImport(target, relation, to)
			}
		}
	}
	// Loading a container reaches the declarations it contains.
	for _, object := range target.input.Index.Objects {
		from, to := target.filePath(object.ContainerID), target.filePath(object.ID)
		if object.ContainerID != "" && from != "" && to != "" && to != from {
			b.reach.edge(from, to)
		}
		if isCallable(object) {
			b.reach.executable[target.filePath(object.ID)] = true
		}
	}
	addPackageInitEdges(target.files(), b.reach.edges)
	roots := seedFiles(target)
	if len(roots) == 0 {
		b.diagnose("dead_module_skipped", target.target.Name+" ("+target.root+"): no entrypoint seeds")
	}
	b.reach.roots = append(b.reach.roots, roots...)
	for _, filePath := range target.files() {
		if _, judged := b.reach.owner[filePath]; !judged {
			b.reach.owner[filePath] = target
		}
	}
}

type reachability struct {
	edges      map[string]map[string]struct{}
	roots      []string
	executable map[string]bool
	owner      map[string]*targetContext
}

func newReachability() *reachability {
	return &reachability{edges: make(map[string]map[string]struct{}), executable: make(map[string]bool), owner: make(map[string]*targetContext)}
}

func (r *reachability) edge(from, to string) {
	if r.edges[from] == nil {
		r.edges[from] = make(map[string]struct{})
	}
	r.edges[from][to] = struct{}{}
}

// addDeadModules judges every executable file once, against the seeds of
// every target: shared code reached by one service is not dead for another.
func (b *builder) addDeadModules() {
	if len(b.reach.roots) == 0 {
		return
	}
	reached := reach(b.reach.roots, b.reach.edges)
	files := make([]string, 0, len(b.reach.owner))
	for filePath := range b.reach.owner {
		files = append(files, filePath)
	}
	sort.Strings(files)
	for _, filePath := range files {
		if _, ok := reached[filePath]; ok || !b.reach.executable[filePath] || isDeclarationFile(filePath) {
			continue
		}
		target := b.reach.owner[filePath]
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
