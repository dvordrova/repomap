package report

import (
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/guidedtour"
)

var ErrNoGuidedTourCandidates = errors.New("guided tour has no eligible candidates")

const (
	GuidedStoryFile = "guided_story.json"

	maxGuidedStoryRecordBytes        = 128 << 10
	maxGuidedTourCandidates          = 12
	maxGuidedTourBeatsPerCandidate   = 32
	maxGuidedTourGapsPerCandidate    = 8
	maxGuidedTourEvidencePerBeat     = 4
	maxGuidedTourComponents          = 64
	maxGuidedTourNameBytes           = 240
	maxGuidedTourSummaryBytes        = 2 << 10
	maxGuidedTourDetailBytes         = 2 << 10
	maxGuidedTourDescriptionBytes    = 1 << 10
	maxGuidedTourEvidenceLabelBytes  = 2 << 10
	minGuidedTourCandidateExactBeats = 3
)

type guidedTourComponentIndex struct {
	components    map[string]ArchitectureComponent
	pathOwners    map[string]map[string]struct{}
	anchorOwners  map[string]map[string]struct{}
	packageOwners map[string]map[string]struct{}
}

type guidedTourFile struct {
	path   string
	reason string
}

// BuildGuidedTourBundle derives a bounded editor input from the already
// coherent architecture projection. It performs no repository analysis and
// never infers ownership from names or path similarity.
func BuildGuidedTourBundle(data *ReportData) (guidedtour.Bundle, error) {
	if data == nil {
		return guidedtour.Bundle{}, fmt.Errorf("guided tour build: report data is nil")
	}
	if data.ArchitectureCanvas == nil {
		return guidedtour.Bundle{}, fmt.Errorf("guided tour build: architecture canvas is unavailable")
	}
	if data.ArchitectureCanvas.Version <= 0 {
		return guidedtour.Bundle{}, fmt.Errorf("guided tour build: architecture canvas version is invalid")
	}
	repoName := boundGuidedTourText(data.RepoName, maxGuidedTourNameBytes)
	if repoName == "" {
		return guidedtour.Bundle{}, fmt.Errorf("guided tour build: repository name is empty")
	}

	openablePaths := stringSet(data.OpenablePaths)
	canvas := data.ArchitectureCanvas
	index := buildGuidedTourComponentIndex(*canvas, data.RepositoryGraph, data.OpenablePaths)
	surfaceIDs := make(map[string]struct{}, len(canvas.Surfaces))
	for _, surface := range canvas.Surfaces {
		if surface.ID != "" {
			surfaceIDs[surface.ID] = struct{}{}
		}
	}

	var candidates []guidedtour.Candidate
	referencedComponents := make(map[string]struct{})
	appendCandidate := func(candidate guidedtour.Candidate) error {
		if len(candidates) >= maxGuidedTourCandidates {
			return nil
		}
		candidateComponents := make(map[string]struct{})
		for _, beat := range candidate.Beats {
			for _, componentID := range beat.ComponentIDs {
				if _, exists := index.components[componentID]; !exists {
					return fmt.Errorf(
						"guided tour build: candidate %q references unknown component %q",
						candidate.ID,
						componentID,
					)
				}
				candidateComponents[componentID] = struct{}{}
			}
		}
		combinedCount := len(referencedComponents)
		for componentID := range candidateComponents {
			if _, exists := referencedComponents[componentID]; !exists {
				combinedCount++
			}
		}
		if combinedCount > maxGuidedTourComponents {
			return nil
		}
		candidates = append(candidates, candidate)
		for componentID := range candidateComponents {
			referencedComponents[componentID] = struct{}{}
		}
		return nil
	}

	flows := append([]ArchitectureFlow(nil), canvas.Flows...)
	sort.Slice(flows, func(i, j int) bool { return flows[i].ID < flows[j].ID })
	savedFlowIDs := make(map[string]struct{}, len(flows))
	for _, flow := range flows {
		savedFlowIDs[string(flow.ID)] = struct{}{}
		candidate := buildGuidedTourTraceCandidate(
			flow,
			*canvas,
			openablePaths,
			index.pathOwners,
			surfaceIDs,
		)
		if len(candidate.Beats) < minGuidedTourCandidateExactBeats {
			continue
		}
		if err := appendCandidate(candidate); err != nil {
			return guidedtour.Bundle{}, err
		}
	}

	suggestions := make(map[string]ArchitectureSuggestion, len(canvas.Suggestions))
	for _, suggestion := range canvas.Suggestions {
		suggestions[suggestion.ID] = suggestion
	}
	anchors := make(map[string]componentmap.BehaviorAnchor, len(canvas.BehaviorAnchors))
	for _, anchor := range canvas.BehaviorAnchors {
		anchors[anchor.ID] = anchor
	}
	flowFiles := indexGuidedTourFlowFiles(data.Flows)
	packageEdges := []EdgeInfo{}
	if data.RepositoryGraph != nil {
		packageEdges = data.RepositoryGraph.PackageEdges
	}
	for _, direction := range data.CandidateDirections {
		if len(candidates) >= maxGuidedTourCandidates {
			break
		}
		// Legacy saved runs predate explicit accepted dispositions. Coherence
		// already treats every non-rejected direction as eligible.
		if direction.Disposition == flowexplain.DirectionRejected {
			continue
		}
		if _, saved := savedFlowIDs[direction.ID]; saved {
			continue
		}
		suggestion, coherent := suggestions[direction.ID]
		if !coherent {
			continue
		}
		candidate := buildGuidedTourDirectionCandidate(
			direction,
			suggestion,
			anchors,
			flowFiles[direction.ID],
			packageEdges,
			openablePaths,
			index,
		)
		if len(candidate.Beats) < minGuidedTourCandidateExactBeats {
			continue
		}
		if err := appendCandidate(candidate); err != nil {
			return guidedtour.Bundle{}, err
		}
	}

	if len(candidates) == 0 {
		return guidedtour.Bundle{}, fmt.Errorf(
			"%w: no candidate has at least %d exact beats",
			ErrNoGuidedTourCandidates,
			minGuidedTourCandidateExactBeats,
		)
	}

	componentIDs := make([]string, 0, len(referencedComponents))
	for componentID := range referencedComponents {
		componentIDs = append(componentIDs, componentID)
	}
	sort.Strings(componentIDs)
	components := make([]guidedtour.Component, 0, len(componentIDs))
	for _, componentID := range componentIDs {
		component := index.components[componentID]
		name := boundGuidedTourText(firstNonEmpty(component.Name, componentID), maxGuidedTourNameBytes)
		description := boundGuidedTourText(
			firstNonEmpty(component.Description, component.Name, componentID),
			maxGuidedTourDescriptionBytes,
		)
		components = append(components, guidedtour.Component{
			ID: componentID, Name: name, Description: description,
		})
	}

	bundle := guidedtour.Bundle{
		Version:       guidedtour.BundleVersion,
		RepoName:      repoName,
		CanvasVersion: canvas.Version,
		Candidates:    candidates,
		Components:    components,
	}
	if _, _, err := guidedtour.BundleHash(bundle); err != nil {
		return guidedtour.Bundle{}, fmt.Errorf("guided tour build: %w", err)
	}
	return bundle, nil
}

func buildGuidedTourTraceCandidate(
	flow ArchitectureFlow,
	canvas ArchitectureCanvas,
	openablePaths map[string]struct{},
	pathOwners map[string]map[string]struct{},
	surfaceIDs map[string]struct{},
) guidedtour.Candidate {
	edgesByTarget := make(map[string][]ArchitectureFlowEdge)
	for _, edge := range canvas.FlowEdges {
		if edge.FlowID != flow.ID {
			continue
		}
		edgesByTarget[edge.To] = append(edgesByTarget[edge.To], edge)
	}
	for target := range edgesByTarget {
		sort.Slice(edgesByTarget[target], func(i, j int) bool {
			return edgesByTarget[target][i].ID < edgesByTarget[target][j].ID
		})
	}

	beats := make([]guidedtour.Beat, 0, min(len(flow.Steps), maxGuidedTourBeatsPerCandidate))
	for _, step := range flow.Steps {
		if len(beats) >= maxGuidedTourBeatsPerCandidate {
			break
		}
		evidenceRefs := guidedTourStepEvidence(
			flow.ID,
			step,
			edgesByTarget[step.ID],
			openablePaths,
			pathOwners,
		)
		if len(evidenceRefs) == 0 {
			continue
		}
		componentIDs := []string{}
		if step.ComponentID != "" {
			componentIDs = append(componentIDs, string(step.ComponentID))
		}
		beatSurfaceIDs := []string{}
		if len(beats) == 0 {
			beatSurfaceIDs = appendExistingSurfaceIDs(
				beatSurfaceIDs,
				surfaceIDs,
				append([]string{flow.StartSurfaceID}, flow.TraceEvidenceSurfaceIDs...)...,
			)
		}
		beats = append(beats, guidedtour.Beat{
			ID:           stableReportID("guided-beat", "saved-trace", string(flow.ID), step.ID),
			Kind:         string(step.Kind),
			Label:        boundGuidedTourText(step.Label, maxGuidedTourNameBytes),
			Detail:       boundGuidedTourText(firstNonEmpty(step.QualifiedName, step.Label), maxGuidedTourDetailBytes),
			Sequence:     len(beats),
			ComponentIDs: componentIDs,
			SurfaceIDs:   beatSurfaceIDs,
			FlowID:       string(flow.ID),
			FlowStepIDs:  []string{step.ID},
			Evidence:     evidenceRefs,
		})
	}

	gaps := make([]guidedtour.Gap, 0, maxGuidedTourGapsPerCandidate)
	for _, frontier := range canvas.Frontiers {
		if frontier.FlowID != flow.ID || len(gaps) >= maxGuidedTourGapsPerCandidate {
			continue
		}
		evidenceRefs := []guidedtour.EvidenceRef{}
		if guidedTourLocationAvailable(frontier.Evidence, openablePaths, pathOwners) {
			evidenceRefs = append(evidenceRefs, guidedtour.EvidenceRef{
				ID:       stableReportID("guided-evidence", "frontier", frontier.ID),
				Kind:     "frontier",
				Label:    boundGuidedTourText(frontier.Reason, maxGuidedTourEvidenceLabelBytes),
				Location: cloneArchitectureLocation(frontier.Evidence),
			})
		}
		gaps = append(gaps, guidedtour.Gap{
			ID: frontier.ID,
			Label: boundGuidedTourText(
				firstNonEmpty(string(frontier.Slot), frontier.Kind, "Unresolved frontier"),
				maxGuidedTourNameBytes,
			),
			Detail:   boundGuidedTourText(frontier.Reason, maxGuidedTourDetailBytes),
			Evidence: evidenceRefs,
		})
	}

	return guidedtour.Candidate{
		ID:            string(flow.ID),
		Name:          boundGuidedTourText(firstNonEmpty(flow.Name, string(flow.ID)), maxGuidedTourNameBytes),
		Kind:          guidedtour.CandidateSavedTrace,
		Trigger:       boundGuidedTourText(firstNonEmpty(flow.Trigger, flow.Command, flow.Goal, flow.Name, string(flow.ID)), maxGuidedTourSummaryBytes),
		Summary:       boundGuidedTourText(firstNonEmpty(flow.MentalModel, flow.Goal, flow.WhyInspect, flow.Name, string(flow.ID)), maxGuidedTourSummaryBytes),
		OrderingBasis: guidedtour.OrderingTrace,
		Beats:         beats,
		Gaps:          gaps,
	}
}

func guidedTourStepEvidence(
	flowID componentmap.FlowID,
	step ArchitectureFlowStep,
	incoming []ArchitectureFlowEdge,
	openablePaths map[string]struct{},
	pathOwners map[string]map[string]struct{},
) []guidedtour.EvidenceRef {
	result := make([]guidedtour.EvidenceRef, 0, maxGuidedTourEvidencePerBeat)
	seen := make(map[string]struct{})
	add := func(id, kind, label string, location *evidence.Location) {
		if len(result) >= maxGuidedTourEvidencePerBeat || !guidedTourLocationAvailable(location, openablePaths, pathOwners) {
			return
		}
		if _, duplicate := seen[id]; duplicate {
			return
		}
		seen[id] = struct{}{}
		result = append(result, guidedtour.EvidenceRef{
			ID: id, Kind: kind,
			Label:    boundGuidedTourText(label, maxGuidedTourEvidenceLabelBytes),
			Location: cloneArchitectureLocation(location),
		})
	}
	add(
		stableReportID("guided-evidence", "flow-step", string(flowID), step.ID, "anchor"),
		"flow_step",
		step.Label,
		step.Location,
	)
	if step.Binding != nil {
		add(
			stableReportID("guided-evidence", "flow-step", string(flowID), step.ID, "binding"),
			"component_binding",
			step.Label,
			step.Binding.Location,
		)
	}
	for _, edge := range incoming {
		location := edge.Evidence
		add(edge.ID, "flow_transition", string(edge.Relation), &location)
	}
	return result
}

func buildGuidedTourDirectionCandidate(
	direction CandidateDirection,
	suggestion ArchitectureSuggestion,
	anchors map[string]componentmap.BehaviorAnchor,
	flowFiles []guidedTourFile,
	packageEdges []EdgeInfo,
	openablePaths map[string]struct{},
	index guidedTourComponentIndex,
) guidedtour.Candidate {
	beats := make([]guidedtour.Beat, 0, maxGuidedTourBeatsPerCandidate)
	for _, anchorID := range suggestion.RelevantAnchorIDs {
		if len(beats) >= maxGuidedTourBeatsPerCandidate {
			break
		}
		anchor, exists := anchors[anchorID]
		if !exists || !guidedTourLocationAvailable(&anchor.Location, openablePaths, index.pathOwners) {
			continue
		}
		components := sortedStringSet(index.anchorOwners[anchorID])
		location := anchor.Location
		beats = append(beats, guidedtour.Beat{
			ID:           stableReportID("guided-beat", "direction-anchor", direction.ID, anchor.ID),
			Kind:         string(anchor.Kind),
			Label:        boundGuidedTourText(anchor.Label, maxGuidedTourNameBytes),
			Detail:       boundGuidedTourText(anchor.Label, maxGuidedTourDetailBytes),
			Sequence:     len(beats),
			ComponentIDs: components,
			SurfaceIDs:   []string{},
			FlowStepIDs:  []string{},
			Evidence: []guidedtour.EvidenceRef{{
				ID: anchor.ID, Kind: "architecture_anchor",
				Label:    boundGuidedTourText(anchor.Label, maxGuidedTourEvidenceLabelBytes),
				Location: &location,
			}},
		})
	}
	files := collectGuidedTourDirectionFiles(direction, flowFiles)
	for _, file := range files {
		if len(beats) >= maxGuidedTourBeatsPerCandidate {
			break
		}
		owners := index.pathOwners[file.path]
		_, openable := openablePaths[file.path]
		if !openable && len(owners) == 0 {
			continue
		}
		if len(owners) != 1 {
			continue
		}
		componentIDs := sortedStringSet(owners)
		location := evidence.Location{Path: file.path}
		beats = append(beats, guidedtour.Beat{
			ID:           stableReportID("guided-beat", "direction-file", direction.ID, file.path),
			Kind:         "file",
			Label:        boundGuidedTourText(path.Base(file.path), maxGuidedTourNameBytes),
			Detail:       boundGuidedTourText(firstNonEmpty(file.reason, "Allowlisted file from the accepted direction."), maxGuidedTourDetailBytes),
			Sequence:     len(beats),
			ComponentIDs: componentIDs,
			SurfaceIDs:   []string{},
			FlowStepIDs:  []string{},
			Evidence: []guidedtour.EvidenceRef{{
				ID:   stableReportID("guided-evidence", "direction-file", direction.ID, file.path),
				Kind: "file", Label: boundGuidedTourText(file.path, maxGuidedTourEvidenceLabelBytes),
				Location: &location,
			}},
		})
	}
	beats = appendGuidedTourPackageImportBeats(
		beats,
		direction.ID,
		packageEdges,
		index.packageOwners,
	)

	gapDetail := "The supplied anchors and files are an editorial reading order; transitions and runtime execution between them are not proven."
	if reason := boundGuidedTourText(suggestion.TraceUnavailableReason, maxGuidedTourSummaryBytes); reason != "" {
		gapDetail += " " + reason
	}
	gaps := []guidedtour.Gap{{
		ID:       stableReportID("guided-gap", "direction-order", direction.ID),
		Label:    "Runtime order is unproven",
		Detail:   boundGuidedTourText(gapDetail, maxGuidedTourDetailBytes),
		Evidence: []guidedtour.EvidenceRef{},
	}}

	return guidedtour.Candidate{
		ID:            direction.ID,
		Name:          boundGuidedTourText(firstNonEmpty(direction.Name, direction.ID), maxGuidedTourNameBytes),
		Kind:          guidedtour.CandidateSuggestedDirection,
		Trigger:       boundGuidedTourText(firstNonEmpty(direction.Trigger, direction.Name, direction.ID), maxGuidedTourSummaryBytes),
		Summary:       boundGuidedTourText(firstNonEmpty(direction.WhyInteresting, direction.DispositionReason, direction.Name, direction.ID), maxGuidedTourSummaryBytes),
		OrderingBasis: guidedtour.OrderingEditorial,
		Beats:         beats,
		Gaps:          gaps,
	}
}

func appendGuidedTourPackageImportBeats(
	beats []guidedtour.Beat,
	directionID string,
	edges []EdgeInfo,
	packageOwners map[string]map[string]struct{},
) []guidedtour.Beat {
	candidateComponents := make(map[string]struct{})
	for _, beat := range beats {
		for _, componentID := range beat.ComponentIDs {
			candidateComponents[componentID] = struct{}{}
		}
	}

	edgesByKey := make(map[string]EdgeInfo, len(edges))
	for _, edge := range edges {
		if edge.From == "" || edge.To == "" || edge.From == edge.To {
			continue
		}
		edgesByKey[edge.From+"\x00"+edge.To] = edge
	}
	edgeKeys := make([]string, 0, len(edgesByKey))
	for key := range edgesByKey {
		edgeKeys = append(edgeKeys, key)
	}
	sort.Strings(edgeKeys)

	for _, key := range edgeKeys {
		if len(beats) >= maxGuidedTourBeatsPerCandidate {
			break
		}
		edge := edgesByKey[key]
		fromOwners := sortedStringSet(packageOwners[edge.From])
		toOwners := sortedStringSet(packageOwners[edge.To])
		if len(fromOwners) != 1 || len(toOwners) != 1 || fromOwners[0] == toOwners[0] {
			continue
		}
		if _, present := candidateComponents[fromOwners[0]]; !present {
			continue
		}
		if _, present := candidateComponents[toOwners[0]]; !present {
			continue
		}

		detail := fmt.Sprintf(
			"Static package import (not a runtime call or execution order): %s imports %s according to saved repository facts.",
			edge.From,
			edge.To,
		)
		evidenceID := stableReportID(
			"guided-evidence",
			"package-import",
			directionID,
			edge.From,
			edge.To,
		)
		beats = append(beats, guidedtour.Beat{
			ID: stableReportID(
				"guided-beat",
				"package-import",
				directionID,
				edge.From,
				edge.To,
			),
			Kind:         "package_import",
			Label:        "Static package import (not a runtime call)",
			Detail:       boundGuidedTourText(detail, maxGuidedTourDetailBytes),
			Sequence:     len(beats),
			ComponentIDs: []string{fromOwners[0], toOwners[0]},
			SurfaceIDs:   []string{},
			FlowStepIDs:  []string{},
			Evidence: []guidedtour.EvidenceRef{{
				ID:       evidenceID,
				Kind:     "package_import",
				Label:    boundGuidedTourText(detail, maxGuidedTourEvidenceLabelBytes),
				Location: nil,
			}},
		})
	}
	return beats
}

func buildGuidedTourComponentIndex(
	canvas ArchitectureCanvas,
	graph *RepositoryGraph,
	openablePaths []string,
) guidedTourComponentIndex {
	index := guidedTourComponentIndex{
		components:    make(map[string]ArchitectureComponent, len(canvas.Components)),
		pathOwners:    make(map[string]map[string]struct{}),
		anchorOwners:  make(map[string]map[string]struct{}),
		packageOwners: make(map[string]map[string]struct{}),
	}
	packageFiles := make(map[string][]string)
	if graph != nil {
		for _, pkg := range graph.Packages {
			if pkg.CanonicalPath == "" {
				continue
			}
			packageFiles[pkg.CanonicalPath] = append(
				packageFiles[pkg.CanonicalPath],
				pkg.Files...,
			)
		}
		for _, filePath := range openablePaths {
			packagePath := guidedTourPackageForFile(graph.Modules, filePath)
			if packagePath != "" {
				packageFiles[packagePath] = append(packageFiles[packagePath], filePath)
			}
		}
	}
	for _, component := range canvas.Components {
		componentID := string(component.ID)
		index.components[componentID] = component
		for _, anchorID := range component.AnchorIDs {
			addStringOwner(index.anchorOwners, anchorID, componentID)
		}
		for _, member := range component.Members {
			for _, fact := range member.Facts {
				if fact.Location != nil && validGuidedTourPath(fact.Location.Path) {
					addStringOwner(index.pathOwners, fact.Location.Path, componentID)
				}
				if fact.Kind == componentmap.FactRepositoryPath && validGuidedTourPath(fact.Value) {
					addStringOwner(index.pathOwners, fact.Value, componentID)
				}
				if fact.Kind == componentmap.FactDeclaration {
					addStringOwner(index.packageOwners, fact.Value, componentID)
					for _, filePath := range packageFiles[fact.Value] {
						if validGuidedTourPath(filePath) {
							addStringOwner(index.pathOwners, filePath, componentID)
						}
					}
				}
			}
		}
	}
	return index
}

func guidedTourPackageForFile(modules []ModuleInfo, filePath string) string {
	if !validGuidedTourPath(filePath) {
		return ""
	}
	filePath = path.Clean(filePath)
	if filePath == "." || filePath == ".." || strings.HasPrefix(filePath, "../") || path.IsAbs(filePath) {
		return ""
	}
	directory := path.Dir(filePath)
	if directory == "." {
		directory = ""
	}
	bestIndex := -1
	bestLength := -1
	for index, module := range modules {
		moduleDir := strings.Trim(path.Clean(module.Dir), "/")
		if moduleDir == "." {
			moduleDir = ""
		}
		if module.Path == "" || (moduleDir != "" && directory != moduleDir && !strings.HasPrefix(directory, moduleDir+"/")) {
			continue
		}
		if len(moduleDir) > bestLength {
			bestIndex = index
			bestLength = len(moduleDir)
		}
	}
	if bestIndex < 0 {
		return ""
	}
	module := modules[bestIndex]
	moduleDir := strings.Trim(path.Clean(module.Dir), "/")
	if moduleDir == "." {
		moduleDir = ""
	}
	relative := directory
	if moduleDir != "" {
		relative = strings.TrimPrefix(directory, moduleDir)
		relative = strings.TrimPrefix(relative, "/")
	}
	if relative == "" {
		return module.Path
	}
	return strings.TrimSuffix(module.Path, "/") + "/" + relative
}

func indexGuidedTourFlowFiles(flows []FlowData) map[string][]guidedTourFile {
	result := make(map[string][]guidedTourFile)
	for _, flow := range flows {
		files := make([]guidedTourFile, 0, len(flow.BundleFiles)+len(flow.FilesToRead))
		for _, file := range flow.BundleFiles {
			files = append(files, guidedTourFile{path: file.Path, reason: file.Reason})
		}
		for _, file := range flow.FilesToRead {
			files = append(files, guidedTourFile{path: file.Path, reason: file.Reason})
		}
		result[flow.ID] = append(result[flow.ID], files...)
	}
	return result
}

func collectGuidedTourDirectionFiles(
	direction CandidateDirection,
	flowFiles []guidedTourFile,
) []guidedTourFile {
	values := make([]guidedTourFile, 0, len(direction.LikelyFiles)+len(flowFiles))
	for _, filePath := range direction.LikelyFiles {
		values = append(values, guidedTourFile{path: filePath})
	}
	values = append(values, flowFiles...)
	result := make([]guidedTourFile, 0, len(values))
	indexes := make(map[string]int)
	for _, value := range values {
		if !validGuidedTourPath(value.path) {
			continue
		}
		if index, duplicate := indexes[value.path]; duplicate {
			if result[index].reason == "" && value.reason != "" {
				result[index].reason = value.reason
			}
			continue
		}
		indexes[value.path] = len(result)
		result = append(result, value)
	}
	return result
}

func guidedTourLocationAvailable(
	location *evidence.Location,
	openablePaths map[string]struct{},
	pathOwners map[string]map[string]struct{},
) bool {
	if location == nil || !validGuidedTourPath(location.Path) {
		return false
	}
	if _, openable := openablePaths[location.Path]; openable {
		return true
	}
	return len(pathOwners[location.Path]) > 0
}

func appendExistingSurfaceIDs(
	result []string,
	known map[string]struct{},
	values ...string,
) []string {
	seen := stringSet(result)
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := known[value]; !exists {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) > maxGuidedTourEvidencePerBeat {
		result = result[:maxGuidedTourEvidencePerBeat]
	}
	return result
}

func addStringOwner(index map[string]map[string]struct{}, key, owner string) {
	if key == "" || owner == "" {
		return
	}
	if index[key] == nil {
		index[key] = make(map[string]struct{})
	}
	index[key][owner] = struct{}{}
}

func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func validGuidedTourPath(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || path.IsAbs(value) ||
		value == "." || value == ".." || strings.HasPrefix(value, "../") ||
		path.Clean(value) != value || strings.Contains(value, `\`) {
		return false
	}
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	return true
}

func boundGuidedTourText(value string, limit int) string {
	var normalized strings.Builder
	for _, char := range value {
		if char < 0x20 || char == 0x7f {
			normalized.WriteByte(' ')
			continue
		}
		normalized.WriteRune(char)
	}
	value = strings.Join(strings.Fields(normalized.String()), " ")
	if len(value) <= limit {
		return value
	}
	end := limit
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return strings.TrimSpace(value[:end])
}

func replaySavedGuidedTour(data *ReportData, storyPath string) string {
	info, err := os.Lstat(storyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		return "guided tour: cannot inspect saved story"
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxGuidedStoryRecordBytes {
		return "guided tour: saved story is not a bounded regular file"
	}
	raw, err := os.ReadFile(storyPath)
	if err != nil {
		return "guided tour: cannot read saved story"
	}
	bundle, err := BuildGuidedTourBundle(data)
	if err != nil {
		return fmt.Sprintf("guided tour: cannot rebuild saved story bundle: %v", err)
	}
	story, err := guidedtour.ReplayRecord(bundle, raw)
	if err != nil {
		return fmt.Sprintf("guided tour: saved story cannot be replayed: %v", err)
	}
	data.GuidedTour = &story
	return ""
}
