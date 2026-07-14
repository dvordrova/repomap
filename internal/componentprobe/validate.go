package componentprobe

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
)

var globalIDPattern = regexp.MustCompile(`^(probe|ev|frontier)-[0-9a-f]{20}$`)
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (b Bundle) Validate() error {
	if b.Version != BundleVersion {
		return fmt.Errorf("component probe: unsupported bundle version %d", b.Version)
	}
	switch b.Round {
	case RoundInitial:
		if b.Parent != nil {
			return fmt.Errorf("component probe: initial round cannot have a parent")
		}
	case RoundFrontier:
		if b.Parent == nil || !sha256Pattern.MatchString(b.Parent.BundleSHA256) ||
			!globalIDPattern.MatchString(b.Parent.AcceptedFrontierID) {
			return fmt.Errorf("component probe: frontier round has an invalid parent binding")
		}
	default:
		return fmt.Errorf("component probe: unsupported round %d", b.Round)
	}
	if b.Focus.SupportBasis != SupportOrientationHypothesis {
		return fmt.Errorf("component probe: focus must remain an orientation hypothesis")
	}
	if err := validateFocus(b.Focus); err != nil {
		return err
	}
	if len(b.SymbolProbes) > hardMaxSymbols {
		return fmt.Errorf("component probe: too many symbol probes")
	}
	switch b.Status {
	case StatusBlocked:
		if len(b.SymbolProbes) != 0 {
			return fmt.Errorf("component probe: blocked bundle contains a symbol probe")
		}
	case StatusConnected, StatusFrontier:
		if len(b.SymbolProbes) == 0 {
			return fmt.Errorf("component probe: usable status has no symbol probe")
		}
	default:
		return fmt.Errorf("component probe: invalid status %q", b.Status)
	}

	globalIDs := make(map[string]struct{})
	knownOrigins := make(map[EvidenceOrigin]struct{})
	probeByID := make(map[string]SymbolProbe, len(b.SymbolProbes))
	primaryEvidence := make(map[string]struct{}, len(b.Focus.PrimaryQuestion.EvidenceIDs))
	for _, id := range b.Focus.PrimaryQuestion.EvidenceIDs {
		primaryEvidence[id] = struct{}{}
	}
	for index, probe := range b.SymbolProbes {
		if err := validateProbe(probe); err != nil {
			return fmt.Errorf("component probe: symbol_probes[%d]: %w", index, err)
		}
		if _, exists := primaryEvidence[probe.SelectedSymbol.ID]; !exists {
			return fmt.Errorf("component probe: symbol probe is outside the primary question")
		}
		if _, exists := probeByID[probe.ID]; exists {
			return fmt.Errorf("component probe: duplicate probe id %q", probe.ID)
		}
		probeByID[probe.ID] = probe
		for _, ref := range probe.EvidenceIndex {
			if err := addGlobalID(globalIDs, ref.ID); err != nil {
				return err
			}
			knownOrigins[ref.Origin] = struct{}{}
		}
	}
	if b.Status == StatusConnected {
		selected := make([]componentstudy.SymbolCandidate, 0, len(b.SymbolProbes))
		for _, probe := range b.SymbolProbes {
			selected = append(selected, probe.SelectedSymbol)
		}
		if deriveStatus(b.SymbolProbes, selected) != StatusConnected {
			return fmt.Errorf("component probe: connected status is not supported by direct static edges")
		}
	}

	if len(b.CallsiteWindows) > hardMaxCallsiteWindows {
		return fmt.Errorf("component probe: too many callsite windows")
	}
	totalCallsiteBytes := 0
	for index, window := range b.CallsiteWindows {
		if err := validateCallsiteWindow(window, knownOrigins); err != nil {
			return fmt.Errorf("component probe: callsite_windows[%d]: %w", index, err)
		}
		if err := addGlobalID(globalIDs, window.EvidenceID); err != nil {
			return err
		}
		for _, line := range window.Lines {
			if err := addGlobalID(globalIDs, line.EvidenceID); err != nil {
				return err
			}
		}
		totalCallsiteBytes += windowBytes(window)
	}
	if totalCallsiteBytes > hardMaxCallsiteBytes {
		return fmt.Errorf("component probe: callsite windows exceed byte bound")
	}

	if len(b.Frontier) > hardMaxFrontier {
		return fmt.Errorf("component probe: too many frontier candidates")
	}
	for index, candidate := range b.Frontier {
		if err := validateFrontier(candidate, knownOrigins); err != nil {
			return fmt.Errorf("component probe: frontier[%d]: %w", index, err)
		}
		if err := addGlobalID(globalIDs, candidate.ID); err != nil {
			return err
		}
	}
	for index, warning := range b.Warnings {
		if strings.TrimSpace(warning.Code) == "" || strings.TrimSpace(warning.Message) == "" {
			return fmt.Errorf("component probe: warnings[%d] is incomplete", index)
		}
		if len(warning.Code) > 128 || len(warning.SymbolID) > 128 || len(warning.Message) > 1024 {
			return fmt.Errorf("component probe: warnings[%d] exceeds text bounds", index)
		}
	}
	return nil
}

func validateFocus(focus Focus) error {
	if focus.PrimaryQuestion.ID == "" || strings.TrimSpace(focus.PrimaryQuestion.Question) == "" ||
		strings.TrimSpace(focus.PrimaryQuestion.Why) == "" || len(focus.PrimaryQuestion.EvidenceIDs) == 0 {
		return fmt.Errorf("component probe: primary question is incomplete")
	}
	if len(focus.SelectedFiles) > 2 {
		return fmt.Errorf("component probe: focus contains too many selected files")
	}
	seed := componentstudy.Seed{
		Version:   componentstudy.SeedVersion,
		RepoName:  "component-probe",
		Goal:      focus.Goal,
		Component: focus.Component,
		Files:     focus.SelectedFiles,
	}
	if err := seed.Validate(); err != nil {
		return fmt.Errorf("component probe: invalid focus: %w", err)
	}
	seen := make(map[string]struct{}, len(focus.PrimaryQuestion.EvidenceIDs))
	for _, id := range focus.PrimaryQuestion.EvidenceIDs {
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("component probe: primary question has an empty evidence id")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("component probe: primary question repeats evidence id %q", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateProbe(probe SymbolProbe) error {
	if !globalIDPattern.MatchString(probe.ID) {
		return fmt.Errorf("invalid probe id %q", probe.ID)
	}
	wantProbeID := stableID("probe", probe.SelectedSymbol.ID, probe.SelectedSymbol.Path, fmt.Sprint(probe.SelectedSymbol.Line), probe.SelectedSymbol.Name)
	if probe.ID != wantProbeID {
		return fmt.Errorf("probe id does not match selected symbol")
	}
	seed := componentstudy.Seed{
		Version:  componentstudy.SeedVersion,
		RepoName: "component-probe",
		Goal: componentstudy.Goal{
			ID: "probe-goal", Kind: componentstudy.GoalOnboarding, Objective: "validate selected symbol",
		},
		Component: componentstudy.Component{ID: "probe-component", Name: "probe", Purpose: "validate selected symbol"},
		Symbols:   []componentstudy.SymbolCandidate{probe.SelectedSymbol},
	}
	if err := seed.Validate(); err != nil {
		return fmt.Errorf("invalid selected symbol: %w", err)
	}
	if err := probe.Structural.Validate(); err != nil {
		return fmt.Errorf("invalid structural evidence: %w", err)
	}
	if !sameSelectedEntity(probe.SelectedSymbol, probe.Structural.Target.Entity) {
		return fmt.Errorf("structural target differs from selected symbol")
	}
	if err := probe.Source.Validate(); err != nil {
		return fmt.Errorf("invalid source card: %w", err)
	}
	if probe.Source.Target.EvidenceID != probe.Structural.Target.EvidenceID ||
		probe.Source.Target.EntityID != probe.Structural.Target.Entity.ID {
		return fmt.Errorf("source card is not bound to structural target")
	}
	if err := probe.Tests.Validate(); err != nil {
		return fmt.Errorf("invalid test references: %w", err)
	}
	if probe.Tests.TargetName != probe.Structural.Target.Entity.Name {
		return fmt.Errorf("test references are not bound to structural target")
	}
	wantIndex := buildEvidenceIndex(probe)
	if !reflect.DeepEqual(probe.EvidenceIndex, wantIndex) {
		return fmt.Errorf("evidence index does not match nested local ids")
	}
	return nil
}

func validateCallsiteWindow(window CallsiteWindow, knownOrigins map[EvidenceOrigin]struct{}) error {
	if !globalIDPattern.MatchString(window.EvidenceID) {
		return fmt.Errorf("invalid evidence id %q", window.EvidenceID)
	}
	if window.Direction != DirectionIncoming && window.Direction != DirectionOutgoing {
		return fmt.Errorf("invalid direction %q", window.Direction)
	}
	if window.Basis != SupportSource || window.Certainty != evidence.CertaintyStatic {
		return fmt.Errorf("callsite window overstates its support basis")
	}
	if _, exists := knownOrigins[window.Origin]; !exists || window.Origin.Artifact != ArtifactStructural {
		return fmt.Errorf("callsite window has unknown structural origin")
	}
	if !validLocation(window.Callsite) || len(window.Provenance) == 0 || len(window.Lines) == 0 {
		return fmt.Errorf("callsite window is incomplete")
	}
	if window.Caller.Location == nil || window.Callee.Location == nil ||
		!validLocation(*window.Caller.Location) || !validLocation(*window.Callee.Location) {
		return fmt.Errorf("callsite window has invalid endpoint")
	}
	if window.StartLine != window.Lines[0].Line || window.EndLine != window.Lines[len(window.Lines)-1].Line ||
		window.Callsite.Line < window.StartLine || window.Callsite.Line > window.EndLine {
		return fmt.Errorf("callsite window bounds do not include the callsite")
	}
	if windowBytes(window) > hardMaxCallsiteWindowBytes {
		return fmt.Errorf("callsite window exceeds byte bound")
	}
	previous := 0
	for _, line := range window.Lines {
		if line.Line <= previous || (previous != 0 && line.Line != previous+1) {
			return fmt.Errorf("callsite source lines are not contiguous")
		}
		wantID := stableID("ev", window.Origin.ProbeID, window.Origin.LocalID, window.Callsite.Path, fmt.Sprint(line.Line))
		if line.EvidenceID != wantID {
			return fmt.Errorf("callsite source line has invalid evidence id")
		}
		previous = line.Line
	}
	wantID := stableID("ev", "callsite-window", window.Origin.ProbeID, window.Origin.LocalID, window.Callsite.Path, fmt.Sprint(window.Callsite.Line))
	if window.EvidenceID != wantID {
		return fmt.Errorf("callsite window has unstable evidence id")
	}
	return validateProvenance(window.Provenance)
}

func validateFrontier(candidate Frontier, knownOrigins map[EvidenceOrigin]struct{}) error {
	if !globalIDPattern.MatchString(candidate.ID) || !validLocation(candidate.Location) ||
		strings.TrimSpace(candidate.Name) == "" || !candidate.Certainty.Valid() || len(candidate.Provenance) == 0 || len(candidate.Origins) == 0 {
		return fmt.Errorf("frontier candidate is incomplete")
	}
	if candidate.RuntimeProof {
		return fmt.Errorf("frontier candidate claims runtime proof")
	}
	switch candidate.Kind {
	case FrontierCallEndpoint:
		if candidate.Direction != DirectionIncoming && candidate.Direction != DirectionOutgoing {
			return fmt.Errorf("call frontier has invalid direction")
		}
		if candidate.Basis != SupportStaticNavigation || candidate.NavigationOnly || candidate.Certainty != evidence.CertaintyStatic {
			return fmt.Errorf("call frontier overstates its support basis")
		}
	case FrontierTestReference:
		if candidate.Direction != DirectionReference || candidate.Basis != SupportTestNavigation ||
			!candidate.NavigationOnly || candidate.EntityKind != evidence.EntityTest {
			return fmt.Errorf("test frontier is not navigation-only")
		}
	default:
		return fmt.Errorf("invalid frontier kind %q", candidate.Kind)
	}
	for _, origin := range candidate.Origins {
		if _, exists := knownOrigins[origin]; !exists {
			return fmt.Errorf("frontier candidate has unknown origin")
		}
	}
	wantID := stableID("frontier", frontierKey(candidate.Kind, candidate.Direction, candidate.Name, candidate.EntityKind, candidate.Location))
	if candidate.ID != wantID {
		return fmt.Errorf("frontier candidate has unstable id")
	}
	return validateProvenance(candidate.Provenance)
}

func addGlobalID(known map[string]struct{}, id string) error {
	if !globalIDPattern.MatchString(id) {
		return fmt.Errorf("component probe: invalid global id %q", id)
	}
	if _, exists := known[id]; exists {
		return fmt.Errorf("component probe: duplicate global id %q", id)
	}
	known[id] = struct{}{}
	return nil
}

func validLocation(location evidence.Location) bool {
	path := filepath.FromSlash(location.Path)
	return location.Path != "" && !filepath.IsAbs(path) && filepath.IsLocal(path) &&
		filepath.ToSlash(filepath.Clean(path)) == location.Path && location.Line > 0 && location.Column >= 0
}

func validateProvenance(values []evidence.Provenance) error {
	for _, provenance := range values {
		if strings.TrimSpace(provenance.Provider) == "" || strings.TrimSpace(provenance.Operation) == "" {
			return fmt.Errorf("provenance is incomplete")
		}
		if provenance.Location != nil && !validLocation(*provenance.Location) {
			return fmt.Errorf("provenance has invalid location")
		}
	}
	return nil
}
