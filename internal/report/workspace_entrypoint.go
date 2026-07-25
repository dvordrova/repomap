package report

import (
	"encoding/json"
	"fmt"

	"github.com/dvordrova/repomap/internal/gofacts"
	"github.com/dvordrova/repomap/internal/workspaceentrypoint"
	"github.com/dvordrova/repomap/internal/workspacegraph"
	"github.com/dvordrova/repomap/internal/workspacesnapshot"
)

const maxReportEntrypointProjectedTriggers = workspaceentrypoint.MaxRawRows

type snapshotExactEntrypoint struct {
	ModulePath        string                          `json:"module_path"`
	ImportPath        string                          `json:"import_path"`
	PackageDir        string                          `json:"package_dir"`
	ModuleRelativeDir string                          `json:"module_relative_dir"`
	ModuleDir         string                          `json:"module_dir"`
	Anchors           []snapshotExactEntrypointAnchor `json:"anchors"`
}

type snapshotExactEntrypointAnchor struct {
	Version int    `json:"version"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
}

// attachAuthorizedWorkspaceEntrypointIndex replaces only the canonical exact
// symbol scalars on already-retained process-entry triggers. Every failure
// keeps the original DiscoveredSurfaces pointer and bytes without a warning.
func attachAuthorizedWorkspaceEntrypointIndex(data *ReportData, authority *RunAuthority) {
	if data == nil || authority == nil || data.DiscoveredSurfaces == nil ||
		data.repositoryGoFacts == nil || data.repositoryEntrypointFacts == nil {
		return
	}
	if err := authority.validate(); err != nil {
		return
	}
	snapshot, err := workspacesnapshot.New(workspacesnapshot.Input{
		AnalysisRoot:   authority.analysisRoot,
		Repository:     authority.repository,
		CapturedInputs: authority.inputs,
		AllowedPaths:   data.OpenablePaths,
	})
	if err != nil {
		return
	}
	graph, err := workspacegraph.New(workspacegraph.Input{
		Snapshot: snapshot,
		GoFacts:  *data.repositoryGoFacts,
	})
	if err != nil {
		return
	}
	index, err := workspaceentrypoint.New(workspaceentrypoint.Input{
		GoFacts: *data.repositoryEntrypointFacts,
		Graph:   graph,
	})
	if err != nil {
		return
	}
	_ = attachWorkspaceEntrypointIndex(data, index)
}

func decodeSnapshotExactEntrypoints(snapshotJSON []byte) (gofacts.Facts, error) {
	span, err := preflightSnapshotExactEntrypoints(snapshotJSON)
	if err != nil {
		return gofacts.Facts{}, err
	}
	var saved []snapshotExactEntrypoint
	if err := json.Unmarshal(snapshotJSON[span.start:span.end], &saved); err != nil {
		return gofacts.Facts{}, fmt.Errorf("workspace entrypoint index: saved Go facts are unavailable")
	}
	if len(saved) > workspaceentrypoint.MaxRawRows {
		return gofacts.Facts{}, fmt.Errorf("workspace entrypoint index: saved Go facts exceed bounds")
	}
	anchorCount := 0
	for _, entrypoint := range saved {
		if len(entrypoint.Anchors) > workspaceentrypoint.MaxRawRows-anchorCount {
			return gofacts.Facts{}, fmt.Errorf("workspace entrypoint index: saved Go facts exceed bounds")
		}
		anchorCount += len(entrypoint.Anchors)
	}

	var entrypoints []gofacts.Entrypoint
	if saved != nil {
		entrypoints = make([]gofacts.Entrypoint, 0, min(len(saved), workspaceentrypoint.MaxRawRows))
	}
	for _, savedEntrypoint := range saved {
		entrypoint := gofacts.Entrypoint{
			ModulePath:        savedEntrypoint.ModulePath,
			ImportPath:        savedEntrypoint.ImportPath,
			PackageDir:        savedEntrypoint.PackageDir,
			ModuleRelativeDir: savedEntrypoint.ModuleRelativeDir,
			ModuleDir:         savedEntrypoint.ModuleDir,
		}
		if savedEntrypoint.Anchors != nil {
			entrypoint.Anchors = make(
				[]gofacts.EntrypointAnchor,
				0,
				min(len(savedEntrypoint.Anchors), workspaceentrypoint.MaxRawRows),
			)
		}
		for _, savedAnchor := range savedEntrypoint.Anchors {
			entrypoint.Anchors = append(entrypoint.Anchors, gofacts.EntrypointAnchor{
				Version: savedAnchor.Version,
				Kind:    gofacts.EntrypointAnchorKind(savedAnchor.Kind),
				Path:    savedAnchor.Path,
				Line:    savedAnchor.Line,
			})
		}
		entrypoints = append(entrypoints, entrypoint)
	}
	return gofacts.Facts{EntrypointPackages: entrypoints}, nil
}

func attachWorkspaceEntrypointIndex(
	data *ReportData,
	index workspaceentrypoint.Index,
) error {
	if data == nil {
		return fmt.Errorf("workspace entrypoint index: report data is required")
	}
	projected, err := projectWorkspaceEntrypoints(data.DiscoveredSurfaces, index)
	if err != nil {
		return err
	}
	data.DiscoveredSurfaces = projected
	return nil
}

func projectWorkspaceEntrypoints(
	legacy *DiscoveredSurfaces,
	index workspaceentrypoint.Index,
) (*DiscoveredSurfaces, error) {
	if legacy == nil {
		return nil, fmt.Errorf("workspace entrypoint index: legacy projection is unavailable")
	}
	if !workspaceEntrypointProjectionBounded(legacy) {
		return nil, fmt.Errorf("workspace entrypoint index: legacy projection exceeds bounds")
	}

	for triggerIndex := range legacy.Triggers {
		trigger := legacy.Triggers[triggerIndex]
		if trigger.Kind != "process_entry" {
			continue
		}
		if trigger.ProcessEntrypoint.Location == nil {
			return nil, fmt.Errorf("workspace entrypoint index: process entry %d is unavailable", triggerIndex)
		}
		entry, ok := index.Lookup(
			trigger.ProcessEntrypoint.Package,
			trigger.ProcessEntrypoint.Location.Path,
			trigger.ProcessEntrypoint.Location.Line,
		)
		if !ok || !reportProcessEntrypointMatches(trigger, entry) {
			return nil, fmt.Errorf("workspace entrypoint index: process entry %d differs", triggerIndex)
		}
	}

	projected := cloneDiscoveredSurfaces(legacy)
	for triggerIndex := range projected.Triggers {
		trigger := &projected.Triggers[triggerIndex]
		if trigger.Kind != "process_entry" {
			continue
		}
		entry, _ := index.Lookup(
			trigger.ProcessEntrypoint.Package,
			trigger.ProcessEntrypoint.Location.Path,
			trigger.ProcessEntrypoint.Location.Line,
		)
		location := *trigger.ProcessEntrypoint.Location
		location.Path = entry.Path
		location.Line = entry.Line
		trigger.ProcessEntrypoint = SurfaceSymbol{
			ID:       entry.Package + "." + entry.Symbol,
			Package:  entry.Package,
			Name:     entry.Symbol,
			Location: &location,
		}
	}
	return projected, nil
}

func workspaceEntrypointProjectionBounded(surfaces *DiscoveredSurfaces) bool {
	if surfaces == nil ||
		len(surfaces.Triggers) > maxReportEntrypointProjectedTriggers ||
		len(surfaces.EntrypointsConsidered) > maxSurfaceCoverageItems ||
		len(surfaces.ConfiguredSeedsMatched) > maxSurfaceCoverageItems ||
		len(surfaces.LoopSignals) > maxSurfaceCoverageItems ||
		len(surfaces.DynamicFrontiers) > maxSurfaceCoverageItems ||
		len(surfaces.UnsupportedDispatch) > maxSurfaceCoverageItems ||
		len(surfaces.PackageDiagnostics) > maxSurfaceCoverageItems ||
		len(surfaces.UnavailablePackages) > maxSurfaceCoverageItems ||
		len(surfaces.BudgetsReached) > maxSurfaceCoverageItems {
		return false
	}
	for _, trigger := range surfaces.Triggers {
		if len(trigger.Identity.Path.Candidates) > maxSurfaceValueCandidates ||
			len(trigger.Dispatcher.Candidates) > maxSurfaceValueCandidates ||
			len(trigger.Handler.Candidates) > maxSurfaceValueCandidates ||
			len(trigger.Middleware) > maxSurfaceNestedItems ||
			len(trigger.WrapperChain) > maxSurfaceNestedItems ||
			len(trigger.Evidence) > maxSurfaceNestedItems ||
			len(trigger.Provenance) > maxSurfaceNestedItems ||
			len(trigger.DynamicFrontier) > maxSurfaceNestedItems ||
			len(trigger.ParticipatingComponentIDs) > maxSurfaceNestedItems {
			return false
		}
		for _, middleware := range trigger.Middleware {
			if len(middleware.Candidates) > maxSurfaceValueCandidates {
				return false
			}
		}
	}
	for _, unavailable := range surfaces.UnavailablePackages {
		if len(unavailable.DiagnosticIDs) > maxSurfaceNestedItems {
			return false
		}
	}
	return true
}

func reportProcessEntrypointMatches(
	trigger DiscoveredTrigger,
	entry workspaceentrypoint.Entry,
) bool {
	location := trigger.ProcessEntrypoint.Location
	registration := trigger.RegistrationSite
	if location == nil || registration == nil {
		return false
	}
	symbolID := entry.Package + "." + entry.Symbol
	return entry.Openable &&
		entry.Kind == string(gofacts.EntrypointAnchorGoMain) &&
		trigger.ProcessEntrypoint.ID == symbolID &&
		trigger.ProcessEntrypoint.Package == entry.Package &&
		trigger.ProcessEntrypoint.Name == entry.Symbol &&
		location.Path == entry.Path &&
		location.Line == entry.Line &&
		location.Column == 0 &&
		trigger.Identity.Name == entry.Symbol &&
		trigger.Identity.Path.Kind == "declaration" &&
		trigger.Identity.Path.Text == entry.Path &&
		trigger.Identity.Path.Known &&
		registration.Path == entry.Path &&
		registration.Line == entry.Line &&
		registration.Column == 0 &&
		trigger.Handler.Kind == "declaration" &&
		trigger.Handler.Text == symbolID &&
		trigger.Handler.Known &&
		reportProcessEntrypointEvidenceMatches(trigger.Evidence, entry) &&
		reportProcessEntrypointProvenanceMatches(trigger.Provenance)
}

func reportProcessEntrypointEvidenceMatches(
	evidence []SurfaceEvidence,
	entry workspaceentrypoint.Entry,
) bool {
	matches := 0
	wantID := fmt.Sprintf("process-entry:%s:%d:0", entry.Path, entry.Line)
	for _, item := range evidence {
		if item.ID != wantID ||
			item.Kind != "process_entry_declaration" ||
			item.Location == nil {
			continue
		}
		if item.Location.Path == entry.Path &&
			item.Location.Line == entry.Line &&
			item.Location.Column == 0 {
			matches++
		}
	}
	return matches == 1
}

func reportProcessEntrypointProvenanceMatches(provenance []SurfaceProvenance) bool {
	for _, item := range provenance {
		if item.Provider == "gofacts" &&
			item.Version == "entrypoint-anchor-v1" &&
			item.Operation == "build_selected_main_declaration" {
			return true
		}
	}
	return false
}

func cloneDiscoveredSurfaces(surfaces *DiscoveredSurfaces) *DiscoveredSurfaces {
	if surfaces == nil {
		return nil
	}
	cloned := *surfaces
	cloned.EntrypointsConsidered = cloneSurfaceSymbols(surfaces.EntrypointsConsidered)
	cloned.ConfiguredSeedsMatched = cloneReportSlice(surfaces.ConfiguredSeedsMatched)
	cloned.Triggers = cloneDiscoveredTriggers(surfaces.Triggers)
	cloned.LoopSignals = cloneSurfaceLoopSignals(surfaces.LoopSignals)
	cloned.DynamicFrontiers = cloneSurfaceFrontiers(surfaces.DynamicFrontiers)
	cloned.UnsupportedDispatch = cloneSurfaceFrontiers(surfaces.UnsupportedDispatch)
	cloned.PackageDiagnostics = cloneSurfacePackageDiagnostics(surfaces.PackageDiagnostics)
	cloned.UnavailablePackages = cloneSurfacePackageAvailability(surfaces.UnavailablePackages)
	cloned.BudgetsReached = cloneReportSlice(surfaces.BudgetsReached)
	return &cloned
}

func cloneDiscoveredTriggers(triggers []DiscoveredTrigger) []DiscoveredTrigger {
	cloned := cloneReportSlice(triggers)
	for index := range cloned {
		cloned[index].Identity.Path = cloneSurfaceValue(triggers[index].Identity.Path)
		cloned[index].ProcessEntrypoint = cloneSurfaceSymbol(triggers[index].ProcessEntrypoint)
		cloned[index].Dispatcher = cloneSurfaceValue(triggers[index].Dispatcher)
		cloned[index].Constructor = cloneSurfaceSymbol(triggers[index].Constructor)
		cloned[index].RegistrationSite = cloneSurfaceLocation(triggers[index].RegistrationSite)
		cloned[index].DescriptorSite = cloneSurfaceLocation(triggers[index].DescriptorSite)
		cloned[index].ServerStartSite = cloneSurfaceLocation(triggers[index].ServerStartSite)
		cloned[index].Handler = cloneSurfaceValue(triggers[index].Handler)
		cloned[index].HandlerLocation = cloneSurfaceLocation(triggers[index].HandlerLocation)
		cloned[index].Middleware = cloneSurfaceValues(triggers[index].Middleware)
		cloned[index].WrapperChain = cloneSurfaceWrappers(triggers[index].WrapperChain)
		cloned[index].Evidence = cloneSurfaceEvidence(triggers[index].Evidence)
		cloned[index].Provenance = cloneReportSlice(triggers[index].Provenance)
		cloned[index].DynamicFrontier = cloneSurfaceFrontiers(triggers[index].DynamicFrontier)
		cloned[index].ParticipatingComponentIDs = cloneReportSlice(
			triggers[index].ParticipatingComponentIDs,
		)
	}
	return cloned
}

func cloneSurfaceSymbols(symbols []SurfaceSymbol) []SurfaceSymbol {
	cloned := cloneReportSlice(symbols)
	for index := range cloned {
		cloned[index] = cloneSurfaceSymbol(symbols[index])
	}
	return cloned
}

func cloneSurfaceSymbol(symbol SurfaceSymbol) SurfaceSymbol {
	cloned := symbol
	cloned.Location = cloneSurfaceLocation(symbol.Location)
	return cloned
}

func cloneSurfaceValues(values []SurfaceValue) []SurfaceValue {
	cloned := cloneReportSlice(values)
	for index := range cloned {
		cloned[index] = cloneSurfaceValue(values[index])
	}
	return cloned
}

func cloneSurfaceValue(value SurfaceValue) SurfaceValue {
	cloned := value
	cloned.Candidates = cloneReportSlice(value.Candidates)
	return cloned
}

func cloneSurfaceWrappers(wrappers []SurfaceWrapper) []SurfaceWrapper {
	cloned := cloneReportSlice(wrappers)
	for index := range cloned {
		cloned[index].Symbol = cloneSurfaceSymbol(wrappers[index].Symbol)
		cloned[index].Callsite = cloneSurfaceLocation(wrappers[index].Callsite)
	}
	return cloned
}

func cloneSurfaceEvidence(evidence []SurfaceEvidence) []SurfaceEvidence {
	cloned := cloneReportSlice(evidence)
	for index := range cloned {
		cloned[index].Location = cloneSurfaceLocation(evidence[index].Location)
	}
	return cloned
}

func cloneSurfaceFrontiers(frontiers []SurfaceFrontier) []SurfaceFrontier {
	cloned := cloneReportSlice(frontiers)
	for index := range cloned {
		cloned[index].Location = cloneSurfaceLocation(frontiers[index].Location)
	}
	return cloned
}

func cloneSurfaceLoopSignals(signals []SurfaceLoopSignal) []SurfaceLoopSignal {
	cloned := cloneReportSlice(signals)
	for index := range cloned {
		cloned[index].Location = cloneSurfaceLocation(signals[index].Location)
	}
	return cloned
}

func cloneSurfacePackageDiagnostics(
	diagnostics []SurfacePackageDiagnostic,
) []SurfacePackageDiagnostic {
	cloned := cloneReportSlice(diagnostics)
	for index := range cloned {
		cloned[index].Location = cloneSurfaceLocation(diagnostics[index].Location)
	}
	return cloned
}

func cloneSurfacePackageAvailability(
	packages []SurfacePackageAvailability,
) []SurfacePackageAvailability {
	cloned := cloneReportSlice(packages)
	for index := range cloned {
		cloned[index].DiagnosticIDs = cloneReportSlice(packages[index].DiagnosticIDs)
	}
	return cloned
}

func cloneSurfaceLocation(location *SurfaceLocation) *SurfaceLocation {
	if location == nil {
		return nil
	}
	cloned := *location
	return &cloned
}

func cloneReportSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	cloned := make([]T, len(values))
	copy(cloned, values)
	return cloned
}
