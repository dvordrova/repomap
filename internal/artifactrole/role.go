// Package artifactrole classifies repository artifacts for bounded context
// selection. Roles are deterministic navigation hints, not claims about
// runtime behavior.
package artifactrole

import (
	"path"
	"sort"
	"strings"
)

// Role describes how a repository artifact should compete for a bounded
// context slot.
type Role string

const (
	RolePrimaryProductionEntry  Role = "primary_production_entry"
	RoleProductionCore          Role = "production_core"
	RoleEffectBoundary          Role = "effect_integration_boundary"
	RolePublicAPI               Role = "public_api"
	RoleExample                 Role = "example"
	RoleTest                    Role = "test"
	RoleFixture                 Role = "fixture"
	RoleGenerated               Role = "generated"
	RolePlayground              Role = "playground_preview_evaluator"
	RoleExperimental            Role = "experimental"
	RoleCurrentDocumentation    Role = "current_documentation"
	RoleHistoricalDocumentation Role = "historical_decision_documentation"
)

// Hints carries exact facts already known by a caller. Path classification
// remains the fallback; a hint never overrides an explicit auxiliary path.
type Hints struct {
	PrimaryEntry   bool
	EffectBoundary bool
	PublicAPI      bool
	Documentation  bool
	Generated      bool
	Test           bool
}

// Classify returns one stable primary role for a repository-relative path.
func Classify(filePath string, hints Hints) Role {
	clean := cleanPath(filePath)
	lower := strings.ToLower(clean)
	segments := pathSegments(lower)
	base := path.Base(lower)

	switch {
	case hints.Generated || isGenerated(base, segments):
		return RoleGenerated
	case hasSegment(
		segments,
		"testdata", "fixture", "fixtures", "golden",
		"mock", "mocks", "fake", "fakes", "stub", "stubs", "testutil", "testingutil",
	):
		return RoleFixture
	case hints.Test || isTestPath(base, segments):
		return RoleTest
	case isPlaygroundPath(segments):
		return RolePlayground
	case hasSegment(segments, "example", "examples", "_examples", "sample", "samples", "demo", "demos"):
		return RoleExample
	case hasSegment(segments, "experiment", "experiments", "experimental", "prototype", "prototypes", "lab", "labs"):
		return RoleExperimental
	case hints.Documentation || isDocumentationPath(base):
		if isHistoricalDocumentation(lower, segments, base) {
			return RoleHistoricalDocumentation
		}
		return RoleCurrentDocumentation
	case hints.PrimaryEntry || isConventionalEntry(lower, segments, base):
		return RolePrimaryProductionEntry
	case hints.PublicAPI || isPublicAPIPath(segments):
		return RolePublicAPI
	case hints.EffectBoundary || isEffectBoundaryPath(lower, segments):
		return RoleEffectBoundary
	default:
		return RoleProductionCore
	}
}

// SelectionPriority orders roles inside an existing context budget.
func SelectionPriority(role Role) int {
	switch role {
	case RolePrimaryProductionEntry:
		return 120
	case RoleEffectBoundary:
		return 115
	case RolePublicAPI:
		return 110
	case RoleProductionCore:
		return 100
	case RoleCurrentDocumentation:
		return 90
	case RoleExperimental:
		return 45
	case RoleExample:
		return 35
	case RoleHistoricalDocumentation:
		return 30
	case RolePlayground:
		return 20
	case RoleTest:
		return 15
	case RoleFixture:
		return 10
	case RoleGenerated:
		return 5
	default:
		return 0
	}
}

// IsProduction reports whether a role may occupy a production context slot.
func IsProduction(role Role) bool {
	switch role {
	case RolePrimaryProductionEntry, RoleProductionCore, RoleEffectBoundary, RolePublicAPI:
		return true
	default:
		return false
	}
}

// SortPaths returns a role-aware, production-interleaved copy without mutating
// the caller's path set. Interleaving prevents a large entrypoint directory
// from consuming every bounded source-signal slot before core and effect
// artifacts are inspected.
func SortPaths(paths []string) []string {
	buckets := make(map[Role][]string)
	for _, filePath := range paths {
		role := Classify(filePath, Hints{})
		buckets[role] = append(buckets[role], filePath)
	}
	roles := []Role{
		RolePrimaryProductionEntry,
		RoleEffectBoundary,
		RolePublicAPI,
		RoleProductionCore,
		RoleCurrentDocumentation,
		RoleExperimental,
		RoleExample,
		RoleHistoricalDocumentation,
		RolePlayground,
		RoleTest,
		RoleFixture,
		RoleGenerated,
	}
	for role := range buckets {
		if SelectionPriority(role) == 0 {
			roles = append(roles, role)
		}
		sort.Slice(buckets[role], func(i, j int) bool {
			return LessPath(buckets[role][i], buckets[role][j], role)
		})
	}

	result := make([]string, 0, len(paths))
	for _, roleGroup := range [][]Role{roles[:4], roles[4:]} {
		for index := 0; ; index++ {
			added := false
			for _, role := range roleGroup {
				if index >= len(buckets[role]) {
					continue
				}
				result = append(result, buckets[role][index])
				added = true
			}
			if !added {
				break
			}
		}
	}
	return result
}

// LessPath provides the stable role-local tie break used by bounded
// selectors. Primary entry declarations prefer an exact conventional main;
// otherwise shallower repository paths precede deeper adapters.
func LessPath(left, right string, role Role) bool {
	left = cleanPath(left)
	right = cleanPath(right)
	if role == RolePrimaryProductionEntry {
		leftMain := isExactConventionalMain(left)
		rightMain := isExactConventionalMain(right)
		if leftMain != rightMain {
			return leftMain
		}
	}
	leftDepth := pathDepth(left)
	rightDepth := pathDepth(right)
	if leftDepth != rightDepth {
		return leftDepth < rightDepth
	}
	return left < right
}

func pathDepth(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "/") + 1
}

func isExactConventionalMain(value string) bool {
	switch strings.ToLower(path.Base(value)) {
	case "main.go", "main.py", "__main__.py", "cli.py":
		return true
	default:
		return false
	}
}

func cleanPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(path.Clean("/"+value), "/")
	if value == "." {
		return ""
	}
	return value
}

func pathSegments(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}

func hasSegment(segments []string, values ...string) bool {
	for _, segment := range segments {
		for _, value := range values {
			if segment == value {
				return true
			}
		}
	}
	return false
}

func hasSegmentPart(segments []string, values ...string) bool {
	for _, segment := range segments {
		parts := strings.FieldsFunc(segment, func(char rune) bool {
			return char == '-' || char == '_' || char == '.'
		})
		if hasSegment(parts, values...) {
			return true
		}
	}
	return false
}

func isGenerated(base string, segments []string) bool {
	return strings.HasSuffix(base, ".pb.go") ||
		strings.HasSuffix(base, ".pb.gw.go") ||
		strings.HasPrefix(base, "zz_generated") ||
		hasSegment(segments, "generated", "gen")
}

func isTestPath(base string, segments []string) bool {
	if strings.HasSuffix(base, "_test.go") || strings.HasPrefix(base, "test_") ||
		strings.HasSuffix(base, "_test.py") {
		return true
	}
	for _, segment := range segments {
		if segment == "test" || segment == "tests" || strings.HasSuffix(segment, "-test") ||
			strings.HasSuffix(segment, "_test") {
			return true
		}
	}
	return false
}

func isPlaygroundPath(segments []string) bool {
	return hasSegmentPart(segments,
		"playground", "preview", "evaluator", "evaluation", "eval",
	)
}

func isDocumentationPath(base string) bool {
	switch path.Ext(base) {
	case ".md", ".markdown", ".mdx", ".rst", ".adoc", ".asciidoc", ".drawio":
		return true
	}
	switch base {
	case "readme", "contributing", "changelog", "architecture", "design":
		return true
	default:
		return false
	}
}

func isHistoricalDocumentation(lower string, segments []string, base string) bool {
	if hasSegment(segments, "archive", "archives", "history", "historical", "decisions", "decision", "adr", "adrs") {
		return true
	}
	if strings.Contains(lower, "/agent-room/") && base != "current.md" {
		return true
	}
	stem := strings.TrimSuffix(base, path.Ext(base))
	if strings.HasPrefix(stem, "decision-") || strings.HasPrefix(stem, "adr-") {
		return true
	}
	if len(stem) >= 4 && stem[0] >= '0' && stem[0] <= '9' {
		return true
	}
	return false
}

func isConventionalEntry(lower string, segments []string, base string) bool {
	if base == "main.go" || base == "main.py" || base == "__main__.py" || base == "cli.py" {
		return true
	}
	for index, segment := range segments {
		if segment == "cmd" && index+1 < len(segments) {
			return true
		}
		if segment == "bin" && index+1 < len(segments) {
			return true
		}
	}
	return lower == "main.go" || lower == "main.py"
}

func isPublicAPIPath(segments []string) bool {
	return hasSegment(segments, "api", "public", "sdk", "client")
}

func isEffectBoundaryPath(lower string, segments []string) bool {
	if hasSegment(segments,
		"backend", "backends", "storage", "store", "stores", "database", "db",
		"transport", "network", "output", "outputs", "sink", "sinks", "writer", "writers",
		"persistence", "replica", "replication", "remote",
	) {
		return true
	}
	return hasToken(lower,
		"backend", "database", "persist", "transport", "network", "client", "writer", "output",
	)
}

func hasToken(value string, tokens ...string) bool {
	parts := strings.FieldsFunc(value, func(char rune) bool {
		return char == '/' || char == '\\' || char == '-' || char == '_' || char == '.'
	})
	return hasSegment(parts, tokens...)
}
