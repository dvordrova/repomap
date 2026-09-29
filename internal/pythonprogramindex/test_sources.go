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
// a test module inside a package (pandas/tests) is selected alone.
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
		if target.Kind == pythontarget.KindExecutable {
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
	}
	var configs []config
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
