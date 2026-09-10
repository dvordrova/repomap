package pythonprogramindex

import (
	"path"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/pelletier/go-toml/v2"
)

// configuredPythonTests records the pytest file-discovery contract, without
// importing tests or executing configuration. An authored pytest table owns its
// explicit patterns; default patterns also require a declared pytest dependency.
// Unsupported settings
// retain unknown source classification. Configurations are read once per batch.
func configuredPythonTests(repository *corpus.Corpus) (map[string]bool, error) {
	type config struct {
		root     string
		patterns []string
	}
	var configs []config
	for _, entry := range repository.Entries() {
		if path.Base(entry.Path) != "pyproject.toml" {
			continue
		}
		content, err := repository.ReadFileAll(entry.ID)
		if err != nil {
			return nil, err
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
		if value, exists := settings["python_files"]; exists {
			patterns = nil
			switch value := value.(type) {
			case string:
				patterns = strings.Fields(value)
			case []any:
				for _, item := range value {
					text, ok := item.(string)
					if !ok {
						patterns = nil
						break
					}
					patterns = append(patterns, text)
				}
			}
		}
		configs[configPosition].patterns = patterns
	}
	// The nearest authored configuration owns a file; an unresolved nearer
	// configuration never falls through to a broader parent rule.
	sort.Slice(configs, func(i, j int) bool { return len(configs[i].root) > len(configs[j].root) })
	tests := make(map[string]bool)
	for _, entry := range repository.Entries() {
		if path.Ext(entry.Path) != ".py" {
			continue
		}
		for _, config := range configs {
			if config.root != "." && !strings.HasPrefix(entry.Path, config.root+"/") {
				continue
			}
			for _, pattern := range config.patterns {
				if strings.Contains(pattern, "/") {
					continue // path-aware custom collectors remain unclassified
				}
				if matched, err := path.Match(pattern, path.Base(entry.Path)); err == nil && matched {
					tests[entry.Path] = true
				}
			}
			break
		}
	}
	return tests, nil
}
