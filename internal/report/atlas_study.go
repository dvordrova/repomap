package report

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"html"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dvordrova/repomap/internal/atlasstudy"
	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/repositoryatlas"
	"github.com/dvordrova/repomap/internal/secretscan"
	"github.com/dvordrova/repomap/internal/studymap"
)

const AtlasStudyUnavailableInsufficientCatalog AtlasStudyUnavailableCode = "insufficient_catalog"

// BuildAtlasStudyInput is the one shared adapter used by the runtime producer
// and report replay. It derives the exact Atlas-first Study input only from
// persisted Atlas, the canonical visible Architecture Canvas, Surface and saved-source values.
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
	if err := validateUsableAtlasStudyCanvas(data); err != nil {
		return atlasstudy.Input{}, err
	}
	studyData, err := atlasStudyLocalD177Data(data)
	if err != nil {
		return atlasstudy.Input{}, err
	}
	atlas, err := repositoryatlas.Canonical(*data.RepositoryAtlas)
	if err != nil {
		return atlasstudy.Input{}, fmt.Errorf("atlas study report: repository Atlas: %w", err)
	}

	input := atlasstudy.Input{
		Atlas: atlas, Language: language, Limits: atlasstudy.DefaultLimits(),
		Architecture: atlasstudy.ArchitectureInput{
			Version:  studyData.ArchitectureCanvas.Version,
			Source:   string(studyData.ArchitectureCanvas.ArchitectureSource),
			Title:    strings.TrimSpace(studyData.ArchitectureCanvas.Title),
			Subtitle: strings.TrimSpace(studyData.ArchitectureCanvas.Subtitle),
		},
	}
	remainderComponentID := studyData.ArchitectureCanvas.LocalRemainderComponentID
	var remainderSubsystemID componentmap.SubsystemID
	for _, component := range studyData.ArchitectureCanvas.Components {
		if component.ID == remainderComponentID && remainderComponentID != "" {
			remainderSubsystemID = component.SubsystemID
			break
		}
	}
	for _, subsystem := range studyData.ArchitectureCanvas.Subsystems {
		if subsystem.ID == remainderSubsystemID && remainderSubsystemID != "" {
			continue
		}
		input.Architecture.Subsystems = append(input.Architecture.Subsystems, atlasstudy.Subsystem{
			ID: string(subsystem.ID), Name: strings.TrimSpace(subsystem.Name),
			Description:  strings.TrimSpace(subsystem.Description),
			Authority:    repositoryatlas.AuthorityInferred,
			ComponentIDs: atlasStudyComponentIDStrings(subsystem.ComponentIDs),
		})
	}
	for _, component := range studyData.ArchitectureCanvas.Components {
		if component.ID == remainderComponentID && remainderComponentID != "" {
			continue
		}
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

	input.Surfaces = atlasStudySurfaces(studyData, atlas)
	shelf := atlasStudyReadingShelf(studyData, input.Surfaces)
	input.ReadingTargets = shelf.targets
	input.ReadingSupports = shelf.supports
	input.ProducerRelations = shelf.relations
	input.RouteSpans = shelf.spans
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

// atlasStudyLocalD177Data keeps Study independent from optional Architecture
// grouping. The complete exact RepositoryGraph and grounding facts rebuild the
// same deterministic D177 landscape for full, partial, and rejected model
// outcomes. Producer-owned anchors, Surfaces, and saved flows remain exact
// local evidence; only their model-derived component joins are ignored when
// they do not resolve in the rebuilt local landscape.
func atlasStudyLocalD177Data(data *ReportData) (*ReportData, error) {
	if data == nil || data.ArchitectureCanvas == nil {
		return nil, fmt.Errorf("atlas study report: architecture canvas is required")
	}
	localInput, err := BuildArchitectureCanvasInput(data)
	if err != nil {
		// Some historical/provider-free report fixtures contain only an already
		// local Canvas and no reconstructable graph or grounding candidates. The
		// same fallback applies regardless of model outcome; current partial
		// synthesis cannot arise without the exact graph required by D202.
		if errors.Is(err, errNoCanonicalArchitectureCandidates) {
			return data, nil
		}
		return nil, fmt.Errorf("atlas study report: rebuild local D177 input: %w", err)
	}
	localCanvas, err := ProjectArchitectureCanvas(localInput)
	if err != nil {
		return nil, fmt.Errorf("atlas study report: rebuild local D177 canvas: %w", err)
	}
	active := data.ArchitectureCanvas
	localCanvas.BehaviorAnchors = append(
		[]componentmap.BehaviorAnchor(nil), active.BehaviorAnchors...,
	)
	localCanvas.Surfaces = append([]ArchitectureSurface(nil), active.Surfaces...)
	localCanvas.Flows = append([]ArchitectureFlow(nil), active.Flows...)
	localCanvas.FlowEdges = append([]ArchitectureFlowEdge(nil), active.FlowEdges...)

	stable := *data
	stable.ArchitectureCanvas = &localCanvas
	return &stable, nil
}

// AtlasStudyInputHasMinimumCatalog reports whether the typed local shelf can
// advertise at least one exact backend-owned span. A focused span legitimately
// contains one locator; D210 removed the former global three-locator gate.
func AtlasStudyInputHasMinimumCatalog(input atlasstudy.Input) bool {
	return len(input.ReadingTargets) > 0 && len(input.ReadingSupports) > 0 && len(input.RouteSpans) > 0
}

// validateUsableAtlasStudyCanvas verifies the complete D177-visible base map
// without making model enrichment acceptance a Study dependency. A rejected
// enrichment may publish only the independently reconstructed local Canvas;
// a partial model Canvas paired with a rejected status is never usable.
func validateUsableAtlasStudyCanvas(data *ReportData) error {
	if data == nil || data.ArchitectureCanvas == nil {
		return fmt.Errorf("atlas study report: architecture canvas is required")
	}
	canvas := data.ArchitectureCanvas
	if canvas.Version != ArchitectureCanvasVersion || canvas.Fallback ||
		len(canvas.Subsystems) == 0 || len(canvas.Components) == 0 {
		return fmt.Errorf("atlas study report: canonical local Architecture Canvas is not usable")
	}
	components := make(map[componentmap.ComponentID]componentmap.SubsystemID, len(canvas.Components))
	for _, component := range canvas.Components {
		if component.ID == "" || component.SubsystemID == "" || strings.TrimSpace(component.Name) == "" {
			return fmt.Errorf("atlas study report: canonical local Architecture component is invalid")
		}
		if _, duplicate := components[component.ID]; duplicate {
			return fmt.Errorf("atlas study report: canonical local Architecture component is duplicated")
		}
		components[component.ID] = component.SubsystemID
	}
	membership := make(map[componentmap.ComponentID]int, len(components))
	seenSubsystems := make(map[componentmap.SubsystemID]struct{}, len(canvas.Subsystems))
	for _, subsystem := range canvas.Subsystems {
		if subsystem.ID == "" || strings.TrimSpace(subsystem.Name) == "" {
			return fmt.Errorf("atlas study report: canonical local Architecture subsystem is invalid")
		}
		if _, duplicate := seenSubsystems[subsystem.ID]; duplicate {
			return fmt.Errorf("atlas study report: canonical local Architecture subsystem is duplicated")
		}
		seenSubsystems[subsystem.ID] = struct{}{}
		seenMembers := make(map[componentmap.ComponentID]struct{}, len(subsystem.ComponentIDs))
		for _, componentID := range subsystem.ComponentIDs {
			if _, duplicate := seenMembers[componentID]; duplicate ||
				components[componentID] != subsystem.ID {
				return fmt.Errorf("atlas study report: canonical local Architecture membership is inconsistent")
			}
			seenMembers[componentID] = struct{}{}
			membership[componentID]++
		}
	}
	for componentID := range components {
		if membership[componentID] != 1 {
			return fmt.Errorf("atlas study report: canonical local Architecture membership is incomplete")
		}
	}
	if status := data.ArchitectureSynthesis; status != nil {
		if err := status.Validate(); err != nil {
			return fmt.Errorf("atlas study report: Architecture status: %w", err)
		}
		rejected := status.State == ArchitectureSynthesisFailed || status.ProposalRejected
		modelCanvas := canvas.ArchitectureSource == componentmap.SourceValidatedModel ||
			canvas.ArchitectureSource == componentmap.SourceNormalizedModel ||
			canvas.ArchitectureSource == componentmap.SourcePartialModel
		if rejected && modelCanvas {
			return fmt.Errorf("atlas study report: rejected enrichment cannot authorize a model Architecture Canvas")
		}
	}
	return nil
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
			canvas.ValidationOutcome != componentmap.ValidationAcceptedNormalized &&
			canvas.ValidationOutcome != componentmap.ValidationAcceptedPartial) ||
		(canvas.ArchitectureSource != componentmap.SourceValidatedModel &&
			canvas.ArchitectureSource != componentmap.SourceNormalizedModel &&
			canvas.ArchitectureSource != componentmap.SourcePartialModel) ||
		status.ArchitectureSource != string(canvas.ArchitectureSource) ||
		status.ArchitectureLevel != canvas.ArchitectureLevel {
		return fmt.Errorf("atlas study report: Architecture is not an accepted model result")
	}
	partial := canvas.ValidationOutcome == componentmap.ValidationAcceptedPartial
	if partial != (canvas.ArchitectureSource == componentmap.SourcePartialModel) ||
		partial != status.ProposalPartial {
		return fmt.Errorf("atlas study report: Architecture partial outcome is inconsistent")
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

type atlasStudyReadingShelfResult struct {
	targets   []atlasstudy.ReadingTarget
	supports  []atlasstudy.ReadingSupport
	relations []atlasstudy.RouteProducerRelation
	spans     []atlasstudy.RouteSpan
}

type atlasStudySupportProof struct {
	role          atlasstudy.SupportRole
	producerID    string
	packageBucket string
	authority     repositoryatlas.Authority
}

func atlasStudyReadingShelf(
	data *ReportData,
	surfaces []atlasstudy.Surface,
) atlasStudyReadingShelfResult {
	if data == nil || data.ArchitectureCanvas == nil {
		return atlasStudyReadingShelfResult{}
	}
	openable := make(map[string]struct{}, len(data.OpenablePaths))
	for _, sourcePath := range data.OpenablePaths {
		openable[sourcePath] = struct{}{}
	}
	advertisedSurfaces := make(map[string]struct{}, len(surfaces))
	for _, surface := range surfaces {
		advertisedSurfaces[surface.ID] = struct{}{}
	}
	packageDeclarationEvidence := exactRepositoryAtlasPackageEvidenceIDs(data)
	semanticTargetKeys := overviewSemanticSourceTargetKeys(data)
	sources := append([]SourceSnippet(nil), data.UserSources...)
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Path != sources[j].Path {
			return sources[i].Path < sources[j].Path
		}
		left, right := atlasStudySourceFocusLine(sources[i]), atlasStudySourceFocusLine(sources[j])
		if left != right {
			return left < right
		}
		if sources[i].EnclosingSymbol != sources[j].EnclosingSymbol {
			return sources[i].EnclosingSymbol < sources[j].EnclosingSymbol
		}
		return sources[i].PresentationSHA256 < sources[j].PresentationSHA256
	})
	resolvedSourcesByLocator := make(map[string][]SourceSnippet, len(sources))
	sourcesByStoredLocator := make(map[string][]SourceSnippet, len(sources))
	for _, source := range sources {
		if atlasStudyPackageDeclarationDrawerOnly(
			source, packageDeclarationEvidence, semanticTargetKeys,
		) {
			continue
		}
		line := atlasStudySourceFocusLine(source)
		if _, ok := openable[source.Path]; !ok || line <= 0 || source.Validate() != nil {
			continue
		}
		storedLocatorKey := strings.Join([]string{
			source.Path, fmt.Sprint(line), strings.TrimSpace(source.EnclosingSymbol),
		}, "\x00")
		sourcesByStoredLocator[storedLocatorKey] = append(
			sourcesByStoredLocator[storedLocatorKey], source,
		)
	}
	for _, candidates := range sourcesByStoredLocator {
		selected, found, conflict := resolveAtlasStudySourceCandidates(candidates)
		if !found || conflict {
			continue
		}
		line := atlasStudySourceFocusLine(selected)
		preSymbol, _, _, _ := atlasStudyReadingTargetContext(selected, line, false, nil)
		association := atlasStudyReadingAssociation(
			data, advertisedSurfaces, selected.Path, line, preSymbol,
		)
		if len(association.supports) == 0 || len(association.principalRefs) == 0 {
			continue
		}
		symbol, kind, _, _ := atlasStudyReadingTargetContext(
			selected, line, association.processEntry, association.surfaceLabels,
		)
		locatorKey := strings.Join([]string{
			string(kind), selected.Path, fmt.Sprint(line), symbol,
		}, "\x00")
		resolvedSourcesByLocator[locatorKey] = append(
			resolvedSourcesByLocator[locatorKey], selected,
		)
	}
	targetsByLocator := make(map[string]atlasstudy.ReadingTarget, len(resolvedSourcesByLocator))
	for locatorKey, candidates := range resolvedSourcesByLocator {
		selected, found, conflict := resolveAtlasStudySourceCandidates(candidates)
		if !found || conflict {
			continue
		}
		line := atlasStudySourceFocusLine(selected)
		preSymbol, _, _, _ := atlasStudyReadingTargetContext(selected, line, false, nil)
		association := atlasStudyReadingAssociation(
			data, advertisedSurfaces, selected.Path, line, preSymbol,
		)
		symbol, kind, label, fact := atlasStudyReadingTargetContext(
			selected, line, association.processEntry, association.surfaceLabels,
		)
		targetsByLocator[locatorKey] = atlasstudy.ReadingTarget{
			ID:    "reading-target-" + atlasStudyDigest(locatorKey),
			Owner: association.owner, RelatedComponentIDs: association.relatedComponentIDs,
			PrincipalRefs: association.principalRefs,
			Kind:          kind, Label: label, Fact: fact,
			Authority: repositoryatlas.AuthorityObserved,
			Location:  evidence.Location{Path: selected.Path, Line: line},
			Symbol:    symbol,
		}
	}
	// Entry handoffs are producer-owned Study eligibility, not merely one more
	// use of the saved-source shelf. Project their exact declarations even when
	// no pre-D210 UserSource happened to include the callee. Source hydration is
	// a separate local projection and never creates the support identity.
	physicalTargets := make(map[string]string, len(targetsByLocator))
	for _, target := range targetsByLocator {
		key := target.Location.Path + "\x00" + fmt.Sprint(target.Location.Line)
		if existing, ok := physicalTargets[key]; ok && existing != target.Symbol {
			physicalTargets[key] = ""
		} else if !ok {
			physicalTargets[key] = target.Symbol
		}
	}
	addHandoffTarget := func(member ArchitectureAnchorMember, processEntry bool) {
		if member.Location.Path == "" || member.Location.Line <= 0 || member.ID == "" {
			return
		}
		if _, ok := openable[member.Location.Path]; !ok {
			return
		}
		physicalKey := member.Location.Path + "\x00" + fmt.Sprint(member.Location.Line)
		if existing, exists := physicalTargets[physicalKey]; exists && existing == member.ID {
			return
		}
		association := atlasStudyReadingAssociation(
			data, advertisedSurfaces, member.Location.Path, member.Location.Line, member.ID,
		)
		if len(association.supports) == 0 || len(association.principalRefs) == 0 {
			return
		}
		kind := atlasStudyReadingTargetKind(member.ID, processEntry)
		label := strings.TrimSpace(member.Name)
		if label == "" {
			label = "Repository declaration"
		}
		fact := "Exact producer-owned repository declaration for an observed entry handoff."
		locatorKey := strings.Join([]string{
			string(kind), member.Location.Path, fmt.Sprint(member.Location.Line), member.ID,
		}, "\x00")
		targetsByLocator[locatorKey] = atlasstudy.ReadingTarget{
			ID: "reading-target-" + atlasStudyDigest(locatorKey), Owner: association.owner,
			RelatedComponentIDs: association.relatedComponentIDs,
			PrincipalRefs:       association.principalRefs,
			Kind:                kind, Label: label, Fact: fact, Authority: repositoryatlas.AuthorityObserved,
			Location: evidence.Location{Path: member.Location.Path, Line: member.Location.Line},
			Symbol:   member.ID,
		}
		if existing, exists := physicalTargets[physicalKey]; exists && existing != member.ID {
			physicalTargets[physicalKey] = ""
		} else {
			physicalTargets[physicalKey] = member.ID
		}
	}
	if data.ArchitectureGrounding != nil && data.ArchitectureGrounding.Version >= ArchitectureGroundingVersion {
		for _, handoff := range data.ArchitectureGrounding.EntryHandoffs {
			addHandoffTarget(handoff.ProcessEntrypoint, true)
			addHandoffTarget(handoff.Callee, false)
		}
	}
	result := atlasStudyReadingShelfResult{
		targets: make([]atlasstudy.ReadingTarget, 0, len(targetsByLocator)),
	}
	associationsByTarget := make(map[string]atlasStudyReadingAssociations, len(targetsByLocator))
	for _, target := range targetsByLocator {
		result.targets = append(result.targets, target)
		associationsByTarget[target.ID] = atlasStudyReadingAssociation(
			data, advertisedSurfaces, target.Location.Path, target.Location.Line, target.Symbol,
		)
	}
	physicalCounts := make(map[string]int, len(result.targets))
	for _, target := range result.targets {
		physicalCounts[target.Location.Path+"\x00"+fmt.Sprint(target.Location.Line)]++
	}
	for _, target := range result.targets {
		key := target.Location.Path + "\x00" + fmt.Sprint(target.Location.Line)
		if physicalCounts[key] < 2 {
			continue
		}
		association := associationsByTarget[target.ID]
		association.supports = slices.DeleteFunc(
			association.supports,
			func(proof atlasStudySupportProof) bool {
				return !strings.HasPrefix(proof.producerID, "entry-handoff-entry:") &&
					!strings.HasPrefix(proof.producerID, "entry-handoff-callee:")
			},
		)
		associationsByTarget[target.ID] = association
	}
	result.targets = slices.DeleteFunc(result.targets, func(target atlasstudy.ReadingTarget) bool {
		return len(associationsByTarget[target.ID].supports) == 0
	})
	sort.Slice(result.targets, func(i, j int) bool { return result.targets[i].ID < result.targets[j].ID })
	result.supports = atlasStudyReadingSupports(result.targets, associationsByTarget)
	result.relations = atlasStudyProducerRelations(data, result.targets, result.supports)
	result.spans = atlasStudyRouteSpans(result.targets, result.supports, result.relations)
	return result
}

// atlasStudyPackageDeclarationDrawerOnly keeps the package catalog clickable
// without multiplying the semantic reading catalog. It suppresses a snippet
// only when every attached identity resolves to the exact adapter-owned
// package-declaration Evidence. Unknown or mixed identities never erase a
// potentially Study-eligible source.
func atlasStudyPackageDeclarationDrawerOnly(
	source SourceSnippet,
	exactPackageEvidence map[string]struct{},
	semanticTargetKeys map[string]struct{},
) bool {
	if len(source.RelatedEvidenceIDs) == 0 {
		return false
	}
	line := atlasStudySourceFocusLine(source)
	if _, semantic := semanticTargetKeys[overviewSourceTargetKey(source.Path, line)]; semantic {
		return false
	}
	for _, evidenceID := range source.RelatedEvidenceIDs {
		if _, exact := exactPackageEvidence[evidenceID]; !exact {
			return false
		}
	}
	return true
}

type atlasStudyReadingAssociations struct {
	owner               atlasstudy.CanonicalRef
	relatedComponentIDs []string
	principalRefs       []atlasstudy.CanonicalRef
	surfaceLabels       []string
	processEntry        bool
	supports            []atlasStudySupportProof
}

func atlasStudyReadingAssociation(
	data *ReportData,
	advertisedSurfaces map[string]struct{},
	sourcePath string,
	line int,
	symbol string,
) atlasStudyReadingAssociations {
	knownComponents := make(map[string]struct{}, len(data.ArchitectureCanvas.Components))
	memberFacts := make(map[componentmap.MemberID][]componentmap.LocalFact)
	relatedComponents := make(map[string]struct{})
	principalRefs := make(map[atlasstudy.CanonicalRef]struct{})
	exactOwners := make(map[string]struct{})
	surfaceLabels := make(map[string]struct{})
	for _, component := range data.ArchitectureCanvas.Components {
		componentID := string(component.ID)
		for _, member := range component.Members {
			memberFacts[member.ID] = append(memberFacts[member.ID], member.Facts...)
		}
		if component.ID == data.ArchitectureCanvas.LocalRemainderComponentID &&
			data.ArchitectureCanvas.LocalRemainderComponentID != "" {
			continue
		}
		knownComponents[componentID] = struct{}{}
		if atlasStudyComponentOwnsPath(data, component, sourcePath) {
			relatedComponents[componentID] = struct{}{}
			principalRefs[atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: componentID}] = struct{}{}
		}
	}
	result := atlasStudyReadingAssociations{}
	for _, surface := range data.ArchitectureCanvas.Surfaces {
		for _, location := range surface.Evidence {
			if line > 0 && location.Path == sourcePath && location.Line == line {
				if surface.Kind == "process_entry" {
					result.processEntry = true
				}
				if _, advertised := advertisedSurfaces[surface.ID]; !advertised {
					break
				}
				authority := atlasStudyAdvertisedSurfaceAuthority(data, surface.ID)
				role := atlasstudy.SupportSurface
				if surface.Kind == "process_entry" && authority != repositoryatlas.AuthorityPartial {
					role = atlasstudy.SupportProcessEntry
				} else if authority == repositoryatlas.AuthorityPartial {
					role = atlasstudy.SupportSurfaceCandidate
				}
				if packageBucket := atlasStudyExactPackageBucket(data, sourcePath, symbol); packageBucket != "" {
					result.supports = append(result.supports, atlasStudySupportProof{
						role: role, producerID: "surface:" + surface.ID,
						packageBucket: packageBucket, authority: authority,
					})
				}
				principalRefs[atlasstudy.CanonicalRef{Kind: atlasstudy.RefSurface, ID: surface.ID}] = struct{}{}
				if label := strings.TrimSpace(surface.Name); label != "" {
					surfaceLabels[label] = struct{}{}
				}
				ownerID := string(surface.OwningComponentID)
				if _, known := knownComponents[ownerID]; ownerID != "" && known {
					exactOwners[ownerID] = struct{}{}
					relatedComponents[ownerID] = struct{}{}
					principalRefs[atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: ownerID}] = struct{}{}
				}
				for _, componentID := range surface.ParticipatingComponentIDs {
					id := string(componentID)
					if _, known := knownComponents[id]; !known {
						continue
					}
					relatedComponents[id] = struct{}{}
					principalRefs[atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: id}] = struct{}{}
				}
				break
			}
		}
	}
	for _, flow := range data.ArchitectureCanvas.Flows {
		acceptedStudyFlow := atlasStudyAcceptedSavedFlow(flow)
		for _, step := range flow.Steps {
			if line <= 0 || step.Location == nil || step.Location.Path != sourcePath || step.Location.Line != line {
				continue
			}
			ownerID := string(step.ComponentID)
			// Any exact local flow component remains presentation context. Only an
			// accepted static saved flow is allowed to create Study eligibility.
			// Conceptual component resolution is optional presentation context and
			// can never erase an exact producer-owned saved-flow support.
			if packageBucket := atlasStudyExactPackageBucket(data, sourcePath, symbol); acceptedStudyFlow && packageBucket != "" {
				result.supports = append(result.supports, atlasStudySupportProof{
					role:          atlasstudy.SupportSavedFlow,
					producerID:    "saved-flow:" + string(flow.ID) + ":" + step.ID,
					packageBucket: packageBucket, authority: repositoryatlas.AuthorityObserved,
				})
			}
			if _, known := knownComponents[ownerID]; ownerID != "" && known {
				exactOwners[ownerID] = struct{}{}
				relatedComponents[ownerID] = struct{}{}
				principalRefs[atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: ownerID}] = struct{}{}
			}
		}
	}
	for _, anchor := range data.ArchitectureCanvas.BehaviorAnchors {
		if line <= 0 || (anchor.ProofMode != componentmap.AnchorProofProcessEntry &&
			anchor.ProofMode != componentmap.AnchorProofCallTarget) {
			continue
		}
		supported := anchor.Location.Path == sourcePath && anchor.Location.Line == line
		if !supported {
			for _, memberID := range anchor.MemberIDs {
				for _, fact := range memberFacts[memberID] {
					if fact.Location == nil || fact.Location.Line <= 0 ||
						(fact.Kind != componentmap.FactDeclaration && fact.Kind != componentmap.FactRepositoryPath) {
						continue
					}
					if fact.Location.Path == sourcePath && fact.Location.Line == line {
						supported = true
						break
					}
				}
				if supported {
					break
				}
			}
		}
		if !supported {
			continue
		}
		switch anchor.ProofMode {
		case componentmap.AnchorProofProcessEntry:
			result.processEntry = true
			if packageBucket := atlasStudyExactPackageBucket(data, sourcePath, symbol); packageBucket != "" {
				result.supports = append(result.supports, atlasStudySupportProof{
					role: atlasstudy.SupportProcessEntry, producerID: "behavior-anchor:" + anchor.ID,
					packageBucket: packageBucket, authority: repositoryatlas.AuthorityObserved,
				})
			}
		case componentmap.AnchorProofCallTarget:
			if packageBucket := atlasStudyExactPackageBucket(data, sourcePath, symbol); packageBucket != "" {
				result.supports = append(result.supports, atlasStudySupportProof{
					role: atlasstudy.SupportObservedCallBoundary, producerID: "behavior-anchor:" + anchor.ID,
					packageBucket: packageBucket, authority: repositoryatlas.AuthorityObserved,
				})
			}
		}
	}
	if data.ArchitectureGrounding != nil && data.ArchitectureGrounding.Version >= ArchitectureGroundingVersion {
		for _, handoff := range data.ArchitectureGrounding.EntryHandoffs {
			switch {
			case atlasStudyHandoffMemberMatches(handoff.ProcessEntrypoint, sourcePath, line, symbol):
				result.processEntry = true
				result.supports = append(result.supports, atlasStudySupportProof{
					role:          atlasstudy.SupportProcessEntry,
					producerID:    "entry-handoff-entry:" + handoff.ID,
					packageBucket: atlasStudyPackageBucketID(handoff.ProcessEntrypoint.Package),
					authority:     repositoryatlas.AuthorityObserved,
				})
			case atlasStudyHandoffMemberMatches(handoff.Callee, sourcePath, line, symbol):
				result.supports = append(result.supports, atlasStudySupportProof{
					role:          atlasstudy.SupportEntryHandoff,
					producerID:    "entry-handoff-callee:" + handoff.ID,
					packageBucket: atlasStudyPackageBucketID(handoff.TargetPackage),
					authority:     repositoryatlas.AuthorityObserved,
				})
			}
		}
	}
	if len(exactOwners) == 1 {
		for ownerID := range exactOwners {
			result.owner = atlasstudy.CanonicalRef{Kind: atlasstudy.RefComponent, ID: ownerID}
		}
	}
	for componentID := range relatedComponents {
		result.relatedComponentIDs = append(result.relatedComponentIDs, componentID)
	}
	sort.Strings(result.relatedComponentIDs)
	for ref := range principalRefs {
		result.principalRefs = append(result.principalRefs, ref)
	}
	sort.Slice(result.principalRefs, func(i, j int) bool {
		if result.principalRefs[i].Kind != result.principalRefs[j].Kind {
			return result.principalRefs[i].Kind < result.principalRefs[j].Kind
		}
		return result.principalRefs[i].ID < result.principalRefs[j].ID
	})
	for label := range surfaceLabels {
		result.surfaceLabels = append(result.surfaceLabels, label)
	}
	sort.Strings(result.surfaceLabels)
	sort.Slice(result.supports, func(i, j int) bool {
		left, right := result.supports[i], result.supports[j]
		if left.role != right.role {
			return left.role < right.role
		}
		if left.packageBucket != right.packageBucket {
			return left.packageBucket < right.packageBucket
		}
		return left.producerID < right.producerID
	})
	return result
}

func atlasStudyAdvertisedSurfaceAuthority(data *ReportData, surfaceID string) repositoryatlas.Authority {
	if data == nil || data.ArchitectureCanvas == nil {
		return repositoryatlas.AuthorityUnknown
	}
	for _, surface := range data.ArchitectureCanvas.Surfaces {
		if surface.ID != surfaceID {
			continue
		}
		switch surface.Resolution {
		case "partial", "dynamic", "provisional":
			return repositoryatlas.AuthorityPartial
		case "exact":
			return repositoryatlas.AuthorityResolved
		default:
			return repositoryatlas.AuthorityUnknown
		}
	}
	return repositoryatlas.AuthorityUnknown
}

func atlasStudyHandoffMemberMatches(
	member ArchitectureAnchorMember,
	sourcePath string,
	line int,
	symbol string,
) bool {
	return line > 0 && symbol != "" && member.ID == symbol &&
		member.Location.Path == sourcePath && member.Location.Line == line
}

// atlasStudyExactPackageBucket restores the exact producer package identity
// without treating a repository path prefix or conceptual component as a
// package. RepositoryGraph is authoritative when present; a qualified saved
// Go symbol is an exact local producer identity for fixtures and historical
// reports whose bounded graph projection is absent.
func atlasStudyExactPackageBucket(data *ReportData, sourcePath, symbol string) string {
	var packages []string
	if data != nil && data.RepositoryGraph != nil {
		for _, pkg := range data.RepositoryGraph.Packages {
			if pkg.CanonicalPath == "" || !slices.Contains(pkg.Files, sourcePath) {
				continue
			}
			packages = append(packages, pkg.CanonicalPath)
		}
		sort.Strings(packages)
		packages = slices.Compact(packages)
		if len(packages) == 1 {
			return atlasStudyPackageBucketID(packages[0])
		}
		if len(packages) > 1 {
			return ""
		}
	}
	symbol = strings.TrimSpace(symbol)
	if index := strings.Index(symbol, ".("); index > 0 {
		return atlasStudyPackageBucketID(symbol[:index])
	}
	if index := strings.LastIndex(symbol, "."); index > 0 {
		return atlasStudyPackageBucketID(symbol[:index])
	}
	return ""
}

func atlasStudyPackageBucketID(canonicalPackage string) string {
	canonicalPackage = strings.TrimSpace(canonicalPackage)
	if canonicalPackage == "" {
		return ""
	}
	return "package-bucket-" + atlasStudyDigest(canonicalPackage)
}

func atlasStudyReadingSupports(
	targets []atlasstudy.ReadingTarget,
	associations map[string]atlasStudyReadingAssociations,
) []atlasstudy.ReadingSupport {
	type supportKey struct {
		targetID      string
		role          atlasstudy.SupportRole
		packageBucket string
	}
	type supportProofSet struct {
		producers   []string
		authorities []repositoryatlas.Authority
	}
	proofs := make(map[supportKey]supportProofSet)
	for _, target := range targets {
		for _, proof := range associations[target.ID].supports {
			if proof.producerID == "" || proof.packageBucket == "" {
				continue
			}
			key := supportKey{
				targetID: target.ID, role: proof.role,
				packageBucket: proof.packageBucket,
			}
			set := proofs[key]
			set.producers = append(set.producers, proof.producerID)
			set.authorities = append(set.authorities, proof.authority)
			proofs[key] = set
		}
	}
	result := make([]atlasstudy.ReadingSupport, 0, len(proofs))
	for key, set := range proofs {
		proofIDs := set.producers
		sort.Strings(proofIDs)
		proofIDs = slices.Compact(proofIDs)
		authority := repositoryatlas.AuthorityResolved
		for _, candidate := range set.authorities {
			if candidate == repositoryatlas.AuthorityPartial {
				authority = repositoryatlas.AuthorityPartial
				break
			}
			if candidate == repositoryatlas.AuthorityObserved {
				authority = repositoryatlas.AuthorityObserved
			}
		}
		identity := strings.Join([]string{
			key.targetID, string(key.role), key.packageBucket,
			string(authority), strings.Join(proofIDs, "\x01"),
		}, "\x00")
		result = append(result, atlasstudy.ReadingSupport{
			ID: "route-support-" + atlasStudyDigest(identity), TargetID: key.targetID,
			PackageBucket: key.packageBucket, Role: key.role, Authority: authority,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func atlasStudyProducerRelations(
	data *ReportData,
	targets []atlasstudy.ReadingTarget,
	supports []atlasstudy.ReadingSupport,
) []atlasstudy.RouteProducerRelation {
	supportByTargetRolePackage := make(map[string]string, len(supports))
	for _, support := range supports {
		key := strings.Join([]string{support.TargetID, string(support.Role), support.PackageBucket}, "\x00")
		if _, duplicate := supportByTargetRolePackage[key]; duplicate {
			// Different producer authorities for the same role/package cannot be
			// chosen as one relation endpoint.
			supportByTargetRolePackage[key] = ""
			continue
		}
		supportByTargetRolePackage[key] = support.ID
	}
	resolveSupport := func(targetID string, role atlasstudy.SupportRole, packageBucket string) string {
		return supportByTargetRolePackage[strings.Join([]string{targetID, string(role), packageBucket}, "\x00")]
	}
	var result []atlasstudy.RouteProducerRelation
	if data != nil && data.ArchitectureGrounding != nil &&
		data.ArchitectureGrounding.Version >= ArchitectureGroundingVersion {
		for _, handoff := range data.ArchitectureGrounding.EntryHandoffs {
			fromTarget := atlasStudyExactHandoffTarget(targets, handoff.ProcessEntrypoint, true)
			toTarget := atlasStudyExactHandoffTarget(targets, handoff.Callee, false)
			fromSupport := resolveSupport(fromTarget, atlasstudy.SupportProcessEntry, atlasStudyPackageBucketID(handoff.ProcessEntrypoint.Package))
			toSupport := resolveSupport(toTarget, atlasstudy.SupportEntryHandoff, atlasStudyPackageBucketID(handoff.TargetPackage))
			if fromTarget == "" || toTarget == "" || fromTarget == toTarget || fromSupport == "" || toSupport == "" {
				continue
			}
			identity := strings.Join([]string{
				string(atlasstudy.RouteRelationEntryHandoff), handoff.ID,
				fromSupport, toSupport, fromTarget, toTarget,
			}, "\x00")
			result = append(result, atlasstudy.RouteProducerRelation{
				ID:   "route-relation-" + atlasStudyDigest(identity),
				Kind: atlasstudy.RouteRelationEntryHandoff, ProducerID: handoff.ID,
				FromSupportID: fromSupport, ToSupportID: toSupport,
				FromTargetID: fromTarget, ToTargetID: toTarget,
			})
		}
	}
	result = append(result, atlasStudySavedFlowRelations(data, targets, supports)...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func atlasStudyExactHandoffTarget(
	targets []atlasstudy.ReadingTarget,
	member ArchitectureAnchorMember,
	processEntry bool,
) string {
	wantKind := atlasStudyReadingTargetKind(member.ID, processEntry)
	matched := ""
	for _, target := range targets {
		if target.Kind != wantKind || target.Symbol != member.ID ||
			target.Location.Path != member.Location.Path || target.Location.Line != member.Location.Line {
			continue
		}
		if matched != "" {
			return ""
		}
		matched = target.ID
	}
	return matched
}

func atlasStudySavedFlowRelations(
	data *ReportData,
	targets []atlasstudy.ReadingTarget,
	supports []atlasstudy.ReadingSupport,
) []atlasstudy.RouteProducerRelation {
	if data == nil || data.ArchitectureCanvas == nil {
		return nil
	}
	supportByTarget := make(map[string]string)
	for _, support := range supports {
		if support.Role != atlasstudy.SupportSavedFlow {
			continue
		}
		if _, duplicate := supportByTarget[support.TargetID]; duplicate {
			supportByTarget[support.TargetID] = ""
		} else {
			supportByTarget[support.TargetID] = support.ID
		}
	}
	edgeCounts := make(map[string]int)
	for _, edge := range data.ArchitectureCanvas.FlowEdges {
		if edge.ID != "" {
			edgeCounts[edge.ID]++
		}
	}
	var result []atlasstudy.RouteProducerRelation
	for _, flow := range data.ArchitectureCanvas.Flows {
		if !atlasStudyAcceptedSavedFlow(flow) {
			continue
		}
		stepTarget := make(map[string]string, len(flow.Steps))
		duplicateSteps := make(map[string]struct{})
		for _, step := range flow.Steps {
			if step.ID == "" {
				continue
			}
			if _, duplicate := stepTarget[step.ID]; duplicate {
				duplicateSteps[step.ID] = struct{}{}
				continue
			}
			stepTarget[step.ID] = atlasStudyExactFlowStepTarget(targets, step)
		}
		transitionCounts := make(map[string]int, len(flow.TransitionIDs))
		for _, transitionID := range flow.TransitionIDs {
			if transitionID != "" {
				transitionCounts[transitionID]++
			}
		}
		for _, edge := range data.ArchitectureCanvas.FlowEdges {
			_, fromFound := stepTarget[edge.From]
			_, toFound := stepTarget[edge.To]
			_, fromDuplicate := duplicateSteps[edge.From]
			_, toDuplicate := duplicateSteps[edge.To]
			if edge.FlowID != flow.ID || edge.ID == "" || edgeCounts[edge.ID] != 1 ||
				transitionCounts[edge.ID] != 1 || edge.Resolution != evidence.ResolutionStatic ||
				edge.Certainty != evidence.CertaintyStatic || strings.TrimSpace(edge.Provider) == "" ||
				edge.Evidence.Path == "" || edge.Evidence.Line <= 0 ||
				!fromFound || !toFound || fromDuplicate || toDuplicate {
				continue
			}
			fromTarget, toTarget := stepTarget[edge.From], stepTarget[edge.To]
			fromSupport, toSupport := supportByTarget[fromTarget], supportByTarget[toTarget]
			if fromTarget == "" || toTarget == "" || fromTarget == toTarget || fromSupport == "" || toSupport == "" {
				continue
			}
			identity := strings.Join([]string{
				string(atlasstudy.RouteRelationSavedFlowEdge), edge.ID,
				fromSupport, toSupport, fromTarget, toTarget,
			}, "\x00")
			result = append(result, atlasstudy.RouteProducerRelation{
				ID:   "route-relation-" + atlasStudyDigest(identity),
				Kind: atlasstudy.RouteRelationSavedFlowEdge, ProducerID: edge.ID,
				FromSupportID: fromSupport, ToSupportID: toSupport,
				FromTargetID: fromTarget, ToTargetID: toTarget,
				SavedFlowID: string(flow.ID), FromStepID: edge.From, ToStepID: edge.To,
				// One saved-flow relation is exactly one accepted directed producer
				// edge. These ordinals describe that two-step edge locally; the
				// presentation order of flow.Steps is deliberately irrelevant.
				FromStepOrdinal: 0, ToStepOrdinal: 1,
			})
		}
	}
	return result
}

func atlasStudyAcceptedSavedFlow(flow ArchitectureFlow) bool {
	return flow.ID != "" && flow.EvidenceBasis == "static" &&
		(flow.Status == "complete" || flow.Status == "partial")
}

func atlasStudyExactFlowStepTarget(
	targets []atlasstudy.ReadingTarget,
	step ArchitectureFlowStep,
) string {
	if step.Location == nil || step.Location.Path == "" || step.Location.Line <= 0 {
		return ""
	}
	wantSymbol := strings.TrimSpace(step.QualifiedName)
	matched := ""
	for _, target := range targets {
		if target.Location.Path != step.Location.Path || target.Location.Line != step.Location.Line ||
			(wantSymbol != "" && target.Symbol != wantSymbol) {
			continue
		}
		if matched != "" {
			return ""
		}
		matched = target.ID
	}
	return matched
}

func atlasStudyRouteSpans(
	targets []atlasstudy.ReadingTarget,
	supports []atlasstudy.ReadingSupport,
	relations []atlasstudy.RouteProducerRelation,
) []atlasstudy.RouteSpan {
	targetByID := make(map[string]atlasstudy.ReadingTarget, len(targets))
	for _, target := range targets {
		targetByID[target.ID] = target
	}
	var result []atlasstudy.RouteSpan
	for _, support := range supports {
		target, ok := targetByID[support.TargetID]
		if !ok {
			continue
		}
		en, ru, job, stage := atlasStudyFocusedSpanPresentation(support.Role, target)
		result = append(result, atlasstudy.RouteSpan{
			ID:              "route-span-" + atlasStudyDigest("focused\x00"+support.ID),
			Kind:            atlasstudy.RouteSpanFocused,
			QuestionEnglish: en, QuestionRussian: ru,
			TargetJob: job, LearningStage: stage,
			RequiredSupportIDs: []string{support.ID},
			AllowedTargetIDs:   []string{target.ID},
		})
	}
	for _, relation := range relations {
		required := []string{relation.FromSupportID, relation.ToSupportID}
		allowed := []string{relation.FromTargetID, relation.ToTargetID}
		sort.Strings(required)
		sort.Strings(allowed)
		en, ru := "How does this exact saved flow move between these two code locations?", "Как сохранённый точный поток связывает эти два места в коде?"
		job, stage := atlasstudy.JobFirstContact, atlasstudy.StageCentralOperation
		if relation.Kind == atlasstudy.RouteRelationEntryHandoff {
			en = "Which direct static source call connects the process entry to this repository-local callee?"
			ru = "Какой прямой статический вызов в исходном коде связывает точку входа с этим локальным вызовом репозитория?"
			stage = atlasstudy.StageOrientation
		}
		if from, ok := targetByID[relation.FromTargetID]; ok {
			if to, ok := targetByID[relation.ToTargetID]; ok {
				if fromRef, fromSymbol := atlasStudyReadableTargetReference(from); fromRef != "" {
					if toRef, toSymbol := atlasStudyReadableTargetReference(to); toRef != "" {
						fromText := atlasStudyQuestionReference(fromRef, fromSymbol)
						toText := atlasStudyQuestionReference(toRef, toSymbol)
						if relation.Kind == atlasstudy.RouteRelationEntryHandoff {
							en = "Which direct static source call connects " + fromText + " to " + toText + "?"
							ru = "Какой прямой статический вызов в исходном коде связывает " + fromText + " и " + toText + "?"
						} else {
							en = "How does this exact saved flow move between " + fromText + " and " + toText + "?"
							ru = "Как точный сохранённый поток связывает " + fromText + " и " + toText + "?"
						}
					}
				}
			}
		}
		result = append(result, atlasstudy.RouteSpan{
			ID:              "route-span-" + atlasStudyDigest("system_path\x00"+relation.ID),
			Kind:            atlasstudy.RouteSpanSystemPath,
			QuestionEnglish: en, QuestionRussian: ru,
			TargetJob: job, LearningStage: stage,
			RequiredSupportIDs: required, AllowedTargetIDs: allowed,
			Joins: []atlasstudy.RouteSpanJoin{{RelationID: relation.ID}},
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func atlasStudyFocusedSpanPresentation(
	role atlasstudy.SupportRole,
	target atlasstudy.ReadingTarget,
) (string, string, atlasstudy.TargetJob, atlasstudy.LearningStage) {
	reference, symbol := atlasStudyReadableTargetReference(target)
	if reference == "" {
		switch role {
		case atlasstudy.SupportProcessEntry:
			return "Where does this application process start?", "Где запускается процесс приложения?", atlasstudy.JobFirstContact, atlasstudy.StageOrientation
		case atlasstudy.SupportEntryHandoff:
			return "What repository code is called directly from the process entry?", "Какой код репозитория точка входа вызывает напрямую?", atlasstudy.JobFirstContact, atlasstudy.StageOrientation
		case atlasstudy.SupportSurface:
			return "Where is this exact application surface implemented?", "Где реализована эта точная поверхность приложения?", atlasstudy.JobIntegrate, atlasstudy.StageIntegration
		case atlasstudy.SupportSurfaceCandidate:
			return "What source marks this partially resolved application surface?", "Какой исходный код отмечает эту частично разрешённую поверхность приложения?", atlasstudy.JobIntegrate, atlasstudy.StageIntegration
		case atlasstudy.SupportSavedFlow:
			return "What happens at this exact saved flow step?", "Что происходит на этом точном шаге сохранённого потока?", atlasstudy.JobMaintain, atlasstudy.StageCentralOperation
		default:
			return "What does this observed static call boundary connect?", "Что связывает эта наблюдаемая статическая граница вызова?", atlasstudy.JobMaintain, atlasstudy.StageCentralOperation
		}
	}
	ref := atlasStudyQuestionReference(reference, symbol)
	switch role {
	case atlasstudy.SupportProcessEntry:
		if symbol {
			return "Where does the " + ref + " function start this application's process?",
				"Где функция " + ref + " запускает процесс этого приложения?",
				atlasstudy.JobFirstContact, atlasstudy.StageOrientation
		}
		return "Where does this application's process start at " + ref + "?",
			"Где начинается процесс этого приложения в " + ref + "?",
			atlasstudy.JobFirstContact, atlasstudy.StageOrientation
	case atlasstudy.SupportEntryHandoff:
		if symbol {
			return "What repository code calls the " + ref + " function directly from the process entry?",
				"Какой код репозитория напрямую вызывает функцию " + ref + " из точки входа?",
				atlasstudy.JobFirstContact, atlasstudy.StageOrientation
		}
		return "What repository code calls " + ref + " directly from the process entry?",
			"Какой код репозитория напрямую вызывает " + ref + " из точки входа?",
			atlasstudy.JobFirstContact, atlasstudy.StageOrientation
	case atlasstudy.SupportSurface:
		return "Where is the " + ref + " surface implemented?",
			"Где реализована поверхность " + ref + "?",
			atlasstudy.JobIntegrate, atlasstudy.StageIntegration
	case atlasstudy.SupportSurfaceCandidate:
		return "What source marks the partially resolved " + ref + " surface?",
			"Какой исходный код отмечает частично разрешённую поверхность " + ref + "?",
			atlasstudy.JobIntegrate, atlasstudy.StageIntegration
	case atlasstudy.SupportSavedFlow:
		return "What happens at the exact saved flow step " + ref + "?",
			"Что происходит на точном шаге сохранённого потока " + ref + "?",
			atlasstudy.JobMaintain, atlasstudy.StageCentralOperation
	default:
		return "What does the observed " + ref + " call boundary connect?",
			"Что связывает наблюдаемая граница вызова " + ref + "?",
			atlasstudy.JobMaintain, atlasstudy.StageCentralOperation
	}
}

// atlasStudyReadableTargetReference returns the exact bounded natural-language
// reference a backend-owned question may use for one reading target, plus
// whether that reference is the exact source symbol (rather than a natural
// label). A bare identifier symbol or the package.Symbol segment after the
// last '/' of a fully-qualified symbol qualifies; canonical IDs, repository
// paths, package buckets and generic fallback labels are private and never
// injected into a question. An empty reference means the target has no
// readable value and callers must keep the generic wording.
func atlasStudyReadableTargetReference(target atlasstudy.ReadingTarget) (string, bool) {
	symbol := strings.TrimSpace(target.Symbol)
	if symbol != "" && !strings.ContainsAny(symbol, "./()") && utf8.ValidString(symbol) &&
		utf8.RuneCountInString(symbol) <= 128 {
		return symbol, true
	}
	// Fully-qualified symbols (containing '/', '.', '(' or ')') carry the
	// repository import path. The segment after the last '/' is the exact
	// package.Symbol reading target and a strict substring of the target's own
	// advertised symbol, so it remains a bounded backend-owned question
	// reference that item-local question validation accepts.
	if symbol != "" && strings.ContainsAny(symbol, "./()") {
		if derived := atlasStudyDerivedQualifiedSymbol(symbol); derived != "" {
			return derived, true
		}
	}
	label := strings.TrimSpace(target.Label)
	if label == "" || strings.ContainsAny(label, "./()") || !utf8.ValidString(label) ||
		utf8.RuneCountInString(label) > 128 {
		return "", false
	}
	switch label {
	case "Application entrypoint", "Repository method", "Repository function",
		"Repository source", "Qualified Go call site", "Repository declaration":
		return "", false
	}
	return label, false
}

// atlasStudyDerivedQualifiedSymbol reduces a fully-qualified symbol to a
// bounded question-safe reference. With a '/' present the segment after the
// last '/' (the exact package.Symbol) is used; otherwise the segment after
// the last '.' (the bare identifier) is used, because a symbol without a
// module path such as "http.ListenAndServe" is itself a private identity and
// must never be injected into a question. The derived value must remain
// bounded exact UTF-8 text with no control characters or spaces; otherwise it
// is not a readable question reference and callers keep the generic wording.
func atlasStudyDerivedQualifiedSymbol(symbol string) string {
	derived := symbol
	if lastSlash := strings.LastIndex(symbol, "/"); lastSlash >= 0 {
		derived = symbol[lastSlash+1:]
	} else if lastDot := strings.LastIndex(symbol, "."); lastDot >= 0 {
		derived = symbol[lastDot+1:]
	}
	derived = strings.TrimSpace(derived)
	if derived == "" || !utf8.ValidString(derived) || utf8.RuneCountInString(derived) > 128 {
		return ""
	}
	for _, value := range derived {
		if unicode.IsControl(value) || value == ' ' {
			return ""
		}
	}
	return derived
}

// atlasStudyQuestionReference renders a readable target reference inside a
// question: bare symbols become backtick-quoted code refs, natural labels stay
// plain prose.
func atlasStudyQuestionReference(reference string, symbol bool) string {
	if symbol {
		return "`" + reference + "`"
	}
	return reference
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

func atlasStudyReadingTargetKind(symbol string, processEntry bool) atlasstudy.ReadingTargetKind {
	if processEntry {
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

func atlasStudyReadingTargetContext(
	source SourceSnippet,
	line int,
	processEntry bool,
	surfaceLabels []string,
) (string, atlasstudy.ReadingTargetKind, string, string) {
	symbol := strings.TrimSpace(source.EnclosingSymbol)
	sourceLine := atlasStudyExactSourceLine(source, line)
	declaration := false
	method := false
	call := false
	if symbol == "" && sourceLanguage(source.Path) == "go" && sourceLine != "" {
		trimmedLine := strings.TrimSpace(sourceLine)
		if strings.HasPrefix(trimmedLine, "func ") {
			value, _, _, ok := boundedSourceDeclaration(source.Path, sourceLine)
			if ok {
				symbol = value
				declaration = true
				method = strings.HasPrefix(trimmedLine, "func (")
			}
		}
		if symbol == "" {
			if value, ok := atlasStudyQualifiedGoCall(sourceLine); ok {
				symbol = value
				call = true
			}
		}
	}
	kind := atlasStudyReadingTargetKind(symbol, processEntry)
	if method && !processEntry {
		kind = atlasstudy.ReadingTargetMethod
	}
	if len(surfaceLabels) == 1 {
		return symbol, kind, surfaceLabels[0], "Exact saved source for this advertised application surface."
	}
	if declaration {
		if method {
			return symbol, kind, "Method " + symbol, "Exact saved Go method declaration."
		}
		return symbol, kind, "Function " + symbol, "Exact saved Go function declaration."
	}
	if call {
		if !processEntry {
			kind = atlasstudy.ReadingTargetFile
		}
		return symbol, kind, "Qualified Go call site", "Exact saved source at a qualified Go call."
	}
	label, fact := atlasStudyReadingTargetText(kind, surfaceLabels)
	return symbol, kind, label, fact
}

func atlasStudyExactSourceLine(source SourceSnippet, line int) string {
	for _, candidate := range source.Lines {
		if candidate.Line == line {
			return candidate.Text
		}
	}
	return ""
}

func atlasStudyQualifiedGoCall(value string) (string, bool) {
	parsed, err := parser.ParseFile(
		token.NewFileSet(), "reading-target.go",
		"package readingtarget\nfunc inspect() {\n"+value+"\n}\n", 0,
	)
	if err != nil || len(parsed.Decls) != 1 {
		return "", false
	}
	declaration, ok := parsed.Decls[0].(*ast.FuncDecl)
	if !ok || declaration.Body == nil || len(declaration.Body.List) != 1 {
		return "", false
	}
	var call *ast.CallExpr
	switch statement := declaration.Body.List[0].(type) {
	case *ast.AssignStmt:
		if len(statement.Rhs) == 1 {
			call, _ = statement.Rhs[0].(*ast.CallExpr)
		}
	case *ast.ExprStmt:
		call, _ = statement.X.(*ast.CallExpr)
	case *ast.ReturnStmt:
		if len(statement.Results) == 1 {
			call, _ = statement.Results[0].(*ast.CallExpr)
		}
	case *ast.GoStmt:
		call = statement.Call
	case *ast.DeferStmt:
		call = statement.Call
	}
	if call == nil {
		return "", false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	receiver, ok := selector.X.(*ast.Ident)
	if !ok || receiver.Name == "" || selector.Sel == nil || selector.Sel.Name == "" {
		return "", false
	}
	return receiver.Name + "." + selector.Sel.Name, true
}

func atlasStudyReadingTargetText(
	kind atlasstudy.ReadingTargetKind,
	surfaceLabels []string,
) (string, string) {
	if len(surfaceLabels) == 1 {
		return surfaceLabels[0], "Exact saved source for this advertised application surface."
	}
	switch kind {
	case atlasstudy.ReadingTargetEntrypoint:
		return "Application entrypoint", "Exact saved source for an application entrypoint."
	case atlasstudy.ReadingTargetMethod:
		return "Repository method", "Exact saved source at this repository method."
	case atlasstudy.ReadingTargetFunction:
		return "Repository function", "Exact saved source at this repository function."
	default:
		return "Repository source", "Exact saved repository source."
	}
}

func bindAtlasStudyReadingTargets(input *atlasstudy.Input) {
	if input == nil {
		return
	}
	byComponent := make(map[string][]string)
	bySurface := make(map[string][]string)
	for _, target := range input.ReadingTargets {
		for _, componentID := range target.RelatedComponentIDs {
			byComponent[componentID] = append(byComponent[componentID], target.ID)
		}
		for _, principal := range target.PrincipalRefs {
			if principal.Kind == atlasstudy.RefSurface {
				bySurface[principal.ID] = append(bySurface[principal.ID], target.ID)
			}
		}
	}
	for index := range input.Architecture.Components {
		input.Architecture.Components[index].ReadingTargetIDs = append(
			[]string(nil), byComponent[input.Architecture.Components[index].ID]...,
		)
		sort.Strings(input.Architecture.Components[index].ReadingTargetIDs)
	}
	for index := range input.Surfaces {
		input.Surfaces[index].ReadingTargetIDs = append(
			[]string(nil), bySurface[input.Surfaces[index].ID]...,
		)
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
		Version: status.Version, ProjectionVersion: AtlasStudyReportProjectionVersion,
		State:           status.State,
		UnavailableCode: AtlasStudyUnavailableCode(status.UnavailableCode),
		FailureCode:     status.FailureCode, DirectionCount: status.DirectionCount,
		ConsideredSpanCount:    status.ConsideredSpanCount,
		AdvertisedSpanCount:    status.AdvertisedSpanCount,
		ModelSelectedSpanCount: status.ModelSelectedSpanCount,
		AcceptedSpanCount:      status.AcceptedSpanCount,
		FrontierComplete:        status.FrontierComplete,
		SelectedItemsComplete:   status.SelectedItemsComplete,
		SupportCoverageComplete: status.SupportCoverageComplete,
		PortfolioTargetMet:      status.PortfolioTargetMet,
	}
	if status.State == atlasstudy.ProductStateAccepted ||
		status.State == atlasstudy.ProductStateAcceptedPartial ||
		status.State == atlasstudy.ProductStateFailed {
		reportStatus.CandidateCoverage, err = projectAtlasStudyCandidateCoverage(status.CandidateCoverage)
		if err != nil {
			return nil, nil, err
		}
		reportStatus.Omissions = projectAtlasStudyOmissions(status.CandidateCoverage.Omissions)
	}
	switch status.State {
	case atlasstudy.ProductStateAccepted, atlasstudy.ProductStateAcceptedPartial:
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
		studyData, localErr := atlasStudyLocalD177Data(data)
		if localErr != nil {
			return nil, nil, localErr
		}
		studyMap, projectErr := projectAtlasStudyMap(
			studyData,
			data.ArchitectureCanvas,
			input,
			result,
		)
		if projectErr != nil {
			return nil, nil, projectErr
		}
		// DirectionCount remains the exact accepted result/status count.
		// Publication counts are an explicit versioned report projection; exact
		// duplicate reading sets remain available as hidden diagnostics.
		reportStatus.PublishedDirectionCount = len(studyMap.Directions)
		reportStatus.HiddenDirectionCount = len(studyMap.HiddenDirections)
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
	if data == nil || data.RepositoryAtlas == nil || data.ArchitectureCanvas == nil {
		return nil
	}
	if status := data.ArchitectureSynthesis; status != nil &&
		status.State == ArchitectureSynthesisUnavailable && status.UnavailableCode == "offline" {
		return &AtlasStudyReportStatus{
			Version: atlasstudy.ResultVersion, ProjectionVersion: AtlasStudyReportProjectionVersion,
			State:           atlasstudy.ProductStateUnavailable,
			UnavailableCode: AtlasStudyUnavailableOffline,
		}
	}
	input, err := BuildAtlasStudyInput(data, atlasstudy.LanguageEnglish)
	if err == nil {
		insufficient := !AtlasStudyInputHasMinimumCatalog(input)
		if !insufficient {
			_, compileErr := atlasstudy.Compile(input)
			var unavailable *atlasstudy.CandidateUnavailableError
			insufficient = errors.As(compileErr, &unavailable)
		}
		if insufficient {
			return &AtlasStudyReportStatus{
				Version: atlasstudy.ResultVersion, ProjectionVersion: AtlasStudyReportProjectionVersion,
				State:           atlasstudy.ProductStateUnavailable,
				UnavailableCode: AtlasStudyUnavailableInsufficientCatalog,
			}
		}
	}
	return nil
}

func projectAtlasStudyMap(
	data *ReportData,
	publishedCanvas *ArchitectureCanvas,
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
		}
		projected.MapTarget = exactPublishedStudyComponentTarget(component, publishedCanvas)
		var owned []atlasstudy.ReadingTarget
		for _, target := range input.ReadingTargets {
			if target.Owner != ref {
				continue
			}
			owned = append(owned, target)
		}
		// A Shape component is conceptual and can legitimately own several
		// exact reading locations. The provider selected only the component,
		// not one representative location, so publish a source action only when
		// local producer proof leaves exactly one possible target.
		if len(owned) == 1 {
			source, sourceErr := exactAtlasStudySource(data, owned[0])
			if sourceErr != nil {
				return RepositoryStudyArea{}, sourceErr
			}
			projected.CodeLocation = &UserCodeLocation{
				Path: owned[0].Location.Path, Line: owned[0].Location.Line,
			}
			projected.Source = &source
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
	seenReadingSets := make(map[string]struct{}, len(result.Directions))
	for _, direction := range result.Directions {
		readingSetKey, err := atlasStudyDirectionReadingSetKey(direction, targets)
		if err != nil {
			return nil, err
		}
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
				Symbol:   target.Symbol,
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
		if _, duplicate := seenReadingSets[readingSetKey]; duplicate {
			markStudyDirectionUserVisible(&projected, false, "duplicate_reading_set")
			studyMap.HiddenDirections = append(studyMap.HiddenDirections, projected)
			continue
		}
		seenReadingSets[readingSetKey] = struct{}{}
		studyMap.Directions = append(studyMap.Directions, projected)
	}
	return studyMap, nil
}

// exactPublishedStudyComponentTarget bridges the local D177 component used by
// Atlas Study to the independently accepted component projection that is
// actually visible in this report. The join uses only exact typed member IDs.
// Ambiguous many-to-many conceptual membership deliberately produces no
// singular focus instead of choosing a component by order.
func exactPublishedStudyComponentTarget(
	local ArchitectureComponent,
	published *ArchitectureCanvas,
) *UserMapTarget {
	if published == nil {
		return nil
	}
	localMembers := make(map[componentmap.MemberID]struct{}, len(local.Members))
	for _, member := range local.Members {
		localMembers[member.ID] = struct{}{}
	}
	if len(localMembers) == 0 {
		return nil
	}
	var matched componentmap.ComponentID
	for _, component := range published.Components {
		if component.ID == published.LocalRemainderComponentID {
			continue
		}
		intersects := false
		for _, member := range component.Members {
			if _, ok := localMembers[member.ID]; ok {
				intersects = true
				break
			}
		}
		if !intersects {
			continue
		}
		if matched != "" {
			return nil
		}
		matched = component.ID
	}
	if matched == "" {
		return nil
	}
	return &UserMapTarget{
		Kind: SemanticSearchTargetComponent, ComponentID: matched,
	}
}

func atlasStudyDirectionReadingSetKey(
	direction atlasstudy.Direction,
	targets map[string]atlasstudy.ReadingTarget,
) (string, error) {
	locators := make([]string, 0, len(direction.Reading))
	for _, reading := range direction.Reading {
		target, ok := targets[reading.Target.ID]
		if reading.Target.Kind != atlasstudy.RefReadingTarget || !ok {
			return "", fmt.Errorf("atlas study report: direction references unavailable reading target")
		}
		locators = append(locators, strings.Join([]string{
			target.ID, string(target.Kind), target.Location.Path,
			fmt.Sprint(target.Location.Line), target.Symbol,
		}, "\x00"))
	}
	sort.Strings(locators)
	return direction.Span.ID + "\x02" + strings.Join(locators, "\x01"), nil
}

func exactAtlasStudySource(
	data *ReportData,
	target atlasstudy.ReadingTarget,
) (SourceSnippet, error) {
	var matches []SourceSnippet
	for _, source := range data.UserSources {
		derivedSymbol, _, _, _ := atlasStudyReadingTargetContext(
			source,
			target.Location.Line,
			target.Kind == atlasstudy.ReadingTargetEntrypoint,
			nil,
		)
		if source.Path != target.Location.Path ||
			atlasStudySourceFocusLine(source) != target.Location.Line ||
			derivedSymbol != target.Symbol || source.Validate() != nil {
			continue
		}
		matches = append(matches, source)
	}
	selected, found, conflict := resolveAtlasStudySourceCandidates(matches)
	if !found {
		return SourceSnippet{}, fmt.Errorf(
			"atlas study report: reading target %q has %d exact saved sources", target.ID, len(matches),
		)
	}
	if conflict {
		return SourceSnippet{}, fmt.Errorf(
			"atlas study report: reading target %q has conflicting exact saved sources", target.ID,
		)
	}
	return selected, nil
}

// resolveAtlasStudySourceCandidates mirrors the saved-source projection's
// exact overlap contract: equivalent excerpts are deduplicated, the narrowest
// containing interval wins deterministically, and only incompatible content
// for that same most-specific interval is a conflict. A broader saved excerpt
// is normal context, not a second reading location.
func resolveAtlasStudySourceCandidates(
	values []SourceSnippet,
) (SourceSnippet, bool, bool) {
	if len(values) == 0 {
		return SourceSnippet{}, false, false
	}
	candidates := append([]SourceSnippet(nil), values...)
	sort.SliceStable(candidates, func(i, j int) bool {
		leftSpan := candidates[i].EndLine - candidates[i].StartLine
		rightSpan := candidates[j].EndLine - candidates[j].StartLine
		if leftSpan != rightSpan {
			return leftSpan < rightSpan
		}
		if candidates[i].StartLine != candidates[j].StartLine {
			return candidates[i].StartLine < candidates[j].StartLine
		}
		return candidates[i].PresentationSHA256 < candidates[j].PresentationSHA256
	})
	selected := candidates[0]
	selectedSpan := selected.EndLine - selected.StartLine
	for _, candidate := range candidates[1:] {
		if candidate.EndLine-candidate.StartLine != selectedSpan ||
			candidate.StartLine != selected.StartLine || candidate.EndLine != selected.EndLine {
			break
		}
		if candidate.Revision != selected.Revision ||
			candidate.ContentSHA256 != selected.ContentSHA256 {
			return SourceSnippet{}, false, true
		}
	}
	return selected, true, false
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
