package report

import (
	"fmt"
	"strconv"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/evidence"
	"github.com/dvordrova/repomap/internal/flowproof"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/modelresearch"
	"github.com/dvordrova/repomap/internal/semanticdiscovery"
)

// addRemainingPresentationTextInventory completes the single typed inventory
// after the historically supported Study, Architecture grouping, Guided Tour,
// and Mechanism fields have been registered. Every address below belongs only
// to the terminal presentation. It is never used as semantic identity,
// evidence identity, retrieval input, or source authority.
func addRemainingPresentationTextInventory(
	bindings *presentationLocalizationBindings,
	data *ReportData,
	protected []localization.ProtectedValue,
) error {
	add := func(
		address string,
		text string,
		setter func(*ReportData, string) bool,
	) error {
		return bindings.addAddress(address, text, protected, setter)
	}
	addObject := func(
		address string,
		text string,
		objectProtected []localization.ProtectedValue,
		setter func(*ReportData, string) bool,
	) error {
		combined := append([]localization.ProtectedValue(nil), protected...)
		combined = append(combined, objectProtected...)
		return bindings.addAddress(address, text, combined, setter)
	}

	if err := addOrientationPresentationText(add, addObject, data); err != nil {
		return err
	}
	if err := addCandidateDirectionProofPresentationText(bindings, data); err != nil {
		return err
	}
	if err := addLegacyComponentPresentationText(add, data); err != nil {
		return err
	}
	if err := addArchitecturePresentationText(addObject, data); err != nil {
		return err
	}
	if err := addSemanticPresentationText(addObject, data); err != nil {
		return err
	}
	if err := addOnboardingPresentationText(add, addObject, data); err != nil {
		return err
	}
	if err := addStudyPresentationText(add, data); err != nil {
		return err
	}
	if err := addMechanismPresentationText(add, data); err != nil {
		return err
	}
	if err := addOperationsPresentationText(add, data); err != nil {
		return err
	}
	if err := addTaskInvestigationPresentationText(addObject, data); err != nil {
		return err
	}
	if err := addSourceExplanationPresentationText(add, addObject, data); err != nil {
		return err
	}
	if err := addResearchPresentationText(addObject, data); err != nil {
		return err
	}
	if err := addSourceEpisodePresentationText(add, data); err != nil {
		return err
	}
	return addSurfacePresentationText(add, data)
}

type presentationInventoryAdder func(
	address string,
	text string,
	setter func(*ReportData, string) bool,
) error

type presentationInventoryObjectAdder func(
	address string,
	text string,
	protected []localization.ProtectedValue,
	setter func(*ReportData, string) bool,
) error

func (add presentationInventoryObjectAdder) with(
	protected []localization.ProtectedValue,
) presentationInventoryAdder {
	return func(
		address string,
		text string,
		setter func(*ReportData, string) bool,
	) error {
		return add(address, text, protected, setter)
	}
}

func presentationAddress(path string, stableParts ...string) string {
	if len(stableParts) == 0 {
		return path
	}
	return path + "/" + presentationOwnerDigest(stableParts...)
}

func candidateDirectionProofProtectedValues(
	direction CandidateDirection,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(localization.ProtectedIdentifier, direction.ID)
	builder.add(localization.ProtectedSymbol, direction.LikelyEntrypoint)
	for _, filePath := range direction.LikelyFiles {
		builder.add(localization.ProtectedPath, filePath)
	}
	if direction.LocalProof == nil {
		return builder.values
	}
	proof := direction.LocalProof.Proof
	builder.add(
		localization.ProtectedIdentifier,
		proof.ID,
		proof.Command,
		proof.SeedSurfaceID,
	)
	for _, anchor := range proof.Anchors {
		builder.add(localization.ProtectedIdentifier, anchor.ID, anchor.Label)
		builder.add(localization.ProtectedSymbol, anchor.QualifiedName)
		if anchor.Location != nil {
			builder.add(localization.ProtectedPath, anchor.Location.Path)
		}
	}
	for _, transition := range proof.Transitions {
		builder.add(
			localization.ProtectedIdentifier,
			transition.ID,
			transition.From,
			transition.To,
		)
		builder.add(localization.ProtectedPath, transition.Evidence.Path)
		if transition.Condition != nil {
			builder.add(localization.ProtectedIdentifier, transition.Condition.Expression)
			builder.add(localization.ProtectedPath, transition.Condition.Location.Path)
		}
	}
	return builder.values
}

func flowPresentationProtectedValues(flow FlowData) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		flow.ID,
		flow.FlowType,
		flow.FlowStatus,
		flow.CandidateBasis,
	)
	for _, step := range flow.LikelyChain {
		for _, filePath := range step.EvidenceFiles {
			builder.add(localization.ProtectedPath, filePath)
		}
	}
	for _, collection := range [][]FileItem{
		flow.FilesToRead,
		flow.TestsToRead,
		flow.BundleFiles,
		flow.BundleTests,
		flow.BundleDocs,
	} {
		for _, item := range collection {
			builder.add(localization.ProtectedPath, item.Path)
		}
	}
	for _, item := range flow.UnverifiedPaths {
		builder.add(localization.ProtectedPath, item.Path)
	}
	for _, packagePath := range flow.BundlePackages {
		builder.add(localization.ProtectedPackage, packagePath)
	}
	for _, edge := range flow.BundleEdges {
		builder.add(localization.ProtectedPackage, edge.From, edge.To)
	}
	return builder.values
}

// objectProtectedValueBuilder keeps opaque identities scoped to the one
// presentation object whose prose may mention them. In particular, ordinary
// lowercase words such as "serve", "start", or "main" must not become global
// translation exclusions merely because they are technical identities in one
// topic or investigation.
type objectProtectedValueBuilder struct {
	values []localization.ProtectedValue
}

func (builder *objectProtectedValueBuilder) add(
	kind localization.ProtectedKind,
	values ...string,
) {
	for _, value := range values {
		if value == "" {
			continue
		}
		builder.values = append(builder.values, localization.ProtectedValue{
			Kind:  kind,
			Value: value,
		})
	}
}

func (builder *objectProtectedValueBuilder) addSource(source SourceSnippet) {
	builder.add(localization.ProtectedPath, source.Path)
	builder.add(localization.ProtectedSymbol, source.EnclosingSymbol)
	builder.add(
		localization.ProtectedIdentifier,
		source.Language,
		source.ContentSHA256,
		source.PresentationSHA256,
		source.Role,
		string(source.LandmarkKind),
		source.Revision,
	)
	for _, id := range source.RelatedEvidenceIDs {
		builder.add(localization.ProtectedIdentifier, id)
	}
}

func sourceSnippetProtectedValues(
	source SourceSnippet,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.addSource(source)
	return builder.values
}

func userTopicProtectedValues(topic UserTopic) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(localization.ProtectedIdentifier, topic.CandidateID)
	for _, symbol := range topic.StartingSymbols {
		builder.add(localization.ProtectedPath, symbol.Path)
		builder.add(localization.ProtectedSymbol, symbol.Symbol)
	}
	return builder.values
}

func semanticArtifactProtectedValues(
	artifact semanticdiscovery.Artifact,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		artifact.ID,
		artifact.CandidateID,
		string(artifact.Kind),
		string(artifact.Verdict),
		string(artifact.Confidence),
	)
	appendFocus := func(focus semanticdiscovery.Focus) {
		for _, id := range focus.ComponentIDs {
			builder.add(localization.ProtectedIdentifier, id)
		}
		for _, id := range focus.FlowIDs {
			builder.add(localization.ProtectedIdentifier, id)
		}
		for _, id := range focus.SurfaceIDs {
			builder.add(localization.ProtectedIdentifier, id)
		}
	}
	appendEvidence := func(evidence semanticdiscovery.EvidenceRef) {
		builder.add(localization.ProtectedIdentifier, evidence.ID, evidence.Kind)
		builder.add(localization.ProtectedPath, evidence.Path)
	}
	appendFocus(artifact.Focus)
	for _, evidence := range artifact.Evidence {
		appendEvidence(evidence)
	}
	for _, statement := range artifact.Statements {
		builder.add(
			localization.ProtectedIdentifier,
			statement.ID,
			string(statement.Basis),
		)
		for _, values := range [][]string{
			statement.SupportIDs,
			statement.SourceGroups,
			statement.AspectIDs,
		} {
			for _, value := range values {
				builder.add(localization.ProtectedIdentifier, value)
			}
		}
	}
	for _, step := range artifact.Steps {
		builder.add(localization.ProtectedIdentifier, step.ID)
		for _, id := range step.StatementIDs {
			builder.add(localization.ProtectedIdentifier, id)
		}
		appendFocus(step.Focus)
		for _, evidence := range step.Evidence {
			appendEvidence(evidence)
		}
	}
	for _, values := range [][]string{
		artifact.RequiredAspectIDs,
		artifact.CoveredAspectIDs,
		artifact.UncoveredAspectIDs,
		artifact.UsedFactIDs,
		artifact.UnusedAvailableFactIDs,
		artifact.RelatedArtifactIDs,
	} {
		for _, value := range values {
			builder.add(localization.ProtectedIdentifier, value)
		}
	}
	return builder.values
}

func appendThesisAreaProtectedValues(
	builder *objectProtectedValueBuilder,
	area RepositoryThesisArea,
) {
	builder.add(localization.ProtectedIdentifier, string(area.Role))
	if area.CodeLocation != nil {
		builder.add(localization.ProtectedPath, area.CodeLocation.Path)
	}
	if area.MapTarget != nil {
		builder.add(
			localization.ProtectedIdentifier,
			string(area.MapTarget.Kind),
			string(area.MapTarget.ComponentID),
			string(area.MapTarget.FlowID),
			area.MapTarget.SurfaceID,
		)
	}
}

func repositoryThesisProtectedValues(
	thesis RepositoryThesis,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(localization.ProtectedIdentifier, thesis.RecommendedArtifactID)
	for _, area := range thesis.Areas {
		appendThesisAreaProtectedValues(&builder, area)
	}
	return builder.values
}

func repositoryGuideProtectedValues(
	guide RepositoryGuide,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(localization.ProtectedIdentifier, guide.StartHereArtifactID)
	for _, values := range [][]string{
		guide.ExtensionArtifactIDs,
		guide.MorePathArtifactIDs,
	} {
		for _, value := range values {
			builder.add(localization.ProtectedIdentifier, value)
		}
	}
	for _, area := range guide.Areas {
		appendThesisAreaProtectedValues(&builder, area)
	}
	for _, target := range guide.ReadNext {
		builder.add(localization.ProtectedPath, target.Path)
		builder.add(localization.ProtectedSymbol, target.Symbol)
	}
	return builder.values
}

func taskInvestigationProtectedValues(
	task TaskInvestigationWorkspace,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		task.TaskID,
		task.State,
		string(task.Locality),
		string(task.Profile),
		string(task.Interpretation.Kind),
		string(task.RoleContract.Profile),
		string(task.CheapExit.Route),
		task.CapturedRevision,
		task.BundleSHA256,
		task.AttemptSHA256,
		task.PackSHA256,
		task.StatusSHA256,
		task.RetrievalTraceSHA256,
		task.RetrievalTraceMarkdownSHA256,
		task.RepositoryStateSHA256,
	)
	builder.add(localization.ProtectedProduct, task.Repository)
	for _, term := range task.Interpretation.FoundTerms {
		builder.add(localization.ProtectedIdentifier, term)
	}
	for _, term := range task.Interpretation.UserProvidedOnly {
		builder.add(localization.ProtectedIdentifier, term)
	}
	for _, value := range task.StagesSkipped {
		builder.add(localization.ProtectedIdentifier, value)
	}
	for _, value := range task.MaterialPaths {
		builder.add(localization.ProtectedPath, value)
	}
	for _, requirement := range task.RoleContract.Key {
		builder.add(localization.ProtectedIdentifier, string(requirement.Role))
	}
	for _, requirement := range task.RoleContract.Supporting {
		builder.add(localization.ProtectedIdentifier, string(requirement.Role))
	}
	for _, requirement := range task.RoleContract.Optional {
		builder.add(localization.ProtectedIdentifier, string(requirement.Role))
	}
	for _, gate := range task.CheapExit.Gates {
		builder.add(localization.ProtectedIdentifier, string(gate.Gate))
	}
	for _, anchor := range task.Anchors {
		builder.add(localization.ProtectedPath, anchor.Path)
		builder.add(localization.ProtectedSymbol, anchor.Symbol)
		builder.add(localization.ProtectedIdentifier, anchor.Section)
		builder.add(localization.ProtectedPackage, anchor.Package)
		builder.add(
			localization.ProtectedIdentifier,
			string(anchor.Role),
			string(anchor.Scope.ScopeKind),
			string(anchor.Scope.NegativeEvidenceBasis),
		)
		builder.addSource(anchor.Source)
	}
	for _, join := range task.EvidenceJoins {
		builder.add(
			localization.ProtectedIdentifier,
			join.Kind,
			string(join.Support),
		)
	}
	for _, hypothesis := range task.WorkingHypothesis {
		builder.add(localization.ProtectedIdentifier, string(hypothesis.Status))
	}
	for _, guidance := range task.ReproduceOrObserve {
		builder.add(localization.ProtectedIdentifier, string(guidance.Authority))
	}
	for _, guidance := range task.Verify.Steps {
		builder.add(localization.ProtectedIdentifier, string(guidance.Authority))
	}
	for _, probe := range task.NextProbes {
		builder.add(localization.ProtectedIdentifier, string(probe.Action))
	}
	return builder.values
}

func modelResearchProtectedValues(
	state modelresearch.State,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	appendRepository := func(repository modelresearch.RepositoryContext) {
		builder.add(localization.ProtectedProduct, repository.Identity)
		builder.add(
			localization.ProtectedIdentifier,
			repository.Revision,
			repository.DirtySHA256,
			repository.Scenario,
		)
	}
	appendFrontier := func(frontier modelresearch.Frontier) {
		builder.add(localization.ProtectedIdentifier, frontier.EvidenceCategory)
		for _, id := range frontier.EvidenceIDs {
			builder.add(localization.ProtectedIdentifier, id)
		}
	}
	appendFinding := func(finding modelresearch.ValidatedFinding) {
		builder.add(localization.ProtectedIdentifier, finding.ID)
		for _, id := range finding.EvidenceIDs {
			builder.add(localization.ProtectedIdentifier, id)
		}
	}
	appendRound := func(round modelresearch.ResearchRound) {
		builder.add(
			localization.ProtectedIdentifier,
			round.ID,
			round.Purpose,
			string(round.Status),
			round.LocalEvidenceBundleSHA256,
			round.ProviderRequestSHA256,
			round.CacheKey,
			round.PromptVersion,
			round.Profile,
			round.Model,
			round.StopReason,
		)
		for _, value := range round.InputEvidenceIDs {
			builder.add(localization.ProtectedIdentifier, value)
		}
		for _, group := range [][]string{
			round.LocalFilesInspected,
			round.ProviderVisiblePaths,
		} {
			for _, value := range group {
				builder.add(localization.ProtectedPath, value)
			}
		}
		for _, finding := range round.ValidatedFindings {
			appendFinding(finding)
		}
		for _, finding := range round.RejectedFindings {
			builder.add(localization.ProtectedIdentifier, finding.Finding.ID)
			for _, id := range finding.Finding.EvidenceIDs {
				builder.add(localization.ProtectedIdentifier, id)
			}
		}
		for _, frontier := range round.UnresolvedFrontiers {
			appendFrontier(frontier)
		}
	}
	appendEvidence := func(item modelresearch.EvidenceItem) {
		builder.add(
			localization.ProtectedIdentifier,
			item.ID,
			string(item.Kind),
			item.Relation,
			string(item.Certainty),
		)
		builder.add(localization.ProtectedSymbol, item.Symbol)
		if item.Location != nil {
			builder.add(localization.ProtectedPath, item.Location.Path)
		}
		for _, provenance := range item.Provenance {
			builder.add(
				localization.ProtectedIdentifier,
				provenance.Provider,
				provenance.Version,
				provenance.Operation,
			)
			if provenance.Location != nil {
				builder.add(localization.ProtectedPath, provenance.Location.Path)
			}
		}
	}
	appendRepository(state.Repository)
	appendRepository(state.Theory.Repository)
	builder.add(localization.ProtectedIdentifier, state.Policy.Version)
	for _, round := range state.Rounds {
		appendRound(round)
	}
	for _, round := range state.SkippedRounds {
		appendRound(round)
	}
	for _, item := range state.Theory.GroundedFacts {
		appendEvidence(item)
	}
	for _, finding := range state.Theory.AcceptedModelInterpretations {
		appendFinding(finding)
	}
	for _, finding := range state.Theory.RejectedModelClaims {
		builder.add(localization.ProtectedIdentifier, finding.Finding.ID)
		for _, id := range finding.Finding.EvidenceIDs {
			builder.add(localization.ProtectedIdentifier, id)
		}
	}
	for _, frontier := range state.Theory.UnresolvedFrontiers {
		appendFrontier(frontier)
	}
	for _, values := range [][]string{
		state.Theory.RelatedComponentIDs,
		state.Theory.RelatedSurfaceIDs,
		state.Theory.RelatedTraceIDs,
	} {
		for _, value := range values {
			builder.add(localization.ProtectedIdentifier, value)
		}
	}
	return builder.values
}

func addOrientationPresentationText(
	add presentationInventoryAdder,
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	for index := range data.HighLevelMap {
		index := index
		owner := presentationAddress(
			"orientation/high_level_map",
			string(data.HighLevelMap[index].Role),
			fmt.Sprint(data.HighLevelMap[index].Evidence),
		)
		if err := add(owner+"/name", data.HighLevelMap[index].Name, func(target *ReportData, text string) bool {
			if index >= len(target.HighLevelMap) {
				return false
			}
			target.HighLevelMap[index].Name = text
			return true
		}); err != nil {
			return err
		}
		if err := add(owner+"/why_it_matters", data.HighLevelMap[index].WhyItMatters, func(target *ReportData, text string) bool {
			if index >= len(target.HighLevelMap) {
				return false
			}
			target.HighLevelMap[index].WhyItMatters = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range data.FirstFilesToOpen {
		index := index
		itemAdd := addObject.with([]localization.ProtectedValue{{
			Kind:  localization.ProtectedPath,
			Value: data.FirstFilesToOpen[index].Path,
		}})
		owner := presentationAddress(
			"orientation/first_files",
			data.FirstFilesToOpen[index].Path,
			strconv.Itoa(data.FirstFilesToOpen[index].Priority),
		)
		if err := itemAdd(owner+"/reason", data.FirstFilesToOpen[index].Reason, func(target *ReportData, text string) bool {
			if index >= len(target.FirstFilesToOpen) {
				return false
			}
			target.FirstFilesToOpen[index].Reason = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range data.ImportantDomainWords {
		index := index
		wordAdd := addObject.with([]localization.ProtectedValue{{
			Kind:  localization.ProtectedIdentifier,
			Value: data.ImportantDomainWords[index].Word,
		}})
		owner := presentationAddress(
			"orientation/domain_words",
			data.ImportantDomainWords[index].Word,
		)
		if err := wordAdd(owner+"/guess", data.ImportantDomainWords[index].Guess, func(target *ReportData, text string) bool {
			if index >= len(target.ImportantDomainWords) {
				return false
			}
			target.ImportantDomainWords[index].Guess = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range data.QuestionsForHuman {
		index := index
		if err := add(
			"orientation/questions/"+strconv.Itoa(index),
			data.QuestionsForHuman[index],
			func(target *ReportData, text string) bool {
				if index >= len(target.QuestionsForHuman) {
					return false
				}
				target.QuestionsForHuman[index] = text
				return true
			},
		); err != nil {
			return err
		}
	}
	for index := range data.OrientationUnverifiedPaths {
		index := index
		pathAdd := addObject.with([]localization.ProtectedValue{{
			Kind:  localization.ProtectedPath,
			Value: data.OrientationUnverifiedPaths[index].Path,
		}})
		owner := presentationAddress(
			"orientation/unverified_paths",
			data.OrientationUnverifiedPaths[index].Path,
		)
		if err := pathAdd(owner+"/reason", data.OrientationUnverifiedPaths[index].Reason, func(target *ReportData, text string) bool {
			if index >= len(target.OrientationUnverifiedPaths) {
				return false
			}
			target.OrientationUnverifiedPaths[index].Reason = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range data.CandidateDirections {
		index := index
		id := data.CandidateDirections[index].ID
		directionAdd := addObject.with(
			candidateDirectionProofProtectedValues(data.CandidateDirections[index]),
		)
		owner := "orientation/directions/" + id
		fields := []struct {
			name string
			text string
			set  func(*CandidateDirection, string)
		}{
			{"name", data.CandidateDirections[index].Name, func(item *CandidateDirection, text string) { item.Name = text }},
			{"trigger", data.CandidateDirections[index].Trigger, func(item *CandidateDirection, text string) { item.Trigger = text }},
			{"why_interesting", data.CandidateDirections[index].WhyInteresting, func(item *CandidateDirection, text string) { item.WhyInteresting = text }},
			{"disposition_reason", data.CandidateDirections[index].DispositionReason, func(item *CandidateDirection, text string) { item.DispositionReason = text }},
		}
		for _, field := range fields {
			field := field
			if err := directionAdd(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				for directionIndex := range target.CandidateDirections {
					if target.CandidateDirections[directionIndex].ID == id {
						field.set(&target.CandidateDirections[directionIndex], text)
						return true
					}
				}
				return false
			}); err != nil {
				return err
			}
		}
		for evidenceIndex := range data.CandidateDirections[index].Evidence {
			evidenceIndex := evidenceIndex
			if err := directionAdd(
				owner+"/evidence/"+strconv.Itoa(evidenceIndex),
				data.CandidateDirections[index].Evidence[evidenceIndex],
				func(target *ReportData, text string) bool {
					for directionIndex := range target.CandidateDirections {
						direction := &target.CandidateDirections[directionIndex]
						if direction.ID != id || evidenceIndex >= len(direction.Evidence) {
							continue
						}
						direction.Evidence[evidenceIndex] = text
						return true
					}
					return false
				},
			); err != nil {
				return err
			}
		}
	}
	for index := range data.CandidateFlows {
		index := index
		canonicalName := data.CandidateFlows[index]
		flowAdd := add
		ownerID := strconv.Itoa(index)
		if index < len(data.CandidateDirections) &&
			data.CandidateDirections[index].ID != "" {
			ownerID = data.CandidateDirections[index].ID
			flowAdd = addObject.with(
				candidateDirectionProofProtectedValues(data.CandidateDirections[index]),
			)
		}
		owner := "orientation/candidate_flow_names/" + ownerID
		if err := flowAdd(owner+"/name", canonicalName, func(target *ReportData, text string) bool {
			if index >= len(target.CandidateFlows) {
				return false
			}
			target.CandidateFlows[index] = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range data.Flows {
		if err := addFlowPresentationText(
			addObject.with(flowPresentationProtectedValues(data.Flows[index])),
			data.Flows[index],
			index,
		); err != nil {
			return err
		}
	}
	for index := range data.Warnings {
		index := index
		if fixedProductWarningMessageID(data, index) != "" {
			continue
		}
		if err := add(
			"run/warnings/"+strconv.Itoa(index),
			data.Warnings[index],
			func(target *ReportData, text string) bool {
				if index >= len(target.Warnings) {
					return false
				}
				if len(target.PresentationWarnings) != len(target.Warnings) {
					target.PresentationWarnings = append(
						[]string(nil),
						target.Warnings...,
					)
				}
				if len(target.PresentationWarningKinds) < len(target.Warnings) {
					kinds := make([]string, len(target.Warnings))
					copy(kinds, target.PresentationWarningKinds)
					target.PresentationWarningKinds = kinds
				}
				target.PresentationWarnings[index] = text
				// ApplyPresentationLocalization always operates on a render clone.
				// Replace dynamic warning prose there so the terminal artifact does
				// not retain a second visible/copyable English value. Fixed
				// deterministic warnings never enter this inventory and remain
				// catalog-owned.
				target.Warnings[index] = text
				return true
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func addCandidateDirectionProofPresentationText(
	bindings *presentationLocalizationBindings,
	data *ReportData,
) error {
	for directionIndex := range data.CandidateDirections {
		directionIndex := directionIndex
		direction := data.CandidateDirections[directionIndex]
		directionID := direction.ID
		owner := "orientation/directions/" + directionID
		protected := candidateDirectionProofProtectedValues(direction)
		add := func(
			address string,
			text string,
			setter func(*ReportData, string) bool,
		) error {
			// Proof prose is bound only to exact identities from this direction.
			// Report-global identifiers may be ordinary language in another proof.
			return bindings.addAddress(address, text, protected, setter)
		}

		if verification := direction.LocalVerification; verification != nil {
			for statementIndex := range verification.Verified {
				statementIndex := statementIndex
				if err := add(
					owner+"/local_verification/verified/"+strconv.Itoa(statementIndex),
					verification.Verified[statementIndex],
					func(target *ReportData, text string) bool {
						current := findCandidateDirectionForPresentation(
							target,
							directionID,
							directionIndex,
						)
						if current == nil || current.LocalVerification == nil ||
							statementIndex >= len(current.LocalVerification.Verified) {
							return false
						}
						current.LocalVerification.Verified[statementIndex] = text
						return true
					},
				); err != nil {
					return err
				}
			}
			for statementIndex := range verification.Missing {
				statementIndex := statementIndex
				if err := add(
					owner+"/local_verification/missing/"+strconv.Itoa(statementIndex),
					verification.Missing[statementIndex],
					func(target *ReportData, text string) bool {
						current := findCandidateDirectionForPresentation(
							target,
							directionID,
							directionIndex,
						)
						if current == nil || current.LocalVerification == nil ||
							statementIndex >= len(current.LocalVerification.Missing) {
							return false
						}
						current.LocalVerification.Missing[statementIndex] = text
						return true
					},
				); err != nil {
					return err
				}
			}
		}

		if direction.LocalProof == nil {
			continue
		}
		for slotIndex := range direction.LocalProof.Proof.Slots {
			slotIndex := slotIndex
			slot := direction.LocalProof.Proof.Slots[slotIndex]
			slotKind := slot.Kind
			slotID := string(slotKind)
			if slotID == "" {
				slotID = strconv.Itoa(slotIndex)
			}
			slotOwner := owner + "/local_proof/slots/" + slotID
			for _, field := range []struct {
				name string
				text string
				set  func(*flowproof.Slot, string)
			}{
				{"summary", slot.Summary, func(item *flowproof.Slot, text string) { item.Summary = text }},
				{"missing", slot.Missing, func(item *flowproof.Slot, text string) { item.Missing = text }},
			} {
				field := field
				if err := add(slotOwner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
					current := findCandidateDirectionForPresentation(
						target,
						directionID,
						directionIndex,
					)
					if current == nil || current.LocalProof == nil {
						return false
					}
					for currentSlotIndex := range current.LocalProof.Proof.Slots {
						currentSlot := &current.LocalProof.Proof.Slots[currentSlotIndex]
						if slotKind != "" && currentSlot.Kind != slotKind {
							continue
						}
						if slotKind == "" && currentSlotIndex != slotIndex {
							continue
						}
						field.set(currentSlot, text)
						return true
					}
					return false
				}); err != nil {
					return err
				}
			}
		}
		if stop := direction.LocalProof.Stop; stop != nil {
			if err := add(
				owner+"/local_proof/stop/message",
				stop.Message,
				func(target *ReportData, text string) bool {
					current := findCandidateDirectionForPresentation(
						target,
						directionID,
						directionIndex,
					)
					if current == nil || current.LocalProof == nil || current.LocalProof.Stop == nil {
						return false
					}
					current.LocalProof.Stop.Message = text
					return true
				},
			); err != nil {
				return err
			}
		}
		if err := add(
			owner+"/local_proof/current_frontier",
			direction.LocalProof.Proof.CurrentFrontier,
			func(target *ReportData, text string) bool {
				current := findCandidateDirectionForPresentation(
					target,
					directionID,
					directionIndex,
				)
				if current == nil || current.LocalProof == nil {
					return false
				}
				current.LocalProof.Proof.CurrentFrontier = text
				return true
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func findCandidateDirectionForPresentation(
	data *ReportData,
	directionID string,
	fallbackIndex int,
) *CandidateDirection {
	for index := range data.CandidateDirections {
		if data.CandidateDirections[index].ID == directionID {
			return &data.CandidateDirections[index]
		}
	}
	if directionID == "" && fallbackIndex >= 0 && fallbackIndex < len(data.CandidateDirections) {
		return &data.CandidateDirections[fallbackIndex]
	}
	return nil
}

func fixedProductWarningMessageID(data *ReportData, index int) string {
	if presentation, ok := runPresentationWarningForIndex(data, index); ok {
		return presentation.MessageID
	}
	if data == nil || index < 0 || index >= len(data.PresentationWarningKinds) {
		return ""
	}
	messageID := data.PresentationWarningKinds[index]
	switch messageID {
	case studyPublicationMessageEditingDidNotFinish,
		studyPublicationMessageChecksFailed,
		studyPublicationMessageNoSourceAdapter,
		studyPublicationMessageNoSourceFunctions:
		return messageID
	default:
		return ""
	}
}

func addFlowPresentationText(
	add presentationInventoryAdder,
	flow FlowData,
	flowIndex int,
) error {
	flowID := flow.ID
	owner := "flows/" + flowID
	flowFields := []struct {
		name string
		text string
		set  func(*FlowData, string)
	}{
		{"name", flow.Name, func(item *FlowData, text string) { item.Name = text }},
		{"summary", flow.Summary, func(item *FlowData, text string) { item.Summary = text }},
		{"error", flow.Error, func(item *FlowData, text string) { item.Error = text }},
	}
	for _, field := range flowFields {
		field := field
		if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
			for index := range target.Flows {
				if target.Flows[index].ID == flowID {
					field.set(&target.Flows[index], text)
					return true
				}
			}
			return false
		}); err != nil {
			return err
		}
	}
	for index := range flow.LikelyChain {
		index := index
		step := flow.LikelyChain[index].Step
		stepOwner := owner + "/chain/" + strconv.Itoa(step)
		if err := add(stepOwner+"/name", flow.LikelyChain[index].Name, func(target *ReportData, text string) bool {
			current := findFlow(target, flowID, flowIndex)
			if current == nil || index >= len(current.LikelyChain) {
				return false
			}
			current.LikelyChain[index].Name = text
			return true
		}); err != nil {
			return err
		}
		if err := add(stepOwner+"/what_happens", flow.LikelyChain[index].WhatHappens, func(target *ReportData, text string) bool {
			current := findFlow(target, flowID, flowIndex)
			if current == nil || index >= len(current.LikelyChain) {
				return false
			}
			current.LikelyChain[index].WhatHappens = text
			return true
		}); err != nil {
			return err
		}
	}
	for _, collection := range []struct {
		name  string
		items []FileItem
		set   func(*FlowData) *[]FileItem
	}{
		{"files_to_read", flow.FilesToRead, func(item *FlowData) *[]FileItem { return &item.FilesToRead }},
		{"tests_to_read", flow.TestsToRead, func(item *FlowData) *[]FileItem { return &item.TestsToRead }},
		{"bundle_files", flow.BundleFiles, func(item *FlowData) *[]FileItem { return &item.BundleFiles }},
		{"bundle_tests", flow.BundleTests, func(item *FlowData) *[]FileItem { return &item.BundleTests }},
		{"bundle_docs", flow.BundleDocs, func(item *FlowData) *[]FileItem { return &item.BundleDocs }},
	} {
		collection := collection
		for index := range collection.items {
			index := index
			item := collection.items[index]
			itemOwner := presentationAddress(
				owner+"/"+collection.name,
				item.Path,
				strconv.Itoa(item.Priority),
			)
			if err := add(itemOwner+"/reason", item.Reason, func(target *ReportData, text string) bool {
				current := findFlow(target, flowID, flowIndex)
				if current == nil {
					return false
				}
				items := collection.set(current)
				if index >= len(*items) {
					return false
				}
				(*items)[index].PresentationReason = text
				return true
			}); err != nil {
				return err
			}
		}
	}
	for index := range flow.UnverifiedPaths {
		index := index
		itemOwner := presentationAddress(
			owner+"/unverified_paths",
			flow.UnverifiedPaths[index].Path,
		)
		if err := add(itemOwner+"/reason", flow.UnverifiedPaths[index].Reason, func(target *ReportData, text string) bool {
			current := findFlow(target, flowID, flowIndex)
			if current == nil || index >= len(current.UnverifiedPaths) {
				return false
			}
			current.UnverifiedPaths[index].Reason = text
			return true
		}); err != nil {
			return err
		}
	}
	for _, collection := range []struct {
		name   string
		values []string
		set    func(*FlowData) *[]string
	}{
		{"unknowns", flow.Unknowns, func(item *FlowData) *[]string { return &item.Unknowns }},
		{"warnings", flow.Warnings, func(item *FlowData) *[]string { return &item.Warnings }},
	} {
		collection := collection
		for index := range collection.values {
			index := index
			if err := add(
				owner+"/"+collection.name+"/"+strconv.Itoa(index),
				collection.values[index],
				func(target *ReportData, text string) bool {
					current := findFlow(target, flowID, flowIndex)
					if current == nil {
						return false
					}
					values := collection.set(current)
					if index >= len(*values) {
						return false
					}
					(*values)[index] = text
					return true
				},
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func findFlow(data *ReportData, id string, fallbackIndex int) *FlowData {
	for index := range data.Flows {
		if data.Flows[index].ID == id {
			return &data.Flows[index]
		}
	}
	if id == "" && fallbackIndex >= 0 && fallbackIndex < len(data.Flows) {
		return &data.Flows[fallbackIndex]
	}
	return nil
}

func addLegacyComponentPresentationText(
	add presentationInventoryAdder,
	data *ReportData,
) error {
	for index := range data.Components {
		index := index
		id := data.Components[index].ID
		owner := "components/" + id
		if err := add(owner+"/name", data.Components[index].Name, func(target *ReportData, text string) bool {
			for componentIndex := range target.Components {
				if target.Components[componentIndex].ID == id {
					target.Components[componentIndex].Name = text
					return true
				}
			}
			return false
		}); err != nil {
			return err
		}
		if err := add(owner+"/model_purpose", data.Components[index].ModelPurpose, func(target *ReportData, text string) bool {
			for componentIndex := range target.Components {
				if target.Components[componentIndex].ID == id {
					target.Components[componentIndex].ModelPurpose = text
					return true
				}
			}
			return false
		}); err != nil {
			return err
		}
		for groupIndex := range data.Components[index].AnchorGroups {
			groupIndex := groupIndex
			group := data.Components[index].AnchorGroups[groupIndex]
			groupID := group.ID
			groupOwner := owner + "/anchor_groups/" + groupID
			if err := add(groupOwner+"/grounding", group.Grounding, func(target *ReportData, text string) bool {
				group := findAnchorGroup(target, id, groupID)
				if group == nil {
					return false
				}
				group.Grounding = text
				return true
			}); err != nil {
				return err
			}
			for noteIndex := range group.ModelNotes {
				noteIndex := noteIndex
				if err := add(
					groupOwner+"/model_notes/"+strconv.Itoa(noteIndex),
					group.ModelNotes[noteIndex],
					func(target *ReportData, text string) bool {
						group := findAnchorGroup(target, id, groupID)
						if group == nil || noteIndex >= len(group.ModelNotes) {
							return false
						}
						group.ModelNotes[noteIndex] = text
						return true
					},
				); err != nil {
					return err
				}
			}
			for signalIndex := range group.LocalContext {
				signalIndex := signalIndex
				if err := add(
					groupOwner+"/local_context/"+strconv.Itoa(signalIndex)+"/reason",
					group.LocalContext[signalIndex].Reason,
					func(target *ReportData, text string) bool {
						group := findAnchorGroup(target, id, groupID)
						if group == nil || signalIndex >= len(group.LocalContext) {
							return false
						}
						group.LocalContext[signalIndex].Reason = text
						return true
					},
				); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func findAnchorGroup(data *ReportData, componentID, groupID string) *AnchorGroup {
	for componentIndex := range data.Components {
		component := &data.Components[componentIndex]
		if component.ID != componentID {
			continue
		}
		for groupIndex := range component.AnchorGroups {
			if component.AnchorGroups[groupIndex].ID == groupID {
				return &component.AnchorGroups[groupIndex]
			}
		}
	}
	return nil
}

func appendArchitectureComponentProtectedValues(
	builder *objectProtectedValueBuilder,
	canvas *ArchitectureCanvas,
	componentIDs []componentmap.ComponentID,
) {
	if builder == nil || canvas == nil {
		return
	}
	components := componentMapByID(canvas.Components)
	for _, componentID := range componentIDs {
		builder.add(localization.ProtectedIdentifier, string(componentID))
		if component, ok := components[componentID]; ok {
			builder.values = append(
				builder.values,
				presentationComponentProtectedValues(component)...,
			)
		}
	}
}

func architectureSuggestionProtectedValues(
	canvas *ArchitectureCanvas,
	suggestion ArchitectureSuggestion,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		suggestion.ID,
		suggestion.CurrentGrounding,
	)
	for _, value := range suggestion.EvidenceReferences {
		builder.add(localization.ProtectedIdentifier, value)
	}
	for _, value := range suggestion.RelevantAnchorIDs {
		builder.add(localization.ProtectedIdentifier, value)
	}
	appendArchitectureComponentProtectedValues(
		&builder,
		canvas,
		suggestion.RelevantComponentIDs,
	)
	if suggestion.StartLocation != nil {
		builder.add(localization.ProtectedPath, suggestion.StartLocation.Path)
	}
	return builder.values
}

func appendArchitectureFlowStepProtectedValues(
	builder *objectProtectedValueBuilder,
	step ArchitectureFlowStep,
) {
	if builder == nil {
		return
	}
	builder.add(
		localization.ProtectedIdentifier,
		step.ID,
		string(step.Kind),
		step.BranchID,
		string(step.ComponentID),
	)
	builder.add(localization.ProtectedSymbol, step.QualifiedName)
	if step.Location != nil {
		builder.add(localization.ProtectedPath, step.Location.Path)
	}
	if step.Binding == nil {
		return
	}
	builder.add(
		localization.ProtectedIdentifier,
		string(step.Binding.FlowID),
		step.Binding.AnchorID,
		string(step.Binding.MemberID.Kind),
		string(step.Binding.Certainty),
	)
	builder.add(
		protectedMemberKind(step.Binding.MemberID.Kind),
		step.Binding.MemberID.Value,
	)
	if step.Binding.Location != nil {
		builder.add(localization.ProtectedPath, step.Binding.Location.Path)
	}
	for _, provenance := range step.Binding.Provenance {
		builder.add(
			localization.ProtectedIdentifier,
			provenance.Provider,
			provenance.Version,
			provenance.Operation,
		)
	}
	for _, scenario := range step.Binding.Scenarios {
		builder.add(
			localization.ProtectedIdentifier,
			scenario.ID,
			scenario.Build.GOOS,
			scenario.Build.GOARCH,
		)
		for _, tag := range scenario.Build.BuildTags {
			builder.add(localization.ProtectedIdentifier, tag)
		}
	}
}

func architectureFlowProtectedValues(
	canvas *ArchitectureCanvas,
	flow ArchitectureFlow,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		string(flow.ID),
		string(flow.Archetype),
		flow.Command,
		flow.Status,
		flow.EvidenceBasis,
		flow.StartSurfaceID,
		flow.SeedSurfaceID,
	)
	for _, value := range flow.TransitionIDs {
		builder.add(localization.ProtectedIdentifier, value)
	}
	for _, value := range flow.TraceEvidenceSurfaceIDs {
		builder.add(localization.ProtectedIdentifier, value)
	}
	for _, value := range flow.RelatedComponentSurfaceIDs {
		builder.add(localization.ProtectedIdentifier, value)
	}
	appendArchitectureComponentProtectedValues(
		&builder,
		canvas,
		flow.ParticipatingComponentIDs,
	)
	for _, step := range flow.Steps {
		appendArchitectureFlowStepProtectedValues(&builder, step)
	}
	for _, branch := range flow.Branches {
		builder.add(
			localization.ProtectedIdentifier,
			branch.ID,
			branch.Kind,
			branch.RootAnchorID,
		)
		for _, collection := range [][]string{
			branch.RootAnchorIDs,
			branch.AnchorIDs,
		} {
			for _, value := range collection {
				builder.add(localization.ProtectedIdentifier, value)
			}
		}
	}
	for _, slot := range flow.Slots {
		builder.add(
			localization.ProtectedIdentifier,
			string(slot.Kind),
			string(slot.Status),
			string(slot.ApplicabilityReason),
		)
		for _, evidenceID := range slot.EvidenceIDs {
			builder.add(localization.ProtectedIdentifier, evidenceID)
		}
	}
	return builder.values
}

func architectureFrontierProtectedValues(
	canvas *ArchitectureCanvas,
	frontier ArchitectureFrontier,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		frontier.ID,
		string(frontier.FlowID),
		frontier.Kind,
		frontier.AnchorID,
		frontier.TransitionID,
		string(frontier.Slot),
	)
	if frontier.Evidence != nil {
		builder.add(localization.ProtectedPath, frontier.Evidence.Path)
	}
	if canvas != nil {
		for _, flow := range canvas.Flows {
			if flow.ID == frontier.FlowID {
				builder.values = append(
					builder.values,
					architectureFlowProtectedValues(canvas, flow)...,
				)
				break
			}
		}
	}
	return builder.values
}

func architectureDiagnosticProtectedValues(
	canvas *ArchitectureCanvas,
	diagnostic ArchitectureDiagnostic,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		diagnostic.ID,
		diagnostic.Source,
		diagnostic.Severity,
		diagnostic.Code,
		string(diagnostic.FlowID),
	)
	if diagnostic.Member != nil {
		builder.add(
			protectedMemberKind(diagnostic.Member.Kind),
			diagnostic.Member.Value,
		)
	}
	if canvas != nil {
		for _, flow := range canvas.Flows {
			if flow.ID == diagnostic.FlowID {
				builder.values = append(
					builder.values,
					architectureFlowProtectedValues(canvas, flow)...,
				)
				break
			}
		}
	}
	return builder.values
}

func architectureSurfaceProtectedValues(
	canvas *ArchitectureCanvas,
	surface ArchitectureSurface,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		surface.ID,
		surface.Source,
		surface.Kind,
		surface.Category,
		surface.OwningExecutable,
		string(surface.OwningComponentID),
		string(surface.RelatedTraceID),
		surface.Status,
		surface.Certainty,
		surface.Resolution,
		surface.SurfaceRole,
		surface.TraceReadiness,
	)
	for _, componentID := range surface.ParticipatingComponentIDs {
		builder.add(localization.ProtectedIdentifier, string(componentID))
	}
	componentIDs := append(
		[]componentmap.ComponentID{surface.OwningComponentID},
		surface.ParticipatingComponentIDs...,
	)
	appendArchitectureComponentProtectedValues(&builder, canvas, componentIDs)
	for _, location := range surface.Evidence {
		builder.add(localization.ProtectedPath, location.Path)
	}
	return builder.values
}

// presentationComponentProtectedValues intentionally differs from the
// historical Architecture-only localization contract. A member display name
// may be prose; its exact technical identity is instead carried by MemberID
// and local facts. Keeping the display name out of this protection set lets
// the complete terminal inventory translate prose without changing those
// identities.
func presentationComponentProtectedValues(
	component ArchitectureComponent,
) []localization.ProtectedValue {
	values := []localization.ProtectedValue{
		{Kind: localization.ProtectedIdentifier, Value: string(component.ID)},
		{Kind: localization.ProtectedIdentifier, Value: string(component.SubsystemID)},
	}
	for _, flowID := range component.ParticipatingFlowIDs {
		values = append(values, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: string(flowID),
		})
	}
	for _, surfaceID := range append(
		append([]string(nil), component.OwnedSurfaceIDs...),
		component.ParticipatingSurfaceIDs...,
	) {
		values = append(values, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: surfaceID,
		})
	}
	for _, investigationID := range component.SuggestedInvestigationIDs {
		values = append(values, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: investigationID,
		})
	}
	for _, anchorID := range component.AnchorIDs {
		values = append(values, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: anchorID,
		})
	}
	for _, sourceID := range component.SourceIDs {
		values = append(values, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: string(sourceID),
		})
	}
	for _, member := range component.Members {
		values = append(values, architectureMemberProtectedValues(member)...)
	}
	return values
}

func presentationSubsystemProtectedValues(
	subsystem ArchitectureSubsystem,
	components map[componentmap.ComponentID]ArchitectureComponent,
) []localization.ProtectedValue {
	values := []localization.ProtectedValue{{
		Kind: localization.ProtectedIdentifier, Value: string(subsystem.ID),
	}}
	for _, sourceID := range subsystem.SourceIDs {
		values = append(values, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: string(sourceID),
		})
	}
	for _, componentID := range subsystem.ComponentIDs {
		values = append(values, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: string(componentID),
		})
		if component, ok := components[componentID]; ok {
			values = append(values, presentationComponentProtectedValues(component)...)
		}
	}
	return values
}

func architectureBehaviorAnchorProtectedValues(
	canvas *ArchitectureCanvas,
	anchor componentmap.BehaviorAnchor,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		anchor.ID,
		string(anchor.Kind),
		anchor.Scenario.ID,
		anchor.Scenario.Build.GOOS,
		anchor.Scenario.Build.GOARCH,
		anchor.Producer.Provider,
		anchor.Producer.Version,
		anchor.Producer.Operation,
		string(anchor.Certainty),
	)
	builder.add(localization.ProtectedPath, anchor.Location.Path)
	for _, tag := range anchor.Scenario.Build.BuildTags {
		builder.add(localization.ProtectedIdentifier, tag)
	}
	for _, memberID := range anchor.MemberIDs {
		builder.add(protectedMemberKind(memberID.Kind), memberID.Value)
		if member, ok := findArchitectureMember(canvas, memberID); ok {
			builder.values = append(
				builder.values,
				architectureMemberProtectedValues(member)...,
			)
		}
	}
	return builder.values
}

func architectureMemberNameIsProse(member componentmap.Candidate) bool {
	// Package and file names are exact technical identities. Symbol and
	// entrypoint candidates may carry an editorial anchor label while their
	// exact declaration remains in the candidate facts. Flow names likewise
	// come from a model-authored direction name or saved proof goal.
	switch member.ID.Kind {
	case componentmap.MemberSymbol, componentmap.MemberEntrypoint,
		componentmap.MemberFlow:
		return true
	default:
		return false
	}
}

func architectureMemberProtectedValues(
	member componentmap.Candidate,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(protectedMemberKind(member.ID.Kind), member.ID.Value)
	if member.ParentID != nil {
		builder.add(
			protectedMemberKind(member.ParentID.Kind),
			member.ParentID.Value,
		)
	}
	for _, participation := range member.Participations {
		builder.add(localization.ProtectedIdentifier, string(participation.FlowID))
		builder.values = append(
			builder.values,
			factProtectedValues(participation.Evidence)...,
		)
	}
	for _, fact := range member.Facts {
		builder.values = append(builder.values, factProtectedValues(fact)...)
	}
	return builder.values
}

func findArchitectureMember(
	canvas *ArchitectureCanvas,
	id componentmap.MemberID,
) (componentmap.Candidate, bool) {
	if canvas == nil {
		return componentmap.Candidate{}, false
	}
	for _, component := range canvas.Components {
		for _, member := range component.Members {
			if member.ID == id {
				return member, true
			}
		}
	}
	return componentmap.Candidate{}, false
}

func addArchitecturePresentationText(
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	canvas := data.ArchitectureCanvas
	if canvas == nil {
		return nil
	}
	for index := range canvas.BehaviorAnchors {
		anchor := canvas.BehaviorAnchors[index]
		id := anchor.ID
		if err := addObject(
			"architecture/behavior_anchors/"+id+"/label",
			anchor.Label,
			architectureBehaviorAnchorProtectedValues(canvas, anchor),
			func(target *ReportData, text string) bool {
				if target.ArchitectureCanvas == nil {
					return false
				}
				for targetIndex := range target.ArchitectureCanvas.BehaviorAnchors {
					targetAnchor := &target.ArchitectureCanvas.BehaviorAnchors[targetIndex]
					if targetAnchor.ID == id {
						targetAnchor.Label = text
						return true
					}
				}
				return false
			},
		); err != nil {
			return err
		}
	}
	for componentIndex := range canvas.Components {
		component := canvas.Components[componentIndex]
		componentID := string(component.ID)
		for memberIndex := range component.Members {
			member := component.Members[memberIndex]
			if !architectureMemberNameIsProse(member) {
				continue
			}
			memberID := member.ID
			owner := "architecture/components/" + componentID + "/members/" +
				presentationOwnerDigest(string(memberID.Kind), memberID.Value) + "/name"
			if err := addObject(
				owner,
				member.Name,
				architectureMemberProtectedValues(member),
				func(target *ReportData, text string) bool {
					if target.ArchitectureCanvas == nil {
						return false
					}
					for targetComponentIndex := range target.ArchitectureCanvas.Components {
						targetComponent := &target.ArchitectureCanvas.Components[targetComponentIndex]
						if string(targetComponent.ID) != componentID {
							continue
						}
						for targetMemberIndex := range targetComponent.Members {
							targetMember := &targetComponent.Members[targetMemberIndex]
							if targetMember.ID == memberID {
								targetMember.Name = text
								return true
							}
						}
						return false
					}
					return false
				},
			); err != nil {
				return err
			}
		}
	}
	for index := range canvas.Suggestions {
		index := index
		id := canvas.Suggestions[index].ID
		owner := "architecture/suggestions/" + id
		add := addObject.with(architectureSuggestionProtectedValues(
			canvas,
			canvas.Suggestions[index],
		))
		fields := []struct {
			name string
			text string
			set  func(*ArchitectureSuggestion, string)
		}{
			{"title", canvas.Suggestions[index].Title, func(item *ArchitectureSuggestion, text string) { item.Title = text }},
			{"reason", canvas.Suggestions[index].Reason, func(item *ArchitectureSuggestion, text string) { item.Reason = text }},
			{"unavailable_reason", canvas.Suggestions[index].UnavailableReason, func(item *ArchitectureSuggestion, text string) { item.UnavailableReason = text }},
			{"trace_unavailable_reason", canvas.Suggestions[index].TraceUnavailableReason, func(item *ArchitectureSuggestion, text string) { item.TraceUnavailableReason = text }},
		}
		for _, field := range fields {
			field := field
			if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				suggestion := findArchitectureSuggestion(target, id)
				if suggestion == nil {
					return false
				}
				field.set(suggestion, text)
				return true
			}); err != nil {
				return err
			}
		}
	}
	for index := range canvas.Flows {
		index := index
		id := string(canvas.Flows[index].ID)
		owner := "architecture/flows/" + id
		add := addObject.with(architectureFlowProtectedValues(
			canvas,
			canvas.Flows[index],
		))
		fields := []struct {
			name string
			text string
			set  func(*ArchitectureFlow, string)
		}{
			{"name", canvas.Flows[index].Name, func(item *ArchitectureFlow, text string) { item.Name = text }},
			{"trigger", canvas.Flows[index].Trigger, func(item *ArchitectureFlow, text string) { item.Trigger = text }},
			{"scope", canvas.Flows[index].Scope, func(item *ArchitectureFlow, text string) { item.Scope = text }},
			{"mental_model", canvas.Flows[index].MentalModel, func(item *ArchitectureFlow, text string) { item.MentalModel = text }},
			{"goal", canvas.Flows[index].Goal, func(item *ArchitectureFlow, text string) { item.Goal = text }},
			{"why_inspect", canvas.Flows[index].WhyInspect, func(item *ArchitectureFlow, text string) { item.WhyInspect = text }},
			{"frontier_summary", canvas.Flows[index].FrontierSummary, func(item *ArchitectureFlow, text string) { item.FrontierSummary = text }},
			{"current_frontier", canvas.Flows[index].CurrentFrontier, func(item *ArchitectureFlow, text string) { item.CurrentFrontier = text }},
		}
		for _, field := range fields {
			field := field
			if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				flow := findArchitectureFlow(target, id)
				if flow == nil {
					return false
				}
				field.set(flow, text)
				return true
			}); err != nil {
				return err
			}
		}
		for stepIndex := range canvas.Flows[index].Steps {
			stepIndex := stepIndex
			stepID := canvas.Flows[index].Steps[stepIndex].ID
			if stepID == "" {
				stepID = presentationOwnerDigest(
					canvas.Flows[index].Steps[stepIndex].QualifiedName,
					architectureFlowStepLocationIdentity(canvas.Flows[index].Steps[stepIndex]),
				)
			}
			if err := add(
				owner+"/steps/"+stepID+"/label",
				canvas.Flows[index].Steps[stepIndex].Label,
				func(target *ReportData, text string) bool {
					flow := findArchitectureFlow(target, id)
					if flow == nil || stepIndex >= len(flow.Steps) {
						return false
					}
					currentID := flow.Steps[stepIndex].ID
					if currentID == "" {
						currentID = presentationOwnerDigest(
							flow.Steps[stepIndex].QualifiedName,
							architectureFlowStepLocationIdentity(flow.Steps[stepIndex]),
						)
					}
					if currentID != stepID {
						return false
					}
					flow.Steps[stepIndex].Label = text
					return true
				},
			); err != nil {
				return err
			}
		}
		for slotIndex := range canvas.Flows[index].Slots {
			slotIndex := slotIndex
			slotKind := canvas.Flows[index].Slots[slotIndex].Kind
			slotOwner := owner + "/slots/" + string(slotKind)
			for _, field := range []struct {
				name string
				text string
				set  func(*flowproof.Slot, string)
			}{
				{"summary", canvas.Flows[index].Slots[slotIndex].Summary, func(item *flowproof.Slot, text string) { item.Summary = text }},
				{"missing", canvas.Flows[index].Slots[slotIndex].Missing, func(item *flowproof.Slot, text string) { item.Missing = text }},
			} {
				field := field
				if err := add(slotOwner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
					flow := findArchitectureFlow(target, id)
					if flow == nil || slotIndex >= len(flow.Slots) ||
						flow.Slots[slotIndex].Kind != slotKind {
						return false
					}
					field.set(&flow.Slots[slotIndex], text)
					return true
				}); err != nil {
					return err
				}
			}
		}
	}
	for index := range canvas.Frontiers {
		index := index
		id := canvas.Frontiers[index].ID
		if id == "" {
			id = presentationOwnerDigest(
				string(canvas.Frontiers[index].FlowID),
				canvas.Frontiers[index].Kind,
				canvas.Frontiers[index].AnchorID,
				canvas.Frontiers[index].TransitionID,
			)
		}
		if err := addObject(
			"architecture/frontiers/"+id+"/reason",
			canvas.Frontiers[index].Reason,
			architectureFrontierProtectedValues(canvas, canvas.Frontiers[index]),
			func(target *ReportData, text string) bool {
				if target.ArchitectureCanvas == nil ||
					index >= len(target.ArchitectureCanvas.Frontiers) {
					return false
				}
				target.ArchitectureCanvas.Frontiers[index].Reason = text
				return true
			},
		); err != nil {
			return err
		}
	}
	for index := range canvas.Diagnostics {
		index := index
		id := canvas.Diagnostics[index].ID
		if id == "" {
			id = presentationOwnerDigest(
				canvas.Diagnostics[index].Source,
				canvas.Diagnostics[index].Code,
				string(canvas.Diagnostics[index].FlowID),
			)
		}
		if err := addObject(
			"architecture/diagnostics/"+id+"/message",
			canvas.Diagnostics[index].Message,
			architectureDiagnosticProtectedValues(canvas, canvas.Diagnostics[index]),
			func(target *ReportData, text string) bool {
				if target.ArchitectureCanvas == nil ||
					index >= len(target.ArchitectureCanvas.Diagnostics) {
					return false
				}
				target.ArchitectureCanvas.Diagnostics[index].Message = text
				return true
			},
		); err != nil {
			return err
		}
	}
	for index := range canvas.Surfaces {
		index := index
		id := canvas.Surfaces[index].ID
		owner := "architecture/surfaces/" + id
		add := addObject.with(architectureSurfaceProtectedValues(
			canvas,
			canvas.Surfaces[index],
		))
		for _, field := range []struct {
			name string
			text string
			set  func(*ArchitectureSurface, string)
		}{
			{"name", canvas.Surfaces[index].Name, func(item *ArchitectureSurface, text string) { item.Name = text }},
			{"trace_unavailable_reason", canvas.Surfaces[index].TraceUnavailableReason, func(item *ArchitectureSurface, text string) { item.TraceUnavailableReason = text }},
			{"trace_readiness_reason", canvas.Surfaces[index].TraceReadinessReason, func(item *ArchitectureSurface, text string) { item.TraceReadinessReason = text }},
		} {
			field := field
			if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				surface := findArchitectureSurface(target, id)
				if surface == nil {
					return false
				}
				field.set(surface, text)
				return true
			}); err != nil {
				return err
			}
		}
	}
	return addArchitectureDebugPresentationText(addObject, data)
}

type architectureProvenanceResolver func(*ReportData) *evidence.Provenance
type architectureScenarioResolver func(*ReportData) *componentmap.ScenarioContext

func addArchitectureDebugPresentationText(
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	canvas := data.ArchitectureCanvas
	if canvas == nil {
		return nil
	}
	addProvenance := func(
		base string,
		index int,
		provenance evidence.Provenance,
		resolve architectureProvenanceResolver,
	) error {
		if provenance.Detail == "" ||
			architectureProvenanceProductMessage(provenance) != "" ||
			architectureProvenanceDetailIsOpaque(provenance) {
			return nil
		}
		address := base + "/provenance/" + strconv.Itoa(index) + "/detail"
		return addObject(
			address,
			provenance.Detail,
			architectureProvenanceProtectedValues(provenance),
			func(target *ReportData, text string) bool {
				current := resolve(target)
				if current == nil ||
					current.Provider != provenance.Provider ||
					current.Version != provenance.Version ||
					current.Operation != provenance.Operation ||
					architecturePresentationLocationIdentity(current.Location) !=
						architecturePresentationLocationIdentity(provenance.Location) {
					return false
				}
				if target.architectureDebugPresentation == nil {
					target.architectureDebugPresentation = make(map[string]string)
				}
				target.architectureDebugPresentation[address] = text
				return true
			},
		)
	}
	addScenario := func(
		base string,
		index int,
		scenario componentmap.ScenarioContext,
		resolve architectureScenarioResolver,
	) error {
		if scenario.Name == "" || architectureScenarioProductMessage(scenario) != "" {
			return nil
		}
		address := base + "/scenarios/" + strconv.Itoa(index) + "/name"
		return addObject(
			address,
			scenario.Name,
			architectureScenarioProtectedValues(scenario),
			func(target *ReportData, text string) bool {
				current := resolve(target)
				if current == nil || current.ID != scenario.ID ||
					current.Build.GOOS != scenario.Build.GOOS ||
					current.Build.GOARCH != scenario.Build.GOARCH {
					return false
				}
				if target.architectureDebugPresentation == nil {
					target.architectureDebugPresentation = make(map[string]string)
				}
				target.architectureDebugPresentation[address] = text
				return true
			},
		)
	}

	for anchorIndex := range canvas.BehaviorAnchors {
		anchorIndex := anchorIndex
		anchor := canvas.BehaviorAnchors[anchorIndex]
		base := "architecture/behavior_anchors/" + anchor.ID
		if err := addProvenance(
			base+"/producer",
			0,
			anchor.Producer,
			func(target *ReportData) *evidence.Provenance {
				if target.ArchitectureCanvas == nil ||
					anchorIndex >= len(target.ArchitectureCanvas.BehaviorAnchors) ||
					target.ArchitectureCanvas.BehaviorAnchors[anchorIndex].ID != anchor.ID {
					return nil
				}
				return &target.ArchitectureCanvas.BehaviorAnchors[anchorIndex].Producer
			},
		); err != nil {
			return err
		}
		if err := addScenario(
			base,
			0,
			anchor.Scenario,
			func(target *ReportData) *componentmap.ScenarioContext {
				if target.ArchitectureCanvas == nil ||
					anchorIndex >= len(target.ArchitectureCanvas.BehaviorAnchors) ||
					target.ArchitectureCanvas.BehaviorAnchors[anchorIndex].ID != anchor.ID {
					return nil
				}
				return &target.ArchitectureCanvas.BehaviorAnchors[anchorIndex].Scenario
			},
		); err != nil {
			return err
		}
	}

	for componentIndex := range canvas.Components {
		componentIndex := componentIndex
		component := canvas.Components[componentIndex]
		for memberIndex := range component.Members {
			memberIndex := memberIndex
			member := component.Members[memberIndex]
			memberOwner := "architecture/components/" + string(component.ID) +
				"/members/" + string(member.ID.Kind) + "/" + member.ID.Value
			for factIndex := range member.Facts {
				factIndex := factIndex
				fact := member.Facts[factIndex]
				factOwner := memberOwner + "/facts/" + strconv.Itoa(factIndex)
				for provenanceIndex := range fact.Provenance {
					provenanceIndex := provenanceIndex
					provenance := fact.Provenance[provenanceIndex]
					if err := addProvenance(
						factOwner,
						provenanceIndex,
						provenance,
						func(target *ReportData) *evidence.Provenance {
							if target.ArchitectureCanvas == nil ||
								componentIndex >= len(target.ArchitectureCanvas.Components) ||
								target.ArchitectureCanvas.Components[componentIndex].ID != component.ID ||
								memberIndex >= len(target.ArchitectureCanvas.Components[componentIndex].Members) ||
								target.ArchitectureCanvas.Components[componentIndex].Members[memberIndex].ID != member.ID ||
								factIndex >= len(target.ArchitectureCanvas.Components[componentIndex].Members[memberIndex].Facts) ||
								provenanceIndex >= len(target.ArchitectureCanvas.Components[componentIndex].Members[memberIndex].Facts[factIndex].Provenance) {
								return nil
							}
							return &target.ArchitectureCanvas.Components[componentIndex].Members[memberIndex].Facts[factIndex].Provenance[provenanceIndex]
						},
					); err != nil {
						return err
					}
				}
			}
		}
	}

	for flowIndex := range canvas.Flows {
		flowIndex := flowIndex
		flow := canvas.Flows[flowIndex]
		flowOwner := "architecture/flows/" + string(flow.ID)
		for slotIndex := range flow.Slots {
			slotIndex := slotIndex
			slot := flow.Slots[slotIndex]
			slotOwner := flowOwner + "/slots/" + string(slot.Kind)
			for provenanceIndex := range slot.Provenance {
				provenanceIndex := provenanceIndex
				provenance := slot.Provenance[provenanceIndex]
				if err := addProvenance(
					slotOwner,
					provenanceIndex,
					provenance,
					func(target *ReportData) *evidence.Provenance {
						flow := findArchitectureFlow(target, string(flow.ID))
						if flow == nil || slotIndex >= len(flow.Slots) ||
							flow.Slots[slotIndex].Kind != slot.Kind ||
							provenanceIndex >= len(flow.Slots[slotIndex].Provenance) {
							return nil
						}
						return &flow.Slots[slotIndex].Provenance[provenanceIndex]
					},
				); err != nil {
					return err
				}
			}
		}
		for stepIndex := range flow.Steps {
			stepIndex := stepIndex
			step := flow.Steps[stepIndex]
			if step.Binding == nil {
				continue
			}
			stepID := step.ID
			if stepID == "" {
				stepID = strconv.Itoa(stepIndex)
			}
			bindingOwner := flowOwner + "/steps/" + stepID + "/binding"
			for provenanceIndex := range step.Binding.Provenance {
				provenanceIndex := provenanceIndex
				provenance := step.Binding.Provenance[provenanceIndex]
				if err := addProvenance(
					bindingOwner,
					provenanceIndex,
					provenance,
					func(target *ReportData) *evidence.Provenance {
						flow := findArchitectureFlow(target, string(flow.ID))
						if flow == nil || stepIndex >= len(flow.Steps) ||
							flow.Steps[stepIndex].Binding == nil ||
							provenanceIndex >= len(flow.Steps[stepIndex].Binding.Provenance) {
							return nil
						}
						return &flow.Steps[stepIndex].Binding.Provenance[provenanceIndex]
					},
				); err != nil {
					return err
				}
			}
			for scenarioIndex := range step.Binding.Scenarios {
				scenarioIndex := scenarioIndex
				scenario := step.Binding.Scenarios[scenarioIndex]
				if err := addScenario(
					bindingOwner,
					scenarioIndex,
					scenario,
					func(target *ReportData) *componentmap.ScenarioContext {
						flow := findArchitectureFlow(target, string(flow.ID))
						if flow == nil || stepIndex >= len(flow.Steps) ||
							flow.Steps[stepIndex].Binding == nil ||
							scenarioIndex >= len(flow.Steps[stepIndex].Binding.Scenarios) {
							return nil
						}
						return &flow.Steps[stepIndex].Binding.Scenarios[scenarioIndex]
					},
				); err != nil {
					return err
				}
			}
		}
	}

	addRelation := func(
		relation componentmap.LocalRelation,
		resolve func(*ReportData) *componentmap.LocalRelation,
	) error {
		id := relation.ID
		if id == "" {
			// A valid Architecture witness always has an exact relation ID.
			// Without one, the browser cannot address a separate presentation
			// value without inventing semantic identity from endpoints.
			return nil
		}
		base := "architecture/structural_relations/" + id
		for provenanceIndex := range relation.Provenance {
			provenanceIndex := provenanceIndex
			provenance := relation.Provenance[provenanceIndex]
			if err := addProvenance(
				base,
				provenanceIndex,
				provenance,
				func(target *ReportData) *evidence.Provenance {
					current := resolve(target)
					if current == nil || provenanceIndex >= len(current.Provenance) {
						return nil
					}
					return &current.Provenance[provenanceIndex]
				},
			); err != nil {
				return err
			}
		}
		for scenarioIndex := range relation.Scenarios {
			scenarioIndex := scenarioIndex
			scenario := relation.Scenarios[scenarioIndex]
			if err := addScenario(
				base,
				scenarioIndex,
				scenario,
				func(target *ReportData) *componentmap.ScenarioContext {
					current := resolve(target)
					if current == nil || scenarioIndex >= len(current.Scenarios) {
						return nil
					}
					return &current.Scenarios[scenarioIndex]
				},
			); err != nil {
				return err
			}
		}
		return nil
	}
	for relationIndex := range canvas.StructuralFacts {
		relationIndex := relationIndex
		relation := canvas.StructuralFacts[relationIndex]
		if err := addRelation(
			relation,
			func(target *ReportData) *componentmap.LocalRelation {
				if target.ArchitectureCanvas == nil ||
					relationIndex >= len(target.ArchitectureCanvas.StructuralFacts) ||
					target.ArchitectureCanvas.StructuralFacts[relationIndex].ID != relation.ID {
					return nil
				}
				return &target.ArchitectureCanvas.StructuralFacts[relationIndex]
			},
		); err != nil {
			return err
		}
	}
	for edgeIndex := range canvas.StructuralEdges {
		edgeIndex := edgeIndex
		edge := canvas.StructuralEdges[edgeIndex]
		if err := addRelation(
			edge.Witness,
			func(target *ReportData) *componentmap.LocalRelation {
				if target.ArchitectureCanvas == nil ||
					edgeIndex >= len(target.ArchitectureCanvas.StructuralEdges) ||
					target.ArchitectureCanvas.StructuralEdges[edgeIndex].ID != edge.ID {
					return nil
				}
				return &target.ArchitectureCanvas.StructuralEdges[edgeIndex].Witness
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func architecturePresentationLocationIdentity(location *evidence.Location) string {
	if location == nil {
		return ""
	}
	return location.Path + ":" + strconv.Itoa(location.Line) + ":" +
		strconv.Itoa(location.Column)
}

func architectureProvenanceProtectedValues(
	provenance evidence.Provenance,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		provenance.Provider,
		provenance.Version,
		provenance.Operation,
	)
	if provenance.Location != nil {
		builder.add(localization.ProtectedPath, provenance.Location.Path)
	}
	return builder.values
}

func architectureScenarioProtectedValues(
	scenario componentmap.ScenarioContext,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		scenario.ID,
		scenario.Build.GOOS,
		scenario.Build.GOARCH,
	)
	for _, tag := range scenario.Build.BuildTags {
		builder.add(localization.ProtectedIdentifier, tag)
	}
	return builder.values
}

func architectureFlowStepLocationIdentity(step ArchitectureFlowStep) string {
	if step.Location == nil {
		return ""
	}
	return step.Location.Path + ":" +
		strconv.Itoa(step.Location.Line) + ":" +
		strconv.Itoa(step.Location.Column)
}

func findArchitectureSuggestion(data *ReportData, id string) *ArchitectureSuggestion {
	if data.ArchitectureCanvas == nil {
		return nil
	}
	for index := range data.ArchitectureCanvas.Suggestions {
		if data.ArchitectureCanvas.Suggestions[index].ID == id {
			return &data.ArchitectureCanvas.Suggestions[index]
		}
	}
	return nil
}

func findArchitectureFlow(data *ReportData, id string) *ArchitectureFlow {
	if data.ArchitectureCanvas == nil {
		return nil
	}
	for index := range data.ArchitectureCanvas.Flows {
		if string(data.ArchitectureCanvas.Flows[index].ID) == id {
			return &data.ArchitectureCanvas.Flows[index]
		}
	}
	return nil
}

func findArchitectureSurface(data *ReportData, id string) *ArchitectureSurface {
	if data.ArchitectureCanvas == nil {
		return nil
	}
	for index := range data.ArchitectureCanvas.Surfaces {
		if data.ArchitectureCanvas.Surfaces[index].ID == id {
			return &data.ArchitectureCanvas.Surfaces[index]
		}
	}
	return nil
}

func addSemanticPresentationText(
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	for artifactIndex := range data.SemanticArtifacts {
		artifactIndex := artifactIndex
		add := addObject.with(semanticArtifactProtectedValues(
			data.SemanticArtifacts[artifactIndex],
		))
		id := data.SemanticArtifacts[artifactIndex].ID
		owner := "semantic_artifacts/" + id
		fields := []struct {
			name string
			text string
			set  func(index int, target *ReportData, text string)
		}{
			{"title", data.SemanticArtifacts[artifactIndex].Title, func(index int, target *ReportData, text string) { target.SemanticArtifacts[index].Title = text }},
			{"summary", data.SemanticArtifacts[artifactIndex].Summary, func(index int, target *ReportData, text string) { target.SemanticArtifacts[index].Summary = text }},
			{"question", data.SemanticArtifacts[artifactIndex].Question, func(index int, target *ReportData, text string) { target.SemanticArtifacts[index].Question = text }},
		}
		for _, field := range fields {
			field := field
			if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				index := findSemanticArtifactIndex(target, id)
				if index < 0 {
					return false
				}
				field.set(index, target, text)
				return true
			}); err != nil {
				return err
			}
		}
		for statementIndex := range data.SemanticArtifacts[artifactIndex].Statements {
			statementIndex := statementIndex
			statementID := data.SemanticArtifacts[artifactIndex].Statements[statementIndex].ID
			if err := add(
				owner+"/statements/"+statementID+"/text",
				data.SemanticArtifacts[artifactIndex].Statements[statementIndex].Text,
				func(target *ReportData, text string) bool {
					index := findSemanticArtifactIndex(target, id)
					if index < 0 {
						return false
					}
					for currentIndex := range target.SemanticArtifacts[index].Statements {
						if target.SemanticArtifacts[index].Statements[currentIndex].ID == statementID {
							target.SemanticArtifacts[index].Statements[currentIndex].Text = text
							return true
						}
					}
					return false
				},
			); err != nil {
				return err
			}
		}
		for stepIndex := range data.SemanticArtifacts[artifactIndex].Steps {
			stepIndex := stepIndex
			stepID := data.SemanticArtifacts[artifactIndex].Steps[stepIndex].ID
			for _, field := range []struct {
				name string
				text string
				set  func(*ReportData, int, int, string)
			}{
				{"title", data.SemanticArtifacts[artifactIndex].Steps[stepIndex].Title, func(target *ReportData, artifact, step int, text string) {
					target.SemanticArtifacts[artifact].Steps[step].Title = text
				}},
				{"explanation", data.SemanticArtifacts[artifactIndex].Steps[stepIndex].Explanation, func(target *ReportData, artifact, step int, text string) {
					target.SemanticArtifacts[artifact].Steps[step].Explanation = text
				}},
			} {
				field := field
				if err := add(owner+"/steps/"+stepID+"/"+field.name, field.text, func(target *ReportData, text string) bool {
					artifact := findSemanticArtifactIndex(target, id)
					if artifact < 0 {
						return false
					}
					for currentIndex := range target.SemanticArtifacts[artifact].Steps {
						if target.SemanticArtifacts[artifact].Steps[currentIndex].ID == stepID {
							field.set(target, artifact, currentIndex, text)
							return true
						}
					}
					return false
				}); err != nil {
					return err
				}
			}
		}
		for _, collection := range []struct {
			name   string
			values []string
			set    func(*ReportData, int) *[]string
		}{
			{"unknowns", data.SemanticArtifacts[artifactIndex].Unknowns, func(target *ReportData, index int) *[]string { return &target.SemanticArtifacts[index].Unknowns }},
		} {
			collection := collection
			for valueIndex := range collection.values {
				valueIndex := valueIndex
				if err := add(
					owner+"/"+collection.name+"/"+strconv.Itoa(valueIndex),
					collection.values[valueIndex],
					func(target *ReportData, text string) bool {
						artifact := findSemanticArtifactIndex(target, id)
						if artifact < 0 {
							return false
						}
						values := collection.set(target, artifact)
						if valueIndex >= len(*values) {
							return false
						}
						(*values)[valueIndex] = text
						return true
					},
				); err != nil {
					return err
				}
			}
		}
	}
	for index := range data.UserTopics {
		index := index
		add := addObject.with(userTopicProtectedValues(data.UserTopics[index]))
		id := data.UserTopics[index].CandidateID
		owner := "user_topics/" + id
		for _, field := range []struct {
			name string
			text string
			set  func(*UserTopic, string)
		}{
			{"title", data.UserTopics[index].Title, func(item *UserTopic, text string) { item.Title = text }},
			{"question", data.UserTopics[index].Question, func(item *UserTopic, text string) { item.Question = text }},
			{"uncertainty", data.UserTopics[index].Uncertainty, func(item *UserTopic, text string) { item.Uncertainty = text }},
		} {
			field := field
			if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				for topicIndex := range target.UserTopics {
					if target.UserTopics[topicIndex].CandidateID == id {
						field.set(&target.UserTopics[topicIndex], text)
						return true
					}
				}
				return false
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func findSemanticArtifactIndex(data *ReportData, id string) int {
	for index := range data.SemanticArtifacts {
		if data.SemanticArtifacts[index].ID == id {
			return index
		}
	}
	return -1
}

func addOnboardingPresentationText(
	add presentationInventoryAdder,
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	if data.RepositoryThesis != nil {
		thesisAdd := addObject.with(repositoryThesisProtectedValues(
			*data.RepositoryThesis,
		))
		if err := thesisAdd(
			"onboarding/repository_thesis/purpose",
			data.RepositoryThesis.Purpose,
			func(target *ReportData, text string) bool {
				if target.RepositoryThesis == nil {
					return false
				}
				target.RepositoryThesis.Purpose = text
				return true
			},
		); err != nil {
			return err
		}
		for index := range data.RepositoryThesis.SystemStory {
			index := index
			if err := thesisAdd(
				"onboarding/repository_thesis/system_story/"+strconv.Itoa(index),
				data.RepositoryThesis.SystemStory[index],
				func(target *ReportData, text string) bool {
					if target.RepositoryThesis == nil ||
						index >= len(target.RepositoryThesis.SystemStory) {
						return false
					}
					target.RepositoryThesis.SystemStory[index] = text
					return true
				},
			); err != nil {
				return err
			}
		}
		for index := range data.RepositoryThesis.Areas {
			if err := addThesisAreaPresentationText(
				thesisAdd,
				"onboarding/repository_thesis/areas",
				data.RepositoryThesis.Areas[index],
				index,
				func(target *ReportData) []RepositoryThesisArea {
					if target.RepositoryThesis == nil {
						return nil
					}
					return target.RepositoryThesis.Areas
				},
			); err != nil {
				return err
			}
		}
	}
	if data.RepositoryGuide != nil {
		guideAdd := addObject.with(repositoryGuideProtectedValues(
			*data.RepositoryGuide,
		))
		if err := guideAdd(
			"onboarding/repository_guide/purpose",
			data.RepositoryGuide.Purpose,
			func(target *ReportData, text string) bool {
				if target.RepositoryGuide == nil {
					return false
				}
				target.RepositoryGuide.Purpose = text
				return true
			},
		); err != nil {
			return err
		}
		for index := range data.RepositoryGuide.SystemStory {
			index := index
			if err := guideAdd(
				"onboarding/repository_guide/system_story/"+strconv.Itoa(index),
				data.RepositoryGuide.SystemStory[index],
				func(target *ReportData, text string) bool {
					if target.RepositoryGuide == nil ||
						index >= len(target.RepositoryGuide.SystemStory) {
						return false
					}
					target.RepositoryGuide.SystemStory[index] = text
					return true
				},
			); err != nil {
				return err
			}
		}
		for index := range data.RepositoryGuide.Areas {
			if err := addThesisAreaPresentationText(
				guideAdd,
				"onboarding/repository_guide/areas",
				data.RepositoryGuide.Areas[index],
				index,
				func(target *ReportData) []RepositoryThesisArea {
					if target.RepositoryGuide == nil {
						return nil
					}
					return target.RepositoryGuide.Areas
				},
			); err != nil {
				return err
			}
		}
		for index := range data.RepositoryGuide.ReadNext {
			index := index
			targetValue := data.RepositoryGuide.ReadNext[index]
			owner := presentationAddress(
				"onboarding/repository_guide/read_next",
				targetValue.Path,
				targetValue.Symbol,
				strconv.Itoa(targetValue.Line),
				strconv.Itoa(targetValue.StepIndex),
			)
			if err := guideAdd(owner+"/label", targetValue.Label, func(target *ReportData, text string) bool {
				if target.RepositoryGuide == nil ||
					index >= len(target.RepositoryGuide.ReadNext) {
					return false
				}
				target.RepositoryGuide.ReadNext[index].Label = text
				return true
			}); err != nil {
				return err
			}
		}
	}
	if data.GuidedTour != nil {
		candidateID := data.GuidedTour.CandidateID
		if err := add(
			"guided_tour/"+candidateID+"/trigger",
			data.GuidedTour.Trigger,
			func(target *ReportData, text string) bool {
				if target.GuidedTour == nil ||
					target.GuidedTour.CandidateID != candidateID {
					return false
				}
				target.GuidedTour.Trigger = text
				return true
			},
		); err != nil {
			return err
		}
		for summaryIndex := range data.GuidedTour.GapSummary {
			for gapIndex := range data.GuidedTour.GapSummary[summaryIndex].Gaps {
				gapID := data.GuidedTour.GapSummary[summaryIndex].Gaps[gapIndex].ID
				for _, field := range []struct {
					name string
					text string
					set  func(*guidedtour.Gap, string)
				}{
					{"label", data.GuidedTour.GapSummary[summaryIndex].Gaps[gapIndex].Label, func(gap *guidedtour.Gap, text string) { gap.Label = text }},
					{"detail", data.GuidedTour.GapSummary[summaryIndex].Gaps[gapIndex].Detail, func(gap *guidedtour.Gap, text string) { gap.Detail = text }},
				} {
					field := field
					if err := add(
						"guided_tour/"+candidateID+"/gaps/"+gapID+"/"+field.name,
						field.text,
						func(target *ReportData, text string) bool {
							if target.GuidedTour == nil {
								return false
							}
							for summaryIndex := range target.GuidedTour.GapSummary {
								for gapIndex := range target.GuidedTour.GapSummary[summaryIndex].Gaps {
									gap := &target.GuidedTour.GapSummary[summaryIndex].Gaps[gapIndex]
									if gap.ID == gapID {
										field.set(gap, text)
										return true
									}
								}
							}
							return false
						},
					); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func addThesisAreaPresentationText(
	add presentationInventoryAdder,
	prefix string,
	area RepositoryThesisArea,
	index int,
	targetAreas func(*ReportData) []RepositoryThesisArea,
) error {
	owner := presentationAddress(prefix, thesisAreaIdentity(area), strconv.Itoa(index))
	if err := add(owner+"/label", area.Label, func(target *ReportData, text string) bool {
		areas := targetAreas(target)
		if index >= len(areas) {
			return false
		}
		areas[index].Label = text
		return true
	}); err != nil {
		return err
	}
	return add(owner+"/responsibility", area.Responsibility, func(target *ReportData, text string) bool {
		areas := targetAreas(target)
		if index >= len(areas) {
			return false
		}
		areas[index].Responsibility = text
		return true
	})
}

func thesisAreaIdentity(area RepositoryThesisArea) string {
	if area.CodeLocation != nil {
		return area.CodeLocation.Path + ":" +
			strconv.Itoa(area.CodeLocation.Line) + ":" +
			strconv.Itoa(area.CodeLocation.Column)
	}
	if area.MapTarget != nil {
		return string(area.MapTarget.Kind) + ":" +
			string(area.MapTarget.ComponentID) + ":" +
			string(area.MapTarget.FlowID) + ":" +
			area.MapTarget.SurfaceID
	}
	return "unresolved"
}

func addStudyPresentationText(
	add presentationInventoryAdder,
	data *ReportData,
) error {
	addDirection := func(direction StudyDirection) error {
		for readingIndex := range direction.ReadingAnchors {
			readingIndex := readingIndex
			reading := direction.ReadingAnchors[readingIndex]
			owner := studyReadingLocalizationOwner(direction.ID, reading)
			for noticeIndex := range reading.Source.noticeCandidates {
				_ = noticeIndex
				// noticeCandidates are private assembly state and never render.
			}
			_ = owner
		}
		for documentIndex := range direction.Documents {
			document := direction.Documents[documentIndex]
			owner := studyDocumentLocalizationOwner(direction.ID, document)
			if err := add(owner+"/label", document.Label, func(target *ReportData, text string) bool {
				return setStudyDocumentLabel(
					target,
					direction.ID,
					studyDocumentIdentity(document),
					text,
				)
			}); err != nil {
				return err
			}
		}
		return nil
	}
	if data.StudyMap != nil {
		for _, direction := range data.StudyMap.Directions {
			if err := addDirection(direction); err != nil {
				return err
			}
		}
		for _, direction := range data.StudyMap.HiddenDirections {
			if err := addDirection(direction); err != nil {
				return err
			}
		}
	}
	if data.IncompleteStudy != nil {
		for _, direction := range data.IncompleteStudy.Directions {
			if err := addDirection(direction); err != nil {
				return err
			}
		}
	}
	return nil
}

func studyDocumentLocalizationOwner(
	directionID string,
	document StudyDocumentReference,
) string {
	return "study/directions/" + directionID + "/documents/" +
		presentationOwnerDigest(studyDocumentIdentity(document))
}

func studyDocumentIdentity(document StudyDocumentReference) string {
	sourceIdentity := ""
	if document.Source != nil {
		sourceIdentity = document.Source.PresentationSHA256
	}
	return document.Location.Path + ":" +
		strconv.Itoa(document.Location.Line) + ":" +
		strconv.Itoa(document.Location.Column) + ":" +
		sourceIdentity
}

func setStudyDocumentLabel(
	data *ReportData,
	directionID,
	documentIdentity,
	text string,
) bool {
	found := false
	apply := func(directions []StudyDirection) {
		for directionIndex := range directions {
			direction := &directions[directionIndex]
			if direction.ID != directionID {
				continue
			}
			for documentIndex := range direction.Documents {
				document := &direction.Documents[documentIndex]
				if studyDocumentIdentity(*document) != documentIdentity {
					continue
				}
				document.Label = text
				found = true
			}
		}
	}
	if data.StudyMap != nil {
		apply(data.StudyMap.Directions)
		apply(data.StudyMap.HiddenDirections)
	}
	if data.IncompleteStudy != nil {
		apply(data.IncompleteStudy.Directions)
	}
	return found
}

func addMechanismPresentationText(
	add presentationInventoryAdder,
	data *ReportData,
) error {
	for mechanismIndex := range data.UserMechanisms {
		mechanismIndex := mechanismIndex
		artifactID := data.UserMechanisms[mechanismIndex].ArtifactID
		for contextIndex := range data.UserMechanisms[mechanismIndex].Context {
			contextIndex := contextIndex
			context := data.UserMechanisms[mechanismIndex].Context[contextIndex]
			owner := presentationAddress(
				"mechanisms/"+artifactID+"/context",
				mechanismContextIdentity(context),
				strconv.Itoa(contextIndex),
			)
			if err := add(owner+"/label", context.Label, func(target *ReportData, text string) bool {
				mechanism := findUserMechanism(target, artifactID)
				if mechanism == nil || contextIndex >= len(mechanism.Context) {
					return false
				}
				mechanism.Context[contextIndex].Label = text
				return true
			}); err != nil {
				return err
			}
			if err := add(owner+"/responsibility", context.Responsibility, func(target *ReportData, text string) bool {
				mechanism := findUserMechanism(target, artifactID)
				if mechanism == nil || contextIndex >= len(mechanism.Context) {
					return false
				}
				mechanism.Context[contextIndex].Responsibility = text
				return true
			}); err != nil {
				return err
			}
		}
		for readIndex := range data.UserMechanisms[mechanismIndex].ReadNext {
			readIndex := readIndex
			readNext := data.UserMechanisms[mechanismIndex].ReadNext[readIndex]
			owner := presentationAddress(
				"mechanisms/"+artifactID+"/read_next",
				readNext.Path,
				readNext.Symbol,
				strconv.Itoa(readNext.Line),
				strconv.Itoa(readNext.StepIndex),
			)
			if err := add(owner+"/label", readNext.Label, func(target *ReportData, text string) bool {
				mechanism := findUserMechanism(target, artifactID)
				if mechanism == nil || readIndex >= len(mechanism.ReadNext) {
					return false
				}
				mechanism.ReadNext[readIndex].Label = text
				return true
			}); err != nil {
				return err
			}
		}
		if err := addMechanismNoticePresentationText(add, data.UserMechanisms[mechanismIndex]); err != nil {
			return err
		}
	}
	return nil
}

func mechanismContextIdentity(context UserMechanismContext) string {
	if context.CodeLocation != nil {
		return context.CodeLocation.Path + ":" +
			strconv.Itoa(context.CodeLocation.Line) + ":" +
			strconv.Itoa(context.CodeLocation.Column)
	}
	if context.MapTarget != nil {
		return string(context.MapTarget.Kind) + ":" +
			string(context.MapTarget.ComponentID) + ":" +
			string(context.MapTarget.FlowID) + ":" +
			context.MapTarget.SurfaceID
	}
	return "unresolved"
}

func findUserMechanism(data *ReportData, artifactID string) *UserMechanism {
	for index := range data.UserMechanisms {
		if data.UserMechanisms[index].ArtifactID == artifactID {
			return &data.UserMechanisms[index]
		}
	}
	return nil
}

func addMechanismNoticePresentationText(
	add presentationInventoryAdder,
	mechanism UserMechanism,
) error {
	for stepIndex := range mechanism.Steps {
		stepIndex := stepIndex
		for noticeIndex := range mechanism.Steps[stepIndex].WhatToNotice {
			noticeIndex := noticeIndex
			notice := mechanism.Steps[stepIndex].WhatToNotice[noticeIndex]
			owner := presentationAddress(
				"mechanisms/"+mechanism.ArtifactID+"/steps/"+strconv.Itoa(stepIndex)+"/notices",
				notice.Path,
				fmt.Sprint(notice.SupportingRanges),
				strconv.Itoa(noticeIndex),
			)
			if err := add(owner+"/text", notice.Text, func(target *ReportData, text string) bool {
				current := findUserMechanism(target, mechanism.ArtifactID)
				if current == nil || stepIndex >= len(current.Steps) ||
					noticeIndex >= len(current.Steps[stepIndex].WhatToNotice) {
					return false
				}
				current.Steps[stepIndex].WhatToNotice[noticeIndex].Text = text
				return true
			}); err != nil {
				return err
			}
		}
	}
	for phaseIndex := range mechanism.Phases {
		phaseIndex := phaseIndex
		for noticeIndex := range mechanism.Phases[phaseIndex].WhatToNotice {
			noticeIndex := noticeIndex
			notice := mechanism.Phases[phaseIndex].WhatToNotice[noticeIndex]
			owner := presentationAddress(
				"mechanisms/"+mechanism.ArtifactID+"/phases/"+strconv.Itoa(phaseIndex)+"/notices",
				notice.Path,
				fmt.Sprint(notice.SupportingRanges),
				strconv.Itoa(noticeIndex),
			)
			if err := add(owner+"/text", notice.Text, func(target *ReportData, text string) bool {
				current := findUserMechanism(target, mechanism.ArtifactID)
				if current == nil || phaseIndex >= len(current.Phases) ||
					noticeIndex >= len(current.Phases[phaseIndex].WhatToNotice) {
					return false
				}
				current.Phases[phaseIndex].WhatToNotice[noticeIndex].Text = text
				return true
			}); err != nil {
				return err
			}
		}
		for detailIndex := range mechanism.Phases[phaseIndex].ImplementationDetails {
			detailIndex := detailIndex
			for noticeIndex := range mechanism.Phases[phaseIndex].ImplementationDetails[detailIndex].WhatToNotice {
				noticeIndex := noticeIndex
				notice := mechanism.Phases[phaseIndex].ImplementationDetails[detailIndex].WhatToNotice[noticeIndex]
				owner := presentationAddress(
					"mechanisms/"+mechanism.ArtifactID+"/phases/"+strconv.Itoa(phaseIndex)+
						"/implementation_details/"+strconv.Itoa(detailIndex)+"/notices",
					notice.Path,
					fmt.Sprint(notice.SupportingRanges),
					strconv.Itoa(noticeIndex),
				)
				if err := add(owner+"/text", notice.Text, func(target *ReportData, text string) bool {
					current := findUserMechanism(target, mechanism.ArtifactID)
					if current == nil || phaseIndex >= len(current.Phases) ||
						detailIndex >= len(current.Phases[phaseIndex].ImplementationDetails) ||
						noticeIndex >= len(current.Phases[phaseIndex].ImplementationDetails[detailIndex].WhatToNotice) {
						return false
					}
					current.Phases[phaseIndex].ImplementationDetails[detailIndex].WhatToNotice[noticeIndex].Text = text
					return true
				}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func addOperationsPresentationText(
	add presentationInventoryAdder,
	data *ReportData,
) error {
	if data.Operations == nil {
		return nil
	}
	for pathIndex := range data.Operations.Paths {
		pathIndex := pathIndex
		pathID := data.Operations.Paths[pathIndex].ID
		owner := "operations/paths/" + pathID
		if err := add(owner+"/title", data.Operations.Paths[pathIndex].Title, func(target *ReportData, text string) bool {
			path := findOperationalPath(target, pathID)
			if path == nil {
				return false
			}
			path.Title = text
			return true
		}); err != nil {
			return err
		}
		if err := add(owner+"/goal", data.Operations.Paths[pathIndex].Goal, func(target *ReportData, text string) bool {
			path := findOperationalPath(target, pathID)
			if path == nil {
				return false
			}
			path.Goal = text
			return true
		}); err != nil {
			return err
		}
		for actionIndex := range data.Operations.Paths[pathIndex].Actions {
			actionIndex := actionIndex
			action := data.Operations.Paths[pathIndex].Actions[actionIndex]
			actionOwner := presentationAddress(
				owner+"/actions",
				action.Reference.Location.Path,
				strconv.Itoa(action.Reference.Location.Line),
				action.Command,
				action.CopyText,
				action.Endpoint,
			)
			if err := add(actionOwner+"/instruction", action.Instruction, func(target *ReportData, text string) bool {
				path := findOperationalPath(target, pathID)
				if path == nil || actionIndex >= len(path.Actions) {
					return false
				}
				path.Actions[actionIndex].Instruction = text
				return true
			}); err != nil {
				return err
			}
			if err := addOperationalReferenceText(
				add,
				actionOwner+"/reference",
				action.Reference,
				func(target *ReportData) *OperationalReference {
					path := findOperationalPath(target, pathID)
					if path == nil || actionIndex >= len(path.Actions) {
						return nil
					}
					return &path.Actions[actionIndex].Reference
				},
			); err != nil {
				return err
			}
		}
		for resultIndex := range data.Operations.Paths[pathIndex].ExpectedResults {
			resultIndex := resultIndex
			result := data.Operations.Paths[pathIndex].ExpectedResults[resultIndex]
			resultOwner := presentationAddress(
				owner+"/expected_results",
				string(result.Kind),
				result.Value,
				strconv.Itoa(result.AfterAction),
				result.Reference.Location.Path,
				strconv.Itoa(result.Reference.Location.Line),
			)
			if err := addOperationalReferenceText(
				add,
				resultOwner+"/reference",
				result.Reference,
				func(target *ReportData) *OperationalReference {
					path := findOperationalPath(target, pathID)
					if path == nil || resultIndex >= len(path.ExpectedResults) {
						return nil
					}
					return &path.ExpectedResults[resultIndex].Reference
				},
			); err != nil {
				return err
			}
		}
		for _, references := range []struct {
			name  string
			items []OperationalReference
			get   func(*RepositoryPavedPath) *[]OperationalReference
		}{
			{"prerequisites", data.Operations.Paths[pathIndex].Prerequisites, func(path *RepositoryPavedPath) *[]OperationalReference { return &path.Prerequisites }},
			{"expected", data.Operations.Paths[pathIndex].Expected, func(path *RepositoryPavedPath) *[]OperationalReference { return &path.Expected }},
			{"troubleshooting", data.Operations.Paths[pathIndex].Troubleshooting, func(path *RepositoryPavedPath) *[]OperationalReference { return &path.Troubleshooting }},
		} {
			references := references
			for referenceIndex := range references.items {
				referenceIndex := referenceIndex
				reference := references.items[referenceIndex]
				referenceOwner := presentationAddress(
					owner+"/"+references.name,
					reference.Location.Path,
					strconv.Itoa(reference.Location.Line),
					reference.Role,
				)
				if err := addOperationalReferenceText(
					add,
					referenceOwner,
					reference,
					func(target *ReportData) *OperationalReference {
						path := findOperationalPath(target, pathID)
						if path == nil {
							return nil
						}
						items := references.get(path)
						if referenceIndex >= len(*items) {
							return nil
						}
						return &(*items)[referenceIndex]
					},
				); err != nil {
					return err
				}
			}
		}
	}
	for landmarkIndex := range data.Operations.Landmarks {
		landmarkIndex := landmarkIndex
		id := data.Operations.Landmarks[landmarkIndex].ID
		owner := "operations/landmarks/" + id
		if err := add(owner+"/label", data.Operations.Landmarks[landmarkIndex].Label, func(target *ReportData, text string) bool {
			if target.Operations == nil {
				return false
			}
			for index := range target.Operations.Landmarks {
				if target.Operations.Landmarks[index].ID == id {
					target.Operations.Landmarks[index].Label = text
					return true
				}
			}
			return false
		}); err != nil {
			return err
		}
		if err := addOperationalReferenceText(
			add,
			owner+"/reference",
			data.Operations.Landmarks[landmarkIndex].Reference,
			func(target *ReportData) *OperationalReference {
				if target.Operations == nil {
					return nil
				}
				for index := range target.Operations.Landmarks {
					if target.Operations.Landmarks[index].ID == id {
						return &target.Operations.Landmarks[index].Reference
					}
				}
				return nil
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func findOperationalPath(data *ReportData, id string) *RepositoryPavedPath {
	if data.Operations == nil {
		return nil
	}
	for index := range data.Operations.Paths {
		if data.Operations.Paths[index].ID == id {
			return &data.Operations.Paths[index]
		}
	}
	return nil
}

func addOperationalReferenceText(
	add presentationInventoryAdder,
	owner string,
	reference OperationalReference,
	targetReference func(*ReportData) *OperationalReference,
) error {
	return add(owner+"/label", reference.Label, func(target *ReportData, text string) bool {
		current := targetReference(target)
		if current == nil {
			return false
		}
		current.Label = text
		return true
	})
}

func addTaskInvestigationPresentationText(
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	task := data.TaskInvestigation
	if task == nil {
		return nil
	}
	add := addObject.with(taskInvestigationProtectedValues(*task))
	taskID := task.TaskID
	owner := "task_investigation/" + taskID
	if err := add(owner+"/task", task.Task, func(target *ReportData, text string) bool {
		if target.TaskInvestigation == nil ||
			target.TaskInvestigation.TaskID != taskID {
			return false
		}
		target.TaskInvestigation.Task = text
		return true
	}); err != nil {
		return err
	}
	for _, field := range []struct {
		name string
		text string
		set  func(*TaskInvestigationInterpretation, string)
	}{
		{"interpretation/restatement", task.Interpretation.Restatement, func(item *TaskInvestigationInterpretation, text string) { item.Restatement = text }},
		{"interpretation/observable", task.Interpretation.Observable, func(item *TaskInvestigationInterpretation, text string) { item.Observable = text }},
	} {
		field := field
		if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
			if target.TaskInvestigation == nil ||
				target.TaskInvestigation.TaskID != taskID {
				return false
			}
			field.set(&target.TaskInvestigation.Interpretation, text)
			return true
		}); err != nil {
			return err
		}
	}
	for index := range task.LikelyAreas {
		index := index
		area := task.LikelyAreas[index]
		areaOwner := presentationAddress(owner+"/areas", fmt.Sprint(area.AnchorIndexes), strconv.Itoa(index))
		if err := add(areaOwner+"/label", area.Label, func(target *ReportData, text string) bool {
			if target.TaskInvestigation == nil || index >= len(target.TaskInvestigation.LikelyAreas) {
				return false
			}
			target.TaskInvestigation.LikelyAreas[index].Label = text
			return true
		}); err != nil {
			return err
		}
		if err := add(areaOwner+"/why", area.Why, func(target *ReportData, text string) bool {
			if target.TaskInvestigation == nil || index >= len(target.TaskInvestigation.LikelyAreas) {
				return false
			}
			target.TaskInvestigation.LikelyAreas[index].Why = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range task.Anchors {
		index := index
		anchor := task.Anchors[index]
		anchorOwner := presentationAddress(
			owner+"/anchors",
			anchor.Path,
			anchor.Symbol,
			strconv.Itoa(anchor.StartLine),
			strconv.Itoa(anchor.EndLine),
		)
		if err := add(anchorOwner+"/why", anchor.Why, func(target *ReportData, text string) bool {
			if target.TaskInvestigation == nil || index >= len(target.TaskInvestigation.Anchors) {
				return false
			}
			target.TaskInvestigation.Anchors[index].Why = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range task.EvidenceJoins {
		index := index
		join := task.EvidenceJoins[index]
		joinOwner := presentationAddress(
			owner+"/joins",
			strconv.Itoa(join.LeftAnchor),
			strconv.Itoa(join.RightAnchor),
			join.Kind,
			fmt.Sprint(join.SupportAnchorIndexes),
		)
		if err := add(joinOwner+"/explanation", join.Explanation, func(target *ReportData, text string) bool {
			if target.TaskInvestigation == nil || index >= len(target.TaskInvestigation.EvidenceJoins) {
				return false
			}
			target.TaskInvestigation.EvidenceJoins[index].Explanation = text
			return true
		}); err != nil {
			return err
		}
		if err := add(joinOwner+"/scope", join.Scope, func(target *ReportData, text string) bool {
			if target.TaskInvestigation == nil || index >= len(target.TaskInvestigation.EvidenceJoins) {
				return false
			}
			target.TaskInvestigation.EvidenceJoins[index].Scope = text
			return true
		}); err != nil {
			return err
		}
	}
	for _, collection := range []struct {
		name   string
		values []TaskInvestigationHypothesis
	}{
		{"working_hypothesis", task.WorkingHypothesis},
	} {
		for index := range collection.values {
			index := index
			hypothesis := collection.values[index]
			hypothesisOwner := presentationAddress(
				owner+"/"+collection.name,
				string(hypothesis.Status),
				fmt.Sprint(hypothesis.SupportAnchorIndexes),
				strconv.Itoa(index),
			)
			if err := add(hypothesisOwner+"/text", hypothesis.Text, func(target *ReportData, text string) bool {
				if target.TaskInvestigation == nil ||
					index >= len(target.TaskInvestigation.WorkingHypothesis) {
					return false
				}
				target.TaskInvestigation.WorkingHypothesis[index].Text = text
				return true
			}); err != nil {
				return err
			}
		}
	}
	if err := addTaskGuidanceCollection(
		add, owner+"/reproduce_or_observe", task.ReproduceOrObserve,
		func(target *ReportData) *[]TaskInvestigationGuidance {
			if target.TaskInvestigation == nil {
				return nil
			}
			return &target.TaskInvestigation.ReproduceOrObserve
		},
	); err != nil {
		return err
	}
	if err := add(owner+"/verify/effect", task.Verify.Effect, func(target *ReportData, text string) bool {
		if target.TaskInvestigation == nil {
			return false
		}
		target.TaskInvestigation.Verify.Effect = text
		return true
	}); err != nil {
		return err
	}
	if err := addTaskGuidanceCollection(
		add, owner+"/verify/steps", task.Verify.Steps,
		func(target *ReportData) *[]TaskInvestigationGuidance {
			if target.TaskInvestigation == nil {
				return nil
			}
			return &target.TaskInvestigation.Verify.Steps
		},
	); err != nil {
		return err
	}
	for index := range task.NextProbes {
		index := index
		probe := task.NextProbes[index]
		probeOwner := presentationAddress(
			owner+"/next_probes",
			string(probe.Action),
			fmt.Sprint(probe.AnchorIndexes),
			strconv.Itoa(index),
		)
		if err := add(probeOwner+"/text", probe.Text, func(target *ReportData, text string) bool {
			if target.TaskInvestigation == nil || index >= len(target.TaskInvestigation.NextProbes) {
				return false
			}
			target.TaskInvestigation.NextProbes[index].Text = text
			return true
		}); err != nil {
			return err
		}
	}
	return nil
}

func addTaskGuidanceCollection(
	add presentationInventoryAdder,
	prefix string,
	values []TaskInvestigationGuidance,
	targetValues func(*ReportData) *[]TaskInvestigationGuidance,
) error {
	for index := range values {
		index := index
		guidance := values[index]
		owner := presentationAddress(
			prefix,
			string(guidance.Authority),
			fmt.Sprint(guidance.SupportAnchorIndexes),
			strconv.Itoa(index),
		)
		if err := add(owner+"/text", guidance.Text, func(target *ReportData, text string) bool {
			items := targetValues(target)
			if items == nil || index >= len(*items) {
				return false
			}
			(*items)[index].Text = text
			return true
		}); err != nil {
			return err
		}
	}
	return nil
}

func addSourceExplanationPresentationText(
	add presentationInventoryAdder,
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	for index := range data.UserSources {
		index := index
		sourceAdd := addObject.with(sourceSnippetProtectedValues(
			data.UserSources[index],
		))
		if err := addSourceLandmarkPresentationText(
			sourceAdd,
			data.UserSources[index],
			func(target *ReportData) *SourceSnippet {
				if index >= len(target.UserSources) {
					return nil
				}
				return &target.UserSources[index]
			},
		); err != nil {
			return err
		}
	}
	if data.StudyMap != nil {
		for areaIndex := range data.StudyMap.Shape {
			areaIndex := areaIndex
			if data.StudyMap.Shape[areaIndex].Source == nil {
				continue
			}
			if err := addSourceLandmarkPresentationText(
				add,
				*data.StudyMap.Shape[areaIndex].Source,
				func(target *ReportData) *SourceSnippet {
					if target.StudyMap == nil || areaIndex >= len(target.StudyMap.Shape) {
						return nil
					}
					return target.StudyMap.Shape[areaIndex].Source
				},
			); err != nil {
				return err
			}
		}
		for directionIndex := range data.StudyMap.Directions {
			if err := addStudyDirectionSourcePresentationText(
				add,
				data.StudyMap.Directions[directionIndex],
				func(target *ReportData) *StudyDirection {
					if target.StudyMap == nil {
						return nil
					}
					return findStudyDirection(target.StudyMap.Directions, data.StudyMap.Directions[directionIndex].ID)
				},
			); err != nil {
				return err
			}
		}
		for directionIndex := range data.StudyMap.HiddenDirections {
			if err := addStudyDirectionSourcePresentationText(
				add,
				data.StudyMap.HiddenDirections[directionIndex],
				func(target *ReportData) *StudyDirection {
					if target.StudyMap == nil {
						return nil
					}
					return findStudyDirection(target.StudyMap.HiddenDirections, data.StudyMap.HiddenDirections[directionIndex].ID)
				},
			); err != nil {
				return err
			}
		}
	}
	if data.IncompleteStudy != nil {
		for directionIndex := range data.IncompleteStudy.Directions {
			if err := addStudyDirectionSourcePresentationText(
				add,
				data.IncompleteStudy.Directions[directionIndex],
				func(target *ReportData) *StudyDirection {
					if target.IncompleteStudy == nil {
						return nil
					}
					return findStudyDirection(
						target.IncompleteStudy.Directions,
						data.IncompleteStudy.Directions[directionIndex].ID,
					)
				},
			); err != nil {
				return err
			}
		}
	}
	for mechanismIndex := range data.UserMechanisms {
		mechanismIndex := mechanismIndex
		artifactID := data.UserMechanisms[mechanismIndex].ArtifactID
		for stepIndex := range data.UserMechanisms[mechanismIndex].Steps {
			stepIndex := stepIndex
			for sourceIndex := range data.UserMechanisms[mechanismIndex].Steps[stepIndex].Sources {
				sourceIndex := sourceIndex
				if err := addSourceLandmarkPresentationText(
					add,
					data.UserMechanisms[mechanismIndex].Steps[stepIndex].Sources[sourceIndex],
					func(target *ReportData) *SourceSnippet {
						mechanism := findUserMechanism(target, artifactID)
						if mechanism == nil || stepIndex >= len(mechanism.Steps) ||
							sourceIndex >= len(mechanism.Steps[stepIndex].Sources) {
							return nil
						}
						return &mechanism.Steps[stepIndex].Sources[sourceIndex]
					},
				); err != nil {
					return err
				}
			}
		}
		for phaseIndex := range data.UserMechanisms[mechanismIndex].Phases {
			phaseIndex := phaseIndex
			for sourceIndex := range data.UserMechanisms[mechanismIndex].Phases[phaseIndex].Sources {
				sourceIndex := sourceIndex
				if err := addSourceLandmarkPresentationText(
					add,
					data.UserMechanisms[mechanismIndex].Phases[phaseIndex].Sources[sourceIndex],
					func(target *ReportData) *SourceSnippet {
						mechanism := findUserMechanism(target, artifactID)
						if mechanism == nil || phaseIndex >= len(mechanism.Phases) ||
							sourceIndex >= len(mechanism.Phases[phaseIndex].Sources) {
							return nil
						}
						return &mechanism.Phases[phaseIndex].Sources[sourceIndex]
					},
				); err != nil {
					return err
				}
			}
			for detailIndex := range data.UserMechanisms[mechanismIndex].Phases[phaseIndex].ImplementationDetails {
				detailIndex := detailIndex
				for sourceIndex := range data.UserMechanisms[mechanismIndex].Phases[phaseIndex].ImplementationDetails[detailIndex].Sources {
					sourceIndex := sourceIndex
					if err := addSourceLandmarkPresentationText(
						add,
						data.UserMechanisms[mechanismIndex].Phases[phaseIndex].ImplementationDetails[detailIndex].Sources[sourceIndex],
						func(target *ReportData) *SourceSnippet {
							mechanism := findUserMechanism(target, artifactID)
							if mechanism == nil ||
								phaseIndex >= len(mechanism.Phases) ||
								detailIndex >= len(mechanism.Phases[phaseIndex].ImplementationDetails) ||
								sourceIndex >= len(mechanism.Phases[phaseIndex].ImplementationDetails[detailIndex].Sources) {
								return nil
							}
							return &mechanism.Phases[phaseIndex].ImplementationDetails[detailIndex].Sources[sourceIndex]
						},
					); err != nil {
						return err
					}
				}
			}
		}
	}
	if err := addOperationsSourcePresentationText(add, data); err != nil {
		return err
	}
	if data.TaskInvestigation != nil {
		for anchorIndex := range data.TaskInvestigation.Anchors {
			anchorIndex := anchorIndex
			sourceAdd := addObject.with(sourceSnippetProtectedValues(
				data.TaskInvestigation.Anchors[anchorIndex].Source,
			))
			if err := addSourceLandmarkPresentationText(
				sourceAdd,
				data.TaskInvestigation.Anchors[anchorIndex].Source,
				func(target *ReportData) *SourceSnippet {
					if target.TaskInvestigation == nil ||
						anchorIndex >= len(target.TaskInvestigation.Anchors) {
						return nil
					}
					return &target.TaskInvestigation.Anchors[anchorIndex].Source
				},
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func addOperationsSourcePresentationText(
	add presentationInventoryAdder,
	data *ReportData,
) error {
	if data.Operations == nil {
		return nil
	}
	for pathIndex := range data.Operations.Paths {
		pathIndex := pathIndex
		path := data.Operations.Paths[pathIndex]
		for actionIndex := range path.Actions {
			actionIndex := actionIndex
			if err := addSourceLandmarkPresentationText(
				add,
				path.Actions[actionIndex].Reference.Source,
				func(target *ReportData) *SourceSnippet {
					current := findOperationalPath(target, path.ID)
					if current == nil || actionIndex >= len(current.Actions) {
						return nil
					}
					return &current.Actions[actionIndex].Reference.Source
				},
			); err != nil {
				return err
			}
		}
		for resultIndex := range path.ExpectedResults {
			resultIndex := resultIndex
			if err := addSourceLandmarkPresentationText(
				add,
				path.ExpectedResults[resultIndex].Reference.Source,
				func(target *ReportData) *SourceSnippet {
					current := findOperationalPath(target, path.ID)
					if current == nil || resultIndex >= len(current.ExpectedResults) {
						return nil
					}
					return &current.ExpectedResults[resultIndex].Reference.Source
				},
			); err != nil {
				return err
			}
		}
		for _, collection := range []struct {
			items []OperationalReference
			get   func(*RepositoryPavedPath) *[]OperationalReference
		}{
			{path.Prerequisites, func(item *RepositoryPavedPath) *[]OperationalReference { return &item.Prerequisites }},
			{path.Expected, func(item *RepositoryPavedPath) *[]OperationalReference { return &item.Expected }},
			{path.Troubleshooting, func(item *RepositoryPavedPath) *[]OperationalReference { return &item.Troubleshooting }},
		} {
			collection := collection
			for referenceIndex := range collection.items {
				referenceIndex := referenceIndex
				if err := addSourceLandmarkPresentationText(
					add,
					collection.items[referenceIndex].Source,
					func(target *ReportData) *SourceSnippet {
						current := findOperationalPath(target, path.ID)
						if current == nil {
							return nil
						}
						items := collection.get(current)
						if referenceIndex >= len(*items) {
							return nil
						}
						return &(*items)[referenceIndex].Source
					},
				); err != nil {
					return err
				}
			}
		}
	}
	for landmarkIndex := range data.Operations.Landmarks {
		landmarkIndex := landmarkIndex
		landmarkID := data.Operations.Landmarks[landmarkIndex].ID
		if err := addSourceLandmarkPresentationText(
			add,
			data.Operations.Landmarks[landmarkIndex].Reference.Source,
			func(target *ReportData) *SourceSnippet {
				if target.Operations == nil {
					return nil
				}
				for index := range target.Operations.Landmarks {
					if target.Operations.Landmarks[index].ID == landmarkID {
						return &target.Operations.Landmarks[index].Reference.Source
					}
				}
				return nil
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func addSourceLandmarkPresentationText(
	add presentationInventoryAdder,
	snippet SourceSnippet,
	targetSource func(*ReportData) *SourceSnippet,
) error {
	identity := snippet.PresentationSHA256
	if identity == "" {
		identity = presentationOwnerDigest(
			snippet.Path,
			strconv.Itoa(snippet.StartLine),
			strconv.Itoa(snippet.EndLine),
			snippet.EnclosingSymbol,
		)
	}
	return add(
		"source_snippets/"+identity+"/landmark_reason",
		snippet.LandmarkReason,
		func(target *ReportData, text string) bool {
			current := targetSource(target)
			if current == nil {
				return false
			}
			current.PresentationLandmarkReason = text
			return true
		},
	)
}

func addStudyDirectionSourcePresentationText(
	add presentationInventoryAdder,
	direction StudyDirection,
	targetDirection func(*ReportData) *StudyDirection,
) error {
	for readingIndex := range direction.ReadingAnchors {
		readingIndex := readingIndex
		if err := addSourceLandmarkPresentationText(
			add,
			direction.ReadingAnchors[readingIndex].Source,
			func(target *ReportData) *SourceSnippet {
				current := targetDirection(target)
				if current == nil || readingIndex >= len(current.ReadingAnchors) {
					return nil
				}
				return &current.ReadingAnchors[readingIndex].Source
			},
		); err != nil {
			return err
		}
	}
	for documentIndex := range direction.Documents {
		documentIndex := documentIndex
		if direction.Documents[documentIndex].Source == nil {
			continue
		}
		if err := addSourceLandmarkPresentationText(
			add,
			*direction.Documents[documentIndex].Source,
			func(target *ReportData) *SourceSnippet {
				current := targetDirection(target)
				if current == nil || documentIndex >= len(current.Documents) {
					return nil
				}
				return current.Documents[documentIndex].Source
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func findStudyDirection(
	directions []StudyDirection,
	id string,
) *StudyDirection {
	for index := range directions {
		if directions[index].ID == id {
			return &directions[index]
		}
	}
	return nil
}

func addResearchPresentationText(
	addObject presentationInventoryObjectAdder,
	data *ReportData,
) error {
	if data.ModelResearch == nil {
		return nil
	}
	add := addObject.with(modelResearchProtectedValues(*data.ModelResearch))
	for roundIndex := range data.ModelResearch.Rounds {
		roundIndex := roundIndex
		roundID := data.ModelResearch.Rounds[roundIndex].ID
		owner := "model_research/rounds/" + roundID
		if err := add(owner+"/question", data.ModelResearch.Rounds[roundIndex].Question, func(target *ReportData, text string) bool {
			if target.ModelResearch == nil {
				return false
			}
			for index := range target.ModelResearch.Rounds {
				if target.ModelResearch.Rounds[index].ID == roundID {
					target.ModelResearch.Rounds[index].Question = text
					return true
				}
			}
			return false
		}); err != nil {
			return err
		}
		for frontierIndex := range data.ModelResearch.Rounds[roundIndex].UnresolvedFrontiers {
			frontierIndex := frontierIndex
			if err := add(
				owner+"/frontiers/"+strconv.Itoa(frontierIndex)+"/question",
				data.ModelResearch.Rounds[roundIndex].UnresolvedFrontiers[frontierIndex].Question,
				func(target *ReportData, text string) bool {
					if target.ModelResearch == nil {
						return false
					}
					for index := range target.ModelResearch.Rounds {
						round := &target.ModelResearch.Rounds[index]
						if round.ID == roundID &&
							frontierIndex < len(round.UnresolvedFrontiers) {
							round.UnresolvedFrontiers[frontierIndex].Question = text
							return true
						}
					}
					return false
				},
			); err != nil {
				return err
			}
		}
	}
	for roundIndex := range data.ModelResearch.SkippedRounds {
		roundIndex := roundIndex
		roundID := data.ModelResearch.SkippedRounds[roundIndex].ID
		owner := "model_research/skipped_rounds/" + roundID
		if err := add(
			owner+"/question",
			data.ModelResearch.SkippedRounds[roundIndex].Question,
			func(target *ReportData, text string) bool {
				if target.ModelResearch == nil {
					return false
				}
				for index := range target.ModelResearch.SkippedRounds {
					if target.ModelResearch.SkippedRounds[index].ID == roundID {
						target.ModelResearch.SkippedRounds[index].Question = text
						return true
					}
				}
				return false
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func addSourceEpisodePresentationText(
	add presentationInventoryAdder,
	data *ReportData,
) error {
	episode := data.presentationSourceEpisode
	if episode == nil {
		return nil
	}
	owner := "source_episode/" + episode.EpisodeID
	if err := add(owner+"/question", episode.Question, func(target *ReportData, text string) bool {
		if target.presentationSourceEpisode == nil ||
			target.presentationSourceEpisode.EpisodeID != episode.EpisodeID {
			return false
		}
		target.presentationSourceEpisode.Question = text
		return true
	}); err != nil {
		return err
	}
	for claimIndex := range episode.Claims {
		claimIndex := claimIndex
		claimID := episode.Claims[claimIndex].ID
		claimOwner := owner + "/claims/" + claimID
		for _, field := range []struct {
			name string
			text string
			set  func(*sourceEpisodeProjectedClaim, string)
		}{
			{"title", episode.Claims[claimIndex].Title, func(item *sourceEpisodeProjectedClaim, text string) { item.Title = text }},
			{"statement", episode.Claims[claimIndex].Statement, func(item *sourceEpisodeProjectedClaim, text string) { item.Statement = text }},
		} {
			field := field
			if err := add(claimOwner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				claim := findSourceEpisodeClaim(target, episode.EpisodeID, claimID)
				if claim == nil {
					return false
				}
				field.set(claim, text)
				return true
			}); err != nil {
				return err
			}
		}
		for limitIndex := range episode.Claims[claimIndex].Limits {
			limitIndex := limitIndex
			if err := add(
				claimOwner+"/limits/"+strconv.Itoa(limitIndex),
				episode.Claims[claimIndex].Limits[limitIndex],
				func(target *ReportData, text string) bool {
					claim := findSourceEpisodeClaim(target, episode.EpisodeID, claimID)
					if claim == nil || limitIndex >= len(claim.Limits) {
						return false
					}
					claim.Limits[limitIndex] = text
					return true
				},
			); err != nil {
				return err
			}
		}
	}
	for gapIndex := range episode.Uncertainties {
		gapIndex := gapIndex
		gapID := episode.Uncertainties[gapIndex].ID
		if err := add(
			owner+"/uncertainties/"+gapID+"/statement",
			episode.Uncertainties[gapIndex].Statement,
			func(target *ReportData, text string) bool {
				gap := findSourceEpisodeGap(target, episode.EpisodeID, gapID)
				if gap == nil {
					return false
				}
				gap.Statement = text
				return true
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func findSourceEpisodeClaim(
	data *ReportData,
	episodeID,
	claimID string,
) *sourceEpisodeProjectedClaim {
	if data.presentationSourceEpisode == nil ||
		data.presentationSourceEpisode.EpisodeID != episodeID {
		return nil
	}
	for index := range data.presentationSourceEpisode.Claims {
		if data.presentationSourceEpisode.Claims[index].ID == claimID {
			return &data.presentationSourceEpisode.Claims[index]
		}
	}
	return nil
}

func findSourceEpisodeGap(
	data *ReportData,
	episodeID,
	gapID string,
) *sourceEpisodeProjectedGap {
	if data.presentationSourceEpisode == nil ||
		data.presentationSourceEpisode.EpisodeID != episodeID {
		return nil
	}
	for index := range data.presentationSourceEpisode.Uncertainties {
		if data.presentationSourceEpisode.Uncertainties[index].ID == gapID {
			return &data.presentationSourceEpisode.Uncertainties[index]
		}
	}
	return nil
}

func addSurfacePresentationText(
	add presentationInventoryAdder,
	data *ReportData,
) error {
	catalog := data.DiscoveredSurfaces
	if catalog == nil {
		return nil
	}
	if err := add(
		"surfaces/scope_statement",
		catalog.ScopeStatement,
		func(target *ReportData, text string) bool {
			if target.DiscoveredSurfaces == nil {
				return false
			}
			target.DiscoveredSurfaces.ScopeStatement = text
			return true
		},
	); err != nil {
		return err
	}
	for triggerIndex := range catalog.Triggers {
		triggerIndex := triggerIndex
		triggerID := catalog.Triggers[triggerIndex].ID
		owner := "surfaces/triggers/" + triggerID
		for _, field := range []struct {
			name string
			text string
			set  func(*DiscoveredTrigger, string)
		}{
			{"unavailable_reason", catalog.Triggers[triggerIndex].UnavailableReason, func(item *DiscoveredTrigger, text string) { item.UnavailableReason = text }},
			{"trace_unavailable_reason", catalog.Triggers[triggerIndex].TraceUnavailableReason, func(item *DiscoveredTrigger, text string) { item.TraceUnavailableReason = text }},
			{"trace_readiness_reason", catalog.Triggers[triggerIndex].TraceReadinessReason, func(item *DiscoveredTrigger, text string) { item.TraceReadinessReason = text }},
		} {
			field := field
			if err := add(owner+"/"+field.name, field.text, func(target *ReportData, text string) bool {
				trigger := findDiscoveredTrigger(target, triggerID)
				if trigger == nil {
					return false
				}
				field.set(trigger, text)
				return true
			}); err != nil {
				return err
			}
		}
		for evidenceIndex := range catalog.Triggers[triggerIndex].Evidence {
			evidenceIndex := evidenceIndex
			evidence := catalog.Triggers[triggerIndex].Evidence[evidenceIndex]
			evidenceID := evidence.ID
			if evidenceID == "" {
				evidenceID = presentationOwnerDigest(
					evidence.Kind,
					surfaceLocationIdentity(evidence.Location),
				)
			}
			if err := add(
				owner+"/evidence/"+evidenceID+"/detail",
				evidence.Detail,
				func(target *ReportData, text string) bool {
					trigger := findDiscoveredTrigger(target, triggerID)
					if trigger == nil || evidenceIndex >= len(trigger.Evidence) {
						return false
					}
					trigger.Evidence[evidenceIndex].Detail = text
					return true
				},
			); err != nil {
				return err
			}
		}
		for frontierIndex := range catalog.Triggers[triggerIndex].DynamicFrontier {
			frontierIndex := frontierIndex
			frontier := catalog.Triggers[triggerIndex].DynamicFrontier[frontierIndex]
			frontierID := surfaceFrontierIdentity(frontier, frontierIndex)
			if err := add(
				owner+"/frontiers/"+frontierID+"/detail",
				frontier.Detail,
				func(target *ReportData, text string) bool {
					trigger := findDiscoveredTrigger(target, triggerID)
					if trigger == nil || frontierIndex >= len(trigger.DynamicFrontier) {
						return false
					}
					trigger.DynamicFrontier[frontierIndex].Detail = text
					return true
				},
			); err != nil {
				return err
			}
		}
	}
	for signalIndex := range catalog.LoopSignals {
		signalIndex := signalIndex
		signal := catalog.LoopSignals[signalIndex]
		owner := presentationAddress(
			"surfaces/loop_signals",
			signal.Kind,
			signal.FunctionID,
			surfaceLocationIdentity(signal.Location),
		)
		if err := add(owner+"/detail", signal.Detail, func(target *ReportData, text string) bool {
			if target.DiscoveredSurfaces == nil ||
				signalIndex >= len(target.DiscoveredSurfaces.LoopSignals) {
				return false
			}
			target.DiscoveredSurfaces.LoopSignals[signalIndex].Detail = text
			return true
		}); err != nil {
			return err
		}
	}
	for _, collection := range []struct {
		name  string
		items []SurfaceFrontier
		get   func(*DiscoveredSurfaces) *[]SurfaceFrontier
	}{
		{"dynamic_frontiers", catalog.DynamicFrontiers, func(item *DiscoveredSurfaces) *[]SurfaceFrontier { return &item.DynamicFrontiers }},
		{"unsupported_dispatch", catalog.UnsupportedDispatch, func(item *DiscoveredSurfaces) *[]SurfaceFrontier { return &item.UnsupportedDispatch }},
	} {
		collection := collection
		for frontierIndex := range collection.items {
			frontierIndex := frontierIndex
			frontier := collection.items[frontierIndex]
			owner := "surfaces/" + collection.name + "/" + surfaceFrontierIdentity(frontier, frontierIndex)
			if err := add(owner+"/detail", frontier.Detail, func(target *ReportData, text string) bool {
				if target.DiscoveredSurfaces == nil {
					return false
				}
				items := collection.get(target.DiscoveredSurfaces)
				if frontierIndex >= len(*items) {
					return false
				}
				(*items)[frontierIndex].Detail = text
				return true
			}); err != nil {
				return err
			}
		}
	}
	for index := range catalog.UnavailablePackages {
		index := index
		item := catalog.UnavailablePackages[index]
		owner := presentationAddress(
			"surfaces/unavailable_packages",
			item.Package,
			item.OwningExecutable,
		)
		if err := add(owner+"/reason", item.Reason, func(target *ReportData, text string) bool {
			if target.DiscoveredSurfaces == nil ||
				index >= len(target.DiscoveredSurfaces.UnavailablePackages) {
				return false
			}
			target.DiscoveredSurfaces.UnavailablePackages[index].Reason = text
			return true
		}); err != nil {
			return err
		}
	}
	for index := range catalog.PackageDiagnostics {
		index := index
		item := catalog.PackageDiagnostics[index]
		id := item.ID
		if id == "" {
			id = presentationOwnerDigest(item.Kind, item.Package, surfaceLocationIdentity(item.Location))
		}
		if err := add(
			"surfaces/package_diagnostics/"+id+"/message",
			item.Message,
			func(target *ReportData, text string) bool {
				if target.DiscoveredSurfaces == nil ||
					index >= len(target.DiscoveredSurfaces.PackageDiagnostics) {
					return false
				}
				target.DiscoveredSurfaces.PackageDiagnostics[index].Message = text
				return true
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func findDiscoveredTrigger(data *ReportData, id string) *DiscoveredTrigger {
	if data.DiscoveredSurfaces == nil {
		return nil
	}
	for index := range data.DiscoveredSurfaces.Triggers {
		if data.DiscoveredSurfaces.Triggers[index].ID == id {
			return &data.DiscoveredSurfaces.Triggers[index]
		}
	}
	return nil
}

func surfaceFrontierIdentity(frontier SurfaceFrontier, fallbackIndex int) string {
	location := surfaceLocationIdentity(frontier.Location)
	if location != "" {
		return presentationOwnerDigest(frontier.Kind, location)
	}
	// Dynamic frontier entries without an exact location can legitimately share
	// a kind. Their bounded collection order is the only stable identity that
	// does not depend on the prose being localized.
	return presentationOwnerDigest(frontier.Kind, strconv.Itoa(fallbackIndex))
}

func surfaceLocationIdentity(location *SurfaceLocation) string {
	if location == nil {
		return ""
	}
	return location.Path + ":" +
		strconv.Itoa(location.Line) + ":" +
		strconv.Itoa(location.Column)
}
