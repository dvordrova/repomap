package cproject

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/dvordrova/repomap/internal/corpus"
)

// fortifyOff keeps libc calls under their written names: with fortification
// the SDK turns snprintf, memcpy and friends into __builtin___*_chk at every
// optimisation level.
var fortifyOff = []string{"-U_FORTIFY_SOURCE", "-D_FORTIFY_SOURCE=0"}

// legacyWarnings stay warnings, which -w silences. clang 16 and later make
// them errors by default for C99 and later (implicit int, implicit function
// declarations, integer/pointer and function pointer mismatches, a return
// without a value), and -w does not reach a warning that is an error by
// default. The compilers such code was built with accept it, and clang still
// builds the complete AST. An older clang ignores a name it does not know.
var legacyWarnings = []string{
	"-Wno-error=implicit-function-declaration", "-Wno-error=implicit-int", "-Wno-error=int-conversion",
	"-Wno-error=incompatible-function-pointer-types", "-Wno-error=incompatible-pointer-types",
	"-Wno-error=return-type", "-Wno-error=return-mismatch",
}

// overrides are added after every unit's own flags; Toolchain.Overrides
// records them.
func overrides() []string { return slices.Concat(fortifyOff, legacyWarnings) }

// clangProgram is the clang executable looked up on PATH.
const clangProgram = "clang"

// ErrClangUnavailable marks a program that failed because clang could not be
// found or run: the required tool is missing, not the program's sources.
var ErrClangUnavailable = errors.New("clang is unavailable")

var toolchainCache sync.Map // clang path -> Toolchain

// probeToolchain records the platform view: clang's version, target, sysroot
// and system include directories. It is probed once per process.
func probeToolchain(ctx context.Context) Toolchain {
	path, err := exec.LookPath(clangProgram)
	if err != nil {
		return Toolchain{Clang: clangProgram, Overrides: overrides(), Err: "clang was not found on PATH (install clang)"}
	}
	if cached, ok := toolchainCache.Load(path); ok {
		return cached.(Toolchain)
	}
	tool := Toolchain{Clang: path, Overrides: overrides()}
	run := func(args ...string) (string, string, error) {
		cmd := exec.CommandContext(ctx, path, args...)
		cmd.Env = append(os.Environ(), "LC_ALL=C")
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}
	version, _, err := run("--version")
	if err != nil {
		tool.Err = fmt.Sprintf("clang --version: %v", err)
		return tool
	}
	tool.Version, _, _ = strings.Cut(strings.TrimSpace(version), "\n")
	if target, _, err := run("-print-target-triple"); err == nil {
		tool.Target = strings.TrimSpace(target)
	}
	if resource, _, err := run("-print-resource-dir"); err == nil {
		tool.ResourceDir = filepath.Clean(strings.TrimSpace(resource))
	}
	_, verbose, err := run("-E", "-v", "-x", "c", os.DevNull, "-o", os.DevNull)
	if err != nil {
		tool.Err = fmt.Sprintf("clang -E -v: %v: %s", err, strings.TrimSpace(verbose))
		return tool
	}
	search := parseSearchList(verbose)
	tool.Sysroot = search.sysroot
	roots := platformRoots(tool.ResourceDir, search.sysroot, search.installed)
	for _, dir := range search.dirs {
		class := FilePackage
		if underAny(roots, dir.Path) {
			class = FilePlatform
		}
		tool.SearchDirs = append(tool.SearchDirs, SearchDir{Path: dir.Path, Class: class, Framework: dir.Framework})
	}
	if ctx.Err() == nil {
		toolchainCache.Store(path, tool)
	}
	return tool
}

type searchList struct {
	sysroot, installed string
	dirs               []SearchDir
}

// parseSearchList reads `clang -E -v`: the cc1 line's -isysroot, the
// InstalledDir, and the include search list.
func parseSearchList(verbose string) searchList {
	var list searchList
	inList := false
	for _, line := range strings.Split(verbose, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "InstalledDir:"):
			list.installed = filepath.Clean(strings.TrimSpace(strings.TrimPrefix(trimmed, "InstalledDir:")))
		case strings.Contains(line, " -cc1 "):
			fields := strings.Fields(line)
			for i, field := range fields {
				if (field == "-isysroot" || field == "--sysroot") && i+1 < len(fields) {
					list.sysroot = filepath.Clean(strings.Trim(fields[i+1], `"`))
				} else if value, ok := strings.CutPrefix(field, "--sysroot="); ok {
					list.sysroot = filepath.Clean(strings.Trim(value, `"`))
				}
			}
		case strings.HasPrefix(trimmed, "#include") && strings.HasSuffix(trimmed, "search starts here:"):
			inList = true
		case trimmed == "End of search list.":
			inList = false
		case inList && trimmed != "":
			dir, framework := strings.CutSuffix(trimmed, " (framework directory)")
			list.dirs = append(list.dirs, SearchDir{Path: filepath.Clean(dir), Framework: framework})
		}
	}
	return list
}

// platformRoots are the directories whose headers are the platform: the
// resource directory, the sysroot's own include and framework directories,
// and the toolchain's include directory. Anything else (/usr/local/include,
// /opt/homebrew/include) is a package.
func platformRoots(resourceDir, sysroot, installed string) []string {
	if sysroot == "" {
		sysroot = "/"
	}
	roots := []string{
		filepath.Join(sysroot, "usr", "include"),
		filepath.Join(sysroot, "System", "Library", "Frameworks"),
		filepath.Join(sysroot, "System", "Library", "SubFrameworks"),
	}
	if resourceDir != "" {
		roots = append(roots, resourceDir)
	}
	if installed != "" {
		roots = append(roots, filepath.Join(filepath.Dir(installed), "include"))
	}
	return roots
}

func underAny(dirs []string, path string) bool {
	for _, dir := range dirs {
		if _, ok := under(dir, path); ok {
			return true
		}
	}
	return false
}

// Flags the adapter keeps from a build's compile line. They change what the
// preprocessor and parser see; everything else (warnings, optimisation,
// debug, dependency output, code generation, and flags only gcc accepts) is
// dropped so clang parses exactly the build's view or fails for a reason in
// the source.
var (
	flagsWithValue = map[string]bool{
		"-D": true, "-U": true, "-I": true, "-include": true, "-imacros": true, "-isystem": true, "-iquote": true, "-idirafter": true,
		"-isysroot": true, "--sysroot": true, "-target": true, "-arch": true, "-F": true, "-iframework": true,
		// Dropped, but their value is not an input.
		"-o": true, "-x": true, "-MF": true, "-MT": true, "-MQ": true, "-Xclang": true, "-Xpreprocessor": true, "-Xlinker": true,
		"-Xassembler": true, "-L": true, "-l": true, "-framework": true, "-install_name": true, "-aux-info": true, "-include-pch": true,
		"-segalign": true, "-seg1addr": true, "-exported_symbols_list": true, "-unexported_symbols_list": true,
	}
	keptWithValue = map[string]bool{
		"-D": true, "-U": true, "-I": true, "-include": true, "-imacros": true, "-isystem": true, "-iquote": true, "-idirafter": true,
		"-isysroot": true, "--sysroot": true, "-target": true, "-arch": true, "-F": true, "-iframework": true,
	}
	keptExact = map[string]bool{
		"-pthread": true, "-ansi": true, "-nostdinc": true, "-nostdlibinc": true, "-nobuiltininc": true, "-undef": true, "-trigraphs": true,
		"-m16": true, "-m32": true, "-m64": true, "-mx32": true, "-mthumb": true, "-marm": true,
		"-fsigned-char": true, "-funsigned-char": true, "-fno-signed-char": true, "-fno-unsigned-char": true,
		"-fshort-enums": true, "-fno-short-enums": true, "-fshort-wchar": true, "-fno-short-wchar": true,
		"-ffreestanding": true, "-fhosted": true, "-fno-builtin": true, "-fbuiltin": true,
		"-fms-extensions": true, "-fno-ms-extensions": true, "-fgnu89-inline": true, "-fno-gnu89-inline": true,
		"-fblocks": true, "-fno-blocks": true, "-fdollars-in-identifiers": true, "-fno-dollars-in-identifiers": true,
		"-ftrigraphs": true, "-fno-trigraphs": true, "-fgnu-keywords": true, "-fno-gnu-keywords": true, "-fasm": true, "-fno-asm": true,
		"-fPIC": true, "-fpic": true, "-fPIE": true, "-fpie": true, "-fno-PIC": true, "-fno-pic": true, "-fno-PIE": true, "-fno-pie": true,
		"-fcommon": true, "-fno-common": true, "-fwrapv": true, "-fno-wrapv": true,
		"-fstack-protector": true, "-fstack-protector-strong": true, "-fstack-protector-all": true, "-fno-stack-protector": true,
	}
	keptPrefix = []string{"-D", "-U", "-I", "-std=", "--sysroot=", "--target=", "-isystem", "-iquote", "-idirafter", "-F",
		"-march=", "-mcpu=", "-mtune=", "-mfloat-abi=", "-mabi=", "-fno-builtin-"}
	targetFeature = regexp.MustCompile(`^-m(no-)?(sse[0-9.a-z]*|ssse3|avx[0-9a-z]*|fma4?|bmi2?|pclmul|aes|popcnt|lzcnt|f16c|crc32|sha|rdrnd|rdseed|mmx|adx|xsave|movbe|neon|vsx|altivec|crypto)$|^-m[a-z]+-version-min=`)
)

// compileFlags splits a compiler command's arguments (without the program)
// into the flags the adapter keeps and the ones it drops, and returns the
// inputs (sources, objects, archives) and the -o output. Only the first
// -arch is kept: several make clang parse the unit once per architecture.
func compileFlags(args []string) (kept, dropped, inputs []string, output string, compileOnly, preprocessOnly bool) {
	arch := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			inputs = append(inputs, arg)
			continue
		}
		switch arg {
		case "-c":
			compileOnly = true
			continue
		case "-E", "-M", "-MM", "-S", "-fsyntax-only":
			preprocessOnly = true
			dropped = append(dropped, arg)
			continue
		}
		if flagsWithValue[arg] {
			value := ""
			if i+1 < len(args) {
				value = args[i+1]
				i++
			}
			switch {
			case arg == "-o":
				output = value
			case arg == "-arch" && arch:
				dropped = append(dropped, arg, value)
			case keptWithValue[arg]:
				arch = arch || arg == "-arch"
				kept = append(kept, arg, value)
			default:
				dropped = append(dropped, arg, value)
			}
			continue
		}
		if keptExact[arg] || targetFeature.MatchString(arg) {
			kept = append(kept, arg)
			continue
		}
		keep := false
		for _, prefix := range keptPrefix {
			if strings.HasPrefix(arg, prefix) && len(arg) > len(prefix) {
				keep = true
				break
			}
		}
		if keep {
			kept = append(kept, arg)
		} else {
			dropped = append(dropped, arg)
		}
	}
	return kept, dropped, inputs, output, compileOnly, preprocessOnly
}

// Store shares parsed units between the programs of one run.
type Store struct {
	parses chan struct{}

	mu         sync.Mutex
	units      map[string]*unitEntry
	directives map[string][]directive
	toolchain  *Toolchain
	corpusSet  map[string]map[string]bool
}

type unitEntry struct {
	done chan struct{}
	unit *Unit
	err  error
}

// NewStore returns an empty store that runs at most four clang processes at a
// time.
func NewStore() *Store {
	return &Store{parses: make(chan struct{}, 4), units: map[string]*unitEntry{}, directives: map[string][]directive{}, corpusSet: map[string]map[string]bool{}}
}

func (store *Store) tool(ctx context.Context) Toolchain {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.toolchain == nil {
		tool := probeToolchain(ctx)
		if ctx.Err() != nil {
			return tool
		}
		store.toolchain = &tool
	}
	return *store.toolchain
}

func (store *Store) corpusPaths(repository *corpus.Corpus) map[string]bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	if set, ok := store.corpusSet[repository.SHA256()]; ok {
		return set
	}
	set := map[string]bool{}
	for _, entry := range repository.Entries() {
		set[entry.Path] = true
	}
	store.corpusSet[repository.SHA256()] = set
	return set
}

func unitKey(spec UnitSpec) string {
	return strings.Join(append([]string{spec.Path, spec.Dir, spec.Source}, spec.Args...), "\x00")
}

// unit parses spec once per store; concurrent callers wait for the first.
func (store *Store) unit(ctx context.Context, env parseEnv, spec UnitSpec) (*Unit, error) {
	key := unitKey(spec)
	store.mu.Lock()
	entry, ok := store.units[key]
	if !ok {
		entry = &unitEntry{done: make(chan struct{})}
		store.units[key] = entry
	}
	store.mu.Unlock()
	if ok {
		select {
		case <-entry.done:
			return entry.unit, entry.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	entry.unit, entry.err = store.parse(ctx, env, spec)
	if entry.err != nil && ctx.Err() != nil {
		// A cancelled parse is not a result another run may reuse.
		store.mu.Lock()
		delete(store.units, key)
		store.mu.Unlock()
	}
	close(entry.done)
	return entry.unit, entry.err
}

// parseEnv is what every unit parse of one program shares.
type parseEnv struct {
	root       string   // absolute repository root
	roots      []string // root and root with symlinks resolved
	repository *corpus.Corpus
	corpus     map[string]bool
	tool       Toolchain
}

func (store *Store) acquire(ctx context.Context) error {
	select {
	case store.parses <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (store *Store) release() { <-store.parses }

// clangArgs is the complete argument list for one unit.
func clangArgs(spec UnitSpec, tool Toolchain, dump ...string) []string {
	args := slices.Clone(spec.Args)
	args = append(args, tool.Overrides...)
	args = append(args, "-fsyntax-only", "-w", "-fno-color-diagnostics", "-fno-caret-diagnostics")
	args = append(args, dump...)
	return append(args, spec.Source)
}

// parse runs `clang -fsyntax-only -w -H -Xclang -ast-dump=json` on one unit
// and decodes its output as it streams.
func (store *Store) parse(ctx context.Context, env parseEnv, spec UnitSpec) (*Unit, error) {
	if err := store.acquire(ctx); err != nil {
		return nil, err
	}
	defer store.release()
	started := time.Now()
	args := clangArgs(spec, env.tool, "-H", "-Xclang", "-ast-dump=json")
	cwd := filepath.Join(env.root, filepath.FromSlash(spec.Dir))
	cmd := exec.CommandContext(ctx, env.tool.Clang, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("C native analysis (install clang): %w: %w", err, ErrClangUnavailable)
	}
	names := &fileNames{cwd: cwd, roots: env.roots, corpus: env.corpus, cache: map[string]fileName{}}
	decls, external, size, decodeErr := decodeUnit(stdout, names)
	if decodeErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	includes, diagnostics := parseIncludeTree(stderr.String(), names)
	var failures []string
	for _, line := range diagnostics {
		if strings.Contains(line, "error:") {
			failures = append(failures, line)
		}
	}
	switch {
	case waitErr != nil || len(failures) > 0:
		if len(failures) == 0 {
			failures = diagnostics
		}
		return nil, unitFailure(spec, waitErr, failures)
	case decodeErr != nil:
		return nil, fmt.Errorf("C native analysis of %s: %w", spec.Path, decodeErr)
	}
	unit := &Unit{UnitSpec: spec, Command: args, Decls: decls, Includes: includes, External: external, Diagnostics: diagnostics, ElapsedMS: time.Since(started).Milliseconds(), JSONBytes: size}
	classes := newClassifier(env, spec, cwd)
	for i := range unit.Includes {
		unit.Includes[i].Class = classes.class(unit.Includes[i].Path)
	}
	store.matchDirectives(env.repository, unit, classes)
	named := store.namedByCorpus(env.repository, unit, classes)
	for i := range unit.External {
		declaration := &unit.External[i]
		declaration.Class = classes.class(declaration.Position.File)
		declaration.Package = classes.packageOf(unit, named, declaration.Position.File)
	}
	return unit, nil
}

func unitFailure(spec UnitSpec, waitErr error, lines []string) error {
	const shown = 8
	detail := strings.Join(lines[:min(len(lines), shown)], "; ")
	if len(lines) > shown {
		detail += fmt.Sprintf("; and %d more", len(lines)-shown)
	}
	if waitErr != nil {
		var exit *exec.ExitError
		if errors.As(waitErr, &exit) {
			return fmt.Errorf("clang could not parse %s (exit status %d): %s", spec.Path, exit.ExitCode(), detail)
		}
		return fmt.Errorf("clang could not parse %s: %v: %s", spec.Path, waitErr, detail)
	}
	return fmt.Errorf("clang could not parse %s: %s", spec.Path, detail)
}

// parseIncludeTree separates clang's -H lines (". ./x.h", ".. /usr/include/y.h")
// from its diagnostics and rebuilds the include tree.
func parseIncludeTree(stderr string, names *fileNames) ([]Include, []string) {
	var includes []Include
	var diagnostics []string
	var stack []int // entry index by depth-1
	for _, line := range strings.Split(stderr, "\n") {
		depth := 0
		for depth < len(line) && line[depth] == '.' {
			depth++
		}
		if depth == 0 || depth >= len(line) || line[depth] != ' ' {
			if strings.TrimSpace(line) != "" {
				diagnostics = append(diagnostics, line)
			}
			continue
		}
		path := names.normalize(line[depth+1:])
		parent := -1
		if depth > 1 && depth-2 < len(stack) {
			parent = stack[depth-2]
		}
		stack = append(stack[:min(depth-1, len(stack))], len(includes))
		includes = append(includes, Include{Path: path, Depth: depth, Parent: parent})
	}
	return includes, diagnostics
}

// directive is one #include line of a corpus file.
type directive struct {
	line    int
	spelled string
	quoted  bool
}

var includeDirective = regexp.MustCompile(`^\s*#\s*(?:include|include_next|import)\s*([<"])([^>"]+)[>"]`)

func scanDirectives(source []byte) []directive {
	var directives []directive
	for number, line := range bytes.Split(source, []byte("\n")) {
		if match := includeDirective.FindSubmatch(line); match != nil {
			directives = append(directives, directive{line: number + 1, spelled: string(match[2]), quoted: match[1][0] == '"'})
		}
	}
	return directives
}

func (store *Store) fileDirectives(repository *corpus.Corpus, path string) []directive {
	store.mu.Lock()
	directives, ok := store.directives[path]
	store.mu.Unlock()
	if ok {
		return directives
	}
	if id, found := repository.ID(path); found {
		if content, err := repository.ReadFileAll(id); err == nil {
			directives = scanDirectives(content.Bytes)
		}
	}
	store.mu.Lock()
	store.directives[path] = directives
	store.mu.Unlock()
	return directives
}

// matchDirectives gives each include entered from a corpus file the line and
// spelling of the directive that entered it. Entries under one parent follow
// the parent's directives in order; a guarded or inactive directive enters
// nothing and is passed over.
func (store *Store) matchDirectives(repository *corpus.Corpus, unit *Unit, classes *classifier) {
	cursors := map[int]int{}
	for i := range unit.Includes {
		entry := &unit.Includes[i]
		parent := unit.Path
		if entry.Parent >= 0 {
			if unit.Includes[entry.Parent].Class != FileCorpus {
				continue
			}
			parent = unit.Includes[entry.Parent].Path
		}
		directives := store.fileDirectives(repository, parent)
		for j := cursors[entry.Parent]; j < len(directives); j++ {
			if classes.names(directives[j], parent, entry.Path) {
				entry.Line, entry.Spelled = directives[j].line, directives[j].spelled
				cursors[entry.Parent] = j + 1
				break
			}
		}
	}
}

// namedByCorpus gives, for each include entry that is not a corpus file, the
// spelling of a directive in a corpus file of this unit that names it, even
// when an include guard made that directive enter nothing.
func (store *Store) namedByCorpus(repository *corpus.Corpus, unit *Unit, classes *classifier) []string {
	files := []string{unit.Path}
	seen := map[string]bool{unit.Path: true}
	for _, include := range unit.Includes {
		if include.Class == FileCorpus && !seen[include.Path] {
			seen[include.Path] = true
			files = append(files, include.Path)
		}
	}
	named := make([]string, len(unit.Includes))
	for _, file := range files {
		for _, d := range store.fileDirectives(repository, file) {
			if d.quoted && classes.corpus[path.Join(path.Dir(file), d.spelled)] {
				continue // "x.h" finds the repository's own x.h first
			}
			for i, include := range unit.Includes {
				if named[i] == "" && include.Class != FileCorpus && classes.names(d, file, include.Path) {
					named[i] = path.Clean(d.spelled)
				}
			}
		}
	}
	return named
}

// classifier decides who owns a file clang read and which file a directive
// finds.
type classifier struct {
	root       string
	roots      []string
	corpus     map[string]bool
	platform   []string
	search     []string // every include directory, longest first
	frameworks map[string]bool
}

// names reports whether the directive d in the corpus file parent finds file
// (a corpus path or an absolute path): next to parent for a quoted name, or
// in one of the unit's include directories.
func (c *classifier) names(d directive, parent, file string) bool {
	target := file
	if !filepath.IsAbs(target) {
		target = filepath.Join(c.root, filepath.FromSlash(target))
	}
	spelled := filepath.FromSlash(d.spelled)
	if d.quoted && filepath.Join(c.root, filepath.FromSlash(path.Dir(parent)), spelled) == target {
		return true
	}
	for _, dir := range c.search {
		if filepath.Join(dir, spelled) == target {
			return true
		}
		// <Framework/Header.h> finds Framework.framework/Headers/Header.h.
		if framework, header, ok := strings.Cut(d.spelled, "/"); ok && c.frameworks[dir] && filepath.Join(dir, framework+".framework", "Headers", filepath.FromSlash(header)) == target {
			return true
		}
	}
	return false
}

func newClassifier(env parseEnv, spec UnitSpec, cwd string) *classifier {
	c := &classifier{root: env.root, roots: env.roots, corpus: env.corpus, frameworks: map[string]bool{}}
	sysroot := env.tool.Sysroot
	var own []string
	for i := 0; i < len(spec.Args); i++ {
		arg, value := spec.Args[i], ""
		if keptWithValue[arg] && i+1 < len(spec.Args) {
			value = spec.Args[i+1]
			i++
		} else {
			for _, prefix := range []string{"-I", "-isystem", "-iquote", "-idirafter", "--sysroot=", "-F"} {
				if rest, ok := strings.CutPrefix(arg, prefix); ok && rest != "" {
					arg, value = strings.TrimSuffix(prefix, "="), rest
					if prefix == "--sysroot=" {
						arg = "--sysroot"
					}
					break
				}
			}
		}
		if value == "" {
			continue
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(cwd, value)
		}
		switch arg {
		case "-isysroot", "--sysroot":
			sysroot = filepath.Clean(value)
		case "-I", "-isystem", "-iquote", "-idirafter":
			own = append(own, filepath.Clean(value))
		case "-F", "-iframework":
			own = append(own, filepath.Clean(value))
			c.frameworks[filepath.Clean(value)] = true
		}
	}
	c.platform = platformRoots(env.tool.ResourceDir, sysroot, "")
	for _, dir := range env.tool.SearchDirs {
		if dir.Class == FilePlatform {
			c.platform = append(c.platform, dir.Path)
		}
		c.search = append(c.search, dir.Path)
		if dir.Framework {
			c.frameworks[dir.Path] = true
		}
	}
	c.search = append(c.search, own...)
	slices.SortStableFunc(c.search, func(a, b string) int { return len(b) - len(a) })
	return c
}

func (c *classifier) class(path string) FileClass {
	switch {
	case path == "":
		return ""
	case strings.HasPrefix(path, "<"):
		return FilePlatform // <built-in>, <scratch space>, <command line>
	case !filepath.IsAbs(path):
		return FileCorpus
	case underAny(c.roots, path):
		return FileRepository
	case underAny(c.platform, path):
		return FilePlatform
	default:
		return FilePackage
	}
}

// packageOf names the header a corpus file includes on the way to file:
// walking file's -H chain towards the unit, the first header that a corpus
// directive names (stdlib.h for qsort declared in _stdlib.h), as that
// directive spells it. A chain no corpus directive names ends at the header
// a corpus file entered, named relative to its include directory.
func (c *classifier) packageOf(unit *Unit, named []string, file string) string {
	at := -1
	for i, entry := range unit.Includes {
		if entry.Path == file {
			at = i
			break
		}
	}
	for at >= 0 {
		entry := unit.Includes[at]
		if named[at] != "" {
			return named[at]
		}
		if entry.Parent < 0 || unit.Includes[entry.Parent].Class == FileCorpus {
			if entry.Spelled != "" {
				return filepath.ToSlash(filepath.Clean(entry.Spelled))
			}
			return c.searchRelative(entry.Path)
		}
		at = entry.Parent
	}
	return ""
}

func (c *classifier) searchRelative(path string) string {
	for _, dir := range c.search {
		if rel, ok := under(dir, path); ok && rel != "." {
			rel = filepath.ToSlash(rel)
			if framework, header, ok := strings.Cut(rel, ".framework/Headers/"); ok {
				return framework + "/" + header
			}
			return rel
		}
	}
	return filepath.ToSlash(path)
}
