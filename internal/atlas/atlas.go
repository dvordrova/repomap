// Package atlas is the reading layer of repomap: the repository as places a
// reader visits (targets, directories, files, symbols, boundaries), the
// one-line answers the model writes about them, and the boxes, zones, arrows
// and joints the code derives from those answers and from the program graph.
//
// Two artifacts live here. places.json (Graph) is what the code knows before
// any model call: every place with its deterministic facts and fallback line,
// and the file-to-file edges. atlas.json (Atlas) is what the page reads: the
// same places folded into boxes per target, with the model's lines beside
// the code's counts. Fields written by the model are marked MODEL in the
// comments; everything else is computed.
package atlas

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

const (
	// GraphVersion and Version change when the shape of the artifacts
	// changes; an artifact of another version is refused, never patched.
	GraphVersion = 26
	Version      = 19

	GraphFilename    = "places.json"
	ArtifactFilename = "atlas.json"
	// TablesFilename is the owner run's human-readable print of every table
	// row: what was asked, the fallback, and what the model answered.
	TablesFilename = "tables.md"
	// TablesDir holds the exact request (and, after a live run, response)
	// bytes of every window, one file per window.
	TablesDir = "tables"
)

// PlaceKind is the closed set of things a reader visits.
type PlaceKind string

const (
	PlaceDirectory  PlaceKind = "directory"
	PlaceFile       PlaceKind = "file"
	PlaceSymbol     PlaceKind = "symbol"
	PlaceBoundary   PlaceKind = "boundary"
	PlaceEntity     PlaceKind = "entity"
	PlaceDocument   PlaceKind = "document"
	PlaceSourceFact PlaceKind = "source_fact"
)

func (kind PlaceKind) Valid() bool {
	switch kind {
	case PlaceDirectory, PlaceFile, PlaceSymbol, PlaceBoundary, PlaceEntity, PlaceDocument, PlaceSourceFact:
		return true
	default:
		return false
	}
}

// Place is one thing a reader can be asked about. ID is a short graph-local
// ordinal whose prefix identifies only the closed place kind. Paths and names
// remain ordinary fields instead of being repeated inside identity strings.
type Place struct {
	ID   string    `json:"id"`
	Kind PlaceKind `json:"kind"`
	Path string    `json:"path"`
	// LineNo is the declaration or call-site line for symbols and boundaries.
	LineNo int `json:"line_no,omitempty"`
	Column int `json:"column,omitempty"`
	// Depth orders the reading: tree depth for directories, the BFS round over
	// the call graph from the union of every target's seeds for files.
	Depth int `json:"depth"`
	// TargetIDs are the program targets whose objects live here.
	TargetIDs []string `json:"target_ids"`
	// Given is the deterministic line the place carries when the model has
	// not answered: a fallback, never a hint the model sees for itself.
	Given string `json:"given"`
	// Parent is the place this one was reached from in the tree: the parent
	// directory for a directory or a file, the file for a symbol or boundary.
	Parent string `json:"parent,omitempty"`

	Directory  *DirectoryFacts `json:"directory,omitempty"`
	File       *FileFacts      `json:"file,omitempty"`
	Symbol     *SymbolFacts    `json:"symbol,omitempty"`
	Boundary   *BoundaryFacts  `json:"boundary,omitempty"`
	Entity     *EntityFacts    `json:"entity,omitempty"`
	Document   *DocumentFacts  `json:"document,omitempty"`
	SourceFact *SourceFact     `json:"source_fact,omitempty"`
}

// SourceFact carries an existing launch or manifest observation into reading.
// It does not create a code-file group, execution edge or model description.
type SourceFact struct {
	Kind          string `json:"kind"`
	Name          string `json:"name,omitempty"`
	Key           string `json:"key"`
	Value         string `json:"value,omitempty"`
	Language      string `json:"language"`
	Component     string `json:"component"`
	ComponentKind string `json:"component_kind"`
	Root          string `json:"root"`
	// ObjectID is the native launch subject, never a provider ref.
	ObjectID string `json:"object_id,omitempty"`
}

// DocumentFacts is one repository-authored Markdown section. Text retains
// commands, links and later paragraphs verbatim; it is a claim, not verified
// behavior. The section's identity and starting line belong to its Place.
type DocumentFacts struct {
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	EndLine  int               `json:"end_line"`
	Headings []DocumentHeading `json:"headings,omitempty"`
}

// DocumentHeading retains a section's written ancestry in the same document.
// It describes author scope, never runtime or target ownership.
type DocumentHeading struct {
	Title string `json:"title"`
	Line  int    `json:"line"`
}

// EntityFacts retains a producer's observation without assigning an
// architecture role. Files is corpus membership, not generated provenance.
type EntityFacts struct {
	Data      *facts.DataObject `json:"data,omitempty"`
	Name      string            `json:"name"`
	Extractor string            `json:"extractor"`
	Status    string            `json:"status"`
	Files     []string          `json:"files"`
}

// EdgeEvidence is the declared source of an observation, or the exact file
// behind an inventory edge. It must never masquerade as a compiler call site.
type EdgeEvidence struct {
	Extractor string `json:"extractor,omitempty"`
	Label     string `json:"label"`
	Path      string `json:"path"`
	LineNo    int    `json:"line_no"`
}

// DirectoryFacts is what the code knows about a directory that holds code
// somewhere beneath it.
type DirectoryFacts struct {
	// Readme is the first readable line of the directory's own README.
	Readme string `json:"readme,omitempty"`
	// Doc is the first sentence of the package documentation: a Go package
	// comment, a Python __init__ docstring, a package.json description.
	Doc string `json:"doc,omitempty"`
	// Dirs and Files are the direct children by name, code-bearing only.
	Dirs  []string `json:"dirs"`
	Files []string `json:"files"`
	// FileCount counts code files beneath this directory, recursively.
	FileCount int `json:"file_count"`
	// TopBox marks the shallowest code directories: the first directory with
	// files on the way down from the repository root. Zones are assigned to
	// top boxes and inherited below them.
	TopBox bool `json:"top_box"`
}

// Decl is one declaration of a file as the model will see it.
type Decl struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature,omitempty"`
	// Aliases are the declaration's names in other formats, "json:count_label".
	Aliases string `json:"aliases,omitempty"`
	// Types are, for a field, where the repository types its declared type
	// names are declared (ProgramIndex Object.Types). Never sent to a model.
	Types []sourcevalue.Anchor `json:"types,omitempty"`
	// Doc is author documentation. File/callable rows use its first sentence;
	// type context retains the existing bounded quote, including later effects.
	Doc    string `json:"doc,omitempty"`
	LineNo int    `json:"line_no"`
	Column int    `json:"column,omitempty"`
	// EndLine is the declaration's last line when the adapter knows it.
	EndLine int `json:"end_line,omitempty"`
	// CodeLines counts the lines of its source range that hold code, not
	// blank, comment-only or docstring lines, as the adapter counts them; a
	// module counts its whole file. Zero is unknown.
	CodeLines int  `json:"code_lines,omitempty"`
	Exported  bool `json:"exported"`
	// Macro says the declaration is a macro, expanded where it is written:
	// its adapter records no use of it (ProgramIndex `macro`).
	Macro bool `json:"macro,omitempty"`
	// Anonymous says the declaration is a function literal written inside
	// another and named after it (ProgramIndex `anonymous`, Go's Open$1):
	// its code, not a declaration of its own. Rows state it from this fact,
	// never from a name's characters.
	Anonymous bool `json:"anonymous,omitempty"`
	// Overloads are a callable's other signatures, written before it and
	// folded into it (ProgramIndex Overload): Python's `@typing.overload`
	// stubs, TypeScript's overload signatures. Each keeps its signature,
	// line and code lines; none is a declaration of its own.
	Overloads []DeclOverload `json:"overloads,omitempty"`
	// FanIn counts distinct callers of this declaration in the graph.
	FanIn int `json:"fan_in"`
	// ObjectID keeps the program-index identity for the page's anchors,
	// qualified by its program (ScopedObjectID). It is never sent to the
	// model.
	ObjectID string `json:"object_id,omitempty"`
}

// DeclOverload is one signature a declaration is also written with.
type DeclOverload struct {
	Signature string `json:"signature,omitempty"`
	LineNo    int    `json:"line_no"`
	EndLine   int    `json:"end_line,omitempty"`
	CodeLines int    `json:"code_lines,omitempty"`
}

// ScopedObjectID is a declaration's Decl.ObjectID: its program-index object
// ID qualified by the program that indexed it ("t1.n4"), because object IDs
// repeat across programs. A GroupsIndex subject of target t1 whose ID is n4
// is the declaration whose ObjectID is ScopedObjectID("t1", "n4").
func ScopedObjectID(targetID, objectID string) string {
	if targetID == "" || objectID == "" {
		return ""
	}
	return targetID + "." + objectID
}

// FileFacts is what the code knows about a code file.
type FileFacts struct {
	// Doc is the module docstring (Python), the leading JSDoc (TS), or the Go
	// package comment when this file carries it.
	Doc   string `json:"doc,omitempty"`
	Decls []Decl `json:"decls"`
	// Callers and Callees are the other files this one is joined to by the
	// graph, as place IDs.
	Callers []string `json:"callers"`
	Callees []string `json:"callees"`
	// Generated files are indexed but never asked about.
	Generated bool `json:"generated,omitempty"`
	// Test files are the target's testing sources; what only they call is
	// testing, not the program.
	Test bool `json:"test,omitempty"`
}

// SymbolFacts is one declaration lifted to a place of its own. Generated
// callables retain their observations for context traversal but are not
// description candidates.
type SymbolFacts struct {
	Decl Decl `json:"decl"`
	// Members are declarations owned by this type in the native index. Their
	// documentation describes the type's interface, never a runtime call path.
	Members []TypeMember `json:"members,omitempty"`
	// Calls are neutral source observations, not framework classifications.
	Calls    []SymbolCall    `json:"calls,omitempty"`
	Bindings []SymbolBinding `json:"bindings,omitempty"`
	CalledBy []SymbolCaller  `json:"called_by,omitempty"`
	// Uses are the other declarations this one reads, hands over to be
	// called later, or is decorated by: the exact and alternatives `reads`,
	// `passes_callback` and `decorates` relations of the targets' program
	// indexes, whether or not a relation carries a pattern; and, for a
	// callable, the repository types its parameters carry (`takes`). They
	// are local keys, never provider prose.
	Uses []SymbolUse `json:"uses,omitempty"`
	// Fields are the fields of repository records this declaration reads
	// or writes, one per site (ProgramIndex relations with a FieldPath).
	// The readers and writers of one field are the declarations whose
	// Fields name the same type place and field. They are local keys, never
	// provider prose.
	Fields []SymbolField `json:"fields,omitempty"`
	// Candidate says the code chose this symbol as a possible key symbol of
	// its file, so a symbol row is asked about it.
	Candidate bool `json:"candidate"`
	// Rank is the symbol's place among its file's candidates, from 1.
	Rank int `json:"rank"`
	// Unreached are the targets holding this declaration whose adapter
	// proved their program never runs it (ProgramIndex `unreachable`): what
	// it calls out to or registers is not those targets'.
	Unreached []string `json:"unreached,omitempty"`
	// Rows are, for a module-level table variable, the rows its initializer
	// writes words in and stores no repository callable (ProgramIndex
	// TableRow): names the program may look what it was given up in.
	Rows []TableRow `json:"rows,omitempty"`
	// ReadAt are, for a table (Rows), each read of it by a declaration, one
	// per site (ProgramIndex `reads` relations): the reader's symbol place
	// and the site. The reader's CalledBy says who calls it. Local keys,
	// never provider prose until a question asks how a table is read.
	ReadAt []TableRead `json:"read_at,omitempty"`
	// Comparisons are the values this declaration compares with two or more
	// different words in two or more cases (ProgramIndex Comparison): a
	// switch's or match's cases, an if/elif chain on one value.
	Comparisons []Comparison `json:"comparisons,omitempty"`
	// Seeds are the targets whose execution begins at this declaration
	// (ProgramIndex target seeds): each is the entry of its program, so its
	// file's grouping gives it a row of its own.
	Seeds []string `json:"seeds,omitempty"`
}

// TableRow is one row of a table variable: its string literals in order.
type TableRow struct {
	Literals []RowLiteral `json:"literals"`
}

// TableRead is one read of a table: ReaderID is the reading declaration's
// symbol place, LineNo and Column the read as written. Form is how the site
// uses the rows (ProgramIndex's shared witness kinds): TableReadMembership
// tests whether a value is one of them, TableReadKeys reads another table,
// KeysOf (its symbol place), with each of them as the key; "" is any other
// read.
type TableRead struct {
	ReaderID string `json:"reader_id"`
	LineNo   int    `json:"line_no"`
	Column   int    `json:"column,omitempty"`
	Form     string `json:"form,omitempty"`
	KeysOf   string `json:"keys_of,omitempty"`
}

// The forms of a table read (TableRead.Form).
const (
	TableReadMembership = "membership"
	TableReadKeys       = "keys"
)

// Comparison is one value a declaration compares with several words, in
// its declaration's file (ProgramIndex Comparison): Value as written,
// Origin where its value comes from, LineNo and Column its first word, and
// its cases in source order.
type Comparison struct {
	Value  string             `json:"value"`
	Origin *sourcevalue.Value `json:"origin,omitempty"`
	LineNo int                `json:"line_no"`
	Column int                `json:"column,omitempty"`
	Cases  []ComparisonCase   `json:"cases"`
}

// ComparisonCase is one case of a comparison: Form "case" or "equals", the
// words it compares with, its first word's position, and BranchLine through
// BranchEnd, the lines it selects, when known.
type ComparisonCase struct {
	Form       string   `json:"form"`
	Words      []string `json:"words"`
	LineNo     int      `json:"line_no"`
	Column     int      `json:"column,omitempty"`
	BranchLine int      `json:"branch_line,omitempty"`
	BranchEnd  int      `json:"branch_end,omitempty"`
}

// RowLiteral is one string literal a row writes, with the field it fills.
type RowLiteral struct {
	Field  string `json:"field,omitempty"`
	Value  string `json:"value"`
	LineNo int    `json:"line_no"`
	Column int    `json:"column,omitempty"`
}

type TypeMember struct {
	Path string `json:"path"`
	Decl Decl   `json:"decl"`
}

// SymbolUse is one use of another declaration: PlaceID is its symbol place,
// Kind the ProgramIndex relation (`reads`, `passes_callback` or
// `decorates`, where the decorated declaration uses its decorator) or
// UseTakes, and Resolution `exact` or `alternatives`.
type SymbolUse struct {
	PlaceID    string `json:"place_id"`
	Kind       string `json:"kind"`
	Resolution string `json:"resolution"`
}

// UseTakes is the use of a repository type by a callable one of whose
// parameters carries it (ProgramIndex parameter `type_id`), always exact.
const UseTakes = "takes"

// SymbolField is one read or write of a record's field: TypeID is the
// record type's symbol place and Field the field's name, Path the field as
// the code reaches it (server.masterhost, redisDb.expires), Kind `reads` or
// `writes`, and LineNo and Column the field as written. Value is, on a
// write, the value the site stores when the adapter recorded it
// (ProgramIndex Relation.Value: `server.dbfilename = "dump.rdb"`).
type SymbolField struct {
	TypeID string             `json:"type_id"`
	Field  string             `json:"field"`
	Path   string             `json:"path"`
	Kind   string             `json:"kind"`
	LineNo int                `json:"line_no"`
	Column int                `json:"column,omitempty"`
	Value  *sourcevalue.Value `json:"value,omitempty"`
}

// SymbolFieldLess orders field accesses by field, then by site.
func SymbolFieldLess(left, right SymbolField) bool {
	if left.TypeID != right.TypeID {
		return placeIDLess(left.TypeID, right.TypeID)
	}
	if left.Field != right.Field {
		return left.Field < right.Field
	}
	if left.LineNo != right.LineNo {
		return left.LineNo < right.LineNo
	}
	if left.Column != right.Column {
		return left.Column < right.Column
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Path < right.Path
}

// SymbolUseLess orders uses by place, kind and resolution.
func SymbolUseLess(left, right SymbolUse) bool {
	if left.PlaceID != right.PlaceID {
		return placeIDLess(left.PlaceID, right.PlaceID)
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Resolution < right.Resolution
}

type SymbolCall struct {
	ReceiverValue *sourcevalue.Value `json:"receiver_value,omitempty"`
	ResultValue   *sourcevalue.Value `json:"result_value,omitempty"`
	API           *CallAPI           `json:"api,omitempty"`
	// SourceArguments retain value provenance for local destination traversal.
	// They are not appended wholesale to every description request.
	SourceArguments []SourceArgument `json:"source_arguments,omitempty"`
	Kind            string           `json:"kind"`
	Name            string           `json:"name"`
	Line            int              `json:"line"`
	Column          int              `json:"column,omitempty"`
	Invocation      string           `json:"invocation,omitempty"`
	Dispatch        string           `json:"dispatch,omitempty"`
	Detail          string           `json:"detail,omitempty"`
	Resolution      string           `json:"resolution,omitempty"`
	Values          []string         `json:"values,omitempty"`
	Arguments       []string         `json:"arguments,omitempty"`
	Evidence        []EdgeEvidence   `json:"evidence,omitempty"`
	// CalleeAnonymous says every callee the call names is a function
	// literal (Decl.Anonymous): code of the declaration holding it.
	CalleeAnonymous bool `json:"callee_anonymous,omitempty"`
	// CalleeIDs refer to compiler-located symbol places, shared across target
	// indexes. They are local retrieval keys and never enter provider prose.
	CalleeIDs []string `json:"callee_ids,omitempty"`
	// Stores are, for a call through a field or a name, where the code first
	// put each function the call reaches there (a command table's row): the
	// order a reader meets those functions in. Local, never provider prose.
	Stores []CallStore `json:"stores,omitempty"`
	// SameValueAs is where the earlier call this call reads the same value
	// as is written (ProgramIndex RelationPattern.SameValueAs): the first
	// spelling of one read, `query.Get("storageClass")` for the else-if
	// arm's `query.Get("storage-class")`. Local, never provider prose.
	SameValueAs *sourcevalue.Anchor `json:"same_value_as,omitempty"`
}

// CallStore is where the code first stored one function a call through a
// field or a name reaches.
type CallStore struct {
	CalleeID string `json:"callee_id"`
	Path     string `json:"path"`
	LineNo   int    `json:"line_no"`
	Column   int    `json:"column,omitempty"`
}

// CallAPI is the exact native external symbol, before display shortening.
type CallAPI struct {
	Package  string `json:"package"`
	Receiver string `json:"receiver,omitempty"`
	Name     string `json:"name"`
	// Signature is the symbol's declared type when the adapter read it.
	Signature string `json:"signature,omitempty"`
}

type SourceArgument struct {
	Position int                `json:"position,omitempty"`
	Keyword  string             `json:"keyword,omitempty"`
	Origin   *sourcevalue.Value `json:"origin,omitempty"`
}

// SymbolCaller is an observed incoming call, not a inferred registration.
type SymbolCaller struct {
	PlaceID string `json:"place_id,omitempty"`
	// ObjectID selects the actual caller declaration for local context retrieval.
	// Provider rows remove it; a matching method name is never a substitute.
	ObjectID   string `json:"object_id,omitempty"`
	Name       string `json:"name"`
	Signature  string `json:"signature,omitempty"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Kind       string `json:"kind"`
	Invocation string `json:"invocation,omitempty"`
	Dispatch   string `json:"dispatch,omitempty"`
	Resolution string `json:"resolution"`
}

// SymbolBinding describes where a callable is supplied or received. It does
// not claim that registration itself executes the callback.
type SymbolBinding struct {
	Arguments []RegistrationArgument `json:"arguments,omitempty"`
	Evidence  []EdgeEvidence         `json:"evidence,omitempty"`
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	// FromAnonymous and ToAnonymous say an end is a function literal
	// (Decl.Anonymous).
	FromAnonymous bool   `json:"from_anonymous,omitempty"`
	ToAnonymous   bool   `json:"to_anonymous,omitempty"`
	Detail        string `json:"detail"`
	Kind          string `json:"kind"`
	Resolution    string `json:"resolution"`
	Path          string `json:"path"`
	Line          int    `json:"line"`
}

// RegistrationArgument is a literal at the exact call that receives this
// callback. It is not an argument of the callback or a runtime value.
type RegistrationArgument struct {
	Position int    `json:"position,omitempty"`
	Keyword  string `json:"keyword,omitempty"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
}

// BoundaryFacts is one integration point: a call into an external symbol with
// literal arguments, a route, a listener, a configuration read.
type BoundaryFacts struct {
	// Source says where the boundary came from: "fact" or "external_call".
	Source string `json:"source"`
	// Origins retain each target's original fact and declaration identities.
	// They are local projection metadata, never model evidence.
	Origins []BoundaryOrigin `json:"origins,omitempty"`
	// ObjectID is the enclosing object; Caller its display name.
	ObjectID  string `json:"object_id,omitempty"`
	SubjectID string `json:"subject_id,omitempty"`
	Caller    string `json:"caller"`
	CallerDoc string `json:"caller_doc,omitempty"`
	// External is the external symbol as package.Receiver.Name; Method and
	// Values are the literal facts the code extracted.
	External string   `json:"external,omitempty"`
	Method   string   `json:"method,omitempty"`
	Values   []string `json:"values"`
	// Words are what the code wrote at a registration, as written: its call
	// word, its literals in order and the address its mount prefixes compose.
	// The model names an entry by choosing among them; nothing here says
	// which is a verb, a path, a command or a topic.
	Words []string `json:"words,omitempty"`
	// WordsGiven are, beside Words, the parameter the call gives each word
	// under when it names one (a keyword argument: description, alias,
	// dest), "" for a word given by position or written otherwise: the
	// reading shows it with the word it offers to name the entry by.
	WordsGiven []string `json:"words_given,omitempty"`
	// Holder is the value the call acts on, as path:line:column of the call
	// that produced it; registrations on one holder belong together.
	Holder string `json:"holder,omitempty"`
	// Handed marks a value of the repository's own handed over without a
	// named callable (Register("k6/x/dns", new(DNS))).
	Handed    bool   `json:"handed,omitempty"`
	Direction string `json:"direction"`
	// GivenKind is the kind the code already knows from facts; empty when the
	// model has to say.
	GivenKind string `json:"given_kind,omitempty"`
	// Registrar is, for a callable handed to the repository's own function
	// that keeps it for later (a registration fact's Registrar), that
	// function and what runs when the call is made. The reading asks what
	// the kept callable becomes (atlas_inputs stored); no outside symbol's
	// role decides it.
	Registrar *RegistrarFacts `json:"registrar,omitempty"`
	// Invocation is, for a registration starting the repository's own
	// callable to run on its own (a Go `go` statement, a coroutine handed
	// to asyncio.create_task), its shared invocation word: goroutine or
	// async_task. The reading asks each such statement what the started
	// callable becomes (atlas_api starts); no outside symbol's role
	// decides it.
	Invocation string `json:"invocation,omitempty"`
}

// RegistrarFacts is the repository function a callable is handed to and
// kept by, as a registration fact records it. Local evidence for the
// stored question; its names are code structure.
type RegistrarFacts struct {
	Name      string        `json:"name"`
	Path      string        `json:"path"`
	Signature string        `json:"signature,omitempty"`
	Slots     []string      `json:"slots"`
	During    []DuringFacts `json:"during,omitempty"`
}

// DuringFacts is one way a registering call is reached (facts
// RegisteredDuring).
type DuringFacts struct {
	Seed     string   `json:"seed,omitempty"`
	HandedTo string   `json:"handed_to,omitempty"`
	Handlers []string `json:"handlers,omitempty"`
	Through  []string `json:"through,omitempty"`
}

// BoundaryOrigin binds a shared source observation to an original target fact.
type BoundaryOrigin struct {
	TargetID string `json:"target_id"`
	FactID   string `json:"fact_id"`
	ObjectID string `json:"object_id,omitempty"`
}

// CanonicalBoundaryOrigins preserves each distinct original identity once.
func CanonicalBoundaryOrigins(origins []BoundaryOrigin) []BoundaryOrigin {
	result := append([]BoundaryOrigin(nil), origins...)
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if a.TargetID != b.TargetID {
			return a.TargetID < b.TargetID
		}
		if a.FactID != b.FactID {
			return a.FactID < b.FactID
		}
		return a.ObjectID < b.ObjectID
	})
	return slices.Compact(result)
}

// Witness is one call site behind an edge.
type Witness struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
	// Kind is the relation between them: calls, passes_callback,
	// binds_implementation, decorates, executes.
	Kind   string `json:"kind,omitempty"`
	Path   string `json:"path"`
	LineNo int    `json:"line_no"`
}

// Edge joins two places: file to file for calls, callbacks, decorators and
// executions; directory to directory for Go package imports.
type Edge struct {
	From      string        `json:"from"`
	To        string        `json:"to"`
	Kind      string        `json:"kind"`
	Count     int           `json:"count"`
	Witnesses []Witness     `json:"witnesses"`
	Evidence  *EdgeEvidence `json:"evidence,omitempty"`
	// Static marks an edge one program's code makes whatever program links
	// it: an import, or a direct call or execution of one exact target. A
	// call through an interface, a function value or a stored callback is
	// the linking program's own wiring, observed only by the programs in
	// Targets: etcd's rafthttp calls raftexample's raftNode.Process only
	// inside raftexample, which links rafthttp and stores its node there.
	Static  bool     `json:"static,omitempty"`
	Targets []string `json:"targets,omitempty"`
}

// Graph is places.json: every place, every edge, the seeds the file rounds
// start from.
type Graph struct {
	Version  int     `json:"version"`
	Revision string  `json:"revision"`
	Places   []Place `json:"places"`
	Edges    []Edge  `json:"edges"`
	// Seeds are the file place IDs where execution can begin, over every
	// target of the run.
	Seeds []string `json:"seeds"`
	// SeedDecls are the symbol places of the declarations execution begins
	// in, where the launch fact names one: a file's endpoint may be several
	// parts when the file's code is split between them.
	SeedDecls []string `json:"seed_decls"`
	SHA256    string   `json:"sha256"`
}

// Atlas is atlas.json: the one artifact the page reads.
type Atlas struct {
	Version    int    `json:"version"`
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	// Targets holds one entry per analyzed target of any language.
	Targets []Target `json:"targets"`
	// Joints are the integrations between targets, at repository level.
	Joints []Joint `json:"joints"`
	// API is MODEL: what the external symbols the repository calls do with
	// what it gives them. A symbol without a role is absent.
	API         []APIRole    `json:"api,omitempty"`
	Budget      Budget       `json:"budget"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	SHA256      string       `json:"sha256"`
}

// APIRole is the model's reading of one external symbol: what a callable
// handed to it becomes, whether it runs around the handlers, whether it
// publishes what its holder holds, what other running system it talks to.
// Each cell is empty when the symbol does not do that. It holds only the
// decisions the boundaries read.
type APIRole struct {
	Symbol    string `json:"symbol"`
	Binds     string `json:"binds,omitempty"`
	Publishes bool   `json:"publishes,omitempty"`
	Talks     string `json:"talks,omitempty"`
	// What the words a call to the symbol is given become is each call's
	// own answer, recorded as the entries it made and the unsure calls.
	Middleware bool `json:"middleware,omitempty"`
}

// EntryKinds are what a handed callable can become; an entry has a named
// kind or is none.
func EntryKinds() []string {
	return []string{BoundaryRequest, BoundaryCommand, BoundaryInteraction, BoundaryScheduled, BoundaryContinuous, BoundaryQueueConsumer, BoundaryExtension, BoundarySetting}
}

// Target is one analyzed program target with its boxes.
type Target struct {
	Data     []DataRecord `json:"data,omitempty"`
	ID       string       `json:"id"`
	Language string       `json:"language"`
	Kind     string       `json:"kind"`
	Name     string       `json:"name"`
	Root     string       `json:"root"`
	// Line is MODEL: the portfolio table's line for this target, or the root
	// directory's line when the run has one target.
	Line string `json:"line"`
	// Role is MODEL: product, library, fixture, tool or example.
	Role       string     `json:"role,omitempty"`
	SharedCode []string   `json:"shared_code,omitempty"`
	Zones      []Zone     `json:"zones"`
	Boxes      []Box      `json:"boxes"`
	Arrows     []Arrow    `json:"arrows"`
	Boundaries []Boundary `json:"boundaries"`
	// OffMap is every file, or declaration of a file, that no drawn part
	// holds, with why. Its files keep their lines, captions and keys here;
	// their boundaries name no box.
	OffMap []OffMapFile `json:"off_map"`
	// MapFailure says why the target has no map of parts at all, in one of
	// the closed MapFailure* words: its parts answer was refused, or no model
	// was asked. The refusals themselves are rejected rows. Every file is then
	// in OffMap with the reason map_failure. Empty when the map was drawn,
	// including the legitimate empty map of a target without code.
	MapFailure string `json:"map_failure,omitempty"`
	// Files and Symbols are the denominators the page shows; it recounts
	// nothing.
	Files   int `json:"files"`
	Symbols int `json:"symbols"`
	// Unsure are the calls outside tests that may declare an input the
	// reading could not decide: words given to an outside symbol whose
	// call's entry question was not decided, or a call giving no word that
	// can name one to a symbol whose words are an entry at another call.
	// Idioms are, by outside symbol and entry kind, how many of its
	// recorded calls made entries of that kind and in which declarations.
	// Both are the launch walk's evidence (GroupsIndex Launch); neither is
	// an input.
	Unsure []UnsureCall `json:"unsure,omitempty"`
	Idioms []Idiom      `json:"idioms,omitempty"`
}

// The reasons a call is unsure: its entry question had no decided answer,
// or none of the words it is given can name an entry.
const (
	UnsureUndecided = "undecided"
	UnsureNoWords   = "no_words"
)

// UnsureCall is one call that may declare an input and was not decided.
// Reason is "undecided" (the call's entry question had no decided answer)
// or "no_words" (none of the words the call is given can name an entry,
// as `add_argument(*opt.cli)`).
type UnsureCall struct {
	ObjectID string `json:"object_id"`
	Path     string `json:"path"`
	LineNo   int    `json:"line_no"`
	Column   int    `json:"column,omitempty"`
	Symbol   string `json:"symbol"`
	Reason   string `json:"reason"`
}

// Idiom is what one outside symbol's word calls made in a target: Entries
// of its Calls became entries of Kind, declared in ObjectIDs. MODEL: each
// call's answer made them.
type Idiom struct {
	Symbol    string   `json:"symbol"`
	Kind      string   `json:"kind"`
	Entries   int      `json:"entries"`
	Calls     int      `json:"calls"`
	ObjectIDs []string `json:"object_ids"`
}

// Zone is one area of a target: a named frame holding several parts.
type Zone struct {
	// ID is a compact atlas-local z* ordinal.
	ID string `json:"id"`
	// Title and Line are MODEL; an empty Line is the explicit state of an
	// area whose description was refused or not asked.
	Title  string   `json:"title"`
	Line   string   `json:"line"`
	BoxIDs []string `json:"box_ids"`
}

// Box is one part of a target's map: the whole files and the boxes of split
// files one parts answer grouped, with the declarations they hold.
type Box struct {
	// MemberIDs names the part's declarations: those of its files, and the
	// methods of its types declared in other files. Native lexical children
	// inherit their declaration's membership.
	MemberIDs []string `json:"member_ids"`
	// ID is a compact atlas-local p* ordinal assigned when the part is accepted.
	ID  string `json:"id"`
	Dir string `json:"dir"`
	// Title and Line are MODEL. Line is the part's description; empty is the
	// explicit no-description state (refused, not asked, or a test-only part).
	Title  string `json:"title"`
	Line   string `json:"line"`
	ZoneID string `json:"zone_id,omitempty"`
	// Side orders the columns: in, mid, out.
	Side string `json:"side"`
	// Open is MODEL under a budget; always true below the threshold.
	Open bool `json:"open"`
	// Core is MODEL: the program exists for this part. ForTests is a fact:
	// every file of the part is test code, so it stays off the canvas.
	// Unreached is a fact too: the part holds code that runs and its
	// program's adapter proved every such declaration `unreachable`, so it
	// stays off that program's canvas (redis-cli's linked list, which it
	// links and never calls).
	Core      bool   `json:"core,omitempty"`
	ForTests  bool   `json:"for_tests,omitempty"`
	Unreached bool   `json:"unreached,omitempty"`
	Files     []File `json:"files"`
}

// OffCanvas reports a part that is kept in the atlas but not drawn: one made
// only of test code, or one its program never runs.
func (box Box) OffCanvas() bool {
	return box.ForTests || box.Unreached
}

// Why a file stays off a target's map of parts.
const (
	// OffMapLeftOut: the parts answer and its placement follow-up left the
	// unit out (a whole file, or one box of a file whose code goes in
	// several boxes), or no part was drawn to place it in.
	OffMapLeftOut = "left_out"
	// OffMapConflict: the parts answer listed the unit in two parts and the
	// follow-up did not settle it.
	OffMapConflict = "conflict"
	// OffMapNoUnits: the file declares nothing a part could hold.
	OffMapNoUnits = "no_units"
	// OffMapFailure: the target has no map of parts at all.
	OffMapFailure = "map_failure"
	// OffMapUndecided: the declarations of a file whose code goes in several
	// boxes that no box of that file took. The file itself is on the map
	// through its other declarations.
	OffMapUndecided = "undecided"
	// OffMapBlocked: helpers of a file whose code goes in several boxes that
	// no box took because a declaration that uses them never got one, so
	// code could not place them and no question was asked about them.
	OffMapBlocked = "blocked"

	// MapFailureRefused: every window of the parts answer was refused.
	MapFailureRefused = "refused"
	// MapFailureNoModel: no model was asked for the parts.
	MapFailureNoModel = "no_model"
)

// ValidOffMapReason reports one of the closed off-map reasons.
func ValidOffMapReason(reason string) bool {
	switch reason {
	case OffMapLeftOut, OffMapConflict, OffMapNoUnits, OffMapFailure, OffMapUndecided, OffMapBlocked:
		return true
	default:
		return false
	}
}

// OffMapFile is one file, or the declarations of a file, that no drawn part
// holds. File carries what a box would: the file's line and the captions and
// keys of the declarations listed here.
type OffMapFile struct {
	// ID is the file's sealed-graph f* place.
	ID     string `json:"id"`
	Reason string `json:"reason"`
	// BoxID names the part that holds the file itself when only the
	// declarations listed here are off the map, such as a method whose type
	// is off the map or a box of the file left out while its other boxes
	// share one part; the file then stays on the map. Empty when no one
	// part holds the file.
	BoxID string `json:"box_id,omitempty"`
	File  File   `json:"file"`
}

// File is one code file inside a box.
type File struct {
	Path string `json:"path"`
	// Line is MODEL; Given stands in.
	Line string `json:"line"`
	// Source says where Line came from: model, cache, given.
	Source string `json:"source"`
	Open   bool   `json:"open"`
	// Asked says the row went to the model.
	Asked   bool     `json:"asked"`
	Symbols []Symbol `json:"symbols"`
	Callers int      `json:"callers"`
	Callees int      `json:"callees"`
}

// Symbol is one declaration on a card.
type Symbol struct {
	ObjectID string `json:"object_id,omitempty"`
	// ID is the existing compact s* place ID.
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Signature string `json:"signature,omitempty"`
	Doc       string `json:"doc,omitempty"`
	LineNo    int    `json:"line_no"`
	Column    int    `json:"column,omitempty"`
	// Line is MODEL for candidates; Doc, then Signature, stand in.
	Line string `json:"line,omitempty"`
	// Alias is an optional MODEL English reader label; Name stays native.
	Alias string `json:"alias,omitempty"`
	// Key is MODEL; without a symbol layer the code ranks.
	Key bool `json:"key"`
	// Helper is MODEL: the helper question decided the declaration's unit
	// serves the work of other declarations, so code placed it with its
	// users.
	Helper bool `json:"helper,omitempty"`
	// Activation and Operation are model interpretations of an exposed action.
	Activation       string `json:"activation,omitempty"`
	Operation        string `json:"operation,omitempty"`
	OperationSummary string `json:"operation_summary,omitempty"`
}

// Arrow joins two boxes of one target, caller to callee.
type Arrow struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Calls     int       `json:"calls"`
	Witnesses []Witness `json:"witnesses"`
	// Sentence is MODEL; "from calls to: a, b, c" stands in.
	Sentence string `json:"sentence"`
}

// Boundary is one integration point drawn beside its box. BoxID is empty
// when the boundary's file is off the map; the boundary is still read.
type Boundary struct {
	Uses      []DestinationUse `json:"uses,omitempty"`
	ID        string           `json:"id"`
	ObjectID  string           `json:"object_id,omitempty"`
	BoxID     string           `json:"box_id"`
	Path      string           `json:"path"`
	LineNo    int              `json:"line_no"`
	Column    int              `json:"column,omitempty"`
	Caller    string           `json:"caller"`
	Direction string           `json:"direction"`
	Kind      string           `json:"kind"`
	External  string           `json:"external,omitempty"`
	// Source preserves whether the anchor is a native fact or an interpretation.
	Source string `json:"source,omitempty"`
	// Destination and Basis are MODEL. Address is one original observed value
	// selected by a closed ref; empty means its runtime address is unknown.
	// DestinationTarget is MODEL: the target of this repository's program
	// the destination is, when the destination question chose one.
	Destination       string   `json:"destination,omitempty"`
	DestinationTarget string   `json:"destination_target,omitempty"`
	Address           string   `json:"address,omitempty"`
	Basis             string   `json:"basis,omitempty"`
	Method            string   `json:"method,omitempty"`
	Values            []string `json:"values"`
	// Name is an entry's name: the words of its registration the model
	// chose, restored as written and joined by one space in the order it
	// wrote them. Empty when no choice was accepted.
	Name string `json:"name,omitempty"`
	// Line is MODEL.
	Line   string `json:"line"`
	FactID string `json:"fact_id,omitempty"`
	// HandlerUnknown marks an entry whose handler is not established: the
	// words a call is given (an option a parser declares) or a value handed
	// over. Caller declares it; the code that acts on it is not a fact yet,
	// so it binds to no part.
	HandlerUnknown bool `json:"handler_unknown,omitempty"`
	// BranchLine and BranchEnd are, for an entry a case of a comparison
	// declares whose lines call into the program's own code, those lines
	// (ComparisonCase): the entry is handled there, by the code of the
	// comparing declaration (ObjectID) the case selects, not by all of it.
	// litestream's case "replicate" in Main.Run.
	BranchLine int `json:"branch_line,omitempty"`
	BranchEnd  int `json:"branch_end,omitempty"`
	// ProgramNotNamed marks a call that starts another program named by
	// none of its words (BoundaryRunsProgram). Its Destination, the word
	// that names the program, is then empty; both empty means the model
	// did not decide which word names it.
	ProgramNotNamed bool `json:"program_not_named,omitempty"`
	// DeclaredOn is the object an incoming boundary's call is made on: the
	// call that produced the value it acts on, followed back through the
	// outside calls that name nothing, with that call as written. A code
	// fact; nil when the value is a parameter, a variable or nothing a call
	// produced.
	DeclaredOn *DeclaredOn `json:"declared_on,omitempty"`
	// Written is an incoming boundary's registration as the code wrote it
	// at its site, folded to one line (lines.CallText): a command table's
	// row with its arity and flags ({"rpush",rpushCommand,3,
	// REDIS_CMD_BULK|REDIS_CMD_DENYOOM,NULL,1,1,1}), a registering call. A
	// code fact for the reader; no request carries it.
	Written string `json:"written,omitempty"`
	// ValueOf is, for an entry a call's words make, the entry it is a value
	// of: the calls of one function comparing elements of one value
	// (strcasecmp(argv[0],"appendfsync") then strcasecmp(argv[1],"always")
	// on the fields sdssplitlen returned) make the lowest element's words
	// entries and each other element's words values of the entry written
	// last before them. A code fact: its answer is still the model's.
	ValueOf string `json:"value_of,omitempty"`
	// Names are, for a row of a table a handler looks up by its key (a
	// value of that handler's entry, reading handed_tables.go), the row's
	// other words as written: what the key names (othello's n names
	// new-game). A code fact.
	Names []string `json:"names,omitempty"`
	// AliasOf is, for an entry a call's words make, the entry it is
	// another spelling of: its call reads the same value as that entry's
	// call (ProgramIndex SameValueAs), both answered the same kind, and
	// that call is written first (`query.Get("storage-class")` of
	// `query.Get("storageClass")`). A code fact; each call's answer is
	// still the model's.
	AliasOf string `json:"alias_of,omitempty"`
}

// DeclaredOn is the call that made the object an entry is declared on, at
// its source site, as the code wrote it.
type DeclaredOn struct {
	Path   string `json:"path"`
	LineNo int    `json:"line_no"`
	Column int    `json:"column,omitempty"`
	Text   string `json:"text,omitempty"`
}

// DestinationUse is one observed argument chain reaching a communication
// mechanism. Address may be a configuration expression rather than a host.
// A frontier records where the original source no longer resolves the value.
// Unread marks a frontier at a value its adapter could not read (sourcevalue
// kind unknown, such as C's `(struct sockaddr*)&sa`): the address is not
// established from code, and the frontier is only the expression written
// there.
type DestinationUse struct {
	Address   string            `json:"address,omitempty"`
	Frontier  string            `json:"frontier,omitempty"`
	Unread    bool              `json:"unread,omitempty"`
	Method    string            `json:"method,omitempty"`
	TargetIDs []string          `json:"target_ids,omitempty"`
	Steps     []DestinationStep `json:"steps"`
}

type DestinationStep struct {
	SubjectID string `json:"subject_id,omitempty"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Column    int    `json:"column,omitempty"`
}

// Joint is one integration between two targets.
type Joint struct {
	ID    string   `json:"id"`
	From  Endpoint `json:"from"`
	To    Endpoint `json:"to"`
	Value string   `json:"value"`
	// SourceKind distinguishes program edges from model-confirmed integration.
	SourceKind string    `json:"source_kind,omitempty"`
	Witnesses  []Witness `json:"witnesses,omitempty"`
	// Same and Label are MODEL, except where one target joins itself: the
	// code joins a program's request to what it serves itself.
	Same     bool   `json:"same"`
	Label    string `json:"label,omitempty"`
	Possible bool   `json:"possible"`
	Blind    bool   `json:"blind"`
}

// Endpoint names one side of a joint: a boundary of a target, or, for a
// joint the code derived from a call or an import across targets, a box.
type Endpoint struct {
	TargetID   string `json:"target_id"`
	BoundaryID string `json:"boundary_id,omitempty"`
	BoxID      string `json:"box_id,omitempty"`
}

// Budget says how much of the repository the model was asked about.
type Budget struct {
	Files       int        `json:"files"`
	OpenAsked   bool       `json:"open_asked"`
	DirsOpened  int        `json:"dirs_opened"`
	FilesOpened int        `json:"files_opened"`
	Stages      []StageUse `json:"stages"`
}

// StageUse is one table's account: rows, windows, how the windows ended.
type StageUse struct {
	Stage    string `json:"stage"`
	Rows     int    `json:"rows"`
	Windows  int    `json:"windows"`
	Live     int    `json:"live"`
	Cached   int    `json:"cached"`
	Reused   int    `json:"reused_rows,omitempty"`
	Rejected int    `json:"rejected"`
	Given    int    `json:"given"`
}

// Diagnostic summarizes rejected.jsonl for one stage.
type Diagnostic struct {
	Stage   string   `json:"stage"`
	Kind    string   `json:"kind"`
	Count   int      `json:"count"`
	Samples []string `json:"samples,omitempty"`
}

// DirectoryID and FileID name places by path.
func DirectoryID(path string) string { return "dir:" + cleanPath(path) }
func FileID(path string) string      { return "file:" + cleanPath(path) }

// SymbolID names a declaration place: path:line:name.
func SymbolID(path string, line int, name string) string {
	return fmt.Sprintf("sym:%s:%d:%s", cleanPath(path), line, name)
}

func cleanPath(path string) string {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	if path == "" {
		return "."
	}
	return path
}

func compactGraphPlaceIDs(graph Graph) (Graph, error) {
	raw, err := json.Marshal(graph)
	if err != nil {
		return Graph{}, fmt.Errorf("atlas: copy graph for identity sealing: %w", err)
	}
	var owned Graph
	if err := json.Unmarshal(raw, &owned); err != nil {
		return Graph{}, fmt.Errorf("atlas: restore graph identity copy: %w", err)
	}
	sort.Slice(owned.Places, func(i, j int) bool {
		left, right := owned.Places[i], owned.Places[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.LineNo != right.LineNo {
			return left.LineNo < right.LineNo
		}
		if left.Column != right.Column {
			return left.Column < right.Column
		}
		return left.ID < right.ID
	})
	remap := make(map[string]string, len(owned.Places))
	ordinals := make(map[PlaceKind]int)
	for position := range owned.Places {
		place := &owned.Places[position]
		if place.ID == "" {
			return Graph{}, fmt.Errorf("atlas: place without construction identity")
		}
		if _, duplicate := remap[place.ID]; duplicate {
			return Graph{}, fmt.Errorf("atlas: duplicate construction identity %q", place.ID)
		}
		ordinals[place.Kind]++
		remap[place.ID] = placePrefix(place.Kind) + strconv.Itoa(ordinals[place.Kind])
	}
	// Construction helpers still use source-shaped keys before the graph is
	// sealed. Resolve those keys when an already sealed graph is extended in
	// memory, without preserving them in the artifact.
	for _, place := range owned.Places {
		compact := remap[place.ID]
		switch place.Kind {
		case PlaceDirectory:
			remap[DirectoryID(place.Path)] = compact
		case PlaceFile:
			remap[FileID(place.Path)] = compact
		case PlaceSymbol:
			if place.Symbol != nil {
				remap[SymbolID(place.Path, place.LineNo, place.Symbol.Decl.Name)] = compact
			}
		}
	}
	mapID := func(id string) string {
		if id == "" {
			return ""
		}
		if compact := remap[id]; compact != "" {
			return compact
		}
		return id
	}
	mapIDs := func(ids []string) {
		for position := range ids {
			ids[position] = mapID(ids[position])
		}
		sort.Slice(ids, func(i, j int) bool { return placeIDLess(ids[i], ids[j]) })
	}
	for position := range owned.Places {
		place := &owned.Places[position]
		place.ID = mapID(place.ID)
		place.Parent = mapID(place.Parent)
		if place.File != nil {
			mapIDs(place.File.Callers)
			mapIDs(place.File.Callees)
		}
		if place.Symbol != nil {
			for call := range place.Symbol.Calls {
				mapIDs(place.Symbol.Calls[call].CalleeIDs)
				for store := range place.Symbol.Calls[call].Stores {
					place.Symbol.Calls[call].Stores[store].CalleeID = mapID(place.Symbol.Calls[call].Stores[store].CalleeID)
				}
			}
			for caller := range place.Symbol.CalledBy {
				place.Symbol.CalledBy[caller].PlaceID = mapID(place.Symbol.CalledBy[caller].PlaceID)
			}
			for use := range place.Symbol.Uses {
				place.Symbol.Uses[use].PlaceID = mapID(place.Symbol.Uses[use].PlaceID)
			}
			for read := range place.Symbol.ReadAt {
				place.Symbol.ReadAt[read].ReaderID = mapID(place.Symbol.ReadAt[read].ReaderID)
				place.Symbol.ReadAt[read].KeysOf = mapID(place.Symbol.ReadAt[read].KeysOf)
			}
			sort.Slice(place.Symbol.Uses, func(i, j int) bool { return SymbolUseLess(place.Symbol.Uses[i], place.Symbol.Uses[j]) })
			place.Symbol.Uses = slices.Compact(place.Symbol.Uses)
			for field := range place.Symbol.Fields {
				place.Symbol.Fields[field].TypeID = mapID(place.Symbol.Fields[field].TypeID)
			}
			sort.Slice(place.Symbol.Fields, func(i, j int) bool { return SymbolFieldLess(place.Symbol.Fields[i], place.Symbol.Fields[j]) })
			place.Symbol.Fields = slices.Compact(place.Symbol.Fields)
		}
		if place.Boundary != nil {
			place.Boundary.SubjectID = mapID(place.Boundary.SubjectID)
		}
	}
	for position := range owned.Edges {
		owned.Edges[position].From = mapID(owned.Edges[position].From)
		owned.Edges[position].To = mapID(owned.Edges[position].To)
	}
	mapIDs(owned.Seeds)
	mapIDs(owned.SeedDecls)
	sort.Slice(owned.Places, func(i, j int) bool { return placeIDLess(owned.Places[i].ID, owned.Places[j].ID) })
	// Equal endpoints and kinds keep the producer's deterministic evidence
	// order. An unstable sort is allowed to reshuffle those otherwise equal
	// rows, which would make the sealed artifact depend on sort internals.
	sort.SliceStable(owned.Edges, func(i, j int) bool {
		left, right := owned.Edges[i], owned.Edges[j]
		if left.From != right.From {
			return placeIDLess(left.From, right.From)
		}
		if left.To != right.To {
			return placeIDLess(left.To, right.To)
		}
		return left.Kind < right.Kind
	})
	return owned, nil
}

func placeIDLess(left, right string) bool {
	if len(left) < 2 || len(right) < 2 || left[0] != right[0] {
		return left < right
	}
	leftOrdinal, leftErr := strconv.Atoi(left[1:])
	rightOrdinal, rightErr := strconv.Atoi(right[1:])
	if leftErr != nil || rightErr != nil {
		return left < right
	}
	return leftOrdinal < rightOrdinal
}

func validPlaceID(id string, kind PlaceKind) bool {
	prefix := placePrefix(kind)
	if !strings.HasPrefix(id, prefix) || len(id) == len(prefix) {
		return false
	}
	ordinal, err := strconv.Atoi(id[len(prefix):])
	return err == nil && ordinal > 0 && prefix+strconv.Itoa(ordinal) == id
}

// Slug turns a title into a stable identifier: lowercase words joined by
// hyphens, nothing else.
func Slug(title string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r > 127 && !strings.ContainsRune(" \t\n-_/.,:;()[]{}\"'", r):
			out.WriteRune(r)
			dash = false
		default:
			if !dash && out.Len() > 0 {
				out.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.TrimSuffix(out.String(), "-")
}

// EncodeGraph seals and encodes places.json.
func EncodeGraph(graph Graph) ([]byte, error) {
	// Seal a canonical, independent copy of local native bindings. Caller
	// iteration order and duplicate observations do not change provider input.
	graph.Places = append([]Place(nil), graph.Places...)
	for i := range graph.Places {
		if graph.Places[i].Boundary != nil {
			boundary := *graph.Places[i].Boundary
			boundary.Origins = CanonicalBoundaryOrigins(boundary.Origins)
			graph.Places[i].Boundary = &boundary
		}
	}
	graph.Version = GraphVersion
	graph.SHA256 = ""
	var err error
	graph, err = compactGraphPlaceIDs(graph)
	if err != nil {
		return nil, err
	}
	if err := validateGraph(graph); err != nil {
		return nil, err
	}
	digest, err := digestOf("repomap-atlas-graph-v3\x00", graph)
	if err != nil {
		return nil, err
	}
	graph.SHA256 = digest
	return json.Marshal(graph)
}

// DecodeGraph reads places.json and checks its seal.
func DecodeGraph(encoded []byte) (Graph, error) {
	var graph Graph
	if err := json.Unmarshal(encoded, &graph); err != nil {
		return Graph{}, fmt.Errorf("atlas: decode %s: %w", GraphFilename, err)
	}
	if graph.Version != GraphVersion {
		return Graph{}, fmt.Errorf("atlas: %s version %d, want %d", GraphFilename, graph.Version, GraphVersion)
	}
	sealed := graph.SHA256
	graph.SHA256 = ""
	digest, err := digestOf("repomap-atlas-graph-v3\x00", graph)
	if err != nil {
		return Graph{}, err
	}
	if digest != sealed {
		return Graph{}, fmt.Errorf("atlas: %s digest mismatch", GraphFilename)
	}
	graph.SHA256 = sealed
	if err := validateGraph(graph); err != nil {
		return Graph{}, err
	}
	return graph, nil
}

// Encode seals and encodes atlas.json.
func Encode(value Atlas) ([]byte, error) {
	value.Version = Version
	value.SHA256 = ""
	if err := Validate(value); err != nil {
		return nil, err
	}
	digest, err := digestOf("repomap-atlas-v3\x00", value)
	if err != nil {
		return nil, err
	}
	value.SHA256 = digest
	return json.Marshal(value)
}

// Decode reads atlas.json and checks its seal.
func Decode(encoded []byte) (Atlas, error) {
	var value Atlas
	if err := json.Unmarshal(encoded, &value); err != nil {
		return Atlas{}, fmt.Errorf("atlas: decode %s: %w", ArtifactFilename, err)
	}
	if value.Version != Version {
		return Atlas{}, fmt.Errorf("atlas: %s version %d, want %d", ArtifactFilename, value.Version, Version)
	}
	sealed := value.SHA256
	value.SHA256 = ""
	digest, err := digestOf("repomap-atlas-v3\x00", value)
	if err != nil {
		return Atlas{}, err
	}
	if digest != sealed {
		return Atlas{}, fmt.Errorf("atlas: %s digest mismatch", ArtifactFilename)
	}
	value.SHA256 = sealed
	if err := Validate(value); err != nil {
		return Atlas{}, err
	}
	return value, nil
}

// PersistGraph writes places.json into a run directory.
func PersistGraph(runDir string, graph Graph) error {
	encoded, err := EncodeGraph(graph)
	if err != nil {
		return err
	}
	return WriteGraph(runDir, encoded)
}

// WriteGraph writes places.json from the bytes EncodeGraph returned, for a
// caller that hands the same sealed bytes on instead of sealing twice.
func WriteGraph(runDir string, encoded []byte) error {
	return writeFile(filepath.Join(runDir, GraphFilename), encoded)
}

// ReadGraph reads places.json from a run directory.
func ReadGraph(runDir string) (Graph, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, GraphFilename))
	if err != nil {
		return Graph{}, fmt.Errorf("atlas: read %s: %w", GraphFilename, err)
	}
	return DecodeGraph(raw)
}

// Persist writes atlas.json into a run directory.
func Persist(runDir string, value Atlas) error {
	encoded, err := Encode(value)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(runDir, ArtifactFilename), encoded)
}

// Read reads atlas.json from a run directory.
func Read(runDir string) (Atlas, error) {
	raw, err := os.ReadFile(filepath.Join(runDir, ArtifactFilename))
	if err != nil {
		return Atlas{}, fmt.Errorf("atlas: read %s: %w", ArtifactFilename, err)
	}
	return Decode(raw)
}

func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("atlas: write %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("atlas: publish %s: %w", filepath.Base(path), err)
	}
	return nil
}

func digestOf(domain string, value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("atlas: encode for digest: %w", err)
	}
	hasher := sha256.New()
	hasher.Write([]byte(domain))
	hasher.Write(encoded)
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func validateGraph(graph Graph) error {
	if graph.Version != GraphVersion {
		return fmt.Errorf("atlas: graph version %d, want %d", graph.Version, GraphVersion)
	}
	seen := make(map[string]PlaceKind, len(graph.Places))
	for position, place := range graph.Places {
		if !place.Kind.Valid() {
			return fmt.Errorf("atlas: place %q has kind %q", place.ID, place.Kind)
		}
		if !validPlaceID(place.ID, place.Kind) {
			return fmt.Errorf("atlas: place %q does not name its kind", place.ID)
		}
		if strings.HasPrefix(place.Path, "/") || strings.Contains(place.Path, "\\") {
			return fmt.Errorf("atlas: place %q has an absolute or unnormalized path", place.ID)
		}
		if _, dup := seen[place.ID]; dup {
			return fmt.Errorf("atlas: place %q appears twice", place.ID)
		}
		if position > 0 && !placeIDLess(graph.Places[position-1].ID, place.ID) {
			return fmt.Errorf("atlas: places are not sorted at %q", place.ID)
		}
		seen[place.ID] = place.Kind
		if boundary := place.Boundary; boundary != nil && len(boundary.Origins) > 0 {
			owned := make(map[string]BoundaryOrigin, len(boundary.Origins))
			for _, origin := range boundary.Origins {
				if boundary.Source != "fact" || origin.TargetID == "" || origin.FactID == "" || !slices.Contains(place.TargetIDs, origin.TargetID) {
					return fmt.Errorf("atlas: boundary %q has an invalid native origin", place.ID)
				}
				if previous, exists := owned[origin.TargetID]; exists && previous != origin {
					return fmt.Errorf("atlas: boundary %q has conflicting native origins for target %q", place.ID, origin.TargetID)
				}
				owned[origin.TargetID] = origin
			}
			for _, target := range place.TargetIDs {
				if _, exists := owned[target]; !exists {
					return fmt.Errorf("atlas: boundary %q lacks the native origin for target %q", place.ID, target)
				}
			}
		}
		if place.Symbol != nil {
			for _, target := range place.Symbol.Unreached {
				if !slices.Contains(place.TargetIDs, target) {
					return fmt.Errorf("atlas: symbol %q is unreached in target %q, which does not hold it", place.ID, target)
				}
			}
			for _, target := range place.Symbol.Seeds {
				if !slices.Contains(place.TargetIDs, target) {
					return fmt.Errorf("atlas: symbol %q is the seed of target %q, which does not hold it", place.ID, target)
				}
			}
		}
		if place.Entity != nil {
			if err := place.Entity.Data.Validate(); err != nil {
				return err
			}
		}
		if place.Kind == PlaceEntity && (place.Entity == nil || place.Entity.Extractor == "" || place.Entity.Files == nil) {
			return fmt.Errorf("atlas: entity %q lacks its observation", place.ID)
		}
		if place.Kind == PlaceDocument {
			if place.Document == nil || place.LineNo < 1 || place.Document.EndLine < place.LineNo || place.Document.Text == "" || place.Document.Title == "" {
				return fmt.Errorf("atlas: document %q lacks its source section", place.ID)
			}
		}
		if place.Kind == PlaceSourceFact {
			fact := place.SourceFact
			if fact == nil || (fact.Kind != "entrypoint" && fact.Kind != "manifest") || fact.Key == "" || place.Path == "" || place.LineNo < 1 || fact.Language == "" || fact.Component == "" || fact.ComponentKind == "" || fact.Root == "" || len(place.TargetIDs) != 1 {
				return fmt.Errorf("atlas: source fact %q lacks its observation or component", place.ID)
			}
			if strings.HasPrefix(fact.Root, "/") || strings.Contains(fact.Root, "\\") || invalidText(fact.Root) || invalidText(fact.Name) || invalidText(fact.Key) || invalidText(fact.Value) {
				return fmt.Errorf("atlas: source fact %q has invalid source text", place.ID)
			}
		}
		if !invalidText(place.Given) {
			continue
		}
		return fmt.Errorf("atlas: place %q has control characters in its line", place.ID)
	}
	for _, place := range graph.Places {
		if place.Parent != "" {
			if _, ok := seen[place.Parent]; !ok {
				return fmt.Errorf("atlas: place %q has unknown parent %q", place.ID, place.Parent)
			}
		}
		if place.Symbol != nil {
			if len(place.Symbol.Members) > 0 && place.Symbol.Decl.Kind != "type" {
				return fmt.Errorf("atlas: non-type %q has owned type declarations", place.ID)
			}
			for _, member := range place.Symbol.Members {
				if member.Path == "" || strings.HasPrefix(member.Path, "/") || strings.Contains(member.Path, "\\") || invalidText(member.Path) || member.Decl.LineNo < 1 || member.Decl.Name == "" || invalidText(member.Decl.Name) || invalidText(member.Decl.Doc) || invalidText(member.Decl.Signature) {
					return fmt.Errorf("atlas: type %q has an invalid member declaration", place.ID)
				}
			}
			for _, call := range place.Symbol.Calls {
				for _, id := range call.CalleeIDs {
					if seen[id] != PlaceSymbol {
						return fmt.Errorf("atlas: symbol %q calls unknown symbol place %q", place.ID, id)
					}
				}
				for _, store := range call.Stores {
					if !slices.Contains(call.CalleeIDs, store.CalleeID) || store.Path == "" || store.LineNo < 1 {
						return fmt.Errorf("atlas: symbol %q stores %q, which its call does not reach", place.ID, store.CalleeID)
					}
				}
			}
			for _, caller := range place.Symbol.CalledBy {
				if caller.PlaceID != "" && seen[caller.PlaceID] != PlaceSymbol {
					return fmt.Errorf("atlas: symbol %q has unknown caller place %q", place.ID, caller.PlaceID)
				}
			}
			for _, use := range place.Symbol.Uses {
				if seen[use.PlaceID] != PlaceSymbol || use.Kind == "" || use.Resolution == "" {
					return fmt.Errorf("atlas: symbol %q uses unknown symbol place %q", place.ID, use.PlaceID)
				}
			}
			for _, field := range place.Symbol.Fields {
				if seen[field.TypeID] != PlaceSymbol || field.Field == "" || field.Path == "" || field.LineNo < 1 ||
					field.Kind != "reads" && field.Kind != "writes" || field.Value != nil && (field.Kind != "writes" || sourcevalue.Validate(field.Value) != nil) {
					return fmt.Errorf("atlas: symbol %q has an invalid field access of %q", place.ID, field.Path)
				}
			}
		}
	}
	for _, edge := range graph.Edges {
		if _, ok := seen[edge.From]; !ok {
			return fmt.Errorf("atlas: edge from unknown place %q", edge.From)
		}
		if _, ok := seen[edge.To]; !ok {
			return fmt.Errorf("atlas: edge to unknown place %q", edge.To)
		}
		if edge.From == edge.To && edge.Kind != "observation" {
			return fmt.Errorf("atlas: edge from %q to itself", edge.From)
		}
		if edge.Count < 1 || edge.Kind == "" {
			return fmt.Errorf("atlas: edge %q -> %q is empty", edge.From, edge.To)
		}
		if edge.Kind == "observation" || edge.Kind == "inventory" || edge.Kind == "data_source" {
			e := edge.Evidence
			if e == nil || e.Path == "" || e.LineNo < 1 || e.Label == "" || len(edge.Witnesses) != 0 || seen[edge.From] != PlaceEntity {
				return fmt.Errorf("atlas: edge %q -> %q lacks source evidence", edge.From, edge.To)
			}
			if edge.Kind == "data_source" && (seen[edge.To] != PlaceSymbol || e.Extractor == "") {
				return fmt.Errorf("atlas: data source edge has incompatible endpoint")
			}
			if edge.Kind == "observation" && (seen[edge.To] != PlaceEntity || e.Extractor == "") || edge.Kind == "inventory" && seen[edge.To] != PlaceFile {
				return fmt.Errorf("atlas: edge %q -> %q has incompatible endpoints", edge.From, edge.To)
			}
		} else if edge.Evidence != nil {
			return fmt.Errorf("atlas: call/import edge carries observation evidence")
		}
	}
	for _, seed := range graph.Seeds {
		if kind, ok := seen[seed]; !ok || kind != PlaceFile {
			return fmt.Errorf("atlas: seed %q is not a file place", seed)
		}
	}
	for _, seed := range graph.SeedDecls {
		if kind, ok := seen[seed]; !ok || kind != PlaceSymbol {
			return fmt.Errorf("atlas: seed declaration %q is not a symbol place", seed)
		}
	}
	return nil
}

func placePrefix(kind PlaceKind) string {
	switch kind {
	case PlaceDirectory:
		return "d"
	case PlaceFile:
		return "f"
	case PlaceSymbol:
		return "s"
	case PlaceEntity:
		return "y"
	case PlaceDocument:
		return "m"
	case PlaceSourceFact:
		return "a"
	default:
		return "b"
	}
}

// Validate checks the atlas shape: unique resolvable IDs, closed kinds and
// directions, relative paths, text without control characters.
func Validate(value Atlas) error {
	if value.Version != Version {
		return fmt.Errorf("atlas: version %d, want %d", value.Version, Version)
	}
	if value.Targets == nil || value.Joints == nil || value.Diagnostics == nil {
		return fmt.Errorf("atlas: collections are missing")
	}
	targets := make(map[string]struct{}, len(value.Targets))
	boundaries := make(map[string]map[string]struct{})
	boxesOf := make(map[string]map[string]struct{})
	for _, target := range value.Targets {
		if target.ID == "" {
			return fmt.Errorf("atlas: target without id")
		}
		if _, dup := targets[target.ID]; dup {
			return fmt.Errorf("atlas: target %q appears twice", target.ID)
		}
		targets[target.ID] = struct{}{}
		if err := ValidateDataRecords(target.Data); err != nil {
			return err
		}
		if invalidText(target.Line) || invalidText(target.Root) {
			return fmt.Errorf("atlas: target %q has invalid text", target.ID)
		}
		if target.Role != "" && !ValidRole(target.Role) {
			return fmt.Errorf("atlas: target %q has role %q", target.ID, target.Role)
		}
		if target.Zones == nil || target.Boxes == nil || target.Arrows == nil || target.Boundaries == nil {
			return fmt.Errorf("atlas: target %q is missing collections", target.ID)
		}
		if target.MapFailure != "" && target.MapFailure != MapFailureRefused && target.MapFailure != MapFailureNoModel {
			return fmt.Errorf("atlas: target %q has an invalid map failure", target.ID)
		}
		boxes := make(map[string]struct{}, len(target.Boxes))
		members := make(map[string]struct{})
		for _, box := range target.Boxes {
			if box.ID == "" || box.Dir == "" {
				return fmt.Errorf("atlas: target %q has a box without identity", target.ID)
			}
			if _, dup := boxes[box.ID]; dup {
				return fmt.Errorf("atlas: box %q appears twice", box.ID)
			}
			boxes[box.ID] = struct{}{}
			if !validSide(box.Side) {
				return fmt.Errorf("atlas: box %q has side %q", box.ID, box.Side)
			}
			if invalidText(box.Title) || invalidText(box.Line) || strings.TrimSpace(box.Title) == "" {
				return fmt.Errorf("atlas: box %q has an invalid title or line", box.ID)
			}
			if box.Files == nil {
				return fmt.Errorf("atlas: box %q is missing collections", box.ID)
			}
			for _, file := range box.Files {
				if strings.HasPrefix(file.Path, "/") || file.Path == "" {
					return fmt.Errorf("atlas: box %q holds a file with path %q", box.ID, file.Path)
				}
				if invalidText(file.Line) || !validSource(file.Source) {
					return fmt.Errorf("atlas: file %q has an invalid line or source", file.Path)
				}
				if file.Symbols == nil {
					return fmt.Errorf("atlas: file %q is missing symbols", file.Path)
				}
				for _, symbol := range file.Symbols {
					if _, dup := members[symbol.ID]; dup {
						return fmt.Errorf("atlas: declaration %q is in two boxes of target %q", symbol.ID, target.ID)
					}
					members[symbol.ID] = struct{}{}
					if symbol.ID == "" || symbol.Name == "" || invalidText(symbol.Line) || invalidText(symbol.Doc) {
						return fmt.Errorf("atlas: file %q has an invalid symbol", file.Path)
					}
				}
			}
		}
		for _, entry := range target.OffMap {
			if entry.ID == "" || !ValidOffMapReason(entry.Reason) || entry.File.Path == "" || strings.HasPrefix(entry.File.Path, "/") {
				return fmt.Errorf("atlas: target %q has an invalid off-map file %q", target.ID, entry.ID)
			}
			if (target.MapFailure != "") != (entry.Reason == OffMapFailure) {
				return fmt.Errorf("atlas: target %q: off-map file %q disagrees with the map failure", target.ID, entry.File.Path)
			}
			if _, known := boxes[entry.BoxID]; entry.BoxID != "" && (!known || len(entry.File.Symbols) == 0) {
				return fmt.Errorf("atlas: off-map declarations of %q name part %q without being declarations of a file it holds", entry.File.Path, entry.BoxID)
			}
			if invalidText(entry.File.Line) || !validSource(entry.File.Source) || entry.File.Symbols == nil {
				return fmt.Errorf("atlas: off-map file %q has an invalid line, source or symbols", entry.File.Path)
			}
			for _, symbol := range entry.File.Symbols {
				if _, dup := members[symbol.ID]; dup {
					return fmt.Errorf("atlas: declaration %q is both on and off the map of target %q", symbol.ID, target.ID)
				}
				members[symbol.ID] = struct{}{}
				if symbol.ID == "" || symbol.Name == "" || invalidText(symbol.Line) || invalidText(symbol.Doc) {
					return fmt.Errorf("atlas: off-map file %q has an invalid symbol", entry.File.Path)
				}
			}
		}
		zones := make(map[string]struct{}, len(target.Zones))
		for _, zone := range target.Zones {
			if zone.ID == "" || strings.TrimSpace(zone.Title) == "" || invalidText(zone.Title) || invalidText(zone.Line) {
				return fmt.Errorf("atlas: target %q has an invalid zone", target.ID)
			}
			if _, dup := zones[zone.ID]; dup {
				return fmt.Errorf("atlas: zone %q appears twice", zone.ID)
			}
			zones[zone.ID] = struct{}{}
			for _, boxID := range zone.BoxIDs {
				if _, ok := boxes[boxID]; !ok {
					return fmt.Errorf("atlas: zone %q names unknown box %q", zone.ID, boxID)
				}
			}
		}
		for _, box := range target.Boxes {
			if box.ZoneID != "" {
				if _, ok := zones[box.ZoneID]; !ok {
					return fmt.Errorf("atlas: box %q names unknown zone %q", box.ID, box.ZoneID)
				}
			}
		}
		arrowIDs := make(map[string]bool, len(target.Arrows))
		for _, arrow := range target.Arrows {
			if arrow.ID == "" || arrowIDs[arrow.ID] {
				return fmt.Errorf("atlas: target %q has an empty or duplicate arrow id %q", target.ID, arrow.ID)
			}
			arrowIDs[arrow.ID] = true
			if _, ok := boxes[arrow.From]; !ok {
				return fmt.Errorf("atlas: arrow from unknown box %q", arrow.From)
			}
			if _, ok := boxes[arrow.To]; !ok {
				return fmt.Errorf("atlas: arrow to unknown box %q", arrow.To)
			}
			if arrow.From == arrow.To || arrow.Calls < 1 || invalidText(arrow.Sentence) {
				return fmt.Errorf("atlas: arrow %q -> %q is invalid", arrow.From, arrow.To)
			}
		}
		owned := make(map[string]struct{}, len(target.Boundaries))
		for _, boundary := range target.Boundaries {
			if boundary.ID == "" {
				return fmt.Errorf("atlas: target %q has a boundary without id", target.ID)
			}
			if _, dup := owned[boundary.ID]; dup {
				return fmt.Errorf("atlas: boundary %q appears twice", boundary.ID)
			}
			owned[boundary.ID] = struct{}{}
			if _, ok := boxes[boundary.BoxID]; !ok && boundary.BoxID != "" {
				return fmt.Errorf("atlas: boundary %q names unknown box %q", boundary.ID, boundary.BoxID)
			}
			if !ValidDirection(boundary.Direction) || !ValidBoundaryKind(boundary.Kind) {
				return fmt.Errorf("atlas: boundary %q has direction %q kind %q", boundary.ID, boundary.Direction, boundary.Kind)
			}
			if invalidText(boundary.Destination) || (boundary.Basis != "" && boundary.Basis != "dispatch" && boundary.Basis != "configuration") {
				return fmt.Errorf("atlas: boundary %q has invalid runtime relationship", boundary.ID)
			}
			if boundary.Source != "" && boundary.Source != "fact" && boundary.Source != "model" && boundary.Source != "external_call" {
				return fmt.Errorf("atlas: boundary %q has invalid source %q", boundary.ID, boundary.Source)
			}
			if invalidText(strings.ReplaceAll(strings.ReplaceAll(boundary.Line, "\n", ""), "\r", "")) || boundary.Values == nil {
				return fmt.Errorf("atlas: boundary %q is invalid", boundary.ID)
			}
			if on := boundary.DeclaredOn; on != nil && (on.Path == "" || on.LineNo < 1 || on.Column < 0 || on.Text != "" && !ValidName(on.Text)) {
				return fmt.Errorf("atlas: boundary %q is declared on an invalid site", boundary.ID)
			}
			if boundary.Written != "" && (boundary.Direction != DirectionIn || !ValidName(boundary.Written)) {
				return fmt.Errorf("atlas: boundary %q has an invalid registration as written", boundary.ID)
			}
			if boundary.ValueOf != "" && (!boundary.HandlerUnknown || boundary.ValueOf == boundary.ID) {
				return fmt.Errorf("atlas: boundary %q is a value of an invalid entry", boundary.ID)
			}
			for _, name := range boundary.Names {
				if boundary.ValueOf == "" || !ValidName(name) {
					return fmt.Errorf("atlas: boundary %q names an invalid word", boundary.ID)
				}
			}
		}
		// An alias names an entry of the same kind whose handler is not
		// established either, and which is no alias itself.
		spelled := make(map[string]Boundary, len(target.Boundaries))
		for _, boundary := range target.Boundaries {
			spelled[boundary.ID] = boundary
		}
		for _, boundary := range target.Boundaries {
			if boundary.AliasOf == "" {
				continue
			}
			first, ok := spelled[boundary.AliasOf]
			if !ok || !boundary.HandlerUnknown || boundary.AliasOf == boundary.ID || !first.HandlerUnknown || first.AliasOf != "" || first.Kind != boundary.Kind || first.Direction != DirectionIn || boundary.Direction != DirectionIn {
				return fmt.Errorf("atlas: boundary %q is another spelling of an invalid entry %q", boundary.ID, boundary.AliasOf)
			}
		}
		for _, call := range target.Unsure {
			if call.Path == "" || call.LineNo < 1 || call.Column < 0 || !ValidName(call.Symbol) || call.Reason != UnsureUndecided && call.Reason != UnsureNoWords {
				return fmt.Errorf("atlas: target %q has an invalid unsure call at %s:%d", target.ID, call.Path, call.LineNo)
			}
		}
		for _, idiom := range target.Idioms {
			if !ValidName(idiom.Symbol) || !slices.Contains(EntryKinds(), idiom.Kind) || idiom.Entries < 1 || idiom.Calls < idiom.Entries {
				return fmt.Errorf("atlas: target %q has an invalid idiom %q", target.ID, idiom.Symbol)
			}
		}
		boundaries[target.ID] = owned
		boxesOf[target.ID] = boxes
	}
	roles := make(map[string]string, len(value.Targets))
	for _, target := range value.Targets {
		roles[target.ID] = target.Role
	}
	for _, target := range value.Targets {
		seenShared := make(map[string]bool)
		for _, id := range target.SharedCode {
			if id == target.ID || seenShared[id] || roles[id] != RoleSharedCode {
				return fmt.Errorf("atlas: target %q cites invalid shared code %q", target.ID, id)
			}
			seenShared[id] = true
		}
	}
	for _, joint := range value.Joints {
		for _, endpoint := range []Endpoint{joint.From, joint.To} {
			owned, ok := boundaries[endpoint.TargetID]
			if !ok {
				return fmt.Errorf("atlas: joint %q names unknown target %q", joint.ID, endpoint.TargetID)
			}
			switch {
			case endpoint.BoundaryID != "" && endpoint.BoxID == "":
				if _, ok := owned[endpoint.BoundaryID]; !ok {
					return fmt.Errorf("atlas: joint %q names unknown boundary %q", joint.ID, endpoint.BoundaryID)
				}
			case endpoint.BoxID != "" && endpoint.BoundaryID == "":
				if _, ok := boxesOf[endpoint.TargetID][endpoint.BoxID]; !ok {
					return fmt.Errorf("atlas: joint %q names unknown box %q", joint.ID, endpoint.BoxID)
				}
			default:
				return fmt.Errorf("atlas: joint %q endpoint names neither a boundary nor a box", joint.ID)
			}
		}
		// One target joins itself only where its request reaches a route or
		// an address it serves itself (reading self_joints.go).
		if joint.From.TargetID == joint.To.TargetID && !(joint.SourceKind == "integration" && joint.From.BoundaryID != "" && joint.To.BoundaryID != "") {
			return fmt.Errorf("atlas: joint %q joins one target to itself", joint.ID)
		}
		if invalidText(joint.Label) || invalidText(joint.Value) {
			return fmt.Errorf("atlas: joint %q has invalid text", joint.ID)
		}
	}
	return nil
}

// Roles, sides, directions and boundary kinds are closed lists.
const (
	RoleProduct    = "product"
	RoleLibrary    = "library"
	RoleFixture    = "fixture"
	RoleTool       = "tool"
	RoleExample    = "example"
	RoleSharedCode = "shared_code"

	SideIn  = "in"
	SideMid = "mid"
	SideOut = "out"

	DirectionIn  = "in"
	DirectionOut = "out"

	// BoundaryClientRequest is what a program sends to another running
	// program over a connection, whatever the protocol: an HTTP request, an
	// RPC, a command on a raw socket, a message on a WebSocket. It is the
	// outgoing side of BoundaryRequest and, like it, claims no protocol.
	BoundaryClientRequest = "client_request"
	// BoundaryRequest is what a client sends over a connection, whatever the
	// protocol: an HTTP route, an RPC method, a protocol command, an event a
	// socket receives. The entry's name is the words the model chooses among
	// those its registration wrote, never a protocol's own shape.
	BoundaryRequest = "request"
	// BoundaryListenAddress is a fixed source fact, never a model kind choice.
	BoundaryListenAddress = "listen_address"
	BoundaryDB            = "db"
	BoundaryQueueProducer = "queue_producer"
	BoundaryQueueConsumer = "queue_consumer"
	BoundarySDK           = "sdk"
	// BoundaryRunsProgram is another program this one starts as a separate
	// process, at the call that names it with its arguments. The program is
	// the word the model chose among that call's words, as written, or none
	// when the program comes from a value the code computes.
	BoundaryRunsProgram = "runs_program"
	BoundaryConfig      = "config"
	// BoundaryScheduled is work a timer or scheduler activates; BoundaryInteraction
	// a user's action in an interface; BoundaryExtension a hook the host
	// program registers with a runtime or plugin system.
	BoundaryScheduled   = "scheduled"
	BoundaryInteraction = "interaction"
	BoundaryExtension   = "extension"
	// BoundaryContinuous is work that runs for as long as the program does,
	// on a thread, task or loop of its own.
	BoundaryContinuous = "continuous"
	// BoundaryCommand is a command a runner activates: a CLI subcommand, a
	// task a task runner names.
	BoundaryCommand = "command"
	// BoundarySetting is what a person writes in the program's own
	// configuration file: a directive or key the program looks for in a
	// configuration file it reads, a key a configuration structure maps from
	// a file, a configuration entry declared with a settings facility. An
	// environment variable is a configuration read (BoundaryConfig), not a
	// setting.
	BoundarySetting = "setting"
	BoundaryOther   = "other"

	SourceModel  = "model"
	SourceCache  = "cache"
	SourceGiven  = "given"
	SourceUnused = "unasked"
)

// Roles lists the target roles in the order the model sees them.
func Roles() []string {
	return []string{RoleProduct, RoleLibrary, RoleFixture, RoleTool, RoleExample, RoleSharedCode}
}

// BoundaryKinds lists the boundary kinds in the order the model sees them.
func BoundaryKinds() []string {
	return []string{
		BoundaryClientRequest, BoundaryRequest, BoundaryDB, BoundaryQueueProducer,
		BoundaryQueueConsumer, BoundaryScheduled, BoundaryContinuous, BoundaryInteraction, BoundaryExtension,
		BoundaryCommand, BoundarySetting, BoundarySDK, BoundaryRunsProgram, BoundaryConfig, BoundaryOther,
	}
}

// IncomingBoundaryKinds lists what a registration handing over a repository
// callable can be: the ways work enters the component.
func IncomingBoundaryKinds() []string {
	return append(EntryKinds(), BoundaryOther)
}

// OutgoingBoundaryKinds lists the kinds an outgoing candidate may take: the
// communication kinds the group index keeps. A route or a configuration
// read is never the kind of a call this component makes.
func OutgoingBoundaryKinds() []string {
	return []string{BoundaryClientRequest, BoundaryDB, BoundaryQueueProducer, BoundaryQueueConsumer, BoundarySDK, BoundaryRunsProgram, BoundaryOther}
}

func ValidRole(role string) bool {
	for _, known := range Roles() {
		if role == known {
			return true
		}
	}
	return false
}

func ValidBoundaryKind(kind string) bool {
	if kind == BoundaryListenAddress {
		return true
	}
	for _, known := range BoundaryKinds() {
		if kind == known {
			return true
		}
	}
	return false
}

func ValidDirection(direction string) bool {
	return direction == DirectionIn || direction == DirectionOut
}

func validSide(side string) bool {
	return side == SideIn || side == SideMid || side == SideOut
}

func validSource(source string) bool {
	switch source {
	case SourceModel, SourceCache, SourceGiven, SourceUnused:
		return true
	default:
		return false
	}
}

// ValidName is a name a reader sees as written: not empty, no space around
// it, valid UTF-8 and no control character.
func ValidName(text string) bool {
	if text == "" || text != strings.TrimSpace(text) || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func invalidText(text string) bool {
	for _, r := range text {
		if r < 0x20 && r != '\t' || r == 0x7f {
			return true
		}
	}
	return false
}

// SortPlaces orders places by ID so the artifact is canonical.
func SortPlaces(places []Place) {
	sort.Slice(places, func(i, j int) bool { return places[i].ID < places[j].ID })
}
