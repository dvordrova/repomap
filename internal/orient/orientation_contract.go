package orient

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
)

var (
	evidenceFilePathPattern = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_./-])((?:/|\./|\.\./)?[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*\.(?:go|md|yaml|yml|json|toml|proto|mod|sum|sh|c|h|rs|py|js|ts))(?:[:#][0-9]+)?`)
	evidenceEscapePattern   = regexp.MustCompile(`(?:^|[\s"'()\[\]{},;=:])((?:/|\.\./)[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*)`)
)

type orientationPart struct {
	ProjectGuess         string                      `json:"project_guess"`
	Confidence           float64                     `json:"confidence"`
	HighLevelMap         []orientationMapItem        `json:"high_level_map"`
	FirstFilesToOpen     fileToOpenList              `json:"first_files_to_open"`
	CandidateFlows       []flowexplain.CandidateFlow `json:"candidate_flows"`
	ImportantDomainWords []orientationDomainWord     `json:"important_domain_words"`
	QuestionsForHuman    []string                    `json:"questions_for_human"`
	UnverifiedPaths      unverifiedPathList          `json:"unverified_paths"`
	Warnings             []string                    `json:"warnings"`
}

type orientationMapItem struct {
	Name         string   `json:"name"`
	Evidence     []string `json:"evidence"`
	WhyItMatters string   `json:"why_it_matters"`
}

type orientationDomainWord struct {
	Word     string   `json:"word"`
	Guess    string   `json:"guess"`
	Evidence []string `json:"evidence"`
}

type fileToOpen struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority,omitempty"`
}

type fileToOpenList []fileToOpen

func (items *fileToOpenList) UnmarshalJSON(data []byte) error {
	var objects []fileToOpen
	if err := json.Unmarshal(data, &objects); err == nil {
		*items = objects
		return nil
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return err
	}
	result := make([]fileToOpen, 0, len(paths))
	for _, path := range paths {
		result = append(result, fileToOpen{Path: path})
	}
	*items = result
	return nil
}

type unverifiedPath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type unverifiedPathList []unverifiedPath

func (items *unverifiedPathList) UnmarshalJSON(data []byte) error {
	var objects []unverifiedPath
	if err := json.Unmarshal(data, &objects); err == nil {
		*items = objects
		return nil
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return err
	}
	result := make([]unverifiedPath, 0, len(paths))
	for _, path := range paths {
		result = append(result, unverifiedPath{Path: path})
	}
	*items = result
	return nil
}

func parseOrientation(data []byte) (orientationPart, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return orientationPart{}, err
	}
	var report orientationPart
	if err := json.Unmarshal(data, &report); err != nil {
		return orientationPart{}, err
	}

	if jsonArrayUsesStrings(fields["first_files_to_open"]) {
		report.Warnings = append(report.Warnings, "parser normalized string items in first_files_to_open")
	}
	if jsonArrayUsesStrings(fields["unverified_paths"]) {
		report.Warnings = append(report.Warnings, "parser normalized string items in unverified_paths")
	}
	known := map[string]struct{}{
		"project_guess": {}, "confidence": {}, "high_level_map": {}, "first_files_to_open": {},
		"candidate_flows": {}, "important_domain_words": {}, "questions_for_human": {},
		"unverified_paths": {}, "warnings": {},
	}
	var unknown []string
	for field := range fields {
		if _, exists := known[field]; !exists {
			unknown = append(unknown, field)
		}
	}
	sort.Strings(unknown)
	for _, field := range unknown {
		report.Warnings = append(report.Warnings, fmt.Sprintf("parser ignored unknown field %q", field))
	}
	return report, nil
}

func jsonArrayUsesStrings(data json.RawMessage) bool {
	if len(data) == 0 {
		return false
	}
	var values []string
	return json.Unmarshal(data, &values) == nil && len(values) > 0
}

func validateOrientation(report orientationPart, allowedPaths, allowedEntrypoints []string) error {
	if strings.TrimSpace(report.ProjectGuess) == "" {
		return fmt.Errorf("orientation: project_guess is required")
	}
	if report.Confidence < 0 || report.Confidence > 1 {
		return fmt.Errorf("orientation: confidence %.3f is outside [0,1]", report.Confidence)
	}
	if len(report.CandidateFlows) == 0 {
		return fmt.Errorf("orientation: at least one candidate flow is required")
	}

	allowed := make(map[string]struct{}, len(allowedPaths))
	for _, path := range allowedPaths {
		allowed[path] = struct{}{}
	}
	entrypoints := make(map[string]struct{}, len(allowedEntrypoints))
	for _, entrypoint := range allowedEntrypoints {
		if entrypoint = strings.TrimSpace(entrypoint); entrypoint != "" && entrypoint != "." {
			entrypoints[entrypoint] = struct{}{}
		}
	}
	validateAllowed := func(field, path string) error {
		if !validRepoRelativePath(path) {
			return fmt.Errorf("orientation: %s has invalid path %q", field, path)
		}
		if _, ok := allowed[path]; !ok {
			return fmt.Errorf("orientation: %s references path outside allowed_paths: %q", field, path)
		}
		return nil
	}
	validateEvidence := func(field string, evidence []string) error {
		for evidenceIndex, statement := range evidence {
			for _, path := range evidencePathMentions(statement) {
				if !validRepoRelativePath(path) {
					return fmt.Errorf("orientation: %s[%d] has invalid path-like evidence %q", field, evidenceIndex, path)
				}
				if _, ok := allowed[path]; !ok {
					return fmt.Errorf("orientation: %s[%d] references path-like evidence outside allowed_paths: %q", field, evidenceIndex, path)
				}
			}
		}
		return nil
	}

	for index, item := range report.HighLevelMap {
		if err := validateEvidence(fmt.Sprintf("high_level_map[%d].evidence", index), item.Evidence); err != nil {
			return err
		}
	}
	for index, item := range report.ImportantDomainWords {
		if err := validateEvidence(fmt.Sprintf("important_domain_words[%d].evidence", index), item.Evidence); err != nil {
			return err
		}
	}

	for index, file := range report.FirstFilesToOpen {
		if err := validateAllowed(fmt.Sprintf("first_files_to_open[%d]", index), file.Path); err != nil {
			return err
		}
	}
	for flowIndex, flow := range report.CandidateFlows {
		if strings.TrimSpace(flow.Name) == "" || strings.TrimSpace(flow.Trigger) == "" {
			return fmt.Errorf("orientation: candidate_flows[%d] is missing name or trigger", flowIndex)
		}
		if flow.Confidence < 0 || flow.Confidence > 1 {
			return fmt.Errorf("orientation: candidate_flows[%d] confidence is outside [0,1]", flowIndex)
		}
		if len(flow.LikelyFiles) == 0 {
			return fmt.Errorf("orientation: candidate_flows[%d] has no likely_files", flowIndex)
		}
		if len(flow.Evidence) == 0 {
			return fmt.Errorf("orientation: candidate_flows[%d] has no evidence", flowIndex)
		}
		if err := validateEvidence(fmt.Sprintf("candidate_flows[%d].evidence", flowIndex), flow.Evidence); err != nil {
			return err
		}
		for pathIndex, path := range flow.LikelyFiles {
			if err := validateAllowed(
				fmt.Sprintf("candidate_flows[%d].likely_files[%d]", flowIndex, pathIndex),
				path,
			); err != nil {
				return err
			}
		}
		entrypoint := strings.TrimSpace(flow.LikelyEntrypoint)
		if entrypoint == "" {
			return fmt.Errorf("orientation: candidate_flows[%d] has no likely_entrypoint", flowIndex)
		}
		if _, isAllowedPath := allowed[entrypoint]; isAllowedPath {
			if !validRepoRelativePath(entrypoint) {
				return fmt.Errorf("orientation: candidate_flows[%d].likely_entrypoint has invalid path %q", flowIndex, entrypoint)
			}
		} else if _, isKnownEntrypoint := entrypoints[entrypoint]; !isKnownEntrypoint {
			return fmt.Errorf("orientation: candidate_flows[%d].likely_entrypoint is not a provided path or package: %q", flowIndex, entrypoint)
		}
	}
	for index, path := range report.UnverifiedPaths {
		if !validRepoRelativePath(path.Path) {
			return fmt.Errorf("orientation: unverified_paths[%d] has invalid path %q", index, path.Path)
		}
	}
	return nil
}

func evidencePathMentions(statement string) []string {
	seen := make(map[string]struct{})
	var paths []string
	for _, pattern := range []*regexp.Regexp{evidenceFilePathPattern, evidenceEscapePattern} {
		for _, match := range pattern.FindAllStringSubmatch(statement, -1) {
			if len(match) < 2 || match[1] == "" {
				continue
			}
			path := strings.TrimSuffix(strings.TrimSuffix(match[1], "."), ",")
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	return paths
}

func orientationEntrypoints(bundle llmbundle.Bundle) []string {
	var entrypoints []string
	for _, entrypoint := range bundle.Go.Entrypoints {
		entrypoints = append(entrypoints, entrypoint.ImportPath, entrypoint.PackageDir)
	}
	for _, candidate := range bundle.Go.OrientationCandidates {
		entrypoints = append(entrypoints, candidate.EntrypointPackage)
	}
	return entrypoints
}

func validRepoRelativePath(path string) bool {
	if path == "" || strings.Contains(path, `\`) {
		return false
	}
	native := filepath.FromSlash(path)
	if filepath.IsAbs(native) || !filepath.IsLocal(native) {
		return false
	}
	return filepath.ToSlash(filepath.Clean(native)) == path && path != "."
}
