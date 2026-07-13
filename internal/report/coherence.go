package report

import (
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/flowexplain"
	"github.com/dvordrova/repomap/internal/flowproof"
)

const (
	surfaceSourceCatalog    = "deterministic_catalog"
	surfaceSourceTraceStart = "saved_trace_start"

	surfaceCategoryApplication = "application"
	surfaceCategoryService     = "secondary_service"
	surfaceCategoryTooling     = "tooling"
	surfaceCategoryTests       = "tests_helpers"
	surfaceCategoryUnassigned  = "unassigned"
	surfaceCategoryDynamic     = "dynamic_unresolved"
	surfaceCategoryUnavailable = "unavailable"
)

type architectureOwnershipIndex struct {
	pathOwners      map[string]map[componentmap.ComponentID]struct{}
	packageOwners   map[string]map[componentmap.ComponentID]struct{}
	symbolOwners    map[string]map[componentmap.ComponentID]struct{}
	memberOwners    map[string]map[componentmap.ComponentID]struct{}
	sourceLocations map[string][]SurfaceLocation
}

// linkArchitectureProductObjects derives presentation-only joins from exact
// saved IDs, membership, package identities, and source locations. Ambiguous
// evidence remains unassigned instead of being guessed from display names.
func linkArchitectureProductObjects(data *ReportData) {
	if data == nil {
		return
	}
	if data.DiscoveredSurfaces != nil {
		for index := range data.DiscoveredSurfaces.Triggers {
			trigger := &data.DiscoveredSurfaces.Triggers[index]
			classifySurfaceExecutable(data, trigger)
			ensureProjectedSurfaceSemantics(trigger)
		}
		refreshSurfaceCatalogCounts(data.DiscoveredSurfaces)
	}
	if data.ArchitectureCanvas == nil {
		return
	}
	canvas := data.ArchitectureCanvas
	canvas.Surfaces = nil
	canvas.Suggestions = nil
	for index := range canvas.Components {
		canvas.Components[index].OwnedSurfaceIDs = nil
		canvas.Components[index].ParticipatingSurfaceIDs = nil
		canvas.Components[index].SuggestedInvestigationIDs = nil
	}
	for index := range canvas.Flows {
		canvas.Flows[index].Status = ""
		canvas.Flows[index].EvidenceBasis = ""
		canvas.Flows[index].WhyInspect = ""
		canvas.Flows[index].GroundedAreas = 0
		canvas.Flows[index].TotalAreas = 0
		canvas.Flows[index].FrontierSummary = ""
		canvas.Flows[index].ParticipatingComponentIDs = nil
		canvas.Flows[index].StartSurfaceID = ""
	}
	owners := buildArchitectureOwnershipIndex(*canvas, data.RepositoryGraph)
	flowLocations := architectureFlowLocations(*canvas)
	flowByLocation := make(map[string]map[componentmap.FlowID]struct{})
	flowByCommand := make(map[string]map[componentmap.FlowID]struct{})
	for flowID, locations := range flowLocations {
		for location := range locations {
			addFlowSet(flowByLocation, location, flowID)
		}
	}
	for _, flow := range canvas.Flows {
		command := strings.TrimSpace(strings.ToLower(flow.Command))
		if command != "" {
			addFlowSet(flowByCommand, command, flow.ID)
		}
	}

	componentIndex := make(map[componentmap.ComponentID]int, len(canvas.Components))
	for index := range canvas.Components {
		componentIndex[canvas.Components[index].ID] = index
	}
	flowIndex := make(map[componentmap.FlowID]int, len(canvas.Flows))
	for index := range canvas.Flows {
		flowIndex[canvas.Flows[index].ID] = index
		populateArchitectureTraceSummary(data, &canvas.Flows[index], *canvas)
	}

	if data.DiscoveredSurfaces != nil {
		for index := range data.DiscoveredSurfaces.Triggers {
			trigger := &data.DiscoveredSurfaces.Triggers[index]
			surface := architectureSurfaceFromTrigger(*trigger, owners, flowByLocation, flowByCommand, flowLocations)
			for _, flow := range canvas.Flows {
				if flow.SeedSurfaceID == trigger.ID {
					surface.RelatedTraceID = flow.ID
					surface.TraceUnavailableReason = ""
					break
				}
			}
			trigger.OwningExecutable = surface.OwningExecutable
			trigger.OwningComponentID = surface.OwningComponentID
			trigger.ParticipatingComponentIDs = append([]componentmap.ComponentID(nil), surface.ParticipatingComponentIDs...)
			trigger.RelatedTraceID = surface.RelatedTraceID
			trigger.TraceUnavailableReason = surface.TraceUnavailableReason
			canvas.Surfaces = append(canvas.Surfaces, surface)
			attachArchitectureSurface(canvas, componentIndex, surface)
			if surface.RelatedTraceID != "" {
				if index, ok := flowIndex[surface.RelatedTraceID]; ok &&
					(canvas.Flows[index].StartSurfaceID == "" || canvas.Flows[index].SeedSurfaceID == surface.ID) {
					canvas.Flows[index].StartSurfaceID = surface.ID
				}
			}
		}
	}

	linkSuggestedInvestigations(data, owners, componentIndex)
	refreshSurfaceCatalogCounts(data.DiscoveredSurfaces)
	sort.Slice(canvas.Surfaces, func(i, j int) bool { return canvas.Surfaces[i].ID < canvas.Surfaces[j].ID })
	for index := range canvas.Components {
		sort.Strings(canvas.Components[index].OwnedSurfaceIDs)
		sort.Strings(canvas.Components[index].ParticipatingSurfaceIDs)
		sort.Strings(canvas.Components[index].SuggestedInvestigationIDs)
	}
}

// ApplyProductCoherence upgrades a loaded saved report with presentation-only
// component/surface/trace joins. Callers must invoke it before publishing the
// report to concurrent readers; it mutates the supplied projection.
func ApplyProductCoherence(data *ReportData) {
	if data != nil && data.ArchitectureCanvas == nil {
		if input, err := BuildArchitectureCanvasInput(data); err == nil {
			if canvas, projectErr := ProjectArchitectureCanvas(input); projectErr == nil {
				data.ArchitectureCanvas = &canvas
			}
		}
	}
	linkArchitectureProductObjects(data)
	refreshProductCounts(data)
}

func buildArchitectureOwnershipIndex(canvas ArchitectureCanvas, graph *RepositoryGraph) architectureOwnershipIndex {
	index := architectureOwnershipIndex{
		pathOwners:      make(map[string]map[componentmap.ComponentID]struct{}),
		packageOwners:   make(map[string]map[componentmap.ComponentID]struct{}),
		symbolOwners:    make(map[string]map[componentmap.ComponentID]struct{}),
		memberOwners:    make(map[string]map[componentmap.ComponentID]struct{}),
		sourceLocations: make(map[string][]SurfaceLocation),
	}
	diagnosticComponents := make(map[componentmap.ComponentID]struct{})
	for _, subsystem := range canvas.Subsystems {
		if subsystem.Category != componentmap.SubsystemCategoryDiagnostic {
			continue
		}
		for _, componentID := range subsystem.ComponentIDs {
			diagnosticComponents[componentID] = struct{}{}
		}
	}
	for _, component := range canvas.Components {
		if _, diagnostic := diagnosticComponents[component.ID]; diagnostic {
			continue
		}
		for _, member := range component.Members {
			addComponentSet(index.memberOwners, member.ID.Value, component.ID)
			switch member.ID.Kind {
			case componentmap.MemberPackage:
				addComponentSet(index.packageOwners, member.ID.Value, component.ID)
			case componentmap.MemberSymbol, componentmap.MemberEntrypoint:
				addComponentSet(index.symbolOwners, member.ID.Value, component.ID)
			case componentmap.MemberFile:
				memberPath := cleanSurfacePath(member.ID.Value)
				addComponentSet(index.pathOwners, memberPath, component.ID)
				addArchitectureSourceLocation(&index, member.ID.Value, SurfaceLocation{Path: memberPath})
			}
			for _, fact := range member.Facts {
				if fact.Location != nil {
					location := SurfaceLocation{
						Path: cleanSurfacePath(fact.Location.Path), Line: fact.Location.Line, Column: fact.Location.Column,
					}
					addComponentSet(index.pathOwners, location.Path, component.ID)
					addArchitectureSourceLocation(&index, member.ID.Value, location)
					addArchitectureSourceLocation(&index, fact.Value, location)
					addArchitectureSourceLocation(&index, location.Path, location)
				}
				if fact.Kind == componentmap.FactRepositoryPath {
					factPath := cleanSurfacePath(fact.Value)
					addComponentSet(index.pathOwners, factPath, component.ID)
					addArchitectureSourceLocation(&index, factPath, SurfaceLocation{Path: factPath})
				}
				if fact.Kind == componentmap.FactDeclaration {
					addComponentSet(index.memberOwners, fact.Value, component.ID)
					for _, pkg := range exactRepositoryPackages(graph, fact.Value) {
						addComponentSet(index.packageOwners, pkg.CanonicalPath, component.ID)
						addComponentSet(index.pathOwners, cleanSurfacePath(pkg.Dir), component.ID)
						for _, filePath := range pkg.Files {
							filePath = cleanSurfacePath(filePath)
							addComponentSet(index.pathOwners, filePath, component.ID)
							location := SurfaceLocation{Path: filePath}
							addArchitectureSourceLocation(&index, pkg.CanonicalPath, location)
							addArchitectureSourceLocation(&index, pkg.Dir, location)
							addArchitectureSourceLocation(&index, filePath, location)
							addArchitectureSourceLocation(&index, member.ID.Value, location)
						}
					}
				}
			}
		}
	}
	return index
}

func exactRepositoryPackages(graph *RepositoryGraph, canonicalPath string) []PackageInfo {
	if graph == nil || strings.TrimSpace(canonicalPath) == "" {
		return nil
	}
	result := make([]PackageInfo, 0, 1)
	for _, pkg := range graph.Packages {
		if pkg.CanonicalPath == canonicalPath {
			result = append(result, pkg)
		}
	}
	return result
}

func architectureFlowLocations(canvas ArchitectureCanvas) map[componentmap.FlowID]map[string]struct{} {
	result := make(map[componentmap.FlowID]map[string]struct{}, len(canvas.Flows))
	for _, flow := range canvas.Flows {
		locations := make(map[string]struct{})
		for _, step := range flow.Steps {
			if step.Location != nil {
				locations[surfaceLocationKey(step.Location.Path, step.Location.Line)] = struct{}{}
			}
			if step.Binding != nil {
				locations[surfaceLocationKey(step.Binding.Location.Path, step.Binding.Location.Line)] = struct{}{}
			}
		}
		for _, edge := range canvas.FlowEdges {
			if edge.FlowID == flow.ID {
				locations[surfaceLocationKey(edge.Evidence.Path, edge.Evidence.Line)] = struct{}{}
			}
		}
		result[flow.ID] = locations
	}
	return result
}

func populateArchitectureTraceSummary(data *ReportData, flow *ArchitectureFlow, canvas ArchitectureCanvas) {
	if flow == nil {
		return
	}
	flow.EvidenceBasis = "static"
	if flow.TraceQuality == "" {
		flow.TraceQuality = flowproof.TraceQualityPartial
	}
	flow.TotalAreas = len(flow.Slots)
	for _, slot := range flow.Slots {
		if slot.Status == flowproof.SlotVerified || slot.Status == flowproof.SlotNotApplicable {
			flow.GroundedAreas++
		}
	}
	switch {
	case flow.TotalAreas > 0 && flow.GroundedAreas == flow.TotalAreas:
		flow.Status = "complete"
	case flow.GroundedAreas > 0:
		flow.Status = "partial"
	default:
		flow.Status = "unresolved"
	}
	if flow.Status == "complete" {
		flow.TraceQuality = flowproof.TraceQualityComplete
	}
	participants := make(map[componentmap.ComponentID]struct{})
	for _, step := range flow.Steps {
		if step.ComponentID != "" {
			participants[step.ComponentID] = struct{}{}
		}
	}
	flow.ParticipatingComponentIDs = sortedComponentIDs(participants)
	flow.FrontierSummary = strings.TrimSpace(flow.CurrentFrontier)
	if flow.FrontierSummary == "" {
		for _, kind := range []flowproof.SlotKind{
			flowproof.SlotIOBoundary,
			flowproof.SlotCoreOperation,
			flowproof.SlotTermination,
			flowproof.SlotConcurrency,
		} {
			for _, slot := range flow.Slots {
				if slot.Kind == kind && strings.TrimSpace(slot.Missing) != "" {
					flow.FrontierSummary = slot.Missing
					break
				}
			}
			if flow.FrontierSummary != "" {
				break
			}
		}
	}
	for _, frontier := range canvas.Frontiers {
		if flow.FrontierSummary == "" && frontier.FlowID == flow.ID && strings.TrimSpace(frontier.Reason) != "" {
			flow.FrontierSummary = frontier.Reason
			break
		}
	}
	for _, direction := range data.CandidateDirections {
		if componentmap.FlowID(direction.ID) == flow.ID {
			flow.WhyInspect = direction.WhyInteresting
			break
		}
	}
	if flow.WhyInspect == "" {
		flow.WhyInspect = firstNonEmpty(flow.MentalModel, flow.Goal, "Inspect the exact static handoffs and current analysis frontier.")
	}
}

func architectureSurfaceFromTrigger(
	trigger DiscoveredTrigger,
	owners architectureOwnershipIndex,
	flowByLocation map[string]map[componentmap.FlowID]struct{},
	flowByCommand map[string]map[componentmap.FlowID]struct{},
	flowLocations map[componentmap.FlowID]map[string]struct{},
) ArchitectureSurface {
	participants := make(map[componentmap.ComponentID]struct{})
	addOwners := func(values map[componentmap.ComponentID]struct{}) {
		for id := range values {
			participants[id] = struct{}{}
		}
	}
	evidence := surfaceTriggerLocations(trigger)
	for _, location := range evidence {
		addOwners(ownersForPath(owners.pathOwners, location.Path))
	}
	addOwners(owners.packageOwners[trigger.ProcessEntrypoint.Package])
	addOwners(owners.symbolOwners[trigger.ProcessEntrypoint.ID])
	addOwners(owners.symbolOwners[trigger.ProcessEntrypoint.Name])
	if trigger.Handler.Known {
		addOwners(owners.symbolOwners[trigger.Handler.Text])
	}

	primary := uniqueOwnerForTrigger(trigger, owners)
	related := make(map[componentmap.FlowID]struct{})
	for _, location := range evidence {
		for flowID := range flowByLocation[surfaceLocationKey(location.Path, location.Line)] {
			related[flowID] = struct{}{}
		}
	}
	if trigger.Kind == "cli_command" {
		command := strings.TrimSpace(strings.ToLower(trigger.Identity.Name))
		commandMatches := flowByCommand[command]
		typedMatches := make(map[componentmap.FlowID]struct{})
		for flowID := range commandMatches {
			if commandSurfaceMatchesFlowExecutable(trigger, flowLocations[flowID]) {
				typedMatches[flowID] = struct{}{}
			}
		}
		related = make(map[componentmap.FlowID]struct{})
		if len(typedMatches) == 1 {
			for flowID := range typedMatches {
				related[flowID] = struct{}{}
			}
		}
	}
	var relatedTrace componentmap.FlowID
	if len(related) == 1 {
		for id := range related {
			relatedTrace = id
		}
	}
	category := surfaceOwnershipCategory(trigger, primary)
	return ArchitectureSurface{
		ID:                        trigger.ID,
		Name:                      surfaceTriggerName(trigger),
		Source:                    surfaceSourceCatalog,
		Kind:                      trigger.Kind,
		Category:                  category,
		OwningExecutable:          firstNonEmpty(trigger.OwningExecutable, surfaceExecutable(trigger)),
		OwningComponentID:         primary,
		ParticipatingComponentIDs: sortedComponentIDs(participants),
		RelatedTraceID:            relatedTrace,
		Status:                    trigger.Status,
		Certainty:                 trigger.Certainty,
		Resolution:                trigger.Resolution,
		Evidence:                  evidence,
		TraceUnavailableReason:    surfaceTraceUnavailableReason(trigger, relatedTrace),
		SurfaceRole:               trigger.SurfaceRole,
		TraceReadiness:            trigger.TraceReadiness,
		TraceReadinessReason:      trigger.TraceReadinessReason,
		Quality:                   trigger.Quality,
	}
}

func commandSurfaceMatchesFlowExecutable(trigger DiscoveredTrigger, locations map[string]struct{}) bool {
	executable := cleanSurfacePath(trigger.OwningExecutable)
	if executable == "" && trigger.ProcessEntrypoint.Location != nil {
		executable = cleanSurfacePath(path.Dir(trigger.ProcessEntrypoint.Location.Path))
	}
	if executable == "" {
		return false
	}
	for location := range locations {
		locationPath, _, _ := strings.Cut(location, "\x00")
		locationPath = cleanSurfacePath(locationPath)
		if locationPath == executable || strings.HasPrefix(locationPath, executable+"/") {
			return true
		}
	}
	return false
}

func architectureSurfaceFromTrace(flow ArchitectureFlow, canvas ArchitectureCanvas) ArchitectureSurface {
	evidence := make([]SurfaceLocation, 0, 1)
	primary := architectureTraceStartComponent(flow, canvas)
	for _, kind := range []flowproof.SlotKind{flowproof.SlotTrigger, flowproof.SlotEntrypoint, flowproof.SlotDispatch} {
		candidates := make(map[componentmap.ComponentID]struct{})
		for _, slot := range flow.Slots {
			if slot.Kind != kind {
				continue
			}
			for _, id := range slot.EvidenceIDs {
				for _, step := range flow.Steps {
					if step.ID != id {
						continue
					}
					if step.ComponentID != "" {
						candidates[step.ComponentID] = struct{}{}
					}
					if len(evidence) == 0 && step.Location != nil {
						evidence = append(evidence, SurfaceLocation{Path: step.Location.Path, Line: step.Location.Line, Column: step.Location.Column})
					}
				}
			}
		}
		if primary == "" && len(candidates) == 1 {
			for id := range candidates {
				primary = id
			}
			break
		}
	}
	if len(evidence) == 0 {
		for _, step := range flow.Steps {
			if step.Location != nil {
				evidence = append(evidence, SurfaceLocation{Path: step.Location.Path, Line: step.Location.Line, Column: step.Location.Column})
				break
			}
			if primary == "" && step.ComponentID != "" {
				primary = step.ComponentID
			}
		}
	}
	executable := ""
	if len(evidence) > 0 {
		executable = path.Dir(evidence[0].Path)
		if executable == "." {
			executable = evidence[0].Path
		}
	}
	return ArchitectureSurface{
		ID:                "trace-start-" + string(flow.ID),
		Name:              firstNonEmpty(flow.Command, flow.Trigger, flow.Name),
		Source:            surfaceSourceTraceStart,
		Category:          surfaceCategoryApplication,
		OwningExecutable:  executable,
		OwningComponentID: primary,
		ParticipatingComponentIDs: appendUniqueComponentID(
			append([]componentmap.ComponentID(nil), flow.ParticipatingComponentIDs...),
			primary,
		),
		RelatedTraceID: flow.ID,
		Status:         "saved_trace_start",
		Certainty:      flow.EvidenceBasis,
		Resolution:     flow.Status,
		Evidence:       evidence,
	}
}

func architectureTraceStartComponent(flow ArchitectureFlow, canvas ArchitectureCanvas) componentmap.ComponentID {
	entryAnchors := make(map[string]struct{})
	for _, anchor := range canvas.BehaviorAnchors {
		if anchor.Kind == componentmap.AnchorProcessEntry {
			entryAnchors[anchor.ID] = struct{}{}
		}
	}
	candidates := make(map[componentmap.ComponentID]struct{})
	for _, component := range canvas.Components {
		if !componentParticipatesInFlow(component, flow.ID) {
			continue
		}
		for _, anchorID := range component.AnchorIDs {
			if _, isEntry := entryAnchors[anchorID]; isEntry {
				candidates[component.ID] = struct{}{}
				break
			}
		}
	}
	if len(candidates) != 1 {
		return ""
	}
	for componentID := range candidates {
		return componentID
	}
	return ""
}

func componentParticipatesInFlow(component ArchitectureComponent, flowID componentmap.FlowID) bool {
	for _, candidate := range component.ParticipatingFlowIDs {
		if candidate == flowID {
			return true
		}
	}
	return false
}

func attachArchitectureSurface(canvas *ArchitectureCanvas, index map[componentmap.ComponentID]int, surface ArchitectureSurface) {
	if canvas == nil {
		return
	}
	if componentIndex, ok := index[surface.OwningComponentID]; ok && surface.OwningComponentID != "" {
		canvas.Components[componentIndex].OwnedSurfaceIDs = append(canvas.Components[componentIndex].OwnedSurfaceIDs, surface.ID)
	}
	for _, componentID := range surface.ParticipatingComponentIDs {
		if componentIndex, ok := index[componentID]; ok {
			canvas.Components[componentIndex].ParticipatingSurfaceIDs = appendUniqueString(
				canvas.Components[componentIndex].ParticipatingSurfaceIDs,
				surface.ID,
			)
		}
	}
}

func linkSuggestedInvestigations(data *ReportData, owners architectureOwnershipIndex, index map[componentmap.ComponentID]int) {
	if data == nil || data.ArchitectureCanvas == nil {
		return
	}
	saved := make(map[string]struct{}, len(data.ArchitectureCanvas.Flows))
	for _, flow := range data.ArchitectureCanvas.Flows {
		saved[string(flow.ID)] = struct{}{}
	}
	for _, direction := range data.CandidateDirections {
		if direction.Disposition == flowexplain.DirectionRejected {
			continue
		}
		if _, traced := saved[direction.ID]; traced {
			continue
		}
		identifiers := suggestionIdentifiers(direction)
		identifierPaths := make(map[string]struct{}, len(identifiers))
		for _, identifier := range identifiers {
			identifierPaths[cleanSurfacePath(identifier)] = struct{}{}
		}
		anchorIDs := make(map[string]struct{})
		anchorMatches := make(map[componentmap.ComponentID]struct{})
		anchorLocations := make(map[string]SurfaceLocation)
		for _, anchor := range data.ArchitectureCanvas.BehaviorAnchors {
			if _, matched := identifierPaths[cleanSurfacePath(anchor.Location.Path)]; !matched {
				continue
			}
			anchorIDs[anchor.ID] = struct{}{}
			anchorLocations[anchor.ID] = SurfaceLocation{
				Path: anchor.Location.Path, Line: anchor.Location.Line, Column: anchor.Location.Column,
			}
			ownersForAnchor := make(map[componentmap.ComponentID]struct{})
			for componentID, componentIndex := range index {
				if slices.Contains(data.ArchitectureCanvas.Components[componentIndex].AnchorIDs, anchor.ID) {
					ownersForAnchor[componentID] = struct{}{}
				}
			}
			if len(ownersForAnchor) == 1 {
				for componentID := range ownersForAnchor {
					anchorMatches[componentID] = struct{}{}
				}
			}
		}
		matches := make(map[componentmap.ComponentID]struct{})
		if len(anchorMatches) > 0 {
			matches = anchorMatches
		} else if len(anchorIDs) == 0 {
			for _, identifier := range identifiers {
				for componentID := range uniqueOwnersForIdentifier(owners, identifier) {
					matches[componentID] = struct{}{}
				}
			}
		}
		orderedAnchorIDs := make([]string, 0, len(anchorIDs))
		for anchorID := range anchorIDs {
			orderedAnchorIDs = append(orderedAnchorIDs, anchorID)
		}
		sort.Strings(orderedAnchorIDs)
		var startLocation *SurfaceLocation
		if len(orderedAnchorIDs) > 0 {
			if location := anchorLocations[orderedAnchorIDs[0]]; validSuggestionLocation(location) {
				startLocation = cloneSurfaceLocationValue(location)
			}
		}

		surfaceMatches := exactSuggestionSurfaceMatches(data.DiscoveredSurfaces, direction, identifiers)
		if len(orderedAnchorIDs) == 0 {
			for _, match := range surfaceMatches {
				if match.trigger.OwningComponentID != "" {
					matches[match.trigger.OwningComponentID] = struct{}{}
				}
			}
		}
		traceable := traceableSuggestionSurfaceMatches(surfaceMatches)
		canStartTrace := len(traceable) == 1 && !candidateBasisExcludesTrace(direction.CandidateBasis)
		if startLocation == nil && len(traceable) == 1 {
			startLocation = cloneSurfaceLocationValue(traceable[0].location)
		}
		if startLocation == nil && len(surfaceMatches) == 1 {
			startLocation = cloneSurfaceLocationValue(surfaceMatches[0].location)
		}
		if startLocation == nil {
			startLocation = exactSuggestionSource(data, owners, identifiers)
		}

		grounding := "unassigned"
		if len(orderedAnchorIDs) > 0 {
			grounding = "exact_anchor"
		} else if len(matches) > 0 {
			grounding = "exact_member"
		} else if len(surfaceMatches) > 0 {
			grounding = "exact_surface"
		} else if startLocation != nil {
			grounding = "exact_source"
		}
		componentIDs := sortedComponentIDs(matches)
		available := startLocation != nil && startLocation.Path != ""
		unavailableReason := ""
		if !available {
			unavailableReason = "no exact local source location matched this suggestion"
		} else {
			data.OpenablePaths = appendUniqueString(data.OpenablePaths, startLocation.Path)
			sort.Strings(data.OpenablePaths)
		}
		traceUnavailableReason := suggestionTraceUnavailableReason(
			direction, surfaceMatches, traceable, available, canStartTrace,
		)
		data.ArchitectureCanvas.Suggestions = append(data.ArchitectureCanvas.Suggestions, ArchitectureSuggestion{
			ID: direction.ID, Title: direction.Name, Reason: direction.WhyInteresting,
			EvidenceReferences: append([]string(nil), direction.Evidence...),
			RelevantAnchorIDs:  orderedAnchorIDs, RelevantComponentIDs: componentIDs,
			CurrentGrounding: grounding, CanStartTrace: canStartTrace,
			InvestigationAvailable: available, UnavailableReason: unavailableReason,
			TraceUnavailableReason: traceUnavailableReason, StartLocation: startLocation,
		})
		for componentID := range matches {
			if componentIndex, ok := index[componentID]; ok {
				data.ArchitectureCanvas.Components[componentIndex].SuggestedInvestigationIDs = appendUniqueString(
					data.ArchitectureCanvas.Components[componentIndex].SuggestedInvestigationIDs,
					direction.ID,
				)
			}
		}
	}
	sort.Slice(data.ArchitectureCanvas.Suggestions, func(i, j int) bool {
		return data.ArchitectureCanvas.Suggestions[i].ID < data.ArchitectureCanvas.Suggestions[j].ID
	})
}

type suggestionSurfaceMatch struct {
	trigger  DiscoveredTrigger
	location SurfaceLocation
}

func suggestionIdentifiers(direction CandidateDirection) []string {
	values := append([]string{direction.LikelyEntrypoint}, direction.LikelyFiles...)
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniqueOwnersForIdentifier(owners architectureOwnershipIndex, identifier string) map[componentmap.ComponentID]struct{} {
	result := make(map[componentmap.ComponentID]struct{})
	add := func(values map[componentmap.ComponentID]struct{}) {
		for componentID := range values {
			result[componentID] = struct{}{}
		}
	}
	identifier = strings.TrimSpace(identifier)
	add(owners.packageOwners[identifier])
	add(owners.symbolOwners[identifier])
	add(owners.memberOwners[identifier])
	add(ownersForPath(owners.pathOwners, identifier))
	if len(result) != 1 {
		return nil
	}
	return result
}

func exactSuggestionSurfaceMatches(
	surfaces *DiscoveredSurfaces,
	direction CandidateDirection,
	identifiers []string,
) []suggestionSurfaceMatch {
	if surfaces == nil {
		return nil
	}
	rawIDs := make(map[string]struct{}, len(identifiers)+1)
	paths := make(map[string]struct{}, len(identifiers))
	rawIDs[direction.ID] = struct{}{}
	for _, identifier := range identifiers {
		rawIDs[identifier] = struct{}{}
		paths[cleanSurfacePath(identifier)] = struct{}{}
	}
	result := make([]suggestionSurfaceMatch, 0)
	for _, trigger := range surfaces.Triggers {
		matched := false
		for _, identifier := range []string{
			trigger.ID, trigger.ProcessEntrypoint.ID, trigger.ProcessEntrypoint.Package,
		} {
			if _, exact := rawIDs[identifier]; identifier != "" && exact {
				matched = true
				break
			}
		}
		if !matched {
			for _, location := range surfaceTriggerLocations(trigger) {
				if _, exact := paths[cleanSurfacePath(location.Path)]; exact {
					matched = true
					break
				}
			}
		}
		if !matched {
			continue
		}
		location, ok := suggestionSurfaceLocation(trigger)
		if !ok {
			continue
		}
		result = append(result, suggestionSurfaceMatch{trigger: trigger, location: location})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].trigger.ID < result[j].trigger.ID })
	return result
}

func suggestionSurfaceLocation(trigger DiscoveredTrigger) (SurfaceLocation, bool) {
	var candidates []*SurfaceLocation
	switch trigger.Kind {
	case "process_entry":
		candidates = append(candidates, trigger.ProcessEntrypoint.Location)
	case "http_route_descriptor":
		candidates = append(candidates, trigger.DescriptorSite)
	case "http_server":
		candidates = append(candidates, trigger.ServerStartSite)
	default:
		candidates = append(candidates, trigger.RegistrationSite)
	}
	candidates = append(candidates,
		trigger.RegistrationSite, trigger.ServerStartSite, trigger.DescriptorSite, trigger.ProcessEntrypoint.Location,
	)
	for _, location := range candidates {
		if location != nil && validSuggestionLocation(*location) {
			return *location, true
		}
	}
	return SurfaceLocation{}, false
}

func traceableSuggestionSurfaceMatches(matches []suggestionSurfaceMatch) []suggestionSurfaceMatch {
	result := make([]suggestionSurfaceMatch, 0, len(matches))
	for _, match := range matches {
		ready := match.trigger.TraceReadiness == SurfaceTraceReady ||
			match.trigger.TraceReadiness == SurfaceTracePartialReady
		if ready && match.trigger.Availability != SurfaceAvailabilityUnavailable {
			result = append(result, match)
		}
	}
	return result
}

func candidateBasisExcludesTrace(basis string) bool {
	return basis == flowexplain.CandidateBasisSourceSignalAggregate ||
		basis == flowexplain.CandidateBasisRuntimeActivity
}

func suggestionTraceUnavailableReason(
	direction CandidateDirection,
	matches []suggestionSurfaceMatch,
	traceable []suggestionSurfaceMatch,
	sourceAvailable bool,
	canStartTrace bool,
) string {
	if canStartTrace {
		return ""
	}
	switch direction.CandidateBasis {
	case flowexplain.CandidateBasisSourceSignalAggregate:
		return "local source-signal aggregate does not identify a singular typed trace seed"
	case flowexplain.CandidateBasisRuntimeActivity:
		return "runtime activity is nested work and does not identify an independent top-level typed trace seed"
	}
	if len(traceable) > 1 {
		return "multiple exact trace-ready surfaces match this suggestion; no singular typed trace seed was selected"
	}
	if len(matches) == 1 {
		return firstNonEmpty(
			matches[0].trigger.TraceReadinessReason,
			surfaceTraceUnavailableReason(matches[0].trigger, ""),
		)
	}
	if len(matches) > 1 {
		return "matched surface evidence does not identify a singular supported typed trace seed"
	}
	if sourceAvailable {
		return "exact source is available, but typed trace expansion requires a singular supported entry surface"
	}
	return "no exact local source location can establish a typed trace seed"
}

func exactSuggestionSource(
	data *ReportData,
	owners architectureOwnershipIndex,
	identifiers []string,
) *SurfaceLocation {
	locations := make([]SurfaceLocation, 0)
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, filePath := range data.OpenablePaths {
		openable[cleanSurfacePath(filePath)] = struct{}{}
	}
	for _, identifier := range identifiers {
		locations = append(locations, owners.sourceLocations[identifier]...)
		locations = append(locations, owners.sourceLocations[cleanSurfacePath(identifier)]...)
		if _, ok := openable[cleanSurfacePath(identifier)]; ok {
			locations = append(locations, SurfaceLocation{Path: cleanSurfacePath(identifier)})
		}
	}
	locations = compactSuggestionLocations(locations)
	if len(locations) == 0 {
		return nil
	}
	return cloneSurfaceLocationValue(locations[0])
}

func compactSuggestionLocations(locations []SurfaceLocation) []SurfaceLocation {
	seen := make(map[string]struct{}, len(locations))
	result := make([]SurfaceLocation, 0, len(locations))
	for _, location := range locations {
		location.Path = cleanSurfacePath(location.Path)
		if !validSuggestionLocation(location) {
			continue
		}
		key := surfaceLocationKey(location.Path, location.Line) + "\x00" + strconv.Itoa(location.Column)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, location)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Path != result[j].Path {
			return result[i].Path < result[j].Path
		}
		leftExact := result[i].Line > 0
		rightExact := result[j].Line > 0
		if leftExact != rightExact {
			return leftExact
		}
		if result[i].Line != result[j].Line {
			return result[i].Line < result[j].Line
		}
		return result[i].Column < result[j].Column
	})
	return result
}

func validSuggestionLocation(location SurfaceLocation) bool {
	locationPath := cleanSurfacePath(location.Path)
	if locationPath == "" || location.Line < 0 {
		return false
	}
	return location.Line > 0 || path.Ext(locationPath) != ""
}

func cloneSurfaceLocationValue(location SurfaceLocation) *SurfaceLocation {
	copy := location
	copy.Path = cleanSurfacePath(copy.Path)
	return &copy
}

func addArchitectureSourceLocation(index *architectureOwnershipIndex, key string, location SurfaceLocation) {
	key = strings.TrimSpace(key)
	location.Path = cleanSurfacePath(location.Path)
	if index == nil || key == "" || location.Path == "" || location.Line < 0 {
		return
	}
	index.sourceLocations[key] = append(index.sourceLocations[key], location)
}

func uniqueOwnerForTrigger(trigger DiscoveredTrigger, owners architectureOwnershipIndex) componentmap.ComponentID {
	candidates := []map[componentmap.ComponentID]struct{}{}
	if trigger.Kind == "cli_command" {
		if trigger.Constructor.Location != nil {
			candidates = append(candidates, ownersForPath(owners.pathOwners, trigger.Constructor.Location.Path))
		}
		candidates = append(candidates, owners.symbolOwners[trigger.Constructor.ID])
	}
	if trigger.RegistrationSite != nil {
		candidates = append(candidates, ownersForPath(owners.pathOwners, trigger.RegistrationSite.Path))
	}
	if trigger.DescriptorSite != nil {
		candidates = append(candidates, ownersForPath(owners.pathOwners, trigger.DescriptorSite.Path))
	}
	if trigger.Handler.Known {
		candidates = append(candidates, owners.symbolOwners[trigger.Handler.Text])
	}
	candidates = append(candidates,
		owners.symbolOwners[trigger.ProcessEntrypoint.ID],
		owners.packageOwners[trigger.ProcessEntrypoint.Package],
	)
	if trigger.ProcessEntrypoint.Location != nil {
		candidates = append(candidates, ownersForPath(owners.pathOwners, trigger.ProcessEntrypoint.Location.Path))
	}
	for _, candidate := range candidates {
		if len(candidate) != 1 {
			continue
		}
		for id := range candidate {
			return id
		}
	}
	return ""
}

func surfaceTriggerLocations(trigger DiscoveredTrigger) []SurfaceLocation {
	seen := make(map[string]struct{})
	result := make([]SurfaceLocation, 0, 4+len(trigger.Evidence))
	add := func(location *SurfaceLocation) {
		if location == nil || location.Path == "" || location.Line <= 0 {
			return
		}
		key := surfaceLocationKey(location.Path, location.Line)
		if _, duplicate := seen[key]; duplicate {
			return
		}
		seen[key] = struct{}{}
		result = append(result, *location)
	}
	add(trigger.RegistrationSite)
	add(trigger.DescriptorSite)
	add(trigger.ServerStartSite)
	add(trigger.ProcessEntrypoint.Location)
	for _, item := range trigger.Evidence {
		add(item.Location)
	}
	for _, frontier := range trigger.DynamicFrontier {
		add(frontier.Location)
	}
	return result
}

func surfaceTriggerName(trigger DiscoveredTrigger) string {
	if trigger.Identity.Name != "" {
		return trigger.Identity.Name
	}
	if trigger.Kind == "http_route" {
		return strings.TrimSpace(strings.Join([]string{trigger.Identity.Method, trigger.Identity.Path.Text}, " "))
	}
	return firstNonEmpty(trigger.Identity.Path.Text, trigger.Handler.Text, trigger.ProcessEntrypoint.Name, trigger.ID)
}

func surfaceExecutable(trigger DiscoveredTrigger) string {
	return firstNonEmpty(trigger.ProcessEntrypoint.ID, trigger.ProcessEntrypoint.Package, trigger.ProcessEntrypoint.Name)
}

func surfaceOwnershipCategory(trigger DiscoveredTrigger, _ componentmap.ComponentID) string {
	if trigger.Availability == SurfaceAvailabilityUnavailable {
		return surfaceCategoryUnavailable
	}
	switch trigger.ExecutableRole {
	case ExecutableRolePrimaryApplication:
		return surfaceCategoryApplication
	case ExecutableRoleSecondaryService:
		return surfaceCategoryService
	case ExecutableRoleTooling, "secondary_tooling":
		return surfaceCategoryTooling
	case ExecutableRoleTestOrHelper:
		return surfaceCategoryTests
	}
	if trigger.ProvisionalID || strings.Contains(strings.ToLower(trigger.Resolution), "dynamic") ||
		strings.Contains(strings.ToLower(trigger.Status), "unknown") {
		return surfaceCategoryDynamic
	}
	return surfaceCategoryUnassigned
}

func classifySurfaceExecutable(data *ReportData, trigger *DiscoveredTrigger) {
	if trigger == nil {
		return
	}
	if trigger.Producer == SurfaceProducerCobra && trigger.ProcessEntrypoint.Location != nil {
		trigger.OwningExecutable = cleanSurfacePath(path.Dir(trigger.ProcessEntrypoint.Location.Path))
	}
	if trigger.OwningExecutable == "" {
		if trigger.ProcessEntrypoint.Location != nil {
			trigger.OwningExecutable = cleanSurfacePath(path.Dir(trigger.ProcessEntrypoint.Location.Path))
		}
		if trigger.OwningExecutable == "." || trigger.OwningExecutable == "" {
			trigger.OwningExecutable = surfaceExecutableForPackage(data, trigger.ProcessEntrypoint.Package)
		}
	}
	if trigger.Availability == "" || trigger.Availability == SurfaceAvailabilityUnknown {
		trigger.Availability = SurfaceAvailabilityAvailable
	}
	if trigger.ExecutableRole != "" && trigger.ExecutableRole != ExecutableRoleUnknown {
		return
	}
	location := ""
	if trigger.ProcessEntrypoint.Location != nil {
		location = "/" + strings.ToLower(cleanSurfacePath(trigger.ProcessEntrypoint.Location.Path))
	}
	if strings.HasSuffix(location, "_test.go") || strings.Contains(location, "/test/") || strings.Contains(location, "/tests/") {
		trigger.ExecutableRole = ExecutableRoleTestOrHelper
		return
	}
	if strings.Contains(location, "/helpers/") {
		if strings.Contains(location, "build") || strings.Contains(location, "release") {
			trigger.ExecutableRole = ExecutableRoleTooling
		} else {
			trigger.ExecutableRole = ExecutableRoleTestOrHelper
		}
		return
	}
	for _, segment := range []string{"/dev/", "/tools/", "/tool/", "/hack/", "/scripts/", "/build/", "/release/"} {
		if strings.Contains(location, segment) {
			trigger.ExecutableRole = ExecutableRoleTooling
			return
		}
	}
	if matchesRepositoryNamedMain(data, trigger) || matchesPrimaryProcessAnchor(data, trigger) {
		trigger.ExecutableRole = ExecutableRolePrimaryApplication
		return
	}
	if trigger.Producer == SurfaceProducerCobra {
		if primaryCommandExecutable(data, trigger) {
			trigger.ExecutableRole = ExecutableRolePrimaryApplication
		} else if trigger.ProcessEntrypoint.Location != nil {
			trigger.ExecutableRole = ExecutableRoleSecondaryService
		} else {
			trigger.ExecutableRole = ExecutableRoleUnknown
		}
		return
	}
	if trigger.ProcessEntrypoint.Location != nil {
		trigger.ExecutableRole = ExecutableRoleSecondaryService
		return
	}
	trigger.ExecutableRole = ExecutableRoleUnknown
}

func matchesRepositoryNamedMain(data *ReportData, trigger *DiscoveredTrigger) bool {
	if data == nil || trigger == nil || trigger.ProcessEntrypoint.Name != "main" ||
		trigger.ProcessEntrypoint.Location == nil {
		return false
	}
	executable := cleanSurfacePath(path.Dir(trigger.ProcessEntrypoint.Location.Path))
	return executable != "" && executable != "." &&
		strings.EqualFold(path.Base(executable), strings.TrimSpace(data.RepoName))
}

func matchesPrimaryProcessAnchor(data *ReportData, trigger *DiscoveredTrigger) bool {
	if data == nil || trigger == nil || data.ArchitectureCanvas == nil || trigger.ProcessEntrypoint.Location == nil {
		return false
	}
	entrypointPath := cleanSurfacePath(trigger.ProcessEntrypoint.Location.Path)
	processEntries := 0
	matched := false
	for _, anchor := range data.ArchitectureCanvas.BehaviorAnchors {
		if anchor.Kind != componentmap.AnchorProcessEntry {
			continue
		}
		processEntries++
		matched = matched || cleanSurfacePath(anchor.Location.Path) == entrypointPath
	}
	return processEntries == 1 && matched
}

func primaryCommandExecutable(data *ReportData, trigger *DiscoveredTrigger) bool {
	if data == nil || trigger == nil || trigger.ProcessEntrypoint.Package == "" {
		return false
	}
	for _, trace := range data.CommandTraces {
		if trace.EntrypointPackage != trigger.ProcessEntrypoint.Package {
			continue
		}
		entrypoint, ok := commandTraceStep(trace, "entrypoint")
		if !ok || data.ArchitectureCanvas == nil {
			continue
		}
		executable := cleanSurfacePath(path.Dir(entrypoint.TargetLocation.Path))
		for _, flow := range data.ArchitectureCanvas.Flows {
			if !strings.EqualFold(strings.TrimSpace(flow.Command), strings.TrimSpace(trace.Command)) {
				continue
			}
			for _, step := range flow.Steps {
				if step.Location != nil && pathWithinExecutable(step.Location.Path, executable) {
					return true
				}
			}
		}
		if matchesPrimaryProcessAnchor(data, trigger) {
			return true
		}
	}
	return false
}

func pathWithinExecutable(value, executable string) bool {
	value = cleanSurfacePath(value)
	executable = cleanSurfacePath(executable)
	return executable != "" && (value == executable || strings.HasPrefix(value, executable+"/"))
}

func surfaceTraceUnavailableReason(trigger DiscoveredTrigger, related componentmap.FlowID) string {
	if related != "" {
		return ""
	}
	if (trigger.TraceReadiness == SurfaceTraceUnsupported || trigger.TraceReadiness == SurfaceTraceRejected) &&
		strings.TrimSpace(trigger.TraceReadinessReason) != "" {
		return trigger.TraceReadinessReason
	}
	if trigger.Kind == "cli_command" {
		return "no saved trace was collected for this command"
	}
	switch trigger.Kind {
	case "process_entry":
		if trigger.Availability == SurfaceAvailabilityUnavailable {
			return "typed analysis is unavailable for this executable"
		}
		return "no saved trace was collected from this process entry"
	case "http_server":
		return "static server start call does not establish a saved request trace"
	case "http_route_descriptor":
		return "route descriptor does not prove consumer registration or request execution"
	case "http_route_frontier":
		return "configuration-built route inventory remains unresolved"
	}
	if !trigger.Handler.Known || strings.Contains(strings.ToLower(trigger.Resolution), "dynamic") ||
		len(trigger.DynamicFrontier) > 0 {
		return "handler unresolved"
	}
	return "unsupported surface kind"
}

func ownersForPath(values map[string]map[componentmap.ComponentID]struct{}, value string) map[componentmap.ComponentID]struct{} {
	value = cleanSurfacePath(value)
	if value == "" {
		return nil
	}
	bestLength := -1
	result := make(map[componentmap.ComponentID]struct{})
	for candidate, owners := range values {
		candidate = cleanSurfacePath(candidate)
		if candidate == "" || (value != candidate && !strings.HasPrefix(value, strings.TrimSuffix(candidate, "/")+"/")) {
			continue
		}
		if len(candidate) < bestLength {
			continue
		}
		if len(candidate) > bestLength {
			clear(result)
			bestLength = len(candidate)
		}
		for id := range owners {
			result[id] = struct{}{}
		}
	}
	return result
}

func cleanSurfacePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(value, "./")
	return strings.TrimSuffix(value, "/")
}

func surfaceLocationKey(value string, line int) string {
	return cleanSurfacePath(value) + "\x00" + strconv.Itoa(line)
}

func addComponentSet(values map[string]map[componentmap.ComponentID]struct{}, key string, id componentmap.ComponentID) {
	key = strings.TrimSpace(key)
	if key == "" || id == "" {
		return
	}
	if values[key] == nil {
		values[key] = make(map[componentmap.ComponentID]struct{})
	}
	values[key][id] = struct{}{}
}

func addFlowSet(values map[string]map[componentmap.FlowID]struct{}, key string, id componentmap.FlowID) {
	if key == "" || id == "" {
		return
	}
	if values[key] == nil {
		values[key] = make(map[componentmap.FlowID]struct{})
	}
	values[key][id] = struct{}{}
}

func sortedComponentIDs(values map[componentmap.ComponentID]struct{}) []componentmap.ComponentID {
	result := make([]componentmap.ComponentID, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendUniqueComponentID(values []componentmap.ComponentID, value componentmap.ComponentID) []componentmap.ComponentID {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
