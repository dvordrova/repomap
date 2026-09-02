package facts

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/programindex"
)

// TargetInput is one analyzed target as the pipeline knows it: its sealed
// ProgramIndex, its dependency catalog, and the root/manifest the target
// portfolio selected. Root and Manifest may be empty; Build then derives the
// root from the target's anchor file.
type TargetInput struct {
	Index        programindex.Index
	Dependencies *dependencies.Catalog
	RunID        string
	Root         string
	Manifest     string
}

// Input is everything the deterministic fact stage reads. Repository may be
// nil, in which case corpus-derived rows (TODOs, negatives, regex passes)
// are skipped. TrackedPaths is the unfiltered git listing and is used for
// path-only facts such as a committed .env file; its contents are never read.
type Input struct {
	Revision     string
	Repository   *corpus.Corpus
	TrackedPaths []string
	Targets      []TargetInput
}

// Build derives the sealed fact layer from its inputs. It is pure: the same
// input always yields a byte-identical sealed Result.
func Build(input Input) (Result, error) {
	builder, err := newBuilder(input)
	if err != nil {
		return Result{}, err
	}
	for _, target := range builder.targets {
		builder.addEntrypoints(target)
		builder.addHTTP(target)
		builder.addConfigReads(target)
		builder.addDynamicExecution(target)
		builder.addReachability(target)
		builder.addDependencies(target)
	}
	builder.addPortals()
	builder.addManifests()
	builder.addTODOs()
	builder.addNegatives()
	targets := make([]Target, 0, len(builder.targets))
	for _, target := range builder.targets {
		targets = append(targets, target.target)
	}
	return Seal(Result{
		Revision:    input.Revision,
		Targets:     targets,
		Facts:       builder.facts,
		Diagnostics: builder.diagnostics,
	})
}

type builder struct {
	input       Input
	source      *sourceReader
	targets     []*targetContext
	facts       []Fact
	ids         map[string]int
	seen        map[string]struct{}
	diagnostics []Diagnostic
}

func newBuilder(input Input) (*builder, error) {
	result := &builder{
		input: input,
		ids:   make(map[string]int),
		seen:  make(map[string]struct{}),
	}
	result.source = newSourceReader(input.Repository, result.diagnose)
	ids := make(map[string]struct{}, len(input.Targets))
	for _, value := range input.Targets {
		target, err := newTargetContext(value)
		if err != nil {
			return nil, err
		}
		if _, duplicate := ids[target.target.ID]; duplicate {
			return nil, fmt.Errorf("facts: duplicate target %s/%s", target.target.Language, target.target.Root)
		}
		ids[target.target.ID] = struct{}{}
		result.targets = append(result.targets, target)
	}
	sort.SliceStable(result.targets, func(i, j int) bool {
		return targetLess(result.targets[i].target, result.targets[j].target)
	})
	return result, nil
}

// add assigns the stable id and appends the row. Ids collide only when two
// rows share root, kind, anchor, line content and principal; the ordinal then
// follows insertion order, which is deterministic because every extractor
// walks sorted inputs.
func (b *builder) add(root string, fact Fact, principal ...string) Fact {
	anchorPath, line := "", ""
	if fact.Anchor != nil {
		anchorPath = fact.Anchor.Path
		line = b.source.line(fact.Anchor.Path, fact.Anchor.Line)
	}
	base := NewFactID(root, fact.Kind, anchorPath, line, principal...)
	b.ids[base]++
	fact.ID = WithOrdinal(base, b.ids[base])
	b.facts = append(b.facts, fact)
	return fact
}

// once reports whether key is new; extractors use it to keep one row per
// observed source position.
func (b *builder) once(key string) bool {
	if _, exists := b.seen[key]; exists {
		return false
	}
	b.seen[key] = struct{}{}
	return true
}

func (b *builder) diagnose(kind, detail string) {
	b.diagnostics = append(b.diagnostics, Diagnostic{Kind: kind, Detail: detail})
}

func (b *builder) ofKind(kind Kind) []Fact {
	var rows []Fact
	for _, fact := range b.facts {
		if fact.Kind == kind {
			rows = append(rows, fact)
		}
	}
	sortFacts(rows)
	return rows
}

// targetForPath returns the id of the target whose root is the longest prefix
// of the path, or "" for repository-level paths.
func (b *builder) targetForPath(filePath string) string {
	best, bestRoot := "", ""
	for _, target := range b.targets {
		root := target.root
		if !underRoot(filePath, root) {
			continue
		}
		if best == "" || len(root) > len(bestRoot) {
			best, bestRoot = target.target.ID, root
		}
	}
	return best
}

func underRoot(filePath, root string) bool {
	if root == "" || root == "." {
		return true
	}
	return strings.HasPrefix(filePath, root+"/")
}

// targetContext is one target with its object map and derived identity.
type targetContext struct {
	input     TargetInput
	target    Target
	root      string
	objects   map[string]programindex.Object
	callbacks map[string]string
}

func newTargetContext(input TargetInput) (*targetContext, error) {
	index := input.Index
	root := input.Root
	if root == "" {
		root = deriveRoot(index.Target)
	}
	if root == "" {
		return nil, fmt.Errorf("facts: target %q has no root", index.Target.Name)
	}
	manifest := input.Manifest
	if manifest == "" {
		manifest = deriveManifest(index.Target)
	}
	result := &targetContext{
		input:     input,
		root:      root,
		objects:   make(map[string]programindex.Object, len(index.Objects)),
		callbacks: make(map[string]string),
	}
	for _, object := range index.Objects {
		result.objects[object.ID] = object
	}
	for _, relation := range index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.SourceArgumentID != "" && len(relation.ToIDs) > 0 {
			result.callbacks[relation.SourceArgumentID] = relation.ToIDs[0]
		}
	}
	result.target = Target{
		ID:              NewTargetID(index.Target.Language, root, manifest),
		ProgramTargetID: index.Target.ID,
		Language:        index.Target.Language,
		Name:            index.Target.Name,
		Kind:            index.Target.Kind,
		Root:            root,
		Manifest:        manifest,
		Anchor:          result.targetAnchor(manifest),
		RunID:           input.RunID,
	}
	return result, nil
}

func deriveRoot(target programindex.Target) string {
	for _, source := range target.Sources {
		if source.FileRef == target.AnchorFileRef {
			return path.Dir(source.Path)
		}
	}
	return ""
}

func deriveManifest(target programindex.Target) string {
	for _, source := range target.Sources {
		if source.FileRef == target.AnchorFileRef && isManifestName(path.Base(source.Path)) {
			return source.Path
		}
	}
	return ""
}

// targetAnchor prefers the first entrypoint seed; a library without seeds is
// anchored to its manifest, and as a last resort to its anchor source file.
func (target *targetContext) targetAnchor(manifest string) Anchor {
	for _, seed := range target.input.Index.Target.Seeds {
		if seed.Location != nil {
			return Anchor{Path: seed.Location.Path, Line: seed.Location.Line}
		}
	}
	if manifest != "" {
		return Anchor{Path: manifest, Line: 1}
	}
	for _, source := range target.input.Index.Target.Sources {
		if source.FileRef == target.input.Index.Target.AnchorFileRef {
			return Anchor{Path: source.Path, Line: 1}
		}
	}
	return Anchor{Path: target.root, Line: 1}
}

func (b *builder) addEntrypoints(target *targetContext) {
	for _, seed := range target.input.Index.Target.Seeds {
		if seed.Location == nil {
			continue
		}
		object, _ := target.object(seed.ObjectID)
		b.add(target.root, Fact{
			Kind:     KindEntrypoint,
			TargetID: target.target.ID,
			Anchor:   &Anchor{Path: seed.Location.Path, Line: seed.Location.Line},
			Symbol:   object.Name,
			ObjectID: seed.ObjectID,
			Key:      string(seed.Kind),
		}, string(seed.Kind), object.Name)
	}
}
