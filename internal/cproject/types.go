// Package cproject reads C programs through clang, run as a subprocess.
//
// Discover finds the programs the repository's build describes: a root
// compile_commands.json, else the link and compile lines a dry run of the root
// Makefile prints, else every .c file on its own with clang's defaults. Parse
// runs `clang -fsyntax-only -w -H -Xclang -ast-dump=json` once per translation
// unit and decodes the repository's own declarations with their exact source
// locations. The unit model below is what the ProgramIndex projection reads;
// nothing here decides a runtime role.
//
// The view is the host platform's build as the build description gives it,
// with _FORTIFY_SOURCE turned off so libc calls keep their written names.
// Toolchain records that view.
package cproject

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

// Discover reads the build description of the repository at root and returns
// its C programs. It returns a nil Project when the corpus has no .c file. It
// never fails for a C-specific reason: a missing clang, a failed dry run or a
// unit that does not parse is recorded in Project.Observations, and the
// affected program fails later in Parse with that explanation.
func Discover(ctx context.Context, root string, repository *corpus.Corpus) (*Project, error) {
	return discover(ctx, root, repository)
}

// Parse decodes every translation unit of one program. Units already parsed by
// store are reused, so a unit linked into several programs is parsed once per
// run; a nil store shares nothing. At most four clang processes run at a time
// per store. Any unit that fails to parse fails the whole program: there is no
// partial result.
func Parse(ctx context.Context, root string, repository *corpus.Corpus, program Program, store *Store) (*Parsed, error) {
	return parse(ctx, root, repository, program, store)
}

// Project is the result of discovery.
type Project struct {
	Root         string           `json:"root"`
	CorpusSHA256 string           `json:"corpus_sha256"`
	Toolchain    Toolchain        `json:"toolchain"`
	Build        Build            `json:"build"`
	Units        []UnitSpec       `json:"units"`
	Included     []IncludedSource `json:"included,omitempty"`
	Programs     []Program        `json:"programs"`
	Observations []Observation    `json:"observations,omitempty"`
}

// BuildKind names where the compile flags and link lines came from.
type BuildKind string

const (
	BuildCompileCommands BuildKind = "compile_commands" // root compile_commands.json
	BuildMake            BuildKind = "make"             // make -n -B on the root Makefile's default goal
	BuildNone            BuildKind = "none"             // clang's defaults for every .c file
)

// Build records the build description discovery read. When reading one
// failed, Kind is BuildNone, Err says why and every unit uses clang's defaults.
type Build struct {
	Kind    BuildKind `json:"kind"`
	Path    string    `json:"path,omitempty"`    // repository-relative compile_commands.json or Makefile
	Command []string  `json:"command,omitempty"` // the dry run that was executed
	Err     string    `json:"error,omitempty"`
}

// UnitSpec is one translation unit: a .c file compiled on its own.
type UnitSpec struct {
	Path    string `json:"path"`     // corpus path of the .c file
	FileRef string `json:"file_ref"` // its corpus file id
	// Dir is the working directory clang runs in, relative to the root ("."
	// for the root). Source is the unit argument as the build wrote it,
	// relative to Dir.
	Dir    string `json:"dir"`
	Source string `json:"source"`
	// Args are the build's compile flags the allowlist kept, in order;
	// Dropped are the ones it removed (warnings, debug, optimisation,
	// dependency output, flags clang may not accept).
	Args    []string `json:"args,omitempty"`
	Dropped []string `json:"dropped,omitempty"`
	// Built is true when the build description compiles the unit, false when
	// it is parsed with clang's defaults.
	Built bool `json:"built,omitempty"`
	// Main is the exact non-static main with a body that discovery found
	// in a unit no link line links. It is nil for linked units, whose main
	// is located by Parse.
	Main *Position `json:"main,omitempty"`
}

// IncludedSource is a .c file that another corpus file #includes. It belongs
// to its includer and is never a unit of its own. When no parsed unit enters
// it (an #ifdef chooses another backend on this platform) it is outside this
// platform's build and appears in Parsed.Outside.
type IncludedSource struct {
	Path      string `json:"path"`
	Includers []Site `json:"includers"`
}

// ProgramKind is what a program target builds.
type ProgramKind string

const (
	ProgramExecutable ProgramKind = "executable"     // a link line, or a unit with main
	ProgramShared     ProgramKind = "shared_library" // a link line with -shared or -dynamiclib
	// ProgramLibrary is one directory's units that no link line links and
	// that define no main.
	ProgramLibrary ProgramKind = "library"
)

// Program is one C target. It carries everything Parse needs, so a typed
// target can hold it on its own.
type Program struct {
	Ref          string      `json:"ref"`
	Selector     string      `json:"selector"` // c:redis-server, c:tools/dump.c, c:lib/, c:./
	Name         string      `json:"name"`     // link output, main unit or library directory, relative to the root
	Kind         ProgramKind `json:"kind"`
	CorpusSHA256 string      `json:"corpus_sha256"`
	// Units are the linked translation units, sorted by path. For a Closure
	// program it is the main unit alone and Pool holds every other unit the
	// linker could resolve a name to.
	Units   []UnitSpec `json:"units"`
	Closure bool       `json:"closure,omitempty"`
	Pool    []UnitSpec `json:"pool,omitempty"`
	// Anchor is the link line (Makefile:49), the main definition of a
	// closure program, or a library's first unit. AnchorFileRef is its file.
	Anchor        Site   `json:"anchor"`
	AnchorFileRef string `json:"anchor_file_ref"`
	// Evidence are the native observations behind the program: c_link,
	// c_main or c_library.
	Evidence []Observation `json:"evidence,omitempty"`
	// LinkArgs are the link line's libraries and link flags (-lm -pthread),
	// recorded and not analysed. Missing are link inputs no compile line of
	// the build produced (prebuilt objects, archives).
	LinkArgs []string `json:"link_args,omitempty"`
	Missing  []string `json:"missing,omitempty"`
	// Included are the project's included .c files, so Parse can report the
	// ones this program's units never enter.
	Included []IncludedSource `json:"included,omitempty"`
	// BuildErr is the build description failure that left these units with
	// clang's defaults; a unit that then fails reports both errors.
	BuildErr string `json:"build_error,omitempty"`
}

// programRef is the program's identity: a digest of everything Parse reads.
func programRef(program Program) string {
	program.Ref = ""
	raw, _ := json.Marshal(program)
	return fmt.Sprintf("c-%x", sha256.Sum256(raw))
}

// Validate checks that the program is complete and that Ref still names it.
func (program Program) Validate() error {
	if !strings.HasPrefix(program.Selector, "c:") || len(program.Selector) == 2 || program.Name == "" || program.AnchorFileRef == "" || program.Anchor.Path == "" || len(program.Units) == 0 {
		return fmt.Errorf("C: invalid program %q", program.Selector)
	}
	switch program.Kind {
	case ProgramExecutable, ProgramShared, ProgramLibrary:
	default:
		return fmt.Errorf("C: program %s has unknown kind %q", program.Selector, program.Kind)
	}
	if program.Closure && len(program.Units) != 1 {
		return fmt.Errorf("C: closure program %s must name its main unit alone", program.Selector)
	}
	for _, unit := range append(slices.Clone(program.Units), program.Pool...) {
		if unit.Path == "" || unit.FileRef == "" || unit.Source == "" || unit.Dir == "" {
			return fmt.Errorf("C: program %s has an incomplete unit %q", program.Selector, unit.Path)
		}
	}
	if program.Ref == "" || program.Ref != programRef(program) {
		return fmt.Errorf("C: program %s identity mismatch", program.Selector)
	}
	return nil
}

// ValidateAgainst also checks that every file the program names is the same
// file in repository.
func (program Program) ValidateAgainst(repository *corpus.Corpus) error {
	if err := program.Validate(); err != nil {
		return err
	}
	if repository == nil || repository.SHA256() != program.CorpusSHA256 {
		return fmt.Errorf("C: corpus binding mismatch for %s", program.Selector)
	}
	if ref, ok := repository.ID(program.Anchor.Path); !ok || string(ref) != program.AnchorFileRef {
		return fmt.Errorf("C: anchor binding mismatch for %s", program.Selector)
	}
	for _, unit := range append(slices.Clone(program.Units), program.Pool...) {
		if ref, ok := repository.ID(unit.Path); !ok || string(ref) != unit.FileRef {
			return fmt.Errorf("C: source binding mismatch: %s", unit.Path)
		}
	}
	return nil
}

// Site is a repository-relative file and line.
type Site struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
}

// Observation is native evidence with its source. Fields and Values follow
// the kind: c_link has Fields{output} and Values = linked units, c_main has the
// main definition's line, c_library has Values = the directory's units.
type Observation struct {
	Kind   string            `json:"kind"`
	Path   string            `json:"path,omitempty"`
	Line   int               `json:"line,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
	Values []string          `json:"values,omitempty"`
}

// Toolchain is the platform view every unit is parsed in.
type Toolchain struct {
	Clang       string      `json:"clang"`
	Version     string      `json:"version,omitempty"`
	Target      string      `json:"target,omitempty"` // target triple (x86_64-apple-darwin24.6.0)
	Sysroot     string      `json:"sysroot,omitempty"`
	ResourceDir string      `json:"resource_dir,omitempty"`
	SearchDirs  []SearchDir `json:"search_dirs,omitempty"`
	// Overrides are the flags added after every unit's own flags.
	Overrides []string `json:"overrides"`
	// Err says why clang could not be run; programs then fail in Parse.
	Err string `json:"error,omitempty"`
}

// SearchDir is one of clang's system include directories.
type SearchDir struct {
	Path      string    `json:"path"`
	Class     FileClass `json:"class"` // FilePlatform or FilePackage
	Framework bool      `json:"framework,omitempty"`
}

// FileClass says who owns a file clang read.
type FileClass string

const (
	FileCorpus     FileClass = "corpus"     // a corpus file
	FileRepository FileClass = "repository" // inside the root but outside the corpus
	// FilePlatform is under the clang resource directory, the sysroot's
	// usr/include or its frameworks, or the toolchain's own include directory.
	FilePlatform FileClass = "platform"
	FilePackage  FileClass = "package" // any other include directory (/usr/local/include)
)

// Parsed is one program's decoded units.
type Parsed struct {
	Program   Program   `json:"program"`
	Toolchain Toolchain `json:"toolchain"`
	// Units are every unit of the program, sorted by path; for a closure
	// program, the units the linker would take.
	Units []*Unit `json:"units"`
	// Main is the exact non-static main with a body; nil when the program
	// has none (a library).
	Main *Main `json:"main,omitempty"`
	// Outside are included .c files that none of the program's units entered
	// on this platform.
	Outside []string `json:"outside,omitempty"`
}

// Main is the program's main definition.
type Main struct {
	Unit string   `json:"unit"` // unit path
	At   Position `json:"at"`   // the name's position
	Node *Node    `json:"-"`    // the FunctionDecl in that unit's Decls
}

// Unit is one decoded translation unit.
type Unit struct {
	UnitSpec
	// Command is clang's complete argument list, without the clang program.
	Command []string `json:"command"`
	// Decls are the top-level declarations whose expansion location is a
	// corpus file, in dump order, with every location resolved. A header's
	// declarations appear in every unit that includes it, and a .c file
	// included by several units appears in each: identities merge by
	// definition location.
	Decls []*Node `json:"-"`
	// Includes is clang's -H include tree in the order files were entered.
	Includes []Include `json:"includes"`
	// External are top-level function, variable, record, typedef and enum
	// declarations outside the corpus, keyed by clang's node id in this dump.
	External []ExternalDecl `json:"external,omitempty"`
	// Diagnostics are clang's stderr lines other than the include tree.
	Diagnostics []string `json:"diagnostics,omitempty"`
	ElapsedMS   int64    `json:"elapsed_ms"`
	JSONBytes   int64    `json:"json_bytes"`
}

// Include is one entry of the -H include tree.
type Include struct {
	Path   string    `json:"path"` // corpus path, else the absolute path clang opened
	Class  FileClass `json:"class"`
	Depth  int       `json:"depth"`  // 1 for a file the unit includes directly
	Parent int       `json:"parent"` // index of the including entry; -1 for the unit itself
	// Line and Spelled are the #include directive in a corpus parent that
	// entered this file (sys/socket.h), when it could be matched.
	Line    int    `json:"line,omitempty"`
	Spelled string `json:"spelled,omitempty"`
}

// ExternalDecl is a declaration outside the corpus that corpus code may name.
type ExternalDecl struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Name     string    `json:"name,omitempty"`
	TagUsed  string    `json:"tag_used,omitempty"`
	Type     string    `json:"type,omitempty"`
	Position Position  `json:"position"`
	Class    FileClass `json:"class"`
	// Package is the first header on the declaring header's -H chain that
	// a corpus file includes itself, as that directive spells it
	// (stdlib.h for qsort declared in _stdlib.h). Empty when the chain
	// reaches no corpus file (a declaration from the command line).
	Package string `json:"package,omitempty"`
}

// Position is one resolved clang source position. File is a corpus path, or
// the absolute path of a file outside the corpus ("<scratch space>" and
// "<built-in>" as clang names them). Offset is the byte offset in File and
// TokLen the length of the token there.
type Position struct {
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	Col    int    `json:"col,omitempty"`
	Offset int    `json:"offset,omitempty"`
	TokLen int    `json:"tok_len,omitempty"`
}

// Valid reports a position clang printed.
func (position Position) Valid() bool { return position.File != "" && position.Line > 0 }

// Loc is a clang source location. Outside macros Spelling equals Expansion.
// For a token a macro produced, Spelling is where the token is written (in the
// macro body, or in the macro argument at the call site) and Expansion is the
// outermost macro name at the site the reader sees.
type Loc struct {
	Spelling  Position `json:"spelling,omitzero"`
	Expansion Position `json:"expansion,omitzero"`
	// MacroArg is clang's isMacroArgExpansion: the token was written in a
	// macro argument.
	MacroArg bool `json:"macro_arg,omitempty"`
}

// FromMacro reports a location a macro expansion produced.
func (loc Loc) FromMacro() bool { return loc.Spelling != loc.Expansion }

// InMacroBody reports a token the macro body wrote rather than the reader:
// its Expansion names the macro at the reader's site and its Spelling points
// into the macro definition.
func (loc Loc) InMacroBody() bool { return loc.FromMacro() && loc.Site() == loc.Expansion }

// Site is where the reader finds the construct: the spelling of a token
// written in a macro argument in the same file as the macro use
// (redisAssert(anetNonBlock(...)) keeps anetNonBlock's own column), else the
// expansion.
func (loc Loc) Site() Position {
	if loc.MacroArg && loc.Spelling.Valid() && loc.Spelling.File == loc.Expansion.File && loc.Spelling.Offset >= loc.Expansion.Offset {
		return loc.Spelling
	}
	return loc.Expansion
}

// Node is one decoded clang AST node. Attributes clang leaves out are zero.
// Empty child slots (a for loop without an init) are nodes with no Kind, so
// child positions keep clang's meaning.
type Node struct {
	ID    string `json:"id,omitempty"`
	Kind  string `json:"kind,omitempty"`
	Loc   Loc    `json:"loc,omitzero"` // declarations
	Begin Loc    `json:"begin,omitzero"`
	End   Loc    `json:"end,omitzero"`

	Name         string `json:"name,omitempty"`
	Type         Type   `json:"type,omitzero"`
	StorageClass string `json:"storage_class,omitempty"` // static, extern, or empty
	Inline       bool   `json:"inline,omitempty"`
	Variadic     bool   `json:"variadic,omitempty"`
	IsImplicit   bool   `json:"is_implicit,omitempty"`
	IsUsed       bool   `json:"is_used,omitempty"`
	IsReferenced bool   `json:"is_referenced,omitempty"`
	PreviousDecl string `json:"previous_decl,omitempty"`
	Init         string `json:"init,omitempty"` // VarDecl: c, call or list
	// Records.
	TagUsed            string `json:"tag_used,omitempty"`
	CompleteDefinition bool   `json:"complete_definition,omitempty"`
	IsBitfield         bool   `json:"is_bitfield,omitempty"`
	// Expressions.
	CastKind             string   `json:"cast_kind,omitempty"`
	Opcode               string   `json:"opcode,omitempty"`
	IsPostfix            bool     `json:"is_postfix,omitempty"`
	IsArrow              bool     `json:"is_arrow,omitempty"`
	Value                string   `json:"value,omitempty"` // literal value as clang prints it
	ReferencedDecl       *DeclRef `json:"referenced_decl,omitempty"`
	ReferencedMemberDecl string   `json:"referenced_member_decl,omitempty"`
	// Type nodes: Decl is the declaration a RecordType, TypedefType or
	// EnumType names; OwnedTagDecl is the tag an ElaboratedType declares
	// (the record of typedef struct {...} Name).
	Decl         *DeclRef `json:"decl,omitempty"`
	OwnedTagDecl *DeclRef `json:"owned_tag_decl,omitempty"`
	// Statements.
	HasElse           bool   `json:"has_else,omitempty"`
	TargetLabelDeclID string `json:"target_label_decl_id,omitempty"`

	Inner []*Node `json:"inner,omitempty"`
	// ArrayFiller is the implicit value of an InitListExpr's elements
	// nobody wrote.
	ArrayFiller *Node `json:"array_filler,omitempty"`
}

// Type is clang's printed type.
type Type struct {
	QualType  string `json:"qual_type,omitempty"`
	Desugared string `json:"desugared,omitempty"`
}

// DeclRef is clang's reference to a declaration by node id.
type DeclRef struct {
	ID   string `json:"id,omitempty"`
	Kind string `json:"kind,omitempty"`
	Name string `json:"name,omitempty"`
	Type Type   `json:"type,omitzero"`
}

// TokenText returns the token at position in source, the file's bytes: the
// macro name written at a macro use, or a field name as written.
func TokenText(source []byte, position Position) string {
	if position.Offset < 0 || position.TokLen <= 0 || position.Offset+position.TokLen > len(source) {
		return ""
	}
	return string(source[position.Offset : position.Offset+position.TokLen])
}
