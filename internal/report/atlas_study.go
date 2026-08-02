package report

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"os"
	"sort"
	"strings"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/studymap"
)

// BuildAtlasStudyInput is the one shared adapter used by the runtime producer
// and report replay. It derives the exact Atlas-first Study input only from
// persisted Atlas, accepted Architecture, Surface and saved-source values.
// Legacy Orientation and repository_study_map artifacts are deliberately not
// inputs to this projection.
func BuildAtlasStudyInput(
	data *ReportData,
	language atlasstudy.Language,
) (atlasstudy.Input, error) {
	if data == nil || data.RepositoryAtlas == nil {
		return atlasstudy.Input{}, fmt.Errorf("atlas study report: repository Atlas is required")
	}
	if data.ArchitectureCanvas == nil {
		return atlasstudy.Input{}, fmt.Errorf("atlas study report: architecture canvas is required")
	}
	if err := validateAcceptedAtlasStudyArchitecture(data); err != nil {
		return atlasstudy.Input{}, err
	}
	atlas, err := repositoryatlas.Canonical(*data.RepositoryAtlas)
	if err != nil {
		return atlasstudy.Input{}, fmt.Errorf("atlas study report: repository Atlas: %w", err)
	}

	input := atlasstudy.Input{
		Atlas: atlas, Language: language, Limits: atlasstudy.DefaultLimits(),
		Architecture: atlasstudy.ArchitectureInput{
			Version:  data.ArchitectureCanvas.Version,
			Source:   string(data.ArchitectureCanvas.ArchitectureSource),
			Title:    strings.TrimSpace(data.ArchitectureCanvas.Title),
			Subtitle: strings.TrimSpace(data.ArchitectureCanvas.Subtitle),
		},
	}
	for _, subsystem := range data.ArchitectureCanvas.Subsystems {
		input.Architecture.Subsystems = append(input.Architecture.Subsystems, atlasstudy.Subsystem{
			ID: string(subsystem.ID), Name: strings.TrimSpace(subsystem.Name),
			Description:  strings.TrimSpace(subsystem.Description),
			Authority:    repositoryatlas.AuthorityInferred,
			ComponentIDs: atlasStudyComponentIDStrings(subsystem.ComponentIDs),
		})
	}
	for _, component := range data.ArchitectureCanvas.Components {
		input.Architecture.Components = append(input.Architecture.Components, atlasstudy.Component{
			ID: string(component.ID), SubsystemID: string(component.SubsystemID),
			Name: strings.TrimSpace(component.Name), Description: strings.TrimSpace(component.Description),
			Authority: repositoryatlas.AuthorityInferred,
		})
	}
	sort.Slice(input.Architecture.Subsystems, func(i, j int) bool {
		return input.Architecture.Subsystems[i].ID < input.Architecture.Subsystems[j].ID
	})
	sort.Slice(input.Architecture.Components, func(i, j int) bool {
		return input.Architecture.Components[i].ID < input.Architecture.Components[j].ID
	})

	input.Surfaces = atlasStudySurfaces(data, atlas)
	input.ReadingTargets = atlasStudyReadingTargets(data, input.Surfaces)
	bindAtlasStudyReadingTargets(&input)
	input.Evidence = atlasStudyEvidence(atlas, input.Surfaces)
	if claim := atlasStudyDocumentedPurpose(data.DocumentedPurpose); claim != "" {
		input.Documents = []atlasstudy.DocumentClaim{{
			ID:    "documented-purpose-" + atlasStudyDigest(claim),
			Label: "Repository documentation", Claim: claim,
			Authority: repositoryatlas.AuthorityObserved,
		}}
	}
	return input, nil
}

func validateAcceptedAtlasStudyArchitecture(data *ReportData) error {
	canvas := data.ArchitectureCanvas
	status := data.ArchitectureSynthesis
	if status == nil {
		return fmt.Errorf("atlas study report: accepted Architecture status is required")
	}
	if err := status.Validate(); err != nil {
		return fmt.Errorf("atlas study report: Architecture status: %w", err)
	}
	if (status.State != ArchitectureSynthesisSucceeded && status.State != ArchitectureSynthesisCached) ||
		!status.ProposalAccepted || status.ProposalRejected || status.FallbackSelected ||
		canvas.Fallback ||
		(canvas.ValidationOutcome != componentmap.ValidationAccepted &&
			canvas.ValidationOutcome != componentmap.ValidationAcceptedNormalized) ||
		(canvas.ArchitectureSource != componentmap.SourceValidatedModel &&
			canvas.ArchitectureSource != componentmap.SourceNormalizedModel) ||
		status.ArchitectureSource != string(canvas.ArchitectureSource) ||
		status.ArchitectureLevel != canvas.ArchitectureLevel {
		return fmt.Errorf("atlas study report: Architecture is not an accepted model result")
	}
	return nil
}

func atlasStudyComponentIDStrings(values []componentmap.ComponentID) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, string(value))
		}
	}
	sort.Strings(result)
	return result
}

func atlasStudySurfaces(data *ReportData, atlas repositoryatlas.Atlas) []atlasstudy.Surface {
	unitBySurface := make(map[string]string)
	for _, entity := range atlas.Entities {
		if entity.Kind == repositoryatlas.EntitySurface && entity.ID != "" {
			unitBySurface[entity.ID] = entity.UnitID
		}
	}
	result := make([]atlasstudy.Surface, 0, len(data.ArchitectureCanvas.Surfaces))
	for _, surface := range data.ArchitectureCanvas.Surfaces {
		unitID := unitBySurface[surface.ID]
		if surface.ID == "" || unitID == "" {
			continue
		}
		result = append(result, atlasstudy.Surface{
			ID: surface.ID, UnitID: unitID, Name: strings.TrimSpace(surface.Name),
			Kind: surface.Kind, Authority: atlasStudySurfaceAuthority(atlas, surface),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func atlasStudySurfaceAuthority(
	atlas repositoryatlas.Atlas,
	surface ArchitectureSurface,
) repositoryatlas.Authority {
	switch surface.Resolution {
	case "partial", "dynamic", "provisional":
		return repositoryatlas.AuthorityPartial
	}
	var authority repositoryatlas.Authority
	for _, relation := range atlas.Relations {
		if relation.Kind != repositoryatlas.RelationExposes ||
			relation.Source.Kind != repositoryatlas.EntitySurface ||
			relation.Source.ID != surface.ID {
			continue
		}
		if authority == "" {
			authority = relation.Authority
		} else if authority != relation.Authority {
			return repositoryatlas.AuthorityConflicted
		}
	}
	if authority != "" {
		return authority
	}
	if surface.Resolution == "exact" {
		return repositoryatlas.AuthorityResolved
	}
	return repositoryatlas.AuthorityUnknown
}

func atlasStudyReadingTargets(
	data *ReportData,
	surfaces []atlasstudy.Surface,
) []atlasstudy.ReadingTarget {
	if data == nil || data.ArchitectureCanvas == nil {
		return nil
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, sourcePath := range data.OpenablePaths {
		openable[sourcePath] = struct{}{}
	}
	surfaceByID := make(map[string]atlasstudy.Surface, len(surfaces))
	for _, surface := range surfaces {
		surfaceByID[surface.ID] = surface
	}
	sources := append([]SourceSnippet(nil), data.UserSources...)
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Path != sources[j].Path {
			return sources[i].Path < sources[j].Path
		}
		left, right := atlasStudySourceFocusLine(sources[i]), atlasStudySourceFocusLine(sources[j])
		if left != right {
			return left < right
		}
		return sources[i].PresentationSHA256 < sources[j].PresentationSHA256
	})
	result := make([]atlasstudy.ReadingTarget, 0, len(sources))
	seen := make(map[string]struct{})
	for _, source := range sources {
		line := atlasStudySourceFocusLine(source)
		if _, ok := openable[source.Path]; !ok || line <= 0 || source.Validate() != nil {
			continue
		}
		owners := atlasStudyReadingOwners(data, surfaceByID, source.Path, line)
		if len(owners) == 0 {
			continue
		}
		for _, owned := range owners {
			kind := atlasStudyReadingTargetKind(source.EnclosingSymbol, owned.surfaceKind)
			id := "reading-target-" + atlasStudyDigest(strings.Join([]string{
				string(owned.ref.Kind), owned.ref.ID, string(kind), source.Path,
				fmt.Sprint(line), source.EnclosingSymbol,
			}, "\x00"))
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			label, fact := atlasStudyReadingTargetText(data, owned.ref, owned.surfaceKind)
			result = append(result, atlasstudy.ReadingTarget{
				ID: id, Owner: owned.ref, Kind: kind, Label: label, Fact: fact,
				Authority: atlasStudyReadingTargetAuthority(owned.ref, surfaceByID),
				Location:  evidence.Location{Path: source.Path, Line: line},
				Symbol:    source.EnclosingSymbol,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func atlasStudyReadingTargetAuthority(
	owner atlasstudy.CanonicalRef,
	surfaces map[string]atlasstudy.Surface,
) repositoryatlas.Authority {
	if owner.Kind == atlasstudy.RefSurface {
		if surface, ok := surfaces[owner.ID]; ok {
			return surface.Authority
		}
		return repositoryatlas.AuthorityUnknown
	}
	if owner.Kind == atlasstudy.RefComponent {
		return repositoryatlas.AuthorityInferred
	}
	return repositoryatlas.AuthorityUnknown
}

type atlasStudyReadingOwner struct {
	ref         atlasstudy.CanonicalRef
	surfaceKind string
}

func atlasStudyReadingOwners(
	data *ReportData,
	surfaces map[string]atlasstudy.Surface,
	sourcePath string,
	line int,
) []atlasStudyReadingOwner {
	result := make([]atlasStudyReadingOwner, 0, 2)
	seen := make(map[atlasstudy.CanonicalRef]struct{})
	for _, surface := range data.ArchitectureCanvas.Surfaces {
		if _, advertised := surfaces[surface.ID]; !advertised {
			continue
		}
		for _, location := range surface.Evidence {
			if location.Path == sourcePath && location.Line == line {
				ref := atlasstudy.CanonicalRef{Kind: atlasstudy.RefSurface, ID: surface.ID}
				if _, duplicate := seen[ref]; !duplicate {
					seen[ref] = struct{}{}
					result = append(result, atlasStudyReadingOwner{ref: ref, surfaceKind: surface.Kind})
				}
				break
			}
		}
	}
	for _, component := range data.ArchitectureCanvas.Components {
		if !atlasStudyComponentOwnsPath(data, component, sourcePath) {
			continue
		}
		ref := atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: string(component.ID)}
		if _, duplicate := seen[ref]; !duplicate {
			seen[ref] = struct{}{}
			result = append(result, atlasStudyReadingOwner{ref: ref})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ref.Kind != result[j].ref.Kind {
			return result[i].ref.Kind < result[j].ref.Kind
		}
		return result[i].ref.ID < result[j].ref.ID
	})
	return result
}

func atlasStudyComponentOwnsPath(
	data *ReportData,
	component ArchitectureComponent,
	sourcePath string,
) bool {
	packages := make(map[string]PackageInfo)
	if data.RepositoryGraph != nil {
		for _, pkg := range data.RepositoryGraph.Packages {
			if pkg.CanonicalPath != "" {
				packages[pkg.CanonicalPath] = pkg
			}
		}
	}
	for _, member := range component.Members {
		for _, fact := range member.Facts {
			if fact.Location != nil && fact.Location.Path == sourcePath {
				return true
			}
			if fact.Kind == componentmap.FactRepositoryPath && fact.Value == sourcePath {
				return true
			}
			if member.ID.Kind != componentmap.MemberPackage || fact.Kind != componentmap.FactDeclaration {
				continue
			}
			pkg, ok := packages[fact.Value]
			if !ok {
				continue
			}
			for _, file := range pkg.Files {
				if file == sourcePath {
					return true
				}
			}
		}
	}
	return false
}

func atlasStudySourceFocusLine(source SourceSnippet) int {
	if len(source.HighlightRanges) > 0 {
		return source.HighlightRanges[0].StartLine
	}
	for _, line := range source.Lines {
		if line.Highlight {
			return line.Line
		}
	}
	return source.StartLine
}

func atlasStudyReadingTargetKind(symbol, surfaceKind string) atlasstudy.ReadingTargetKind {
	if surfaceKind == "process_entry" {
		return atlasstudy.ReadingTargetEntrypoint
	}
	if strings.Contains(symbol, ").") {
		return atlasstudy.ReadingTargetMethod
	}
	if strings.TrimSpace(symbol) != "" {
		return atlasstudy.ReadingTargetFunction
	}
	return atlasstudy.ReadingTargetFile
}

func atlasStudyReadingTargetText(
	data *ReportData,
	owner atlasstudy.CanonicalRef,
	surfaceKind string,
) (string, string) {
	if owner.Kind == atlasstudy.RefSurface {
		for _, surface := range data.ArchitectureCanvas.Surfaces {
			if surface.ID == owner.ID {
				label := strings.TrimSpace(surface.Name)
				if label == "" {
					label = surface.Kind
				}
				return label, strings.TrimSpace(strings.Join([]string{
					surfaceKind, surface.Status, surface.Resolution,
				}, " "))
			}
		}
	}
	for _, component := range data.ArchitectureCanvas.Components {
		if string(component.ID) != owner.ID {
			continue
		}
		fact := strings.TrimSpace(component.Description)
		if fact == "" {
			fact = "Exact local source owned by this architecture component."
		}
		return strings.TrimSpace(component.Name), fact
	}
	return "Repository source", "Exact locally saved source."
}

func bindAtlasStudyReadingTargets(input *atlasstudy.Input) {
	if input == nil {
		return
	}
	byOwner := make(map[atlasstudy.CanonicalRef][]string)
	for _, target := range input.ReadingTargets {
		byOwner[target.Owner] = append(byOwner[target.Owner], target.ID)
	}
	for index := range input.Architecture.Components {
		owner := atlasstudy.CanonicalRef{
			Kind: atlasstudy.RefComponent, ID: input.Architecture.Components[index].ID,
		}
		input.Architecture.Components[index].ReadingTargetIDs = append(
			[]string(nil), byOwner[owner]...,
		)
		sort.Strings(input.Architecture.Components[index].ReadingTargetIDs)
	}
	for index := range input.Surfaces {
		owner := atlasstudy.CanonicalRef{Kind: atlasstudy.RefSurface, ID: input.Surfaces[index].ID}
		input.Surfaces[index].ReadingTargetIDs = append([]string(nil), byOwner[owner]...)
		sort.Strings(input.Surfaces[index].ReadingTargetIDs)
	}
}

func atlasStudyEvidence(
	atlas repositoryatlas.Atlas,
	surfaces []atlasstudy.Surface,
) []atlasstudy.EvidenceFact {
	knownSurfaces := make(map[string]struct{}, len(surfaces))
	for _, surface := range surfaces {
		knownSurfaces[surface.ID] = struct{}{}
	}
	type evidenceSubjects struct {
		authority repositoryatlas.Authority
		refs      map[atlasstudy.CanonicalRef]struct{}
		fact      string
	}
	byEvidence := make(map[string]*evidenceSubjects, len(atlas.Evidence))
	add := func(
		evidenceIDs []string,
		authority repositoryatlas.Authority,
		fact string,
		refs ...atlasstudy.CanonicalRef,
	) {
		for _, evidenceID := range evidenceIDs {
			entry := byEvidence[evidenceID]
			if entry == nil {
				entry = &evidenceSubjects{
					authority: authority, refs: make(map[atlasstudy.CanonicalRef]struct{}), fact: fact,
				}
				byEvidence[evidenceID] = entry
			}
			for _, ref := range refs {
				entry.refs[ref] = struct{}{}
			}
		}
	}
	for _, observation := range atlas.Observations {
		if observation.Subject.Kind != repositoryatlas.EntitySurface {
			continue
		}
		if _, ok := knownSurfaces[observation.Subject.ID]; !ok {
			continue
		}
		add(
			observation.EvidenceRefs,
			repositoryatlas.AuthorityObserved,
			"Local Atlas observation with exact saved evidence.",
			atlasstudy.CanonicalRef{Kind: atlasstudy.RefUnit, ID: observation.UnitID},
			atlasstudy.CanonicalRef{Kind: atlasstudy.RefSurface, ID: observation.Subject.ID},
		)
	}
	for _, relation := range atlas.Relations {
		if relation.Source.Kind != repositoryatlas.EntitySurface {
			continue
		}
		if _, ok := knownSurfaces[relation.Source.ID]; !ok {
			continue
		}
		add(
			relation.EvidenceRefs,
			relation.Authority,
			strings.TrimSpace(fmt.Sprintf("%s relation during %s", relation.Kind, relation.Phase)),
			atlasstudy.CanonicalRef{Kind: atlasstudy.RefUnit, ID: relation.UnitID},
			atlasstudy.CanonicalRef{Kind: atlasstudy.RefSurface, ID: relation.Source.ID},
		)
	}
	result := make([]atlasstudy.EvidenceFact, 0, len(byEvidence))
	for _, item := range atlas.Evidence {
		entry := byEvidence[item.ID]
		if entry == nil || len(entry.refs) == 0 {
			continue
		}
		refs := make([]atlasstudy.CanonicalRef, 0, len(entry.refs))
		for ref := range entry.refs {
			refs = append(refs, ref)
		}
		sort.Slice(refs, func(i, j int) bool {
			if refs[i].Kind != refs[j].Kind {
				return refs[i].Kind < refs[j].Kind
			}
			return refs[i].ID < refs[j].ID
		})
		result = append(result, atlasstudy.EvidenceFact{
			ID: item.ID, SubjectRefs: refs, Authority: entry.authority, Fact: entry.fact,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func readAtlasStudyReportProduct(
	runDir string,
	data *ReportData,
) (*AtlasStudyReportStatus, *RepositoryStudyMap, error) {
	root, err := os.OpenRoot(runDir)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: open run directory: %w", err)
	}
	defer root.Close()
	requestRaw, hasRequest, err := readOptionalAtlasStudyArtifact(
		root, atlasstudy.RequestArtifactFilename, atlasstudy.MaxRequestArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	resultRaw, hasResult, err := readOptionalAtlasStudyArtifact(
		root, atlasstudy.ResultArtifactFilename, atlasstudy.MaxResultArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	statusRaw, hasStatus, err := readOptionalAtlasStudyArtifact(
		root, atlasstudy.StatusArtifactFilename, atlasstudy.MaxStatusArtifactBytes,
	)
	if err != nil {
		return nil, nil, err
	}
	if !hasRequest && !hasResult && !hasStatus {
		return uncalledAtlasStudyReportStatus(data), nil, nil
	}
	if !hasRequest || !hasStatus {
		return nil, nil, fmt.Errorf("atlas study report: artifact set requires request and status")
	}
	request, err := atlasstudy.DecodeRequestRecord(requestRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: request: %w", err)
	}
	status, err := atlasstudy.DecodeStatus(statusRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("atlas study report: status: %w", err)
	}
	input, err := BuildAtlasStudyInput(data, request.Language)
	if err != nil {
		return nil, nil, err
	}
	if err := atlasstudy.ValidateRequestRecordAgainstInput(request, input); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: request binding: %w", err)
	}
	if err := atlasstudy.ValidateStatusAgainstInput(status, input); err != nil {
		return nil, nil, fmt.Errorf("atlas study report: status binding: %w", err)
	}
	reportStatus := &AtlasStudyReportStatus{
		Version: status.Version, State: status.State,
		UnavailableCode: AtlasStudyUnavailableCode(status.UnavailableCode),
		FailureCode:     status.FailureCode, DirectionCount: status.DirectionCount,
	}
	switch status.State {
	case atlasstudy.ProductStateAccepted:
		if !hasResult {
			return nil, nil, fmt.Errorf("atlas study report: accepted status requires result")
		}
		result, decodeErr := atlasstudy.DecodeResultRecord(resultRaw)
		if decodeErr != nil {
			return nil, nil, fmt.Errorf("atlas study report: result: %w", decodeErr)
		}
		if validateErr := atlasstudy.ValidateResultRecordAgainstInput(result, input); validateErr != nil {
			return nil, nil, fmt.Errorf("atlas study report: result binding: %w", validateErr)
		}
		if status.DirectionCount != len(result.Directions) {
			return nil, nil, fmt.Errorf("atlas study report: result/status direction count mismatch")
		}
		studyMap, projectErr := projectAtlasStudyMap(data, input, result)
		if projectErr != nil {
			return nil, nil, projectErr
		}
		return reportStatus, studyMap, nil
	case atlasstudy.ProductStateUnavailable, atlasstudy.ProductStateFailed:
		if hasResult {
			return nil, nil, fmt.Errorf("atlas study report: terminal non-accepted status cannot contain result")
		}
		return reportStatus, nil, nil
	case atlasstudy.ProductStatePrepared:
		return nil, nil, fmt.Errorf("atlas study report: prepared status is not publishable")
	default:
		return nil, nil, fmt.Errorf("atlas study report: unsupported state %q", status.State)
	}
}

func readOptionalAtlasStudyArtifact(
	root *os.Root,
	name string,
	limit int,
) ([]byte, bool, error) {
	if _, err := root.Lstat(name); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("atlas study report: inspect %s: %w", name, err)
	}
	data, err := readManifestFile(root, name, limit)
	if err != nil {
		return nil, false, fmt.Errorf("atlas study report: %w", err)
	}
	if _, found := secretscan.DetectAlways(string(data)); found {
		return nil, false, fmt.Errorf("atlas study report: %s contains an obvious credential", name)
	}
	return data, true, nil
}

func uncalledAtlasStudyReportStatus(data *ReportData) *AtlasStudyReportStatus {
	if data == nil || data.ArchitectureSynthesis == nil || data.RepositoryAtlas == nil {
		return nil
	}
	status := data.ArchitectureSynthesis
	if status.State == ArchitectureSynthesisUnavailable && status.UnavailableCode == "offline" {
		return &AtlasStudyReportStatus{
			Version: atlasstudy.Version, State: atlasstudy.ProductStateUnavailable,
			UnavailableCode: AtlasStudyUnavailableOffline,
		}
	}
	if status.State == ArchitectureSynthesisFailed ||
		((status.State == ArchitectureSynthesisSucceeded || status.State == ArchitectureSynthesisCached) &&
			(!status.ProposalAccepted || status.ProposalRejected || status.FallbackSelected)) {
		return &AtlasStudyReportStatus{
			Version: atlasstudy.Version, State: atlasstudy.ProductStateUnavailable,
			UnavailableCode: AtlasStudyUnavailableArchitectureEnrichment,
		}
	}
	return nil
}

func projectAtlasStudyMap(
	data *ReportData,
	input atlasstudy.Input,
	result atlasstudy.ResultRecord,
) (*RepositoryStudyMap, error) {
	components := make(map[string]ArchitectureComponent, len(data.ArchitectureCanvas.Components))
	for _, component := range data.ArchitectureCanvas.Components {
		id := string(component.ID)
		if _, duplicate := components[id]; duplicate {
			return nil, fmt.Errorf("atlas study report: duplicate Architecture component %q", id)
		}
		components[id] = component
	}
	targets := make(map[string]atlasstudy.ReadingTarget, len(input.ReadingTargets))
	for _, target := range input.ReadingTargets {
		targets[target.ID] = target
	}
	area := func(ref atlasstudy.CanonicalRef) (RepositoryStudyArea, error) {
		component, ok := components[ref.ID]
		if ref.Kind != atlasstudy.RefComponent || !ok {
			return RepositoryStudyArea{}, fmt.Errorf("atlas study report: Shape references unavailable component")
		}
		projected := RepositoryStudyArea{
			ID: ref.ID, Name: component.Name, Responsibility: component.Description,
			MapTarget: &UserMapTarget{
				Kind: SemanticSearchTargetComponent, ComponentID: componentmap.ComponentID(ref.ID),
			},
		}
		for _, target := range input.ReadingTargets {
			if target.Owner != ref {
				continue
			}
			source, sourceErr := exactAtlasStudySource(data, target)
			if sourceErr != nil {
				return RepositoryStudyArea{}, sourceErr
			}
			projected.CodeLocation = &UserCodeLocation{
				Path: target.Location.Path, Line: target.Location.Line,
			}
			projected.Source = &source
			break
		}
		return projected, nil
	}
	studyMap := &RepositoryStudyMap{
		Version: result.Version, RepositoryType: studymap.RepositoryType(result.RepositoryType),
		Brief: RepositoryBrief{
			WhatItIs: result.Brief.WhatItIs.Text, Problem: result.Brief.Problem.Text,
			MainInput:             result.Brief.MainInput.Text,
			CentralResponsibility: result.Brief.CentralResponsibility.Text,
			ObservableResult:      result.Brief.ObservableResult.Text,
		},
	}
	for _, term := range result.Brief.DomainTerms {
		studyMap.Brief.DomainTerms = append(studyMap.Brief.DomainTerms, RepositoryBriefDomainTerm{
			Term: term.Term, Meaning: term.Meaning,
		})
	}
	for _, ref := range result.ShapeComponentRefs {
		projected, err := area(ref)
		if err != nil {
			return nil, err
		}
		studyMap.Shape = append(studyMap.Shape, projected)
	}
	for _, direction := range result.Directions {
		projected := StudyDirection{
			ID: direction.ID, Question: direction.Question,
			WhyItMatters: direction.WhyItMatters, LearningOutcome: direction.LearningOutcome,
			TargetUserJob: studymap.TargetJob(direction.TargetJob),
			LearningStage: studymap.LearningStage(direction.LearningStage),
		}
		seenAnchors := make(map[StudyCodeAnchor]struct{})
		for _, reading := range direction.Reading {
			target, ok := targets[reading.Target.ID]
			if reading.Target.Kind != atlasstudy.RefReadingTarget || !ok {
				return nil, fmt.Errorf("atlas study report: direction references unavailable reading target")
			}
			source, err := exactAtlasStudySource(data, target)
			if err != nil {
				return nil, err
			}
			projected.ReadingAnchors = append(projected.ReadingAnchors, StudyReadingAnchor{
				Label: string(reading.Label), WhatToLookFor: reading.WhatToLookFor,
				Location: UserCodeLocation{Path: target.Location.Path, Line: target.Location.Line},
				Source:   source,
			})
			anchor := StudyCodeAnchor{
				Path: target.Location.Path, Symbol: target.Symbol, Line: target.Location.Line,
			}
			if _, duplicate := seenAnchors[anchor]; !duplicate {
				seenAnchors[anchor] = struct{}{}
				projected.PrincipalAnchors = append(projected.PrincipalAnchors, anchor)
			}
		}
		for _, principal := range direction.PrincipalRefs {
			if principal.Kind != atlasstudy.RefComponent {
				continue
			}
			projectedArea, err := area(principal)
			if err != nil {
				return nil, err
			}
			projected.Areas = append(projected.Areas, projectedArea)
		}
		projected.DebugCoverage = studyDirectionCoverage(projected)
		studyMap.Directions = append(studyMap.Directions, projected)
	}
	return studyMap, nil
}

func exactAtlasStudySource(
	data *ReportData,
	target atlasstudy.ReadingTarget,
) (SourceSnippet, error) {
	var matches []SourceSnippet
	for _, source := range data.UserSources {
		if source.Path != target.Location.Path ||
			atlasStudySourceFocusLine(source) != target.Location.Line ||
			source.EnclosingSymbol != target.Symbol || source.Validate() != nil {
			continue
		}
		matches = append(matches, source)
	}
	if len(matches) != 1 {
		return SourceSnippet{}, fmt.Errorf(
			"atlas study report: reading target %q has %d exact saved sources", target.ID, len(matches),
		)
	}
	return matches[0], nil
}

func atlasStudyDocumentedPurpose(value string) string {
	value = strings.NewReplacer("<br>", " ", "<br/>", " ", "<br />", " ").Replace(value)
	value = html.UnescapeString(value)
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func atlasStudyDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:12])
}
