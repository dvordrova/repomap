package corpus

import (
	"path"
	"slices"
	"strings"
)

// ToolingDirectories hold what surrounds a repository's programs, not the
// programs: agent hooks, CI workflows and actions, editor settings, and test
// inputs. Morfeu's .claude/hooks/obsidian-session-context.py carries a main
// guard, reached the target portfolio as a native target and ended every run
// with WARN "Target not analyzed". A testdata directory holds inputs of the
// tests of the code around it (the Go tool never builds it): repomap's own
// run offered 22 fixture modules as targets beside its one program, and its
// fixture schemas and SQL became the program's data.
var ToolingDirectories = []string{".claude", ".github", ".vscode", "testdata"}

// ToolingPath reports whether a repository-relative file lies in a tooling
// directory at any depth.
func ToolingPath(filePath string) bool {
	for _, segment := range strings.Split(path.Dir(filePath), "/") {
		if slices.Contains(ToolingDirectories, segment) {
			return true
		}
	}
	return false
}
