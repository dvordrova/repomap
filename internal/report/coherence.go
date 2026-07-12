package report

import (
	"path"
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
	surfaceCategoryTooling     = "tooling"
	surfaceCategoryTests       = "tests_helpers"
	surfaceCategoryUnassigned  = "unassigned"
	surfaceCategoryDynamic     = "dynamic_unresolved"
)

type architectureOwnershipIndex struct {
	pathOwners    map[string]map[componentmap.ComponentID]struct{}
	packageOwners map[string]map[componentmap.ComponentID]struct{}
	symbolOwners  map[string]map[componentmap.ComponentID]struct{}
}

// linkArchitectureProductObjects derives presentation-only joins from exact
// saved IDs, membership, package identities, and source locations. Ambiguous
// evidence remains unassigned instead of being guessed from display names.
func linkArchitectureProductObjects(data *ReportData) {
	if data == nil || data.ArchitectureCanvas == nil {
		return
	}
	canvas := data.ArchitectureCanvas
	canvas.Surfaces = nil
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
	owners := buildArchitectureOwnershipIndex(canvas.Components)
	flowLocations := architectureFlowLocations(*canvas)
	flowByLocation := make(map[string]map[componentmap.FlowID]struct{})
	for flowID, locations := range flowLocations {
		for location := range locations {
			addFlowSet(flowByLocation, location, flowID)
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
			surface := architectureSurfaceFromTrigger(*trigger, owners, flowByLocation)
			canvas.Surfaces = append(canvas.Surfaces, surface)
			attachArchitectureSurface(canvas, componentIndex, surface)
			if surface.RelatedTraceID != "" {
				if index, ok := flowIndex[surface.RelatedTraceID]; ok && canvas.Flows[index].StartSurfaceID == "" {
					canvas.Flows[index].StartSurfaceID = surface.ID
				}
			}
		}
	}

	for index := range canvas.Flows {
		flow := &canvas.Flows[index]
		if flow.StartSurfaceID != "" {
			continue
		}
		surface := architectureSurfaceFromTrace(*flow)
		flow.StartSurfaceID = surface.ID
		canvas.Surfaces = append(canvas.Surfaces, surface)
		attachArchitectureSurface(canvas, componentIndex, surface)
	}

	linkSuggestedInvestigations(data, owners, componentIndex)
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
	linkArchitectureProductObjects(data)
	refreshProductCounts(data)
}

func buildArchitectureOwnershipIndex(components []ArchitectureComponent) architectureOwnershipIndex {
	index := architectureOwnershipIndex{
		pathOwners:    make(map[string]map[componentmap.ComponentID]struct{}),
		packageOwners: make(map[string]map[componentmap.ComponentID]struct{}),
		symbolOwners:  make(map[string]map[componentmap.ComponentID]struct{}),
	}
	for _, component := range components {
		for _, member := range component.Members {
			switch member.ID.Kind {
			case componentmap.MemberPackage:
				addComponentSet(index.packageOwners, member.ID.Value, component.ID)
			case componentmap.MemberSymbol, componentmap.MemberEntrypoint:
				addComponentSet(index.symbolOwners, member.ID.Value, component.ID)
			}
			for _, fact := range member.Facts {
				if fact.Location != nil {
					addComponentSet(index.pathOwners, cleanSurfacePath(fact.Location.Path), component.ID)
				}
				if fact.Kind == componentmap.FactRepositoryPath {
					addComponentSet(index.pathOwners, cleanSurfacePath(fact.Value), component.ID)
				}
			}
		}
	}
	return index
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
	participants := make(map[componentmap.ComponentID]struct{})
	for _, step := range flow.Steps {
		if step.ComponentID != "" {
			participants[step.ComponentID] = struct{}{}
		}
	}
	flow.ParticipatingComponentIDs = sortedComponentIDs(participants)
	for _, frontier := range canvas.Frontiers {
		if frontier.FlowID == flow.ID && strings.TrimSpace(frontier.Reason) != "" {
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
		OwningExecutable:          surfaceExecutable(trigger),
		OwningComponentID:         primary,
		ParticipatingComponentIDs: sortedComponentIDs(participants),
		RelatedTraceID:            relatedTrace,
		Status:                    trigger.Status,
		Certainty:                 trigger.Certainty,
		Resolution:                trigger.Resolution,
		Evidence:                  evidence,
		TraceUnavailableReason:    surfaceTraceUnavailableReason(trigger, relatedTrace),
	}
}

func architectureSurfaceFromTrace(flow ArchitectureFlow) ArchitectureSurface {
	evidence := make([]SurfaceLocation, 0, 1)
	var primaryCandidates = make(map[componentmap.ComponentID]struct{})
	startIDs := make(map[string]struct{})
	for _, kind := range []flowproof.SlotKind{flowproof.SlotTrigger, flowproof.SlotEntrypoint, flowproof.SlotDispatch} {
		for _, slot := range flow.Slots {
			if slot.Kind != kind {
				continue
			}
			for _, id := range slot.EvidenceIDs {
				startIDs[id] = struct{}{}
			}
		}
	}
	for _, step := range flow.Steps {
		if _, ok := startIDs[step.ID]; !ok && len(startIDs) > 0 {
			continue
		}
		if step.ComponentID != "" {
			primaryCandidates[step.ComponentID] = struct{}{}
		}
		if len(evidence) == 0 && step.Location != nil {
			evidence = append(evidence, SurfaceLocation{Path: step.Location.Path, Line: step.Location.Line, Column: step.Location.Column})
		}
	}
	var primary componentmap.ComponentID
	if len(primaryCandidates) == 1 {
		for id := range primaryCandidates {
			primary = id
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
		ID:                        "trace-start-" + string(flow.ID),
		Name:                      firstNonEmpty(flow.Command, flow.Trigger, flow.Name),
		Source:                    surfaceSourceTraceStart,
		Category:                  surfaceCategoryApplication,
		OwningExecutable:          executable,
		OwningComponentID:         primary,
		ParticipatingComponentIDs: append([]componentmap.ComponentID(nil), flow.ParticipatingComponentIDs...),
		RelatedTraceID:            flow.ID,
		Status:                    "saved_trace_start",
		Certainty:                 flow.EvidenceBasis,
		Resolution:                flow.Status,
		Evidence:                  evidence,
	}
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
		matches := make(map[componentmap.ComponentID]struct{})
		paths := append([]string{direction.LikelyEntrypoint}, direction.LikelyFiles...)
		for _, candidatePath := range paths {
			for id := range ownersForPath(owners.pathOwners, candidatePath) {
				matches[id] = struct{}{}
			}
		}
		for componentID := range matches {
			if componentIndex, ok := index[componentID]; ok {
				data.ArchitectureCanvas.Components[componentIndex].SuggestedInvestigationIDs = appendUniqueString(
					data.ArchitectureCanvas.Components[componentIndex].SuggestedInvestigationIDs,
					direction.ID,
				)
			}
		}
	}
}

func uniqueOwnerForTrigger(trigger DiscoveredTrigger, owners architectureOwnershipIndex) componentmap.ComponentID {
	candidates := []map[componentmap.ComponentID]struct{}{}
	if trigger.RegistrationSite != nil {
		candidates = append(candidates, ownersForPath(owners.pathOwners, trigger.RegistrationSite.Path))
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

func surfaceOwnershipCategory(trigger DiscoveredTrigger, owner componentmap.ComponentID) string {
	if trigger.ProvisionalID || strings.Contains(strings.ToLower(trigger.Resolution), "dynamic") ||
		strings.Contains(strings.ToLower(trigger.Status), "unknown") {
		return surfaceCategoryDynamic
	}
	location := trigger.ProcessEntrypoint.Location
	if location != nil {
		value := "/" + strings.ToLower(cleanSurfacePath(location.Path))
		if strings.HasSuffix(value, "_test.go") || strings.Contains(value, "/test/") || strings.Contains(value, "/tests/") ||
			strings.Contains(value, "/integration/") {
			return surfaceCategoryTests
		}
		for _, segment := range []string{"/tools/", "/tool/", "/hack/", "/scripts/", "/build/", "/release/"} {
			if strings.Contains(value, segment) {
				return surfaceCategoryTooling
			}
		}
	}
	if owner == "" {
		return surfaceCategoryUnassigned
	}
	return surfaceCategoryApplication
}

func surfaceTraceUnavailableReason(trigger DiscoveredTrigger, related componentmap.FlowID) string {
	if related != "" {
		return ""
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
