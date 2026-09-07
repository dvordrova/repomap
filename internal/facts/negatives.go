package facts

import (
	"path"
	"strings"
)

// addNegatives records missing recognized paths within the inspected inventory.
// Readable source entries alone cannot establish that configuration is absent.
func (b *builder) addNegatives() {
	if b.input.Repository == nil {
		return
	}
	paths := b.input.Repository.VisiblePaths()
	if !hasAny(paths, isTestPath) {
		b.addNegative(NegativeNoTests, "no recognized test files found in inspected paths", nil)
	}
	if !hasAny(paths, isDockerPath) {
		b.addNegative(NegativeNoDockerfile, "no Dockerfile or compose file found in inspected paths", nil)
	}
	if !hasAny(paths, isCIPath) {
		b.addNegative(NegativeNoCI, "no recognized CI configuration found in inspected paths", nil)
	}
	if !hasAny(paths, isLicensePath) {
		b.addNegative(NegativeNoLicense, "no LICENSE file found in inspected paths", nil)
	}
	if !hasAny(paths, isContributingPath) {
		b.addNegative(NegativeNoContributing, "no CONTRIBUTING file found in inspected paths", nil)
	}
	if !hasAny(paths, isChangelogPath) {
		b.addNegative(NegativeNoChangelog, "no changelog found in inspected paths", nil)
	}
	if !hasAny(paths, isLinterConfigPath) {
		b.addNegative(NegativeNoLinter, "no recognized linter or formatter configuration found in inspected paths", nil)
	}
}

func (b *builder) addNegative(name, detail string, anchor *Anchor) {
	b.add(".", Fact{Kind: KindNegative, Anchor: anchor, Key: name, Text: detail}, name)
}

func hasAny(paths []string, predicate func(string) bool) bool {
	for _, filePath := range paths {
		if predicate(filePath) {
			return true
		}
	}
	return false
}

func isTestPath(filePath string) bool {
	base := path.Base(filePath)
	switch {
	case strings.HasSuffix(base, "_test.go"), strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"),
		strings.HasSuffix(base, "_test.py"), strings.Contains(base, ".test."), strings.Contains(base, ".spec."):
		return true
	}
	for _, segment := range strings.Split(path.Dir(filePath), "/") {
		if segment == "tests" || segment == "__tests__" {
			return true
		}
	}
	return false
}

func isDockerPath(filePath string) bool {
	base := path.Base(filePath)
	switch {
	case strings.HasPrefix(base, "Dockerfile"):
		return true
	case strings.HasPrefix(base, "docker-compose") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml")):
		return true
	case base == "compose.yml" || base == "compose.yaml":
		return true
	}
	return false
}

func isCIPath(filePath string) bool {
	base := path.Base(filePath)
	switch {
	case strings.HasPrefix(filePath, ".github/workflows/"), strings.HasPrefix(filePath, ".circleci/"):
		return true
	case base == ".gitlab-ci.yml", base == "Jenkinsfile", base == ".travis.yml", base == "azure-pipelines.yml":
		return true
	}
	return false
}

func isLicensePath(filePath string) bool {
	base := strings.ToUpper(path.Base(filePath))
	return path.Dir(filePath) == "." && (strings.HasPrefix(base, "LICENSE") || strings.HasPrefix(base, "COPYING"))
}

func isContributingPath(filePath string) bool {
	base := strings.ToUpper(path.Base(filePath))
	dir := path.Dir(filePath)
	return (dir == "." || dir == ".github" || dir == "docs") && strings.HasPrefix(base, "CONTRIBUTING")
}

func isChangelogPath(filePath string) bool {
	base := strings.ToUpper(path.Base(filePath))
	dir := strings.ToUpper(path.Dir(filePath))
	return dir == "CHANGELOG" || dir == "CHANGES" || dir == "RELEASES" || dir == "HISTORY" ||
		path.Dir(filePath) == "." && (strings.HasPrefix(base, "CHANGELOG") || strings.HasPrefix(base, "CHANGES") ||
			strings.HasPrefix(base, "HISTORY") || strings.HasPrefix(base, "RELEASES"))
}

// isLinterConfigPath is any of the usual style and lint configurations at the
// repository or a nested project, across the languages this tool reads.
func isLinterConfigPath(filePath string) bool {
	base := path.Base(filePath)
	for _, name := range []string{
		".golangci.yml", ".golangci.yaml", ".golangci.toml", ".golangci.json",
		".eslintrc", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.json", ".eslintrc.yml", ".eslintrc.yaml",
		"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts", "biome.json", "biome.jsonc",
		".prettierrc", ".prettierrc.json", ".prettierrc.js", ".prettierrc.cjs", ".prettierrc.yml", ".prettierrc.yaml", "prettier.config.js", "prettier.config.cjs",
		".editorconfig", "ruff.toml", ".ruff.toml", ".flake8", "setup.cfg", "tox.ini", ".pylintrc", "pylintrc", "mypy.ini", ".pre-commit-config.yaml",
	} {
		if base == name {
			return true
		}
	}
	return false
}
