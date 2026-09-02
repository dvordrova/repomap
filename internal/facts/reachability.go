package facts

import (
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

// isDeclarationFile excludes TypeScript ambient declarations: they are never
// imported at runtime, so unreachability says nothing about them.
func isDeclarationFile(filePath string) bool {
	return strings.HasSuffix(filePath, ".d.ts")
}
