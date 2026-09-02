package facts

import (
	"fmt"
	"path"
	"strings"
)

// ReadmeMinBytes is the size under which a README is reported as too short
// to orient a newcomer.
const ReadmeMinBytes = 200

// addNegatives states what the repository lacks. Rows are repository-level
// and only exist when a corpus was provided.
func (b *builder) addNegatives() {
	if b.input.Repository == nil {
		return
	}
	paths := b.source.paths()
	b.addReadmeNegative(paths)
	if !hasAny(paths, isTestPath) {
		b.addNegative(NegativeNoTests, "no test files", nil)
	}
	if !hasAny(paths, isDockerPath) {
		b.addNegative(NegativeNoDockerfile, "no Dockerfile or compose file", nil)
	}
	if !hasAny(paths, isCIPath) {
		b.addNegative(NegativeNoCI, "no CI configuration", nil)
	}
}

func (b *builder) addReadmeNegative(paths []string) {
	readme := ""
	for _, filePath := range paths {
		if !strings.Contains(filePath, "/") && strings.HasPrefix(strings.ToUpper(filePath), "README") {
			readme = filePath
			break
		}
	}
	if readme == "" {
		b.addNegative(NegativeNoReadme, "no README at the repository root", &Anchor{Path: "README.md", Line: 1})
		return
	}
	file, ok := b.source.file(readme)
	if ok && file.size < ReadmeMinBytes {
		b.addNegative(NegativeReadmeTooShort, fmt.Sprintf("%s is %d bytes", readme, file.size), &Anchor{Path: readme, Line: 1})
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
