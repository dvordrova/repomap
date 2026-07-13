package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/evidenceref"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/sourcesignals"
)

type snapshotJSON struct {
	RepoName string `json:"repo_name"`
	GoFacts  *struct {
		Modules []struct {
			ID          string `json:"id"`
			ModulePath  string `json:"module_path"`
			ModuleDir   string `json:"module_dir"`
			DisplayName string `json:"display_name"`
		} `json:"modules"`
		Packages      []PackageInfo          `json:"packages"`
		CommandTraces []gofacts.CommandTrace `json:"command_traces"`
	} `json:"go_facts"`
}

type llmBundleJSON struct {
	ProviderAllowedPaths []string               `json:"allowed_paths"`
	SourceSignals        []sourcesignals.Signal `json:"source_signals"`
	Go                   struct {
		ModuleSummaries []struct {
			ModulePath string `json:"module_path"`
			ModuleDir  string `json:"module_dir"`
		} `json:"module_summaries"`
		ImportantEdges []EdgeInfo `json:"important_edges"`
	} `json:"go"`
}

type runMetadataJSON struct {
	CreatedAt               string   `json:"created_at"`
	Model                   string   `json:"model"`
	PromptVersion           string   `json:"prompt_version"`
	CompactContextBytes     int      `json:"compact_context_bytes"`
	ExternalRequestBytes    int      `json:"external_request_bytes"`
	ProviderRequestCount    int      `json:"provider_request_count"`
	CandidateDirectionCount int      `json:"candidate_direction_count"`
	AcceptedDirectionCount  int      `json:"accepted_direction_count"`
	RejectedDirectionCount  int      `json:"rejected_direction_count"`
	ProviderLatencyMillis   *int64   `json:"provider_latency_ms"`
	SurfaceDiscoveryRan     bool     `json:"surface_discovery_ran"`
	SurfaceDiscoveryCount   int      `json:"surface_discovery_count"`
	SurfaceDiscoveryMillis  *int64   `json:"surface_discovery_ms"`
	Warnings                []string `json:"warnings"`
}

type orientationReportJSON struct {
	ProjectGuess         string                      `json:"project_guess"`
	Confidence           float64                     `json:"confidence"`
	HighLevelMap         []orientationMapItemJSON    `json:"high_level_map"`
	FirstFilesToOpen     flexFileItems               `json:"first_files_to_open"`
	CandidateFlows       []orientationCandidateJSON  `json:"candidate_flows"`
	ImportantDomainWords []orientationDomainWordJSON `json:"important_domain_words"`
	QuestionsForHuman    []string                    `json:"questions_for_human"`
	UnverifiedPaths      flexPathItems               `json:"unverified_paths"`
	Warnings             []string                    `json:"warnings"`
}

type orientationMapItemJSON struct {
	Name         string            `json:"name"`
	Role         componentmap.Role `json:"role"`
	Evidence     []string          `json:"evidence"`
	WhyItMatters string            `json:"why_it_matters"`
}

type orientationDomainWordJSON struct {
	Word     string   `json:"word"`
	Guess    string   `json:"guess"`
	Evidence []string `json:"evidence"`
}

type orientationCandidateJSON struct {
	Name              string                        `json:"name"`
	FlowType          string                        `json:"flow_type"`
	Trigger           string                        `json:"trigger"`
	LikelyEntrypoint  string                        `json:"likely_entrypoint"`
	LikelyFiles       []string                      `json:"likely_files"`
	WhyInteresting    string                        `json:"why_interesting"`
	Evidence          []string                      `json:"evidence"`
	Confidence        float64                       `json:"confidence"`
	LocalVerification *flowexplain.FlowVerification `json:"local_verification"`
	LocalProof        *flowproof.Session            `json:"local_proof"`
	Disposition       string                        `json:"disposition"`
	DispositionReason string                        `json:"disposition_reason"`
}

type flowReportJSON struct {
	Summary            string               `json:"summary"`
	Confidence         float64              `json:"confidence"`
	FlowName           string               `json:"flow_name"`
	LikelyChain        flexChainSteps       `json:"likely_chain"`
	FilesToReadInOrder flexFileItems        `json:"files_to_read_in_order"`
	TestsToRead        flexFileItems        `json:"tests_to_read"`
	UnverifiedPaths    flexPathItems        `json:"unverified_paths"`
	Unknowns           flexStringsOrObjects `json:"unknowns"`
	Warnings           flexStringsOrObjects `json:"warnings"`
}

type flowStatusJSON struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"`
}

type chainStepJSON struct {
	Step          flexInt     `json:"step"`
	Name          string      `json:"name"`
	WhatHappens   string      `json:"what_happens"`
	Description   string      `json:"description"`
	Reason        string      `json:"reason"`
	Role          string      `json:"role"`
	File          string      `json:"file"`
	Function      string      `json:"function"`
	EvidenceFiles flexStrings `json:"evidence_files"`
	Confidence    float64     `json:"confidence"`
}

// flexInt accepts int, "1", "Step 1", or missing values.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	var i int
	if err := json.Unmarshal(b, &i); err == nil {
		*f = flexInt(i)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("step: must be int or string: %s", string(b))
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Step ")
	s = strings.TrimPrefix(s, "step ")
	s = strings.TrimSpace(s)
	parsed, err := strconv.Atoi(s)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(parsed)
	return nil
}

type fileItemJSON struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Priority int    `json:"priority"`
}

type pathItemJSON struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// flexFileItems accepts both []object and []string for files_to_read_in_order / tests_to_read.
type flexFileItems []fileItemJSON

func (f *flexFileItems) UnmarshalJSON(b []byte) error {
	var objs []fileItemJSON
	if err := json.Unmarshal(b, &objs); err == nil {
		*f = objs
		return nil
	}
	var strs []string
	if err := json.Unmarshal(b, &strs); err != nil {
		return fmt.Errorf("must be array of objects or strings: %s", string(b))
	}
	result := make([]fileItemJSON, len(strs))
	for i, s := range strs {
		result[i] = fileItemJSON{Path: s, Reason: "", Priority: i + 1}
	}
	*f = result
	return nil
}

// flexPathItems accepts both []object and []string for unverified_paths.
type flexPathItems []pathItemJSON

func (f *flexPathItems) UnmarshalJSON(b []byte) error {
	var objs []pathItemJSON
	if err := json.Unmarshal(b, &objs); err == nil {
		*f = objs
		return nil
	}
	var strs []string
	if err := json.Unmarshal(b, &strs); err != nil {
		return fmt.Errorf("must be array of objects or strings: %s", string(b))
	}
	result := make([]pathItemJSON, len(strs))
	for i, s := range strs {
		result[i] = pathItemJSON{Path: s, Reason: ""}
	}
	*f = result
	return nil
}

// flexChainSteps accepts []chainStepJSON directly, or an object with a "steps" array.
type flexChainSteps []chainStepJSON

func (f *flexChainSteps) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = nil
		return nil
	}

	// Try direct array first
	var steps []chainStepJSON
	if err := json.Unmarshal(b, &steps); err == nil {
		*f = steps
		return nil
	}

	// Try object with "steps" field
	var obj struct {
		Steps []chainStepJSON `json:"steps"`
	}
	if err := json.Unmarshal(b, &obj); err == nil && len(obj.Steps) > 0 {
		*f = obj.Steps
		return nil
	}

	return fmt.Errorf("flexChainSteps: must be array of chain steps or object with steps field: %s", string(b))
}

// flexStringsOrObjects accepts []string, []object (with text/description/reason/question/uncertainty/warning/message fields),
// or object/map (grouped unknowns), or a bare string. Normalizes to a flat []string.
type flexStringsOrObjects []string

func (f *flexStringsOrObjects) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = nil
		return nil
	}

	// Try bare string first
	var singleStr string
	if err := json.Unmarshal(b, &singleStr); err == nil && strings.TrimSpace(singleStr) != "" {
		*f = []string{strings.TrimSpace(singleStr)}
		return nil
	}

	// Try []string first
	var strs []string
	if err := json.Unmarshal(b, &strs); err == nil {
		*f = strs
		return nil
	}

	// Try []object — extract first meaningful text-like field
	var objs []map[string]json.RawMessage
	if err := json.Unmarshal(b, &objs); err == nil {
		result := make([]string, 0, len(objs))
		for _, obj := range objs {
			result = append(result, extractFlexString(obj))
		}
		*f = result
		return nil
	}

	// Try object/map (grouped form)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err == nil {
		var result []string
		for k, v := range m {
			var subObjs []map[string]json.RawMessage
			if json.Unmarshal(v, &subObjs) == nil {
				for _, obj := range subObjs {
					s := extractFlexString(obj)
					if s != "" {
						result = append(result, k+": "+s)
					}
				}
				continue
			}
			var subStrs []string
			if json.Unmarshal(v, &subStrs) == nil {
				for _, s := range subStrs {
					result = append(result, k+": "+s)
				}
				continue
			}
			var singleStr string
			if json.Unmarshal(v, &singleStr) == nil && singleStr != "" {
				result = append(result, k+": "+singleStr)
			}
		}
		sort.Strings(result)
		*f = result
		return nil
	}

	return fmt.Errorf("flexStringsOrObjects: must be array of strings, array of objects, or object/map: %s", string(b))
}

var flexStringFields = []string{"text", "uncertainty", "warning", "question", "message", "description", "reason", "path"}

func extractFlexString(obj map[string]json.RawMessage) string {
	// First look for single-word descriptive fields
	pairs := make([]string, 0, 2)
	for _, field := range flexStringFields {
		if raw, ok := obj[field]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
				pairs = append(pairs, strings.TrimSpace(s))
			}
		}
	}
	if len(pairs) == 0 {
		// Fallback: use the first field
		for _, raw := range obj {
			var s string
			if json.Unmarshal(raw, &s) == nil && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	return strings.Join(pairs, " — ")
}

// flexStrings accepts both []string and []object (with "path" field) for evidence_files.
type flexStrings []string

func (f *flexStrings) UnmarshalJSON(b []byte) error {
	var strs []string
	if err := json.Unmarshal(b, &strs); err == nil {
		*f = strs
		return nil
	}
	var objs []struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(b, &objs); err != nil {
		return fmt.Errorf("must be array of strings or objects with path field: %s", string(b))
	}
	result := make([]string, len(objs))
	for i, o := range objs {
		result[i] = o.Path
	}
	*f = result
	return nil
}

func ReadRunDir(runDir string) (*ReportData, error) {
	absDir, err := filepath.Abs(runDir)
	if err != nil {
		return nil, fmt.Errorf("resolve run dir: %w", err)
	}
	data := &ReportData{ArtifactsDir: absDir}
	var parseWarnings []string

	if w := parseSnapshot(filepath.Join(absDir, "snapshot.json"), data); w != "" {
		parseWarnings = append(parseWarnings, w)
	}
	if w := parseRunMetadata(filepath.Join(absDir, "metadata.json"), data); w != "" {
		parseWarnings = append(parseWarnings, w)
	}
	if state, err := modelresearch.ReadState(absDir); err == nil {
		data.ModelResearch = &state
		if data.Run == nil {
			data.Run = &RunInfo{}
		}
		data.Run.ProviderRequestCount = state.Usage.SemanticCalls
		data.Run.ExternalRequestBytes = state.Usage.RequestBytes
	} else if !os.IsNotExist(err) {
		parseWarnings = append(parseWarnings, fmt.Sprintf("model research: %v", err))
	}
	architectureStatus, warning := readArchitectureSynthesisStatus(
		filepath.Join(absDir, ArchitectureSynthesisStatusFile),
	)
	data.ArchitectureSynthesis = architectureStatus
	if warning != "" {
		parseWarnings = append(parseWarnings, warning)
	}
	if warning = architectureSynthesisUserWarning(data.ArchitectureSynthesis); warning != "" {
		parseWarnings = append(parseWarnings, warning)
	}
	if data.Run != nil && data.ArchitectureSynthesis != nil && data.ModelResearch == nil {
		data.Run.ProviderRequestCount += data.ArchitectureSynthesis.ProviderRequestCount
	}
	if w := parseOrientationReport(filepath.Join(absDir, "orientation_report.json"), data); w != "" {
		parseWarnings = append(parseWarnings, w)
	}
	if w := parseLLMBundle(filepath.Join(absDir, "llm_bundle.json"), data); w != "" {
		parseWarnings = append(parseWarnings, w)
	}
	var surfaceWarnings []string
	data.DiscoveredSurfaces, surfaceWarnings = parseDiscoveredSurfaces(absDir)
	parseWarnings = append(parseWarnings, surfaceWarnings...)
	mergeCommandSurfaceCatalog(data)
	data.ArchitectureGrounding, warning = parseArchitectureGrounding(absDir)
	if warning != "" {
		parseWarnings = append(parseWarnings, warning)
	}
	ensureArchitectureGrounding(data)

	flowWarnings, err := parseFlows(filepath.Join(absDir, "flows"), data)
	if err != nil {
		return nil, fmt.Errorf("read flows from %s: %w", absDir, err)
	}
	parseWarnings = append(parseWarnings, flowWarnings...)
	canonicalizeReportEvidence(data)
	collectOpenablePaths(data)
	buildComponents(data)
	if w := projectSavedArchitectureCanvas(data, filepath.Join(absDir, ArchitectureSynthesisFile)); w != "" {
		parseWarnings = append(parseWarnings, w)
	}
	linkArchitectureProductObjects(data)

	enrich(data)

	sort.Slice(data.Flows, func(i, j int) bool {
		if data.Flows[i].ID == data.RecommendedFlow {
			return true
		}
		if data.Flows[j].ID == data.RecommendedFlow {
			return false
		}
		return data.Flows[i].ID < data.Flows[j].ID
	})

	data.Warnings = append(data.Warnings, parseWarnings...)
	return data, nil
}

func parseLLMBundle(path string, data *ReportData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return fmt.Sprintf("llm bundle: %v", err)
	}
	var bundle llmBundleJSON
	if err := json.Unmarshal(b, &bundle); err != nil {
		return fmt.Sprintf("llm bundle unmarshal: %v", err)
	}
	data.OpenablePaths = append(data.OpenablePaths, bundle.ProviderAllowedPaths...)
	for _, signal := range bundle.SourceSignals {
		location := evidence.Location{Path: signal.Path, Line: signal.Line}
		data.evidenceLocations = append(data.evidenceLocations, location)
		data.sourceSignals = append(data.sourceSignals, SourceSignal{
			Path:     signal.Path,
			Line:     signal.Line,
			Category: signal.Category,
			Snippet:  signal.Snippet,
			Reason:   signal.Reason,
		})
	}
	if len(bundle.Go.ModuleSummaries) > 0 || len(bundle.Go.ImportantEdges) > 0 {
		graph := data.RepositoryGraph
		if graph == nil {
			graph = &RepositoryGraph{}
		}
		graph.PackageEdges = bundle.Go.ImportantEdges
		for _, module := range bundle.Go.ModuleSummaries {
			if module.ModulePath == "" {
				continue
			}
			dir := filepath.ToSlash(filepath.Clean(module.ModuleDir))
			if dir == "." {
				dir = ""
			}
			if !repositoryGraphHasModule(graph.Modules, module.ModulePath, dir) {
				graph.Modules = append(graph.Modules, ModuleInfo{Path: module.ModulePath, Dir: dir})
			}
		}
		data.RepositoryGraph = graph
	}
	return ""
}

func projectSavedArchitectureCanvas(data *ReportData, synthesisPath string) string {
	saved, readErr := os.ReadFile(synthesisPath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return ""
		}
		return fmt.Sprintf("architecture map unavailable: cannot read saved synthesis: %v", readErr)
	}
	input, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		return fmt.Sprintf("architecture canvas: %v", err)
	}
	replayed, replayErr := ReplayArchitectureSynthesis(input, saved)
	if replayErr != nil {
		return fmt.Sprintf("architecture map unavailable: saved grouping is invalid: %v", replayErr)
	}
	canvas, err := ProjectArchitectureCanvas(replayed)
	if err != nil {
		return fmt.Sprintf("architecture canvas projection: %v", err)
	}
	data.ArchitectureCanvas = &canvas
	return ""
}

func canonicalizeReportEvidence(data *ReportData) {
	normalize := func(field string, statements []string) []string {
		for index, statement := range statements {
			canonical, grounded := evidenceref.Canonicalize(statement, data.OpenablePaths, data.evidenceLocations)
			statements[index] = canonical
			if !grounded {
				data.Warnings = append(data.Warnings, fmt.Sprintf("removed ungrounded line claim from %s[%d]", field, index))
			}
		}
		return statements
	}
	for index := range data.HighLevelMap {
		data.HighLevelMap[index].Evidence = normalize(
			fmt.Sprintf("high_level_map[%d].evidence", index),
			data.HighLevelMap[index].Evidence,
		)
	}
	for index := range data.CandidateDirections {
		data.CandidateDirections[index].Evidence = normalize(
			fmt.Sprintf("candidate_directions[%d].evidence", index),
			data.CandidateDirections[index].Evidence,
		)
	}
	for index := range data.ImportantDomainWords {
		data.ImportantDomainWords[index].Evidence = normalize(
			fmt.Sprintf("important_domain_words[%d].evidence", index),
			data.ImportantDomainWords[index].Evidence,
		)
	}
}

func collectOpenablePaths(data *ReportData) {
	paths := make(map[string]struct{}, len(data.OpenablePaths))
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			paths[value] = struct{}{}
		}
	}
	for _, path := range data.OpenablePaths {
		add(path)
	}
	for _, file := range data.FirstFilesToOpen {
		add(file.Path)
	}
	for _, direction := range data.CandidateDirections {
		add(direction.LikelyEntrypoint)
		for _, path := range direction.LikelyFiles {
			add(path)
		}
		if direction.LocalProof != nil {
			for _, anchor := range direction.LocalProof.Proof.Anchors {
				if anchor.Location != nil {
					add(anchor.Location.Path)
				}
			}
			for _, transition := range direction.LocalProof.Proof.Transitions {
				add(transition.Evidence.Path)
			}
		}
	}
	for _, flow := range data.Flows {
		for _, file := range flow.FilesToRead {
			add(file.Path)
		}
		for _, file := range flow.TestsToRead {
			add(file.Path)
		}
		for _, file := range flow.BundleFiles {
			add(file.Path)
		}
		for _, file := range flow.BundleTests {
			add(file.Path)
		}
		for _, file := range flow.BundleDocs {
			add(file.Path)
		}
		for _, step := range flow.LikelyChain {
			for _, path := range step.EvidenceFiles {
				add(path)
			}
		}
	}
	collectDiscoveredSurfacePaths(data.DiscoveredSurfaces, add)
	if data.ArchitectureGrounding != nil {
		for _, anchor := range data.ArchitectureGrounding.BehaviorAnchors {
			add(anchor.Location.Path)
			for _, member := range anchor.AssociatedMembers {
				add(member.Location.Path)
			}
		}
	}
	if data.ModelResearch != nil {
		for _, fact := range data.ModelResearch.Theory.GroundedFacts {
			if fact.Location != nil {
				add(fact.Location.Path)
			}
		}
	}
	data.OpenablePaths = data.OpenablePaths[:0]
	for path := range paths {
		data.OpenablePaths = append(data.OpenablePaths, path)
	}
	sort.Strings(data.OpenablePaths)
}

func parseRunMetadata(path string, data *ReportData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return fmt.Sprintf("metadata: %v", err)
	}
	var metadata runMetadataJSON
	if err := json.Unmarshal(b, &metadata); err != nil {
		return fmt.Sprintf("metadata unmarshal: %v", err)
	}
	data.Run = &RunInfo{
		CreatedAt:               metadata.CreatedAt,
		Model:                   metadata.Model,
		PromptVersion:           metadata.PromptVersion,
		CompactContextBytes:     metadata.CompactContextBytes,
		ExternalRequestBytes:    metadata.ExternalRequestBytes,
		ProviderRequestCount:    metadata.ProviderRequestCount,
		CandidateDirectionCount: metadata.CandidateDirectionCount,
		AcceptedDirectionCount:  metadata.AcceptedDirectionCount,
		RejectedDirectionCount:  metadata.RejectedDirectionCount,
		ProviderLatencyMillis:   metadata.ProviderLatencyMillis,
		SurfaceDiscoveryRan:     metadata.SurfaceDiscoveryRan,
		SurfaceDiscoveryCount:   metadata.SurfaceDiscoveryCount,
		SurfaceDiscoveryMillis:  metadata.SurfaceDiscoveryMillis,
	}
	data.Warnings = append(data.Warnings, metadata.Warnings...)
	return ""
}

func parseSnapshot(path string, data *ReportData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("snapshot: %v", err)
	}
	var snap snapshotJSON
	if err := json.Unmarshal(b, &snap); err != nil {
		return fmt.Sprintf("snapshot unmarshal: %v", err)
	}
	data.RepoName = snap.RepoName
	if snap.GoFacts != nil {
		data.CommandTraces = append([]gofacts.CommandTrace(nil), snap.GoFacts.CommandTraces...)
	}
	if snap.GoFacts != nil && (len(snap.GoFacts.Modules) > 0 || len(snap.GoFacts.Packages) > 0) {
		graph := &RepositoryGraph{Version: 2, Packages: append([]PackageInfo(nil), snap.GoFacts.Packages...)}
		for _, module := range snap.GoFacts.Modules {
			dir := filepath.ToSlash(filepath.Clean(module.ModuleDir))
			if dir == "." {
				dir = ""
			}
			graph.Modules = append(graph.Modules, ModuleInfo{
				ID: module.ID, Path: module.ModulePath, Dir: dir, DisplayName: module.DisplayName,
			})
		}
		data.RepositoryGraph = graph
	}
	return ""
}

func repositoryGraphHasModule(modules []ModuleInfo, modulePath, moduleDir string) bool {
	for _, module := range modules {
		if module.Path == modulePath && module.Dir == moduleDir {
			return true
		}
	}
	return false
}

func parseOrientationReport(path string, data *ReportData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("orientation: %v", err)
	}
	var or orientationReportJSON
	if err := json.Unmarshal(b, &or); err != nil {
		return fmt.Sprintf("orientation unmarshal: %v", err)
	}
	data.ProjectGuess = or.ProjectGuess
	data.OrientationConfidence = or.Confidence
	for _, item := range or.HighLevelMap {
		role, _ := componentmap.Normalize(string(item.Role))
		data.HighLevelMap = append(data.HighLevelMap, Subsystem{
			Name:         item.Name,
			Role:         role,
			Evidence:     append([]string{}, item.Evidence...),
			WhyItMatters: item.WhyItMatters,
		})
	}
	for _, file := range or.FirstFilesToOpen {
		data.FirstFilesToOpen = append(data.FirstFilesToOpen, FileItem{
			Path:     file.Path,
			Reason:   file.Reason,
			Priority: file.Priority,
		})
	}
	for _, cf := range or.CandidateFlows {
		classified := flowexplain.CandidateFlow{
			Confidence: cf.Confidence, LocalVerification: cf.LocalVerification, LocalProof: cf.LocalProof,
			Disposition: cf.Disposition, DispositionReason: cf.DispositionReason,
		}
		if classified.Disposition == "" {
			flowexplain.ClassifyCandidateFlow(&classified)
		}
		data.CandidateFlows = append(data.CandidateFlows, cf.Name)
		data.CandidateDirections = append(data.CandidateDirections, CandidateDirection{
			ID:                flowexplain.GenerateFlowID(cf.Name),
			Name:              cf.Name,
			FlowType:          cf.FlowType,
			Trigger:           cf.Trigger,
			LikelyEntrypoint:  cf.LikelyEntrypoint,
			LikelyFiles:       append([]string{}, cf.LikelyFiles...),
			WhyInteresting:    cf.WhyInteresting,
			Evidence:          append([]string{}, cf.Evidence...),
			Confidence:        cf.Confidence,
			LocalVerification: cf.LocalVerification,
			LocalProof:        cf.LocalProof,
			Disposition:       classified.Disposition,
			DispositionReason: classified.DispositionReason,
		})
	}
	for _, word := range or.ImportantDomainWords {
		data.ImportantDomainWords = append(data.ImportantDomainWords, DomainWord{
			Word:     word.Word,
			Guess:    word.Guess,
			Evidence: append([]string{}, word.Evidence...),
		})
	}
	data.QuestionsForHuman = append(data.QuestionsForHuman, or.QuestionsForHuman...)
	for _, item := range or.UnverifiedPaths {
		data.OrientationUnverifiedPaths = append(data.OrientationUnverifiedPaths, PathItem{
			Path:   item.Path,
			Reason: item.Reason,
		})
	}
	data.Warnings = append(data.Warnings, or.Warnings...)
	return ""
}

func parseFlows(flowsDir string, data *ReportData) ([]string, error) {
	entries, err := os.ReadDir(flowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var warnings []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		fd := FlowData{ID: e.Name()}
		flowDir := filepath.Join(flowsDir, e.Name())

		if w := parseFlowBundle(filepath.Join(flowDir, "flow_bundle.json"), &fd); w != "" {
			warnings = append(warnings, fmt.Sprintf("flow %s: %s", fd.ID, w))
		}
		if w := parseFlowStatus(filepath.Join(flowDir, "flow_status.json"), &fd); w != "" {
			warnings = append(warnings, fmt.Sprintf("flow %s: %s", fd.ID, w))
		}
		if w := parseFlowReport(filepath.Join(flowDir, "flow_report.json"), &fd); w != "" {
			warnings = append(warnings, fmt.Sprintf("flow %s: %s", fd.ID, w))
		}

		data.Flows = append(data.Flows, fd)
	}
	return warnings, nil
}

func parseFlowStatus(path string, fd *FlowData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return fmt.Sprintf("status: %v", err)
	}
	var status flowStatusJSON
	if err := json.Unmarshal(b, &status); err != nil {
		return fmt.Sprintf("status unmarshal: %v", err)
	}
	if status.Version != 1 {
		return fmt.Sprintf("unsupported flow status version %d", status.Version)
	}
	switch status.Mode {
	case "local_only", "expansion_requested", "succeeded", "failed":
		fd.FlowStatus = status.Mode
		return ""
	default:
		return fmt.Sprintf("unsupported flow status mode %q", status.Mode)
	}
}

func parseFlowBundle(path string, fd *FlowData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("bundle: %v", err)
	}
	var fb flowexplain.FlowBundle
	if err := json.Unmarshal(b, &fb); err != nil {
		return fmt.Sprintf("bundle unmarshal: %v", err)
	}
	fd.BundleSummary.SelectedFilesCount = len(fb.SelectedFiles)
	fd.BundleSummary.SelectedTestsCount = len(fb.SelectedTests)
	fd.BundleSummary.SelectedDocsCount = len(fb.SelectedDocs)
	fd.BundleSummary.SelectedPkgsCount = len(fb.SelectedPackages)
	fd.BundleSummary.RelatedEdgesCount = len(fb.RelatedEdges)
	if fb.FlowSeed.Name != "" {
		fd.Name = fb.FlowSeed.Name
	}
	fd.FlowType = fb.FlowSeed.FlowType
	fd.BundleFiles = make([]FileItem, 0, len(fb.SelectedFiles))
	for _, sf := range fb.SelectedFiles {
		fd.BundleFiles = append(fd.BundleFiles, FileItem{Path: sf.Path, Reason: strings.Join(sf.Reasons, ", ")})
	}
	fd.BundleTests = make([]FileItem, 0, len(fb.SelectedTests))
	for _, st := range fb.SelectedTests {
		fd.BundleTests = append(fd.BundleTests, FileItem{Path: st.Path, Reason: strings.Join(st.Reasons, ", ")})
	}
	fd.BundleDocs = make([]FileItem, 0, len(fb.SelectedDocs))
	for _, sd := range fb.SelectedDocs {
		fd.BundleDocs = append(fd.BundleDocs, FileItem{Path: sd.Path, Reason: strings.Join(sd.Reasons, ", ")})
	}
	fd.BundlePackages = fb.SelectedPackages
	fd.BundleEdges = make([]EdgeInfo, 0, len(fb.RelatedEdges))
	for _, e := range fb.RelatedEdges {
		fd.BundleEdges = append(fd.BundleEdges, EdgeInfo{From: e.From, To: e.To})
	}
	return ""
}

func parseFlowReport(path string, fd *FlowData) string {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && fd.Name != "" {
			switch fd.FlowStatus {
			case "local_only":
				fd.EvidenceOnly = true
				return ""
			case "expansion_requested":
				fd.Error = "flow expansion was requested but no completed report was saved"
				return fd.Error
			case "succeeded":
				fd.Error = "flow status is succeeded but the report is missing"
				return fd.Error
			case "failed":
				fd.Error = "flow explanation failed; see error.txt in the run artifacts"
				return fd.Error
			}
			if _, modelErr := os.Stat(filepath.Join(filepath.Dir(path), "error.txt")); modelErr == nil {
				fd.Error = "flow explanation failed; see error.txt in the run artifacts"
				return fd.Error
			}
			fd.Error = "flow report is missing and no explicit flow status was saved"
			return fd.Error
		}
		fd.Error = fmt.Sprintf("cannot read flow report: %v", err)
		return fd.Error
	}
	if len(b) == 0 {
		fd.Error = "flow report is empty"
		return fd.Error
	}
	if fd.FlowStatus == "local_only" || fd.FlowStatus == "failed" || fd.FlowStatus == "expansion_requested" {
		fd.Error = fmt.Sprintf("flow status %q conflicts with a saved report", fd.FlowStatus)
		return fd.Error
	}
	var fr flowReportJSON
	if err := json.Unmarshal(b, &fr); err != nil {
		fd.Error = fmt.Sprintf("invalid flow report JSON: %v", err)
		return fd.Error
	}

	bundlePaths := buildBundlePathSet(fd)

	for _, fi := range fr.FilesToReadInOrder {
		fd.FilesToRead = append(fd.FilesToRead, FileItem{
			Path:     fi.Path,
			Reason:   fi.Reason,
			Priority: fi.Priority,
		})
		if fi.Path != "" && !bundlePaths[fi.Path] {
			fd.Warnings = append(fd.Warnings, fmt.Sprintf("unverified path in files_to_read_in_order: %s", fi.Path))
		}
	}
	for _, ti := range fr.TestsToRead {
		fd.TestsToRead = append(fd.TestsToRead, FileItem{
			Path:   ti.Path,
			Reason: ti.Reason,
		})
		if ti.Path != "" && !bundlePaths[ti.Path] {
			fd.Warnings = append(fd.Warnings, fmt.Sprintf("unverified path in tests_to_read: %s", ti.Path))
		}
	}
	for i, cs := range fr.LikelyChain {
		stepNum := int(cs.Step)
		if stepNum <= 0 {
			stepNum = i + 1
		}

		whatHappens := cs.WhatHappens
		if whatHappens == "" {
			whatHappens = cs.Description
		}
		if whatHappens == "" {
			whatHappens = cs.Reason
		}
		if whatHappens == "" && cs.Role != "" {
			if cs.Function != "" {
				whatHappens = cs.Role + ": " + cs.Function
			} else {
				whatHappens = cs.Role
			}
		}

		name := cs.Name
		if name == "" && cs.Role != "" {
			name = cs.Role
		}
		if name == "" && cs.Function != "" {
			name = cs.Function
		}
		if name == "" && cs.Description != "" {
			name = firstSentence(cs.Description)
		}

		// Fold File into evidence_files if not already present
		evidenceFiles := cs.EvidenceFiles
		if len(evidenceFiles) == 0 && cs.File != "" {
			evidenceFiles = []string{cs.File}
		}

		fd.LikelyChain = append(fd.LikelyChain, ChainStep{
			Step:          stepNum,
			Name:          name,
			WhatHappens:   whatHappens,
			EvidenceFiles: evidenceFiles,
			Confidence:    cs.Confidence,
		})
		for _, ef := range evidenceFiles {
			if ef != "" && !bundlePaths[ef] {
				fd.Warnings = append(fd.Warnings, fmt.Sprintf("unverified path in likely_chain evidence: %s", ef))
			}
		}
	}
	for _, up := range fr.UnverifiedPaths {
		fd.UnverifiedPaths = append(fd.UnverifiedPaths, PathItem{
			Path:   up.Path,
			Reason: up.Reason,
		})
	}
	if fr.FlowName != "" {
		fd.Name = fr.FlowName
	}
	fd.Summary = fr.Summary
	fd.Confidence = fr.Confidence
	fd.Unknowns = fr.Unknowns
	for _, w := range fr.Warnings {
		fd.Warnings = append(fd.Warnings, w)
	}
	return ""
}

func buildBundlePathSet(fd *FlowData) map[string]bool {
	paths := make(map[string]bool)
	for _, fi := range fd.BundleFiles {
		if fi.Path != "" {
			paths[fi.Path] = true
		}
	}
	for _, fi := range fd.BundleTests {
		if fi.Path != "" {
			paths[fi.Path] = true
		}
	}
	for _, fi := range fd.BundleDocs {
		if fi.Path != "" {
			paths[fi.Path] = true
		}
	}
	return paths
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	// Split on ". " that is followed by a capital letter or number
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			if i+1 < len(s) && s[i+1] == ' ' {
				if i+2 < len(s) && (s[i+2] >= 'A' && s[i+2] <= 'Z' || s[i+2] >= '1' && s[i+2] <= '9') {
					return s[:i+1]
				}
			}
		}
	}
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}
