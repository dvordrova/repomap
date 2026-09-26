package cproject

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// dryRunTimeout bounds `make -n -B`. The dry run prints recipes without
// running them, but GNU make still runs $(shell ...), recipes that call
// $(MAKE), and rules that remake included makefiles.
var dryRunTimeout = 30 * time.Second

// buildDescription is what a compile_commands.json or a dry run printed.
type buildDescription struct {
	build     Build
	fromBuild bool // compile flags came from the build description
	compiles  []compileRecord
	links     []linkRecord
	archives  map[string][]string // absolute archive or partial-link object -> absolute members
}

type compileRecord struct {
	spec   UnitSpec
	object string // absolute object path, or a synthetic key for compile-and-link lines
}

type linkRecord struct {
	written string // -o as the command wrote it
	output  string // absolute output path
	dir     string // absolute directory the command ran in
	inputs  []string
	args    []string
	shared  bool
}

func readBuild(ctx context.Context, env parseEnv) (buildDescription, error) {
	if raw, err := os.ReadFile(filepath.Join(env.root, "compile_commands.json")); err == nil {
		description, err := readCompileCommands(env, raw)
		if err == nil {
			return description, nil
		}
		return buildDescription{build: Build{Kind: BuildNone, Path: "compile_commands.json", Err: err.Error()}}, nil
	}
	// The names as they are on disk: a case-insensitive file system would
	// find Makefile under any spelling.
	present := map[string]bool{}
	if entries, err := os.ReadDir(env.root); err == nil {
		for _, entry := range entries {
			present[entry.Name()] = !entry.IsDir()
		}
	}
	makefile := ""
	for _, name := range []string{"GNUmakefile", "makefile", "Makefile"} {
		if present[name] {
			makefile = name
			break
		}
	}
	if makefile == "" {
		return buildDescription{build: Build{Kind: BuildNone}}, nil
	}
	command := []string{"make", "-n", "-B", "-w"}
	output, err := dryRun(ctx, env.root, command)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return buildDescription{}, ctxErr
	}
	if err != nil {
		return buildDescription{build: Build{Kind: BuildNone, Path: makefile, Command: command, Err: err.Error()}}, nil
	}
	description := parseDryRun(env, output)
	description.build = Build{Kind: BuildMake, Path: makefile, Command: command}
	description.fromBuild = true
	return description, nil
}

// dryRun runs make without its caller's make environment and returns stdout.
func dryRun(ctx context.Context, root string, command []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, dryRunTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = root
	for _, variable := range os.Environ() {
		name, _, _ := strings.Cut(variable, "=")
		switch name {
		case "MAKEFLAGS", "MFLAGS", "MAKELEVEL", "MAKEFILES", "GNUMAKEFLAGS", "MAKEOVERRIDES", "LC_ALL":
			continue
		}
		cmd.Env = append(cmd.Env, variable)
	}
	cmd.Env = append(cmd.Env, "LC_ALL=C")
	ownProcessGroup(cmd)
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("%s did not finish within %s", strings.Join(command, " "), dryRunTimeout)
	}
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("%s failed: %s", strings.Join(command, " "), detail)
	}
	return stdout.String(), nil
}

func readCompileCommands(env parseEnv, raw []byte) (buildDescription, error) {
	var entries []struct {
		Directory string   `json:"directory"`
		File      string   `json:"file"`
		Arguments []string `json:"arguments"`
		Command   string   `json:"command"`
		Output    string   `json:"output"`
	}
	if err := json.Unmarshal(raw, &entries); err != nil {
		return buildDescription{}, fmt.Errorf("compile_commands.json: %w", err)
	}
	description := buildDescription{build: Build{Kind: BuildCompileCommands, Path: "compile_commands.json"}, fromBuild: true}
	for _, entry := range entries {
		argv := entry.Arguments
		if len(argv) == 0 {
			commands, ok := shellCommands(entry.Command)
			if !ok || len(commands) == 0 {
				continue
			}
			argv = commands[0]
		}
		argv = commandWords(slices.Clone(argv))
		if len(argv) < 2 || path.Ext(entry.File) != ".c" {
			continue
		}
		dir := entry.Directory
		if !filepath.IsAbs(dir) {
			dir = filepath.Join(env.root, dir)
		}
		kept, dropped, _, _, _, _ := compileFlags(argv[1:])
		file := entry.File
		if spec, ok := unitSpec(env, filepath.Clean(dir), file, kept, dropped); ok {
			description.compiles = append(description.compiles, compileRecord{spec: spec, object: entry.Output})
		}
	}
	return description, nil
}

// unitSpec binds a compiled source to its corpus file.
func unitSpec(env parseEnv, dir, source string, kept, dropped []string) (UnitSpec, bool) {
	file := source
	if !filepath.IsAbs(file) {
		file = filepath.Join(dir, file)
	}
	file = filepath.Clean(file)
	var rel, relDir string
	for _, root := range env.roots {
		if value, ok := under(root, file); ok {
			rel = filepath.ToSlash(value)
			if value, ok := under(root, dir); ok {
				relDir = filepath.ToSlash(value)
			}
			break
		}
	}
	if rel == "" || relDir == "" || !env.corpus[rel] {
		return UnitSpec{}, false
	}
	ref, ok := env.repository.ID(rel)
	if !ok {
		return UnitSpec{}, false
	}
	return UnitSpec{Path: rel, FileRef: string(ref), Dir: relDir, Source: source, Args: kept, Dropped: dropped, Built: true}, true
}

var (
	makeDirectory = regexp.MustCompile("^\\S*make(?:\\[\\d+\\])?: (Entering|Leaving) directory [`'\"](.*)['\"]\\s*$")
	compilerName  = regexp.MustCompile(`^(?:[\w.+]+-)*(?:cc|gcc|clang|c89|c99|tcc|icc|icx|xlc)(?:-[0-9][0-9.]*)?$`)
	archiverName  = regexp.MustCompile(`^(?:[\w.+]+-)*(?:llvm-|gcc-)?ar$`)
	variableWord  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	wrappers      = map[string]bool{"ccache": true, "distcc": true, "sccache": true, "icecc": true, "env": true, "exec": true, "time": true, "nice": true}
)

// parseDryRun reads the compile, link and archive commands `make -n -B -w`
// printed, following the directories make reports and `cd` in a recipe.
func parseDryRun(env parseEnv, output string) buildDescription {
	description := buildDescription{archives: map[string][]string{}}
	dirs := []string{env.root}
	var logical []string
	pending := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.HasSuffix(line, "\\") {
			pending += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		logical = append(logical, pending+line)
		pending = ""
	}
	if pending != "" {
		logical = append(logical, pending)
	}
	synthetic := 0
	for _, line := range logical {
		if match := makeDirectory.FindStringSubmatch(line); match != nil {
			if match[1] == "Entering" {
				dirs = append(dirs, filepath.Clean(match[2]))
			} else if len(dirs) > 1 {
				dirs = dirs[:len(dirs)-1]
			}
			continue
		}
		commands, ok := shellCommands(line)
		if !ok {
			continue
		}
		cwd := dirs[len(dirs)-1]
		for _, words := range commands {
			words = commandWords(words)
			if len(words) == 0 {
				continue
			}
			program := filepath.Base(words[0])
			switch {
			case program == "cd":
				target := os.Getenv("HOME")
				if len(words) > 1 {
					target = words[1]
				}
				if !filepath.IsAbs(target) {
					target = filepath.Join(cwd, target)
				}
				cwd = filepath.Clean(target)
			case archiverName.MatchString(program):
				if len(words) >= 4 && !strings.HasPrefix(words[1], "-") && strings.ContainsAny(words[1], "rq") {
					archive := absolute(cwd, words[2])
					for _, member := range words[3:] {
						if strings.HasSuffix(member, ".o") {
							description.archives[archive] = append(description.archives[archive], absolute(cwd, member))
						}
					}
				}
			case compilerName.MatchString(program):
				args := words[1:]
				kept, dropped, inputs, out, compileOnly, preprocessOnly := compileFlags(args)
				if preprocessOnly && !compileOnly {
					continue
				}
				var sources []string
				for _, input := range inputs {
					if path.Ext(input) == ".c" {
						sources = append(sources, input)
					}
				}
				if compileOnly {
					for _, source := range sources {
						object := strings.TrimSuffix(filepath.Base(source), ".c") + ".o"
						if out != "" && len(sources) == 1 {
							object = out
						}
						if spec, ok := unitSpec(env, cwd, source, kept, dropped); ok {
							description.compiles = append(description.compiles, compileRecord{spec: spec, object: absolute(cwd, object)})
						}
					}
					continue
				}
				if out == "" {
					out = "a.out"
				}
				link := linkRecord{written: out, output: absolute(cwd, out), dir: cwd, shared: slices.Contains(args, "-shared") || slices.Contains(args, "-dynamiclib")}
				for _, input := range inputs {
					if path.Ext(input) == ".c" {
						synthetic++
						key := fmt.Sprintf("%s#%d", link.output, synthetic)
						if spec, ok := unitSpec(env, cwd, input, kept, dropped); ok {
							description.compiles = append(description.compiles, compileRecord{spec: spec, object: key})
							link.inputs = append(link.inputs, key)
						}
						continue
					}
					switch path.Ext(input) {
					case ".o", ".a", ".so", ".dylib", ".lo":
						link.inputs = append(link.inputs, absolute(cwd, input))
					}
				}
				link.args = linkArgs(args)
				if strings.HasSuffix(out, ".o") {
					// A partial link combines objects like an archive.
					description.archives[link.output] = append(description.archives[link.output], link.inputs...)
					continue
				}
				description.links = append(description.links, link)
			}
		}
	}
	return description
}

func absolute(dir, file string) string {
	if filepath.IsAbs(file) {
		return filepath.Clean(file)
	}
	return filepath.Join(dir, file)
}

// commandWords removes what runs the compiler rather than being it: make's
// recipe prefixes, variable assignments and wrappers such as ccache.
func commandWords(words []string) []string {
	for len(words) > 0 {
		first := strings.TrimLeft(words[0], "@-+")
		switch {
		case first == "":
			words = words[1:]
		case variableWord.MatchString(first):
			words = words[1:]
		case wrappers[filepath.Base(first)]:
			words = words[1:]
		default:
			words[0] = first
			return words
		}
	}
	return nil
}

// linkArgs keeps the libraries and link flags of a link line.
func linkArgs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-l" || arg == "-L" || arg == "-framework":
			if i+1 < len(args) {
				out = append(out, arg, args[i+1])
				i++
			}
		case strings.HasPrefix(arg, "-l"), strings.HasPrefix(arg, "-L"), strings.HasPrefix(arg, "-Wl,"),
			arg == "-pthread", arg == "-shared", arg == "-dynamiclib", arg == "-static", arg == "-rdynamic":
			out = append(out, arg)
		}
	}
	return out
}

// shellCommands splits one recipe line into its commands' words. Quotes and
// backslashes are honoured; && || ; | & ( ) separate commands and a
// redirection's target is dropped. It reports false for a line it cannot
// split.
func shellCommands(line string) ([][]string, bool) {
	var commands [][]string
	var words []string
	var word strings.Builder
	inWord, dropNext := false, false
	flush := func() {
		if inWord {
			if dropNext {
				dropNext = false
			} else {
				words = append(words, word.String())
			}
		}
		word.Reset()
		inWord = false
	}
	end := func() {
		flush()
		if len(words) > 0 {
			commands = append(commands, words)
		}
		words = nil
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch c {
		case ' ', '\t':
			flush()
		case '\\':
			if i+1 < len(line) {
				i++
				word.WriteByte(line[i])
				inWord = true
			}
		case '\'':
			j := strings.IndexByte(line[i+1:], '\'')
			if j < 0 {
				return nil, false
			}
			word.WriteString(line[i+1 : i+1+j])
			inWord = true
			i += j + 1
		case '"':
			i++
			for ; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) && strings.IndexByte("\"\\$`", line[i+1]) >= 0 {
					i++
				}
				word.WriteByte(line[i])
			}
			if i >= len(line) {
				return nil, false
			}
			inWord = true
		case ';', '&', '|', '(', ')':
			end()
		case '>', '<':
			// 2>&1 and >file: the descriptor digits belong to the operator.
			if inWord && strings.Trim(word.String(), "0123456789") == "" {
				word.Reset()
				inWord = false
			}
			flush()
			for i+1 < len(line) && (line[i+1] == '>' || line[i+1] == '&') {
				i++
			}
			if i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9' && line[i] == '&' {
				i++
				continue
			}
			dropNext = true
		default:
			word.WriteByte(c)
			inWord = true
		}
	}
	end()
	return commands, true
}

// linkPrograms turns the build's link lines into programs and reports which
// units they link.
func linkPrograms(env parseEnv, description buildDescription) ([]Program, map[string]bool, []Observation) {
	objects := map[string]UnitSpec{}
	for _, record := range description.compiles {
		objects[record.object] = record.spec
	}
	var programs []Program
	var observations []Observation
	linked := map[string]bool{}
	for _, link := range description.links {
		var specs []UnitSpec
		var missing []string
		seen := map[string]bool{}
		var add func(input string, depth int)
		add = func(input string, depth int) {
			if spec, ok := objects[input]; ok {
				if key := unitKey(spec); !seen[key] {
					seen[key] = true
					specs = append(specs, spec)
				}
				return
			}
			if members, ok := description.archives[input]; ok && depth < 8 {
				for _, member := range members {
					add(member, depth+1)
				}
				return
			}
			missing = append(missing, relativeTo(env.roots, input))
		}
		for _, input := range link.inputs {
			add(input, 0)
		}
		name := relativeTo(env.roots, link.output)
		if len(specs) == 0 {
			observations = append(observations, Observation{Kind: "c_link_without_units", Path: name, Values: missing})
			continue
		}
		sortUnits(specs)
		var paths []string
		for _, spec := range specs {
			linked[unitKey(spec)] = true
			paths = append(paths, spec.Path)
		}
		anchor := Site{Path: specs[0].Path}
		if makefile := dirMakefile(env, link.dir); makefile != "" {
			anchor = Site{Path: makefile, Line: ruleLine(env, makefile, link.written)}
		}
		kind := ProgramExecutable
		if link.shared {
			kind = ProgramShared
		}
		programs = append(programs, Program{Selector: "c:" + name, Name: name, Kind: kind, Units: specs, Anchor: anchor, LinkArgs: link.args, Missing: missing,
			Evidence: []Observation{{Kind: "c_link", Path: anchor.Path, Line: anchor.Line, Fields: map[string]string{"output": name}, Values: paths}}})
	}
	return programs, linked, observations
}

func relativeTo(roots []string, file string) string {
	for _, root := range roots {
		if rel, ok := under(root, file); ok {
			return filepath.ToSlash(rel)
		}
	}
	if rel, err := filepath.Rel(roots[0], file); err == nil {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}

// dirMakefile is the corpus makefile make reads in dir, in make's order.
func dirMakefile(env parseEnv, dir string) string {
	rel := relativeTo(env.roots, dir)
	for _, name := range []string{"GNUmakefile", "makefile", "Makefile"} {
		candidate := path.Join(rel, name)
		if env.corpus[candidate] {
			return candidate
		}
	}
	return ""
}

// ruleLine is the line of the rule whose targets name the link output as
// written, or 0 when the rule names it through a variable.
func ruleLine(env parseEnv, makefile, output string) int {
	id, ok := env.repository.ID(makefile)
	if !ok {
		return 0
	}
	content, err := env.repository.ReadFileAll(id)
	if err != nil {
		return 0
	}
	rule := regexp.MustCompile(`^(?:[^:#=\t][^:#=]*\s)?` + regexp.QuoteMeta(output) + `(?:\s[^:#=]*)?::?(?:[^=]|$)`)
	for number, line := range strings.Split(string(content.Bytes), "\n") {
		if rule.MatchString(line) {
			return number + 1
		}
	}
	return 0
}
