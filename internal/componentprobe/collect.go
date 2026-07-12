package componentprobe

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/analyzer"
	"github.com/dvordrova/repomap/internal/componentstudy"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/reporead"
	"github.com/dvordrova/repomap/internal/sourcecard"
	"github.com/dvordrova/repomap/internal/symbol"
	"github.com/dvordrova/repomap/internal/testevidence"
)

// Collect probes only the symbols selected for the plan's primary question.
// Individual structural failures become warnings so another selected symbol
// can still produce a useful partial dossier.
func Collect(
	ctx context.Context,
	provider Provider,
	repoPath string,
	study componentstudy.Bundle,
	plan componentstudy.Plan,
	opts Options,
) (Bundle, error) {
	primary, selected, opts, err := validateInputs(provider, repoPath, study, plan, opts)
	if err != nil {
		return Bundle{}, err
	}

	bundle := Bundle{
		Version: BundleVersion,
		Round:   RoundInitial,
		Status:  StatusBlocked,
		Focus: Focus{
			Goal:            study.Goal,
			Component:       study.Component,
			PrimaryQuestion: primary,
			SelectedFiles:   append([]componentstudy.FileCandidate(nil), plan.SelectedFiles...),
			SupportBasis:    SupportOrientationHypothesis,
		},
		SymbolProbes:    []SymbolProbe{},
		CallsiteWindows: []CallsiteWindow{},
		Frontier:        []Frontier{},
		Warnings:        []Warning{},
	}

	for _, selectedSymbol := range selected {
		if err := ctx.Err(); err != nil {
			return bundle, err
		}
		probe, err := collectSymbol(ctx, provider, repoPath, selectedSymbol)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return bundle, err
			}
			bundle.Warnings = append(bundle.Warnings, Warning{
				Code:     "symbol.probe_failed",
				SymbolID: selectedSymbol.ID,
				Message:  safeError(repoPath, err),
			})
			continue
		}
		bundle.SymbolProbes = append(bundle.SymbolProbes, probe)
	}

	if len(bundle.SymbolProbes) == 0 {
		bundle.Warnings = append(bundle.Warnings, Warning{
			Code:    "probe.blocked",
			Message: "no selected symbol produced usable structural and source evidence",
		})
		if validateErr := bundle.Validate(); validateErr != nil {
			return Bundle{}, validateErr
		}
		return bundle, fmt.Errorf("component probe: no selected symbol probe survived")
	}

	reader, err := reporead.New(repoPath)
	if err != nil {
		bundle.Warnings = append(bundle.Warnings, Warning{
			Code:    "callsites.reader_unavailable",
			Message: safeError(repoPath, err),
		})
	} else {
		windows, warnings := collectCallsiteWindows(reader, bundle.SymbolProbes, opts)
		bundle.CallsiteWindows = windows
		bundle.Warnings = append(bundle.Warnings, warnings...)
		if closeErr := reader.Close(); closeErr != nil {
			bundle.Warnings = append(bundle.Warnings, Warning{
				Code:    "callsites.reader_close_failed",
				Message: safeError(repoPath, closeErr),
			})
		}
	}

	var omittedFrontier int
	bundle.Frontier, omittedFrontier = buildFrontier(bundle.SymbolProbes, selected, opts.MaxFrontier)
	if omittedFrontier > 0 {
		bundle.Warnings = append(bundle.Warnings, Warning{
			Code:    "frontier.truncated",
			Message: fmt.Sprintf("omitted %d lower-priority frontier candidates", omittedFrontier),
		})
	}
	bundle.Status = deriveStatus(bundle.SymbolProbes, selected)
	if err := bundle.Validate(); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func collectSymbol(
	ctx context.Context,
	provider Provider,
	repoPath string,
	selected componentstudy.SymbolCandidate,
) (SymbolProbe, error) {
	entity, err := selectedEntity(selected)
	if err != nil {
		return SymbolProbe{}, err
	}
	graph, err := provider.AnalyzeExactSymbol(ctx, analyzer.ExactSymbolRequest{
		RepoPath: repoPath,
		Symbol:   entity,
	})
	if err != nil {
		return SymbolProbe{}, fmt.Errorf("analyze exact symbol: %w", err)
	}
	structural, err := symbol.Build(graph, symbol.Options{
		MaxCandidates:        1,
		MaxIncomingCalls:     hardMaxCallsPerDirection,
		MaxOutgoingCalls:     hardMaxCallsPerDirection,
		MaxProvenancePerFact: hardMaxProvenancePerFact,
	})
	if err != nil {
		return SymbolProbe{}, fmt.Errorf("build structural evidence: %w", err)
	}
	if !sameSelectedEntity(selected, structural.Target.Entity) {
		return SymbolProbe{}, fmt.Errorf("structural target does not match selected symbol")
	}
	source, err := sourcecard.Read(sourcecard.Request{
		RepoPath:         repoPath,
		TargetEvidenceID: structural.Target.EvidenceID,
		Target:           structural.Target.Entity,
	}, sourcecard.Limits{
		MaxFileBytes:   hardMaxCallsiteFileBytes,
		MaxWindowLines: hardMaxSourceLines,
		MaxWindowBytes: hardMaxSourceBytes,
		MaxLineBytes:   8 * 1024,
	})
	if err != nil {
		return SymbolProbe{}, fmt.Errorf("read source card: %w", err)
	}
	tests, err := testevidence.CollectTarget(ctx, provider, repoPath, structural, testevidence.Options{
		MaxSearches:            1,
		MaxReferencesPerSearch: 4,
	})
	if err != nil {
		return SymbolProbe{}, fmt.Errorf("collect test references: %w", err)
	}
	probe := SymbolProbe{
		ID:             stableID("probe", selected.ID, selected.Path, fmt.Sprint(selected.Line), selected.Name),
		SelectedSymbol: selected,
		Structural:     structural,
		Source:         source,
		Tests:          tests,
	}
	probe.EvidenceIndex = buildEvidenceIndex(probe)
	return probe, nil
}

func validateInputs(
	provider Provider,
	repoPath string,
	study componentstudy.Bundle,
	plan componentstudy.Plan,
	opts Options,
) (componentstudy.Question, []componentstudy.SymbolCandidate, Options, error) {
	if provider == nil {
		return componentstudy.Question{}, nil, Options{}, fmt.Errorf("component probe: provider is required")
	}
	if strings.TrimSpace(repoPath) == "" {
		return componentstudy.Question{}, nil, Options{}, fmt.Errorf("component probe: repository path is required")
	}
	if err := study.Validate(); err != nil {
		return componentstudy.Question{}, nil, Options{}, fmt.Errorf("component probe: invalid study bundle: %w", err)
	}
	if err := plan.Validate(study); err != nil {
		return componentstudy.Question{}, nil, Options{}, fmt.Errorf("component probe: invalid study plan: %w", err)
	}
	opts, err := normalizeOptions(opts)
	if err != nil {
		return componentstudy.Question{}, nil, Options{}, err
	}
	var primary componentstudy.Question
	for _, question := range plan.Questions {
		if question.ID == plan.PrimaryQuestionID {
			primary = question
			break
		}
	}
	if primary.ID == "" {
		return componentstudy.Question{}, nil, Options{}, fmt.Errorf("component probe: primary question is required")
	}
	referenced := make(map[string]struct{}, len(primary.EvidenceIDs))
	for _, id := range primary.EvidenceIDs {
		referenced[id] = struct{}{}
	}
	selected := make([]componentstudy.SymbolCandidate, 0, len(plan.SelectedSymbols))
	for _, candidate := range plan.SelectedSymbols {
		if _, ok := referenced[candidate.ID]; ok {
			selected = append(selected, candidate)
		}
	}
	if len(selected) == 0 {
		return componentstudy.Question{}, nil, Options{}, fmt.Errorf("component probe: primary question selects no symbol")
	}
	if len(selected) > hardMaxSymbols {
		return componentstudy.Question{}, nil, Options{}, fmt.Errorf("component probe: primary question selects more than %d symbols", hardMaxSymbols)
	}
	return primary, selected, opts, nil
}

func normalizeOptions(opts Options) (Options, error) {
	defaults := Options{
		MaxCallsiteWindows:     hardMaxCallsiteWindows,
		CallsiteLinesBefore:    hardMaxCallsiteLinesBefore,
		CallsiteLinesAfter:     hardMaxCallsiteLinesAfter,
		MaxCallsiteWindowBytes: hardMaxCallsiteWindowBytes,
		MaxCallsiteBytes:       hardMaxCallsiteBytes,
		MaxFrontier:            hardMaxFrontier,
	}
	values := []struct {
		name string
		got  *int
		max  int
		def  int
	}{
		{"max callsite windows", &opts.MaxCallsiteWindows, hardMaxCallsiteWindows, defaults.MaxCallsiteWindows},
		{"callsite lines before", &opts.CallsiteLinesBefore, hardMaxCallsiteLinesBefore, defaults.CallsiteLinesBefore},
		{"callsite lines after", &opts.CallsiteLinesAfter, hardMaxCallsiteLinesAfter, defaults.CallsiteLinesAfter},
		{"max callsite window bytes", &opts.MaxCallsiteWindowBytes, hardMaxCallsiteWindowBytes, defaults.MaxCallsiteWindowBytes},
		{"max callsite bytes", &opts.MaxCallsiteBytes, hardMaxCallsiteBytes, defaults.MaxCallsiteBytes},
		{"max frontier", &opts.MaxFrontier, hardMaxFrontier, defaults.MaxFrontier},
	}
	for _, value := range values {
		if *value.got == 0 {
			*value.got = value.def
		}
		if *value.got < 0 || *value.got > value.max {
			return Options{}, fmt.Errorf("component probe: %s must be between 0 and %d", value.name, value.max)
		}
	}
	if opts.MaxCallsiteWindows > 0 && (opts.MaxCallsiteWindowBytes == 0 || opts.MaxCallsiteBytes == 0) {
		return Options{}, fmt.Errorf("component probe: enabled callsite windows require positive byte bounds")
	}
	return opts, nil
}

func selectedEntity(selected componentstudy.SymbolCandidate) (evidence.Entity, error) {
	kind := evidence.EntityKind(selected.Kind)
	if kind != evidence.EntityFunction && kind != evidence.EntityMethod {
		return evidence.Entity{}, fmt.Errorf("selected symbol %q is not a function or method", selected.ID)
	}
	if selected.Line <= 0 || selected.Column <= 0 {
		return evidence.Entity{}, fmt.Errorf("selected symbol %q has no exact declaration position", selected.ID)
	}
	location := evidence.Location{
		Path:   selected.Path,
		Line:   selected.Line,
		Column: selected.Column,
	}
	return evidence.Entity{
		ID:       selected.ID,
		Kind:     kind,
		Name:     selected.Name,
		Language: "go",
		Location: &location,
	}, nil
}

func sameSelectedEntity(selected componentstudy.SymbolCandidate, got evidence.Entity) bool {
	return got.Location != nil &&
		got.Kind == evidence.EntityKind(selected.Kind) &&
		callableName(got.Name) == callableName(selected.Name) &&
		got.Location.Path == selected.Path &&
		got.Location.Line == selected.Line &&
		got.Location.Column == selected.Column
}

func safeError(repoPath string, err error) string {
	message := strings.TrimSpace(err.Error())
	paths := []string{repoPath}
	if absolute, absoluteErr := filepath.Abs(repoPath); absoluteErr == nil {
		paths = append(paths, absolute)
		if resolved, resolvedErr := filepath.EvalSymlinks(absolute); resolvedErr == nil {
			paths = append(paths, resolved)
		}
	}
	sort.Slice(paths, func(i, j int) bool { return len(paths[i]) > len(paths[j]) })
	for _, path := range paths {
		if path != "" {
			message = strings.ReplaceAll(message, path, "<repo>")
		}
	}
	const maxBytes = 1024
	if len(message) > maxBytes {
		message = message[:maxBytes]
	}
	return message
}

func callableName(name string) string {
	name = strings.TrimSpace(name)
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		return name[index+1:]
	}
	return name
}
