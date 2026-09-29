// Package places builds the atlas graph: every directory and file a reader
// can visit, with the deterministic facts the code knows about it, and the
// file-to-file edges of the program graph. It is pure over its inputs and
// makes no model call.
package places

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

const (
	// docstringReach is how many lines above a declaration its docstring may
	// start; the same rule the page uses to put authors' words on cards.
	docstringReach = 12
	// generatedMarkerLines is how deep the generated-code marker is looked
	// for. kubernetes puts it after the license header.
	generatedMarkerLines = 30
	// maxReadBytes bounds what places reads from a file for its facts.
	maxReadBytes = 256 << 10
	// maxLineRunes bounds README and doc lines.
	maxLineRunes = 200
)

// TargetInput is one analyzed target: its program index and, for Go, the
// dependency catalog that carries package imports.
type TargetInput struct {
	Index programindex.Index
	// ReadIndex loads one saved, validated index; Index then carries only
	// its Target.
	ReadIndex    func() (programindex.Index, error)
	Dependencies *dependencies.Catalog
	// Root is the target's root directory, repository-relative.
	Root string
	// AbsorbedRoot is the root of a library folded into this program, which
	// it also claims (claimByRoot): shallower than Root, it gives the
	// program the library's other files without lifting Root to its depth,
	// so a sibling rooted with the program keeps sharing Root's files.
	AbsorbedRoot string
}

func (target TargetInput) read() (TargetInput, error) {
	if target.ReadIndex == nil {
		return target, nil
	}
	index, err := target.ReadIndex()
	if err != nil {
		return TargetInput{}, err
	}
	target.Index = index
	return target, nil
}

// Input is everything places reads.
type Input struct {
	Revision   string
	Repository *corpus.Corpus
	Targets    []TargetInput
	Claims     claims.Result
	// Facts carries the boundaries the code already knows: routes, client
	// calls, listeners, configuration reads, dynamic execution, and the
	// observed launch seeds and manifest values reused by Learn/questions.
	Facts facts.Result
}

var generatedMarker = regexp.MustCompile(`(?i)code generated .* do not edit|do not edit`)

// Build derives the graph. Same inputs give byte-identical output.
func Build(input Input) (atlas.Graph, error) {
	if input.Repository == nil {
		return atlas.Graph{}, fmt.Errorf("atlas places: repository corpus is required")
	}
	if len(input.Targets) == 0 {
		return atlas.Graph{}, fmt.Errorf("atlas places: no targets")
	}
	b := &builder{
		input:             input,
		files:             make(map[string]*fileState),
		dirs:              make(map[string]*dirState),
		byID:              make(map[string]programindex.Object),
		fileOf:            make(map[string]string),
		fanIn:             make(map[string]int),
		edges:             make(map[edgeKey]*atlas.Edge),
		docs:              make(map[string][]claims.Claim),
		readmes:           make(map[string]corpus.Entry),
		entries:           make(map[string]corpus.Entry),
		seeds:             make(map[string]struct{}),
		seedDecls:         make(map[string]struct{}),
		seedTargets:       make(map[string]map[string]struct{}),
		tableRows:         make(map[string][]atlas.TableRow),
		tableReadRows:     make(map[string]map[atlas.TableRead]bool),
		comparisons:       make(map[string][]atlas.Comparison),
		targetOf:          make(map[string]map[string]struct{}),
		bounds:            make(map[boundaryKey]*boundaryState),
		workspace:         make(map[string]struct{}),
		symbolCallerRows:  make(map[string]map[string]atlas.SymbolCaller),
		symbolBindingRows: make(map[string]map[string]atlas.SymbolBinding),
		symbolCallRows:    make(map[string]map[string]atlas.SymbolCall),
		symbolUseRows:     make(map[string]map[atlas.SymbolUse]bool),
		symbolFieldRows:   make(map[string]map[atlas.SymbolField]*sourcevalue.Value),
		factSubjects:      make(map[string]string),
		unreached:         make(map[string]map[string]struct{}),
	}
	for _, fact := range input.Facts.Facts {
		if fact.ObjectID != "" {
			b.factSubjects[scopedObjectID(fact.TargetID, fact.ObjectID)] = ""
		}
	}
	for _, target := range input.Targets {
		if target.Dependencies == nil {
			continue
		}
		for _, dependency := range target.Dependencies.Dependencies {
			if dependency.Kind == dependencies.KindWorkspace {
				b.workspace[dependency.PackagePath] = struct{}{}
			}
		}
		for _, importer := range target.Dependencies.Importers {
			b.workspace[importer.PackagePath] = struct{}{}
		}
	}
	b.indexClaims()
	b.indexCorpus()
	for _, saved := range input.Targets {
		target, err := saved.read()
		if err != nil {
			return atlas.Graph{}, err
		}
		if saved.ReadIndex == nil {
			if err := target.Index.Validate(); err != nil {
				return atlas.Graph{}, fmt.Errorf("atlas places: target %s: %w", target.Index.Target.Name, err)
			}
		}
		b.collectObjects(target)
		b.collectEdges(target)
		b.collectImports(target)
		b.collectSeeds(target)
		b.collectSymbolCallers(b.symbolCallerRows, target)
		b.collectSymbolBindings(b.symbolBindingRows, target)
		b.collectSymbolCalls(b.symbolCallRows, target)
		b.collectSymbolUses(b.symbolUseRows, target)
		b.collectSymbolFields(target)
		b.collectTableReads(target)
	}
	b.releaseTargetObjects()
	// A located seed may refer to a file supplied by a later target. Resolve
	// that membership after collecting the complete file inventory, without
	// loading every target again just to read its relations and seeds.
	for filePath := range b.seeds {
		if _, exists := b.files[filePath]; !exists {
			delete(b.seeds, filePath)
		}
	}
	b.claimByRoot()
	if err := b.readFiles(); err != nil {
		return atlas.Graph{}, err
	}
	b.collectDirectories()
	b.assignDepths()
	b.collectSymbols()
	b.collectBoundaries()
	return b.graph()
}

type fileState struct {
	path      string
	decls     []atlas.Decl
	doc       string
	generated bool
	test      bool
	targets   map[string]struct{}
	callers   map[string]struct{}
	callees   map[string]struct{}
	depth     int
	language  string
}

type dirState struct {
	path    string
	dirs    map[string]struct{}
	files   map[string]struct{}
	count   int
	targets map[string]struct{}
	readme  string
	doc     string
	topBox  bool
}

type edgeKey struct{ from, to, kind string }

type boundaryKey struct {
	path    string
	line    int
	kind    string
	column  int
	method  string
	values  string
	subject string
	// callee is the outside symbol and the call word. Targets share a place
	// only when they saw the same call; a target that resolved another
	// symbol at that position keeps its own place rather than borrowing the
	// first target's External, and two different calls at one position
	// never meet.
	callee string
}

type boundaryState struct {
	place atlas.Place
}

type builder struct {
	input             Input
	files             map[string]*fileState
	dirs              map[string]*dirState
	byID              map[string]programindex.Object
	symbolOf          map[string]string // native object -> shared, compiler-located symbol place
	factSubjects      map[string]string // only native object IDs requested by saved facts
	fileOf            map[string]string
	fanIn             map[string]int
	edges             map[edgeKey]*atlas.Edge
	docs              map[string][]claims.Claim
	readmes           map[string]corpus.Entry
	entries           map[string]corpus.Entry
	seeds             map[string]struct{}
	seedDecls         map[string]struct{}            // symbol places of seed declarations
	seedTargets       map[string]map[string]struct{} // symbol place -> targets it is the seed of
	tableRows         map[string][]atlas.TableRow    // symbol place of a table variable -> its word rows
	tableReadRows     map[string]map[atlas.TableRead]bool
	comparisons       map[string][]atlas.Comparison // symbol place -> the values it compares with several words
	targetOf          map[string]map[string]struct{}
	bounds            map[boundaryKey]*boundaryState
	symbols           []atlas.Place
	symbolCallerRows  map[string]map[string]atlas.SymbolCaller
	symbolBindingRows map[string]map[string]atlas.SymbolBinding
	symbolCallRows    map[string]map[string]atlas.SymbolCall
	symbolUseRows     map[string]map[atlas.SymbolUse]bool                 // symbol place -> what it reads, hands over or is decorated by
	symbolFieldRows   map[string]map[atlas.SymbolField]*sourcevalue.Value // symbol place -> the record fields it reads and writes, with the value a write stores
	memberOwners      map[string]string                                   // retained declaration -> native owner's symbol place
	unreached         map[string]map[string]struct{}                      // symbol place -> targets whose program never runs it
	typeFields        map[string]typeField
	// workspace lists the package paths of the repository's own modules, from
	// the dependency catalogs: a call into one of them is not an integration.
	workspace map[string]struct{}
}

type typeField struct {
	owner  string
	member atlas.TypeMember
}

func scopedObjectID(targetID, objectID string) string {
	return atlas.ScopedObjectID(targetID, objectID)
}

func (b *builder) indexClaims() {
	for _, claim := range b.input.Claims.Claims {
		if claim.Source == claims.SourceDocstring && claim.Path != "" {
			b.docs[claim.Path] = append(b.docs[claim.Path], claim)
		}
	}
	for filePath := range b.docs {
		sort.Slice(b.docs[filePath], func(i, j int) bool { return b.docs[filePath][i].Line < b.docs[filePath][j].Line })
	}
}

func (b *builder) indexCorpus() {
	for _, entry := range b.input.Repository.Entries() {
		b.entries[entry.Path] = entry
		base := strings.ToLower(path.Base(entry.Path))
		if base == "readme.md" || base == "readme" || base == "readme.rst" || base == "readme.txt" {
			dir := parentDir(entry.Path)
			if _, taken := b.readmes[dir]; !taken || base == "readme.md" {
				b.readmes[dir] = entry
			}
		}
	}
}

// declarationKinds says which objects are declarations a reader sees.
func declaration(object programindex.Object, byID map[string]programindex.Object) bool {
	switch object.Kind {
	case programindex.ObjectFunction, programindex.ObjectMethod, programindex.ObjectType:
		// Closures are named after their function with a "$n" suffix; they
		// are code, not declarations a reader looks up.
		if object.Name == "" || object.Name == "call result" || strings.Contains(object.Name, "$") {
			return false
		}
		return object.Location != nil
	case programindex.ObjectVariable:
		// Module-level variables of Python and TypeScript are declarations;
		// ContainerID is lexical containment; OwnerID identifies a callable/type
		// owner and is empty on module-level variables. Locals are not shown.
		if object.Location == nil || object.ContainerID == "" {
			return false
		}
		container, ok := byID[object.ContainerID]
		if !ok || container.Kind != programindex.ObjectModule {
			return false
		}
		return object.Name != "" && object.Name != "self" && !strings.HasPrefix(object.Name, "_")
	default:
		return false
	}
}

// Only the current target needs native-object lookups. Cross-target consumers
// keep the source-located declarations and observations they actually publish.
func (b *builder) useTargetObjects(index programindex.Index) {
	b.byID = make(map[string]programindex.Object, len(index.Objects))
	b.fileOf = make(map[string]string)
	b.symbolOf = make(map[string]string)
	callbacks := make(map[string]bool)
	executionOwners := make(map[string]bool)
	for _, relation := range index.Relations {
		switch relation.Kind {
		case programindex.RelationCalls, programindex.RelationExecutes, programindex.RelationInvokesExternal, programindex.RelationReads, programindex.RelationWrites:
			executionOwners[relation.FromID] = true
		}
		// A source-located closure can own a real call even when returned
		// from a factory rather than registered as a callback. Its caller
		// identity must survive for argument provenance and question evidence.
		if relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationInvokesExternal || relation.Kind == programindex.RelationExecutes {
			callbacks[relation.FromID] = true
		}
		if relation.Kind == programindex.RelationPassesCallback || relation.Kind == programindex.RelationBindsImplementation {
			for _, id := range relation.ToIDs {
				callbacks[id] = true
			}
		}
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				for _, id := range argument.ObjectIDs {
					callbacks[id] = true
				}
			}
		}
	}
	for _, object := range index.Objects {
		b.byID[object.ID] = object
	}
	for _, object := range index.Objects {
		if object.Location == nil || object.Kind == programindex.ObjectExternalSymbol {
			continue
		}
		filePath := atlasPath(object.Location.Path)
		b.fileOf[object.ID] = filePath
		// A module body can call or read another part directly. It needs its
		// own closed grouping choice; a file's declarations are not its caller.
		moduleBody := object.Kind == programindex.ObjectModule && object.Name != "" && executionOwners[object.ID]
		// A Go package-level variable whose initializer calls is the caller
		// of those calls (GO), a declaration like a module body.
		packageVariable := object.Kind == programindex.ObjectVariable && executionOwners[object.ID] && b.byID[object.ContainerID].Kind == programindex.ObjectPackage
		if !declaration(object, b.byID) && !moduleBody && !packageVariable && !(object.Kind == programindex.ObjectFunction && callbacks[object.ID]) {
			continue
		}
		name := object.Name
		if object.Kind == programindex.ObjectMethod && !strings.Contains(name, ".") {
			if owner, ok := b.byID[object.OwnerID]; ok && owner.Kind == programindex.ObjectType {
				name = owner.Name + "." + name
			}
		}
		// A file two targets index carries each declaration in both indexes.
		b.symbolOf[object.ID] = atlas.SymbolID(filePath, object.Location.Line, name)
		if len(object.Rows) > 0 && b.tableRows[b.symbolOf[object.ID]] == nil {
			var rows []atlas.TableRow
			// ProgramIndex validates every row: it has literals, each with its
			// location.
			for _, row := range object.Rows {
				literals := make([]atlas.RowLiteral, 0, len(row.Literals))
				for _, literal := range row.Literals {
					literals = append(literals, atlas.RowLiteral{Field: literal.Field, Value: literal.Value, LineNo: literal.Location.Line, Column: literal.Location.Column})
				}
				rows = append(rows, atlas.TableRow{Literals: literals})
			}
			b.tableRows[b.symbolOf[object.ID]] = rows
		}
		if len(object.Comparisons) > 0 && b.comparisons[b.symbolOf[object.ID]] == nil {
			b.comparisons[b.symbolOf[object.ID]] = atlasComparisons(object.Comparisons)
		}
	}
}

func (b *builder) releaseTargetObjects() {
	b.byID, b.fileOf, b.symbolOf = nil, nil, nil
}

func (b *builder) collectObjects(target TargetInput) {
	index := target.Index
	targetID := index.Target.ID
	b.useTargetObjects(index)
	if b.memberOwners == nil {
		b.memberOwners = make(map[string]string)
		b.typeFields = make(map[string]typeField)
	}
	for _, object := range index.Objects {
		filePath, located := b.fileOf[object.ID]
		if !located {
			continue
		}
		state := b.file(filePath)
		state.targets[targetID] = struct{}{}
		if state.language == "" {
			state.language = index.Target.Language
		}
		if b.symbolOf[object.ID] == "" {
			continue
		}
		scopedID := scopedObjectID(targetID, object.ID)
		// Keep the fact's exact target-local identity before declarations from
		// overlapping targets merge and the current native lookups are released.
		if _, needed := b.factSubjects[scopedID]; needed {
			b.factSubjects[scopedID] = b.symbolOf[object.ID]
		}
		if object.Unreachable {
			id := b.symbolOf[object.ID]
			if b.unreached[id] == nil {
				b.unreached[id] = make(map[string]struct{})
			}
			b.unreached[id][targetID] = struct{}{}
		}
		name := object.Name
		if object.Kind == programindex.ObjectMethod && !strings.Contains(name, ".") {
			if owner, ok := b.byID[object.OwnerID]; ok && owner.Kind == programindex.ObjectType {
				name = owner.Name + "." + name
			}
		}
		if state.hasDecl(object.Location.Line, name) {
			continue
		}
		state.decls = append(state.decls, atlas.Decl{
			Name:      name,
			Kind:      string(object.Kind),
			Signature: object.Signature,
			Aliases:   aliasText(object.Aliases),
			LineNo:    object.Location.Line,
			Column:    object.Location.Column,
			EndLine:   object.EndLine,
			CodeLines: object.CodeLines,
			Exported:  object.Visibility == programindex.VisibilityPublic,
			Macro:     object.Macro,
			ObjectID:  scopedID,
		})
		if owner := b.byID[object.OwnerID]; owner.Kind == programindex.ObjectType && owner.Location != nil {
			b.memberOwners[scopedID] = b.symbolOf[owner.ID]
		}
	}
	for _, object := range index.Objects {
		owner := b.byID[object.OwnerID]
		if object.Kind != programindex.ObjectVariable || object.Location == nil || owner.Kind != programindex.ObjectType || object.ContainerID != owner.ID {
			continue
		}
		id := b.symbolOf[owner.ID]
		filePath := atlasPath(object.Location.Path)
		file := b.files[filePath]
		if id == "" || file == nil {
			continue
		}
		key := fmt.Sprintf("%s\x00%s\x00%d\x00%d\x00%s", id, filePath, object.Location.Line, object.Location.Column, object.Name)
		// The previous all-object pass sorted native IDs before deduplicating
		// fields. Preserve that representative independently of target order.
		scopedID := scopedObjectID(targetID, object.ID)
		if previous, exists := b.typeFields[key]; exists && previous.member.Decl.ObjectID <= scopedID {
			continue
		}
		b.typeFields[key] = typeField{owner: id, member: atlas.TypeMember{Path: filePath, Decl: atlas.Decl{
			ObjectID: scopedID, Name: object.Name, Kind: string(object.Kind), Signature: object.Signature, Aliases: aliasText(object.Aliases), Types: typeAnchors(object.Types),
			LineNo: object.Location.Line, Column: object.Location.Column, Exported: object.Visibility == programindex.VisibilityPublic,
		}}}
	}
	if root := atlasPath(target.Root); root != "" {
		b.targetOf[targetID] = map[string]struct{}{root: {}}
	}
}

// claimByRoot gives a file under another target's root to that target
// alone: a program reaches the files of the libraries it uses, but they are
// the library's, and a call into them is the seam between the two. The
// deepest indexed root wins. Targets with the same root keep their shared
// files; their order cannot erase the library beside an executable. A
// program's absorbed root is a second, shallower root of the same program.
func (b *builder) claimByRoot() {
	type root struct {
		targetID string
		path     string
	}
	var roots []root
	for _, target := range b.input.Targets {
		path := atlasPath(target.Root)
		if path == "" {
			continue
		}
		roots = append(roots, root{target.Index.Target.ID, path})
		// atlasPath reads an empty path as the repository root: only a
		// program that absorbed a library has a second root.
		if target.AbsorbedRoot != "" {
			roots = append(roots, root{target.Index.Target.ID, atlasPath(target.AbsorbedRoot)})
		}
	}
	for filePath, state := range b.files {
		owners, depth := make(map[string]struct{}), -1
		for _, r := range roots {
			if _, indexed := state.targets[r.targetID]; !indexed {
				continue
			}
			if r.path != "." && filePath != r.path && !strings.HasPrefix(filePath, r.path+"/") {
				continue
			}
			d := strings.Count(r.path, "/") + 1
			if r.path == "." {
				d = 0
			}
			if d > depth {
				owners, depth = make(map[string]struct{}), d
			}
			if d == depth {
				owners[r.targetID] = struct{}{}
			}
		}
		if len(owners) > 0 {
			state.targets = owners
		}
	}
}

func (state *fileState) hasDecl(line int, name string) bool {
	for _, decl := range state.decls {
		if decl.LineNo == line && decl.Name == name {
			return true
		}
	}
	return false
}

func (b *builder) file(filePath string) *fileState {
	state, ok := b.files[filePath]
	if !ok {
		state = &fileState{
			path: filePath, targets: make(map[string]struct{}),
			callers: make(map[string]struct{}), callees: make(map[string]struct{}),
		}
		b.files[filePath] = state
	}
	return state
}

func (b *builder) collectEdges(target TargetInput) {
	for _, relation := range target.Index.Relations {
		kind := edgeKind(relation.Kind)
		if kind == "" {
			continue
		}
		from, ok := b.fileOf[relation.FromID]
		if !ok {
			continue
		}
		caller := b.byID[relation.FromID]
		for _, toID := range relation.ToIDs {
			b.fanIn[scopedObjectID(target.Index.Target.ID, toID)]++
			to, ok := b.fileOf[toID]
			if !ok || to == from {
				continue
			}
			callee := b.byID[toID]
			b.addEdge(atlas.FileID(from), atlas.FileID(to), kind, atlas.Witness{
				Caller: displayName(caller, b.byID), Callee: displayName(callee, b.byID),
				Path: from, LineNo: relationLine(relation),
			})
			b.file(from).callees[to] = struct{}{}
			b.file(to).callers[from] = struct{}{}
		}
	}
}

func edgeKind(kind programindex.RelationKind) string {
	switch kind {
	case programindex.RelationCalls:
		return "calls"
	case programindex.RelationImports:
		return "imports"
	case programindex.RelationPassesCallback:
		return "passes_callback"
	case programindex.RelationBindsImplementation:
		return "binds_implementation"
	case programindex.RelationDecorates:
		return "decorates"
	case programindex.RelationExecutes:
		return "executes"
	default:
		return ""
	}
}

func relationLine(relation programindex.Relation) int {
	if relation.Location != nil {
		return relation.Location.Line
	}
	for _, witness := range relation.Witnesses {
		if witness.Location != nil {
			return witness.Location.Line
		}
	}
	return 0
}

func displayName(object programindex.Object, byID map[string]programindex.Object) string {
	name := object.Name
	if object.Kind == programindex.ObjectMethod && !strings.Contains(name, ".") {
		if owner, ok := byID[object.OwnerID]; ok && owner.Kind == programindex.ObjectType {
			name = owner.Name + "." + name
		}
	}
	if name == "" {
		return string(object.Kind)
	}
	return name
}

func (b *builder) addEdge(from, to, kind string, witness atlas.Witness) {
	key := edgeKey{from, to, kind}
	edge, ok := b.edges[key]
	if !ok {
		edge = &atlas.Edge{From: from, To: to, Kind: kind}
		b.edges[key] = edge
	}
	edge.Count++
	if witness.Caller != "" && len(edge.Witnesses) < 64 {
		witness.Kind = kind
		edge.Witnesses = append(edge.Witnesses, witness)
	}
}

// collectImports adds directory-to-directory edges for Go package imports of
// workspace packages, which the object graph does not carry.
func (b *builder) collectImports(target TargetInput) {
	catalog := target.Dependencies
	if catalog == nil {
		return
	}
	importers := make(map[string]dependencies.Importer, len(catalog.Importers))
	for _, importer := range catalog.Importers {
		importers[importer.Ref] = importer
	}
	for _, dependency := range catalog.Dependencies {
		if dependency.Kind != dependencies.KindWorkspace || dependency.RepositoryPath == "" {
			continue
		}
		to := atlasPath(dependency.RepositoryPath)
		for _, ref := range dependency.ImporterRefs {
			importer, ok := importers[ref]
			if !ok || importer.RepositoryPath == "" {
				continue
			}
			from := atlasPath(importer.RepositoryPath)
			if from == to {
				continue
			}
			// An import has no call site to witness; the edge counts alone.
			b.addEdge(atlas.DirectoryID(from), atlas.DirectoryID(to), "imports", atlas.Witness{})
		}
	}
}

func (b *builder) collectSeeds(target TargetInput) {
	for _, seed := range target.Index.Target.Seeds {
		// The declaration execution begins in, when it is one of the
		// graph's symbol places, locates the entry inside its file.
		if symbol := b.symbolOf[seed.ObjectID]; symbol != "" {
			b.seedDecls[symbol] = struct{}{}
			if b.seedTargets[symbol] == nil {
				b.seedTargets[symbol] = make(map[string]struct{})
			}
			b.seedTargets[symbol][target.Index.Target.ID] = struct{}{}
		}
		if filePath, ok := b.fileOf[seed.ObjectID]; ok {
			b.seeds[filePath] = struct{}{}
			continue
		}
		if seed.Location != nil {
			b.seeds[atlasPath(seed.Location.Path)] = struct{}{}
		}
	}
}

// readFiles attaches docstrings to declarations, finds module docs and marks
// generated files.
func (b *builder) readFiles() error {
	for _, target := range b.input.Targets {
		for _, source := range target.Index.Target.TestSources {
			if state := b.files[atlasPath(source)]; state != nil {
				state.test = true
			}
		}
	}
	for filePath, state := range b.files {
		sort.Slice(state.decls, func(i, j int) bool {
			if state.decls[i].LineNo != state.decls[j].LineNo {
				return state.decls[i].LineNo < state.decls[j].LineNo
			}
			return state.decls[i].Name < state.decls[j].Name
		})
		for i := range state.decls {
			state.decls[i].FanIn = b.fanIn[state.decls[i].ObjectID]
			state.decls[i].Doc = b.docstringFor(filePath, state.decls[i].LineNo, state.decls)
		}
		state.doc = b.moduleDoc(filePath, state)
		entry, ok := b.entries[filePath]
		if !ok {
			continue
		}
		state.generated = generatedByName(filePath)
		if state.generated {
			continue
		}
		content, err := b.input.Repository.ReadFile(entry.ID, 8<<10)
		if err != nil {
			return fmt.Errorf("atlas places: read %s: %w", filePath, err)
		}
		state.generated = generatedByMarker(content.Bytes)
	}
	return nil
}

func generatedByName(filePath string) bool {
	base := path.Base(filePath)
	return strings.HasPrefix(base, "zz_generated") || strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, ".pb.gw.go") || strings.HasSuffix(base, "_generated.go") ||
		strings.HasSuffix(base, ".gen.go") || strings.HasSuffix(base, ".d.ts") ||
		strings.HasSuffix(base, ".min.js")
}

func generatedByMarker(content []byte) bool {
	lines := bytes.Split(content, []byte("\n"))
	if len(lines) > generatedMarkerLines {
		lines = lines[:generatedMarkerLines]
	}
	for _, line := range lines {
		trimmed := bytes.TrimSpace(line)
		if !bytes.HasPrefix(trimmed, []byte("//")) && !bytes.HasPrefix(trimmed, []byte("#")) &&
			!bytes.HasPrefix(trimmed, []byte("/*")) && !bytes.HasPrefix(trimmed, []byte("*")) {
			continue
		}
		if generatedMarker.Match(trimmed) && bytes.Contains(bytes.ToLower(trimmed), []byte("generated")) {
			return true
		}
	}
	return false
}

// docstringFor finds the author quote attached to this declaration. Python
// body docstrings carry their declaration line; comment-based languages use
// the existing neighbouring-comment rule.
func (b *builder) docstringFor(filePath string, line int, decls []atlas.Decl) string {
	return firstSentence(b.quotedDocstringFor(filePath, line, decls))
}

// quotedDocstringFor retains the existing bounded author quote. Type contracts
// need later sentences as well: these often state effects or lifecycle rules.
func (b *builder) quotedDocstringFor(filePath string, line int, decls []atlas.Decl) string {
	docs := b.docs[filePath]
	if claims.CPath(filePath) {
		// A C docstring describes only the declaration whose header it names.
		declared := make([]int, len(decls))
		for i, decl := range decls {
			declared[i] = decl.LineNo
		}
		return claims.CDocstring(docs, line, declared)
	}
	best := ""
	for _, doc := range docs {
		if strings.EqualFold(path.Ext(filePath), ".py") {
			if doc.DeclarationLine == line {
				return doc.Text
			}
			continue
		}
		clojure := strings.HasSuffix(filePath, ".clj") || strings.HasSuffix(filePath, ".cljc") || strings.HasSuffix(filePath, ".cljs")
		if clojure && doc.Line < line || !clojure && doc.Line > line || line-doc.Line > docstringReach || doc.Line-line > docstringReach {
			continue
		}
		between := false
		for _, decl := range decls {
			if decl.LineNo > min(doc.Line, line) && decl.LineNo < max(doc.Line, line) {
				between = true
				break
			}
		}
		if between {
			continue
		}
		best = doc.Text
	}
	return best
}

// moduleDoc is the file's own documentation: a Python module docstring, a
// leading JSDoc, a Go package comment when this file carries it, or a C
// file's opening comment that no declaration follows directly.
func (b *builder) moduleDoc(filePath string, state *fileState) string {
	docs := b.docs[filePath]
	if len(docs) == 0 {
		return ""
	}
	first := docs[0]
	firstDecl := 0
	if len(state.decls) > 0 {
		firstDecl = state.decls[0].LineNo
	}
	switch strings.ToLower(path.Ext(filePath)) {
	case ".py":
		if first.DeclarationLine == 0 && (firstDecl == 0 || first.Line <= firstDecl) {
			return firstSentence(first.Text)
		}
	case ".go":
		if strings.HasPrefix(first.Text, "Package ") {
			return firstSentence(first.Text)
		}
	case ".c", ".h":
		if first.DeclarationLine == 0 {
			return firstSentence(first.Text)
		}
	default:
		if firstDecl == 0 || firstDecl-first.Line > docstringReach {
			return firstSentence(first.Text)
		}
	}
	return ""
}

func (b *builder) collectDirectories() {
	for filePath, state := range b.files {
		dir := parentDir(filePath)
		d := b.dir(dir)
		d.files[path.Base(filePath)] = struct{}{}
		for targetID := range state.targets {
			d.targets[targetID] = struct{}{}
		}
		child := dir
		for {
			b.dir(child).count++
			if child == "." {
				break
			}
			parent := parentDir(child)
			p := b.dir(parent)
			p.dirs[path.Base(child)] = struct{}{}
			for targetID := range state.targets {
				p.targets[targetID] = struct{}{}
			}
			child = parent
		}
	}
	for dir, state := range b.dirs {
		state.readme = b.readmeLine(dir)
		state.doc = b.packageDoc(dir)
		state.topBox = len(state.files) > 0 && !b.hasFilesAbove(dir)
	}
}

func (b *builder) hasFilesAbove(dir string) bool {
	for dir != "." {
		dir = parentDir(dir)
		if state, ok := b.dirs[dir]; ok && len(state.files) > 0 {
			return true
		}
	}
	return false
}

func (b *builder) dir(dir string) *dirState {
	state, ok := b.dirs[dir]
	if !ok {
		state = &dirState{
			path: dir, dirs: make(map[string]struct{}), files: make(map[string]struct{}),
			targets: make(map[string]struct{}),
		}
		b.dirs[dir] = state
	}
	return state
}

func (b *builder) readmeLine(dir string) string {
	entry, ok := b.readmes[dir]
	if !ok {
		return ""
	}
	content, err := b.input.Repository.ReadFile(entry.ID, maxReadBytes)
	if err != nil || !utf8.Valid(content.Bytes) {
		return ""
	}
	// A README's first readable line is often its title alone; the first
	// line that says something is the first with a few words in it.
	first := ""
	for _, line := range strings.Split(string(content.Bytes), "\n") {
		text := readableLine(line)
		if text == "" {
			continue
		}
		if first == "" {
			first = text
		}
		if len(strings.Fields(text)) >= 4 {
			return truncateRunes(text, maxLineRunes)
		}
	}
	return truncateRunes(first, maxLineRunes)
}

// readableLine strips markdown decoration and keeps prose only.
func readableLine(line string) string {
	text := strings.TrimSpace(line)
	if text == "" || strings.HasPrefix(text, "<") || strings.HasPrefix(text, "[![") ||
		strings.HasPrefix(text, "```") || strings.HasPrefix(text, "|") || strings.HasPrefix(text, "---") ||
		strings.HasPrefix(text, "===") {
		return ""
	}
	text = strings.TrimLeft(text, "#> ")
	text = strings.NewReplacer("**", "", "__", "", "`", "").Replace(text)
	// [label](url) -> label
	for {
		open := strings.Index(text, "](")
		if open < 0 {
			break
		}
		start := strings.LastIndex(text[:open], "[")
		end := strings.Index(text[open:], ")")
		if start < 0 || end < 0 {
			break
		}
		text = text[:start] + text[start+1:open] + text[open+end+1:]
	}
	text = strings.TrimSpace(text)
	letters := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 3 {
		return ""
	}
	return text
}

// packageDoc is the directory's package documentation: the Go package
// comment of any file here, a Python __init__ docstring, or the description
// of a package.json.
func (b *builder) packageDoc(dir string) string {
	if init := path.Join(dir, "__init__.py"); b.hasFile(init) {
		if state, ok := b.files[init]; ok && state.doc != "" {
			return state.doc
		}
		if docs := b.docs[init]; len(docs) > 0 {
			return firstSentence(docs[0].Text)
		}
	}
	if entry, ok := b.entries[path.Join(dir, "package.json")]; ok {
		if content, err := b.input.Repository.ReadFile(entry.ID, maxReadBytes); err == nil {
			var manifest struct {
				Description string `json:"description"`
			}
			if json.Unmarshal(content.Bytes, &manifest) == nil && manifest.Description != "" {
				return truncateRunes(strings.TrimSpace(manifest.Description), maxLineRunes)
			}
		}
	}
	state, ok := b.dirs[dir]
	if !ok {
		return ""
	}
	names := make([]string, 0, len(state.files))
	for name := range state.files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if file, ok := b.files[path.Join(dir, name)]; ok && strings.HasPrefix(file.doc, "Package ") {
			return file.doc
		}
	}
	// Go package comments live above the package clause, which claims does
	// not quote; read the first lines of each Go file for it.
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		entry, ok := b.entries[path.Join(dir, name)]
		if !ok {
			continue
		}
		content, err := b.input.Repository.ReadFile(entry.ID, 16<<10)
		if err != nil {
			continue
		}
		if doc := goPackageComment(string(content.Bytes)); doc != "" {
			return doc
		}
	}
	return ""
}

func goPackageComment(source string) string {
	lines := strings.Split(source, "\n")
	var block []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "package "):
			if len(block) > 0 {
				text := strings.Join(block, " ")
				if strings.HasPrefix(text, "Package ") {
					return firstSentence(text)
				}
			}
			return ""
		case strings.HasPrefix(trimmed, "//"):
			if strings.HasPrefix(trimmed, "//go:") {
				continue
			}
			block = append(block, strings.TrimSpace(strings.TrimPrefix(trimmed, "//")))
		case trimmed == "":
			block = nil
		default:
			if strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
				block = append(block, strings.TrimSpace(strings.Trim(trimmed, "/* ")))
				continue
			}
			return ""
		}
	}
	return ""
}

func (b *builder) hasFile(filePath string) bool {
	_, ok := b.entries[filePath]
	return ok
}

// assignDepths runs the BFS over file edges from the union of seeds. Files no
// round reaches come last.
func (b *builder) assignDepths() {
	depth := make(map[string]int, len(b.files))
	queue := make([]string, 0, len(b.seeds))
	for seed := range b.seeds {
		queue = append(queue, seed)
		depth[seed] = 0
	}
	sort.Strings(queue)
	last := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		next := make([]string, 0)
		for callee := range b.files[current].callees {
			if _, seen := depth[callee]; seen {
				continue
			}
			depth[callee] = depth[current] + 1
			if depth[callee] > last {
				last = depth[callee]
			}
			next = append(next, callee)
		}
		sort.Strings(next)
		queue = append(queue, next...)
	}
	for filePath, state := range b.files {
		if d, ok := depth[filePath]; ok {
			state.depth = d
		} else {
			state.depth = last + 1
		}
	}
}

// collectSymbols keeps every eligible declaration, ordered by documentation,
// visibility and callers. Ranking changes presentation, never evidence coverage.
func (b *builder) collectSymbols() {
	calls := b.symbolCalls()
	bindings := b.symbolBindings()
	callers := b.symbolCallers()
	uses := b.symbolUses()
	fields := b.symbolFields()
	members := b.typeMembers()
	for filePath, state := range b.files {
		ranked := append([]atlas.Decl(nil), state.decls...)
		sort.SliceStable(ranked, func(i, j int) bool {
			a, c := ranked[i], ranked[j]
			if (a.Exported && a.Doc != "") != (c.Exported && c.Doc != "") {
				return a.Exported && a.Doc != ""
			}
			if a.Exported != c.Exported {
				return a.Exported
			}
			if (a.Doc != "") != (c.Doc != "") {
				return a.Doc != ""
			}
			if a.FanIn != c.FanIn {
				return a.FanIn > c.FanIn
			}
			return a.LineNo < c.LineNo
		})
		for rank, decl := range ranked {
			if state.generated && decl.Kind != "function" && decl.Kind != "method" && decl.Kind != "type" {
				continue
			}
			given := decl.Doc
			if given == "" {
				given = decl.Signature
			}
			if given == "" {
				given = decl.Kind + " " + decl.Name
			}
			id := atlas.SymbolID(filePath, decl.LineNo, decl.Name)
			if decl.Kind == string(programindex.ObjectType) {
				decl.Doc = b.quotedDocstringFor(filePath, decl.LineNo, state.decls)
			}
			// A target the file no longer belongs to (claimByRoot) holds
			// no declaration of it to leave unreached.
			var unreached []string
			for _, target := range sortedKeys(b.unreached[id]) {
				if _, holds := state.targets[target]; holds {
					unreached = append(unreached, target)
				}
			}
			var seeds []string
			for _, target := range sortedKeys(b.seedTargets[id]) {
				if _, holds := state.targets[target]; holds {
					seeds = append(seeds, target)
				}
			}
			b.symbols = append(b.symbols, atlas.Place{
				ID: id, Kind: atlas.PlaceSymbol, Path: filePath,
				LineNo: decl.LineNo, Column: decl.Column, Depth: state.depth, TargetIDs: sortedKeys(state.targets),
				Parent: atlas.FileID(filePath), Given: truncateRunes(given, maxLineRunes),
				Symbol: &atlas.SymbolFacts{Decl: decl, Members: members[id], Calls: calls[id], Bindings: bindings[id], CalledBy: callers[id], Uses: uses[id], Fields: fields[id], Candidate: !state.generated, Rank: rank + 1, Unreached: unreached, Seeds: seeds, Rows: b.tableRows[id],
					ReadAt: b.tableReads(id), Comparisons: b.comparisons[id]},
			})
		}
	}
	// Some declarations are intentionally not lifted (for example incidental
	// closures). Their names remain observations, but
	// they cannot become dangling context links.
	known := make(map[string]bool, len(b.symbols))
	for _, place := range b.symbols {
		known[place.ID] = true
	}
	for objectID, subjectID := range b.factSubjects {
		if !known[subjectID] {
			delete(b.factSubjects, objectID)
		}
	}
	for i := range b.symbols {
		symbol := b.symbols[i].Symbol
		for j := range symbol.Calls {
			var ids []string
			for _, id := range symbol.Calls[j].CalleeIDs {
				if known[id] {
					ids = append(ids, id)
				}
			}
			symbol.Calls[j].CalleeIDs = ids
		}
		for j := range symbol.CalledBy {
			if !known[symbol.CalledBy[j].PlaceID] {
				symbol.CalledBy[j].PlaceID = ""
			}
		}
		symbol.Uses = slices.DeleteFunc(symbol.Uses, func(use atlas.SymbolUse) bool { return !known[use.PlaceID] })
		if len(symbol.Uses) == 0 {
			symbol.Uses = nil
		}
		symbol.Fields = slices.DeleteFunc(symbol.Fields, func(field atlas.SymbolField) bool { return !known[field.TypeID] })
		if len(symbol.Fields) == 0 {
			symbol.Fields = nil
		}
	}
}

// collectSymbolUses keeps, for each lifted declaration, the declarations its
// exact or alternatives `reads`, `passes_callback` and `decorates` relations
// name: what it reads, what it hands over to be called later and, for a
// decorated declaration, its decorator. Unlike the calls lifted for context,
// these need no pattern, so a decoration written without arguments counts.
// A callable also uses, exactly (`takes`), each repository type one of its
// parameters carries (the ProgramIndex parameter's `type_id`): redis.c's
// freeIOJob and queueIOJob take an iojob.
func (b *builder) collectSymbolUses(rows map[string]map[atlas.SymbolUse]bool, target TargetInput) {
	add := func(from, to string, use atlas.SymbolUse) {
		if from == "" || to == "" || to == from {
			return
		}
		if rows[from] == nil {
			rows[from] = make(map[atlas.SymbolUse]bool)
		}
		use.PlaceID = to
		rows[from][use] = true
	}
	for _, object := range target.Index.Objects {
		for _, parameter := range object.Parameters {
			if parameter.TypeID != "" {
				add(b.symbolOf[object.ID], b.symbolOf[parameter.TypeID], atlas.SymbolUse{Kind: atlas.UseTakes, Resolution: string(programindex.ResolutionExact)})
			}
		}
	}
	for _, relation := range target.Index.Relations {
		switch relation.Kind {
		case programindex.RelationReads, programindex.RelationPassesCallback, programindex.RelationDecorates:
		default:
			continue
		}
		if relation.Resolution != programindex.ResolutionExact && relation.Resolution != programindex.ResolutionAlternatives {
			continue
		}
		from := b.symbolOf[relation.FromID]
		for _, to := range relation.ToIDs {
			add(from, b.symbolOf[to], atlas.SymbolUse{Kind: string(relation.Kind), Resolution: string(relation.Resolution)})
		}
	}
}

// collectSymbolFields keeps, for each lifted declaration, the record fields
// its reads and writes name with a field path, at the field as written, and
// the value a write stores when the adapter recorded it.
// They stay apart from Uses: a field is no declaration of its own, and what
// a declaration uses feeds the role split.
func (b *builder) collectSymbolFields(target TargetInput) {
	for _, relation := range target.Index.Relations {
		if relation.FieldPath == "" || relation.Location == nil {
			continue
		}
		from := b.symbolOf[relation.FromID]
		field, known := b.byID[relation.ToIDs[0]]
		if from == "" || !known {
			continue
		}
		typeID := b.symbolOf[field.OwnerID]
		if typeID == "" {
			continue
		}
		if b.symbolFieldRows[from] == nil {
			b.symbolFieldRows[from] = make(map[atlas.SymbolField]*sourcevalue.Value)
		}
		b.symbolFieldRows[from][atlas.SymbolField{TypeID: typeID, Field: field.Name, Path: relation.FieldPath, Kind: string(relation.Kind),
			LineNo: relation.Location.Line, Column: relation.Location.Column}] = sourcevalue.Clone(relation.Value)
	}
}

// atlasComparisons are an object's comparisons as the places graph keeps
// them; ProgramIndex validates each one located, with located cases.
func atlasComparisons(values []programindex.Comparison) []atlas.Comparison {
	result := make([]atlas.Comparison, 0, len(values))
	for _, value := range values {
		comparison := atlas.Comparison{Value: value.Value, Origin: sourcevalue.Clone(value.Origin), LineNo: value.Location.Line, Column: value.Location.Column}
		for _, item := range value.Cases {
			written := atlas.ComparisonCase{Form: string(item.Form), Words: slices.Clone(item.Words), LineNo: item.Location.Line, Column: item.Location.Column}
			if item.Branch != nil {
				written.BranchLine, written.BranchEnd = item.Branch.Line, item.Branch.EndLine
			}
			comparison.Cases = append(comparison.Cases, written)
		}
		result = append(result, comparison)
	}
	return result
}

// collectTableReads keeps, for each table (an object with rows), every
// located read of it by a lifted declaration, one per site and form: a
// membership test, the keys of each table it is read as (the shared
// witness kinds), or a plain read.
func (b *builder) collectTableReads(target TargetInput) {
	for _, relation := range target.Index.Relations {
		if relation.Kind != programindex.RelationReads || relation.Location == nil {
			continue
		}
		reader := b.symbolOf[relation.FromID]
		if reader == "" {
			continue
		}
		site := atlas.TableRead{ReaderID: reader, LineNo: relation.Location.Line, Column: relation.Location.Column}
		var reads []atlas.TableRead
		for _, witness := range relation.Witnesses {
			switch witness.Kind {
			case programindex.WitnessMembership:
				read := site
				read.Form = atlas.TableReadMembership
				reads = append(reads, read)
			case programindex.WitnessKeys:
				if of := b.symbolOf[witness.ObjectID]; of != "" && len(b.byID[witness.ObjectID].Rows) > 0 {
					read := site
					read.Form, read.KeysOf = atlas.TableReadKeys, of
					reads = append(reads, read)
				}
			}
		}
		if len(reads) == 0 {
			reads = []atlas.TableRead{site}
		}
		for _, to := range relation.ToIDs {
			table := b.symbolOf[to]
			if table == "" || len(b.byID[to].Rows) == 0 {
				continue
			}
			if b.tableReadRows[table] == nil {
				b.tableReadRows[table] = make(map[atlas.TableRead]bool)
			}
			for _, read := range reads {
				b.tableReadRows[table][read] = true
			}
		}
	}
}

// tableReads are a table's reads in reader, line and column order.
func (b *builder) tableReads(id string) []atlas.TableRead {
	set := b.tableReadRows[id]
	if len(set) == 0 {
		return nil
	}
	result := make([]atlas.TableRead, 0, len(set))
	for read := range set {
		result = append(result, read)
	}
	sort.Slice(result, func(i, j int) bool {
		a, c := result[i], result[j]
		if a.ReaderID != c.ReaderID {
			return a.ReaderID < c.ReaderID
		}
		if a.LineNo != c.LineNo {
			return a.LineNo < c.LineNo
		}
		if a.Column != c.Column {
			return a.Column < c.Column
		}
		return a.Form+"\x00"+a.KeysOf < c.Form+"\x00"+c.KeysOf
	})
	return result
}

func (b *builder) symbolFields() map[string][]atlas.SymbolField {
	result := make(map[string][]atlas.SymbolField, len(b.symbolFieldRows))
	for id, set := range b.symbolFieldRows {
		for field, value := range set {
			field.Value = value
			result[id] = append(result[id], field)
		}
		sort.Slice(result[id], func(i, j int) bool { return atlas.SymbolFieldLess(result[id][i], result[id][j]) })
	}
	return result
}

func (b *builder) symbolUses() map[string][]atlas.SymbolUse {
	result := make(map[string][]atlas.SymbolUse, len(b.symbolUseRows))
	for id, set := range b.symbolUseRows {
		for use := range set {
			result[id] = append(result[id], use)
		}
		sort.Slice(result[id], func(i, j int) bool { return atlas.SymbolUseLess(result[id][i], result[id][j]) })
	}
	return result
}

// typeMembers follows native ownership, including declarations in other files.
// A matching prefix, file, or method name never establishes membership.
func (b *builder) typeMembers() map[string][]atlas.TypeMember {
	result := make(map[string][]atlas.TypeMember)
	for path, file := range b.files {
		for _, decl := range file.decls {
			id := b.memberOwners[decl.ObjectID]
			if id != "" {
				decl.Doc = b.quotedDocstringFor(path, decl.LineNo, file.decls)
				result[id] = append(result[id], atlas.TypeMember{Path: path, Decl: decl})
			}
		}
	}
	// Class attributes are native declarations owned by the type, without
	// becoming unrelated top-level symbol candidates. Local variables and
	// assignments to an arbitrary instance never acquire type ownership here.
	for _, field := range b.typeFields {
		result[field.owner] = append(result[field.owner], field.member)
	}
	for id := range result {
		sort.Slice(result[id], func(i, j int) bool {
			a, c := result[id][i], result[id][j]
			if a.Path != c.Path {
				return a.Path < c.Path
			}
			if a.Decl.LineNo != c.Decl.LineNo {
				return a.Decl.LineNo < c.Decl.LineNo
			}
			if a.Decl.Column != c.Decl.Column {
				return a.Decl.Column < c.Decl.Column
			}
			return a.Decl.Name < c.Decl.Name
		})
	}
	return result
}

func (b *builder) collectSymbolCallers(rows map[string]map[string]atlas.SymbolCaller, target TargetInput) {
	for _, relation := range target.Index.Relations {
		if relation.Kind != programindex.RelationCalls && relation.Kind != programindex.RelationExecutes {
			continue
		}
		from, ok := b.byID[relation.FromID]
		if !ok || from.Location == nil {
			continue
		}
		row := atlas.SymbolCaller{ObjectID: scopedObjectID(target.Index.Target.ID, from.ID), PlaceID: b.symbolOf[from.ID], Name: displayName(from, b.byID), Signature: from.Signature, Path: from.Location.Path, Line: relationLine(relation), Kind: string(relation.Kind), Invocation: relation.Invocation, Dispatch: relation.Dispatch, Resolution: string(relation.Resolution)}
		key := row
		if key.PlaceID != "" {
			key.ObjectID = ""
		}
		raw, _ := json.Marshal(key)
		for _, nativeID := range relation.ToIDs {
			id := b.symbolOf[nativeID]
			if id == "" {
				continue
			}
			if rows[id] == nil {
				rows[id] = make(map[string]atlas.SymbolCaller)
			}
			rows[id][string(raw)] = row
		}
	}
}

func (b *builder) symbolCallers() map[string][]atlas.SymbolCaller {
	rows := b.symbolCallerRows
	if rows == nil {
		rows = make(map[string]map[string]atlas.SymbolCaller)
		for _, target := range b.input.Targets {
			b.useTargetObjects(target.Index)
			b.collectSymbolCallers(rows, target)
		}
	}
	result := make(map[string][]atlas.SymbolCaller)
	for id, values := range rows {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result[id] = append(result[id], values[key])
		}
	}
	return result
}

func (b *builder) collectSymbolBindings(rows map[string]map[string]atlas.SymbolBinding, target TargetInput) {
	// Callback provenance already identifies its exact argument at one call
	// site. Retain that registration's neighbouring literal arguments so a
	// handler can see its path/topic without reading unrelated factory calls.
	registrations := make(map[string][]atlas.RegistrationArgument)
	registrationEvidence := make(map[string][]atlas.EdgeEvidence)
	valueUses := make(map[string][]atlas.EdgeEvidence)
	producers := make(map[string][]programindex.RelationPattern)
	for _, relation := range target.Index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.ResultID != "" {
				producers[pattern.ResultID] = append(producers[pattern.ResultID], pattern)
			}
			if pattern.ReceiverID == "" || pattern.Location == nil {
				continue
			}
			valueUses[pattern.ReceiverID] = append(valueUses[pattern.ReceiverID], atlas.EdgeEvidence{
				Extractor: "registration_result_use", Label: "call on the registration result: " + pattern.Selector,
				Path: pattern.Location.Path, LineNo: pattern.Location.Line,
			})
		}
	}
	for _, relation := range target.Index.Relations {
		if relation.Kind == programindex.RelationPassesCallback && relation.SourceArgumentID != "" {
			registrations[relation.SourceArgumentID] = nil
		}
	}
	for _, relation := range target.Index.Relations {
		for _, pattern := range relation.Patterns {
			if pattern.Location == nil {
				continue
			}
			var selected []string
			for _, argument := range pattern.Arguments {
				if _, needed := registrations[argument.ID]; needed {
					selected = append(selected, argument.ID)
				}
			}
			if len(selected) == 0 {
				continue
			}
			var arguments []atlas.RegistrationArgument
			for _, argument := range pattern.Arguments {
				if value, ok := literalArgument(argument); ok {
					arguments = append(arguments, atlas.RegistrationArgument{Position: argument.Position, Keyword: argument.Keyword,
						Kind: string(argument.Kind), Value: value, Path: pattern.Location.Path, Line: pattern.Location.Line})
				}
			}
			for _, id := range selected {
				registrations[id] = arguments
				label := "receiving call: " + pattern.Selector
				if len(relation.ToIDs) == 1 {
					if recipient, ok := b.byID[relation.ToIDs[0]]; ok {
						label = "receiving call: " + displayName(recipient, b.byID)
					}
				}
				registrationEvidence[id] = append(registrationEvidence[id], atlas.EdgeEvidence{
					Extractor: "callback_registration", Label: label,
					Path: pattern.Location.Path, LineNo: pattern.Location.Line,
				})
				if pattern.ResultID != "" {
					registrationEvidence[id] = append(registrationEvidence[id], valueUses[pattern.ResultID]...)
				}
				// Fluent registration arguments belong to the exact receiver
				// producer, e.g. job.at("00:07").do(callback). Keep that syntax
				// beside the callback without interpreting a schedule locally.
				seen := map[string]bool{}
				var receiverEvidence func(string)
				receiverEvidence = func(receiverID string) {
					if receiverID == "" || seen[receiverID] {
						return
					}
					seen[receiverID] = true
					for _, producer := range producers[receiverID] {
						if producer.Location == nil {
							continue
						}
						var literals []string
						for _, argument := range producer.Arguments {
							if value, ok := literalArgument(argument); ok {
								literals = append(literals, strconv.Quote(value))
							}
						}
						registrationEvidence[id] = append(registrationEvidence[id], atlas.EdgeEvidence{
							Extractor: "registration_receiver_call", Label: producer.Selector + "(" + strings.Join(literals, ", ") + ")",
							Path: producer.Location.Path, LineNo: producer.Location.Line,
						})
						receiverEvidence(producer.ReceiverID)
					}
				}
				receiverEvidence(pattern.ReceiverID)
			}
		}
	}
	for _, relation := range target.Index.Relations {
		if relation.Kind != programindex.RelationPassesCallback && relation.Kind != programindex.RelationBindsImplementation {
			continue
		}
		from, known := b.byID[relation.FromID]
		if !known {
			continue
		}
		var evidence []atlas.EdgeEvidence
		for _, witness := range relation.Witnesses {
			if (witness.Kind == "callable_receiver_field" || witness.Kind == "interface_field_assignment") && witness.Location != nil {
				evidence = append(evidence, atlas.EdgeEvidence{Extractor: witness.Kind, Label: witness.Detail, Path: witness.Location.Path, LineNo: witness.Location.Line})
			}
		}
		for _, id := range relation.ToIDs {
			to, known := b.byID[id]
			if !known {
				continue
			}
			for _, witness := range relation.Witnesses {
				if witness.Kind == "callable_receiver_field" || witness.Kind == "interface_field_assignment" {
					continue
				}
				row := atlas.SymbolBinding{From: displayName(from, b.byID), To: displayName(to, b.byID), Detail: witness.Detail, Kind: string(relation.Kind), Resolution: string(relation.Resolution)}
				row.Evidence = append(append([]atlas.EdgeEvidence{}, evidence...), registrationEvidence[relation.SourceArgumentID]...)
				row.Evidence = canonicalBindingEvidence(row.Evidence)
				row.Arguments = registrations[relation.SourceArgumentID]
				if witness.Location != nil {
					row.Path, row.Line = witness.Location.Path, witness.Location.Line
				}
				raw, _ := json.Marshal(row)
				for _, owner := range []string{b.symbolOf[relation.FromID], b.symbolOf[id]} {
					if owner == "" {
						continue
					}
					if rows[owner] == nil {
						rows[owner] = make(map[string]atlas.SymbolBinding)
					}
					rows[owner][string(raw)] = row
				}
			}
		}
	}
}

// Binding evidence is a set of native observations. Target-local relation
// order must not create different binding identities for the same set.
func canonicalBindingEvidence(evidence []atlas.EdgeEvidence) []atlas.EdgeEvidence {
	sort.Slice(evidence, func(i, j int) bool {
		a, b := evidence[i], evidence[j]
		if a.Extractor != b.Extractor {
			return a.Extractor < b.Extractor
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.LineNo < b.LineNo
	})
	n := 0
	for _, observation := range evidence {
		if n == 0 || evidence[n-1] != observation {
			evidence[n] = observation
			n++
		}
	}
	return evidence[:n]
}

func (b *builder) symbolBindings() map[string][]atlas.SymbolBinding {
	rows := b.symbolBindingRows
	if rows == nil {
		rows = make(map[string]map[string]atlas.SymbolBinding)
		for _, target := range b.input.Targets {
			b.useTargetObjects(target.Index)
			b.collectSymbolBindings(rows, target)
		}
	}
	result := make(map[string][]atlas.SymbolBinding)
	for id, values := range rows {
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result[id] = append(result[id], values[key])
		}
	}
	return result
}

// symbolCalls preserves call-site evidence without a framework vocabulary.
// Multiple target indexes may contain the same declaration and witness.
func symbolCallKey(call atlas.SymbolCall) string {
	// Keep the existing evidence order independent of a local source
	// column. The column distinguishes otherwise identical source sites,
	// but must not reorder provider facts and invalidate unrelated answers.
	column := call.Column
	call.Column = 0
	raw, _ := json.Marshal(call)
	return fmt.Sprintf("%s:%d", raw, column)
}

func (b *builder) collectSymbolCalls(byObject map[string]map[string]atlas.SymbolCall, target TargetInput) {
	add := func(id string, call atlas.SymbolCall) {
		symbol := b.symbolOf[id]
		if symbol == "" {
			return
		}
		// A method declared on an external interface names the API exactly;
		// which implementation runs there was not observed.
		if call.Dispatch == programindex.DispatchInterfaceMethod {
			call.Resolution = string(programindex.ResolutionUnresolved)
		}
		if byObject[symbol] == nil {
			byObject[symbol] = make(map[string]atlas.SymbolCall)
		}
		byObject[symbol][symbolCallKey(call)] = call
	}
	for _, relation := range target.Index.Relations {
		if relation.Kind == programindex.RelationImports {
			continue
		}
		// Compiler dispatch and direct-call witnesses need not have a value
		// pattern. Dropping them removed the call into an implementation from
		// handler evidence, especially for interface dispatch.
		if len(relation.Patterns) == 0 && (relation.Kind == programindex.RelationCalls || relation.Kind == programindex.RelationExecutes || relation.Kind == programindex.RelationInvokesExternal) {
			var evidence []atlas.EdgeEvidence
			for _, witness := range relation.Witnesses {
				if witness.Kind == "interface_field_assignment" && witness.Location != nil {
					evidence = append(evidence, atlas.EdgeEvidence{Extractor: witness.Kind, Label: witness.Detail, Path: witness.Location.Path, LineNo: witness.Location.Line})
				}
			}
			for _, witness := range relation.Witnesses {
				// A receiver assignment supports this call; it is not another
				// call at the constructor's source line.
				if witness.Kind == "interface_field_assignment" {
					continue
				}
				call := atlas.SymbolCall{Kind: string(relation.Kind), Invocation: relation.Invocation, Dispatch: relation.Dispatch, Resolution: string(relation.Resolution), Detail: witness.Detail, Evidence: evidence}
				if witness.Location != nil {
					call.Line = witness.Location.Line
					call.Column = witness.Location.Column
				}
				for _, id := range relation.ToIDs {
					if object, ok := b.byID[id]; ok {
						call.Name = displayName(object, b.byID)
						call.CalleeIDs = nil
						if symbolID := b.symbolOf[id]; symbolID != "" {
							call.CalleeIDs = []string{symbolID}
						}
						add(relation.FromID, call)
					}
				}
				if len(relation.ToIDs) == 0 && call.Detail != "" {
					add(relation.FromID, call)
				}
			}
		}
		for _, pattern := range relation.Patterns {
			call := atlas.SymbolCall{Kind: string(relation.Kind), Name: pattern.Selector, Invocation: relation.Invocation, Dispatch: relation.Dispatch, Resolution: string(relation.Resolution), ReceiverValue: sourcevalue.Clone(pattern.ReceiverValue), ResultValue: sourcevalue.Clone(pattern.ResultValue)}
			for _, witness := range pattern.Context {
				if witness.Location != nil {
					call.Evidence = append(call.Evidence, atlas.EdgeEvidence{Extractor: witness.Kind, Label: witness.Detail, Path: witness.Location.Path, LineNo: witness.Location.Line})
				}
			}
			for _, id := range relation.ToIDs {
				if symbolID := b.symbolOf[id]; symbolID != "" {
					call.CalleeIDs = appendUnique(call.CalleeIDs, symbolID)
				}
			}
			sort.Strings(call.CalleeIDs)
			call.Stores = b.callStores(relation, call.CalleeIDs)
			// The selector alone loses the receiver/package: context.Background
			// and a remote client's Background would become the same evidence.
			if len(relation.ToIDs) == 1 {
				if object, ok := b.byID[relation.ToIDs[0]]; ok && object.External != nil {
					call.Name = externalName(*object.External)
					if object.External.RepositoryPath == "" {
						call.API = &atlas.CallAPI{Package: object.External.PackagePath, Receiver: object.External.Receiver, Name: object.External.Name, Signature: object.Signature}
					}
				}
			}
			if pattern.Location != nil {
				call.Line = pattern.Location.Line
				call.Column = pattern.Location.Column
			}
			for _, argument := range pattern.Arguments {
				if value, ok := literalArgument(argument); ok {
					call.Values = appendUnique(call.Values, value)
				}
				if argument.Origin != nil {
					call.SourceArguments = append(call.SourceArguments, atlas.SourceArgument{Position: argument.Position, Keyword: argument.Keyword, Origin: sourcevalue.Clone(argument.Origin)})
				}
				for _, id := range argument.ObjectIDs {
					if object, ok := b.byID[id]; ok {
						call.Arguments = appendUnique(call.Arguments, displayName(object, b.byID))
					}
				}
			}
			if call.Name == "" {
				continue
			}
			add(relation.FromID, call)
		}
	}
}

// callStores are where the code first stored each function a call reaches
// through a field or a name, from the witnesses that name the function they
// store, by callee.
func (b *builder) callStores(relation programindex.Relation, callees []string) []atlas.CallStore {
	first := map[string]atlas.CallStore{}
	for _, witness := range relation.Witnesses {
		id := b.symbolOf[witness.ObjectID]
		if witness.ObjectID == "" || witness.Location == nil || id == "" || !slices.Contains(callees, id) {
			continue
		}
		store := atlas.CallStore{CalleeID: id, Path: atlasPath(witness.Location.Path), LineNo: witness.Location.Line, Column: witness.Location.Column}
		previous, seen := first[id]
		if !seen || store.Path < previous.Path || store.Path == previous.Path && (store.LineNo < previous.LineNo || store.LineNo == previous.LineNo && store.Column < previous.Column) {
			first[id] = store
		}
	}
	var stores []atlas.CallStore
	for _, id := range callees {
		if store, ok := first[id]; ok {
			stores = append(stores, store)
		}
	}
	return stores
}

func (b *builder) symbolCalls() map[string][]atlas.SymbolCall {
	byObject := b.symbolCallRows
	if byObject == nil {
		byObject = make(map[string]map[string]atlas.SymbolCall)
		for _, target := range b.input.Targets {
			b.useTargetObjects(target.Index)
			b.collectSymbolCalls(byObject, target)
		}
	}
	result := make(map[string][]atlas.SymbolCall)
	for id, rows := range byObject {
		// A narrower target view can leave the same dispatch unresolved while
		// another view observes possible receivers. Keep every candidate and
		// its open resolution, but do not turn the empty view into another call.
		// Matching all other fields (including the compiler column) preserves
		// distinct call sites, dispatch details and independent evidence. An
		// exact call does not subsume an uncertain observation from another view.
		for _, call := range rows {
			if call.Resolution != string(programindex.ResolutionAlternatives) || len(call.CalleeIDs) == 0 || call.Line < 1 || call.Column < 1 {
				continue
			}
			call.Name, call.Resolution = "", string(programindex.ResolutionUnresolved)
			call.CalleeIDs, call.Evidence = nil, nil
			delete(rows, symbolCallKey(call))
		}
		keys := make([]string, 0, len(rows))
		for key := range rows {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			result[id] = append(result[id], rows[key])
		}
	}
	return result
}

// collectBoundaries lifts the facts the code already knows as integration
// points into boundary places. Only the same anchored observation is shared
// across targets; different methods, paths, columns and callees stay
// separate. Every adapter gives each call its own position, so one target
// never has two facts in one place: if it does, sealing refuses the graph,
// and nothing pairs such facts with another target's by order.
func (b *builder) collectBoundaries() {
	// Facts name their own target rows; the atlas speaks in program target
	// IDs, so a fact's target is translated before it names a place.
	programTarget := make(map[string]string, len(b.input.Facts.Targets))
	for _, target := range b.input.Facts.Targets {
		programTarget[target.ID] = target.ID
	}
	for _, fact := range b.input.Facts.Facts {
		if fact.Anchor == nil {
			continue
		}
		targetID, ok := programTarget[fact.TargetID]
		if !ok || targetID == "" {
			continue
		}
		var direction, kind, method, external, holder, invocation string
		var values, words []string
		var kept *atlas.RegistrarFacts
		switch fact.Kind {
		case facts.KindRegistration:
			// The shape is the fact; its kind is the model's to decide. A
			// registration handing over a callable brings work in; one that
			// only names an address sends work out.
			direction, method, external = atlas.DirectionOut, fact.Method, fact.Text
			if fact.ObjectID != "" {
				direction = atlas.DirectionIn
			}
			values = registrationValues(fact)
			words = registrationWords(fact)
			if fact.Holder != nil {
				holder = fmt.Sprintf("%s:%d:%d", fact.Holder.Path, fact.Holder.Line, fact.Holder.Column)
			}
			if registrar := fact.Registrar; registrar != nil {
				kept = &atlas.RegistrarFacts{Name: registrar.Name, Path: registrar.Path, Signature: registrar.Signature, Slots: slices.Clone(registrar.Slots)}
				for _, during := range registrar.During {
					kept.During = append(kept.During, atlas.DuringFacts{Seed: during.Seed, HandedTo: during.HandedTo, Handlers: slices.Clone(during.Handlers), Through: slices.Clone(during.Through)})
				}
				// The literals are the call's; the registrar's name is no word.
				words = slices.Clone(fact.Values)
			}
			if fact.Invocation != "" {
				// The statement starting a callable (`go`) names nothing:
				// its words are the started call's literals.
				invocation, words = fact.Invocation, slices.Clone(fact.Values)
			}
		case facts.KindSQLQuery:
			direction, kind = atlas.DirectionOut, atlas.BoundaryDB
			if fact.Key != "" {
				values = append(values, strings.Split(fact.Key, ", ")...)
			}
			values = append(values, fact.Value)
		case facts.KindConfigRead:
			direction, kind, values = atlas.DirectionOut, atlas.BoundaryConfig, []string{fact.Key}
			if fact.Value != "" {
				values = append(values, "default "+fact.Value)
			}
		default:
			continue
		}
		filePath := atlasPath(fact.Anchor.Path)
		file, ok := b.files[filePath]
		if !ok {
			continue
		}
		encodedValues, _ := json.Marshal(values)
		// The boundary belongs to the callable handed over when there is
		// one, otherwise to the declaration making the call.
		owner := fact.ObjectID
		if owner == "" {
			owner = fact.OwnerID
		}
		objectID := scopedObjectID(targetID, owner)
		key := boundaryKey{path: filePath, line: fact.Anchor.Line, column: fact.Anchor.Column, kind: kind,
			method: method, values: string(encodedValues), subject: b.factSubjects[objectID], callee: external + "\x00" + fact.Key}
		origin := atlas.BoundaryOrigin{TargetID: targetID, FactID: fact.ID, ObjectID: objectID}
		if state, exists := b.bounds[key]; exists {
			state.place.Boundary.Origins = append(state.place.Boundary.Origins, origin)
			state.place.TargetIDs = appendUnique(state.place.TargetIDs, targetID)
			continue
		}
		caller, callerDoc := b.callerOf(file, objectID, fact.Symbol, fact.Anchor.Line)
		b.bounds[key] = &boundaryState{place: atlas.Place{
			ID: nativeBoundaryID(key), Kind: atlas.PlaceBoundary, Path: filePath,
			LineNo: fact.Anchor.Line, Column: fact.Anchor.Column, Depth: file.depth, TargetIDs: []string{targetID},
			Parent: atlas.FileID(filePath),
			Boundary: &atlas.BoundaryFacts{
				Source: "fact", Origins: []atlas.BoundaryOrigin{origin}, ObjectID: objectID, SubjectID: b.factSubjects[objectID],
				Caller: caller, CallerDoc: callerDoc, External: external, Method: method, Values: values, Words: words,
				Holder: holder, Handed: fact.Kind == facts.KindRegistration && fact.Handed, Direction: direction, GivenKind: kind,
				Registrar: kept, Invocation: invocation,
			},
		}}
	}
}

func externalName(external programindex.ExternalSymbol) string {
	pkg := external.PackagePath
	if slash := strings.LastIndex(pkg, "/"); slash >= 0 {
		pkg = pkg[slash+1:]
	}
	name := external.Name
	if strings.HasPrefix(name, pkg+".") {
		name = strings.TrimPrefix(name, pkg+".")
	}
	if external.Receiver != "" && !strings.HasPrefix(name, external.Receiver+".") {
		name = strings.TrimPrefix(external.Receiver, "*") + "." + name
	}
	return pkg + "." + name
}

func literalArgument(argument programindex.PatternArgument) (string, bool) {
	switch argument.Kind {
	case programindex.PatternLiteralString:
		return argument.Value, argument.Value != ""
	case programindex.PatternStringTemplate:
		var text strings.Builder
		for _, part := range argument.Parts {
			if part.Kind == programindex.PatternPartHole {
				text.WriteString("{param}")
				continue
			}
			text.WriteString(part.Text)
		}
		return text.String(), text.Len() > 0
	default:
		return "", false
	}
}

// packageMatches reports whether a package path is one of the candidates or
// beneath one, tolerating a Go major-version suffix.
func packageMatches(packagePath string, candidates ...string) (string, bool) {
	if slash := strings.LastIndex(packagePath, "/"); slash >= 0 {
		suffix := packagePath[slash+1:]
		if len(suffix) >= 2 && suffix[0] == 'v' && strings.Trim(suffix[1:], "0123456789") == "" {
			packagePath = packagePath[:slash]
		}
	}
	for _, candidate := range candidates {
		if packagePath == candidate || strings.HasPrefix(packagePath, candidate+"/") {
			return candidate, true
		}
	}
	return "", false
}

// callerOf names the declaration a boundary sits in and its docstring.
func (b *builder) callerOf(file *fileState, objectID, symbol string, line int) (string, string) {
	if file == nil {
		return symbol, ""
	}
	if objectID != "" {
		for _, decl := range file.decls {
			if decl.ObjectID == objectID {
				return decl.Name, decl.Doc
			}
		}
	}
	if symbol != "" {
		for _, decl := range file.decls {
			if decl.Name == symbol || strings.HasSuffix(decl.Name, "."+symbol) {
				return decl.Name, decl.Doc
			}
		}
	}
	best := atlas.Decl{}
	for _, decl := range file.decls {
		if decl.LineNo <= line && decl.LineNo >= best.LineNo {
			best = decl
		}
	}
	if best.Name != "" {
		return best.Name, best.Doc
	}
	return symbol, ""
}

func boundaryID(filePath string, line int, kind string) string {
	return fmt.Sprintf("bnd:%s:%d:%s", filePath, line, kind)
}

func nativeBoundaryID(key boundaryKey) string {
	encoded, _ := json.Marshal([]any{key.path, key.line, key.column, key.kind, key.method, key.values, key.subject, key.callee})
	return fmt.Sprintf("%s:%x", boundaryID(key.path, key.line, key.kind), sha256.Sum256(encoded))
}

// ownedScopes keeps the observed target scopes whose page also holds the
// file, in their observed order.
func ownedScopes(observed []string, owners map[string]struct{}) []string {
	kept := make([]string, 0, len(observed))
	for _, id := range observed {
		if _, owns := owners[id]; owns {
			kept = append(kept, id)
		}
	}
	return kept
}

// originsWithin keeps the native origins of the retained target scopes; the
// atlas requires origins and scopes to name the same targets.
func originsWithin(origins []atlas.BoundaryOrigin, targetIDs []string) []atlas.BoundaryOrigin {
	kept := make([]atlas.BoundaryOrigin, 0, len(origins))
	for _, origin := range origins {
		for _, id := range targetIDs {
			if origin.TargetID == id {
				kept = append(kept, origin)
				break
			}
		}
	}
	return kept
}

func appendUnique(values []string, more ...string) []string {
	for _, value := range more {
		if value == "" {
			continue
		}
		found := false
		for _, existing := range values {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			values = append(values, value)
		}
	}
	return values
}

// boundaryGiven is the fallback line of a boundary: what it touches, in
// which declaration.
func boundaryGiven(place atlas.Place) string {
	facts := place.Boundary
	subject := facts.External
	if subject == "" {
		subject = facts.GivenKind
	}
	detail := strings.Join(facts.Values, ", ")
	if facts.Method != "" {
		detail = facts.Method + " " + detail
	}
	text := subject
	if detail != "" {
		text += " " + detail
	}
	if facts.Caller != "" {
		text += " in " + facts.Caller
	}
	return truncateRunes(strings.TrimSpace(text), maxLineRunes)
}

func (b *builder) graph() (atlas.Graph, error) {
	graph := atlas.Graph{Version: atlas.GraphVersion, Revision: b.input.Revision}
	for dir, state := range b.dirs {
		place := atlas.Place{
			ID: atlas.DirectoryID(dir), Kind: atlas.PlaceDirectory, Path: dir,
			Depth: treeDepth(dir), TargetIDs: sortedKeys(state.targets),
			Directory: &atlas.DirectoryFacts{
				Readme: state.readme, Doc: state.doc,
				Dirs: sortedKeys(state.dirs), Files: sortedKeys(state.files),
				FileCount: state.count, TopBox: state.topBox,
			},
		}
		if dir != "." {
			place.Parent = atlas.DirectoryID(parentDir(dir))
		}
		place.Given = directoryGiven(place)
		graph.Places = append(graph.Places, place)
	}
	for filePath, state := range b.files {
		place := atlas.Place{
			ID: atlas.FileID(filePath), Kind: atlas.PlaceFile, Path: filePath,
			Depth: state.depth, TargetIDs: sortedKeys(state.targets),
			Parent: atlas.DirectoryID(parentDir(filePath)),
			File: &atlas.FileFacts{
				Doc: state.doc, Decls: state.decls,
				Callers: fileIDs(state.callers), Callees: fileIDs(state.callees),
				Generated: state.generated, Test: state.test,
			},
		}
		if place.File.Decls == nil {
			place.File.Decls = []atlas.Decl{}
		}
		place.Given = fileGiven(place)
		graph.Places = append(graph.Places, place)
	}
	graph.Places = append(graph.Places, b.symbols...)
	for _, state := range b.bounds {
		place := state.place
		// An external candidate follows its file's ownership as before. A
		// native observation retains only its original target scopes, and
		// only among the targets holding its file: a target whose page lacks
		// the file has no box for the boundary, and the atlas refuses a
		// boundary outside every box. Freqtrade observed one route fact from
		// seven index views while one product held the file.
		if file, ok := b.files[place.Path]; ok {
			if place.Boundary.Source != "fact" {
				place.TargetIDs = sortedKeys(file.targets)
			} else {
				place.TargetIDs = ownedScopes(place.TargetIDs, file.targets)
				place.Boundary.Origins = originsWithin(place.Boundary.Origins, place.TargetIDs)
				if len(place.TargetIDs) == 0 {
					// No page can hold it, so no row should be bought for it.
					continue
				}
			}
		}
		place.Boundary.Origins = atlas.CanonicalBoundaryOrigins(place.Boundary.Origins)
		// This is only the shared row's representative. Target projection uses
		// Origins; semantic context uses the compiler-located SubjectID.
		if len(place.Boundary.Origins) > 0 {
			place.Boundary.ObjectID = place.Boundary.Origins[0].ObjectID
		}
		sort.Strings(place.TargetIDs)
		if place.Boundary.Values == nil {
			place.Boundary.Values = []string{}
		}
		place.Given = boundaryGiven(place)
		graph.Places = append(graph.Places, place)
	}
	for position := range graph.Places {
		sanitizePlace(&graph.Places[position])
	}
	b.addExtractions(&graph)
	b.addSourceFacts(&graph)
	if err := b.addDocuments(&graph); err != nil {
		return atlas.Graph{}, err
	}
	atlas.SortPlaces(graph.Places)
	known := make(map[string]struct{}, len(graph.Places))
	for _, place := range graph.Places {
		known[place.ID] = struct{}{}
	}
	for _, edge := range b.edges {
		// An import of a package whose declarations the index did not reach
		// names a directory with no place; the edge has nowhere to land.
		if _, ok := known[edge.From]; !ok {
			continue
		}
		if _, ok := known[edge.To]; !ok {
			continue
		}
		sort.SliceStable(edge.Witnesses, func(i, j int) bool {
			if edge.Witnesses[i].LineNo != edge.Witnesses[j].LineNo {
				return edge.Witnesses[i].LineNo < edge.Witnesses[j].LineNo
			}
			return edge.Witnesses[i].Caller+edge.Witnesses[i].Callee < edge.Witnesses[j].Caller+edge.Witnesses[j].Callee
		})
		if edge.Witnesses == nil {
			edge.Witnesses = []atlas.Witness{}
		}
		graph.Edges = append(graph.Edges, *edge)
	}
	sort.Slice(graph.Edges, func(i, j int) bool {
		a, c := graph.Edges[i], graph.Edges[j]
		if a.From != c.From {
			return a.From < c.From
		}
		if a.To != c.To {
			return a.To < c.To
		}
		if a.Kind != c.Kind {
			return a.Kind < c.Kind
		}
		if a.Evidence != nil && c.Evidence != nil {
			if a.Evidence.Label != c.Evidence.Label {
				return a.Evidence.Label < c.Evidence.Label
			}
			if a.Evidence.Path != c.Evidence.Path {
				return a.Evidence.Path < c.Evidence.Path
			}
			return a.Evidence.LineNo < c.Evidence.LineNo
		}
		return false
	})
	if graph.Edges == nil {
		graph.Edges = []atlas.Edge{}
	}
	graph.Seeds = make([]string, 0, len(b.seeds))
	for seed := range b.seeds {
		graph.Seeds = append(graph.Seeds, atlas.FileID(seed))
	}
	sort.Strings(graph.Seeds)
	graph.SeedDecls = make([]string, 0, len(b.seedDecls))
	for seed := range b.seedDecls {
		if _, ok := known[seed]; ok {
			graph.SeedDecls = append(graph.SeedDecls, seed)
		}
	}
	sort.Strings(graph.SeedDecls)
	return graph, nil
}

// directoryGiven is the fallback line of a directory: its README's first
// line, its package doc, or what it holds.
func directoryGiven(place atlas.Place) string {
	facts := place.Directory
	if facts.Readme != "" {
		return facts.Readme
	}
	if facts.Doc != "" {
		return facts.Doc
	}
	names := append(append([]string{}, facts.Dirs...), facts.Files...)
	if len(names) > 3 {
		names = names[:3]
	}
	unit := "files"
	if facts.FileCount == 1 {
		unit = "file"
	}
	if len(names) == 0 {
		return fmt.Sprintf("%d %s", facts.FileCount, unit)
	}
	return fmt.Sprintf("%d %s: %s", facts.FileCount, unit, strings.Join(names, ", "))
}

// fileGiven is the fallback line of a file: its module doc, its first
// docstring, or its declarations.
func fileGiven(place atlas.Place) string {
	facts := place.File
	if facts.Doc != "" {
		return facts.Doc
	}
	for _, decl := range facts.Decls {
		if decl.Doc != "" {
			return decl.Doc
		}
	}
	names := make([]string, 0, 3)
	for _, decl := range facts.Decls {
		names = append(names, decl.Name)
		if len(names) == 3 {
			break
		}
	}
	unit := "declarations"
	if len(facts.Decls) == 1 {
		unit = "declaration"
	}
	if len(names) == 0 {
		return "no declarations"
	}
	return fmt.Sprintf("%d %s: %s", len(facts.Decls), unit, strings.Join(names, ", "))
}

// sanitizePlace keeps every text of a place printable: a literal argument
// with a newline or a docstring with a tab would otherwise be refused by the
// atlas, and a request must never carry a control character.
func sanitizePlace(place *atlas.Place) {
	place.Given = cleanText(place.Given)
	if place.Directory != nil {
		place.Directory.Readme = cleanText(place.Directory.Readme)
		place.Directory.Doc = cleanText(place.Directory.Doc)
	}
	if place.File != nil {
		place.File.Doc = cleanText(place.File.Doc)
		for i := range place.File.Decls {
			place.File.Decls[i].Doc = cleanText(place.File.Decls[i].Doc)
			place.File.Decls[i].Signature = cleanText(place.File.Decls[i].Signature)
		}
	}
	if place.Symbol != nil {
		place.Symbol.Decl.Doc = cleanText(place.Symbol.Decl.Doc)
		place.Symbol.Decl.Signature = cleanText(place.Symbol.Decl.Signature)
		for i := range place.Symbol.Members {
			place.Symbol.Members[i].Decl.Doc = cleanText(place.Symbol.Members[i].Decl.Doc)
			place.Symbol.Members[i].Decl.Signature = cleanText(place.Symbol.Members[i].Decl.Signature)
		}
	}
	if place.Boundary != nil {
		place.Boundary.CallerDoc = cleanText(place.Boundary.CallerDoc)
		for i := range place.Boundary.Values {
			place.Boundary.Values[i] = cleanText(place.Boundary.Values[i])
		}
	}
}

// cleanText replaces control characters with spaces and collapses runs.
func cleanText(text string) string {
	if text == "" {
		return ""
	}
	dirty := false
	for _, r := range text {
		if r < 0x20 || r == 0x7f {
			dirty = true
			break
		}
	}
	if !dirty {
		return text
	}
	fields := strings.FieldsFunc(text, func(r rune) bool { return r < 0x20 || r == 0x7f || unicode.IsSpace(r) })
	return strings.Join(fields, " ")
}

func fileIDs(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for filePath := range set {
		result = append(result, atlas.FileID(filePath))
	}
	sort.Strings(result)
	return result
}

func sortedKeys(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func parentDir(filePath string) string {
	dir := path.Dir(filePath)
	if dir == "" || dir == "/" {
		return "."
	}
	return dir
}

func treeDepth(dir string) int {
	if dir == "." {
		return 0
	}
	return strings.Count(dir, "/") + 1
}

func atlasPath(value string) string {
	value = strings.TrimPrefix(strings.ReplaceAll(value, "\\", "/"), "./")
	value = path.Clean(value)
	if value == "" || value == "/" {
		return "."
	}
	return strings.TrimPrefix(value, "/")
}

// firstSentence keeps the first sentence of a docstring, bounded.
func firstSentence(text string) string {
	text = strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if text == "" {
		return ""
	}
	for i, r := range text {
		if (r == '.' || r == '!' || r == '?') && (i+1 == len(text) || text[i+1] == ' ') {
			// Do not cut "e.g." or a version like "v1.2".
			if i >= 2 && (unicode.IsDigit(rune(text[i-1])) && i+1 < len(text) && unicode.IsDigit(rune(text[i+1]))) {
				continue
			}
			return truncateRunes(text[:i+1], maxLineRunes)
		}
	}
	return truncateRunes(text, maxLineRunes)
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	cut := limit - 1
	for cut > limit/2 && !unicode.IsSpace(runes[cut]) {
		cut--
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}

// aliasText renders a declaration's other-format names for a reader:
// "json:count_label db:count".
// typeAnchors are a field's type declarations as source anchors.
func typeAnchors(locations []programindex.Location) []sourcevalue.Anchor {
	var result []sourcevalue.Anchor
	for _, location := range locations {
		result = append(result, sourcevalue.Anchor{Path: location.Path, Line: location.Line, Column: location.Column})
	}
	return result
}

func aliasText(aliases []programindex.Alias) string {
	parts := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		parts = append(parts, alias.Format+":"+alias.Name)
	}
	return strings.Join(parts, " ")
}

// registrationValues are what the model reads about a registration: the
// address it answers on when one exists, then its other literals. The call
// word travels as the external symbol behind the call.
func registrationValues(fact facts.Fact) []string {
	if fact.Path != "" {
		// The address, with its mount prefixes composed, stands for the
		// literal it came from.
		return []string{fact.Path}
	}
	return appendUnique(nil, fact.Values...)
}

// registrationWords are what the code wrote at a registration, each once and
// as written: the call word (GET, HandleFunc, the record type of a table
// row), every literal in order, and the address with its mount prefixes
// composed when that is not one of the literals. The model names an entry
// from them; a verb, a path, a command name or a topic is not told apart here.
func registrationWords(fact facts.Fact) []string {
	var words []string
	if fact.Key != "" {
		words = append(words, fact.Key)
	}
	words = appendUnique(words, fact.Values...)
	if fact.Path != "" {
		words = appendUnique(words, fact.Path)
	}
	return words
}
