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
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/sourcevalue"
)

const (
	// GraphVersion and Version change when the shape of the artifacts
	// changes; an artifact of another version is refused, never patched.
	GraphVersion = 14
	Version      = 7

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

// Place is one thing a reader can be asked about. ID is "dir:<path>",
// "file:<path>", "sym:<path>:<line>:<name>" or "bnd:<path>:<line>". A path
// is repository-relative and slash-separated; one place per path, however
// many targets reach it.
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
	// Doc is author documentation. File/callable rows use its first sentence;
	// type context retains the existing bounded quote, including later effects.
	Doc      string `json:"doc,omitempty"`
	LineNo   int    `json:"line_no"`
	Column   int    `json:"column,omitempty"`
	Exported bool   `json:"exported"`
	// FanIn counts distinct callers of this declaration in the graph.
	FanIn int `json:"fan_in"`
	// ObjectID keeps the program-index identity for the page's anchors. It is
	// never sent to the model.
	ObjectID string `json:"object_id,omitempty"`
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
	// Candidate says the code chose this symbol as a possible key symbol of
	// its file, so a symbol row is asked about it.
	Candidate bool `json:"candidate"`
	// Rank is the symbol's place among its file's candidates, from 1.
	Rank int `json:"rank"`
}

type TypeMember struct {
	Path string `json:"path"`
	Decl Decl   `json:"decl"`
}

type SymbolCall struct {
	// DispatchObservations preserve the original native views when a declared
	// external interface and unresolved runtime dispatch address one call site.
	DispatchObservations []DispatchObservation `json:"dispatch_observations,omitempty"`
	ReceiverValue        *sourcevalue.Value    `json:"receiver_value,omitempty"`
	ResultValue          *sourcevalue.Value    `json:"result_value,omitempty"`
	API                  *CallAPI              `json:"api,omitempty"`
	// SourceArguments retain value provenance for local destination traversal.
	// They are not appended wholesale to every description request.
	SourceArguments []SourceArgument `json:"source_arguments,omitempty"`
	Kind            string           `json:"kind"`
	Name            string           `json:"name"`
	Line            int              `json:"line"`
	Column          int              `json:"column,omitempty"`
	Invocation      string           `json:"invocation,omitempty"`
	Detail          string           `json:"detail,omitempty"`
	Resolution      string           `json:"resolution,omitempty"`
	Values          []string         `json:"values,omitempty"`
	Arguments       []string         `json:"arguments,omitempty"`
	Evidence        []EdgeEvidence   `json:"evidence,omitempty"`
	// CalleeIDs refer to compiler-located symbol places, shared across target
	// indexes. They are local retrieval keys and never enter provider prose.
	CalleeIDs []string `json:"callee_ids,omitempty"`
}

type DispatchObservation struct {
	Kind       string            `json:"kind"`
	Invocation string            `json:"invocation"`
	Resolution string            `json:"resolution"`
	Detail     string            `json:"detail,omitempty"`
	Witnesses  []DispatchWitness `json:"witnesses,omitempty"`
}

type DispatchWitness struct {
	Kind             string `json:"kind"`
	Detail           string `json:"detail,omitempty"`
	SourceExpression string `json:"source_expression,omitempty"`
	Path             string `json:"path,omitempty"`
	Line             int    `json:"line,omitempty"`
	Column           int    `json:"column,omitempty"`
}

// CallAPI is the exact native external symbol, before display shortening.
type CallAPI struct {
	Package  string `json:"package"`
	Receiver string `json:"receiver,omitempty"`
	Name     string `json:"name"`
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
	Resolution string `json:"resolution"`
}

// SymbolBinding describes where a callable is supplied or received. It does
// not claim that registration itself executes the callback.
type SymbolBinding struct {
	Arguments  []RegistrationArgument `json:"arguments,omitempty"`
	Evidence   []EdgeEvidence         `json:"evidence,omitempty"`
	From       string                 `json:"from"`
	To         string                 `json:"to"`
	Detail     string                 `json:"detail"`
	Invocation string                 `json:"invocation"`
	Resolution string                 `json:"resolution"`
	Path       string                 `json:"path"`
	Line       int                    `json:"line"`
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
	External  string   `json:"external,omitempty"`
	Method    string   `json:"method,omitempty"`
	Values    []string `json:"values"`
	Direction string   `json:"direction"`
	// GivenKind is the kind the code already knows from facts; empty when the
	// model has to say.
	GivenKind string `json:"given_kind,omitempty"`
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
	Seeds  []string `json:"seeds"`
	SHA256 string   `json:"sha256"`
}

// Atlas is atlas.json: the one artifact the page reads.
type Atlas struct {
	Version    int    `json:"version"`
	Repository string `json:"repository"`
	Revision   string `json:"revision"`
	// Targets holds one entry per analyzed target of any language.
	Targets []Target `json:"targets"`
	// Joints are the integrations between targets, at repository level.
	Joints      []Joint      `json:"joints"`
	Budget      Budget       `json:"budget"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	SHA256      string       `json:"sha256"`
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
	// Files and Symbols are the denominators the page shows; it recounts
	// nothing.
	Files   int `json:"files"`
	Symbols int `json:"symbols"`
	// Trace is the main path: box IDs from the entrypoint forward.
	Trace []string `json:"trace"`
}

// Zone is one part of a target: a named frame holding several boxes.
type Zone struct {
	// ID is the slug of the title, so colour and anchor hold between runs.
	ID string `json:"id"`
	// Title and Line are MODEL.
	Title  string   `json:"title"`
	Line   string   `json:"line"`
	BoxIDs []string `json:"box_ids"`
}

// Box is one box on the map: a directory, or a directory split by the
// model's box choices.
type Box struct {
	// ID is the directory path, or the path plus the slug of a title the
	// model started.
	ID  string `json:"id"`
	Dir string `json:"dir"`
	// Title and Line are MODEL; the directory name and Given stand in.
	Title  string `json:"title"`
	Line   string `json:"line"`
	ZoneID string `json:"zone_id,omitempty"`
	// Side orders the columns: in, mid, out.
	Side string `json:"side"`
	// Open is MODEL under a budget; always true below the threshold.
	Open  bool   `json:"open"`
	Files []File `json:"files"`
	Keys  []Key  `json:"keys"`
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
	// ID is path:line:name.
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
	// Activation and Operation are model interpretations of an exposed action.
	Activation       string `json:"activation,omitempty"`
	Operation        string `json:"operation,omitempty"`
	OperationSummary string `json:"operation_summary,omitempty"`
}

// Key is one key symbol shown on a box.
type Key struct {
	SymbolID string `json:"symbol_id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Doc      string `json:"doc,omitempty"`
	LineNo   int    `json:"line_no"`
}

// Arrow joins two boxes of one target, caller to callee.
type Arrow struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Calls     int       `json:"calls"`
	Witnesses []Witness `json:"witnesses"`
	// Sentence is MODEL; "from calls to: a, b, c" stands in.
	Sentence string `json:"sentence"`
}

// Boundary is one integration point drawn beside its box.
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
	Destination string   `json:"destination,omitempty"`
	Address     string   `json:"address,omitempty"`
	Basis       string   `json:"basis,omitempty"`
	Method      string   `json:"method,omitempty"`
	Values      []string `json:"values"`
	// Line is MODEL.
	Line   string `json:"line"`
	FactID string `json:"fact_id,omitempty"`
}

// DestinationUse is one observed argument chain reaching a communication
// mechanism. Address may be a configuration expression rather than a host.
// A frontier records where the original source no longer resolves the value.
type DestinationUse struct {
	Address   string            `json:"address,omitempty"`
	Frontier  string            `json:"frontier,omitempty"`
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
	// Same and Label are MODEL.
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
	if err := validateGraph(graph); err != nil {
		return nil, err
	}
	digest, err := digestOf("repomap-atlas-graph-v2\x00", graph)
	if err != nil {
		return nil, err
	}
	graph.SHA256 = digest
	return json.MarshalIndent(graph, "", "  ")
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
	digest, err := digestOf("repomap-atlas-graph-v2\x00", graph)
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
	digest, err := digestOf("repomap-atlas-v2\x00", value)
	if err != nil {
		return nil, err
	}
	value.SHA256 = digest
	return json.MarshalIndent(value, "", "  ")
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
	digest, err := digestOf("repomap-atlas-v2\x00", value)
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
		if place.ID == "" || !strings.HasPrefix(place.ID, placePrefix(place.Kind)) {
			return fmt.Errorf("atlas: place %q does not name its kind", place.ID)
		}
		if strings.HasPrefix(place.Path, "/") || strings.Contains(place.Path, "\\") {
			return fmt.Errorf("atlas: place %q has an absolute or unnormalized path", place.ID)
		}
		if _, dup := seen[place.ID]; dup {
			return fmt.Errorf("atlas: place %q appears twice", place.ID)
		}
		if position > 0 && graph.Places[position-1].ID >= place.ID {
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
			}
			for _, caller := range place.Symbol.CalledBy {
				if caller.PlaceID != "" && seen[caller.PlaceID] != PlaceSymbol {
					return fmt.Errorf("atlas: symbol %q has unknown caller place %q", place.ID, caller.PlaceID)
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
	return nil
}

func placePrefix(kind PlaceKind) string {
	switch kind {
	case PlaceDirectory:
		return "dir:"
	case PlaceFile:
		return "file:"
	case PlaceSymbol:
		return "sym:"
	case PlaceEntity:
		return "entity:"
	case PlaceDocument:
		return "doc:"
	case PlaceSourceFact:
		return "fact:"
	default:
		return "bnd:"
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
		if target.Zones == nil || target.Boxes == nil || target.Arrows == nil ||
			target.Boundaries == nil || target.Trace == nil {
			return fmt.Errorf("atlas: target %q is missing collections", target.ID)
		}
		boxes := make(map[string]struct{}, len(target.Boxes))
		files := make(map[string]struct{})
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
			if box.Files == nil || box.Keys == nil {
				return fmt.Errorf("atlas: box %q is missing collections", box.ID)
			}
			for _, file := range box.Files {
				if strings.HasPrefix(file.Path, "/") || file.Path == "" {
					return fmt.Errorf("atlas: box %q holds a file with path %q", box.ID, file.Path)
				}
				if _, dup := files[file.Path]; dup {
					return fmt.Errorf("atlas: file %q is in two boxes of target %q", file.Path, target.ID)
				}
				files[file.Path] = struct{}{}
				if invalidText(file.Line) || !validSource(file.Source) {
					return fmt.Errorf("atlas: file %q has an invalid line or source", file.Path)
				}
				if file.Symbols == nil {
					return fmt.Errorf("atlas: file %q is missing symbols", file.Path)
				}
				for _, symbol := range file.Symbols {
					if symbol.ID == "" || symbol.Name == "" || invalidText(symbol.Line) || invalidText(symbol.Doc) {
						return fmt.Errorf("atlas: file %q has an invalid symbol", file.Path)
					}
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
		for _, arrow := range target.Arrows {
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
			if _, ok := boxes[boundary.BoxID]; !ok {
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
		}
		boundaries[target.ID] = owned
		boxesOf[target.ID] = boxes
		for _, boxID := range target.Trace {
			if _, ok := boxes[boxID]; !ok {
				return fmt.Errorf("atlas: trace names unknown box %q", boxID)
			}
		}
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
		if joint.From.TargetID == joint.To.TargetID {
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

	BoundaryHTTPClient = "http_client"
	BoundaryHTTPServer = "http_server"
	// BoundaryListenAddress is a fixed source fact, never a model kind choice.
	BoundaryListenAddress = "listen_address"
	BoundaryDB            = "db"
	BoundaryQueueProducer = "queue_producer"
	BoundaryQueueConsumer = "queue_consumer"
	BoundarySDK           = "sdk"
	BoundaryConfig        = "config"
	BoundaryOther         = "other"

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
		BoundaryHTTPClient, BoundaryHTTPServer, BoundaryDB, BoundaryQueueProducer,
		BoundaryQueueConsumer, BoundarySDK, BoundaryConfig, BoundaryOther,
	}
}

// OutgoingBoundaryKinds lists the kinds an outgoing candidate may take: the
// communication kinds the group index keeps. A route or a configuration
// read is never the kind of a call this component makes.
func OutgoingBoundaryKinds() []string {
	return []string{BoundaryHTTPClient, BoundaryDB, BoundaryQueueProducer, BoundaryQueueConsumer, BoundarySDK, BoundaryOther}
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
