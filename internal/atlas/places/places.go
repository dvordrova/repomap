// Package places builds the atlas graph: every directory and file a reader
// can visit, with the deterministic facts the code knows about it, and the
// file-to-file edges of the program graph. It is pure over its inputs and
// makes no model call.
package places

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/claims"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/dependencies"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/programindex"
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
	Index        programindex.Index
	Dependencies *dependencies.Catalog
	// Root is the target's root directory, repository-relative.
	Root string
}

// Input is everything places reads.
type Input struct {
	Revision   string
	Repository *corpus.Corpus
	Targets    []TargetInput
	Claims     claims.Result
	// Facts carries the boundaries the code already knows: routes, client
	// calls, listeners, configuration reads, dynamic execution.
	Facts facts.Result
}

// sdkPackages are standard-library packages whose calls with literal
// arguments are integration points even though the runtime is not an SDK:
// exactly these packages, not their subpackages (net/http client calls with
// a literal URL are already facts).
var sdkPackages = []string{"database/sql", "net"}

// sdkNeverPackages are packages whose literal arguments are never an
// integration: format strings, separators, error text, log messages, flag
// names, metric names, assertions. etcd's first atlas found 1,055 of its
// 1,791 "boundaries" in calls to zap.
var sdkNeverPackages = []string{
	"fmt", "strings", "errors", "bytes", "path", "path/filepath", "encoding/json", "io", "os",
	"time", "regexp", "flag", "html/template", "text/template", "sort", "strconv", "unicode", "log",
	"context", "sync", "math", "os/exec", "os/signal", "bufio", "crypto/sha256", "encoding/hex", "runtime",
	"go.uber.org/zap", "go.uber.org/multierr", "github.com/sirupsen/logrus", "k8s.io/klog",
	"github.com/golang/glog", "github.com/rs/zerolog", "github.com/go-logr/logr", "log/slog",
	"google.golang.org/grpc/status", "google.golang.org/grpc/codes", "google.golang.org/grpc/grpclog",
	"github.com/spf13/pflag", "github.com/spf13/cobra", "github.com/spf13/viper", "github.com/urfave/cli",
	"github.com/pkg/errors", "github.com/stretchr/testify", "github.com/onsi/ginkgo", "github.com/onsi/gomega",
	"github.com/prometheus/client_golang", "go.opentelemetry.io/otel", "github.com/google/go-cmp",
	"golang.org/x/exp", "golang.org/x/sync", "golang.org/x/text", "golang.org/x/net/context",
	"github.com/dustin/go-humanize", "github.com/olekukonko/tablewriter", "gopkg.in/yaml", "sigs.k8s.io/yaml",
	"github.com/xiang90/probing", "github.com/coreos/go-semver", "github.com/gogo/protobuf",
	"google.golang.org/protobuf", "github.com/golang/protobuf",
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
		input:     input,
		files:     make(map[string]*fileState),
		dirs:      make(map[string]*dirState),
		byID:      make(map[string]programindex.Object),
		fileOf:    make(map[string]string),
		fanIn:     make(map[string]int),
		edges:     make(map[edgeKey]*atlas.Edge),
		docs:      make(map[string][]claims.Claim),
		readmes:   make(map[string]corpus.Entry),
		entries:   make(map[string]corpus.Entry),
		seeds:     make(map[string]struct{}),
		targetOf:  make(map[string]map[string]struct{}),
		bounds:    make(map[boundaryKey]*boundaryState),
		workspace: make(map[string]struct{}),
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
	for _, target := range input.Targets {
		if err := target.Index.Validate(); err != nil {
			return atlas.Graph{}, fmt.Errorf("atlas places: target %s: %w", target.Index.Target.Name, err)
		}
		b.collectObjects(target)
	}
	for _, target := range input.Targets {
		b.collectEdges(target)
		b.collectImports(target)
		b.collectSeeds(target)
	}
	if err := b.readFiles(); err != nil {
		return atlas.Graph{}, err
	}
	b.collectDirectories()
	b.assignDepths()
	b.collectSymbols()
	b.collectBoundaries()
	for _, target := range input.Targets {
		b.collectExternalCalls(target)
	}
	return b.graph()
}

type fileState struct {
	path      string
	decls     []atlas.Decl
	doc       string
	generated bool
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
	path string
	line int
	kind string
}

type boundaryState struct {
	place atlas.Place
}

type builder struct {
	input    Input
	files    map[string]*fileState
	dirs     map[string]*dirState
	byID     map[string]programindex.Object
	fileOf   map[string]string
	fanIn    map[string]int
	edges    map[edgeKey]*atlas.Edge
	docs     map[string][]claims.Claim
	readmes  map[string]corpus.Entry
	entries  map[string]corpus.Entry
	seeds    map[string]struct{}
	targetOf map[string]map[string]struct{}
	bounds   map[boundaryKey]*boundaryState
	symbols  []atlas.Place
	// workspace lists the package paths of the repository's own modules, from
	// the dependency catalogs: a call into one of them is not an integration.
	workspace map[string]struct{}
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
		// Go's "variable" objects are call results and locals are not shown.
		if object.Location == nil || object.OwnerID == "" {
			return false
		}
		owner, ok := byID[object.OwnerID]
		if !ok || owner.Kind != programindex.ObjectModule {
			return false
		}
		return object.Name != "" && object.Name != "self" && !strings.HasPrefix(object.Name, "_")
	default:
		return false
	}
}

func (b *builder) collectObjects(target TargetInput) {
	index := target.Index
	targetID := index.Target.ID
	for _, object := range index.Objects {
		b.byID[object.ID] = object
	}
	for _, object := range index.Objects {
		if object.Location == nil || object.Kind == programindex.ObjectExternalSymbol {
			continue
		}
		filePath := atlasPath(object.Location.Path)
		b.fileOf[object.ID] = filePath
		state := b.file(filePath)
		state.targets[targetID] = struct{}{}
		if state.language == "" {
			state.language = index.Target.Language
		}
		if !declaration(object, b.byID) {
			continue
		}
		name := object.Name
		if object.Kind == programindex.ObjectMethod && !strings.Contains(name, ".") {
			if owner, ok := b.byID[object.OwnerID]; ok && owner.Kind == programindex.ObjectType {
				name = owner.Name + "." + name
			}
		}
		// A file two targets index carries each declaration in both indexes.
		if state.hasDecl(object.Location.Line, name) {
			continue
		}
		state.decls = append(state.decls, atlas.Decl{
			Name:      name,
			Kind:      string(object.Kind),
			Signature: shortSignature(object.Signature),
			LineNo:    object.Location.Line,
			Column:    object.Location.Column,
			Exported:  object.Visibility == programindex.VisibilityPublic,
			ObjectID:  object.ID,
		})
	}
	if root := atlasPath(target.Root); root != "" {
		b.targetOf[targetID] = map[string]struct{}{root: {}}
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
			b.fanIn[toID]++
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
		if filePath, ok := b.fileOf[seed.ObjectID]; ok {
			b.seeds[filePath] = struct{}{}
			continue
		}
		if seed.Location != nil {
			if _, ok := b.files[atlasPath(seed.Location.Path)]; ok {
				b.seeds[atlasPath(seed.Location.Path)] = struct{}{}
			}
		}
	}
}

// readFiles attaches docstrings to declarations, finds module docs and marks
// generated files.
func (b *builder) readFiles() error {
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

// docstringFor finds the docstring that belongs to the declaration at line:
// the nearest docstring above it within reach, with no other declaration
// between them.
func (b *builder) docstringFor(filePath string, line int, decls []atlas.Decl) string {
	docs := b.docs[filePath]
	best := ""
	for _, doc := range docs {
		if doc.Line > line || line-doc.Line > docstringReach {
			continue
		}
		between := false
		for _, decl := range decls {
			if decl.LineNo > doc.Line && decl.LineNo < line {
				between = true
				break
			}
		}
		if between {
			continue
		}
		best = doc.Text
	}
	return firstSentence(best)
}

// moduleDoc is the file's own documentation: a Python module docstring, a
// leading JSDoc, or a Go package comment when this file carries it.
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
		if firstDecl == 0 || firstDecl-first.Line > docstringReach || first.Line <= 2 {
			return firstSentence(first.Text)
		}
	case ".go":
		if strings.HasPrefix(first.Text, "Package ") {
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

// shortSignature drops module paths from a Go signature: a reader says
// corpus.Entry, not github.com/owner/repo/internal/corpus.Entry.
func shortSignature(signature string) string {
	if !strings.Contains(signature, "/") {
		return signature
	}
	var out strings.Builder
	token := strings.Builder{}
	flush := func() {
		text := token.String()
		if slash := strings.LastIndex(text, "/"); slash >= 0 && strings.Contains(text[slash:], ".") {
			text = text[slash+1:]
		}
		out.WriteString(text)
		token.Reset()
	}
	for _, r := range signature {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '/' || r == '.' || r == '_' || r == '-' {
			token.WriteRune(r)
			continue
		}
		flush()
		out.WriteRune(r)
	}
	flush()
	return out.String()
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

// MaxSymbolCandidates is how many declarations of one file may become
// symbol places: exported and documented first, then by callers.
const MaxSymbolCandidates = 10

// collectSymbols lifts each file's most telling declarations to places.
func (b *builder) collectSymbols() {
	for filePath, state := range b.files {
		if state.generated {
			continue
		}
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
			if rank == MaxSymbolCandidates {
				break
			}
			given := decl.Doc
			if given == "" {
				given = decl.Signature
			}
			if given == "" {
				given = decl.Kind + " " + decl.Name
			}
			b.symbols = append(b.symbols, atlas.Place{
				ID: atlas.SymbolID(filePath, decl.LineNo, decl.Name), Kind: atlas.PlaceSymbol, Path: filePath,
				LineNo: decl.LineNo, Depth: state.depth, TargetIDs: sortedKeys(state.targets),
				Parent: atlas.FileID(filePath), Given: truncateRunes(given, maxLineRunes),
				Symbol: &atlas.SymbolFacts{Decl: decl, Candidate: true, Rank: rank + 1},
			})
		}
	}
}

// collectBoundaries lifts the facts the code already knows as integration
// points into boundary places, one per anchor and kind. A route registered
// under three prefixes is one boundary with three values.
func (b *builder) collectBoundaries() {
	// Facts name their own target rows; the atlas speaks in program target
	// IDs, so a fact's target is translated before it names a place.
	programTarget := make(map[string]string, len(b.input.Facts.Targets))
	for _, target := range b.input.Facts.Targets {
		programTarget[target.ID] = target.ProgramTargetID
	}
	for _, fact := range b.input.Facts.Facts {
		if fact.Anchor == nil {
			continue
		}
		targetID, ok := programTarget[fact.TargetID]
		if !ok || targetID == "" {
			continue
		}
		var direction, kind, method string
		var values []string
		switch fact.Kind {
		case facts.KindHTTPRoute:
			direction, kind, method, values = atlas.DirectionIn, atlas.BoundaryHTTPServer, fact.Method, []string{fact.Path}
		case facts.KindHTTPCall:
			direction, kind, method, values = atlas.DirectionOut, atlas.BoundaryHTTPClient, fact.Method, []string{fact.Path}
		case facts.KindListenAddress:
			direction, kind, values = atlas.DirectionIn, atlas.BoundaryHTTPServer, []string{fact.Value}
		case facts.KindConfigRead:
			direction, kind, values = atlas.DirectionOut, atlas.BoundaryConfig, []string{fact.Key}
			if fact.Value != "" {
				values = append(values, "default "+fact.Value)
			}
		case facts.KindDynamicExecution:
			direction, kind, values = atlas.DirectionOut, atlas.BoundaryOther, []string{fact.Key}
		default:
			continue
		}
		filePath := atlasPath(fact.Anchor.Path)
		file, ok := b.files[filePath]
		if !ok {
			continue
		}
		key := boundaryKey{path: filePath, line: fact.Anchor.Line, kind: kind}
		if state, exists := b.bounds[key]; exists {
			state.place.Boundary.Values = appendUnique(state.place.Boundary.Values, values...)
			state.place.TargetIDs = appendUnique(state.place.TargetIDs, targetID)
			continue
		}
		caller, callerDoc := b.callerOf(file, fact.ObjectID, fact.Symbol, fact.Anchor.Line)
		b.bounds[key] = &boundaryState{place: atlas.Place{
			ID: boundaryID(filePath, fact.Anchor.Line, kind), Kind: atlas.PlaceBoundary, Path: filePath,
			LineNo: fact.Anchor.Line, Depth: file.depth, TargetIDs: []string{targetID},
			Parent: atlas.FileID(filePath),
			Boundary: &atlas.BoundaryFacts{
				Source: "fact", FactID: fact.ID, ObjectID: fact.ObjectID,
				Caller: caller, CallerDoc: callerDoc, Method: method, Values: values,
				Direction: direction, GivenKind: kind,
			},
		}}
	}
}

// collectExternalCalls lifts calls into non-platform packages that carry a
// literal argument: an SDK client method called with a topic, a table, a
// bucket. The model says what kind of integration it is.
func (b *builder) collectExternalCalls(target TargetInput) {
	for _, relation := range target.Index.Relations {
		if relation.Kind != programindex.RelationInvokesExternal {
			continue
		}
		from, ok := b.fileOf[relation.FromID]
		if !ok {
			continue
		}
		var external *programindex.ExternalSymbol
		for _, toID := range relation.ToIDs {
			if object, ok := b.byID[toID]; ok && object.External != nil {
				external = object.External
				break
			}
		}
		if external == nil || !sdkCandidate(*external) {
			continue
		}
		if _, own := b.workspace[external.PackagePath]; own {
			continue
		}
		var values []string
		line := relationLine(relation)
		for _, pattern := range relation.Patterns {
			if pattern.Location != nil && line == 0 {
				line = pattern.Location.Line
			}
			for _, argument := range pattern.Arguments {
				if value, ok := literalArgument(argument); ok {
					values = appendUnique(values, value)
				}
			}
		}
		if len(values) == 0 || line == 0 {
			continue
		}
		if len(values) > 8 {
			values = values[:8]
		}
		claimed := false
		for key := range b.bounds {
			if key.path == from && key.line == line {
				claimed = true
				break
			}
		}
		if claimed {
			continue
		}
		key := boundaryKey{path: from, line: line, kind: "sdk"}
		if state, exists := b.bounds[key]; exists {
			state.place.Boundary.Values = appendUnique(state.place.Boundary.Values, values...)
			state.place.TargetIDs = appendUnique(state.place.TargetIDs, target.Index.Target.ID)
			continue
		}
		caller := b.byID[relation.FromID]
		callerName, callerDoc := b.callerOf(b.files[from], relation.FromID, displayName(caller, b.byID), line)
		b.bounds[key] = &boundaryState{place: atlas.Place{
			ID: boundaryID(from, line, "sdk"), Kind: atlas.PlaceBoundary, Path: from,
			LineNo: line, Depth: b.files[from].depth, TargetIDs: []string{target.Index.Target.ID},
			Parent: atlas.FileID(from),
			Boundary: &atlas.BoundaryFacts{
				Source: "external_call", ObjectID: relation.FromID,
				Caller: callerName, CallerDoc: callerDoc,
				External: externalName(*external), Values: values,
				Direction: atlas.DirectionOut,
			},
		}}
	}
}

func sdkCandidate(external programindex.ExternalSymbol) bool {
	if _, never := packageMatches(external.PackagePath, sdkNeverPackages...); never {
		return false
	}
	if external.AuthorityKind == programindex.ExternalAuthorityPackage {
		return true
	}
	for _, exact := range sdkPackages {
		if external.PackagePath == exact {
			return true
		}
	}
	return false
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
				Generated: state.generated,
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
		return a.Kind < c.Kind
	})
	if graph.Edges == nil {
		graph.Edges = []atlas.Edge{}
	}
	graph.Seeds = make([]string, 0, len(b.seeds))
	for seed := range b.seeds {
		graph.Seeds = append(graph.Seeds, atlas.FileID(seed))
	}
	sort.Strings(graph.Seeds)
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
