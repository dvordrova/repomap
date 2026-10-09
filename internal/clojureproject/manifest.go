package clojureproject

import (
	"path"
	"strings"
)

// A build description as the repository writes it: deps.edn's paths and
// aliases, and shadow-cljs.edn's builds. The reader splits EDN forms with
// their positions only; it evaluates nothing, and a value it does not read
// (a reader tag, a computed alias) is simply not a row.

// Alias is one deps.edn alias: what `clj -M:name` adds and runs.
type Alias struct {
	Name       string
	Line       int
	MainOpts   []string
	ExecFn     string
	ExtraPaths []string
	ExtraDeps  []string
}

// Entry is a function or namespace a shadow-cljs build starts from: a
// module's :init-fn or :entries, a build's :entries, or a node script's :main.
type Entry struct {
	Key    string
	Symbol string
	Line   int
}

// ShadowBuild is one build of a shadow-cljs.edn.
type ShadowBuild struct {
	ID, Target, OutputDir string
	Line                  int
	Entries               []Entry
}

// Manifest is what one deps.edn, project.clj or shadow-cljs.edn says.
type Manifest struct {
	Paths        []string
	PathsLine    int
	Dependencies []Row
	Aliases      []Alias
	// TestPaths are a Leiningen project's test directories.
	TestPaths []string
	// shadow-cljs.edn
	SourcePaths     []string
	SourcePathsLine int
	DepsAliases     []string
	DepsLine        int
	DevHTTP         []Row
	Builds          []ShadowBuild
}

// Row is one quoted manifest value with its line.
type Row struct {
	Key, Value string
	Line       int
}

type ednNode struct {
	// kind is '{', '[', '(', '#' (a set), ':' (a keyword), '"' (a string)
	// or 'y' (anything else written as one token: a symbol, a number).
	kind     rune
	text     string
	line     int
	children []ednNode
}

func readEDN(data []byte) []ednNode {
	s := newSource(data)
	nodes, _ := forms(s.text, 0, 0)
	result := make([]ednNode, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, s.ednNode(node))
	}
	return result
}

func (s source) ednNode(node form) ednNode {
	text := string(s.text[node.start:node.end])
	result := ednNode{kind: 'y', text: text, line: s.location("", node.start).Line}
	if text == "" {
		return result
	}
	switch first := []rune(text)[0]; {
	case first == '{' || first == '[' || first == '(':
		result.kind = first
	case strings.HasPrefix(text, "#{"):
		result.kind = '#'
	case first == '"':
		if value, err := clojureString(text); err == nil {
			result.kind, result.text = '"', value
		}
		return result
	case first == ':' && !strings.HasPrefix(text, "::"):
		result.kind, result.text = ':', text[1:]
		return result
	default:
		return result
	}
	for _, child := range node.children {
		result.children = append(result.children, s.ednNode(child))
	}
	return result
}

// entry is a map's value under a keyword key.
func (n ednNode) entry(key string) (ednNode, bool) {
	if n.kind != '{' {
		return ednNode{}, false
	}
	for i := 0; i+1 < len(n.children); i += 2 {
		if k := n.children[i]; k.kind == ':' && k.text == key {
			return n.children[i+1], true
		}
	}
	return ednNode{}, false
}

// pairs are a map's entries in the order written.
func (n ednNode) pairs() [][2]ednNode {
	if n.kind != '{' {
		return nil
	}
	var result [][2]ednNode
	for i := 0; i+1 < len(n.children); i += 2 {
		result = append(result, [2]ednNode{n.children[i], n.children[i+1]})
	}
	return result
}

// words are a vector's or list's strings, keywords and symbols as written.
func (n ednNode) words() []string {
	if n.kind != '[' && n.kind != '(' {
		return nil
	}
	var result []string
	for _, child := range n.children {
		if child.kind == '"' || child.kind == ':' || child.kind == 'y' {
			result = append(result, child.text)
		}
	}
	return result
}

// ReadManifest reads a deps.edn, project.clj or shadow-cljs.edn by its name.
func ReadManifest(name string, data []byte) Manifest {
	var manifest Manifest
	nodes := readEDN(data)
	switch name {
	case "deps.edn":
		if len(nodes) > 0 {
			manifest.readDeps(nodes[0])
		}
	case "shadow-cljs.edn":
		if len(nodes) > 0 {
			manifest.readShadow(nodes[0])
		}
	case "project.clj":
		for _, node := range nodes {
			if node.kind == '(' && len(node.children) >= 3 && node.children[0].text == "defproject" {
				manifest.readProject(node.children[3:])
			}
		}
	}
	return manifest
}

func (m *Manifest) readDeps(root ednNode) {
	if paths, ok := root.entry("paths"); ok {
		m.Paths, m.PathsLine = paths.words(), paths.line
	}
	if deps, ok := root.entry("deps"); ok {
		m.Dependencies = dependencyRows(deps)
	}
	aliases, ok := root.entry("aliases")
	if !ok {
		return
	}
	for _, pair := range aliases.pairs() {
		if pair[0].kind != ':' || pair[1].kind != '{' {
			continue
		}
		alias := Alias{Name: pair[0].text, Line: pair[0].line}
		if opts, ok := pair[1].entry("main-opts"); ok {
			alias.MainOpts = opts.words()
		}
		if fn, ok := pair[1].entry("exec-fn"); ok && fn.kind == 'y' {
			alias.ExecFn = fn.text
		}
		if paths, ok := pair[1].entry("extra-paths"); ok {
			alias.ExtraPaths = paths.words()
		}
		if deps, ok := pair[1].entry("extra-deps"); ok {
			for _, dep := range deps.pairs() {
				alias.ExtraDeps = append(alias.ExtraDeps, dep[0].text)
			}
		}
		m.Aliases = append(m.Aliases, alias)
	}
}

// dependencyRows are the coordinates a :deps map pins with a Maven version.
func dependencyRows(deps ednNode) []Row {
	var rows []Row
	for _, dep := range deps.pairs() {
		if version, ok := dep[1].entry("mvn/version"); ok && version.kind == '"' {
			rows = append(rows, Row{Key: "dependency." + dep[0].text, Value: version.text, Line: dep[0].line})
		}
	}
	return rows
}

func (m *Manifest) readShadow(root ednNode) {
	if paths, ok := root.entry("source-paths"); ok {
		m.SourcePaths, m.SourcePathsLine = paths.words(), paths.line
	}
	if deps, ok := root.entry("deps"); ok {
		m.DepsLine = deps.line
		if aliases, ok := deps.entry("aliases"); ok {
			m.DepsAliases = aliases.words()
		} else if deps.kind == 'y' && deps.text == "true" {
			m.DepsAliases = []string{}
		}
	}
	if http, ok := root.entry("dev-http"); ok {
		for _, pair := range http.pairs() {
			value := pair[1].text
			if pair[1].kind == '{' {
				value = ""
				if served, ok := pair[1].entry("root"); ok {
					value = served.text
				}
			} else if pair[1].kind == '[' {
				value = strings.Join(pair[1].words(), ", ")
			}
			m.DevHTTP = append(m.DevHTTP, Row{Key: "dev-http." + pair[0].text, Value: value, Line: pair[0].line})
		}
	}
	builds, ok := root.entry("builds")
	if !ok {
		return
	}
	for _, pair := range builds.pairs() {
		if pair[0].kind != ':' || pair[1].kind != '{' {
			continue
		}
		build := ShadowBuild{ID: pair[0].text, Line: pair[0].line}
		if target, ok := pair[1].entry("target"); ok {
			build.Target = strings.TrimPrefix(target.text, ":")
		}
		if dir, ok := pair[1].entry("output-dir"); ok && dir.kind == '"' {
			build.OutputDir = dir.text
		}
		if main, ok := pair[1].entry("main"); ok && main.kind == 'y' {
			build.Entries = append(build.Entries, Entry{Key: "main", Symbol: main.text, Line: main.line})
		}
		if entries, ok := pair[1].entry("entries"); ok && entries.kind == '[' {
			for _, ns := range entries.children {
				if ns.kind == 'y' {
					build.Entries = append(build.Entries, Entry{Key: "entries", Symbol: ns.text, Line: ns.line})
				}
			}
		}
		if modules, ok := pair[1].entry("modules"); ok {
			for _, module := range modules.pairs() {
				if fn, ok := module[1].entry("init-fn"); ok && fn.kind == 'y' {
					build.Entries = append(build.Entries, Entry{Key: "modules." + module[0].text + ".init-fn", Symbol: fn.text, Line: fn.line})
				}
				if entries, ok := module[1].entry("entries"); ok {
					for _, ns := range entries.children {
						if ns.kind == 'y' {
							build.Entries = append(build.Entries, Entry{Key: "modules." + module[0].text + ".entries", Symbol: ns.text, Line: ns.line})
						}
					}
				}
			}
		}
		m.Builds = append(m.Builds, build)
	}
}

// readProject reads a defproject's options: the keyword/value pairs after
// its name and version.
func (m *Manifest) readProject(options []ednNode) {
	for i := 0; i+1 < len(options); i += 2 {
		key, value := options[i], options[i+1]
		if key.kind != ':' {
			return
		}
		switch key.text {
		case "source-paths":
			m.Paths, m.PathsLine = value.words(), key.line
		case "test-paths":
			m.TestPaths = value.words()
			if m.TestPaths == nil {
				m.TestPaths = []string{}
			}
		case "main":
			if value.kind == 'y' {
				m.Aliases = append(m.Aliases, Alias{Name: "main", Line: key.line, MainOpts: []string{"-m", value.text}})
			}
		}
	}
}

// Rows are the run-relevant values a newcomer reads in the manifest: its
// paths and pinned dependencies, each alias's main options, exec function,
// extra paths and extra dependencies, and each shadow-cljs build's target,
// output and entries, with the line that writes each. Every alias is
// quoted, as every package.json script is: which one runs which program,
// the entry facts beside them say.
func (m Manifest) Rows(name string) []Row {
	var rows []Row
	if len(m.Paths) > 0 {
		rows = append(rows, Row{Key: "paths", Value: strings.Join(m.Paths, ", "), Line: m.PathsLine})
	}
	rows = append(rows, m.Dependencies...)
	for _, alias := range m.Aliases {
		prefix := "aliases." + alias.Name + "."
		if name == "project.clj" {
			prefix = ""
			if alias.Name == "main" && len(alias.MainOpts) == 2 {
				rows = append(rows, Row{Key: "main", Value: alias.MainOpts[1], Line: alias.Line})
			}
			continue
		}
		if len(alias.MainOpts) > 0 {
			rows = append(rows, Row{Key: prefix + "main-opts", Value: strings.Join(alias.MainOpts, " "), Line: alias.Line})
		}
		if alias.ExecFn != "" {
			rows = append(rows, Row{Key: prefix + "exec-fn", Value: alias.ExecFn, Line: alias.Line})
		}
		if len(alias.ExtraPaths) > 0 {
			rows = append(rows, Row{Key: prefix + "extra-paths", Value: strings.Join(alias.ExtraPaths, ", "), Line: alias.Line})
		}
		if len(alias.ExtraDeps) > 0 {
			rows = append(rows, Row{Key: prefix + "extra-deps", Value: strings.Join(alias.ExtraDeps, ", "), Line: alias.Line})
		}
	}
	if len(m.SourcePaths) > 0 {
		rows = append(rows, Row{Key: "source-paths", Value: strings.Join(m.SourcePaths, ", "), Line: m.SourcePathsLine})
	}
	if m.DepsAliases != nil {
		value := "true"
		if len(m.DepsAliases) > 0 {
			value = strings.Join(m.DepsAliases, ", ")
		}
		rows = append(rows, Row{Key: "deps.aliases", Value: value, Line: m.DepsLine})
	}
	rows = append(rows, m.DevHTTP...)
	for _, build := range m.Builds {
		prefix := "builds." + build.ID + "."
		if build.Target != "" {
			rows = append(rows, Row{Key: prefix + "target", Value: build.Target, Line: build.Line})
		}
		if build.OutputDir != "" {
			rows = append(rows, Row{Key: prefix + "output-dir", Value: build.OutputDir, Line: build.Line})
		}
		for _, entry := range build.Entries {
			rows = append(rows, Row{Key: prefix + entry.Key, Value: entry.Symbol, Line: entry.Line})
		}
	}
	return rows
}

// testRunners are the namespaces that run the test frameworks this adapter
// names (clojure.test, speclj) from the command line: an alias whose main
// options or exec function run one names the directories it adds as tests.
func testRunner(namespace string) bool {
	switch namespace {
	case "speclj.main", "cognitect.test-runner", "cognitect.test-runner.api", "kaocha.runner":
		return true
	}
	return false
}

// TestDirectories are the directories the build description runs as
// tests, relative to the repository: a deps.edn alias that runs a test
// runner (`:main-opts ["-m" "speclj.main"]`, an `:exec-fn` in
// cognitect.test-runner.api) names its `:extra-paths`, and a Leiningen
// project its `:test-paths` (Leiningen's own default `test` when it writes
// none). An alias's extra paths alone name no tests: :dev adds tooling.
func (m Manifest) TestDirectories(name, dir string) []string {
	var dirs []string
	add := func(paths []string) {
		for _, p := range paths {
			p = strings.TrimSuffix(path.Clean(p), "/")
			if p == "." || p == "" || strings.HasPrefix(p, "..") || path.IsAbs(p) {
				continue
			}
			dirs = append(dirs, path.Join(dir, p))
		}
	}
	switch name {
	case "project.clj":
		if m.TestPaths == nil {
			add([]string{"test"})
		} else {
			add(m.TestPaths)
		}
	case "deps.edn":
		for _, alias := range m.Aliases {
			runs := false
			for i := 0; i+1 < len(alias.MainOpts); i++ {
				if (alias.MainOpts[i] == "-m" || alias.MainOpts[i] == "--main") && testRunner(alias.MainOpts[i+1]) {
					runs = true
				}
			}
			if ns, _, ok := strings.Cut(alias.ExecFn, "/"); ok && testRunner(ns) {
				runs = true
			}
			if runs {
				add(alias.ExtraPaths)
			}
		}
	}
	return dirs
}
