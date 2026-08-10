package report

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/analysistarget"
	"github.com/dvordrova/repomap/internal/entrycall"
)

// EntryCallReportProjectionVersion is the report-owned wire version for the
// flat exact-call-family projection and D283's separately bound model-assisted
// entry surfaces. Neither projection claims a runtime sequence, mechanism, or
// Canvas edge.
const EntryCallReportProjectionVersion = 2

const (
	EntryCallSurfaceOriginModelAssisted        = "model_assisted"
	EntryCallSurfaceStateExactRegistration     = "exact_registration"
	EntryCallSurfaceStateDeclaredDescriptor    = "declared_descriptor"
	EntryCallSurfaceRuntimeReachabilityUnknown = "not_established"
)

// EntryCallReportProjection promotes only accepted entry-call artifacts whose
// exact root declaration belongs to the selected executable target.
type EntryCallReportProjection struct {
	Version         int                            `json:"version"`
	Families        []EntryCallReportFamily        `json:"families"`
	Surfaces        []EntryCallReportSurface       `json:"surfaces"`
	SurfaceCoverage EntryCallReportSurfaceCoverage `json:"surface_coverage"`
}

// EntryCallReportFamily is one exact direct-call family restored from local
// authority. Request-local refs and provider ordering are intentionally absent.
type EntryCallReportFamily struct {
	RootDeclaration entrycall.Location   `json:"root_declaration"`
	CallerLabel     string               `json:"caller_label"`
	CalleeLabel     string               `json:"callee_label"`
	Invocation      entrycall.Invocation `json:"invocation"`
	WitnessCount    int                  `json:"witness_count"`
	Callsites       []entrycall.Location `json:"callsites"`
}

// EntryCallReportSurface is local restoration of one accepted refs-only
// semantic proposal. Request-local candidate/root refs and provider order are
// deliberately absent. These records remain separate from Trigger Catalog,
// Atlas, Architecture and Canvas authority.
type EntryCallReportSurface struct {
	ID                  string                         `json:"id"`
	RootDeclaration     entrycall.Location             `json:"root_declaration"`
	Kind                string                         `json:"kind"`
	Role                string                         `json:"role"`
	Form                entrycall.SurfaceCandidateForm `json:"form"`
	Site                entrycall.Location             `json:"site"`
	Identity            *EntryCallReportSurfaceValue   `json:"identity,omitempty"`
	Method              *EntryCallReportSurfaceValue   `json:"method,omitempty"`
	Path                *EntryCallReportSurfaceValue   `json:"path,omitempty"`
	Handler             *EntryCallReportSurfaceValue   `json:"handler,omitempty"`
	Origin              string                         `json:"origin"`
	State               string                         `json:"state"`
	RuntimeReachability string                         `json:"runtime_reachability"`
}

type EntryCallReportSurfaceValue struct {
	Kind     entrycall.SurfaceFactKind `json:"kind"`
	Text     string                    `json:"text"`
	Location *entrycall.Location       `json:"location,omitempty"`
}

// EntryCallReportSurfaceCoverage is aggregate local accounting only. It
// exposes no candidate identity and is safe bounded copy for Entrypoints.
type EntryCallReportSurfaceCoverage struct {
	ConsideredCandidates          int `json:"considered_candidates"`
	AdvertisedCandidates          int `json:"advertised_candidates"`
	OmittedCandidates             int `json:"omitted_candidates"`
	ConsideredFacts               int `json:"considered_facts"`
	AdvertisedFacts               int `json:"advertised_facts"`
	OmittedFacts                  int `json:"omitted_facts"`
	UnsafeFactsExcluded           int `json:"unsafe_facts_excluded"`
	UnreachableCandidatesExcluded int `json:"unreachable_candidates_excluded"`
	SelectedProposals             int `json:"selected_proposals"`
	RejectedProposals             int `json:"rejected_proposals"`
}

type entryCallReportArtifactBinding struct {
	statusSHA256          string
	resultSHA256          string
	repositoryStateSHA256 string
}

// loadEntryCallReportProjection reads the optional debug sidecar family. A
// missing or closed status has no report projection. Accepted artifacts are a
// fail-closed pair and are promoted only after all cross-artifact bindings
// have been proven.
func loadEntryCallReportProjection(
	runDir string,
	data *ReportData,
	expectedRepositoryStateSHA256 string,
) error {
	if data == nil {
		return fmt.Errorf("entry call report: report data is required")
	}
	data.EntryCall = nil
	data.entryCallArtifactBinding = nil

	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("entry call report: open run directory: %w", err)
	}
	defer root.Close()

	statusRaw, hasStatus, resultRaw, hasResult, err := readOptionalEntryCallArtifactPair(root)
	if err != nil {
		return err
	}
	if !hasStatus {
		if hasResult {
			return fmt.Errorf("entry call report: result is present without status")
		}
		return nil
	}

	status, err := entrycall.DecodeStatus(statusRaw)
	if err != nil {
		return fmt.Errorf("entry call report: status: %w", err)
	}
	if status.State != entrycall.StatusAccepted && status.State != entrycall.StatusAcceptedPartial {
		if hasResult {
			return fmt.Errorf("entry call report: closed status has an unexpected result")
		}
		return nil
	}

	if !hasResult {
		return fmt.Errorf("entry call report: accepted status requires result")
	}
	projection, binding, err := acceptedEntryCallReportProjection(
		statusRaw,
		resultRaw,
		data.AnalysisTarget,
		expectedRepositoryStateSHA256,
	)
	if err != nil {
		return err
	}
	data.EntryCall = projection
	data.entryCallArtifactBinding = binding
	return nil
}

func acceptedEntryCallReportProjection(
	statusRaw, resultRaw []byte,
	target *analysistarget.Target,
	expectedRepositoryStateSHA256 string,
) (*EntryCallReportProjection, *entryCallReportArtifactBinding, error) {
	status, err := entrycall.DecodeStatus(statusRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("entry call report: status: %w", err)
	}
	if status.State != entrycall.StatusAccepted && status.State != entrycall.StatusAcceptedPartial {
		return nil, nil, fmt.Errorf("entry call report: status is not accepted")
	}
	result, err := entrycall.DecodeResult(resultRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("entry call report: result: %w", err)
	}
	resultSHA256 := manifestSHA256(resultRaw)
	if status.ResultSHA256 != resultSHA256 ||
		status.PromptVersion != result.PromptVersion ||
		status.RequestRef != result.RequestRef ||
		status.RequestSHA256 != result.RequestSHA256 ||
		status.SubstrateSHA256 != result.SubstrateSHA256 ||
		status.RepositoryStateSHA256 != result.RepositoryStateSHA256 {
		return nil, nil, fmt.Errorf("entry call report: status/result identity mismatch")
	}
	if status.SelectedFamilies != result.SelectedFamilyCount() ||
		status.RejectedFamilies != result.RejectedFamilyCount() {
		return nil, nil, fmt.Errorf("entry call report: status/result family counts mismatch")
	}
	if status.SelectedSurfaces != result.SelectedSurfaceCount() ||
		status.RejectedSurfaces != result.RejectedSurfaceCount() ||
		!reflect.DeepEqual(status.SurfaceCandidateCoverage, result.SurfaceCandidateCoverage) {
		return nil, nil, fmt.Errorf("entry call report: status/result surface accounting mismatch")
	}
	if expectedRepositoryStateSHA256 != "" &&
		result.RepositoryStateSHA256 != expectedRepositoryStateSHA256 {
		return nil, nil, fmt.Errorf("entry call report: repository state binding mismatch")
	}

	projection, err := projectEntryCallFamilies(target, result)
	if err != nil {
		return nil, nil, err
	}
	return projection, &entryCallReportArtifactBinding{
		statusSHA256:          manifestSHA256(statusRaw),
		resultSHA256:          resultSHA256,
		repositoryStateSHA256: result.RepositoryStateSHA256,
	}, nil
}

func projectEntryCallFamilies(
	target *analysistarget.Target,
	result entrycall.Result,
) (*EntryCallReportProjection, error) {
	if target == nil {
		return nil, fmt.Errorf("entry call report: accepted result requires analysis target")
	}
	if err := target.Validate(); err != nil {
		return nil, fmt.Errorf("entry call report: analysis target: %w", err)
	}
	if target.Kind == analysistarget.KindLibraryPackage || target.Kind == analysistarget.KindModuleLibrary {
		// Library roots are intentionally unresolved in target v1. The accepted
		// artifact remains hash-bound, but no process-root projection is invented.
		return nil, nil
	}
	if target.Kind != analysistarget.KindExecutablePackage {
		return nil, fmt.Errorf("entry call report: unsupported analysis target kind %q", target.Kind)
	}

	targetRoots := make(map[string]struct{}, len(target.Roots))
	for _, root := range target.Roots {
		targetRoots[entryCallRootKey(root.Path, root.Line)] = struct{}{}
	}
	seenRoots := make(map[string]struct{}, len(result.Entries))
	projection := &EntryCallReportProjection{
		Version:         EntryCallReportProjectionVersion,
		Families:        make([]EntryCallReportFamily, 0, result.SelectedFamilyCount()),
		Surfaces:        make([]EntryCallReportSurface, 0, result.SelectedSurfaceCount()),
		SurfaceCoverage: projectEntryCallSurfaceCoverage(result),
	}
	rootDeclarations := make(map[string]entrycall.Location, len(result.Entries))
	for _, entry := range result.Entries {
		rootKey := entryCallRootKey(entry.Declaration.Path, entry.Declaration.Line)
		if _, exact := targetRoots[rootKey]; !exact {
			return nil, fmt.Errorf("entry call report: result root is outside analysis target")
		}
		if _, duplicate := seenRoots[rootKey]; duplicate {
			return nil, fmt.Errorf("entry call report: duplicate exact result root")
		}
		seenRoots[rootKey] = struct{}{}
		rootDeclarations[entry.RootRef] = entry.Declaration
		for _, family := range entry.Families {
			projection.Families = append(projection.Families, EntryCallReportFamily{
				RootDeclaration: entry.Declaration,
				CallerLabel:     family.CallerLabel,
				CalleeLabel:     family.CalleeLabel,
				Invocation:      family.Invocation,
				WitnessCount:    family.WitnessCount,
				Callsites:       append([]entrycall.Location{}, family.Callsites...),
			})
		}
	}
	seenSurfaceIDs := make(map[string]struct{}, len(result.SurfaceProposals))
	for index, proposal := range result.SurfaceProposals {
		rootDeclaration, knownRoot := rootDeclarations[proposal.RootRef]
		if !knownRoot {
			return nil, fmt.Errorf("entry call report: surface proposal %d has unknown root", index)
		}
		if _, duplicate := seenSurfaceIDs[proposal.ID]; duplicate {
			return nil, fmt.Errorf("entry call report: duplicate surface proposal identity")
		}
		seenSurfaceIDs[proposal.ID] = struct{}{}
		surface, err := projectEntryCallSurface(rootDeclaration, proposal)
		if err != nil {
			return nil, fmt.Errorf("entry call report: surface proposal %d: %w", index, err)
		}
		projection.Surfaces = append(projection.Surfaces, surface)
	}
	sort.Slice(projection.Surfaces, func(i, j int) bool {
		left, right := projection.Surfaces[i], projection.Surfaces[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Site.Path != right.Site.Path {
			return left.Site.Path < right.Site.Path
		}
		if left.Site.Line != right.Site.Line {
			return left.Site.Line < right.Site.Line
		}
		if left.Site.Column != right.Site.Column {
			return left.Site.Column < right.Site.Column
		}
		return left.ID < right.ID
	})
	return projection, nil
}

func projectEntryCallSurfaceCoverage(result entrycall.Result) EntryCallReportSurfaceCoverage {
	coverage := result.SurfaceCandidateCoverage
	return EntryCallReportSurfaceCoverage{
		ConsideredCandidates:          coverage.ConsideredCandidates,
		AdvertisedCandidates:          coverage.AdvertisedCandidates,
		OmittedCandidates:             coverage.OmittedCandidates,
		ConsideredFacts:               coverage.ConsideredFacts,
		AdvertisedFacts:               coverage.AdvertisedFacts,
		OmittedFacts:                  coverage.OmittedFacts,
		UnsafeFactsExcluded:           coverage.UnsafeFactsExcluded,
		UnreachableCandidatesExcluded: coverage.UnreachableCandidatesExcluded,
		SelectedProposals:             result.SelectedSurfaceCount(),
		RejectedProposals:             result.RejectedSurfaceCount(),
	}
}

func projectEntryCallSurface(
	rootDeclaration entrycall.Location,
	proposal entrycall.ResultSurfaceProposal,
) (EntryCallReportSurface, error) {
	surface := EntryCallReportSurface{
		ID:                  proposal.ID,
		RootDeclaration:     rootDeclaration,
		Kind:                proposal.Kind,
		Role:                proposal.Role,
		Form:                proposal.Form,
		Site:                proposal.Site,
		Identity:            projectEntryCallSurfaceValue(proposal.Identity),
		Method:              projectEntryCallSurfaceValue(proposal.Method),
		Path:                projectEntryCallSurfaceValue(proposal.Path),
		Handler:             projectEntryCallSurfaceValue(proposal.Handler),
		Origin:              EntryCallSurfaceOriginModelAssisted,
		RuntimeReachability: EntryCallSurfaceRuntimeReachabilityUnknown,
	}
	switch proposal.Kind {
	case entrycall.SurfaceKindHTTPRoute:
		if proposal.Handler == nil {
			surface.State = EntryCallSurfaceStateDeclaredDescriptor
		} else {
			surface.State = EntryCallSurfaceStateExactRegistration
		}
	case entrycall.SurfaceKindScheduledJob:
		if proposal.Handler == nil {
			surface.State = EntryCallSurfaceStateDeclaredDescriptor
		} else {
			surface.State = EntryCallSurfaceStateExactRegistration
		}
	case entrycall.SurfaceKindCLICommand:
		surface.State = EntryCallSurfaceStateDeclaredDescriptor
	default:
		return EntryCallReportSurface{}, fmt.Errorf("unsupported kind %q", proposal.Kind)
	}
	return surface, nil
}

func projectEntryCallSurfaceValue(value *entrycall.ResultSurfaceValue) *EntryCallReportSurfaceValue {
	if value == nil {
		return nil
	}
	projected := &EntryCallReportSurfaceValue{Kind: value.Kind, Text: value.Text}
	if value.Location != nil {
		location := *value.Location
		projected.Location = &location
	}
	return projected
}

func validateEntryCallReportProjection(
	target *analysistarget.Target,
	projection *EntryCallReportProjection,
	openablePaths []string,
) error {
	if projection == nil {
		return nil
	}
	if target == nil || target.Kind != analysistarget.KindExecutablePackage {
		return fmt.Errorf("entry call projection requires executable analysis target")
	}
	if err := target.Validate(); err != nil {
		return fmt.Errorf("entry call projection analysis target: %w", err)
	}
	if projection.Version != EntryCallReportProjectionVersion || projection.Families == nil ||
		projection.Surfaces == nil ||
		len(projection.Families) > entrycall.MaxRoots*entrycall.MaxSelectedFamiliesPerRoot ||
		len(projection.Surfaces) > entrycall.MaxSelectedSurfaceProposals ||
		!validEntryCallReportSurfaceCoverage(projection.SurfaceCoverage, len(projection.Surfaces)) {
		return fmt.Errorf("entry call projection identity is invalid")
	}
	targetRoots := make(map[string]struct{}, len(target.Roots))
	for _, root := range target.Roots {
		targetRoots[entryCallRootKey(root.Path, root.Line)] = struct{}{}
	}
	openable := make(map[string]struct{}, len(openablePaths))
	for _, path := range openablePaths {
		openable[path] = struct{}{}
	}
	for index, family := range projection.Families {
		if !validEntryCallReportLocation(family.RootDeclaration) {
			return fmt.Errorf("entry call family %d root declaration is invalid", index)
		}
		if _, exact := targetRoots[entryCallRootKey(family.RootDeclaration.Path, family.RootDeclaration.Line)]; !exact {
			return fmt.Errorf("entry call family %d root is outside analysis target", index)
		}
		if !validEntryCallReportLabel(family.CallerLabel) ||
			!validEntryCallReportLabel(family.CalleeLabel) ||
			!family.Invocation.Valid() || family.WitnessCount <= 0 ||
			len(family.Callsites) == 0 || len(family.Callsites) > entrycall.MaxRepresentativeCallsites {
			return fmt.Errorf("entry call family %d is invalid", index)
		}
		for callsiteIndex, callsite := range family.Callsites {
			if !validEntryCallReportLocation(callsite) {
				return fmt.Errorf("entry call family %d callsite %d is invalid", index, callsiteIndex)
			}
			if _, authorized := openable[callsite.Path]; !authorized {
				return fmt.Errorf("entry call family %d callsite %d is not openable", index, callsiteIndex)
			}
		}
	}
	seenSurfaceIDs := make(map[string]struct{}, len(projection.Surfaces))
	for index, surface := range projection.Surfaces {
		if !validEntryCallReportSurfaceID(surface.ID) {
			return fmt.Errorf("entry call surface %d identity is invalid", index)
		}
		if _, duplicate := seenSurfaceIDs[surface.ID]; duplicate {
			return fmt.Errorf("entry call surface %d identity is duplicated", index)
		}
		seenSurfaceIDs[surface.ID] = struct{}{}
		if !validEntryCallReportLocation(surface.RootDeclaration) {
			return fmt.Errorf("entry call surface %d root declaration is invalid", index)
		}
		if _, exact := targetRoots[entryCallRootKey(
			surface.RootDeclaration.Path,
			surface.RootDeclaration.Line,
		)]; !exact {
			return fmt.Errorf("entry call surface %d root is outside analysis target", index)
		}
		if err := validateEntryCallReportSurface(surface, openable); err != nil {
			return fmt.Errorf("entry call surface %d: %w", index, err)
		}
	}
	return nil
}

func validEntryCallReportSurfaceCoverage(
	coverage EntryCallReportSurfaceCoverage,
	selected int,
) bool {
	values := []int{
		coverage.ConsideredCandidates,
		coverage.AdvertisedCandidates,
		coverage.OmittedCandidates,
		coverage.ConsideredFacts,
		coverage.AdvertisedFacts,
		coverage.OmittedFacts,
		coverage.UnsafeFactsExcluded,
		coverage.UnreachableCandidatesExcluded,
		coverage.SelectedProposals,
		coverage.RejectedProposals,
	}
	for _, value := range values {
		if value < 0 {
			return false
		}
	}
	return coverage.AdvertisedCandidates <= entrycall.MaxSurfaceCandidates &&
		coverage.AdvertisedCandidates <= coverage.ConsideredCandidates &&
		coverage.AdvertisedCandidates+coverage.OmittedCandidates == coverage.ConsideredCandidates &&
		coverage.AdvertisedFacts <= entrycall.MaxSurfaceFacts &&
		coverage.AdvertisedFacts <= coverage.ConsideredFacts &&
		coverage.AdvertisedFacts+coverage.OmittedFacts == coverage.ConsideredFacts &&
		coverage.SelectedProposals == selected &&
		coverage.SelectedProposals <= entrycall.MaxSelectedSurfaceProposals &&
		coverage.RejectedProposals <= entrycall.MaxSurfaceCandidates &&
		coverage.SelectedProposals+coverage.RejectedProposals <= coverage.AdvertisedCandidates
}

func validateEntryCallReportSurface(
	surface EntryCallReportSurface,
	openable map[string]struct{},
) error {
	if !surface.Form.Valid() || !validEntryCallReportLocation(surface.Site) {
		return fmt.Errorf("form or site is invalid")
	}
	if _, authorized := openable[surface.Site.Path]; !authorized {
		return fmt.Errorf("site is not openable")
	}
	if surface.Origin != EntryCallSurfaceOriginModelAssisted ||
		surface.RuntimeReachability != EntryCallSurfaceRuntimeReachabilityUnknown {
		return fmt.Errorf("truth state is invalid")
	}
	values := []*EntryCallReportSurfaceValue{
		surface.Identity,
		surface.Method,
		surface.Path,
		surface.Handler,
	}
	for _, value := range values {
		if err := validateEntryCallReportSurfaceValue(value, openable); err != nil {
			return err
		}
	}
	switch surface.Kind {
	case entrycall.SurfaceKindHTTPRoute:
		if surface.Form != entrycall.SurfaceCandidateDirectCall || surface.Identity != nil ||
			surface.Path == nil || surface.Path.Kind != entrycall.SurfaceFactString ||
			!strings.HasPrefix(surface.Path.Text, "/") ||
			(surface.Method != nil && surface.Method.Kind != entrycall.SurfaceFactString &&
				surface.Method.Kind != entrycall.SurfaceFactToken) {
			return fmt.Errorf("HTTP route contract is invalid")
		}
		withHandler := surface.Role == entrycall.SurfaceRoleEntrySurface &&
			surface.State == EntryCallSurfaceStateExactRegistration &&
			surface.Handler != nil && surface.Handler.Kind == entrycall.SurfaceFactCallable &&
			surface.Handler.Location != nil
		descriptor := surface.Role == entrycall.SurfaceRoleDescriptor &&
			surface.State == EntryCallSurfaceStateDeclaredDescriptor && surface.Handler == nil
		if !withHandler && !descriptor {
			return fmt.Errorf("HTTP route contract is invalid")
		}
	case entrycall.SurfaceKindCLICommand:
		if surface.Role != entrycall.SurfaceRoleDescriptor ||
			surface.Form != entrycall.SurfaceCandidateKeyedComposite ||
			surface.State != EntryCallSurfaceStateDeclaredDescriptor ||
			surface.Identity == nil || surface.Identity.Kind == entrycall.SurfaceFactCallable ||
			surface.Method != nil || surface.Path != nil ||
			(surface.Handler != nil &&
				(surface.Handler.Kind != entrycall.SurfaceFactCallable || surface.Handler.Location == nil)) {
			return fmt.Errorf("CLI descriptor contract is invalid")
		}
	case entrycall.SurfaceKindScheduledJob:
		// Identity is the exact stable job name restored by the backend, or its
		// exact schedule when the registration has no separate name. It is not a
		// route path, so the existing generic identity field keeps projection v2
		// sufficient and leaves method/path unset.
		if surface.Form != entrycall.SurfaceCandidateDirectCall ||
			surface.Identity == nil || surface.Identity.Kind != entrycall.SurfaceFactString ||
			surface.Identity.Location == nil ||
			surface.Method != nil || surface.Path != nil {
			return fmt.Errorf("scheduled job contract is invalid")
		}
		withHandler := surface.Role == entrycall.SurfaceRoleEntrySurface &&
			surface.State == EntryCallSurfaceStateExactRegistration &&
			surface.Handler != nil && surface.Handler.Kind == entrycall.SurfaceFactCallable &&
			surface.Handler.Location != nil
		descriptor := surface.Role == entrycall.SurfaceRoleDescriptor &&
			surface.State == EntryCallSurfaceStateDeclaredDescriptor && surface.Handler == nil
		if !withHandler && !descriptor {
			return fmt.Errorf("scheduled job contract is invalid")
		}
	default:
		return fmt.Errorf("kind is invalid")
	}
	return nil
}

func validateEntryCallReportSurfaceValue(
	value *EntryCallReportSurfaceValue,
	openable map[string]struct{},
) error {
	if value == nil {
		return nil
	}
	if !value.Kind.Valid() || value.Text == "" || strings.TrimSpace(value.Text) != value.Text ||
		utf8.RuneCountInString(value.Text) > entrycall.MaxSurfaceFactValueRunes {
		return fmt.Errorf("surface value is invalid")
	}
	for _, character := range value.Text {
		if unicode.IsControl(character) {
			return fmt.Errorf("surface value contains control characters")
		}
	}
	if value.Location != nil {
		if !validEntryCallReportLocation(*value.Location) {
			return fmt.Errorf("surface value location is invalid")
		}
		if _, authorized := openable[value.Location.Path]; !authorized {
			return fmt.Errorf("surface value location is not openable")
		}
	}
	return nil
}

func validEntryCallReportSurfaceID(value string) bool {
	const prefix = "model-surface-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+24 {
		return false
	}
	for _, character := range value[len(prefix):] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return true
}

func validEntryCallReportLocation(location entrycall.Location) bool {
	return location.Line > 0 && location.Column >= 0 && validateManifestPath(location.Path) == nil
}

func validEntryCallReportLabel(value string) bool {
	if value == "" || strings.TrimSpace(value) != value ||
		strings.Join(strings.Fields(value), " ") != value ||
		utf8.RuneCountInString(value) > entrycall.MaxLabelRunes {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '/' || character == '\\' {
			return false
		}
	}
	return true
}

func entryCallRootKey(path string, line int) string {
	return fmt.Sprintf("%s\x00%d", path, line)
}

func readOptionalEntryCallArtifact(
	root *os.Root,
	name string,
) ([]byte, bool, error) {
	if _, err := root.Lstat(name); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("entry call report: inspect %s: %w", name, err)
	}
	data, err := readManifestFile(root, name, entrycall.MaxArtifactBytes)
	if err != nil {
		return nil, false, fmt.Errorf("entry call report: %w", err)
	}
	return data, true, nil
}

// readOptionalEntryCallArtifactPair selects one exact versioned pair. A
// current partial pair is never completed from legacy bytes; legacy v2 is
// consulted only when both current artifacts are absent.
func readOptionalEntryCallArtifactPair(
	root *os.Root,
) (statusRaw []byte, hasStatus bool, resultRaw []byte, hasResult bool, err error) {
	statusRaw, hasStatus, err = readOptionalEntryCallArtifact(root, entrycall.StatusArtifactFilename)
	if err != nil {
		return nil, false, nil, false, err
	}
	resultRaw, hasResult, err = readOptionalEntryCallArtifact(root, entrycall.ResultArtifactFilename)
	if err != nil {
		return nil, false, nil, false, err
	}
	if hasStatus || hasResult {
		return statusRaw, hasStatus, resultRaw, hasResult, nil
	}
	statusRaw, hasStatus, err = readOptionalEntryCallArtifact(root, entrycall.LegacyV2StatusArtifactFilename)
	if err != nil {
		return nil, false, nil, false, err
	}
	resultRaw, hasResult, err = readOptionalEntryCallArtifact(root, entrycall.LegacyV2ResultArtifactFilename)
	if err != nil {
		return nil, false, nil, false, err
	}
	return statusRaw, hasStatus, resultRaw, hasResult, nil
}

func entryCallReportMaterial(
	data *ReportData,
	repositoryStateSHA256 string,
) (string, string, error) {
	if data == nil {
		return "", "", fmt.Errorf("report data is required")
	}
	binding := data.entryCallArtifactBinding
	if binding == nil {
		if data.EntryCall != nil {
			return "", "", fmt.Errorf("projection lacks accepted artifact binding")
		}
		return "", "", nil
	}
	if !validManifestSHA256(binding.statusSHA256) ||
		!validManifestSHA256(binding.resultSHA256) ||
		binding.repositoryStateSHA256 != repositoryStateSHA256 {
		return "", "", fmt.Errorf("accepted artifact binding is invalid")
	}
	if data.EntryCall == nil {
		if data.AnalysisTarget == nil ||
			(data.AnalysisTarget.Kind != analysistarget.KindLibraryPackage &&
				data.AnalysisTarget.Kind != analysistarget.KindModuleLibrary) {
			return "", "", fmt.Errorf("accepted executable artifacts lack projection")
		}
	} else if err := validateEntryCallReportProjection(
		data.AnalysisTarget,
		data.EntryCall,
		data.OpenablePaths,
	); err != nil {
		return "", "", err
	}
	return binding.statusSHA256, binding.resultSHA256, nil
}

// VerifyEntryCallArtifacts binds accepted entry-call artifact bytes to the exact
// repository state and re-derives the public flat projection. A closed status
// may remain as optional debug state, but it must not carry a result or report
// projection.
func (m RunManifest) VerifyEntryCallArtifacts(runDir string, reportJSON []byte) error {
	if m.Version != CurrentRunManifestVersion {
		return fmt.Errorf("report manifest: unsupported version %d", m.Version)
	}
	var persisted struct {
		AnalysisTarget *analysistarget.Target     `json:"analysis_target"`
		EntryCall      *EntryCallReportProjection `json:"entry_call"`
		OpenablePaths  []string                   `json:"openable_paths"`
	}
	if err := json.Unmarshal(reportJSON, &persisted); err != nil {
		return fmt.Errorf("report manifest: decode entry call projection: %w", err)
	}

	root, err := os.OpenRoot(runDir)
	if err != nil {
		return fmt.Errorf("report manifest: open entry call run: %w", err)
	}
	defer root.Close()
	statusRaw, hasStatus, resultRaw, hasResult, err := readOptionalEntryCallArtifactPair(root)
	if err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}

	material := m.MaterialInputs
	bound := material.EntryCallStatusSHA256 != ""
	if !bound {
		if persisted.EntryCall != nil {
			return fmt.Errorf("report manifest: entry call projection lacks artifact authority")
		}
		if !hasStatus {
			if hasResult {
				return fmt.Errorf("report manifest: unbound entry call result is present")
			}
			return nil
		}
		status, decodeErr := entrycall.DecodeStatus(statusRaw)
		if decodeErr != nil {
			return fmt.Errorf("report manifest: entry call status: %w", decodeErr)
		}
		if status.State == entrycall.StatusAccepted || status.State == entrycall.StatusAcceptedPartial {
			return fmt.Errorf("report manifest: accepted entry call status is unbound")
		}
		if hasResult {
			return fmt.Errorf("report manifest: closed entry call status has an unexpected result")
		}
		return nil
	}
	if !hasStatus || !hasResult ||
		manifestSHA256(statusRaw) != material.EntryCallStatusSHA256 ||
		manifestSHA256(resultRaw) != material.EntryCallResultSHA256 {
		return fmt.Errorf("report manifest: entry call artifact sha256 mismatch")
	}
	projection, _, err := acceptedEntryCallReportProjection(
		statusRaw,
		resultRaw,
		persisted.AnalysisTarget,
		m.RepositoryStateSHA256,
	)
	if err != nil {
		return fmt.Errorf("report manifest: entry call artifacts: %w", err)
	}
	if err := validateEntryCallReportProjection(
		persisted.AnalysisTarget,
		projection,
		persisted.OpenablePaths,
	); err != nil {
		return fmt.Errorf("report manifest: %w", err)
	}
	if !reflect.DeepEqual(projection, persisted.EntryCall) {
		return fmt.Errorf("report manifest: entry call artifacts do not match report")
	}
	return nil
}
