// Package programindex owns the sealed, language-neutral handoff from a
// language adapter to the semantic domain cubes.
//
// IDs in this package are compact artifact-local identities. Provider-facing
// code uses them directly with a closed request allowlist; there is no second
// identity namespace for repository facts.
package programindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/sourcevalue"
)

const (
	Version          = 28
	ArtifactFilename = "program-index.json"

	// These exported values are advisory scale thresholds. ProgramIndex does
	// not use them as local collection caps; valid repository scale is retained
	// completely and crossing these values produces diagnostics only.
	MaxTargetSources        = 4_096
	MaxTargetSeeds          = 4_096
	MaxObjects              = 131_072
	MaxRelations            = 262_144
	MaxTargetsPerRelation   = 64
	MaxWitnessesPerRelation = 64
	MaxPatternsPerRelation  = 524_288
	MaxArgumentsPerPattern  = 128
	MaxPatternParts         = 64
	MaxObjectsPerPatternRef = 64
	MaxWitnesses            = 524_288
	MaxPatterns             = 524_288
	MaxPatternArguments     = 2_097_152
	// MaxTextBytes is advisory. Individual semantic strings remain lossless.
	MaxTextBytes = 16 * 1024
	// AdvisoryAggregateTextBytes and AdvisoryIndexBytes are the former local
	// artifact thresholds. They drive warnings only. The Max* names remain as
	// zero compatibility sentinels for readers that interpret zero as unbounded.
	AdvisoryAggregateTextBytes = 64 * 1024 * 1024
	AdvisoryIndexBytes         = 128 * 1024 * 1024
	MaxAggregateTextBytes      = 0
	MaxIndexBytes              = 0
	// MaxObservedCount is the former portable-count ceiling. It is retained as
	// an advisory warning threshold only; int is the representation authority.
	MaxObservedCount = 1<<31 - 1
)

// ObjectKind is deliberately small and language-neutral. Language adapters
// retain richer declaration kinds in their private indexes.
type ObjectKind string

const (
	ObjectModule         ObjectKind = "module"
	ObjectPackage        ObjectKind = "package"
	ObjectType           ObjectKind = "type"
	ObjectFunction       ObjectKind = "function"
	ObjectMethod         ObjectKind = "method"
	ObjectLambda         ObjectKind = "lambda"
	ObjectVariable       ObjectKind = "variable"
	ObjectExternalSymbol ObjectKind = "external_symbol"
)

func (kind ObjectKind) Valid() bool {
	switch kind {
	case ObjectModule, ObjectPackage, ObjectType, ObjectFunction, ObjectMethod,
		ObjectLambda, ObjectVariable, ObjectExternalSymbol:
		return true
	default:
		return false
	}
}

// Callable reports a declaration that runs: only a callable can be proven
// `unreachable`, and a type, variable or module runs nothing of its own.
func (kind ObjectKind) Callable() bool {
	return kind == ObjectFunction || kind == ObjectMethod || kind == ObjectLambda
}

// Visibility is the language-neutral reachability fact needed to distinguish
// public target APIs from implementation objects. Unknown is explicit when an
// adapter cannot establish that boundary.
type Visibility string

const (
	VisibilityPublic   Visibility = "public"
	VisibilityInternal Visibility = "internal"
	VisibilityUnknown  Visibility = "unknown"
)

func (visibility Visibility) Valid() bool {
	return visibility == VisibilityPublic || visibility == VisibilityInternal || visibility == VisibilityUnknown
}

// RelationKind describes structural program facts, not product semantics.
type RelationKind string

const (
	RelationCalls          RelationKind = "calls"
	RelationImports        RelationKind = "imports"
	RelationImplements     RelationKind = "implements"
	RelationDecorates      RelationKind = "decorates"
	RelationPassesCallback RelationKind = "passes_callback"
	// RelationBindsImplementation records a concrete value observed to fill an
	// interface-typed slot. Its targets are the implementation methods carried
	// by that value; nothing is invoked at the binding site.
	RelationBindsImplementation RelationKind = "binds_implementation"
	RelationSources             RelationKind = "sources"
	RelationExecutes            RelationKind = "executes"
	RelationReads               RelationKind = "reads"
	RelationWrites              RelationKind = "writes"
	RelationInvokesExternal     RelationKind = "invokes_external"
)

func (kind RelationKind) Valid() bool {
	switch kind {
	case RelationCalls, RelationImports, RelationImplements,
		RelationDecorates, RelationPassesCallback, RelationBindsImplementation, RelationSources,
		RelationExecutes, RelationReads, RelationWrites, RelationInvokesExternal:
		return true
	default:
		return false
	}
}

// Resolution is the closed amount of authority a language adapter has for a
// relation's retained target set.
type Resolution string

const (
	ResolutionExact Resolution = "exact"
	// ResolutionAlternatives retains one or more locally observed possible
	// targets without claiming that runtime dispatch is exact. The retained
	// set can contain a single syntactic candidate in a dynamic language; any
	// adapter-known omissions remain explicit in TargetsOmitted.
	ResolutionAlternatives Resolution = "alternatives"
	ResolutionUnresolved   Resolution = "unresolved"
)

func (resolution Resolution) Valid() bool {
	return resolution == ResolutionExact || resolution == ResolutionAlternatives || resolution == ResolutionUnresolved
}

// Location is an exact repository-relative source position.
type Location struct {
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// TargetSource binds one repository-corpus file identity to its exact
// repository-relative path. Keeping the pair together prevents consumers from
// guessing the ref/path association by array position or lexical order.
// LineRange is a run of whole source lines of one file, both included.
type LineRange struct {
	Line    int `json:"line"`
	EndLine int `json:"end_line"`
	// Column and EndColumn, when the adapter writes them, are where on its
	// first and last lines the range begins and ends (a comparison case's
	// branch: two cases on one line are told apart by them).
	Column    int `json:"column,omitempty"`
	EndColumn int `json:"end_column,omitempty"`
}

type TargetSource struct {
	FileRef string `json:"file_ref"`
	Path    string `json:"path"`
}

// SeedKind states the exact structural reason an object can begin execution
// for the selected target. It does not claim that the object runs in every
// process invocation.
type SeedKind string

const (
	SeedCallable    SeedKind = "callable"
	SeedModule      SeedKind = "module"
	SeedMainGuard   SeedKind = "main_guard"
	SeedScript      SeedKind = "script"
	SeedBoundObject SeedKind = "bound_object"
)

func (kind SeedKind) Valid() bool {
	switch kind {
	case SeedCallable, SeedModule, SeedMainGuard, SeedScript, SeedBoundObject:
		return true
	default:
		return false
	}
}

// ExportBasis says how an adapter knows a library's exports.
type ExportBasis string

const (
	// ExportsConsumerHeaders: C functions with external linkage declared in
	// a repository header that a program the build links with the library
	// includes from its own units.
	ExportsConsumerHeaders ExportBasis = "consumer_headers"
	// ExportsLinkage: every C function with external linkage, which a
	// program loading or linking the library may call by name; no program
	// in the repository links it.
	ExportsLinkage ExportBasis = "linkage"
	// ExportsVisibility: the language's own exported names (Go
	// capitalization outside internal packages, Python public names in
	// public modules, a JS/TS export, a Clojure public var).
	ExportsVisibility ExportBasis = "visibility"
	// ExportsEntryModules: what a JS/TS package's declared entry modules
	// export.
	ExportsEntryModules ExportBasis = "entry_modules"
)

func (basis ExportBasis) Valid() bool {
	switch basis {
	case ExportsConsumerHeaders, ExportsLinkage, ExportsVisibility, ExportsEntryModules:
		return true
	default:
		return false
	}
}

// TargetExportInput binds an adapter-local callable ref to its declaration.
type TargetExportInput struct {
	ObjectRef string
	Location  *Location
}

// TargetExport is one sealed export: a local callable at its declaration.
type TargetExport struct {
	ObjectID string    `json:"object_id"`
	Location *Location `json:"location"`
}

// TargetSeedInput binds an adapter-local object ref to one exact launch fact.
type TargetSeedInput struct {
	ObjectRef string
	Kind      SeedKind
	Location  *Location
}

// TargetSeed is the sealed language-neutral launch handoff consumed by later
// semantic cubes and presentation projections.
type TargetSeed struct {
	ObjectID string    `json:"object_id"`
	Kind     SeedKind  `json:"kind"`
	Location *Location `json:"location"`
}

// TargetInput is the adapter-owned target scope before local identity sealing.
// Sources are exact source/root/manifest evidence. AnchorFileRef is one member
// selected only as the stable display and identity anchor. Selector is the
// adapter-owned declaration key that distinguishes otherwise identical target
// views, such as Python console_scripts and gui_scripts aliases.
type TargetInput struct {
	// ID is assigned once by the complete target plan. Standalone cube calls
	// omit it and receive t1.
	ID            string
	TestSources   []string
	Language      string
	Kind          string
	Name          string
	Selector      string
	Sources       []TargetSource
	AnchorFileRef string
	// Seeds establish where execution can begin for this selected target.
	// A library has none: its API is Exports, never a launch root.
	Seeds []TargetSeedInput
	// Exports are a library's API: the callables a program using it may
	// call, where control enters the library, by its adapter's rule
	// (ExportBasis). An executable has none. They are not launch roots: the
	// Main flow, start-up phases and the atlas never read them as seeds.
	Exports     []TargetExportInput
	ExportBasis ExportBasis
	// Executables are the names the repository's build gives this
	// program's executable, a build fact of its adapter: C's linked program
	// (the Makefile target), a Go main package's directory, a Python
	// console_script, package.json bin commands. Empty when the build names
	// none.
	Executables []string
	// Libraries are the import names under which the same build also
	// installs this program's code as a library: the declared top-level
	// packages of a library target of the same manifest that the portfolio
	// folded into this program (DISCOVERY, "Target selection"). Empty when
	// no library was folded into it.
	Libraries []string
}

// Target is one exact selected program scope. It remains independent of a
// provider request and can cover several executable roots or library sources.
type Target struct {
	// TestSources contains adapter-observed testing files from the full index,
	// independently of the native target's smaller root/manifest Sources set.
	TestSources   []string       `json:"test_sources,omitempty"`
	ID            string         `json:"id"`
	Language      string         `json:"language"`
	Kind          string         `json:"kind"`
	Name          string         `json:"name"`
	Selector      string         `json:"selector"`
	Sources       []TargetSource `json:"sources"`
	AnchorFileRef string         `json:"anchor_file_ref"`
	Seeds         []TargetSeed   `json:"seeds"`
	// Executables are the names the build gives the program's executable
	// (TargetInput.Executables), sorted and each once.
	Executables []string `json:"executables,omitempty"`
	// Libraries are the import names of the library facet folded into this
	// program (TargetInput.Libraries), sorted and each once.
	Libraries []string `json:"libraries,omitempty"`
	// Exports are a library's API (TargetInput.Exports), sorted by object
	// ID, with how its adapter knows them.
	Exports     []TargetExport `json:"exports,omitempty"`
	ExportBasis ExportBasis    `json:"export_basis,omitempty"`
}

// Snapshot returns a consumer-owned copy of the selected target boundary.
func (target Target) Snapshot() Target {
	result := target
	result.TestSources = slices.Clone(target.TestSources)
	result.Sources = cloneTargetSources(target.Sources)
	result.Seeds = cloneTargetSeeds(target.Seeds)
	result.Executables = slices.Clone(target.Executables)
	result.Libraries = slices.Clone(target.Libraries)
	result.Exports = cloneTargetExports(target.Exports)
	return result
}

// rebindTargetID installs the target's one portfolio-local t* identity and
// reseals the exact same fact graph. Object and relation IDs are target-local
// already and therefore do not change.
func rebindTargetID(index Index, targetID string) (Index, error) {
	if err := index.Validate(); err != nil {
		return Index{}, fmt.Errorf("program index: rebind target: %w", err)
	}
	if !validCompactID(targetID, "t") {
		return Index{}, fmt.Errorf("program index: invalid compact target ID %q", targetID)
	}
	if index.Target.ID == targetID {
		return index.Snapshot(), nil
	}
	if index.Categorization != nil {
		assignments := make([]CategoryAssignment, len(index.Categorization.Assignments))
		for position, assignment := range index.Categorization.Assignments {
			assignments[position] = CategoryAssignment{
				SubjectID:  assignment.SubjectID,
				Categories: append([]Category(nil), assignment.Categories...),
			}
		}
		documentationSHA := index.Categorization.ReducedDocumentationSHA256
		base, err := Base(index)
		if err != nil {
			return Index{}, err
		}
		rebound, err := rebindTargetID(base, targetID)
		if err != nil {
			return Index{}, err
		}
		return Enrich(rebound, documentationSHA, assignments)
	}
	result := index.Snapshot()
	result.Target.ID = targetID
	result.SHA256 = ""
	seal, err := indexDigest(result)
	if err != nil {
		return Index{}, err
	}
	result.SHA256 = seal
	if err := result.Validate(); err != nil {
		return Index{}, err
	}
	return result, nil
}

// RebindTargetID is rebindTargetID for an adapter that validates its own
// projection against an index the portfolio has already numbered.
func RebindTargetID(index Index, targetID string) (Index, error) {
	return rebindTargetID(index, targetID)
}

// RebindTargetSet assigns t1..tN once for a complete target set. Ordinals
// follow target content, not discovery order, and the returned slice preserves
// caller order.
func RebindTargetSet(indexes []Index) ([]Index, error) {
	type candidate struct {
		position int
		key      string
	}
	candidates := make([]candidate, len(indexes))
	for position, index := range indexes {
		if err := index.Validate(); err != nil {
			return nil, fmt.Errorf("program index: rebind target set: %w", err)
		}
		target := index.Target.Snapshot()
		target.ID = ""
		encoded, err := json.Marshal(target)
		if err != nil {
			return nil, fmt.Errorf("program index: encode target ordering key: %w", err)
		}
		candidates[position] = candidate{position: position, key: string(encoded)}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].key < candidates[j].key })
	for position := 1; position < len(candidates); position++ {
		if candidates[position-1].key == candidates[position].key {
			return nil, fmt.Errorf("program index: duplicate target in set")
		}
	}
	result := make([]Index, len(indexes))
	for ordinal, candidate := range candidates {
		rebound, err := rebindTargetID(indexes[candidate.position], compactOrdinalID("t", ordinal))
		if err != nil {
			return nil, err
		}
		result[candidate.position] = rebound
	}
	return result, nil
}

func compactOrdinalID(prefix string, zeroBased int) string {
	return prefix + strconv.Itoa(zeroBased+1)
}

// Validate checks the standalone target shape and its compact portfolio-local identity. This
// lets manifests and report adapters bind the same language-neutral target
// without retaining an entire Index in memory.
func (target Target) Validate() error {
	if err := validateTargetShape(target); err != nil {
		return err
	}
	if !validCompactID(target.ID, "t") {
		return fmt.Errorf("program index: target identity is not compact")
	}
	return nil
}

// ObjectInput is one adapter fact before SourceRef relationships are resolved
// to stable program-index IDs.
type ObjectInput struct {
	SourceRef string
	Kind      ObjectKind
	// Name is presentation text, never identity. Adapters keep repository
	// source paths in Location instead of concatenating them into this field.
	// Logical module/package names may retain their language-native spelling.
	Name         string
	Visibility   Visibility
	Signature    string
	OwnerRef     string
	ContainerRef string
	Location     *Location
	// DocstringRanges are original comment ranges attached to this
	// declaration by its native parser, in Location.Path. No text or role
	// is inferred; absence leaves attachment unknown.
	DocstringRanges []LineRange
	// EndLine is the last line of the declaration's source, when the adapter
	// knows where it ends; zero otherwise.
	EndLine int
	// CodeLines counts the lines of the declaration's own source that hold
	// code: not blank, not comment-only and not a docstring. The adapter's
	// own lexer or parser counts them; zero means unknown. A module counts
	// its whole file.
	CodeLines int
	// Unreachable is the adapter's proof that nothing this program runs
	// reaches the callable: no chain of calls and address uses from the
	// program's entry points names it. Only an adapter that sees every way
	// its language can reach a callable sets it; false claims nothing.
	Unreachable bool
	// Macro says the declaration is a macro: code the compiler expands where
	// it is written (Clojure's `defmacro`), so a use of it is no runtime
	// relation and the adapter records none. Only a callable can be one.
	Macro bool
	// Anonymous says the callable is written as an expression inside another
	// declaration and has no name of its own, so its name is the adapter's
	// and no declaration a reader looks up: a Go function literal, which
	// go/ssa names after the function holding it (Open$1). Any adapter may
	// set it on a callable; a lambda is anonymous by its kind already.
	Anonymous bool
	// Directory is an adapter-observed repository directory for a package or
	// module. It remains available when that boundary has no source file.
	Directory string
	// External is adapter-owned origin and symbol authority. It is valid only
	// for ObjectExternalSymbol and avoids forcing consumers to recover package
	// or platform boundaries from presentation text or raw identity syntax.
	External *ExternalSymbol
	Aliases  []Alias
	// Types are, for a variable (a field), where the repository types its
	// declared type names are declared (`DBs []*DBConfig` names DBConfig).
	Types []Location
	// Parameters and Results are a callable's values in order; TypeRef names
	// the repository type a value carries when the adapter resolved one.
	Parameters []TypedNameInput
	Results    []TypedNameInput
	// ParameterStores are the stores of the callable's own parameters into a
	// field or a module-level variable.
	ParameterStores []ParameterStore
	// Rows are, for a module-level table variable, the rows of its
	// initializer that write string literals and store no repository
	// callable (a row that stores one is a registration, D1).
	Rows []TableRow
	// Comparisons are, for a callable or a module body, the values it
	// compares with two or more different words (see Comparison).
	Comparisons []Comparison
	// Overloads are, for a callable, the other signatures it is declared
	// with before its implementation (see Overload).
	Overloads []OverloadInput
}

// OverloadInput is one Overload as an adapter hands it, its values' types
// by source ref.
type OverloadInput struct {
	Signature  string
	Location   *Location
	EndLine    int
	CodeLines  int
	Parameters []TypedNameInput
	Results    []TypedNameInput
}

// Overload is one signature a callable is declared with besides its own:
// a Python `@typing.overload` stub (its decorator resolving to
// typing.overload) or a TypeScript overload signature, each written before
// the implementation of the same name, which is what runs and what a call
// reaches (PYTHON, JSTS). It keeps its own source place, lines and typed
// values, and is no declaration of its own: beets's BeatportClient.search,
// two stubs and an implementation, is one method. Overloads are in source
// order, in the callable's file, before it.
type Overload struct {
	Signature  string      `json:"signature,omitempty"`
	Location   *Location   `json:"location"`
	EndLine    int         `json:"end_line,omitempty"`
	CodeLines  int         `json:"code_lines,omitempty"`
	Parameters []TypedName `json:"parameters,omitempty"`
	Results    []TypedName `json:"results,omitempty"`
}

// validOverloads checks a callable's overloads: each located in the
// callable's file before it, in source order, with its own lines.
func validOverloads(kind ObjectKind, location *Location, overloads []Overload) bool {
	if len(overloads) == 0 {
		return true
	}
	if !callableKind(kind) || location == nil {
		return false
	}
	var previous *Location
	for _, overload := range overloads {
		at := overload.Location
		if at == nil || !validLocation(*at) || at.Path != location.Path || !locationBefore(*at, *location) || previous != nil && !locationBefore(*previous, *at) ||
			!validOptionalText(overload.Signature) || !validEndLine(at, overload.EndLine) || !validCodeLines(at, overload.EndLine, overload.CodeLines) {
			return false
		}
		for _, typed := range append(append([]TypedName(nil), overload.Parameters...), overload.Results...) {
			if !validOptionalText(typed.Name) || !validOptionalText(typed.Type) || typed.Name == "" && typed.Type == "" || typed.TypeID != "" && !validCompactID(typed.TypeID, "n") {
				return false
			}
		}
		previous = at
	}
	return true
}

// locationBefore says a comes before b in one file.
func locationBefore(a, b Location) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Column < b.Column
}

func cloneOverloads(values []Overload) []Overload {
	if values == nil {
		return nil
	}
	result := make([]Overload, len(values))
	for position, value := range values {
		result[position] = value
		result[position].Location = cloneLocation(value.Location)
		result[position].Parameters = slices.Clone(value.Parameters)
		result[position].Results = slices.Clone(value.Results)
	}
	return result
}

// ParameterStore is a callable storing one of its own parameters, as it
// received it, in a field of a record or object, or in a module-level
// variable. Parameter is the parameter's one-based position as a call site
// counts its arguments (a method's receiver excluded), Name its declared
// name, Slot the field or variable as the code names it (`Type.field`, or the
// variable's name) and Location the store. A store into a local variable, of
// a value computed from the parameter, or into a container's element is none.
// It states the structure only: what the stored value is later used for is
// not decided here.
type ParameterStore struct {
	Parameter int       `json:"parameter"`
	Name      string    `json:"name,omitempty"`
	Slot      string    `json:"slot"`
	Location  *Location `json:"location"`
}

// TableRow is one row of a module-level table that writes string literals
// and stores no repository callable: {"get", 2, REDIS_CMD_INLINE}, or one
// element of an array of strings. Literals are its string literals in
// order, each with the field it fills ("" for an array element) and where.
type TableRow struct {
	Literals []RowLiteral `json:"literals"`
}

// RowLiteral is one string literal a table row writes.
type RowLiteral struct {
	Field    string    `json:"field,omitempty"`
	Value    string    `json:"value"`
	Location *Location `json:"location"`
}

// Comparison is one value a callable or a module body compares with two or
// more different words: the cases of a switch, match or case statement on
// the value, and the == comparisons of the same value with a word (a
// multi-way dispatch): two or more different non-empty words in two or more
// cases. A lone comparison is none, and so is one condition naming several
// words for one branch. Value is the compared
// expression as written; Origin is where its value comes from, as the
// adapter records source values; Location is where it is first compared.
// Cases are in source order. It states the structure only: what the words
// are is decided later.
type Comparison struct {
	Value    string             `json:"value"`
	Origin   *sourcevalue.Value `json:"origin,omitempty"`
	Location *Location          `json:"location"`
	Cases    []ComparisonCase   `json:"cases"`
}

// ComparisonForm is the closed syntax one case is written in.
type ComparisonForm string

const (
	// ComparisonCaseForm is a case of a switch, match or case statement.
	ComparisonCaseForm ComparisonForm = "case"
	// ComparisonEquals is an == comparison with a word, or a test of
	// membership in a written list of words.
	ComparisonEquals ComparisonForm = "equals"
)

func (form ComparisonForm) Valid() bool {
	return form == ComparisonCaseForm || form == ComparisonEquals
}

// ComparisonCase is one branch of a Comparison: the words it compares the
// value with (`case "a", "b":` has two; comparisons joined by or in one
// condition are one case), where the first is written, and Branch, the
// lines of the code the case selects, both included, when the adapter
// knows them (a case body, the block an if statement's condition guards).
type ComparisonCase struct {
	Form     ComparisonForm `json:"form"`
	Words    []string       `json:"words"`
	Location *Location      `json:"location"`
	Branch   *LineRange     `json:"branch,omitempty"`
	// Exclusive says the adapter knows Branch runs only when the value is
	// one of Words: its condition is comparisons of the value joined by ||,
	// or one of them joined by && to anything, and no case falls through
	// into it. A condition such as `len(v) != 2 || v == "nu"`, or a case
	// another falls into, is not. False claims nothing.
	Exclusive bool `json:"exclusive,omitempty"`
}

func cloneComparisons(values []Comparison) []Comparison {
	if len(values) == 0 {
		return nil
	}
	result := make([]Comparison, len(values))
	for i, value := range values {
		result[i] = Comparison{Value: value.Value, Origin: sourcevalue.Clone(value.Origin), Location: cloneLocation(value.Location)}
		result[i].Cases = make([]ComparisonCase, len(value.Cases))
		for j, item := range value.Cases {
			result[i].Cases[j] = ComparisonCase{Form: item.Form, Words: slices.Clone(item.Words), Location: cloneLocation(item.Location), Branch: cloneLineRange(item.Branch), Exclusive: item.Exclusive}
		}
	}
	return result
}

// canonicalComparisons orders comparisons and their cases by where they are
// written.
func canonicalComparisons(values []Comparison) []Comparison {
	result := cloneComparisons(values)
	for i := range result {
		sort.SliceStable(result[i].Cases, func(a, b int) bool {
			return compareOptionalLocations(result[i].Cases[a].Location, result[i].Cases[b].Location) < 0
		})
	}
	sort.SliceStable(result, func(a, b int) bool { return compareOptionalLocations(result[a].Location, result[b].Location) < 0 })
	return result
}

// validComparisons accepts comparisons of a callable or a module body only,
// each located, in source order, with located cases in source order that
// compare the value with two or more different non-empty words in two or
// more cases.
func validComparisons(kind ObjectKind, values []Comparison) bool {
	if len(values) > 0 && !callableKind(kind) && kind != ObjectModule {
		return false
	}
	for position, value := range values {
		if !validText(value.Value) || value.Location == nil || !validLocation(*value.Location) || len(value.Cases) == 0 ||
			position > 0 && compareOptionalLocations(values[position-1].Location, value.Location) >= 0 ||
			sourcevalue.Validate(value.Origin) != nil {
			return false
		}
		distinct := map[string]bool{}
		worded := 0
		for at, item := range value.Cases {
			if !item.Form.Valid() || len(item.Words) == 0 || item.Location == nil || !validLocation(*item.Location) ||
				at > 0 && compareOptionalLocations(value.Cases[at-1].Location, item.Location) >= 0 ||
				item.Branch != nil && (item.Branch.Line < 1 || item.Branch.EndLine < item.Branch.Line) {
				return false
			}
			counted := false
			for _, word := range item.Words {
				if !utf8.ValidString(word) || strings.ContainsRune(word, 0) {
					return false
				}
				if word != "" {
					distinct[word] = true
					if !counted {
						worded, counted = worded+1, true
					}
				}
			}
		}
		if len(distinct) < 2 || worded < 2 {
			return false
		}
	}
	return true
}

func canonicalParameterStores(values []ParameterStore) []ParameterStore {
	if len(values) == 0 {
		return nil
	}
	result := make([]ParameterStore, 0, len(values))
	for _, value := range values {
		value.Location = cloneLocation(value.Location)
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return compareParameterStores(result[i], result[j]) < 0 })
	return slices.CompactFunc(result, func(a, b ParameterStore) bool { return compareParameterStores(a, b) == 0 })
}

func compareParameterStores(a, b ParameterStore) int {
	if a.Parameter != b.Parameter {
		return a.Parameter - b.Parameter
	}
	if a.Slot != b.Slot {
		return strings.Compare(a.Slot, b.Slot)
	}
	if a.Name != b.Name {
		return strings.Compare(a.Name, b.Name)
	}
	switch {
	case a.Location == nil || b.Location == nil:
		return 0
	case a.Location.Path != b.Location.Path:
		return strings.Compare(a.Location.Path, b.Location.Path)
	case a.Location.Line != b.Location.Line:
		return a.Location.Line - b.Location.Line
	default:
		return a.Location.Column - b.Location.Column
	}
}

func validParameterStores(kind ObjectKind, values []ParameterStore) bool {
	if len(values) > 0 && !callableKind(kind) && kind != ObjectType {
		return false
	}
	for position, value := range values {
		if value.Parameter < 1 || !validOptionalText(value.Name) || !validText(value.Slot) || value.Location == nil || !validLocation(*value.Location) ||
			position > 0 && compareParameterStores(values[position-1], value) >= 0 {
			return false
		}
	}
	return true
}

func cloneRows(values []TableRow) []TableRow {
	if len(values) == 0 {
		return nil
	}
	result := make([]TableRow, len(values))
	for i, row := range values {
		result[i].Literals = make([]RowLiteral, len(row.Literals))
		for j, literal := range row.Literals {
			literal.Location = cloneLocation(literal.Location)
			result[i].Literals[j] = literal
		}
	}
	return result
}

func validRows(kind ObjectKind, values []TableRow) bool {
	if len(values) > 0 && kind != ObjectVariable {
		return false
	}
	for _, row := range values {
		if len(row.Literals) == 0 {
			return false
		}
		for _, literal := range row.Literals {
			if !utf8.ValidString(literal.Value) || strings.ContainsRune(literal.Value, 0) || !validOptionalText(literal.Field) || literal.Location == nil || !validLocation(*literal.Location) {
				return false
			}
		}
	}
	return true
}

// TypedNameInput is one value of a callable's signature as an adapter hands
// it over: its name, its type as short text, and the source ref of the
// repository type it carries, if any.
type TypedNameInput struct {
	Name    string
	Type    string
	TypeRef string
}

// TypedName is one sealed value of a callable's signature. TypeID is the
// repository type the value carries, empty for a builtin, an outside type, a
// function or an anonymous type.
type TypedName struct {
	Name   string `json:"name,omitempty"`
	Type   string `json:"type,omitempty"`
	TypeID string `json:"type_id,omitempty"`
}

// Alias is a name a declaration takes in another format, such as the JSON key
// of a Go struct field declared with `json:"count_label"`.
type Alias struct {
	Format string `json:"format"`
	Name   string `json:"name"`
}

func canonicalAliases(values []Alias) []Alias {
	if len(values) == 0 {
		return nil
	}
	result := slices.Clone(values)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Format+"\x00"+result[i].Name < result[j].Format+"\x00"+result[j].Name
	})
	return slices.Compact(result)
}

func validAliases(values []Alias) bool {
	for position, alias := range values {
		if !validText(alias.Format) || !validText(alias.Name) ||
			position > 0 && values[position-1].Format+"\x00"+values[position-1].Name >= alias.Format+"\x00"+alias.Name {
			return false
		}
	}
	return true
}

// ExternalAuthorityKind is the adapter-established origin class for an
// external symbol. PackagePath remains the exact raw language-tool identity;
// consumers use this closed kind rather than parsing that identity.
type ExternalAuthorityKind string

const (
	ExternalAuthorityPackage  ExternalAuthorityKind = "package"
	ExternalAuthorityPlatform ExternalAuthorityKind = "platform"
)

func (kind ExternalAuthorityKind) Valid() bool {
	return kind == ExternalAuthorityPackage || kind == ExternalAuthorityPlatform
}

// ExternalSymbol is the exact language-tool identity of an external program
// object. PackagePath is the raw import/package identity used to join package
// authorities to dependencies.Catalog. Receiver is optional because free
// functions and package variables do not have one.
type ExternalSymbol struct {
	// RepositoryPath identifies a compiler-observed package in this repository
	// outside this target. Empty means no repository origin was established.
	RepositoryPath string                `json:"repository_path,omitempty"`
	AuthorityKind  ExternalAuthorityKind `json:"authority_kind"`
	PackagePath    string                `json:"package_path"`
	Receiver       string                `json:"receiver,omitempty"`
	Name           string                `json:"name"`
}

// IsExternalPackageAuthority reports exact adapter-owned package authority.
func IsExternalPackageAuthority(value *ExternalSymbol) bool {
	return value != nil && value.AuthorityKind == ExternalAuthorityPackage
}

// IsExternalPlatformAuthority reports exact adapter-owned standard-runtime
// platform authority.
func IsExternalPlatformAuthority(value *ExternalSymbol) bool {
	return value != nil && value.AuthorityKind == ExternalAuthorityPlatform
}

// Object is one language-neutral program declaration or external symbol.
// Signature, ownership, containment and location are optional because not all
// adapters can establish them with exact local authority.
type Object struct {
	ID              string          `json:"id"`
	SourceRef       string          `json:"-"`
	Kind            ObjectKind      `json:"kind"`
	Name            string          `json:"name"`
	Visibility      Visibility      `json:"visibility"`
	Signature       string          `json:"signature,omitempty"`
	OwnerID         string          `json:"owner_id,omitempty"`
	ContainerID     string          `json:"container_id,omitempty"`
	Location        *Location       `json:"location,omitempty"`
	DocstringRanges []LineRange     `json:"docstring_ranges,omitempty"`
	EndLine         int             `json:"end_line,omitempty"`
	CodeLines       int             `json:"code_lines,omitempty"`
	Unreachable     bool            `json:"unreachable,omitempty"`
	Macro           bool            `json:"macro,omitempty"`
	Anonymous       bool            `json:"anonymous,omitempty"`
	Directory       string          `json:"directory,omitempty"`
	External        *ExternalSymbol `json:"external,omitempty"`
	Aliases         []Alias         `json:"aliases,omitempty"`
	// Types are, for a variable, where the repository types its declared
	// type names are declared (see ObjectInput.Types).
	Types      []Location  `json:"types,omitempty"`
	Parameters []TypedName `json:"parameters,omitempty"`
	Results    []TypedName `json:"results,omitempty"`
	// ParameterStores are the callable's own parameters it stores in a field
	// or a module-level variable (see ParameterStore).
	ParameterStores []ParameterStore `json:"parameter_stores,omitempty"`
	// Rows are a table variable's rows that write words and store no
	// repository callable (see TableRow).
	Rows []TableRow `json:"rows,omitempty"`
	// Comparisons are the values a callable or a module body compares with
	// two or more different words (see Comparison).
	Comparisons []Comparison `json:"comparisons,omitempty"`
	// Overloads are a callable's other signatures, written before it (see
	// Overload).
	Overloads []Overload `json:"overloads,omitempty"`
}

// Witness preserves one bounded local fact supporting a relation. Kind and
// Detail remain adapter facts; they are not model-authored semantics.
// SourceExpression is an optional exact, adapter-observed expression from the
// repository source. Consumers may interpret it according to the relation and
// witness kinds without parsing human-oriented Detail text.
//
// ObjectID names the declaration a witness is about when the adapter knows
// it: the function a store put into the field or name an unresolved call reads
// (`readQueryFromClient stored in aeFileEvent.rfileProc under a condition`).
// It is identity only; the call stays unresolved and the witness never becomes
// its target. Adapters hand the object over as ObjectRef, which New resolves.
type Witness struct {
	Kind             string    `json:"kind"`
	Detail           string    `json:"detail,omitempty"`
	SourceExpression string    `json:"source_expression,omitempty"`
	Location         *Location `json:"location,omitempty"`
	ObjectID         string    `json:"object_id,omitempty"`
	ObjectRef        string    `json:"-"`
}

// Witness kinds every adapter shares on a reads relation of a variable: how
// the site uses the variable's elements (a table's rows). Membership: the
// site tests whether a value is one of them (`command in NO_CONFIG`). Keys:
// each element is a key the program reads another module-level variable
// with, which ObjectID names and Location shows subscripted: the variable
// iterated where it is read (`OPTIONS[name] for name in ARGS`), or handed to
// a repository callable that iterates that parameter (`build_args(
// optionlist=ARGS)` where build_args does `for val in optionlist:
// OPTIONS[val]`). Any other read keeps its adapter's kind.
const (
	WitnessMembership = "membership"
	WitnessKeys       = "keys"
)

// PatternForm is the closed syntactic shape retained for adapter-neutral
// pattern classification. It describes source syntax only, never framework or
// protocol semantics.
type PatternForm string

const (
	PatternCall          PatternForm = "call"
	PatternDecoratorCall PatternForm = "decorator_call"
)

func (form PatternForm) Valid() bool {
	return form == PatternCall || form == PatternDecoratorCall
}

// PatternValueKind states how much exact string structure the adapter retained
// for one call argument.
type PatternValueKind string

const (
	PatternLiteralString  PatternValueKind = "literal_string"
	PatternStringTemplate PatternValueKind = "string_template"
	PatternDynamic        PatternValueKind = "dynamic"
)

func (kind PatternValueKind) Valid() bool {
	return kind == PatternLiteralString || kind == PatternStringTemplate || kind == PatternDynamic
}

// PatternPartKind is one closed component of a string template. Hole names are
// deliberately not retained: only literal text carries matching authority.
type PatternPartKind string

const (
	PatternPartLiteral PatternPartKind = "literal"
	PatternPartHole    PatternPartKind = "hole"
)

func (kind PatternPartKind) Valid() bool {
	return kind == PatternPartLiteral || kind == PatternPartHole
}

type PatternPartInput struct {
	Kind PatternPartKind
	Text string
}

type PatternPart struct {
	Kind PatternPartKind `json:"kind"`
	Text string          `json:"text,omitempty"`
}

// PatternValueResolution states whether a locally reconstructed argument
// value is exact for the use or remains one possible runtime value. This is
// deliberately separate from Relation.Resolution: a mutable language binding
// may name one exact source object while its initializer is still only a
// possible value at the later use.
type PatternValueResolution string

const (
	PatternValueExact    PatternValueResolution = "exact"
	PatternValuePossible PatternValueResolution = "possible"
)

func (resolution PatternValueResolution) Valid() bool {
	return resolution == PatternValueExact || resolution == PatternValuePossible
}

// PatternValueSourceKind is the adapter-neutral structural joint used to
// recover a value without asking a model to copy it. New source kinds belong
// here only when a language adapter can retain their complete local evidence.
type PatternValueSourceKind string

const (
	PatternValueSourceInitializer    PatternValueSourceKind = "initializer"
	PatternValueSourceActualArgument PatternValueSourceKind = "actual_argument"
)

func (kind PatternValueSourceKind) Valid() bool {
	return kind == PatternValueSourceInitializer || kind == PatternValueSourceActualArgument
}

// PatternValueCandidateInput is one adapter-observed value reconstruction for
// a dynamic argument. SourceObjectRefs and SourceArgumentRefs bind the
// reconstruction to canonical ProgramIndex identities during sealing.
type PatternValueCandidateInput struct {
	Kind                    PatternValueKind
	Value                   string
	Parts                   []PatternPartInput
	Resolution              PatternValueResolution
	SourceKind              PatternValueSourceKind
	SourceObjectRefs        []string
	SourceObjectsObserved   int
	SourceArgumentRefs      []PatternArgumentRefInput
	SourceArgumentsObserved int
}

// PatternValueCandidate is one sealed, identity-bound value reconstruction.
// ID includes its owning argument, value shape, authority, provenance kind,
// and canonical source objects or arguments, so it cannot be moved between
// uses.
type PatternValueCandidate struct {
	ID                      string                 `json:"id"`
	Kind                    PatternValueKind       `json:"kind"`
	Value                   string                 `json:"value,omitempty"`
	Parts                   []PatternPart          `json:"parts,omitempty"`
	Resolution              PatternValueResolution `json:"resolution"`
	SourceKind              PatternValueSourceKind `json:"source_kind"`
	SourceObjectIDs         []string               `json:"source_object_ids,omitempty"`
	SourceObjectsObserved   int                    `json:"-"`
	SourceObjectsOmitted    int                    `json:"source_objects_omitted,omitempty"`
	SourceArgumentIDs       []string               `json:"source_argument_ids,omitempty"`
	SourceArgumentsObserved int                    `json:"-"`
	SourceArgumentsOmitted  int                    `json:"source_arguments_omitted,omitempty"`
}

// PatternArgumentInput is one adapter-observed positional or keyword
// argument. Exactly one of Position (one-based) and Keyword is set.
type PatternArgumentInput struct {
	Origin                  *sourcevalue.Value
	Position                int
	Keyword                 string
	Kind                    PatternValueKind
	Value                   string
	Parts                   []PatternPartInput
	ObjectRefs              []string
	Resolution              Resolution
	ObjectsObserved         int
	ValueCandidates         []PatternValueCandidateInput
	ValueCandidatesObserved int
}

// PatternArgumentRefInput identifies one exact nested argument without asking
// an adapter to predict canonical ProgramIndex IDs. RelationSourceRef selects
// the owning relation, PatternSourceRef selects its nested pattern, and the
// positional/keyword key selects the argument. Resolution is fail-closed when
// that adapter-local tuple is absent or ambiguous.
type PatternArgumentRefInput struct {
	RelationSourceRef string
	PatternSourceRef  string
	Position          int
	Keyword           string
}

// PatternRefInput identifies one nested pattern by its relation's and its
// own adapter SourceRef; New resolves it to the pattern's sealed ID.
type PatternRefInput struct {
	RelationSourceRef string
	PatternSourceRef  string
}

// PatternArgument is one sealed argument. ID is stable under input ordering
// and is derived from its owning pattern plus its positional or keyword key.
type PatternArgument struct {
	Origin                  *sourcevalue.Value      `json:"origin,omitempty"`
	ID                      string                  `json:"id"`
	Position                int                     `json:"position,omitempty"`
	Keyword                 string                  `json:"keyword,omitempty"`
	Kind                    PatternValueKind        `json:"kind"`
	Value                   string                  `json:"value,omitempty"`
	Parts                   []PatternPart           `json:"parts,omitempty"`
	ObjectIDs               []string                `json:"object_ids,omitempty"`
	Resolution              Resolution              `json:"resolution,omitempty"`
	ObjectsObserved         int                     `json:"-"`
	ObjectsOmitted          int                     `json:"objects_omitted,omitempty"`
	ValueCandidates         []PatternValueCandidate `json:"value_candidates,omitempty"`
	ValueCandidatesObserved int                     `json:"-"`
	ValueCandidatesOmitted  int                     `json:"value_candidates_omitted,omitempty"`
}

// RelationPatternInput retains one bounded syntactic candidate nested in its
// owning relation. Object refs are temporary joins within the same Input.
type RelationPatternInput struct {
	ReceiverValue *sourcevalue.Value
	ResultValue   *sourcevalue.Value
	// Context contains source-anchored enclosing control statements for this
	// exact call site. It neither classifies the callable nor changes the call.
	Context []Witness
	// Branch is, for a call written in an if statement's condition, the lines
	// of the statement that condition guards: where the code the comparison
	// selects is written (`strcasecmp(argv[0],"slaveof")`'s block).
	Branch                   *LineRange
	SourceRef                string
	Form                     PatternForm
	Selector                 string
	Location                 *Location
	ResultRef                string
	ReceiverRef              string
	ReceiverOriginRefs       []string
	ReceiverOriginResolution Resolution
	ReceiverOriginsObserved  int
	Arguments                []PatternArgumentInput
	ArgumentsObserved        int
	// SameValueAs is, for a call that reads the same value as an earlier
	// call (RelationPattern.SameValueAs), that call's pattern.
	SameValueAs *PatternRefInput
}

// RelationPattern is a sealed source-syntax candidate. Its identity is local
// to the owning relation; SourceRef therefore needs to be unique only there.
type RelationPattern struct {
	ReceiverValue            *sourcevalue.Value `json:"receiver_value,omitempty"`
	ResultValue              *sourcevalue.Value `json:"result_value,omitempty"`
	Context                  []Witness          `json:"context,omitempty"`
	Branch                   *LineRange         `json:"branch,omitempty"`
	ID                       string             `json:"id"`
	SourceRef                string             `json:"-"`
	Form                     PatternForm        `json:"form"`
	Selector                 string             `json:"selector"`
	Location                 *Location          `json:"location,omitempty"`
	ResultID                 string             `json:"result_id,omitempty"`
	ReceiverID               string             `json:"receiver_id,omitempty"`
	ReceiverOriginIDs        []string           `json:"receiver_origin_ids,omitempty"`
	ReceiverOriginResolution Resolution         `json:"receiver_origin_resolution,omitempty"`
	ReceiverOriginsObserved  int                `json:"-"`
	ReceiverOriginsOmitted   int                `json:"receiver_origins_omitted,omitempty"`
	Arguments                []PatternArgument  `json:"arguments,omitempty"`
	ArgumentsObserved        int                `json:"-"`
	ArgumentsOmitted         int                `json:"arguments_omitted,omitempty"`
	// SameValueAs is the pattern of an earlier call this call is another
	// spelling of: a call of the same callee from the same declaration,
	// written the same but for its string literals, either another operand
	// of one `||` (JS/TS `??`, Clojure `or`) written the same around its
	// call (`query.Get("forcePathStyle") != "" ||
	// query.Get("force-path-style") != ""`), or the header of another arm
	// of one if/else-if chain whose headers are written the same but for
	// the call's words and whose bodies are written the same (`if v :=
	// query.Get("storageClass"); v != "" { storageClass = v } else if v :=
	// query.Get("storage-class"); …`). It names the first call of its
	// group, which names none. A code fact: what the value is, and whether
	// either call is an input, stays the reading's.
	SameValueAs string `json:"same_value_as,omitempty"`
}

// Invocation says how a call runs. Empty is an ordinary call that returns
// before its caller continues; every adapter uses the same words.
const (
	InvocationDeferred  = "deferred"
	InvocationGoroutine = "goroutine"
	InvocationAsyncTask = "async_task"
	InvocationConstruct = "construct"
)

func validInvocation(value string) bool {
	return value == "" || value == InvocationDeferred || value == InvocationGoroutine ||
		value == InvocationAsyncTask || value == InvocationConstruct
}

// Dispatch says how a call's target was found. Empty is a static target.
// Interface targets are implementations; an interface method target is the
// declared method of an external interface whose implementation is unknown.
const (
	DispatchInterface       = "interface"
	DispatchInterfaceMethod = "interface_method"
	DispatchFunctionValue   = "function_value"
)

func validDispatch(value string) bool {
	return value == "" || value == DispatchInterface || value == DispatchInterfaceMethod || value == DispatchFunctionValue
}

// Guard is, on a call relation, the strongest construct of its function that
// every one of its call sites runs under, with that construct's location:
// GuardBranch an if or else arm, a case, a ?: arm or the right operand of a
// short-circuit operator (a call in a condition itself runs unguarded);
// GuardError a path that fails (what a raise, throw or panic hands over, an
// except or catch body, an arm ending in one, Go's arm taken when a value of
// the error type is not nil); GuardNoReturn a call of a function that never
// returns, or an arm ending in one (C: a callee whose type says noreturn,
// outside the corpus, or a corpus function every path of whose body ends in
// such a call). One unguarded site leaves the relation unguarded; the
// weakest of its sites' guards stands for it. It is a native fact of the
// code's structure, saved, never a provider row (owner, control review
// 2026-10-03: Lua's forprep calls luaG_runerror only when the step is zero,
// and the error message's allocation had read as the path's outcome).
//
// Condition is, for an arm a condition decides, that condition's own code
// as written, trimmed of surrounding whitespace only (C, GO, PYTHON), and
// When on which outcome the arm runs: GuardWhenHolds (an if arm, a ?: arm
// taken when it holds, an && operand after the ones before it), GuardWhenFails
// (an else arm, the other ?: arm, an || operand) or GuardWhenMatches (a case
// of a switch or match, Condition its subject). Absent, the arm has no one
// condition (a select clause, an except body, a try's else) or the guard is
// a call that never returns. It is display only, never model input (owner,
// 2026-10-05: lua.c:361 reads "only if `script`").
type Guard struct {
	Kind      string    `json:"kind"`
	Location  *Location `json:"location,omitempty"`
	Condition string    `json:"condition,omitempty"`
	When      string    `json:"when,omitempty"`
}

// The outcomes of a guard's condition its arm runs on (Guard.When).
const (
	GuardWhenHolds   = "holds"
	GuardWhenFails   = "fails"
	GuardWhenMatches = "matches"
)

// GuardCondition is a guard's Condition and When for an arm: the code
// trimmed of surrounding whitespace, both empty when no code is known.
func GuardCondition(code, when string) (string, string) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", ""
	}
	return code, when
}

// The guard kinds, weakest first; an error and a call that never returns are
// equally strong.
const (
	GuardBranch   = "branch"
	GuardError    = "error"
	GuardNoReturn = "noreturn"
)

// GuardStrength orders guard kinds: 0 for none, 1 a branch, 2 a failing path.
func GuardStrength(guard *Guard) int {
	switch {
	case guard == nil:
		return 0
	case guard.Kind == GuardBranch:
		return 1
	default:
		return 2
	}
}

// Fails says a guard is a failing path: an error or a call that never returns.
func (guard *Guard) Fails() bool {
	return guard != nil && (guard.Kind == GuardError || guard.Kind == GuardNoReturn)
}

// WeakestGuard folds the guards of several sites of one relation: an
// unguarded site leaves none, else the weakest stands (the first of equal
// strength). A written condition belongs to the folded call only when every
// site has that same condition and outcome. Never change the input guards.
func WeakestGuard(guards []*Guard) *Guard {
	var weakest *Guard
	common := true
	for position, guard := range guards {
		if guard == nil {
			return nil
		}
		if position > 0 && (guard.Condition != guards[0].Condition || guard.When != guards[0].When) {
			common = false
		}
		if position == 0 || GuardStrength(guard) < GuardStrength(weakest) {
			weakest = guard
		}
	}
	if !common {
		weakest = cloneGuard(weakest)
		weakest.Condition, weakest.When = "", ""
	}
	return weakest
}

// BasisImplements is, on a call through a repository interface whose value
// no observed flow gives, how its targets are known: they are the methods of
// the repository's types that implement the interface (owner, 2026-09-16 and
// 2026-09-30: an interface call follows the repository's implementations,
// one exact, several alternatives), not a traced binding of this value. An
// absent basis is an observed one.
const BasisImplements = "implements"

// RelationInput cites ObjectInput.SourceRef values. TargetsObserved and
// WitnessesObserved are mandatory adapter measurements; the core never derives
// them from retained rows.
type RelationInput struct {
	SourceRef         string
	Kind              RelationKind
	FromRef           string
	ToRefs            []string
	Resolution        Resolution
	Invocation        string
	Dispatch          string
	Location          *Location
	TargetsObserved   int
	Witnesses         []Witness
	WitnessesObserved int
	Patterns          []RelationPatternInput
	PatternsObserved  int
	SourceArgument    *PatternArgumentRefInput
	// FieldPath is, on a reads or writes relation whose target is a field
	// of a record, the field as the code reaches it (Relation.FieldPath).
	FieldPath string
	// Basis is how a call's targets are known (Relation.Basis).
	Basis string
	// Guard is the construct every site of the call runs under
	// (Relation.Guard).
	Guard *Guard
	// Value is, on a writes relation with a FieldPath, the value the site
	// assigns (Relation.Value).
	Value *sourcevalue.Value
}

// Relation is one typed, locally resolved edge or uncertainty joint.
type Relation struct {
	ID                string            `json:"id"`
	SourceRef         string            `json:"-"`
	Kind              RelationKind      `json:"kind"`
	FromID            string            `json:"from_id"`
	ToIDs             []string          `json:"to_ids,omitempty"`
	Resolution        Resolution        `json:"resolution"`
	Invocation        string            `json:"invocation,omitempty"`
	Dispatch          string            `json:"dispatch,omitempty"`
	Location          *Location         `json:"location,omitempty"`
	TargetsObserved   int               `json:"-"`
	TargetsOmitted    int               `json:"targets_omitted,omitempty"`
	Witnesses         []Witness         `json:"witnesses,omitempty"`
	WitnessesObserved int               `json:"-"`
	WitnessesOmitted  int               `json:"witnesses_omitted,omitempty"`
	Patterns          []RelationPattern `json:"patterns,omitempty"`
	PatternsObserved  int               `json:"-"`
	PatternsOmitted   int               `json:"patterns_omitted,omitempty"`
	SourceArgumentID  string            `json:"source_argument_id,omitempty"`
	// FieldPath is, on a reads or writes relation of a record's field, the
	// field as the code reaches it: the root file-scope variable, or the
	// record type holding the chain's first field when the root is any
	// other value (a parameter, a local, a call's result), then each field
	// of the chain, elements left out (server.masterhost, server.db.expires,
	// redisDb.expires for db->expires). The target is the field itself, so
	// the readers and writers of one field gather across functions whatever
	// root each reaches it from.
	FieldPath string `json:"field_path,omitempty"`
	// Basis is, on a call through a repository interface whose value no
	// observed flow gives, BasisImplements: its targets are the
	// implementations of the interface's method in the repository (GO).
	Basis string `json:"basis,omitempty"`
	// Guard is, on a calls, invokes_external or passes_callback relation,
	// the construct every site of it runs under (Guard).
	Guard *Guard `json:"guard,omitempty"`
	// Value is, on a writes relation with a FieldPath, the value a plain
	// assignment stores in the field there, as the adapter records any
	// source value (`server.dbfilename = "dump.rdb"` stores the literal,
	// `= zstrdup(argv[1])` the call's result). A compound assignment, ++
	// and --, an element of an array field and an adapter recording no
	// value have none (C only; GO, PYTHON, JSTS, CLOJURE).
	Value *sourcevalue.Value `json:"value,omitempty"`
}

// CoverageInput retains adapter observations that could not all be represented
// by bounded Object and Relation rows. Measured is mandatory: the core never
// invents a complete ledger from the rows that happened to survive an adapter.
type CoverageInput struct {
	Measured          bool
	ObjectsObserved   int
	RelationsObserved int
}

// Coverage makes both index contents and the adapter's omission frontier
// explicit. Target, witness and nested-pattern omissions are aggregated from
// Relation rows.
type Coverage struct {
	ObjectsObserved              int `json:"-"`
	ObjectsIndexed               int `json:"-"`
	ObjectsOmitted               int `json:"objects_omitted,omitempty"`
	RelationsObserved            int `json:"-"`
	RelationsIndexed             int `json:"-"`
	RelationsOmitted             int `json:"relations_omitted,omitempty"`
	ExactRelations               int `json:"-"`
	AlternativeRelations         int `json:"-"`
	UnresolvedRelations          int `json:"-"`
	TargetsObserved              int `json:"-"`
	TargetsIndexed               int `json:"-"`
	TargetsOmitted               int `json:"targets_omitted,omitempty"`
	WitnessesObserved            int `json:"-"`
	WitnessesIndexed             int `json:"-"`
	WitnessesOmitted             int `json:"witnesses_omitted,omitempty"`
	PatternsObserved             int `json:"-"`
	PatternsIndexed              int `json:"-"`
	PatternsOmitted              int `json:"patterns_omitted,omitempty"`
	ArgumentsObserved            int `json:"-"`
	ArgumentsIndexed             int `json:"-"`
	ArgumentsOmitted             int `json:"arguments_omitted,omitempty"`
	ReceiverOriginsObserved      int `json:"-"`
	ReceiverOriginsIndexed       int `json:"-"`
	ReceiverOriginsOmitted       int `json:"receiver_origins_omitted,omitempty"`
	ArgumentObjectsObserved      int `json:"-"`
	ArgumentObjectsIndexed       int `json:"-"`
	ArgumentObjectsOmitted       int `json:"-"`
	ArgumentValuesObserved       int `json:"-"`
	ArgumentValuesIndexed        int `json:"-"`
	ArgumentValuesOmitted        int `json:"-"`
	ValueSourcesObserved         int `json:"-"`
	ValueSourcesIndexed          int `json:"-"`
	ValueSourcesOmitted          int `json:"-"`
	ValueArgumentSourcesObserved int `json:"-"`
	ValueArgumentSourcesIndexed  int `json:"-"`
	ValueArgumentSourcesOmitted  int `json:"-"`
}

type Input struct {
	ScenarioSHA256 string
	SourceSHA256   string
	Target         TargetInput
	Objects        []ObjectInput
	Relations      []RelationInput
	Coverage       CoverageInput
	FieldWrites    []FieldWrites
}

// FieldWrites is, once per field a source value references by key (kind
// field_writes: package path, type and field), every write the code makes
// into it as one source value: what a read whose instance cannot be followed
// may hold (ProgramIndex 27, Go).
type FieldWrites struct {
	Field string            `json:"field"`
	Value sourcevalue.Value `json:"value"`
}

// Index is the canonical, bounded and SHA-sealed language-neutral handoff.
type Index struct {
	Version        int             `json:"version"`
	ScenarioSHA256 string          `json:"scenario_sha256"`
	SourceSHA256   string          `json:"source_sha256"`
	Target         Target          `json:"target"`
	Objects        []Object        `json:"objects"`
	Relations      []Relation      `json:"relations"`
	Coverage       Coverage        `json:"coverage,omitzero"`
	Categorization *Categorization `json:"categorization,omitempty"`
	FieldWrites    []FieldWrites   `json:"field_writes,omitempty"`
	SHA256         string          `json:"sha256"`
}

// Encode validates and returns the canonical JSON artifact bytes.
func Encode(index Index) ([]byte, error) {
	if err := index.Validate(); err != nil {
		return nil, err
	}
	return EncodeValidated(index)
}

// EncodeValidated returns the artifact bytes of an index its caller has
// validated already: Encode without validating a second time. A witness or
// pattern at its relation's own location writes that location as {}, so the
// path, line and column are written once, on the relation; the reader
// restores them. {} is never a location itself (a location has a path, a
// line and a column), and a witness or pattern without a location still
// writes none. The seal is over the index, not over these bytes.
func EncodeValidated(index Index) ([]byte, error) {
	encoded, err := json.Marshal(storedIndexOf(index))
	if err != nil {
		return nil, fmt.Errorf("program index: encode artifact: %w", err)
	}
	return encoded, nil
}

// storedIndex is an Index as its artifact writes it: its relations name a
// witness's or pattern's location that equals the relation's own as {}.
type storedIndex struct {
	indexArtifact
	Relations []storedRelation `json:"relations"`
}

type storedRelation struct {
	Relation
	Witnesses []storedWitness `json:"witnesses,omitempty"`
	Patterns  []storedPattern `json:"patterns,omitempty"`
}

type storedWitness struct {
	Witness
	Location *storedLocation `json:"location,omitempty"`
}

type storedPattern struct {
	RelationPattern
	Location *storedLocation `json:"location,omitempty"`
}

// storedLocation is a Location as the artifact writes it; the empty one is
// the relation's own location.
type storedLocation struct {
	Path   string `json:"path,omitempty"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
}

func storedIndexOf(index Index) storedIndex {
	stored := storedIndex{indexArtifact: indexArtifact(index), Relations: make([]storedRelation, len(index.Relations))}
	for position, relation := range index.Relations {
		row := storedRelation{Relation: relation}
		if len(relation.Witnesses) > 0 {
			row.Witnesses = make([]storedWitness, len(relation.Witnesses))
			for witness, value := range relation.Witnesses {
				row.Witnesses[witness] = storedWitness{Witness: value, Location: storedLocationOf(value.Location, relation.Location)}
			}
		}
		if len(relation.Patterns) > 0 {
			row.Patterns = make([]storedPattern, len(relation.Patterns))
			for pattern, value := range relation.Patterns {
				row.Patterns[pattern] = storedPattern{RelationPattern: value, Location: storedLocationOf(value.Location, relation.Location)}
			}
		}
		stored.Relations[position] = row
	}
	return stored
}

func storedLocationOf(location, relation *Location) *storedLocation {
	switch {
	case location == nil:
		return nil
	case relation != nil && *location == *relation:
		return &storedLocation{}
	default:
		return &storedLocation{Path: location.Path, Line: location.Line, Column: location.Column}
	}
}

// restoreRelationLocation gives back the relation's own location that the
// artifact wrote as {}. Without a relation location {} stays empty, and
// validation refuses it.
func restoreRelationLocation(location **Location, relation *Location) {
	if *location != nil && **location == (Location{}) && relation != nil {
		*location = cloneLocation(relation)
	}
}

// Decode strictly decodes one JSON artifact, rejects unknown fields and
// trailing JSON values, then validates identities, references and the seal.
func Decode(encoded []byte) (Index, error) {
	index, err := decodeArtifact(encoded)
	if err != nil {
		return Index{}, err
	}
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

// DecodeVerified decodes an artifact whose bytes a caller has matched to the
// SHA-256 recorded when a validated index was written (report's program
// portfolio): validating it again re-encoded and hashed the whole index on
// every read, and the report reads each one several times (Metabase:
// minutes of a render).
func DecodeVerified(encoded []byte) (Index, error) {
	return decodeArtifact(encoded)
}

func decodeArtifact(encoded []byte) (Index, error) {
	if len(encoded) == 0 {
		return Index{}, fmt.Errorf("program index: invalid artifact size")
	}
	// The artifact shape is decoded here directly: through Index's own
	// UnmarshalJSON the decoder would scan the whole value twice more.
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var decoded indexArtifact
	if err := decoder.Decode(&decoded); err != nil {
		return Index{}, fmt.Errorf("program index: decode artifact: %w", err)
	}
	index := decoded.restore()
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Index{}, fmt.Errorf("program index: trailing JSON value")
		}
		return Index{}, fmt.Errorf("program index: trailing data: %w", err)
	}
	return index, nil
}

// UnmarshalJSON strictly decodes the compact artifact and restores the counts
// it omits: every observed count is its retained rows plus the stored omission,
// empty collections are absent, and coverage is compiled from the rows.
func (index *Index) UnmarshalJSON(encoded []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var decoded indexArtifact
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*index = decoded.restore()
	return nil
}

// indexArtifact is the stored shape of an Index, without its UnmarshalJSON.
type indexArtifact Index

func (decoded indexArtifact) restore() Index {
	index := Index(decoded)
	index.Objects = emptyIfNil(index.Objects)
	index.Relations = emptyIfNil(index.Relations)
	for position := range index.Relations {
		restoreRelationCounts(&index.Relations[position])
	}
	index.Coverage = compileCoverage(index.Objects, index.Relations,
		len(index.Objects)+decoded.Coverage.ObjectsOmitted, len(index.Relations)+decoded.Coverage.RelationsOmitted)
	return index
}

// validGuardCondition: a condition is code as written, trimmed, with one of
// the closed outcomes; none has neither.
func validGuardCondition(guard *Guard) bool {
	if guard.Condition == "" {
		return guard.When == ""
	}
	return guard.Condition == strings.TrimSpace(guard.Condition) && utf8.ValidString(guard.Condition) &&
		(guard.When == GuardWhenHolds || guard.When == GuardWhenFails || guard.When == GuardWhenMatches)
}

func cloneGuard(guard *Guard) *Guard {
	if guard == nil {
		return nil
	}
	copied := *guard
	copied.Location = cloneLocation(guard.Location)
	return &copied
}

func restoreRelationCounts(relation *Relation) {
	relation.ToIDs = emptyIfNil(relation.ToIDs)
	relation.Witnesses = emptyIfNil(relation.Witnesses)
	relation.Patterns = emptyIfNil(relation.Patterns)
	relation.TargetsObserved = len(relation.ToIDs) + relation.TargetsOmitted
	relation.WitnessesObserved = len(relation.Witnesses) + relation.WitnessesOmitted
	relation.PatternsObserved = len(relation.Patterns) + relation.PatternsOmitted
	for position := range relation.Witnesses {
		restoreRelationLocation(&relation.Witnesses[position].Location, relation.Location)
	}
	for position := range relation.Patterns {
		pattern := &relation.Patterns[position]
		restoreRelationLocation(&pattern.Location, relation.Location)
		pattern.ReceiverOriginIDs = emptyIfNil(pattern.ReceiverOriginIDs)
		pattern.Arguments = emptyIfNil(pattern.Arguments)
		pattern.ReceiverOriginsObserved = len(pattern.ReceiverOriginIDs) + pattern.ReceiverOriginsOmitted
		pattern.ArgumentsObserved = len(pattern.Arguments) + pattern.ArgumentsOmitted
		for position := range pattern.Arguments {
			argument := &pattern.Arguments[position]
			argument.Parts = emptyIfNil(argument.Parts)
			argument.ObjectIDs = emptyIfNil(argument.ObjectIDs)
			argument.ValueCandidates = emptyIfNil(argument.ValueCandidates)
			argument.ObjectsObserved = len(argument.ObjectIDs) + argument.ObjectsOmitted
			argument.ValueCandidatesObserved = len(argument.ValueCandidates) + argument.ValueCandidatesOmitted
			for position := range argument.ValueCandidates {
				candidate := &argument.ValueCandidates[position]
				candidate.Parts = emptyIfNil(candidate.Parts)
				candidate.SourceObjectIDs = emptyIfNil(candidate.SourceObjectIDs)
				candidate.SourceArgumentIDs = emptyIfNil(candidate.SourceArgumentIDs)
				candidate.SourceObjectsObserved = len(candidate.SourceObjectIDs) + candidate.SourceObjectsOmitted
				candidate.SourceArgumentsObserved = len(candidate.SourceArgumentIDs) + candidate.SourceArgumentsOmitted
			}
		}
	}
}

func emptyIfNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

// New resolves adapter-local source refs, assigns compact artifact identities,
// canonicalizes all collections, derives coverage, validates and seals the
// result. It never truncates or rejects an input at an advisory collection
// threshold.
func New(input Input) (Index, error) {
	targetSources, err := canonicalizeTargetSources(input.Target.Sources)
	if err != nil {
		return Index{}, err
	}
	index := Index{
		Version:        Version,
		ScenarioSHA256: input.ScenarioSHA256,
		SourceSHA256:   input.SourceSHA256,
		Target: Target{
			TestSources: slices.Clone(input.Target.TestSources),
			Language:    input.Target.Language, Kind: input.Target.Kind, Name: input.Target.Name,
			Selector: input.Target.Selector, Sources: targetSources, AnchorFileRef: input.Target.AnchorFileRef,
			Seeds: []TargetSeed{},
		},
		Objects:   make([]Object, 0, len(input.Objects)),
		Relations: make([]Relation, 0, len(input.Relations)),
	}
	for _, writes := range input.FieldWrites {
		index.FieldWrites = append(index.FieldWrites, FieldWrites{Field: writes.Field, Value: *sourcevalue.Clone(&writes.Value)})
	}
	slices.SortFunc(index.FieldWrites, func(x, y FieldWrites) int { return strings.Compare(x.Field, y.Field) })
	sort.Strings(index.Target.TestSources)
	index.Target.TestSources = slices.Compact(index.Target.TestSources)
	if len(input.Target.Executables) > 0 {
		index.Target.Executables = slices.Clone(input.Target.Executables)
		sort.Strings(index.Target.Executables)
		index.Target.Executables = slices.Compact(index.Target.Executables)
	}
	if len(input.Target.Libraries) > 0 {
		index.Target.Libraries = slices.Clone(input.Target.Libraries)
		sort.Strings(index.Target.Libraries)
		index.Target.Libraries = slices.Compact(index.Target.Libraries)
	}
	if err := validateTargetShape(index.Target); err != nil {
		return Index{}, err
	}
	seedInputs, err := canonicalizeTargetSeedInputs(input.Target.Seeds)
	if err != nil {
		return Index{}, err
	}
	bindings := make([]objectBinding, 0, len(input.Objects))
	for _, value := range input.Objects {
		if err := validateObjectInput(value); err != nil {
			return Index{}, err
		}
		bindings = append(bindings, objectBinding{SourceRef: value.SourceRef})
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].SourceRef < bindings[j].SourceRef })
	for position := range bindings {
		if position > 0 && bindings[position-1].SourceRef == bindings[position].SourceRef {
			return Index{}, fmt.Errorf("program index: duplicate object source ref %q", bindings[position].SourceRef)
		}
	}
	ordinals := readingOrder(input, seedInputs)
	for position := range bindings {
		bindings[position].ID = compactID("n", ordinals[bindings[position].SourceRef])
	}
	for _, value := range input.Objects {
		id, err := resolveObjectRef(bindings, value.SourceRef)
		if err != nil {
			return Index{}, err
		}
		object := Object{
			ID: id, SourceRef: value.SourceRef,
			Kind: value.Kind, Name: value.Name, Visibility: value.Visibility,
			Signature: value.Signature, Location: cloneLocation(value.Location), EndLine: value.EndLine, CodeLines: value.CodeLines, Unreachable: value.Unreachable, Macro: value.Macro, Anonymous: value.Anonymous, Directory: value.Directory,
			DocstringRanges: canonicalDocstringRanges(value.DocstringRanges),
			External:        cloneExternalSymbol(value.External), Aliases: canonicalAliases(value.Aliases), Types: slices.Clone(value.Types),
			ParameterStores: canonicalParameterStores(value.ParameterStores), Rows: cloneRows(value.Rows),
			Comparisons: canonicalComparisons(value.Comparisons),
		}
		index.Objects = append(index.Objects, object)
	}
	for position, value := range input.Objects {
		ownerID, err := resolveObjectRef(bindings, value.OwnerRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: object %q owner: %w", value.SourceRef, err)
		}
		containerID, err := resolveObjectRef(bindings, value.ContainerRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: object %q container: %w", value.SourceRef, err)
		}
		if ownerID == index.Objects[position].ID || containerID == index.Objects[position].ID {
			return Index{}, fmt.Errorf("program index: object %q owns or contains itself", value.SourceRef)
		}
		index.Objects[position].OwnerID = ownerID
		index.Objects[position].ContainerID = containerID
		for _, values := range []struct {
			inputs []TypedNameInput
			sealed *[]TypedName
		}{{value.Parameters, &index.Objects[position].Parameters}, {value.Results, &index.Objects[position].Results}} {
			for _, typed := range values.inputs {
				typeID, err := resolveObjectRef(bindings, typed.TypeRef)
				if err != nil {
					return Index{}, fmt.Errorf("program index: object %q value type: %w", value.SourceRef, err)
				}
				*values.sealed = append(*values.sealed, TypedName{Name: typed.Name, Type: typed.Type, TypeID: typeID})
			}
		}
		for _, overload := range value.Overloads {
			sealed := Overload{Signature: overload.Signature, Location: cloneLocation(overload.Location), EndLine: overload.EndLine, CodeLines: overload.CodeLines}
			for _, values := range []struct {
				inputs []TypedNameInput
				sealed *[]TypedName
			}{{overload.Parameters, &sealed.Parameters}, {overload.Results, &sealed.Results}} {
				for _, typed := range values.inputs {
					typeID, err := resolveObjectRef(bindings, typed.TypeRef)
					if err != nil {
						return Index{}, fmt.Errorf("program index: object %q overload value type: %w", value.SourceRef, err)
					}
					*values.sealed = append(*values.sealed, TypedName{Name: typed.Name, Type: typed.Type, TypeID: typeID})
				}
			}
			index.Objects[position].Overloads = append(index.Objects[position].Overloads, sealed)
		}
		if !validOverloads(index.Objects[position].Kind, index.Objects[position].Location, index.Objects[position].Overloads) {
			return Index{}, fmt.Errorf("program index: object %q overloads are invalid", value.SourceRef)
		}
	}
	sort.Slice(index.Objects, func(i, j int) bool { return compactIDLess(index.Objects[i].ID, index.Objects[j].ID, "n") })
	for position := 1; position < len(index.Objects); position++ {
		if index.Objects[position-1].ID == index.Objects[position].ID {
			return Index{}, fmt.Errorf("program index: duplicate object identity %q", index.Objects[position].ID)
		}
	}
	index.Target.Seeds = make([]TargetSeed, 0, len(seedInputs))
	for _, seedInput := range seedInputs {
		id, err := resolveObjectRef(bindings, seedInput.ObjectRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: target seed: %w", err)
		}
		object, ok := objectWithID(index.Objects, id)
		if !ok || object.Kind == ObjectExternalSymbol {
			return Index{}, fmt.Errorf("program index: target seed %q is not a local program object", seedInput.ObjectRef)
		}
		index.Target.Seeds = append(index.Target.Seeds, TargetSeed{
			ObjectID: id,
			Kind:     seedInput.Kind,
			Location: cloneLocation(seedInput.Location),
		})
	}
	sort.Slice(index.Target.Seeds, func(i, j int) bool {
		return compareTargetSeeds(index.Target.Seeds[i], index.Target.Seeds[j]) < 0
	})
	if len(input.Target.Exports) > 0 {
		if !input.Target.ExportBasis.Valid() {
			return Index{}, fmt.Errorf("program index: target exports need their basis")
		}
		index.Target.ExportBasis = input.Target.ExportBasis
	}
	for _, export := range input.Target.Exports {
		if !validText(export.ObjectRef) || export.Location == nil || !validLocation(*export.Location) {
			return Index{}, fmt.Errorf("program index: invalid target export input")
		}
		id, err := resolveObjectRef(bindings, export.ObjectRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: target export: %w", err)
		}
		index.Target.Exports = append(index.Target.Exports, TargetExport{ObjectID: id, Location: cloneLocation(export.Location)})
	}
	sort.Slice(index.Target.Exports, func(i, j int) bool {
		return compactIDLess(index.Target.Exports[i].ObjectID, index.Target.Exports[j].ObjectID, "n")
	})
	index.Target.Exports = slices.CompactFunc(index.Target.Exports, func(a, b TargetExport) bool { return a.ObjectID == b.ObjectID })
	index.Target.ID = input.Target.ID
	if index.Target.ID == "" {
		index.Target.ID = "t1"
	}
	if !validCompactID(index.Target.ID, "t") {
		return Index{}, fmt.Errorf("program index: invalid target input ID %q", index.Target.ID)
	}

	relationIDs, err := compactRelationIDs(input.Relations, bindings)
	if err != nil {
		return Index{}, err
	}
	pendingSourceArguments := make(map[string]PatternArgumentRefInput)
	pendingValueSourceArguments := make(map[string]pendingPatternValueSourceArguments)
	pendingSameValues := make(map[string]PatternRefInput)
	for relationPosition, value := range input.Relations {
		if !validText(value.SourceRef) || !value.Kind.Valid() || !value.Resolution.Valid() || !validText(value.FromRef) ||
			!validInvocation(value.Invocation) || !validDispatch(value.Dispatch) || !validOptionalLocation(value.Location) {
			return Index{}, fmt.Errorf("program index: invalid relation input")
		}
		fromID, err := resolveObjectRef(bindings, value.FromRef)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q source: %w", value.SourceRef, err)
		}
		toRefs := cloneStrings(value.ToRefs)
		for _, ref := range toRefs {
			if !validText(ref) {
				return Index{}, fmt.Errorf("program index: relation %q has invalid target ref", value.SourceRef)
			}
		}
		sort.Strings(toRefs)
		toRefs = compactStrings(toRefs)
		toIDs := make([]string, 0, len(toRefs))
		for _, ref := range toRefs {
			id, resolveErr := resolveObjectRef(bindings, ref)
			if resolveErr != nil {
				return Index{}, fmt.Errorf("program index: relation %q target: %w", value.SourceRef, resolveErr)
			}
			toIDs = append(toIDs, id)
		}
		sort.Strings(toIDs)
		toIDs = compactStrings(toIDs)

		named := cloneWitnesses(value.Witnesses)
		for position := range named {
			if named[position].ObjectID != "" {
				return Index{}, fmt.Errorf("program index: relation %q witness names an object without a ref", value.SourceRef)
			}
			if ref := named[position].ObjectRef; ref != "" {
				id, resolveErr := resolveObjectRef(bindings, ref)
				if resolveErr != nil {
					return Index{}, fmt.Errorf("program index: relation %q witness: %w", value.SourceRef, resolveErr)
				}
				named[position].ObjectID, named[position].ObjectRef = id, ""
			}
		}
		witnesses, err := canonicalWitnesses(named)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q: %w", value.SourceRef, err)
		}
		relationID := relationIDs[relationPosition]
		patterns, err := canonicalizeRelationPatterns(
			value.Patterns, relationID, bindings, pendingValueSourceArguments, pendingSameValues,
		)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q patterns: %w", value.SourceRef, err)
		}
		relation := Relation{
			ID:        relationID,
			SourceRef: value.SourceRef, Kind: value.Kind, FromID: fromID, ToIDs: toIDs,
			Resolution: value.Resolution, Invocation: value.Invocation, Dispatch: value.Dispatch, Location: cloneLocation(value.Location),
			TargetsObserved: value.TargetsObserved, TargetsOmitted: value.TargetsObserved - len(toIDs),
			Witnesses: witnesses, WitnessesObserved: value.WitnessesObserved,
			WitnessesOmitted: value.WitnessesObserved - len(witnesses),
			Patterns:         patterns, PatternsObserved: value.PatternsObserved,
			PatternsOmitted: value.PatternsObserved - len(patterns),
			FieldPath:       value.FieldPath,
			Basis:           value.Basis,
			Guard:           cloneGuard(value.Guard),
			Value:           sourcevalue.Clone(value.Value),
		}
		if value.SourceArgument != nil {
			if value.Kind != RelationPassesCallback || !validPatternArgumentRefInput(*value.SourceArgument) {
				return Index{}, fmt.Errorf("program index: relation %q has invalid source argument input", value.SourceRef)
			}
			pendingSourceArguments[relationID] = *value.SourceArgument
		}
		index.Relations = append(index.Relations, relation)
	}
	if err := resolvePatternValueSourceArgumentReferences(index.Relations, pendingValueSourceArguments); err != nil {
		return Index{}, err
	}
	if err := resolveSameValuePatterns(index.Relations, pendingSameValues); err != nil {
		return Index{}, err
	}
	for position := range index.Relations {
		reference, ok := pendingSourceArguments[index.Relations[position].ID]
		if !ok {
			continue
		}
		argumentID, err := resolvePatternArgumentReference(index.Relations, reference)
		if err != nil {
			return Index{}, fmt.Errorf("program index: relation %q source argument: %w", index.Relations[position].SourceRef, err)
		}
		index.Relations[position].SourceArgumentID = argumentID
	}
	for _, relation := range index.Relations {
		if err := validateRelationShape(relation); err != nil {
			return Index{}, err
		}
	}
	sort.Slice(index.Relations, func(i, j int) bool { return compactIDLess(index.Relations[i].ID, index.Relations[j].ID, "e") })
	for position := 1; position < len(index.Relations); position++ {
		if index.Relations[position-1].ID == index.Relations[position].ID {
			return Index{}, fmt.Errorf("program index: duplicate relation identity %q", index.Relations[position].ID)
		}
	}

	if !input.Coverage.Measured {
		return Index{}, fmt.Errorf("program index: adapter coverage was not measured")
	}
	objectsObserved := input.Coverage.ObjectsObserved
	relationsObserved := input.Coverage.RelationsObserved
	index.Coverage = compileCoverage(index.Objects, index.Relations, objectsObserved, relationsObserved)
	if err := validateCoverage(index.Coverage, len(index.Objects), len(index.Relations)); err != nil {
		return Index{}, err
	}
	digest, err := indexDigest(index)
	if err != nil {
		return Index{}, err
	}
	index.SHA256 = digest
	if err := index.Validate(); err != nil {
		return Index{}, err
	}
	return index, nil
}

// Snapshot returns a deep copy suitable for a consumer-owned handoff.
func (index Index) Snapshot() Index {
	result := index
	result.Target = index.Target.Snapshot()
	result.Objects = make([]Object, len(index.Objects))
	copy(result.Objects, index.Objects)
	for position := range result.Objects {
		result.Objects[position].Location = cloneLocation(index.Objects[position].Location)
		result.Objects[position].DocstringRanges = slices.Clone(index.Objects[position].DocstringRanges)
		result.Objects[position].External = cloneExternalSymbol(index.Objects[position].External)
		result.Objects[position].Aliases = slices.Clone(index.Objects[position].Aliases)
		result.Objects[position].Types = slices.Clone(index.Objects[position].Types)
		result.Objects[position].ParameterStores = canonicalParameterStores(index.Objects[position].ParameterStores)
		result.Objects[position].Rows = cloneRows(index.Objects[position].Rows)
		result.Objects[position].Comparisons = cloneComparisons(index.Objects[position].Comparisons)
		result.Objects[position].Overloads = cloneOverloads(index.Objects[position].Overloads)
	}
	result.Relations = make([]Relation, len(index.Relations))
	copy(result.Relations, index.Relations)
	for position := range result.Relations {
		result.Relations[position].ToIDs = cloneStrings(index.Relations[position].ToIDs)
		result.Relations[position].Location = cloneLocation(index.Relations[position].Location)
		result.Relations[position].Witnesses = cloneWitnesses(index.Relations[position].Witnesses)
		result.Relations[position].Patterns = cloneRelationPatterns(index.Relations[position].Patterns)
		result.Relations[position].Value = sourcevalue.Clone(index.Relations[position].Value)
	}
	result.Categorization = cloneCategorization(index.Categorization)
	if index.FieldWrites != nil {
		result.FieldWrites = make([]FieldWrites, len(index.FieldWrites))
		for position, writes := range index.FieldWrites {
			result.FieldWrites[position] = FieldWrites{Field: writes.Field, Value: *sourcevalue.Clone(&writes.Value)}
		}
	}
	return result
}

// Validate checks identity bindings, canonical order, references, bounds,
// coverage and the complete-index SHA seal.
func (index Index) Validate() error {
	if index.Version != Version || !validSHA256(index.ScenarioSHA256) || !validSHA256(index.SourceSHA256) {
		return fmt.Errorf("program index: invalid producer identity")
	}
	if index.Objects == nil || index.Relations == nil {
		return fmt.Errorf("program index: missing collections")
	}
	if err := index.Target.Validate(); err != nil {
		return err
	}
	for position, writes := range index.FieldWrites {
		if !validText(writes.Field) || position > 0 && index.FieldWrites[position-1].Field >= writes.Field {
			return fmt.Errorf("program index: field writes %q out of order or invalid", writes.Field)
		}
		if err := sourcevalue.Validate(&writes.Value); err != nil {
			return fmt.Errorf("program index: field writes %q: %w", writes.Field, err)
		}
	}
	for position, object := range index.Objects {
		if err := validateObject(object); err != nil {
			return err
		}
		if position > 0 && !compactIDLess(index.Objects[position-1].ID, object.ID, "n") {
			return fmt.Errorf("program index: objects are not canonical")
		}
	}
	for _, object := range index.Objects {
		if object.OwnerID != "" {
			owner, ok := objectWithID(index.Objects, object.OwnerID)
			if object.OwnerID == object.ID || !ok {
				return fmt.Errorf("program index: object %q has invalid owner", object.ID)
			}
			if object.Kind == ObjectMethod && owner.Kind != ObjectType {
				return fmt.Errorf("program index: method %q owner is not a type", object.ID)
			}
		}
		if object.ContainerID != "" {
			if object.ContainerID == object.ID || !hasObjectID(index.Objects, object.ContainerID) {
				return fmt.Errorf("program index: object %q has invalid container", object.ID)
			}
		}
	}
	for _, seed := range index.Target.Seeds {
		object, ok := objectWithID(index.Objects, seed.ObjectID)
		if !ok || object.Kind == ObjectExternalSymbol {
			return fmt.Errorf("program index: target has invalid seed object %q", seed.ObjectID)
		}
		if err := validateTargetSeedBinding(seed, object); err != nil {
			return err
		}
	}
	for _, export := range index.Target.Exports {
		// An export is a callable at its own declaration.
		object, ok := objectWithID(index.Objects, export.ObjectID)
		if !ok || object.Kind != ObjectFunction && object.Kind != ObjectMethod || object.Location == nil ||
			object.Location.Path != export.Location.Path || object.Location.Line != export.Location.Line {
			return fmt.Errorf("program index: target export %q is no local callable at its declaration", export.ObjectID)
		}
	}
	type patternArgumentAuthority struct {
		fromID         string
		resolution     Resolution
		targetCount    int
		targetsOmitted int
		argument       PatternArgument
	}
	argumentsByID := make(map[string]patternArgumentAuthority)
	for _, relation := range index.Relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if _, exists := argumentsByID[argument.ID]; exists {
					return fmt.Errorf("program index: duplicate pattern argument identity %q", argument.ID)
				}
				argumentsByID[argument.ID] = patternArgumentAuthority{
					fromID: relation.FromID, resolution: relation.Resolution,
					targetCount: len(relation.ToIDs), targetsOmitted: relation.TargetsOmitted,
					argument: argument,
				}
			}
		}
	}
	for position, relation := range index.Relations {
		if err := validateRelationShape(relation); err != nil {
			return err
		}
		if !hasObjectID(index.Objects, relation.FromID) {
			return fmt.Errorf("program index: relation %q has unknown source", relation.ID)
		}
		for _, id := range relation.ToIDs {
			if !hasObjectID(index.Objects, id) {
				return fmt.Errorf("program index: relation %q has unknown target", relation.ID)
			}
		}
		if relation.FieldPath != "" {
			field, _ := objectWithID(index.Objects, relation.ToIDs[0])
			owner, _ := objectWithID(index.Objects, field.OwnerID)
			if field.Kind != ObjectVariable || owner.Kind != ObjectType {
				return fmt.Errorf("program index: relation %q has a field path and no field", relation.ID)
			}
		}
		for _, witness := range relation.Witnesses {
			if witness.ObjectID != "" && !hasObjectID(index.Objects, witness.ObjectID) {
				return fmt.Errorf("program index: relation %q witness names an unknown object", relation.ID)
			}
		}
		if !validTableReadForm(index.Objects, relation) {
			return fmt.Errorf("program index: relation %q has a table read form off a read of variables", relation.ID)
		}
		for _, pattern := range relation.Patterns {
			for _, witness := range pattern.Context {
				if witness.ObjectID != "" {
					return fmt.Errorf("program index: pattern %q control context names an object", pattern.ID)
				}
			}
			if pattern.ResultID != "" && !hasObjectID(index.Objects, pattern.ResultID) {
				return fmt.Errorf("program index: pattern %q has unknown result", pattern.ID)
			}
			if pattern.ReceiverID != "" && !hasObjectID(index.Objects, pattern.ReceiverID) {
				return fmt.Errorf("program index: pattern %q has unknown receiver", pattern.ID)
			}
			for _, id := range pattern.ReceiverOriginIDs {
				if !hasObjectID(index.Objects, id) {
					return fmt.Errorf("program index: pattern %q has unknown receiver origin", pattern.ID)
				}
			}
			for _, argument := range pattern.Arguments {
				for _, id := range argument.ObjectIDs {
					if !hasObjectID(index.Objects, id) {
						return fmt.Errorf("program index: pattern argument %q has unknown object", argument.ID)
					}
				}
				for _, candidate := range argument.ValueCandidates {
					for _, id := range candidate.SourceObjectIDs {
						if !hasObjectID(index.Objects, id) {
							return fmt.Errorf("program index: pattern value candidate %q has unknown source object", candidate.ID)
						}
					}
					for _, id := range candidate.SourceArgumentIDs {
						if id == argument.ID {
							return fmt.Errorf("program index: pattern value candidate %q cites its owning argument", candidate.ID)
						}
						authority, ok := argumentsByID[id]
						if !ok {
							return fmt.Errorf("program index: pattern value candidate %q has unknown source argument", candidate.ID)
						}
						if candidate.SourceKind == PatternValueSourceActualArgument &&
							(authority.resolution != ResolutionExact || authority.targetCount != 1 || authority.targetsOmitted != 0 ||
								!samePatternValue(candidate.Kind, candidate.Value, candidate.Parts, authority.argument)) {
							return fmt.Errorf("program index: pattern value candidate %q has incompatible actual source argument", candidate.ID)
						}
					}
				}
			}
		}
		if relation.SourceArgumentID != "" {
			authority, ok := argumentsByID[relation.SourceArgumentID]
			if !ok {
				return fmt.Errorf("program index: relation %q has unknown source argument", relation.ID)
			}
			argument := authority.argument
			targetsWithinArgument := true
			for _, targetID := range relation.ToIDs {
				if !slices.Contains(argument.ObjectIDs, targetID) {
					targetsWithinArgument = false
					break
				}
			}
			// A neutral argument may resolve to callable and non-callable
			// declarations for the same language symbol. The callback relation
			// retains only callable targets, so it is authoritative when those
			// targets are a measured subset of the source argument authority.
			if authority.fromID != relation.FromID || argument.Resolution != relation.Resolution ||
				!targetsWithinArgument || argument.ObjectsObserved != relation.TargetsObserved {
				return fmt.Errorf("program index: relation %q source argument authority mismatch", relation.ID)
			}
		}
		if position > 0 && !compactIDLess(index.Relations[position-1].ID, relation.ID, "e") {
			return fmt.Errorf("program index: relations are not canonical")
		}
	}
	if err := validateSameValuePatterns(index.Relations); err != nil {
		return err
	}
	if err := validateCoverage(index.Coverage, len(index.Objects), len(index.Relations)); err != nil {
		return err
	}
	wantCoverage := compileCoverage(index.Objects, index.Relations, index.Coverage.ObjectsObserved, index.Coverage.RelationsObserved)
	if index.Coverage != wantCoverage {
		return fmt.Errorf("program index: coverage mismatch")
	}
	if err := validateCategorization(index); err != nil {
		return err
	}
	want, err := indexDigest(index)
	if err != nil {
		return err
	}
	if !validSHA256(index.SHA256) || index.SHA256 != want {
		return fmt.Errorf("program index: sha256 mismatch")
	}
	return nil
}

type objectBinding struct {
	SourceRef string
	ID        string
}

func compactRelationIDs(values []RelationInput, bindings []objectBinding) ([]string, error) {
	type candidate struct {
		position int
		key      string
	}
	candidates := make([]candidate, 0, len(values))
	for position, value := range values {
		if !validText(value.SourceRef) || !value.Kind.Valid() || !value.Resolution.Valid() || !validText(value.FromRef) {
			return nil, fmt.Errorf("program index: invalid relation input")
		}
		fromID, err := resolveObjectRef(bindings, value.FromRef)
		if err != nil {
			return nil, fmt.Errorf("program index: relation %q source: %w", value.SourceRef, err)
		}
		candidates = append(candidates, candidate{
			position: position,
			key:      strings.Join([]string{value.SourceRef, string(value.Kind), fromID}, "\x00"),
		})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].key < candidates[j].key })
	for position := 1; position < len(candidates); position++ {
		if candidates[position-1].key == candidates[position].key {
			return nil, fmt.Errorf("program index: duplicate relation identity input")
		}
	}
	// Relations read in the order of their source objects, then of their
	// sites: e1 is what n1 does first.
	sort.SliceStable(candidates, func(i, j int) bool {
		a, b := values[candidates[i].position], values[candidates[j].position]
		if a.FromRef != b.FromRef {
			aID, _ := resolveObjectRef(bindings, a.FromRef)
			bID, _ := resolveObjectRef(bindings, b.FromRef)
			return compactIDLess(aID, bID, "n")
		}
		if c := compareOptionalLocations(a.Location, b.Location); c != 0 {
			return c < 0
		}
		return candidates[i].key < candidates[j].key
	})
	ids := make([]string, len(values))
	for position, candidate := range candidates {
		ids[candidate.position] = compactID("e", position+1)
	}
	return ids, nil
}

// readingOrder numbers objects the way a reader meets them: the launch seeds
// first, then breadth-first along calls and bindings in source order, each
// declaration followed by its owner. What no entry reaches follows by file
// and line; objects without a location, then external symbols, come last.
func readingOrder(input Input, seeds []TargetSeedInput) map[string]int {
	objects := make(map[string]ObjectInput, len(input.Objects))
	for _, object := range input.Objects {
		objects[object.SourceRef] = object
	}
	type edge struct {
		to       string
		location *Location
	}
	next := make(map[string][]edge)
	for _, relation := range input.Relations {
		if relation.Kind == RelationImports {
			continue
		}
		for _, to := range relation.ToRefs {
			next[relation.FromRef] = append(next[relation.FromRef], edge{to, relation.Location})
		}
	}
	for from := range next {
		edges := next[from]
		sort.SliceStable(edges, func(i, j int) bool {
			if c := compareOptionalLocations(edges[i].location, edges[j].location); c != 0 {
				return c < 0
			}
			return edges[i].to < edges[j].to
		})
	}
	ordinals := make(map[string]int, len(input.Objects))
	var queue []string
	visit := func(ref string) {
		if _, seen := ordinals[ref]; seen || ref == "" {
			return
		}
		if _, known := objects[ref]; !known {
			return
		}
		ordinals[ref] = len(ordinals) + 1
		queue = append(queue, ref)
	}
	for _, seed := range seeds {
		visit(seed.ObjectRef)
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, edge := range next[current] {
			visit(edge.to)
		}
		visit(objects[current].OwnerRef)
		visit(objects[current].ContainerRef)
	}
	rest := make([]ObjectInput, 0, len(input.Objects))
	for _, object := range input.Objects {
		if _, seen := ordinals[object.SourceRef]; !seen {
			rest = append(rest, object)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		a, b := rest[i], rest[j]
		if (a.Kind == ObjectExternalSymbol) != (b.Kind == ObjectExternalSymbol) {
			return b.Kind == ObjectExternalSymbol
		}
		if c := compareOptionalLocations(a.Location, b.Location); c != 0 {
			return c < 0
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.SourceRef < b.SourceRef
	})
	for _, object := range rest {
		ordinals[object.SourceRef] = len(ordinals) + 1
	}
	return ordinals
}

// compareOptionalLocations orders located values by path, line and column;
// an unlocated value sorts after every located one.
func compareOptionalLocations(a, b *Location) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	}
	if a.Path != b.Path {
		return strings.Compare(a.Path, b.Path)
	}
	if a.Line != b.Line {
		return a.Line - b.Line
	}
	return a.Column - b.Column
}

func compactID(prefix string, ordinal int) string {
	return prefix + strconv.Itoa(ordinal)
}

func validCompactID(value, prefix string) bool {
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return false
	}
	ordinal, err := strconv.Atoi(value[len(prefix):])
	return err == nil && ordinal > 0 && compactID(prefix, ordinal) == value
}

func compactIDLess(left, right, prefix string) bool {
	leftOrdinal, leftErr := strconv.Atoi(strings.TrimPrefix(left, prefix))
	rightOrdinal, rightErr := strconv.Atoi(strings.TrimPrefix(right, prefix))
	if leftErr != nil || rightErr != nil {
		return left < right
	}
	return leftOrdinal < rightOrdinal
}

// ValidTargetID reports whether value is a canonical repository-local tN ID.
func ValidTargetID(value string) bool {
	return validCompactID(value, "t")
}

// TargetIDLess orders compact target IDs by ordinal, so t10 follows t9.
func TargetIDLess(left, right string) bool {
	return compactIDLess(left, right, "t")
}

type pendingPatternValueSourceArguments struct {
	Refs     []PatternArgumentRefInput
	Observed int
}

func canonicalizeTargetSources(values []TargetSource) ([]TargetSource, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("program index: target has no source evidence")
	}
	result := cloneTargetSources(values)
	pathsByRef := make(map[string]string, len(result))
	refsByPath := make(map[string]string, len(result))
	for _, source := range result {
		if !validText(source.FileRef) || !validPath(source.Path) {
			return nil, fmt.Errorf("program index: invalid target source")
		}
		if previous, exists := pathsByRef[source.FileRef]; exists && previous != source.Path {
			return nil, fmt.Errorf("program index: target file ref %q has conflicting paths", source.FileRef)
		}
		if previous, exists := refsByPath[source.Path]; exists && previous != source.FileRef {
			return nil, fmt.Errorf("program index: target source path %q has conflicting file refs", source.Path)
		}
		pathsByRef[source.FileRef] = source.Path
		refsByPath[source.Path] = source.FileRef
	}
	sort.Slice(result, func(i, j int) bool { return compareTargetSources(result[i], result[j]) < 0 })
	compacted := result[:0]
	for _, source := range result {
		if len(compacted) == 0 || compareTargetSources(compacted[len(compacted)-1], source) != 0 {
			compacted = append(compacted, source)
		}
	}
	return compacted, nil
}

func canonicalizeTargetSeedInputs(values []TargetSeedInput) ([]TargetSeedInput, error) {
	result := make([]TargetSeedInput, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
		seed := result[position]
		if !validText(seed.ObjectRef) || !seed.Kind.Valid() || seed.Location == nil || !validLocation(*seed.Location) {
			return nil, fmt.Errorf("program index: invalid target seed input")
		}
	}
	sort.Slice(result, func(i, j int) bool { return compareTargetSeedInputs(result[i], result[j]) < 0 })
	compacted := result[:0]
	for _, seed := range result {
		if len(compacted) == 0 || compareTargetSeedInputs(compacted[len(compacted)-1], seed) != 0 {
			compacted = append(compacted, seed)
		}
	}
	return compacted, nil
}

func compareTargetSources(left, right TargetSource) int {
	if order := strings.Compare(left.FileRef, right.FileRef); order != 0 {
		return order
	}
	return strings.Compare(left.Path, right.Path)
}

func compareTargetSeedInputs(left, right TargetSeedInput) int {
	if order := strings.Compare(left.ObjectRef, right.ObjectRef); order != 0 {
		return order
	}
	if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
		return order
	}
	return strings.Compare(locationKey(left.Location), locationKey(right.Location))
}

func compareTargetSeeds(left, right TargetSeed) int {
	if order := strings.Compare(left.ObjectID, right.ObjectID); order != 0 {
		return order
	}
	if order := strings.Compare(string(left.Kind), string(right.Kind)); order != 0 {
		return order
	}
	return strings.Compare(locationKey(left.Location), locationKey(right.Location))
}

func hasTargetSourceRef(sources []TargetSource, wanted string) bool {
	position := sort.Search(len(sources), func(position int) bool {
		return sources[position].FileRef >= wanted
	})
	return position < len(sources) && sources[position].FileRef == wanted
}

func hasTargetSourcePath(sources []TargetSource, wanted string) bool {
	for _, source := range sources {
		if source.Path == wanted {
			return true
		}
	}
	return false
}

func resolveObjectRef(bindings []objectBinding, ref string) (string, error) {
	if ref == "" {
		return "", nil
	}
	position := sort.Search(len(bindings), func(position int) bool { return bindings[position].SourceRef >= ref })
	if position == len(bindings) || bindings[position].SourceRef != ref {
		return "", fmt.Errorf("unknown object ref %q", ref)
	}
	return bindings[position].ID, nil
}

// validTableReadForm checks the shared table-read witnesses: a membership or
// keys form only on a read of variables, one of the two per site, and a keys
// witness naming a variable other than the one read.
func validTableReadForm(objects []Object, relation Relation) bool {
	membership, keys := false, false
	for _, witness := range relation.Witnesses {
		switch witness.Kind {
		case WitnessMembership:
			membership = true
		case WitnessKeys:
			keys = true
			if object, _ := objectWithID(objects, witness.ObjectID); object.Kind != ObjectVariable || slices.Contains(relation.ToIDs, witness.ObjectID) {
				return false
			}
		}
	}
	if !membership && !keys {
		return true
	}
	if membership && keys || relation.Kind != RelationReads {
		return false
	}
	for _, id := range relation.ToIDs {
		if object, _ := objectWithID(objects, id); object.Kind != ObjectVariable {
			return false
		}
	}
	return true
}

func hasObjectID(objects []Object, id string) bool {
	position := sort.Search(len(objects), func(position int) bool { return !compactIDLess(objects[position].ID, id, "n") })
	return position < len(objects) && objects[position].ID == id
}

func objectWithID(objects []Object, id string) (Object, bool) {
	position := sort.Search(len(objects), func(position int) bool { return !compactIDLess(objects[position].ID, id, "n") })
	if position == len(objects) || objects[position].ID != id {
		return Object{}, false
	}
	return objects[position], true
}

func validateTargetShape(target Target) error {
	if !validText(target.Language) || !validText(target.Kind) || !validText(target.Name) ||
		!validText(target.Selector) || len(target.Sources) == 0 ||
		!validText(target.AnchorFileRef) || !hasTargetSourceRef(target.Sources, target.AnchorFileRef) ||
		target.Seeds == nil {
		return fmt.Errorf("program index: invalid target")
	}
	for position, source := range target.TestSources {
		if !validPath(source) || position > 0 && target.TestSources[position-1] >= source {
			return fmt.Errorf("program index: invalid or noncanonical test sources")
		}
	}
	for position, name := range target.Executables {
		if !validText(name) || strings.ContainsAny(name, "/\\ \t\r\n") || position > 0 && target.Executables[position-1] >= name {
			return fmt.Errorf("program index: invalid or noncanonical executables")
		}
	}
	for position, name := range target.Libraries {
		if !validText(name) || strings.ContainsAny(name, "/\\ \t\r\n") || position > 0 && target.Libraries[position-1] >= name {
			return fmt.Errorf("program index: invalid or noncanonical libraries")
		}
	}
	pathsByRef := make(map[string]string, len(target.Sources))
	refsByPath := make(map[string]string, len(target.Sources))
	for position, source := range target.Sources {
		if !validText(source.FileRef) || !validPath(source.Path) {
			return fmt.Errorf("program index: invalid target source")
		}
		if position > 0 && compareTargetSources(target.Sources[position-1], source) >= 0 {
			return fmt.Errorf("program index: target sources are not canonical")
		}
		if previous, exists := pathsByRef[source.FileRef]; exists && previous != source.Path {
			return fmt.Errorf("program index: target file ref %q has conflicting paths", source.FileRef)
		}
		if previous, exists := refsByPath[source.Path]; exists && previous != source.FileRef {
			return fmt.Errorf("program index: target source path %q has conflicting file refs", source.Path)
		}
		pathsByRef[source.FileRef] = source.Path
		refsByPath[source.Path] = source.FileRef
	}
	for position, seed := range target.Seeds {
		if !validText(seed.ObjectID) || !seed.Kind.Valid() || seed.Location == nil || !validLocation(*seed.Location) ||
			!hasTargetSourcePath(target.Sources, seed.Location.Path) {
			return fmt.Errorf("program index: invalid target seed")
		}
		if position > 0 && compareTargetSeeds(target.Seeds[position-1], seed) >= 0 {
			return fmt.Errorf("program index: target seeds are not canonical")
		}
	}
	if len(target.Exports) > 0 != (target.ExportBasis != "") || target.ExportBasis != "" && !target.ExportBasis.Valid() {
		return fmt.Errorf("program index: invalid target export basis")
	}
	for position, export := range target.Exports {
		if !validText(export.ObjectID) || export.Location == nil || !validLocation(*export.Location) ||
			position > 0 && !compactIDLess(target.Exports[position-1].ObjectID, export.ObjectID, "n") {
			return fmt.Errorf("program index: invalid or noncanonical target exports")
		}
	}
	return nil
}

func validateObjectInput(value ObjectInput) error {
	if !value.Visibility.Valid() {
		return fmt.Errorf("program index: invalid object visibility")
	}
	if !validText(value.SourceRef) || !value.Kind.Valid() || !validText(value.Name) ||
		!validOptionalText(value.Signature) || !validOptionalText(value.OwnerRef) ||
		!validOptionalText(value.ContainerRef) || !validOptionalLocation(value.Location) || !validObjectDirectory(value.Kind, value.Directory) ||
		!validDocstringRanges(value.Location, canonicalDocstringRanges(value.DocstringRanges)) ||
		!validAliases(canonicalAliases(value.Aliases)) || !validTypeLocations(value.Kind, value.Types) || !validEndLine(value.Location, value.EndLine) || !validCodeLines(value.Location, value.EndLine, value.CodeLines) ||
		(value.Unreachable || value.Macro) && !callableKind(value.Kind) || value.Anonymous && !callableKind(value.Kind) ||
		!validParameterStores(value.Kind, canonicalParameterStores(value.ParameterStores)) || !validRows(value.Kind, value.Rows) ||
		!validComparisons(value.Kind, canonicalComparisons(value.Comparisons)) {
		return fmt.Errorf("program index: invalid object input")
	}
	for _, typed := range append(append([]TypedNameInput(nil), value.Parameters...), value.Results...) {
		if !validOptionalText(typed.Name) || !validOptionalText(typed.Type) || typed.Name == "" && typed.Type == "" || !validOptionalText(typed.TypeRef) {
			return fmt.Errorf("program index: invalid object value type")
		}
	}
	if err := validateExternalSymbolBinding(value.Kind, value.External); err != nil {
		return err
	}
	return nil
}

func validateObject(value Object) error {
	if !validCompactID(value.ID, "n") || !value.Kind.Valid() || !validText(value.Name) || !value.Visibility.Valid() ||
		!validOptionalText(value.Signature) || !validOptionalText(value.OwnerID) ||
		!validOptionalText(value.ContainerID) || !validOptionalLocation(value.Location) || !validObjectDirectory(value.Kind, value.Directory) ||
		!validDocstringRanges(value.Location, value.DocstringRanges) ||
		!validAliases(value.Aliases) || !validTypeLocations(value.Kind, value.Types) || !validEndLine(value.Location, value.EndLine) || !validCodeLines(value.Location, value.EndLine, value.CodeLines) ||
		(value.Unreachable || value.Macro) && !callableKind(value.Kind) || value.Anonymous && !callableKind(value.Kind) ||
		!validParameterStores(value.Kind, value.ParameterStores) || !validRows(value.Kind, value.Rows) ||
		!validComparisons(value.Kind, value.Comparisons) || !validOverloads(value.Kind, value.Location, value.Overloads) {
		return fmt.Errorf("program index: invalid object")
	}
	for _, typed := range append(append([]TypedName(nil), value.Parameters...), value.Results...) {
		if !validOptionalText(typed.Name) || !validOptionalText(typed.Type) || typed.Name == "" && typed.Type == "" || typed.TypeID != "" && !validCompactID(typed.TypeID, "n") {
			return fmt.Errorf("program index: invalid object value type")
		}
	}
	if err := validateExternalSymbolBinding(value.Kind, value.External); err != nil {
		return err
	}
	return nil
}

func canonicalDocstringRanges(lines []LineRange) []LineRange {
	if len(lines) == 0 {
		return nil
	}
	owned := slices.Clone(lines)
	slices.SortFunc(owned, compareDocstringRanges)
	return slices.Compact(owned)
}

func compareDocstringRanges(a, b LineRange) int {
	for _, pair := range [][2]int{{a.Line, b.Line}, {a.Column, b.Column}, {a.EndLine, b.EndLine}, {a.EndColumn, b.EndColumn}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func validDocstringRanges(location *Location, lines []LineRange) bool {
	if len(lines) > 0 && (location == nil || !validOptionalLocation(location)) {
		return false
	}
	for position, line := range lines {
		if line.Line < 1 || line.Column < 1 || line.EndLine < line.Line || line.EndColumn < 1 || line.EndLine == line.Line && line.EndColumn <= line.Column || position > 0 && compareDocstringRanges(lines[position-1], line) >= 0 {
			return false
		}
	}
	return true
}

func validateTargetSeedBinding(seed TargetSeed, object Object) error {
	compatible := false
	sameLine := false
	switch seed.Kind {
	case SeedCallable:
		compatible = object.Kind == ObjectFunction || object.Kind == ObjectMethod || object.Kind == ObjectLambda
		sameLine = true
	case SeedModule:
		compatible = object.Kind == ObjectModule || object.Kind == ObjectPackage
		sameLine = true
	case SeedMainGuard, SeedScript:
		compatible = object.Kind == ObjectModule || object.Kind == ObjectPackage
	case SeedBoundObject:
		compatible = object.Kind == ObjectVariable || object.Kind == ObjectType
		sameLine = true
	}
	if !compatible || object.Location == nil || seed.Location == nil || object.Location.Path != seed.Location.Path ||
		sameLine && object.Location.Line != seed.Location.Line {
		return fmt.Errorf("program index: target seed is incompatible with object %q", object.ID)
	}
	return nil
}

func validateRelationShape(value Relation) error {
	if !validCompactID(value.ID, "e") || !value.Kind.Valid() || !value.Resolution.Valid() ||
		!validText(value.FromID) || !validInvocation(value.Invocation) || !validDispatch(value.Dispatch) || !validOptionalLocation(value.Location) ||
		!validOptionalText(value.SourceArgumentID) ||
		value.ToIDs == nil || value.Witnesses == nil || value.Patterns == nil ||
		!canonicalStrings(value.ToIDs) {
		return fmt.Errorf("program index: invalid relation")
	}
	if value.SourceArgumentID != "" && value.Kind != RelationPassesCallback {
		return fmt.Errorf("program index: source argument is only valid for callback transfer")
	}
	if value.FieldPath != "" && (!validText(value.FieldPath) || value.Kind != RelationReads && value.Kind != RelationWrites || len(value.ToIDs) != 1) {
		return fmt.Errorf("program index: a field path belongs to a read or write of one field")
	}
	if value.Value != nil && (value.Kind != RelationWrites || value.FieldPath == "" || sourcevalue.Validate(value.Value) != nil) {
		return fmt.Errorf("program index: a written value belongs to a write of one field")
	}
	if value.Basis != "" && (value.Basis != BasisImplements || value.Kind != RelationCalls || value.Dispatch != DispatchInterface || value.Resolution == ResolutionUnresolved) {
		return fmt.Errorf("program index: an implements basis belongs to a resolved interface call")
	}
	if guard := value.Guard; guard != nil && (guard.Kind != GuardBranch && guard.Kind != GuardError && guard.Kind != GuardNoReturn ||
		!validOptionalLocation(guard.Location) || !validGuardCondition(guard) ||
		value.Kind != RelationCalls && value.Kind != RelationInvokesExternal && value.Kind != RelationPassesCallback) {
		return fmt.Errorf("program index: a guard belongs to a call or a callback handed over, with a known kind")
	}
	if value.TargetsObserved <= 0 || value.TargetsObserved < len(value.ToIDs) ||
		value.TargetsOmitted != value.TargetsObserved-len(value.ToIDs) ||
		value.WitnessesObserved <= 0 || value.WitnessesObserved < len(value.Witnesses) ||
		value.WitnessesOmitted != value.WitnessesObserved-len(value.Witnesses) ||
		value.PatternsObserved < len(value.Patterns) ||
		value.PatternsOmitted != value.PatternsObserved-len(value.Patterns) {
		return fmt.Errorf("program index: invalid relation coverage")
	}
	previousWitness := ""
	for _, witness := range value.Witnesses {
		if err := validateWitness(witness); err != nil {
			return err
		}
		key := witnessKey(witness)
		if previousWitness != "" && previousWitness >= key {
			return fmt.Errorf("program index: witnesses are not canonical")
		}
		previousWitness = key
	}
	for position, pattern := range value.Patterns {
		if err := validateRelationPatternShape(pattern, value.ID); err != nil {
			return err
		}
		if pattern.ID != value.ID+"p"+strconv.Itoa(position+1) {
			return fmt.Errorf("program index: relation patterns are not canonical")
		}
	}
	switch value.Resolution {
	case ResolutionExact:
		if len(value.ToIDs) != 1 || value.TargetsOmitted != 0 || len(value.Witnesses) == 0 {
			return fmt.Errorf("program index: invalid exact relation")
		}
	case ResolutionAlternatives:
		if len(value.ToIDs) < 1 || len(value.Witnesses) == 0 {
			return fmt.Errorf("program index: invalid alternatives relation")
		}
	case ResolutionUnresolved:
		if len(value.ToIDs) != 0 {
			return fmt.Errorf("program index: invalid unresolved relation")
		}
	}
	return nil
}

func validateWitness(value Witness) error {
	if !validText(value.Kind) || !validOptionalText(value.Detail) ||
		!validPatternString(value.SourceExpression) || !validOptionalLocation(value.Location) ||
		value.ObjectRef != "" || value.ObjectID != "" && !validCompactID(value.ObjectID, "n") ||
		value.Kind == WitnessKeys && value.ObjectID == "" {
		return fmt.Errorf("program index: invalid witness")
	}
	return nil
}

func canonicalWitnesses(values []Witness) ([]Witness, error) {
	result := cloneWitnesses(values)
	for _, witness := range result {
		if err := validateWitness(witness); err != nil {
			return nil, err
		}
	}
	sort.Slice(result, func(i, j int) bool { return witnessKey(result[i]) < witnessKey(result[j]) })
	if len(result) < 2 {
		return result, nil
	}
	compacted := result[:1]
	for _, witness := range result[1:] {
		if witnessKey(compacted[len(compacted)-1]) != witnessKey(witness) {
			compacted = append(compacted, witness)
		}
	}
	return compacted, nil
}

func canonicalizeRelationPatterns(
	values []RelationPatternInput,
	relationID string,
	bindings []objectBinding,
	pendingValueSources map[string]pendingPatternValueSourceArguments,
	pendingSameValues map[string]PatternRefInput,
) ([]RelationPattern, error) {
	values = slices.Clone(values)
	sort.Slice(values, func(i, j int) bool { return values[i].SourceRef < values[j].SourceRef })
	result := make([]RelationPattern, 0, len(values))
	for position, value := range values {
		if !validText(value.SourceRef) || !value.Form.Valid() || !validText(value.Selector) ||
			!validOptionalLocation(value.Location) ||
			!validOptionalText(value.ResultRef) || !validOptionalText(value.ReceiverRef) {
			return nil, fmt.Errorf("invalid pattern input")
		}
		resultID, err := resolveObjectRef(bindings, value.ResultRef)
		if err != nil {
			return nil, fmt.Errorf("pattern %q result: %w", value.SourceRef, err)
		}
		receiverID, err := resolveObjectRef(bindings, value.ReceiverRef)
		if err != nil {
			return nil, fmt.Errorf("pattern %q receiver: %w", value.SourceRef, err)
		}
		receiverOriginIDs, receiverOriginsOmitted, err := resolvePatternObjectRefs(
			bindings, value.ReceiverOriginRefs, value.ReceiverOriginResolution, value.ReceiverOriginsObserved,
		)
		if err != nil {
			return nil, fmt.Errorf("pattern %q receiver origins: %w", value.SourceRef, err)
		}
		if position > 0 && values[position-1].SourceRef == value.SourceRef {
			return nil, fmt.Errorf("duplicate pattern source ref %q", value.SourceRef)
		}
		id := relationID + "p" + strconv.Itoa(position+1)
		arguments, err := canonicalizePatternArguments(value.Arguments, id, bindings, pendingValueSources)
		if err != nil {
			return nil, fmt.Errorf("pattern %q arguments: %w", value.SourceRef, err)
		}
		control, err := canonicalWitnesses(value.Context)
		if err != nil {
			return nil, fmt.Errorf("pattern %q context: %w", value.SourceRef, err)
		}
		if len(control) == 0 {
			control = nil
		}
		if branch := value.Branch; branch != nil && (branch.Line < 1 || branch.EndLine < branch.Line) {
			return nil, fmt.Errorf("pattern %q branch: lines %d-%d", value.SourceRef, branch.Line, branch.EndLine)
		}
		if same := value.SameValueAs; same != nil {
			if !validText(same.RelationSourceRef) || !validText(same.PatternSourceRef) {
				return nil, fmt.Errorf("pattern %q names an invalid pattern it reads the same value as", value.SourceRef)
			}
			pendingSameValues[id] = *same
		}
		pattern := RelationPattern{
			ID: id, SourceRef: value.SourceRef, Form: value.Form, Selector: value.Selector, Context: control, Branch: cloneLineRange(value.Branch),
			Location: cloneLocation(value.Location),
			ResultID: resultID, ReceiverID: receiverID, ReceiverOriginIDs: receiverOriginIDs,
			ReceiverValue: sourcevalue.Clone(value.ReceiverValue), ResultValue: sourcevalue.Clone(value.ResultValue),
			ReceiverOriginResolution: value.ReceiverOriginResolution,
			ReceiverOriginsObserved:  value.ReceiverOriginsObserved, ReceiverOriginsOmitted: receiverOriginsOmitted,
			Arguments: arguments, ArgumentsObserved: value.ArgumentsObserved,
			ArgumentsOmitted: value.ArgumentsObserved - len(arguments),
		}
		result = append(result, pattern)
	}
	return result, nil
}

func canonicalizePatternArguments(
	values []PatternArgumentInput,
	patternID string,
	bindings []objectBinding,
	pendingValueSources map[string]pendingPatternValueSourceArguments,
) ([]PatternArgument, error) {
	values = slices.Clone(values)
	sort.Slice(values, func(i, j int) bool {
		left, right := values[i], values[j]
		if left.Position > 0 && right.Position == 0 {
			return true
		}
		if left.Position == 0 && right.Position > 0 {
			return false
		}
		if left.Position > 0 {
			return left.Position < right.Position
		}
		return left.Keyword < right.Keyword
	})
	result := make([]PatternArgument, 0, len(values))
	for position, value := range values {
		if !validPatternArgumentKey(value.Position, value.Keyword) || !value.Kind.Valid() {
			return nil, fmt.Errorf("invalid argument input")
		}
		if err := sourcevalue.Validate(value.Origin); err != nil {
			return nil, err
		}
		switch value.Kind {
		case PatternLiteralString:
			if len(value.Parts) != 0 || !validPatternString(value.Value) {
				return nil, fmt.Errorf("invalid literal argument input")
			}
		case PatternStringTemplate:
			if value.Value != "" {
				return nil, fmt.Errorf("invalid template argument input")
			}
		case PatternDynamic:
			if value.Value != "" || len(value.Parts) != 0 {
				return nil, fmt.Errorf("invalid dynamic argument input")
			}
		}
		parts, err := canonicalizePatternParts(value.Parts)
		if err != nil {
			return nil, err
		}
		objectIDs, objectsOmitted, err := resolvePatternObjectRefs(bindings, value.ObjectRefs, value.Resolution, value.ObjectsObserved)
		if err != nil {
			return nil, fmt.Errorf("argument %q objects: %w", patternArgumentKey(value.Position, value.Keyword), err)
		}
		if position > 0 && patternArgumentKey(values[position-1].Position, values[position-1].Keyword) == patternArgumentKey(value.Position, value.Keyword) {
			return nil, fmt.Errorf("duplicate argument key %q", patternArgumentKey(value.Position, value.Keyword))
		}
		argumentID := patternID + "a" + strconv.Itoa(position+1)
		valueCandidates, valueCandidatesOmitted, err := canonicalizePatternValueCandidates(
			value.ValueCandidates, value.ValueCandidatesObserved, argumentID, bindings, pendingValueSources,
		)
		if err != nil {
			return nil, fmt.Errorf("argument %q value candidates: %w", patternArgumentKey(value.Position, value.Keyword), err)
		}
		argument := PatternArgument{
			Origin:   sourcevalue.Clone(value.Origin),
			ID:       argumentID,
			Position: value.Position, Keyword: value.Keyword, Kind: value.Kind, Value: value.Value, Parts: parts,
			ObjectIDs: objectIDs, Resolution: value.Resolution,
			ObjectsObserved: value.ObjectsObserved, ObjectsOmitted: objectsOmitted,
			ValueCandidates: valueCandidates, ValueCandidatesObserved: value.ValueCandidatesObserved,
			ValueCandidatesOmitted: valueCandidatesOmitted,
		}
		result = append(result, argument)
	}
	return result, nil
}

func canonicalizePatternValueCandidates(
	values []PatternValueCandidateInput,
	observed int,
	argumentID string,
	bindings []objectBinding,
	pendingValueSources map[string]pendingPatternValueSourceArguments,
) ([]PatternValueCandidate, int, error) {
	if observed != len(values) {
		return nil, 0, fmt.Errorf("incomplete candidate coverage")
	}
	result := make([]PatternValueCandidate, 0, len(values))
	for _, value := range values {
		if !value.Resolution.Valid() || !value.SourceKind.Valid() ||
			value.Kind != PatternLiteralString && value.Kind != PatternStringTemplate {
			return nil, 0, fmt.Errorf("invalid candidate input")
		}
		switch value.Kind {
		case PatternLiteralString:
			if len(value.Parts) != 0 || !validPatternString(value.Value) {
				return nil, 0, fmt.Errorf("invalid literal candidate input")
			}
		case PatternStringTemplate:
			if value.Value != "" {
				return nil, 0, fmt.Errorf("invalid template candidate input")
			}
		}
		parts, err := canonicalizePatternParts(value.Parts)
		if err != nil {
			return nil, 0, err
		}
		sourceIDs, sourceOmitted, err := resolvePatternValueSourceRefs(
			bindings, value.SourceObjectRefs, value.SourceObjectsObserved,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("source objects: %w", err)
		}
		sourceArgumentRefs, err := canonicalizePatternArgumentReferences(
			value.SourceArgumentRefs, value.SourceArgumentsObserved,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("source arguments: %w", err)
		}
		candidate := PatternValueCandidate{
			Kind: value.Kind, Value: value.Value, Parts: parts, Resolution: value.Resolution,
			SourceKind: value.SourceKind, SourceObjectIDs: sourceIDs,
			SourceObjectsObserved: value.SourceObjectsObserved, SourceObjectsOmitted: sourceOmitted,
			SourceArgumentIDs: []string{}, SourceArgumentsObserved: value.SourceArgumentsObserved,
		}
		switch value.SourceKind {
		case PatternValueSourceInitializer:
			if len(sourceIDs) == 0 || len(sourceArgumentRefs) != 0 {
				return nil, 0, fmt.Errorf("initializer candidate has invalid sources")
			}
			candidate.ID = patternValueCandidateIdentity(argumentID, candidate)
		case PatternValueSourceActualArgument:
			if len(sourceIDs) != 0 || len(sourceArgumentRefs) == 0 || value.Resolution != PatternValuePossible {
				return nil, 0, fmt.Errorf("actual-argument candidate has invalid sources or authority")
			}
			candidate.ID = pendingPatternValueCandidateIdentity(argumentID, candidate, sourceArgumentRefs)
			if _, duplicate := pendingValueSources[candidate.ID]; duplicate {
				return nil, 0, fmt.Errorf("duplicate pending candidate identity %q", candidate.ID)
			}
			pendingValueSources[candidate.ID] = pendingPatternValueSourceArguments{
				Refs: sourceArgumentRefs, Observed: value.SourceArgumentsObserved,
			}
		}
		result = append(result, candidate)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	for position := 1; position < len(result); position++ {
		if result[position-1].ID == result[position].ID {
			return nil, 0, fmt.Errorf("duplicate candidate identity %q", result[position].ID)
		}
	}
	for position := range result {
		oldID := result[position].ID
		newID := argumentID + "v" + strconv.Itoa(position+1)
		if pending, ok := pendingValueSources[oldID]; ok {
			delete(pendingValueSources, oldID)
			pendingValueSources[newID] = pending
		}
		result[position].ID = newID
	}
	return result, observed - len(result), nil
}

func resolvePatternValueSourceRefs(
	bindings []objectBinding,
	refs []string,
	observed int,
) ([]string, int, error) {
	canonicalRefs := cloneStrings(refs)
	for _, ref := range canonicalRefs {
		if !validText(ref) {
			return nil, 0, fmt.Errorf("invalid source object ref")
		}
	}
	sort.Strings(canonicalRefs)
	for position := 1; position < len(canonicalRefs); position++ {
		if canonicalRefs[position-1] == canonicalRefs[position] {
			return nil, 0, fmt.Errorf("duplicate source object ref %q", canonicalRefs[position])
		}
	}
	ids := make([]string, 0, len(canonicalRefs))
	for _, ref := range canonicalRefs {
		id, err := resolveObjectRef(bindings, ref)
		if err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if observed != len(ids) {
		return nil, 0, fmt.Errorf("incomplete source object coverage")
	}
	return ids, 0, nil
}

func canonicalizePatternArgumentReferences(
	refs []PatternArgumentRefInput,
	observed int,
) ([]PatternArgumentRefInput, error) {
	result := make([]PatternArgumentRefInput, len(refs))
	copy(result, refs)
	for _, ref := range result {
		if !validPatternArgumentRefInput(ref) {
			return nil, fmt.Errorf("invalid source argument ref")
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return patternArgumentReferenceKey(result[i]) < patternArgumentReferenceKey(result[j])
	})
	for position := 1; position < len(result); position++ {
		if patternArgumentReferenceKey(result[position-1]) == patternArgumentReferenceKey(result[position]) {
			return nil, fmt.Errorf("duplicate source argument ref")
		}
	}
	if observed != len(result) {
		return nil, fmt.Errorf("incomplete source argument coverage")
	}
	return result, nil
}

func canonicalizePatternParts(values []PatternPartInput) ([]PatternPart, error) {
	result := make([]PatternPart, 0, len(values))
	for _, value := range values {
		if !value.Kind.Valid() || value.Kind == PatternPartHole && value.Text != "" ||
			value.Kind == PatternPartLiteral && !validPatternString(value.Text) {
			return nil, fmt.Errorf("invalid template part")
		}
		if value.Kind == PatternPartLiteral && value.Text == "" {
			continue
		}
		if value.Kind == PatternPartLiteral && len(result) > 0 && result[len(result)-1].Kind == PatternPartLiteral {
			combined := result[len(result)-1].Text + value.Text
			if !validPatternString(combined) {
				return nil, fmt.Errorf("template literal bound exceeded")
			}
			result[len(result)-1].Text = combined
			continue
		}
		result = append(result, PatternPart{Kind: value.Kind, Text: value.Text})
	}
	return result, nil
}

func resolvePatternObjectRefs(bindings []objectBinding, refs []string, resolution Resolution, observed int) ([]string, int, error) {
	canonicalRefs := cloneStrings(refs)
	for _, ref := range canonicalRefs {
		if !validText(ref) {
			return nil, 0, fmt.Errorf("invalid object ref")
		}
	}
	sort.Strings(canonicalRefs)
	canonicalRefs = compactStrings(canonicalRefs)
	ids := make([]string, 0, len(canonicalRefs))
	for _, ref := range canonicalRefs {
		id, err := resolveObjectRef(bindings, ref)
		if err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	ids = compactStrings(ids)
	omitted := observed - len(ids)
	if err := validateOptionalObjectAuthority(ids, resolution, observed, omitted); err != nil {
		return nil, 0, err
	}
	return ids, omitted, nil
}

func validateRelationPatternShape(value RelationPattern, relationID string) error {
	if err := sourcevalue.Validate(value.ReceiverValue); err != nil {
		return err
	}
	if err := sourcevalue.Validate(value.ResultValue); err != nil {
		return err
	}
	for i, witness := range value.Context {
		if err := validateWitness(witness); err != nil {
			return err
		}
		if i > 0 && witnessKey(value.Context[i-1]) >= witnessKey(witness) {
			return fmt.Errorf("program index: noncanonical pattern context")
		}
	}
	if !strings.HasPrefix(value.ID, relationID+"p") || !value.Form.Valid() || !validText(value.Selector) ||
		!validOptionalLocation(value.Location) ||
		!validOptionalText(value.ResultID) || !validOptionalText(value.ReceiverID) ||
		value.ReceiverOriginIDs == nil || value.Arguments == nil ||
		!canonicalStringsAllowEmpty(value.ReceiverOriginIDs) {
		return fmt.Errorf("program index: invalid relation pattern")
	}
	if err := validateOptionalObjectAuthority(value.ReceiverOriginIDs, value.ReceiverOriginResolution,
		value.ReceiverOriginsObserved, value.ReceiverOriginsOmitted); err != nil {
		return fmt.Errorf("program index: invalid pattern receiver origin coverage")
	}
	if value.ArgumentsObserved < len(value.Arguments) ||
		value.ArgumentsOmitted != value.ArgumentsObserved-len(value.Arguments) {
		return fmt.Errorf("program index: invalid pattern argument coverage")
	}
	for position, argument := range value.Arguments {
		if err := validatePatternArgumentShape(argument, value.ID); err != nil {
			return err
		}
		if argument.ID != value.ID+"a"+strconv.Itoa(position+1) ||
			position > 0 && comparePatternArguments(value.Arguments[position-1], argument) >= 0 {
			return fmt.Errorf("program index: pattern arguments are not canonical at %s: previous=%d/%q current=%s:%d/%q",
				value.ID, value.Arguments[max(0, position-1)].Position, value.Arguments[max(0, position-1)].Keyword,
				argument.ID, argument.Position, argument.Keyword)
		}
	}
	return nil
}

func validatePatternArgumentShape(value PatternArgument, patternID string) error {
	if err := sourcevalue.Validate(value.Origin); err != nil {
		return err
	}
	if !strings.HasPrefix(value.ID, patternID+"a") || !validPatternArgumentKey(value.Position, value.Keyword) || !value.Kind.Valid() ||
		value.Parts == nil || value.ObjectIDs == nil || value.ValueCandidates == nil ||
		!canonicalStringsAllowEmpty(value.ObjectIDs) {
		return fmt.Errorf("program index: invalid pattern argument")
	}
	if err := validateOptionalObjectAuthority(value.ObjectIDs, value.Resolution, value.ObjectsObserved, value.ObjectsOmitted); err != nil {
		return fmt.Errorf("program index: invalid pattern argument object coverage")
	}
	if value.ValueCandidatesObserved != len(value.ValueCandidates) || value.ValueCandidatesOmitted != 0 {
		return fmt.Errorf("program index: invalid pattern value candidate coverage")
	}
	if value.Kind != PatternDynamic && len(value.ValueCandidates) != 0 {
		return fmt.Errorf("program index: resolved values require a dynamic pattern argument")
	}
	for position, candidate := range value.ValueCandidates {
		if err := validatePatternValueCandidateShape(candidate, value.ID); err != nil {
			return err
		}
		if candidate.ID != value.ID+"v"+strconv.Itoa(position+1) {
			return fmt.Errorf("program index: pattern value candidates are not canonical")
		}
	}
	switch value.Kind {
	case PatternLiteralString:
		if !validPatternString(value.Value) || len(value.Parts) != 0 {
			return fmt.Errorf("program index: invalid literal pattern argument")
		}
	case PatternStringTemplate:
		if value.Value != "" || len(value.Parts) == 0 {
			return fmt.Errorf("program index: invalid template pattern argument")
		}
		hasHole := false
		previousLiteral := false
		for _, part := range value.Parts {
			if !part.Kind.Valid() || part.Kind == PatternPartHole && part.Text != "" ||
				part.Kind == PatternPartLiteral && (part.Text == "" || !validPatternString(part.Text)) ||
				part.Kind == PatternPartLiteral && previousLiteral {
				return fmt.Errorf("program index: invalid template pattern part")
			}
			hasHole = hasHole || part.Kind == PatternPartHole
			previousLiteral = part.Kind == PatternPartLiteral
		}
		if !hasHole {
			return fmt.Errorf("program index: template pattern has no hole")
		}
	case PatternDynamic:
		if value.Value != "" || len(value.Parts) != 0 {
			return fmt.Errorf("program index: invalid dynamic pattern argument")
		}
	}
	return nil
}

func validatePatternValueCandidateShape(value PatternValueCandidate, argumentID string) error {
	if !strings.HasPrefix(value.ID, argumentID+"v") || !value.Resolution.Valid() || !value.SourceKind.Valid() ||
		value.Parts == nil || value.SourceObjectIDs == nil || value.SourceArgumentIDs == nil ||
		!canonicalStringsAllowEmpty(value.SourceObjectIDs) || !canonicalStringsAllowEmpty(value.SourceArgumentIDs) ||
		value.SourceObjectsObserved != len(value.SourceObjectIDs) || value.SourceObjectsOmitted != 0 ||
		value.SourceArgumentsObserved != len(value.SourceArgumentIDs) || value.SourceArgumentsOmitted != 0 {
		return fmt.Errorf("program index: invalid pattern value candidate")
	}
	switch value.SourceKind {
	case PatternValueSourceInitializer:
		if len(value.SourceObjectIDs) == 0 || len(value.SourceArgumentIDs) != 0 || value.SourceArgumentsObserved != 0 {
			return fmt.Errorf("program index: invalid initializer value candidate sources")
		}
	case PatternValueSourceActualArgument:
		if len(value.SourceObjectIDs) != 0 || value.SourceObjectsObserved != 0 ||
			len(value.SourceArgumentIDs) != 1 || value.SourceArgumentsObserved != 1 ||
			value.Resolution != PatternValuePossible {
			return fmt.Errorf("program index: invalid actual-argument value candidate sources")
		}
	}
	switch value.Kind {
	case PatternLiteralString:
		if !validPatternString(value.Value) || len(value.Parts) != 0 {
			return fmt.Errorf("program index: invalid literal pattern value candidate")
		}
	case PatternStringTemplate:
		if value.Value != "" || len(value.Parts) == 0 {
			return fmt.Errorf("program index: invalid template pattern value candidate")
		}
		hasHole := false
		previousLiteral := false
		for _, part := range value.Parts {
			if !part.Kind.Valid() || part.Kind == PatternPartHole && part.Text != "" ||
				part.Kind == PatternPartLiteral && (part.Text == "" || !validPatternString(part.Text)) ||
				part.Kind == PatternPartLiteral && previousLiteral {
				return fmt.Errorf("program index: invalid template pattern value candidate part")
			}
			hasHole = hasHole || part.Kind == PatternPartHole
			previousLiteral = part.Kind == PatternPartLiteral
		}
		if !hasHole {
			return fmt.Errorf("program index: template pattern value candidate has no hole")
		}
	default:
		return fmt.Errorf("program index: invalid pattern value candidate kind")
	}
	return nil
}

func patternValueCandidateIdentity(argumentID string, value PatternValueCandidate) string {
	fields := []string{
		argumentID, string(value.Kind), value.Value, string(value.Resolution), string(value.SourceKind),
	}
	for _, part := range value.Parts {
		fields = append(fields, string(part.Kind), part.Text)
	}
	fields = append(fields, "source-objects")
	fields = append(fields, value.SourceObjectIDs...)
	fields = append(fields, "source-arguments")
	fields = append(fields, value.SourceArgumentIDs...)
	return stableID("program-pattern-value", fields...)
}

func pendingPatternValueCandidateIdentity(
	argumentID string,
	value PatternValueCandidate,
	refs []PatternArgumentRefInput,
) string {
	fields := []string{
		argumentID, string(value.Kind), value.Value, string(value.Resolution), string(value.SourceKind),
	}
	for _, part := range value.Parts {
		fields = append(fields, string(part.Kind), part.Text)
	}
	for _, ref := range refs {
		fields = append(fields, patternArgumentReferenceKey(ref))
	}
	return stableID("program-pattern-value-pending", fields...)
}

func validateOptionalObjectAuthority(ids []string, resolution Resolution, observed, omitted int) error {
	if observed < 0 || observed < len(ids) || omitted != observed-len(ids) {
		return fmt.Errorf("invalid object coverage")
	}
	if resolution == "" {
		if observed != 0 || len(ids) != 0 || omitted != 0 {
			return fmt.Errorf("object authority is missing resolution")
		}
		return nil
	}
	if !resolution.Valid() || observed == 0 {
		return fmt.Errorf("invalid object resolution")
	}
	switch resolution {
	case ResolutionExact:
		if len(ids) != 1 || omitted != 0 {
			return fmt.Errorf("invalid exact object authority")
		}
	case ResolutionAlternatives:
		if len(ids) == 0 {
			return fmt.Errorf("invalid alternative object authority")
		}
	case ResolutionUnresolved:
		if len(ids) != 0 {
			return fmt.Errorf("invalid unresolved object authority")
		}
	}
	return nil
}

func validPatternArgumentRefInput(value PatternArgumentRefInput) bool {
	return validText(value.RelationSourceRef) && validText(value.PatternSourceRef) &&
		validPatternArgumentKey(value.Position, value.Keyword)
}

func patternArgumentReferenceKey(value PatternArgumentRefInput) string {
	return strings.Join([]string{
		value.RelationSourceRef, value.PatternSourceRef,
		patternArgumentKey(value.Position, value.Keyword),
	}, "\x00")
}

func resolvePatternArgumentReference(
	relations []Relation,
	reference PatternArgumentRefInput,
) (string, error) {
	if !validPatternArgumentRefInput(reference) {
		return "", fmt.Errorf("invalid reference")
	}
	argumentID := ""
	for _, relation := range relations {
		if relation.SourceRef != reference.RelationSourceRef {
			continue
		}
		for _, pattern := range relation.Patterns {
			if pattern.SourceRef != reference.PatternSourceRef {
				continue
			}
			for _, argument := range pattern.Arguments {
				if argument.Position != reference.Position || argument.Keyword != reference.Keyword {
					continue
				}
				if argumentID != "" && argumentID != argument.ID {
					return "", fmt.Errorf("ambiguous reference")
				}
				argumentID = argument.ID
			}
		}
	}
	if argumentID == "" {
		return "", fmt.Errorf("unknown reference")
	}
	return argumentID, nil
}

// resolveSameValuePatterns gives each pattern that reads the same value as
// another the other pattern's sealed ID. A reference naming no pattern, or
// several, refuses the index: the adapter wrote a pattern it did not keep.
func resolveSameValuePatterns(relations []Relation, pending map[string]PatternRefInput) error {
	if len(pending) == 0 {
		return nil
	}
	type at struct{ relation, pattern int }
	bySourceRef := make(map[PatternRefInput][]string)
	patterns := make(map[string]at)
	for relationPosition, relation := range relations {
		for patternPosition, pattern := range relation.Patterns {
			key := PatternRefInput{RelationSourceRef: relation.SourceRef, PatternSourceRef: pattern.SourceRef}
			bySourceRef[key] = append(bySourceRef[key], pattern.ID)
			patterns[pattern.ID] = at{relationPosition, patternPosition}
		}
	}
	for id, reference := range pending {
		named := bySourceRef[reference]
		if len(named) != 1 {
			return fmt.Errorf("program index: pattern %q reads the same value as %d patterns of %q/%q", id, len(named), reference.RelationSourceRef, reference.PatternSourceRef)
		}
		position := patterns[id]
		relations[position.relation].Patterns[position.pattern].SameValueAs = named[0]
	}
	return nil
}

// validateSameValuePatterns holds each pattern that reads the same value as
// another to the fact's shape: the other is a pattern of a relation of the
// same kind from the same declaration to the same targets, written before
// it in the same file, and names none itself.
func validateSameValuePatterns(relations []Relation) error {
	type named struct {
		relation Relation
		pattern  RelationPattern
	}
	var patterns map[string]named
	for _, relation := range relations {
		for _, pattern := range relation.Patterns {
			if pattern.SameValueAs == "" {
				continue
			}
			if patterns == nil {
				patterns = make(map[string]named)
				for _, relation := range relations {
					for _, pattern := range relation.Patterns {
						patterns[pattern.ID] = named{relation, pattern}
					}
				}
			}
			first, ok := patterns[pattern.SameValueAs]
			if !ok || first.pattern.ID == pattern.ID || first.pattern.SameValueAs != "" ||
				first.relation.FromID != relation.FromID || first.relation.Kind != relation.Kind ||
				!slices.Equal(first.relation.ToIDs, relation.ToIDs) ||
				first.pattern.Location == nil || pattern.Location == nil || first.pattern.Location.Path != pattern.Location.Path ||
				!locationLess(*first.pattern.Location, *pattern.Location) {
				return fmt.Errorf("program index: pattern %q reads the same value as an invalid pattern %q", pattern.ID, pattern.SameValueAs)
			}
		}
	}
	return nil
}

func locationLess(a, b Location) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Column < b.Column
}

func resolvePatternValueSourceArgumentReferences(
	relations []Relation,
	pending map[string]pendingPatternValueSourceArguments,
) error {
	resolvedPending := make(map[string]struct{}, len(pending))
	for relationPosition := range relations {
		for patternPosition := range relations[relationPosition].Patterns {
			pattern := &relations[relationPosition].Patterns[patternPosition]
			for argumentPosition := range pattern.Arguments {
				argument := &pattern.Arguments[argumentPosition]
				for candidatePosition := range argument.ValueCandidates {
					candidate := &argument.ValueCandidates[candidatePosition]
					value, ok := pending[candidate.ID]
					if !ok {
						continue
					}
					ids := make([]string, 0, len(value.Refs))
					for _, ref := range value.Refs {
						id, err := resolvePatternArgumentReference(relations, ref)
						if err != nil {
							return fmt.Errorf("program index: pattern value source argument: %w", err)
						}
						if id == argument.ID {
							return fmt.Errorf("program index: pattern value candidate cites its owning argument")
						}
						sourceRelation, sourceArgument, ok := patternArgumentAuthorityWithID(relations, id)
						if !ok || sourceRelation.Resolution != ResolutionExact || len(sourceRelation.ToIDs) != 1 ||
							sourceRelation.TargetsOmitted != 0 || !samePatternValue(candidate.Kind, candidate.Value, candidate.Parts, sourceArgument) {
							return fmt.Errorf("program index: actual value source argument has incompatible authority")
						}
						ids = append(ids, id)
					}
					sort.Strings(ids)
					for position := 1; position < len(ids); position++ {
						if ids[position-1] == ids[position] {
							return fmt.Errorf("program index: duplicate resolved value source argument")
						}
					}
					if value.Observed != len(ids) {
						return fmt.Errorf("program index: incomplete resolved value source arguments")
					}
					pendingID := candidate.ID
					candidate.SourceArgumentIDs = ids
					candidate.SourceArgumentsObserved = value.Observed
					candidate.SourceArgumentsOmitted = 0
					resolvedPending[pendingID] = struct{}{}
				}
			}
		}
	}
	if len(resolvedPending) != len(pending) {
		return fmt.Errorf("program index: unresolved pending pattern value candidate")
	}
	return nil
}

func patternArgumentAuthorityWithID(relations []Relation, id string) (Relation, PatternArgument, bool) {
	for _, relation := range relations {
		for _, pattern := range relation.Patterns {
			for _, argument := range pattern.Arguments {
				if argument.ID == id {
					return relation, argument, true
				}
			}
		}
	}
	return Relation{}, PatternArgument{}, false
}

func samePatternValue(kind PatternValueKind, value string, parts []PatternPart, source PatternArgument) bool {
	if source.Kind != kind || source.Value != value || len(source.Parts) != len(parts) {
		return false
	}
	for position := range parts {
		if source.Parts[position] != parts[position] {
			return false
		}
	}
	return kind == PatternLiteralString || kind == PatternStringTemplate
}

func validPatternArgumentKey(position int, keyword string) bool {
	return (position > 0 && keyword == "") ||
		(position == 0 && validText(keyword))
}

func patternArgumentKey(position int, keyword string) string {
	if position > 0 {
		return "position:" + strconv.Itoa(position)
	}
	return "keyword:" + keyword
}

func comparePatternArguments(left, right PatternArgument) int {
	if left.Position > 0 && right.Position == 0 {
		return -1
	}
	if left.Position == 0 && right.Position > 0 {
		return 1
	}
	if left.Position > 0 {
		if left.Position < right.Position {
			return -1
		}
		if left.Position > right.Position {
			return 1
		}
		return 0
	}
	return strings.Compare(left.Keyword, right.Keyword)
}

func compileCoverage(objects []Object, relations []Relation, objectsObserved, relationsObserved int) Coverage {
	coverage := Coverage{
		ObjectsObserved: objectsObserved, ObjectsIndexed: len(objects), ObjectsOmitted: objectsObserved - len(objects),
		RelationsObserved: relationsObserved, RelationsIndexed: len(relations), RelationsOmitted: relationsObserved - len(relations),
	}
	for _, relation := range relations {
		switch relation.Resolution {
		case ResolutionExact:
			coverage.ExactRelations++
		case ResolutionAlternatives:
			coverage.AlternativeRelations++
		case ResolutionUnresolved:
			coverage.UnresolvedRelations++
		}
		coverage.TargetsObserved += relation.TargetsObserved
		coverage.TargetsIndexed += len(relation.ToIDs)
		coverage.TargetsOmitted += relation.TargetsOmitted
		coverage.WitnessesObserved += relation.WitnessesObserved
		coverage.WitnessesIndexed += len(relation.Witnesses)
		coverage.WitnessesOmitted += relation.WitnessesOmitted
		coverage.PatternsObserved += relation.PatternsObserved
		coverage.PatternsIndexed += len(relation.Patterns)
		coverage.PatternsOmitted += relation.PatternsOmitted
		for _, pattern := range relation.Patterns {
			coverage.ArgumentsObserved += pattern.ArgumentsObserved
			coverage.ArgumentsIndexed += len(pattern.Arguments)
			coverage.ArgumentsOmitted += pattern.ArgumentsOmitted
			coverage.ReceiverOriginsObserved += pattern.ReceiverOriginsObserved
			coverage.ReceiverOriginsIndexed += len(pattern.ReceiverOriginIDs)
			coverage.ReceiverOriginsOmitted += pattern.ReceiverOriginsOmitted
			for _, argument := range pattern.Arguments {
				coverage.ArgumentObjectsObserved += argument.ObjectsObserved
				coverage.ArgumentObjectsIndexed += len(argument.ObjectIDs)
				coverage.ArgumentObjectsOmitted += argument.ObjectsOmitted
				coverage.ArgumentValuesObserved += argument.ValueCandidatesObserved
				coverage.ArgumentValuesIndexed += len(argument.ValueCandidates)
				coverage.ArgumentValuesOmitted += argument.ValueCandidatesOmitted
				for _, candidate := range argument.ValueCandidates {
					coverage.ValueSourcesObserved += candidate.SourceObjectsObserved
					coverage.ValueSourcesIndexed += len(candidate.SourceObjectIDs)
					coverage.ValueSourcesOmitted += candidate.SourceObjectsOmitted
					coverage.ValueArgumentSourcesObserved += candidate.SourceArgumentsObserved
					coverage.ValueArgumentSourcesIndexed += len(candidate.SourceArgumentIDs)
					coverage.ValueArgumentSourcesOmitted += candidate.SourceArgumentsOmitted
				}
			}
		}
	}
	return coverage
}

func validateCoverage(value Coverage, objectsIndexed, relationsIndexed int) error {
	counts := []int{
		value.ObjectsObserved, value.ObjectsIndexed, value.ObjectsOmitted,
		value.RelationsObserved, value.RelationsIndexed, value.RelationsOmitted,
		value.ExactRelations, value.AlternativeRelations, value.UnresolvedRelations,
		value.TargetsObserved, value.TargetsIndexed, value.TargetsOmitted,
		value.WitnessesObserved, value.WitnessesIndexed, value.WitnessesOmitted,
		value.PatternsObserved, value.PatternsIndexed, value.PatternsOmitted,
		value.ArgumentsObserved, value.ArgumentsIndexed, value.ArgumentsOmitted,
		value.ReceiverOriginsObserved, value.ReceiverOriginsIndexed, value.ReceiverOriginsOmitted,
		value.ArgumentObjectsObserved, value.ArgumentObjectsIndexed, value.ArgumentObjectsOmitted,
		value.ArgumentValuesObserved, value.ArgumentValuesIndexed, value.ArgumentValuesOmitted,
		value.ValueSourcesObserved, value.ValueSourcesIndexed, value.ValueSourcesOmitted,
		value.ValueArgumentSourcesObserved, value.ValueArgumentSourcesIndexed, value.ValueArgumentSourcesOmitted,
	}
	for _, count := range counts {
		if count < 0 {
			return fmt.Errorf("program index: invalid coverage count")
		}
	}
	if value.ObjectsIndexed != objectsIndexed || value.ObjectsObserved < objectsIndexed ||
		value.ObjectsOmitted != value.ObjectsObserved-objectsIndexed ||
		value.RelationsIndexed != relationsIndexed || value.RelationsObserved < relationsIndexed ||
		value.RelationsOmitted != value.RelationsObserved-relationsIndexed ||
		value.ExactRelations+value.AlternativeRelations+value.UnresolvedRelations != relationsIndexed ||
		value.TargetsOmitted != value.TargetsObserved-value.TargetsIndexed ||
		value.WitnessesOmitted != value.WitnessesObserved-value.WitnessesIndexed ||
		value.PatternsOmitted != value.PatternsObserved-value.PatternsIndexed ||
		value.ArgumentsOmitted != value.ArgumentsObserved-value.ArgumentsIndexed ||
		value.ReceiverOriginsOmitted != value.ReceiverOriginsObserved-value.ReceiverOriginsIndexed ||
		value.ArgumentObjectsOmitted != value.ArgumentObjectsObserved-value.ArgumentObjectsIndexed ||
		value.ArgumentValuesOmitted != value.ArgumentValuesObserved-value.ArgumentValuesIndexed ||
		value.ValueSourcesOmitted != value.ValueSourcesObserved-value.ValueSourcesIndexed ||
		value.ValueArgumentSourcesOmitted != value.ValueArgumentSourcesObserved-value.ValueArgumentSourcesIndexed {
		return fmt.Errorf("program index: invalid coverage")
	}
	return nil
}

func indexDigest(index Index) (string, error) {
	// Hashing only reads nested collections; the value copy owns its SHA field.
	payload := index
	payload.SHA256 = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("program index: encode digest material: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func stableID(prefix string, fields ...string) string {
	digest := sha256.New()
	for _, field := range append([]string{prefix}, fields...) {
		digest.Write([]byte(strconv.Itoa(len(field))))
		digest.Write([]byte{0})
		digest.Write([]byte(field))
	}
	return prefix + "-" + hex.EncodeToString(digest.Sum(nil))
}

func witnessKey(value Witness) string {
	return strings.Join([]string{
		locationKey(value.Location), value.Kind, value.Detail, value.SourceExpression, value.ObjectID,
	}, "\x00")
}

func locationKey(value *Location) string {
	if value == nil {
		return ""
	}
	return value.Path + ":" + strconv.Itoa(value.Line) + ":" + strconv.Itoa(value.Column)
}

// validEndLine accepts no end, or an end at or after the declaration's line.
func validEndLine(location *Location, endLine int) bool {
	return endLine == 0 || location != nil && endLine >= location.Line
}

// callableKind is a declaration that runs: only a callable can be proven
// unreachable.
func callableKind(kind ObjectKind) bool {
	return kind.Callable()
}

// validCodeLines accepts no count, or a count of a located declaration that
// fits in its source range when the range is known.
func validCodeLines(location *Location, endLine, codeLines int) bool {
	switch {
	case codeLines == 0:
		return true
	case codeLines < 0 || location == nil:
		return false
	case endLine > 0:
		return codeLines <= endLine-location.Line+1
	default:
		return true
	}
}

// validTypeLocations holds a variable's type declarations: valid, each once.
func validTypeLocations(kind ObjectKind, values []Location) bool {
	if len(values) == 0 {
		return true
	}
	if kind != ObjectVariable {
		return false
	}
	for position, value := range values {
		if !validLocation(value) || slices.Contains(values[:position], value) {
			return false
		}
	}
	return true
}

func validOptionalLocation(value *Location) bool {
	return value == nil || validLocation(*value)
}

func validObjectDirectory(kind ObjectKind, directory string) bool {
	return directory == "" || ((kind == ObjectPackage || kind == ObjectModule) && (directory == "." || validPath(directory)))
}

func validLocation(value Location) bool {
	return validPath(value.Path) && value.Line > 0 && value.Column > 0
}

func validPath(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || strings.Contains(value, "\\") || strings.HasPrefix(value, "/") ||
		!fs.ValidPath(value) || value == "." || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validText(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validOptionalText(value string) bool {
	return value == "" || validText(value)
}

// ValidName reports whether an adapter may use value as a name the index
// accepts (an external symbol's package path or name): not empty, no
// surrounding space, valid UTF-8 and no control character.
func ValidName(value string) bool {
	return validText(value)
}

func validPatternString(value string) bool {
	return utf8.ValidString(value)
}

func validateExternalSymbolBinding(kind ObjectKind, value *ExternalSymbol) error {
	if value == nil {
		return nil
	}
	if kind != ObjectExternalSymbol || !value.AuthorityKind.Valid() || !validText(value.PackagePath) ||
		!validOptionalText(value.Receiver) || !validText(value.Name) ||
		(value.RepositoryPath != "" && (value.AuthorityKind != ExternalAuthorityPackage ||
			(value.RepositoryPath != "." && !validPath(value.RepositoryPath)))) {
		return fmt.Errorf("program index: invalid external symbol authority")
	}
	return nil
}

func cloneExternalSymbol(value *ExternalSymbol) *ExternalSymbol {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func canonicalStrings(values []string) bool {
	if values == nil || !sort.StringsAreSorted(values) {
		return false
	}
	for position, value := range values {
		if !validText(value) || position > 0 && values[position-1] == value {
			return false
		}
	}
	return true
}

func canonicalStringsAllowEmpty(values []string) bool {
	if values == nil {
		return false
	}
	if len(values) == 0 {
		return true
	}
	return canonicalStrings(values)
}

func compactStrings(values []string) []string {
	if len(values) < 2 {
		return values
	}
	result := values[:1]
	for _, value := range values[1:] {
		if result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func cloneStrings(values []string) []string {
	result := make([]string, len(values))
	copy(result, values)
	return result
}

func cloneTargetSources(values []TargetSource) []TargetSource {
	result := make([]TargetSource, len(values))
	copy(result, values)
	return result
}

func cloneTargetExports(values []TargetExport) []TargetExport {
	if values == nil {
		return nil
	}
	result := make([]TargetExport, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
	}
	return result
}

func cloneTargetSeeds(values []TargetSeed) []TargetSeed {
	result := make([]TargetSeed, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
	}
	return result
}

func cloneLocation(value *Location) *Location {
	if value == nil {
		return nil
	}
	result := *value
	return &result
}

func cloneWitnesses(values []Witness) []Witness {
	result := make([]Witness, len(values))
	copy(result, values)
	for position := range result {
		result[position].Location = cloneLocation(values[position].Location)
	}
	return result
}

func cloneLineRange(value *LineRange) *LineRange {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneRelationPatterns(values []RelationPattern) []RelationPattern {
	result := make([]RelationPattern, len(values))
	copy(result, values)
	for position := range result {
		result[position].ReceiverValue = sourcevalue.Clone(values[position].ReceiverValue)
		result[position].ResultValue = sourcevalue.Clone(values[position].ResultValue)
		result[position].Location = cloneLocation(values[position].Location)
		result[position].Branch = cloneLineRange(values[position].Branch)
		if len(values[position].Context) > 0 {
			result[position].Context = cloneWitnesses(values[position].Context)
		} else {
			result[position].Context = nil
		}
		result[position].ReceiverOriginIDs = cloneStrings(values[position].ReceiverOriginIDs)
		result[position].Arguments = clonePatternArguments(values[position].Arguments)
	}
	return result
}

func clonePatternArguments(values []PatternArgument) []PatternArgument {
	result := make([]PatternArgument, len(values))
	copy(result, values)
	for position := range result {
		result[position].Parts = clonePatternParts(values[position].Parts)
		result[position].ObjectIDs = cloneStrings(values[position].ObjectIDs)
		result[position].ValueCandidates = clonePatternValueCandidates(values[position].ValueCandidates)
		result[position].Origin = sourcevalue.Clone(values[position].Origin)
	}
	return result
}

func clonePatternValueCandidates(values []PatternValueCandidate) []PatternValueCandidate {
	result := make([]PatternValueCandidate, len(values))
	copy(result, values)
	for position := range result {
		result[position].Parts = clonePatternParts(values[position].Parts)
		result[position].SourceObjectIDs = cloneStrings(values[position].SourceObjectIDs)
		result[position].SourceArgumentIDs = cloneStrings(values[position].SourceArgumentIDs)
	}
	return result
}

func clonePatternParts(values []PatternPart) []PatternPart {
	result := make([]PatternPart, len(values))
	copy(result, values)
	return result
}
