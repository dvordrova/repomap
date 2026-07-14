package evidenceref

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/evidence"
)

// Extract returns allowlisted repository references from canonicalized prose.
// A line is present only for explicit path:line syntax; path-only mentions stay
// useful navigation anchors without being promoted to exact source evidence.
func Extract(statement string, allowedPaths []string) []evidence.Location {
	type match struct {
		index    int
		location evidence.Location
	}
	paths := append([]string(nil), allowedPaths...)
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	seen := make(map[string]struct{})
	var matches []match
	for _, path := range paths {
		if path == "" || !strings.Contains(statement, path) {
			continue
		}
		pattern := regexp.MustCompile(`(^|[^A-Za-z0-9_./-])(` + regexp.QuoteMeta(path) + `)(?::([0-9]+))?($|[^A-Za-z0-9_./-])`)
		for _, indexes := range pattern.FindAllStringSubmatchIndex(statement, -1) {
			if len(indexes) < 8 {
				continue
			}
			line := 0
			if indexes[6] >= 0 && indexes[7] >= 0 {
				line, _ = strconv.Atoi(statement[indexes[6]:indexes[7]])
			}
			key := path + ":" + strconv.Itoa(line)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			matches = append(matches, match{
				index:    indexes[4],
				location: evidence.Location{Path: path, Line: line},
			})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].index == matches[j].index {
			if matches[i].location.Path == matches[j].location.Path {
				return matches[i].location.Line < matches[j].location.Line
			}
			return matches[i].location.Path < matches[j].location.Path
		}
		return matches[i].index < matches[j].index
	})
	locations := make([]evidence.Location, 0, len(matches))
	for _, item := range matches {
		locations = append(locations, item.location)
	}
	return locations
}

// Canonicalize rewrites model prose that cites a known source location into
// path:line form. A claimed line absent from the deterministic locations is
// removed instead of becoming a misleading editor link.
func Canonicalize(statement string, allowedPaths []string, locations []evidence.Location) (string, bool) {
	linesByPath := make(map[string]map[int]struct{})
	for _, location := range locations {
		if location.Path == "" || location.Line <= 0 {
			continue
		}
		if linesByPath[location.Path] == nil {
			linesByPath[location.Path] = make(map[int]struct{})
		}
		linesByPath[location.Path][location.Line] = struct{}{}
	}

	paths := append([]string(nil), allowedPaths...)
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	grounded := true
	for _, path := range paths {
		if path == "" || !strings.Contains(statement, path) {
			continue
		}
		quotedPath := regexp.QuoteMeta(path)
		boundary := `(^|[^A-Za-z0-9_./-])`

		immediate := regexp.MustCompile(boundary + `(` + quotedPath + `)\s+lines?\s+([0-9]+)(?:\s*-\s*[0-9]+)?`)
		statement = replaceLineClaim(statement, immediate, path, 3, 0, linesByPath[path], &grounded)

		atLine := regexp.MustCompile(boundary + `(` + quotedPath + `)([^.;\n]{0,160}?)\s+at\s+line\s+([0-9]+)`)
		statement = replaceLineClaim(statement, atLine, path, 4, 3, linesByPath[path], &grounded)

		canonical := regexp.MustCompile(boundary + `(` + quotedPath + `)[:#]([0-9]+)`)
		statement = replaceLineClaim(statement, canonical, path, 3, 0, linesByPath[path], &grounded)
	}
	return statement, grounded
}

func replaceLineClaim(
	statement string,
	pattern *regexp.Regexp,
	path string,
	lineGroup int,
	middleGroup int,
	allowedLines map[int]struct{},
	grounded *bool,
) string {
	return pattern.ReplaceAllStringFunc(statement, func(match string) string {
		parts := pattern.FindStringSubmatch(match)
		if len(parts) <= lineGroup {
			return match
		}
		line, err := strconv.Atoi(parts[lineGroup])
		if err != nil {
			return match
		}

		middle := ""
		if middleGroup > 0 && len(parts) > middleGroup {
			middle = strings.TrimRight(parts[middleGroup], " \t")
		}
		if _, ok := allowedLines[line]; !ok {
			*grounded = false
			return parts[1] + path + middle
		}
		return parts[1] + path + ":" + strconv.Itoa(line) + middle
	})
}
