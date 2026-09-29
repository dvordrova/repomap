package gofacts

import (
	"bytes"
	"context"
	"fmt"
	"path"
	"runtime"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/gotarget"
	"github.com/dvordrova/repomap/internal/reporead"
	"go.yaml.in/yaml/v3"
)

// A main package can build only with build tags: litestream's
// cmd/litestream-vfs has no file without `vfs` or SQLITE3VFS_LOADABLE_EXT,
// and its Makefile builds it with `go build -tags vfs,SQLITE3VFS_LOADABLE_EXT
// ... ./cmd/litestream-vfs`. The run's own build selection sees no program
// there. The repository's build descriptions say which tags build it: a
// make recipe or a goreleaser build naming the package. That is a code fact
// with a source line, not a guess, and the package is analysed with those
// tags. Shell scripts are not read: a script runs wherever it is started, so
// its relative package paths name no exact directory.

// BuildTagSource is one line of a build description that builds a package
// with -tags.
type BuildTagSource struct {
	Path string `json:"path"`
	Line int    `json:"line"`
}

// TaggedBuild is one module loaded again for Platform with the run's tags
// and Tags, the tags build descriptions give Packages, main packages that
// build only with them. Facts holds that load; it has no tagged builds of
// its own.
type TaggedBuild struct {
	ModuleDir string          `json:"module_dir"`
	Tags      []string        `json:"tags"`
	Platform  string          `json:"platform"`
	Packages  []TaggedPackage `json:"packages"`
	Facts     *Facts          `json:"facts"`
}

// TaggedPackage is one main package a tagged build makes a program, with
// every description line that builds it with those tags for that platform.
type TaggedPackage struct {
	PackageDir string           `json:"package_dir"`
	Sources    []BuildTagSource `json:"sources"`
}

// describedBuild is one package directory a build description builds with
// tags, and the GOOS and GOARCH the line sets ("" when it sets none).
type describedBuild struct {
	packageDir string
	tags       []string
	goos       string
	goarch     string
	source     BuildTagSource
}

// buildCommand is one `go build` or `go install` with -tags, as written: the
// directory it runs in, the GOOS and GOARCH it sets and its package
// arguments.
type buildCommand struct {
	cwd      string
	tags     []string
	goos     string
	goarch   string
	packages []string
	line     int
}

// loadTaggedBuilds loads each module again with the tags its build
// descriptions give packages that are no program under the run's own tags,
// for the platform each line builds (the run's where the line sets none),
// and keeps the packages that load makes programs. One platform is loaded
// per package and tags, in order: the run's, the host's, then the other
// lines' as written, the first that makes it a program. A package that two
// different tag sets both make a program is ambiguous: it is recorded and
// analysed with neither. cgo is the go command's own default for the
// platform, as in the typed load that follows.
func loadTaggedBuilds(
	ctx context.Context,
	reader *reporead.Reader,
	repoRoot string,
	fileList []string,
	target gotarget.Target,
	runTags []string,
	facts *Facts,
) ([]TaggedBuild, []string, error) {
	described, warnings := describedBuilds(reader, fileList, facts.Modules)
	programs := make(map[string]bool)
	for _, entrypoint := range facts.EntrypointPackages {
		programs[entrypoint.PackageDir] = true
	}
	host := gotarget.Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	type groupKey struct{ moduleDir, tags, platform string }
	type group struct {
		key     groupKey
		rank    int
		first   BuildTagSource
		sources map[string][]BuildTagSource
	}
	groups := make(map[groupKey]*group)
	for _, build := range described {
		if programs[build.packageDir] {
			continue // it builds without them
		}
		module, ok := owningModule(facts.Modules, build.packageDir)
		if !ok {
			continue
		}
		loadTags, err := gotarget.CanonicalBuildTags(append(append([]string(nil), runTags...), build.tags...))
		if err != nil || strings.Join(loadTags, ",") == strings.Join(runTags, ",") {
			continue // the run's own load already had them
		}
		goos, goarch := target.GOOS, target.GOARCH
		if build.goos != "" {
			goos = build.goos
		}
		if build.goarch != "" {
			goarch = build.goarch
		}
		platform, err := gotarget.FromParts(goos, goarch)
		if err != nil {
			continue
		}
		key := groupKey{moduleDir: module.ModuleDir, tags: strings.Join(build.tags, ","), platform: platform.String()}
		current := groups[key]
		if current == nil {
			rank := 2
			switch platform {
			case target:
				rank = 0
			case host:
				rank = 1
			}
			current = &group{key: key, rank: rank, first: build.source, sources: make(map[string][]BuildTagSource)}
			groups[key] = current
		}
		current.sources[build.packageDir] = append(current.sources[build.packageDir], build.source)
	}
	ordered := make([]*group, 0, len(groups))
	for _, value := range groups {
		ordered = append(ordered, value)
	}
	sort.Slice(ordered, func(i, j int) bool {
		left, right := ordered[i], ordered[j]
		if left.rank != right.rank {
			return left.rank < right.rank
		}
		if left.first != right.first {
			return left.first.Path < right.first.Path || left.first.Path == right.first.Path && left.first.Line < right.first.Line
		}
		return left.key.platform < right.key.platform
	})
	var builds []TaggedBuild
	resolved := make(map[string]bool)
	madeBy := make(map[string][]string)
	for _, current := range ordered {
		key := current.key
		var pending []string
		for dir := range current.sources {
			if !resolved[dir+"\x00"+key.tags] {
				pending = append(pending, dir)
			}
		}
		if len(pending) == 0 {
			continue
		}
		sort.Strings(pending)
		tags := strings.Split(key.tags, ",")
		loadTags, _ := gotarget.CanonicalBuildTags(append(append([]string(nil), runTags...), tags...))
		platform, _ := gotarget.Parse(key.platform)
		loaded, err := loadModules(ctx, reader, repoRoot, fileList, platform, loadTags, []string{key.moduleDir})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, nil, ctxErr
			}
			warnings = append(warnings, fmt.Sprintf("module %s with -tags %s for %s: not loaded: %v", key.moduleDir, key.tags, key.platform, err))
			continue
		}
		loadedPrograms := make(map[string]bool)
		for _, entrypoint := range loaded.EntrypointPackages {
			loadedPrograms[entrypoint.PackageDir] = true
		}
		build := TaggedBuild{ModuleDir: key.moduleDir, Tags: tags, Platform: key.platform, Facts: loaded}
		for _, dir := range pending {
			if !loadedPrograms[dir] {
				continue
			}
			resolved[dir+"\x00"+key.tags] = true
			build.Packages = append(build.Packages, TaggedPackage{PackageDir: dir, Sources: canonicalBuildTagSources(current.sources[dir])})
			madeBy[dir] = append(madeBy[dir], key.tags)
		}
		if len(build.Packages) > 0 {
			builds = append(builds, build)
		}
	}
	for dir, tagSets := range madeBy {
		if len(tagSets) < 2 {
			continue
		}
		sort.Strings(tagSets)
		warnings = append(warnings, fmt.Sprintf(
			"package %s: build descriptions make it a program with different -tags (%s); it is analysed with neither",
			dir, strings.Join(tagSets, "; "),
		))
		for index := range builds {
			builds[index].Packages = removeTaggedPackage(builds[index].Packages, dir)
		}
	}
	kept := builds[:0]
	for _, build := range builds {
		if len(build.Packages) > 0 {
			kept = append(kept, build)
		}
	}
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].ModuleDir != kept[j].ModuleDir {
			return kept[i].ModuleDir < kept[j].ModuleDir
		}
		if joined, other := strings.Join(kept[i].Tags, ","), strings.Join(kept[j].Tags, ","); joined != other {
			return joined < other
		}
		return kept[i].Platform < kept[j].Platform
	})
	return kept, warnings, nil
}

func removeTaggedPackage(values []TaggedPackage, dir string) []TaggedPackage {
	result := values[:0]
	for _, value := range values {
		if value.PackageDir != dir {
			result = append(result, value)
		}
	}
	return result
}

func canonicalBuildTagSources(values []BuildTagSource) []BuildTagSource {
	result := append([]BuildTagSource(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		return result[i].Line < result[j].Line
	})
	compacted := result[:0]
	for _, value := range result {
		if len(compacted) == 0 || compacted[len(compacted)-1] != value {
			compacted = append(compacted, value)
		}
	}
	return compacted
}

// owningModule is the module whose directory holds dir most closely.
func owningModule(modules []ModuleFact, dir string) (ModuleFact, bool) {
	var best ModuleFact
	found := false
	for _, module := range modules {
		moduleDir := module.ModuleDir
		if moduleDir != "." && dir != moduleDir && !strings.HasPrefix(dir, moduleDir+"/") {
			continue
		}
		if !found || len(moduleDir) > len(best.ModuleDir) || best.ModuleDir == "." {
			best, found = module, true
		}
	}
	return best, found
}

// describedBuilds reads every build description outside the tooling
// directories: makefiles and goreleaser configurations.
func describedBuilds(reader *reporead.Reader, fileList []string, modules []ModuleFact) ([]describedBuild, []string) {
	var result []describedBuild
	var warnings []string
	files := append([]string(nil), fileList...)
	sort.Strings(files)
	for _, file := range files {
		kind := buildDescriptionKind(file)
		if kind == "" || corpus.ToolingPath(file) {
			continue
		}
		content, err := reader.ReadFileAll(file)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("build description %s: not read: %v", file, err))
			continue
		}
		var commands []buildCommand
		switch kind {
		case "make":
			commands = makefileCommands(path.Dir(file), content.Bytes)
		case "goreleaser":
			commands = goreleaserCommands(path.Dir(file), content.Bytes)
		}
		for _, command := range commands {
			for _, dir := range commandPackageDirs(command, modules) {
				result = append(result, describedBuild{
					packageDir: dir, tags: command.tags, goos: command.goos, goarch: command.goarch,
					source: BuildTagSource{Path: file, Line: command.line},
				})
			}
		}
	}
	return result, warnings
}

func buildDescriptionKind(file string) string {
	base := path.Base(file)
	switch {
	case base == "Makefile" || base == "makefile" || base == "GNUmakefile" || path.Ext(base) == ".mk":
		return "make"
	case base == ".goreleaser.yml" || base == ".goreleaser.yaml" || base == "goreleaser.yml" || base == "goreleaser.yaml":
		return "goreleaser"
	}
	return ""
}

// commandPackageDirs resolves a command's package arguments to repository
// directories: a relative directory from where it runs, the directory of
// the .go files it names, or an import path of one of the modules. A
// pattern (`./...`), a versioned or absolute argument, or an import path of
// no module of the repository names no directory.
func commandPackageDirs(command buildCommand, modules []ModuleFact) []string {
	arguments := command.packages
	if len(arguments) == 0 {
		arguments = []string{"."}
	}
	seen := make(map[string]bool)
	var result []string
	add := func(dir string) {
		dir = path.Clean(dir)
		if dir == ".." || strings.HasPrefix(dir, "../") || seen[dir] {
			return
		}
		seen[dir] = true
		result = append(result, dir)
	}
	for _, argument := range arguments {
		switch {
		case strings.Contains(argument, "...") || strings.Contains(argument, "@") || path.IsAbs(argument):
		case strings.HasSuffix(argument, ".go"):
			add(path.Dir(path.Join(command.cwd, argument)))
		case argument == "." || argument == ".." || strings.HasPrefix(argument, "./") || strings.HasPrefix(argument, "../"):
			add(path.Join(command.cwd, argument))
		default:
			var best ModuleFact
			for _, module := range modules {
				if (argument == module.ModulePath || strings.HasPrefix(argument, module.ModulePath+"/")) &&
					len(module.ModulePath) > len(best.ModulePath) {
					best = module
				}
			}
			if best.ModulePath != "" {
				add(path.Join(best.ModuleDir, strings.TrimPrefix(argument, best.ModulePath)))
			}
		}
	}
	return result
}

// makeUnresolved stands for a make reference the makefile itself does not
// settle: an automatic variable, a function, or a variable it never sets.
const makeUnresolved = "\x00"

// makefileCommands reads each recipe line of a makefile, with the variables
// the makefile sets outside conditionals expanded, as make runs it in the
// makefile's directory.
func makefileCommands(dir string, content []byte) []buildCommand {
	lines := makeLogicalLines(content)
	variables := makeVariables(lines)
	var result []buildCommand
	defining := false
	for _, line := range lines {
		if len(line.pieces) == 0 {
			continue
		}
		if word, _, _ := strings.Cut(strings.TrimSpace(line.pieces[0].text), " "); defining || word == "define" {
			defining = word != "endef"
			continue
		}
		if !strings.HasPrefix(line.pieces[0].text, "\t") {
			continue
		}
		var text strings.Builder
		var starts []int
		for index, piece := range line.pieces {
			value := piece.text
			if index == 0 {
				value = strings.TrimLeft(value, "\t @-+")
			}
			if index > 0 {
				text.WriteByte(' ')
			}
			starts = append(starts, text.Len())
			text.WriteString(variables.expand(value, 0))
		}
		lineAt := func(offset int) int {
			at := line.pieces[0].line
			for index, start := range starts {
				if offset >= start {
					at = line.pieces[index].line
				}
			}
			return at
		}
		result = append(result, shellBuildCommands(text.String(), dir, lineAt)...)
	}
	return result
}

type makePiece struct {
	text string
	line int
}

type makeLine struct{ pieces []makePiece }

// makeLogicalLines joins backslash-continued lines, keeping each physical
// line's number.
func makeLogicalLines(content []byte) []makeLine {
	var result []makeLine
	var current makeLine
	for index, raw := range strings.Split(string(bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))), "\n") {
		continued := strings.HasSuffix(raw, "\\") && !strings.HasSuffix(raw, "\\\\")
		text := strings.TrimSuffix(raw, "\\")
		if len(current.pieces) > 0 {
			text = strings.TrimLeft(text, " \t")
		}
		current.pieces = append(current.pieces, makePiece{text: text, line: index + 1})
		if !continued {
			result = append(result, current)
			current = makeLine{}
		}
	}
	if len(current.pieces) > 0 {
		result = append(result, current)
	}
	return result
}

type makeVariableSet map[string]string

// makeVariables are the values the makefile sets outside conditionals and
// define blocks, in order: =, := and ::= set, ?= sets an unset one, += adds.
// A variable also set inside a conditional, by a define block or by != has
// no value the makefile alone decides.
func makeVariables(lines []makeLine) makeVariableSet {
	values := make(map[string]string)
	undecided := make(map[string]bool)
	depth := 0
	defining := false
	for _, line := range lines {
		if len(line.pieces) == 0 || strings.HasPrefix(line.pieces[0].text, "\t") {
			continue
		}
		var joined []string
		for _, piece := range line.pieces {
			joined = append(joined, piece.text)
		}
		text := strings.TrimSpace(strings.Join(joined, " "))
		word, rest, _ := strings.Cut(text, " ")
		if defining {
			if word == "endef" {
				defining = false
			}
			continue
		}
		switch word {
		case "ifeq", "ifneq", "ifdef", "ifndef":
			depth++
			continue
		case "endif":
			depth--
			continue
		case "else":
			continue
		case "define":
			defining = true
			name := strings.Fields(rest)
			if len(name) > 0 {
				undecided[name[0]] = true
			}
			continue
		}
		name, operator, value, ok := makeAssignment(text)
		if !ok {
			continue
		}
		if depth > 0 || operator == "!=" {
			undecided[name] = true
			continue
		}
		switch operator {
		case "?=":
			if _, set := values[name]; !set {
				values[name] = value
			}
		case "+=":
			if previous, set := values[name]; set && previous != "" {
				values[name] = previous + " " + value
			} else {
				values[name] = value
			}
		default:
			values[name] = value
		}
	}
	for name := range undecided {
		delete(values, name)
	}
	return values
}

func makeAssignment(text string) (name, operator, value string, ok bool) {
	for {
		trimmed := strings.TrimPrefix(strings.TrimPrefix(text, "export "), "override ")
		if trimmed == text {
			break
		}
		text = strings.TrimSpace(trimmed)
	}
	end := 0
	for end < len(text) && (text[end] == '_' || text[end] == '.' || text[end] == '-' ||
		text[end] >= 'a' && text[end] <= 'z' || text[end] >= 'A' && text[end] <= 'Z' || text[end] >= '0' && text[end] <= '9') {
		end++
	}
	if end == 0 {
		return "", "", "", false
	}
	name = text[:end]
	rest := strings.TrimLeft(text[end:], " \t")
	for _, candidate := range []string{":::=", "::=", ":=", "?=", "+=", "!=", "="} {
		if strings.HasPrefix(rest, candidate) {
			value = strings.TrimSpace(rest[len(candidate):])
			if comment := strings.Index(value, "#"); comment >= 0 {
				value = strings.TrimSpace(value[:comment])
			}
			return name, candidate, value, true
		}
	}
	return "", "", "", false
}

// expand replaces $(NAME) and ${NAME} by the makefile's value and $$ by $;
// anything else a $ starts is makeUnresolved.
func (variables makeVariableSet) expand(text string, depth int) string {
	if depth > 16 {
		return makeUnresolved
	}
	var result strings.Builder
	for index := 0; index < len(text); index++ {
		if text[index] != '$' {
			result.WriteByte(text[index])
			continue
		}
		if index+1 >= len(text) {
			result.WriteString(makeUnresolved)
			break
		}
		next := text[index+1]
		switch next {
		case '$':
			result.WriteByte('$')
			index++
		case '(', '{':
			closing := byte(')')
			if next == '{' {
				closing = '}'
			}
			end, nested := index+2, 1
			for ; end < len(text) && nested > 0; end++ {
				switch text[end] {
				case next:
					nested++
				case closing:
					nested--
				}
			}
			reference := text[index+2 : max(index+2, end-1)]
			if value, ok := variables[reference]; ok && nested == 0 {
				result.WriteString(variables.expand(value, depth+1))
			} else {
				result.WriteString(makeUnresolved)
			}
			index = end - 1
		default:
			result.WriteString(makeUnresolved)
			index++
		}
	}
	return result.String()
}

// shellWord is one shell word: its text with quotes removed, where it starts,
// whether nothing in it is expanded at run time, and whether it is a
// command separator.
type shellWord struct {
	text      string
	offset    int
	resolved  bool
	separator bool
}

func shellWords(line string) []shellWord {
	var words []shellWord
	var current strings.Builder
	start, resolved, open := 0, true, false
	flush := func() {
		if open {
			words = append(words, shellWord{text: current.String(), offset: start, resolved: resolved})
		}
		current.Reset()
		resolved, open = true, false
	}
	begin := func(at int) {
		if !open {
			start, open = at, true
		}
	}
	for index := 0; index < len(line); index++ {
		character := line[index]
		switch {
		case character == ' ' || character == '\t':
			flush()
		case character == '#' && !open:
			flush()
			return words
		case strings.IndexByte("&|;()", character) >= 0:
			flush()
			words = append(words, shellWord{text: string(character), offset: index, resolved: true, separator: true})
			if index+1 < len(line) && (character == '&' || character == '|') && line[index+1] == character {
				index++
			}
		case character == '\'':
			begin(index)
			end := strings.IndexByte(line[index+1:], '\'')
			if end < 0 {
				current.WriteString(line[index+1:])
				resolved = false
				index = len(line)
				continue
			}
			current.WriteString(line[index+1 : index+1+end])
			index += end + 1
		case character == '"':
			begin(index)
			index++
			for ; index < len(line) && line[index] != '"'; index++ {
				switch {
				case line[index] == '\\' && index+1 < len(line):
					index++
					current.WriteByte(line[index])
				case line[index] == '$' || line[index] == '`':
					resolved = false
					current.WriteByte(line[index])
				default:
					current.WriteByte(line[index])
				}
			}
			if index >= len(line) {
				resolved = false
			}
		case character == '\\' && index+1 < len(line):
			begin(index)
			index++
			current.WriteByte(line[index])
		default:
			begin(index)
			if character == '$' || character == '`' || character == 0 || character == '*' || character == '?' || character == '~' {
				resolved = false
			}
			current.WriteByte(character)
		}
		if strings.Contains(current.String(), makeUnresolved) {
			resolved = false
		}
	}
	flush()
	return words
}

// shellBuildCommands finds each `go build` or `go install` with -tags in one
// shell line started in dir. A `cd` before it in the same line moves it; a
// `cd` the line does not settle leaves the rest of the line unplaced.
func shellBuildCommands(line, dir string, lineAt func(int) int) []buildCommand {
	var result []buildCommand
	cwd, placed := dir, true
	words := shellWords(line)
	for start := 0; start < len(words); {
		end := start
		for end < len(words) && !words[end].separator {
			end++
		}
		command := words[start:end]
		start = end + 1
		// The line's own GOOS and GOARCH are the platform it builds; one
		// set only at run time leaves the command unplaced.
		environment := map[string]string{}
		platformKnown := true
		assignments := func() {
			for len(command) > 0 && isShellAssignment(command[0].text) {
				name, value, _ := strings.Cut(command[0].text, "=")
				if name == "GOOS" || name == "GOARCH" {
					environment[name] = value
					platformKnown = platformKnown && command[0].resolved
				}
				command = command[1:]
			}
		}
		assignments()
		if len(command) > 0 && command[0].resolved && command[0].text == "env" {
			command = command[1:]
			assignments()
		}
		if len(command) == 0 {
			continue
		}
		switch name := command[0].text; {
		case !command[0].resolved:
		case name == "cd" || name == "pushd" || name == "popd":
			if name == "cd" && len(command) == 2 && command[1].resolved && !path.IsAbs(command[1].text) {
				cwd = path.Join(cwd, command[1].text)
			} else {
				placed = false
			}
		case (name == "go" || strings.HasSuffix(name, "/go")) && placed && platformKnown:
			if built, ok := goBuildCommand(command[1:], cwd); ok {
				built.line = lineAt(command[0].offset)
				built.goos, built.goarch = environment["GOOS"], environment["GOARCH"]
				result = append(result, built)
			}
		}
	}
	return result
}

// redirection reports a word that starts the command's redirections
// (`> build.log`, `2>&1`): no package argument follows it.
func redirection(word string) bool {
	return strings.IndexAny(strings.TrimLeft(word, "0123456789"), "<>") == 0
}

func isShellAssignment(word string) bool {
	name, _, found := strings.Cut(word, "=")
	if !found || name == "" {
		return false
	}
	for index, character := range name {
		if character != '_' && (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') &&
			(index == 0 || character < '0' || character > '9') {
			return false
		}
	}
	return true
}

// goBuildValueFlags are the go build and install flags that take a value.
var goBuildValueFlags = map[string]bool{
	"C": true, "o": true, "p": true, "asmflags": true, "buildmode": true, "compiler": true,
	"covermode": true, "coverpkg": true, "gccgoflags": true, "gcflags": true, "installsuffix": true,
	"ldflags": true, "mod": true, "modfile": true, "overlay": true, "pgo": true, "pkgdir": true,
	"tags": true, "toolexec": true,
}

// goBuildCommand reads the words after `go`: build or install, its flags and
// its package arguments. A flag or package argument expanded only at run
// time leaves the command unread; the value of a flag other than -tags and
// -C does not matter.
func goBuildCommand(words []shellWord, cwd string) (buildCommand, bool) {
	if len(words) == 0 || !words[0].resolved || words[0].text != "build" && words[0].text != "install" {
		return buildCommand{}, false
	}
	command := buildCommand{cwd: cwd}
	tagged := false
	index := 1
	for ; index < len(words); index++ {
		word := words[index]
		if word.text == "--" && word.resolved {
			index++
			break
		}
		if !strings.HasPrefix(word.text, "-") || word.text == "-" {
			break
		}
		if !word.resolved && !strings.Contains(word.text, "=") {
			return buildCommand{}, false
		}
		name, value, inline := strings.Cut(strings.TrimLeft(word.text, "-"), "=")
		valueResolved := word.resolved
		if goBuildValueFlags[name] && !inline {
			if index+1 >= len(words) {
				return buildCommand{}, false
			}
			index++
			value, valueResolved = words[index].text, words[index].resolved
		}
		if !word.resolved && (name == "tags" || name == "C") || name == "" {
			return buildCommand{}, false
		}
		switch name {
		case "tags":
			if !valueResolved {
				return buildCommand{}, false
			}
			tags, err := gotarget.ParseBuildTags(value)
			if err != nil {
				return buildCommand{}, false
			}
			command.tags, tagged = tags, true
		case "C":
			if !valueResolved || path.IsAbs(value) {
				return buildCommand{}, false
			}
			command.cwd = path.Join(command.cwd, value)
		}
	}
	for ; index < len(words); index++ {
		if redirection(words[index].text) {
			break
		}
		if !words[index].resolved {
			return buildCommand{}, false
		}
		command.packages = append(command.packages, words[index].text)
	}
	if !tagged || len(command.tags) == 0 {
		return buildCommand{}, false
	}
	return command, true
}

// goreleaserCommands reads each goreleaser build: its package is main
// (default .) under dir (default .), from the configuration's directory; its
// tags are its tags list or a -tags in its flags. A template, or tags given
// both ways, leaves the build unread.
func goreleaserCommands(dir string, content []byte) []buildCommand {
	var document yaml.Node
	if yaml.Unmarshal(content, &document) != nil || len(document.Content) == 0 {
		return nil
	}
	builds := yamlMapValue(document.Content[0], "builds")
	if builds == nil || builds.Kind != yaml.SequenceNode {
		return nil
	}
	var result []buildCommand
	for _, build := range builds.Content {
		if build.Kind != yaml.MappingNode {
			continue
		}
		if skip := yamlMapValue(build, "skip"); skip != nil && skip.Value != "false" {
			continue
		}
		main, buildDir := ".", "."
		if node := yamlMapValue(build, "main"); node != nil && node.Kind == yaml.ScalarNode {
			main = node.Value
		}
		if node := yamlMapValue(build, "dir"); node != nil && node.Kind == yaml.ScalarNode {
			buildDir = node.Value
		}
		if strings.Contains(main+buildDir, "{{") || path.IsAbs(main) || path.IsAbs(buildDir) {
			continue
		}
		// Each way of giving tags is one reading; two, or one that does not
		// read, leave the build unread.
		var readings []buildCommand
		unread := false
		if node := yamlMapValue(build, "tags"); node != nil {
			values, ok := yamlStrings(node)
			tags, err := gotarget.CanonicalBuildTags(values)
			unread = unread || !ok || err != nil
			readings = append(readings, buildCommand{tags: tags, line: node.Line})
		}
		if node := yamlMapValue(build, "flags"); node != nil {
			items := []*yaml.Node{node}
			if node.Kind == yaml.SequenceNode {
				items = node.Content
			}
			for _, item := range items {
				if item.Kind != yaml.ScalarNode || strings.Contains(item.Value, "{{") {
					unread = true
					continue
				}
				words := shellWords(item.Value)
				for position, word := range words {
					name, value, inline := strings.Cut(strings.TrimLeft(word.text, "-"), "=")
					if name != "tags" || !strings.HasPrefix(word.text, "-") {
						continue
					}
					resolved := word.resolved
					if !inline && position+1 < len(words) {
						value, resolved = words[position+1].text, words[position+1].resolved
					}
					tags, err := gotarget.ParseBuildTags(value)
					unread = unread || !resolved || err != nil
					readings = append(readings, buildCommand{tags: tags, line: item.Line})
				}
			}
		}
		if unread || len(readings) != 1 || len(readings[0].tags) == 0 {
			continue
		}
		tags, line := readings[0].tags, readings[0].line
		// A build lists the platforms it builds; one listing none builds
		// goreleaser's defaults, which are not read here: the run's.
		platforms := [][2]string{{"", ""}}
		goos, goosOK := yamlListed(build, "goos")
		goarch, goarchOK := yamlListed(build, "goarch")
		if !goosOK || !goarchOK {
			continue
		}
		if len(goos) > 0 || len(goarch) > 0 {
			platforms = nil
			for _, system := range append(goos, "")[:max(1, len(goos))] {
				for _, architecture := range append(goarch, "")[:max(1, len(goarch))] {
					platforms = append(platforms, [2]string{system, architecture})
				}
			}
		}
		cwd := path.Join(dir, buildDir)
		for _, platform := range platforms {
			result = append(result, buildCommand{
				cwd: cwd, tags: tags, goos: platform[0], goarch: platform[1],
				packages: []string{"./" + main}, line: line,
			})
		}
	}
	return result
}

// yamlListed is a build's list of plain values under key, if it has one.
func yamlListed(build *yaml.Node, key string) ([]string, bool) {
	node := yamlMapValue(build, key)
	if node == nil {
		return nil, true
	}
	return yamlStrings(node)
}

func yamlMapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func yamlStrings(node *yaml.Node) ([]string, bool) {
	switch node.Kind {
	case yaml.ScalarNode:
		if strings.Contains(node.Value, "{{") {
			return nil, false
		}
		return []string{node.Value}, true
	case yaml.SequenceNode:
		var values []string
		for _, item := range node.Content {
			if item.Kind != yaml.ScalarNode || strings.Contains(item.Value, "{{") {
				return nil, false
			}
			values = append(values, item.Value)
		}
		return values, true
	}
	return nil, false
}
