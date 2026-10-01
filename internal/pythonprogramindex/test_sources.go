package pythonprogramindex

import (
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/pythontarget"
	"github.com/pelletier/go-toml/v2"
)

// pythonTestSources is the pytest selection (pytestSelection) and every
// Python file of a test directory: the topmost directory below a resolved
// pytest table's own directory that holds a selected test module while no
// file under it is a module the project's build declares
// (pythontarget.Target.DeclaresModule) or any program's launch file, in a
// project whose build declares its packages. A distribution ships its
// declared packages; a directory beside them holding its tests is test code
// whole, its helper modules and the data modules its tests load by path
// included (freqtrade's tests/conftest_trades.py and tests/strategy/strats,
// which had drawn "Strategy test fixtures" and "Test fixtures" parts on
// the product map). A directory holding a declared module is never one, so
// a test module inside a package (pandas/tests) is selected alone. A script
// (a `__main__` guard or a shebang, whose build gives it no name) launched
// from such a directory blocks nothing: it is test code, a stub or a runner
// the tests start (beets' test/testall.py and test/rsrc/convert_stub.py had
// been two of its seven programs); a program the build declares (a console
// script, a package's `__main__`) does.
func pythonTestSources(repository *corpus.Corpus, targets []pythontarget.Target) (map[string]bool, error) {
	tests, owners, err := pytestSelection(repository)
	if err != nil || len(tests) == 0 {
		return tests, err
	}
	declaring := map[string]bool{}
	blocked := map[string]bool{}
	block := func(file string) {
		for dir := path.Dir(file); ; dir = path.Dir(dir) {
			blocked[dir] = true
			if dir == "." || dir == "/" {
				return
			}
		}
	}
	for _, target := range targets {
		if len(target.DeclaredPackages) > 0 {
			declaring[target.ProjectDir] = true
		}
		for _, module := range target.Modules {
			if target.DeclaresModule(module) {
				block(module.Path)
			}
		}
		if target.Kind == pythontarget.KindExecutable && !scriptLaunch(target) {
			if info, ok := repository.Info(target.AnchorFileRef); ok {
				block(info.Entry.Path)
			}
		}
	}
	directories := map[string]bool{}
	for test := range tests {
		root := owners[test]
		if !declaring[root] {
			continue
		}
		// The topmost directory strictly below the table's own directory.
		var chain []string
		for dir := path.Dir(test); dir != root && dir != "." && dir != "/"; dir = path.Dir(dir) {
			chain = append(chain, dir)
		}
		for i := len(chain) - 1; i >= 0; i-- {
			if !blocked[chain[i]] {
				directories[chain[i]] = true
				break
			}
		}
	}
	if len(directories) == 0 {
		return tests, nil
	}
	for _, entry := range repository.Entries() {
		if path.Ext(entry.Path) != ".py" || tests[entry.Path] {
			continue
		}
		for dir := path.Dir(entry.Path); dir != "." && dir != "/"; dir = path.Dir(dir) {
			if directories[dir] {
				tests[entry.Path] = true
				break
			}
		}
	}
	return tests, nil
}

// TestSources are a Python project's test sources (pythonTestSources): the
// files pytest's configuration selects, and every file of a test directory
// beside the declared packages. Target discovery leaves an executable
// launched from one out of the programs: a test-only executable is test
// code, not a program a newcomer runs.
func TestSources(repository *corpus.Corpus, targets []pythontarget.Target) (map[string]bool, error) {
	return pythonTestSources(repository, targets)
}

// scriptLaunch reports an executable launched only as a script: by a
// `__main__` guard or a shebang, which no build names.
func scriptLaunch(target pythontarget.Target) bool {
	if len(target.Basis) == 0 {
		return false
	}
	for _, basis := range target.Basis {
		if basis.Kind != pythontarget.BasisNameMainGuard && basis.Kind != pythontarget.BasisPythonShebang {
			return false
		}
	}
	return true
}

// configuredPythonTests records the pytest file-discovery contract, without
// importing tests or executing configuration. An authored pytest table owns its
// explicit patterns; default patterns also require a declared pytest dependency.
// Under a resolved configuration, conftest.py is pytest's own plugin file.
// Unsupported settings
// retain unknown source classification. Configurations are read once per batch.
func configuredPythonTests(repository *corpus.Corpus) (map[string]bool, error) {
	tests, _, err := pytestSelection(repository)
	return tests, err
}

// pytestSelection is configuredPythonTests with, for each selected file,
// the directory of the resolved pytest table owning it.
func pytestSelection(repository *corpus.Corpus) (map[string]bool, map[string]string, error) {
	type config struct {
		root     string
		patterns []string
		resolved bool
		// iniPriority marks a configuration read from an INI file.
		iniPriority int
	}
	var configs []config
	// The directories whose pyproject.toml declares pytest: an INI
	// configuration beside it uses pytest's default patterns too.
	declared := map[string]bool{}
	for _, entry := range repository.Entries() {
		if path.Base(entry.Path) != "pyproject.toml" {
			continue
		}
		content, err := repository.ReadFileAll(entry.ID)
		if err != nil {
			return nil, nil, err
		}
		var document struct {
			Project struct {
				Dependencies         []string            `toml:"dependencies"`
				OptionalDependencies map[string][]string `toml:"optional-dependencies"`
			}
			DependencyGroups map[string][]any `toml:"dependency-groups"`
			Tool             map[string]any
		}
		if toml.Unmarshal(content.Bytes, &document) != nil {
			continue
		}
		dependencies := append([]string{}, document.Project.Dependencies...)
		for _, values := range document.Project.OptionalDependencies {
			dependencies = append(dependencies, values...)
		}
		for _, values := range document.DependencyGroups {
			for _, value := range values {
				if value, ok := value.(string); ok {
					dependencies = append(dependencies, value)
				}
			}
		}
		authority := false
		for _, dependency := range dependencies {
			name := strings.TrimSpace(strings.ToLower(dependency))
			if cut := strings.IndexAny(name, "[<>=!~; @"); cut >= 0 {
				name = name[:cut]
			}
			authority = authority || name == "pytest"
		}
		declared[path.Dir(entry.Path)] = authority
		settings, present := document.Tool["pytest"].(map[string]any)
		if !present {
			continue
		}
		configs = append(configs, config{root: path.Dir(entry.Path)})
		configPosition := len(configs) - 1
		if old, ok := settings["ini_options"].(map[string]any); ok {
			if len(settings) != 1 {
				continue // conflicting native and INI-style tables are unresolved
			}
			settings = old
		}
		// pytest's documented defaults apply only under this exact framework
		// binding. File names on their own never establish a testing role.
		patterns := []string{"test_*.py", "*_test.py"}
		if _, explicit := settings["python_files"]; !explicit && !authority {
			continue
		}
		resolved := true
		if value, exists := settings["python_files"]; exists {
			patterns = nil
			switch value := value.(type) {
			case string:
				patterns = strings.Fields(value)
			case []any:
				for _, item := range value {
					text, ok := item.(string)
					if !ok {
						patterns, resolved = nil, false
						break
					}
					patterns = append(patterns, text)
				}
			default:
				resolved = false
			}
		}
		configs[configPosition].patterns = patterns
		configs[configPosition].resolved = resolved
	}
	// pytest's INI configurations: pytest.ini's [pytest] before a
	// pyproject.toml table, tox.ini's [pytest] and setup.cfg's
	// [tool:pytest] after one (beets configures pytest in setup.cfg).
	byRoot := map[string]int{}
	for position, config := range configs {
		byRoot[config.root] = position
	}
	for _, name := range []string{"pytest.ini", "tox.ini", "setup.cfg"} {
		section := "pytest"
		if name == "setup.cfg" {
			section = "tool:pytest"
		}
		for _, entry := range repository.Entries() {
			if path.Base(entry.Path) != name {
				continue
			}
			root := path.Dir(entry.Path)
			if at, known := byRoot[root]; known && !(name == "pytest.ini" && configs[at].iniPriority == 0) {
				continue
			}
			content, err := repository.ReadFileAll(entry.ID)
			if err != nil {
				return nil, nil, err
			}
			values, present := iniSection(string(content.Bytes), section)
			if !present {
				continue
			}
			authority := declared[root] || name == "setup.cfg" && setupCFGRequiresPytest(string(content.Bytes))
			patterns, explicit := strings.Fields(values["python_files"]), false
			if _, explicit = values["python_files"]; !explicit {
				if !authority {
					continue
				}
				patterns = []string{"test_*.py", "*_test.py"}
			}
			found := config{root: root, patterns: patterns, resolved: true, iniPriority: 1}
			if at, known := byRoot[root]; known {
				configs[at] = found
			} else {
				byRoot[root] = len(configs)
				configs = append(configs, found)
			}
		}
	}
	// The nearest authored configuration owns a file; an unresolved nearer
	// configuration never falls through to a broader parent rule.
	sort.Slice(configs, func(i, j int) bool { return len(configs[i].root) > len(configs[j].root) })
	tests := make(map[string]bool)
	owners := make(map[string]string)
	for _, entry := range repository.Entries() {
		if path.Ext(entry.Path) != ".py" {
			continue
		}
		for _, config := range configs {
			if config.root != "." && !strings.HasPrefix(entry.Path, config.root+"/") {
				continue
			}
			if config.resolved && path.Base(entry.Path) == "conftest.py" {
				tests[entry.Path] = true
			}
			for _, pattern := range config.patterns {
				if strings.Contains(pattern, "/") {
					continue // path-aware custom collectors remain unclassified
				}
				if matched, err := path.Match(pattern, path.Base(entry.Path)); err == nil && matched {
					tests[entry.Path] = true
				}
			}
			if tests[entry.Path] && config.resolved {
				owners[entry.Path] = config.root
			}
			break
		}
	}
	return tests, owners, nil
}

// iniSection is one INI section's keys and values, a value's indented
// continuation lines joined to it by a space; whether the section exists.
func iniSection(text, name string) (map[string]string, bool) {
	values, inside, present, key := map[string]string{}, false, false, ""
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";"):
		case strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]"):
			inside = strings.TrimSpace(trimmed[1:len(trimmed)-1]) == name
			present = present || inside
			key = ""
		case !inside:
		case key != "" && (line[0] == ' ' || line[0] == '\t'):
			values[key] = strings.TrimSpace(values[key] + " " + trimmed)
		default:
			name, value, found := strings.Cut(trimmed, "=")
			if !found {
				name, value, found = strings.Cut(trimmed, ":")
			}
			if found {
				key = strings.TrimSpace(name)
				values[key] = strings.TrimSpace(value)
			}
		}
	}
	return values, present
}

// setupCFGRequiresPytest reports a setup.cfg whose options require pytest:
// install_requires, tests_require or an extra listing it.
func setupCFGRequiresPytest(text string) bool {
	requires := func(value string) bool {
		for _, requirement := range strings.Fields(strings.ReplaceAll(value, ",", " ")) {
			name := strings.ToLower(requirement)
			if cut := strings.IndexAny(name, "[<>=!~;@"); cut >= 0 {
				name = name[:cut]
			}
			if name == "pytest" {
				return true
			}
		}
		return false
	}
	if options, ok := iniSection(text, "options"); ok && (requires(options["install_requires"]) || requires(options["tests_require"])) {
		return true
	}
	extras, _ := iniSection(text, "options.extras_require")
	for _, value := range extras {
		if requires(value) {
			return true
		}
	}
	return false
}
