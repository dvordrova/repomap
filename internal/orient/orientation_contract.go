package orient

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/evidenceref"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/llmbundle"
	"github.com/dvordrova/repomap/internal/modelresearch"
)

var (
	// The consumed terminal delimiter is part of the grammar so a shorter
	// extension cannot match a prefix of a longer one (for example, .ts in
	// .tsx). evidencePathMentions resumes at the capture boundary so consuming
	// that delimiter does not hide an immediately adjacent path.
	evidenceFilePathPattern  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_./-])((?:/|\./|\.\./)?[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*\.(?:go|md|yaml|yml|json|toml|proto|mod|sum|sh|c|h|rs|py|js|tsx|ts))(?:[:#][0-9]+)?(?:$|[^A-Za-z0-9_./-]|\.(?:$|[^A-Za-z0-9_./-]))`)
	evidenceEscapePattern    = regexp.MustCompile(`(?:^|[\s"'()\[\]{},;=:])((?:/|\.\./)[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*)`)
	versionedAPIRoutePattern = regexp.MustCompile(`(?i)^/(?:api/)?v[0-9]+(?:/|$)`)
)

type orientationPart struct {
	ProjectGuess         string                           `json:"project_guess"`
	Confidence           float64                          `json:"confidence"`
	HighLevelMap         []orientationMapItem             `json:"high_level_map"`
	FirstFilesToOpen     fileToOpenList                   `json:"first_files_to_open"`
	CandidateFlows       []flowexplain.CandidateFlow      `json:"candidate_flows"`
	ImportantDomainWords []orientationDomainWord          `json:"important_domain_words"`
	QuestionsForHuman    []string                         `json:"questions_for_human"`
	ResearchQuestions    []modelresearch.ProposedQuestion `json:"research_questions,omitempty"`
	UnverifiedPaths      unverifiedPathList               `json:"unverified_paths"`
	Warnings             []string                         `json:"warnings"`
	// confidenceWarningDiagnostics is a producer-owned, render-only account of
	// warnings appended by the local confidence gate. It never enters the
	// canonical orientation JSON.
	confidenceWarningDiagnostics []ConfidenceWarningDiagnostic
}

type orientationMapItem struct {
	Name         string            `json:"name"`
	Role         componentmap.Role `json:"role"`
	Evidence     []string          `json:"evidence"`
	WhyItMatters string            `json:"why_it_matters"`
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
	for index := range report.CandidateFlows {
		// Candidate basis is local policy metadata, never provider authority.
		report.CandidateFlows[index].CandidateBasis = flowexplain.CandidateBasisModelOrientation
	}

	if jsonArrayUsesStrings(fields["first_files_to_open"]) {
		report.Warnings = append(report.Warnings, "parser normalized string items in first_files_to_open")
	}
	if jsonArrayUsesStrings(fields["unverified_paths"]) {
		report.Warnings = append(report.Warnings, "parser normalized string items in unverified_paths")
	}
	known := map[string]struct{}{
		"project_guess": {}, "confidence": {}, "high_level_map": {}, "first_files_to_open": {},
		"candidate_flows": {}, "important_domain_words": {}, "questions_for_human": {}, "research_questions": {},
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
	if orientationHasWildcardEvidence(report) {
		report.Warnings = append(report.Warnings, "parser treated wildcard evidence as unverified prose")
	}
	for index := range report.HighLevelMap {
		value := string(report.HighLevelMap[index].Role)
		role, ok := componentmap.Normalize(value)
		report.HighLevelMap[index].Role = role
		if !ok {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser normalized unknown high_level_map[%d].role %q to %q",
				index,
				value,
				componentmap.RoleUnknown,
			))
		}
	}
	return report, nil
}

func orientationHasWildcardEvidence(report orientationPart) bool {
	hasWildcard := func(values []string) bool {
		for _, value := range values {
			if strings.Contains(value, "/") && strings.ContainsAny(value, "*?[]") {
				return true
			}
		}
		return false
	}
	for _, item := range report.HighLevelMap {
		if hasWildcard(item.Evidence) {
			return true
		}
	}
	for _, item := range report.CandidateFlows {
		if hasWildcard(item.Evidence) {
			return true
		}
	}
	for _, item := range report.ImportantDomainWords {
		if hasWildcard(item.Evidence) {
			return true
		}
	}
	return false
}

func jsonArrayUsesStrings(data json.RawMessage) bool {
	if len(data) == 0 {
		return false
	}
	var values []string
	return json.Unmarshal(data, &values) == nil && len(values) > 0
}

// normalizeOrientationGrounding tolerates provider drift without weakening the
// structured navigation contract. Ungrounded prose and navigation entries are
// dropped, while remaining exact paths can repair an entrypoint or seed.
func normalizeOrientationGrounding(
	report *orientationPart,
	allowedPaths []string,
	allowedEntrypoints []string,
	signalLocations []evidence.Location,
) {
	allowed := make(map[string]struct{}, len(allowedPaths))
	for _, filePath := range allowedPaths {
		allowed[filePath] = struct{}{}
	}
	entrypoints := make(map[string]struct{}, len(allowedEntrypoints))
	for _, entrypoint := range allowedEntrypoints {
		entrypoint = strings.TrimSpace(entrypoint)
		if entrypoint != "" && entrypoint != "." {
			entrypoints[entrypoint] = struct{}{}
		}
	}

	filterEvidence := func(field string, evidence []string) []string {
		filtered := make([]string, 0, len(evidence))
		for index, statement := range evidence {
			var lineGrounded bool
			statement, lineGrounded = evidenceref.Canonicalize(statement, allowedPaths, signalLocations)
			if !lineGrounded {
				report.Warnings = append(
					report.Warnings,
					fmt.Sprintf("parser removed ungrounded line claim from %s[%d]", field, index),
				)
			}
			grounded := true
			for _, filePath := range evidencePathMentions(statement) {
				if !validRepoRelativePath(filePath) {
					grounded = false
					break
				}
				if _, ok := allowed[filePath]; !ok {
					grounded = false
					break
				}
			}
			if grounded {
				filtered = append(filtered, statement)
				continue
			}
			report.Warnings = append(
				report.Warnings,
				fmt.Sprintf("parser dropped ungrounded path-like evidence from %s[%d]", field, index),
			)
		}
		return filtered
	}

	normalizedFirstFiles := make([]fileToOpen, 0, len(report.FirstFilesToOpen))
	seenFirstFiles := make(map[string]struct{}, len(report.FirstFilesToOpen))
	for index, item := range report.FirstFilesToOpen {
		candidate := strings.TrimSpace(item.Path)
		_, grounded := allowed[candidate]
		if !validRepoRelativePath(candidate) || !grounded {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser dropped first_files_to_open[%d] outside allowed_paths: %q",
				index,
				candidate,
			))
			continue
		}
		if _, duplicate := seenFirstFiles[candidate]; duplicate {
			continue
		}
		seenFirstFiles[candidate] = struct{}{}
		item.Path = candidate
		normalizedFirstFiles = append(normalizedFirstFiles, item)
	}
	report.FirstFilesToOpen = normalizedFirstFiles

	normalizedUnverifiedPaths := make(
		unverifiedPathList,
		0,
		len(report.UnverifiedPaths),
	)
	seenUnverifiedPaths := make(map[string]struct{}, len(report.UnverifiedPaths))
	for index, item := range report.UnverifiedPaths {
		candidate := strings.TrimSpace(item.Path)
		candidate = strings.TrimRight(candidate, "/")
		if !validRepoRelativePath(candidate) {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser dropped unverified_paths[%d] with invalid path: %q",
				index,
				item.Path,
			))
			continue
		}
		if candidate != item.Path {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser normalized unverified_paths[%d] to %q",
				index,
				candidate,
			))
		}
		if _, duplicate := seenUnverifiedPaths[candidate]; duplicate {
			continue
		}
		seenUnverifiedPaths[candidate] = struct{}{}
		item.Path = candidate
		normalizedUnverifiedPaths = append(normalizedUnverifiedPaths, item)
	}
	report.UnverifiedPaths = normalizedUnverifiedPaths

	for index := range report.HighLevelMap {
		report.HighLevelMap[index].Evidence = filterEvidence(
			fmt.Sprintf("high_level_map[%d].evidence", index),
			report.HighLevelMap[index].Evidence,
		)
	}
	for index := range report.ImportantDomainWords {
		report.ImportantDomainWords[index].Evidence = filterEvidence(
			fmt.Sprintf("important_domain_words[%d].evidence", index),
			report.ImportantDomainWords[index].Evidence,
		)
	}
	normalizedFlows := make([]flowexplain.CandidateFlow, 0, len(report.CandidateFlows))
	for index := range report.CandidateFlows {
		flow := report.CandidateFlows[index]
		switch flow.FlowType {
		case "", flowexplain.FlowTypeRequest:
			flow.FlowType = flowexplain.FlowTypeRequest
		case flowexplain.FlowTypeOperational:
		default:
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser removed unsupported candidate_flows[%d].flow_type",
				index,
			))
			flow.FlowType = ""
		}
		flow.Evidence = filterEvidence(
			fmt.Sprintf("candidate_flows[%d].evidence", index),
			flow.Evidence,
		)

		filteredFiles := make([]string, 0, len(flow.LikelyFiles))
		seenFiles := make(map[string]struct{}, len(flow.LikelyFiles))
		for fileIndex, candidate := range flow.LikelyFiles {
			candidate = strings.TrimSpace(candidate)
			if !validRepoRelativePath(candidate) {
				report.Warnings = append(report.Warnings, fmt.Sprintf(
					"parser dropped invalid candidate_flows[%d].likely_files[%d]",
					index,
					fileIndex,
				))
				continue
			}
			if _, ok := allowed[candidate]; !ok {
				report.Warnings = append(report.Warnings, fmt.Sprintf(
					"parser dropped ungrounded candidate_flows[%d].likely_files[%d]",
					index,
					fileIndex,
				))
				continue
			}
			if _, duplicate := seenFiles[candidate]; duplicate {
				continue
			}
			seenFiles[candidate] = struct{}{}
			filteredFiles = append(filteredFiles, candidate)
		}
		flow.LikelyFiles = filteredFiles

		entrypoint := strings.TrimSpace(flow.LikelyEntrypoint)
		_, isAllowedPath := allowed[entrypoint]
		_, isAllowedPackage := entrypoints[entrypoint]
		if !isAllowedPath && !isAllowedPackage {
			flow.LikelyEntrypoint = ""
			if len(flow.LikelyFiles) > 0 {
				flow.LikelyEntrypoint = flow.LikelyFiles[0]
				report.Warnings = append(
					report.Warnings,
					fmt.Sprintf("parser replaced ungrounded candidate_flows[%d].likely_entrypoint with an allowed likely_file", index),
				)
			}
		}

		if len(flow.LikelyFiles) == 0 {
			if _, ok := allowed[flow.LikelyEntrypoint]; ok {
				flow.LikelyFiles = append(flow.LikelyFiles, flow.LikelyEntrypoint)
			}
		}
		if len(flow.LikelyFiles) == 0 {
			for _, statement := range flow.Evidence {
				for _, candidate := range evidencePathMentions(statement) {
					if _, ok := allowed[candidate]; !ok {
						continue
					}
					if _, duplicate := seenFiles[candidate]; duplicate {
						continue
					}
					seenFiles[candidate] = struct{}{}
					flow.LikelyFiles = append(flow.LikelyFiles, candidate)
				}
			}
		}
		if flow.LikelyEntrypoint == "" && len(flow.LikelyFiles) > 0 {
			flow.LikelyEntrypoint = flow.LikelyFiles[0]
			report.Warnings = append(
				report.Warnings,
				fmt.Sprintf("parser derived candidate_flows[%d].likely_entrypoint from grounded evidence", index),
			)
		}
		if len(flow.LikelyFiles) == 0 {
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser dropped candidate_flows[%d] because no grounded file remained",
				index,
			))
			continue
		}
		if len(flow.Evidence) == 0 {
			flow.Evidence = []string{flow.LikelyFiles[0]}
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser replaced empty candidate_flows[%d].evidence with its first grounded file",
				index,
			))
		}
		normalizedFlows = append(normalizedFlows, flow)
	}
	report.CandidateFlows = normalizedFlows
	normalizeCandidateFlowIDs(report)
}

func normalizeCandidateFlowIDs(report *orientationPart) {
	used := make(map[string]struct{}, len(report.CandidateFlows))
	for index := range report.CandidateFlows {
		flow := &report.CandidateFlows[index]
		originalName := flow.Name
		flowID := flowexplain.GenerateFlowID(originalName)
		if _, exists := used[flowID]; !exists {
			used[flowID] = struct{}{}
			continue
		}
		for suffix := 2; ; suffix++ {
			flow.Name = fmt.Sprintf("%s (%d)", originalName, suffix)
			flowID = flowexplain.GenerateFlowID(flow.Name)
			if _, exists := used[flowID]; exists {
				continue
			}
			used[flowID] = struct{}{}
			report.Warnings = append(report.Warnings, fmt.Sprintf(
				"parser disambiguated candidate_flows[%d].name because its local id collided with an earlier direction",
				index,
			))
			break
		}
	}
}

func validateOrientation(report orientationPart, allowedPaths, allowedEntrypoints []string) error {
	if err := validateOrientationStructure(report); err != nil {
		return err
	}
	return validateLegacyOrientationGrounding(report, allowedPaths, allowedEntrypoints)
}

// validateResolvedOrientation runs after exact request-local ref resolution.
// Repository identity and evidence authority have already been proven by typed
// lookup, so legacy path/evidence lexical parsing is not a second authority.
func validateResolvedOrientation(report orientationPart) error {
	if len(report.UnverifiedPaths) != 0 {
		return fmt.Errorf("orientation: resolved response cannot contain unverified_paths")
	}
	return validateOrientationStructure(report)
}

func validateOrientationStructure(report orientationPart) error {
	if strings.TrimSpace(report.ProjectGuess) == "" {
		return fmt.Errorf("orientation: project_guess is required")
	}
	if report.Confidence < 0 || report.Confidence > 1 {
		return fmt.Errorf("orientation: confidence %.3f is outside [0,1]", report.Confidence)
	}
	if len(report.CandidateFlows) == 0 {
		return fmt.Errorf("orientation: at least one candidate flow is required")
	}

	flowIDs := make(map[string]string, len(report.CandidateFlows))
	for flowIndex, flow := range report.CandidateFlows {
		if strings.TrimSpace(flow.Name) == "" || strings.TrimSpace(flow.Trigger) == "" {
			return fmt.Errorf("orientation: candidate_flows[%d] is missing name or trigger", flowIndex)
		}
		flowID := flowexplain.GenerateFlowID(flow.Name)
		if previousName, exists := flowIDs[flowID]; exists {
			return fmt.Errorf(
				"orientation: candidate_flows[%d] name %q collides with %q as id %q",
				flowIndex,
				flow.Name,
				previousName,
				flowID,
			)
		}
		flowIDs[flowID] = flow.Name
		if flow.Confidence < 0 || flow.Confidence > 1 {
			return fmt.Errorf("orientation: candidate_flows[%d] confidence is outside [0,1]", flowIndex)
		}
		if flow.Disposition != "" && flow.Disposition != flowexplain.DirectionAccepted && flow.Disposition != flowexplain.DirectionRejected {
			return fmt.Errorf("orientation: candidate_flows[%d] has no local acceptance disposition", flowIndex)
		}
		if flow.Disposition == flowexplain.DirectionRejected && strings.TrimSpace(flow.DispositionReason) == "" {
			return fmt.Errorf("orientation: candidate_flows[%d] rejected without a reason", flowIndex)
		}
		if len(flow.LikelyFiles) == 0 {
			return fmt.Errorf("orientation: candidate_flows[%d] has no likely_files", flowIndex)
		}
		if len(flow.Evidence) == 0 {
			return fmt.Errorf("orientation: candidate_flows[%d] has no evidence", flowIndex)
		}
		entrypoint := strings.TrimSpace(flow.LikelyEntrypoint)
		if entrypoint == "" {
			return fmt.Errorf("orientation: candidate_flows[%d] has no likely_entrypoint", flowIndex)
		}
	}
	return nil
}

func validateLegacyOrientationGrounding(report orientationPart, allowedPaths, allowedEntrypoints []string) error {
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
	validateEvidence := func(field string, statements []string) error {
		for evidenceIndex, statement := range statements {
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
		if err := validateEvidence(fmt.Sprintf("candidate_flows[%d].evidence", flowIndex), flow.Evidence); err != nil {
			return err
		}
		for pathIndex, path := range flow.LikelyFiles {
			if err := validateAllowed(fmt.Sprintf("candidate_flows[%d].likely_files[%d]", flowIndex, pathIndex), path); err != nil {
				return err
			}
		}
		entrypoint := strings.TrimSpace(flow.LikelyEntrypoint)
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
	appendPath := func(path string) {
		path = strings.TrimSuffix(strings.TrimSuffix(path, "."), ",")
		if path == "" || looksLikeHTTPRoute(statement, path) {
			return
		}
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	for offset := 0; offset < len(statement); {
		match := evidenceFilePathPattern.FindStringSubmatchIndex(statement[offset:])
		if len(match) < 4 || match[2] < 0 || match[3] <= match[2] {
			break
		}
		captureStart := offset + match[2]
		captureEnd := offset + match[3]
		appendPath(statement[captureStart:captureEnd])
		// Resume at the end of the captured path, before the terminal delimiter
		// consumed by the RE2 grammar. This preserves adjacent path matches.
		offset = captureEnd
	}

	for _, match := range evidenceEscapePattern.FindAllStringSubmatch(statement, -1) {
		if len(match) >= 2 {
			appendPath(match[1])
		}
	}
	return paths
}

func looksLikeHTTPRoute(statement, path string) bool {
	if !strings.HasPrefix(path, "/") || filepath.Ext(path) != "" {
		return false
	}
	if versionedAPIRoutePattern.MatchString(path) {
		return true
	}
	for offset := 0; offset < len(statement); {
		index := strings.Index(statement[offset:], path)
		if index < 0 {
			break
		}
		index += offset
		prefix := strings.TrimSpace(statement[:index])
		words := strings.Fields(prefix)
		if len(words) > 0 {
			method := strings.Trim(words[len(words)-1], "`'\"([{,:;")
			switch strings.ToUpper(method) {
			case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
				return true
			}
		}
		offset = index + len(path)
	}
	return false
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
