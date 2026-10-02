package cproject

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/dvordrova/repomap/internal/corpus"
)

func discover(ctx context.Context, root string, repository *corpus.Corpus) (*Project, error) {
	if repository == nil {
		return nil, fmt.Errorf("C: repository is required")
	}
	// A .c file in a tooling directory (testdata/, .github/) is an input of
	// the tests or tools around it, never a unit: no dry run or clang probe
	// reads it.
	hasC := false
	for _, entry := range repository.Entries() {
		if path.Ext(entry.Path) == ".c" && !corpus.ToolingPath(entry.Path) {
			hasC = true
			break
		}
	}
	if !hasC {
		return nil, nil
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	project := &Project{Root: abs, CorpusSHA256: repository.SHA256(), Toolchain: probeToolchain(ctx)}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if project.Toolchain.Err != "" {
		project.Observations = append(project.Observations, Observation{Kind: "c_toolchain", Fields: map[string]string{"error": project.Toolchain.Err}})
	}
	env := parseEnv{root: abs, roots: rootsOf(abs), repository: repository, corpus: map[string]bool{}, tool: project.Toolchain}
	for _, entry := range repository.Entries() {
		env.corpus[entry.Path] = true
	}
	project.Included = scanIncluded(repository)
	included := map[string]bool{}
	for _, source := range project.Included {
		included[source.Path] = true
	}

	description, err := readBuild(ctx, env)
	if err != nil {
		return nil, err
	}
	project.Build = description.build
	if description.build.Err != "" {
		project.Observations = append(project.Observations, Observation{Kind: "c_build_error", Path: description.build.Path, Fields: map[string]string{"error": description.build.Err}})
	}
	// A directory's own makefile the root never reaches, as a developer runs
	// make there.
	var nestedObservations []Observation
	var nestedFailed map[string]string
	project.Nested, nestedObservations, nestedFailed = readNested(ctx, env, &description, func(file string) bool { return included[file] || corpus.ToolingPath(file) })
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	project.Observations = append(project.Observations, nestedObservations...)
	// Units the build compiles, then every other .c file with clang's defaults.
	var units []UnitSpec
	compiled := map[string]bool{}
	seenUnits := map[string]bool{}
	for _, record := range description.compiles {
		if key := unitKey(record.spec); !seenUnits[key] {
			seenUnits[key] = true
			units = append(units, record.spec)
		}
		compiled[record.spec.Path] = true
	}
	for _, entry := range repository.Entries() {
		if path.Ext(entry.Path) != ".c" || compiled[entry.Path] || included[entry.Path] || corpus.ToolingPath(entry.Path) {
			continue
		}
		units = append(units, UnitSpec{Path: entry.Path, FileRef: string(entry.ID), Dir: ".", Source: entry.Path})
	}

	programs, linked, observations := linkPrograms(env, description)
	project.Observations = append(project.Observations, observations...)

	// Units no link line links: a filtered parse finds an exact main.
	var unlinked []int
	for i, spec := range units {
		if !linked[unitKey(spec)] {
			unlinked = append(unlinked, i)
		}
	}
	if len(unlinked) > 0 && project.Toolchain.Err == "" {
		results := make([]mainResult, len(unlinked))
		var wait sync.WaitGroup
		slots := make(chan struct{}, 4)
		for n, i := range unlinked {
			wait.Add(1)
			go func() {
				defer wait.Done()
				slots <- struct{}{}
				defer func() { <-slots }()
				results[n] = findMains(ctx, env, units[i])
			}()
		}
		wait.Wait()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for n, i := range unlinked {
			result := results[n]
			if result.err != "" {
				project.Observations = append(project.Observations, Observation{Kind: "c_unit_error", Path: units[i].Path, Fields: map[string]string{"error": result.err}})
			}
			if len(result.mains) == 1 {
				main := result.mains[0]
				units[i].Main = &main
			}
		}
	}
	sortUnits(units)
	project.Units = units

	// A unit with main and no link line is a program whose files the linker
	// closure decides; the others are their directory's library.
	libraries := map[string][]UnitSpec{}
	for _, spec := range units {
		if linked[unitKey(spec)] {
			continue
		}
		if spec.Main == nil {
			dir := path.Dir(spec.Path)
			libraries[dir] = append(libraries[dir], spec)
			continue
		}
		var pool []UnitSpec
		for _, other := range units {
			if unitKey(other) != unitKey(spec) && other.Main == nil {
				pool = append(pool, other)
			}
		}
		program := Program{Selector: "c:" + spec.Path, Name: spec.Path, Kind: ProgramExecutable, Units: []UnitSpec{spec}, Closure: true, Pool: pool,
			Anchor:   Site{Path: spec.Main.File, Line: spec.Main.Line},
			Evidence: []Observation{{Kind: "c_main", Path: spec.Main.File, Line: spec.Main.Line}}}
		programs = append(programs, program)
	}
	for dir, specs := range libraries {
		sortUnits(specs)
		var paths []string
		for _, spec := range specs {
			paths = append(paths, spec.Path)
		}
		program := Program{Selector: "c:" + dir + "/", Name: dir, Kind: ProgramLibrary, Units: specs, Anchor: Site{Path: specs[0].Path},
			Evidence: []Observation{{Kind: "c_library", Path: dir, Values: paths}}}
		programs = append(programs, program)
	}

	seen := map[string]bool{}
	for _, program := range programs {
		if seen[program.Selector] {
			project.Observations = append(project.Observations, Observation{Kind: "c_duplicate_program", Path: program.Anchor.Path, Line: program.Anchor.Line, Fields: map[string]string{"selector": program.Selector}})
			continue
		}
		seen[program.Selector] = true
		ref, ok := repository.ID(program.Anchor.Path)
		if !ok {
			project.Observations = append(project.Observations, Observation{Kind: "c_unanchored_program", Path: program.Anchor.Path, Fields: map[string]string{"selector": program.Selector}})
			continue
		}
		program.AnchorFileRef = string(ref)
		program.CorpusSHA256 = project.CorpusSHA256
		program.Included = project.Included
		if !description.fromBuild {
			program.BuildErr = description.build.Err
		}
		// A unit its own makefile could not compile names that makefile's
		// failure.
		for _, spec := range program.Units {
			if reason := nestedFailed[spec.Path]; reason != "" && !spec.Built {
				program.BuildErr = reason
				break
			}
		}
		program.Ref = programRef(program)
		project.Programs = append(project.Programs, program)
	}
	slices.SortFunc(project.Programs, func(a, b Program) int { return strings.Compare(a.Selector, b.Selector) })
	return project, nil
}

func rootsOf(root string) []string {
	roots := []string{root}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		roots = append(roots, resolved)
	}
	return roots
}

func sortUnits(units []UnitSpec) {
	slices.SortStableFunc(units, func(a, b UnitSpec) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(unitKey(a), unitKey(b))
	})
}

var includedSource = regexp.MustCompile(`^\s*#\s*include\s*"([^"]+\.c)"`)

// scanIncluded finds .c files another corpus file pulls in with #include.
func scanIncluded(repository *corpus.Corpus) []IncludedSource {
	includers := map[string][]Site{}
	for _, entry := range repository.Entries() {
		if ext := path.Ext(entry.Path); ext != ".c" && ext != ".h" {
			continue
		}
		content, err := repository.ReadFileAll(entry.ID)
		if err != nil || !bytes.Contains(content.Bytes, []byte(`.c"`)) {
			continue
		}
		for number, line := range bytes.Split(content.Bytes, []byte("\n")) {
			match := includedSource.FindSubmatch(line)
			if match == nil {
				continue
			}
			spelled := string(match[1])
			for _, candidate := range []string{path.Join(path.Dir(entry.Path), spelled), path.Clean(spelled)} {
				if _, ok := repository.ID(candidate); ok {
					includers[candidate] = append(includers[candidate], Site{Path: entry.Path, Line: number + 1})
					break
				}
			}
		}
	}
	var out []IncludedSource
	for file, sites := range includers {
		out = append(out, IncludedSource{Path: file, Includers: sites})
	}
	slices.SortFunc(out, func(a, b IncludedSource) int { return strings.Compare(a.Path, b.Path) })
	return out
}

// mainResult is a filtered parse's exact main definitions and errors.
type mainResult struct {
	mains []Position
	err   string
}

// findMains runs clang with -ast-dump-filter=main on one unit and keeps only
// an exact, non-static FunctionDecl main with a body in a corpus file (the
// filter matches substrings such as remainder or getdomainname).
func findMains(ctx context.Context, env parseEnv, spec UnitSpec) mainResult {
	args := clangArgs(spec, env.tool, "-Xclang", "-ast-dump=json", "-Xclang", "-ast-dump-filter=main")
	cwd := filepath.Join(env.root, filepath.FromSlash(spec.Dir))
	cmd := exec.CommandContext(ctx, env.tool.Clang, args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return mainResult{err: err.Error()}
	}
	if err := cmd.Start(); err != nil {
		return mainResult{err: err.Error()}
	}
	names := &fileNames{cwd: cwd, roots: env.roots, corpus: env.corpus, cache: map[string]fileName{}}
	nodes, decodeErr := decodeFiltered(stdout, names)
	if decodeErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	var result mainResult
	var failures []string
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.Contains(line, "error:") {
			failures = append(failures, line)
		}
	}
	switch {
	case waitErr != nil || len(failures) > 0:
		if len(failures) == 0 {
			failures = []string{strings.TrimSpace(stderr.String())}
		}
		result.err = unitFailure(spec, waitErr, failures).Error()
	case decodeErr != nil:
		result.err = decodeErr.Error()
	}
	seen := map[Position]bool{}
	for _, node := range nodes {
		if exactMain(node) && env.corpus[node.Loc.Expansion.File] && !seen[node.Loc.Expansion] {
			seen[node.Loc.Expansion] = true
			result.mains = append(result.mains, node.Loc.Expansion)
		}
	}
	return result
}

func exactMain(node *Node) bool {
	return node.Kind == "FunctionDecl" && node.Name == "main" && node.StorageClass != "static" && !node.IsImplicit && hasBody(node)
}

func hasBody(node *Node) bool {
	for _, child := range node.Inner {
		if child.Kind == "CompoundStmt" {
			return true
		}
	}
	return false
}

func parse(ctx context.Context, root string, repository *corpus.Corpus, program Program, store *Store) (*Parsed, error) {
	if repository == nil {
		return nil, fmt.Errorf("C: repository is required")
	}
	if err := program.ValidateAgainst(repository); err != nil {
		return nil, err
	}
	if store == nil {
		store = NewStore()
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	tool := store.tool(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if tool.Err != "" {
		return nil, program.explain(fmt.Errorf("C native analysis (install clang): %s: %w", tool.Err, ErrClangUnavailable))
	}
	env := parseEnv{root: abs, roots: rootsOf(abs), repository: repository, corpus: store.corpusPaths(repository), tool: tool}
	specs := slices.Clone(program.Units)
	if program.Closure {
		specs = append(specs, program.Pool...)
	}
	units := make([]*Unit, len(specs))
	errs := make([]error, len(specs))
	var wait sync.WaitGroup
	for i, spec := range specs {
		wait.Add(1)
		go func() {
			defer wait.Done()
			units[i], errs[i] = store.unit(ctx, env, spec)
		}()
	}
	wait.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A pool unit that fails matters only when the closure may need it.
	var failed []int
	for i, err := range errs {
		if err == nil {
			continue
		}
		if !program.Closure || i == 0 {
			return nil, program.explain(err)
		}
		failed = append(failed, i)
	}
	if program.Closure {
		var pool []*Unit
		for i := 1; i < len(units); i++ {
			if errs[i] == nil {
				pool = append(pool, units[i])
			}
		}
		taken, unresolved, err := linkClosure(units[0], pool)
		if err != nil {
			return nil, program.explain(err)
		}
		for _, i := range failed {
			if name := firstMentioned(repository, specs[i], unresolved); name != "" {
				return nil, program.explain(fmt.Errorf("%w; it may define %s, which %s uses", errs[i], name, program.Name))
			}
		}
		units = taken
	}
	slices.SortStableFunc(units, func(a, b *Unit) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(unitKey(a.UnitSpec), unitKey(b.UnitSpec))
	})
	parsed := &Parsed{Program: program, Toolchain: tool, Units: units, Outside: outsideSources(program, units)}
	for _, unit := range units {
		for _, node := range unit.Decls {
			if !exactMain(node) {
				continue
			}
			if parsed.Main != nil && parsed.Main.At != node.Loc.Expansion {
				return nil, fmt.Errorf("C program %s defines main twice: %s:%d and %s:%d", program.Selector, parsed.Main.At.File, parsed.Main.At.Line, node.Loc.Expansion.File, node.Loc.Expansion.Line)
			}
			if parsed.Main == nil {
				parsed.Main = &Main{Unit: unit.Path, At: node.Loc.Expansion, Node: node}
			}
		}
	}
	return parsed, nil
}

// explain adds the build description failure that left the units with
// clang's defaults.
func (program Program) explain(err error) error {
	if program.BuildErr == "" {
		return err
	}
	return fmt.Errorf("%w (the build description could not be read, so clang's defaults were used: %s)", err, program.BuildErr)
}

// outsideSources are included .c files whose includer a unit entered but that
// no unit entered itself: an #if chose another file on this platform.
func outsideSources(program Program, units []*Unit) []string {
	entered := map[string]bool{}
	own := map[string]bool{}
	for _, unit := range units {
		entered[unit.Path], own[unit.Path] = true, true
		for _, include := range unit.Includes {
			if include.Class == FileCorpus {
				entered[include.Path] = true
			}
		}
	}
	var outside []string
	for _, source := range program.Included {
		if own[source.Path] || entered[source.Path] {
			continue
		}
		for _, includer := range source.Includers {
			if entered[includer.Path] {
				outside = append(outside, source.Path)
				break
			}
		}
	}
	return outside
}
