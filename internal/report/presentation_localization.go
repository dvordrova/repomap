package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/dvordrova/repomap/internal/componentmap"
	"github.com/dvordrova/repomap/internal/guidedtour"
	"github.com/dvordrova/repomap/internal/localization"
	"github.com/dvordrova/repomap/internal/tasklens"
)

// PresentationLocalizationContractVersion changes whenever the complete
// presentation inventory or its stable owner binding changes. It is part of
// cache identity.
const (
	PresentationLocalizationContractVersion = "report-presentation-localization-v9"
	PresentationTextInventoryVersion        = "presentation-text-inventory-v6"
)

// PresentationTextInventory is the complete bounded terminal-presentation
// prose inventory for one canonical English report. Field IDs are stable
// presentation addresses; they do not become semantic identity or authority.
type PresentationTextInventory struct {
	Version   string
	Canonical localization.CanonicalArtifact
	Input     localization.Input
}

// PreparedPresentationLocalization is retained as the call-site name for the
// validated inventory used by the cache and projection stages.
type PreparedPresentationLocalization = PresentationTextInventory

type presentationLocalizationBinding struct {
	spec    localization.FieldSpec
	setters []func(*ReportData, string) bool
}

type presentationLocalizationBindings struct {
	ordered []string
	byID    map[string]*presentationLocalizationBinding
}

func newPresentationLocalizationBindings() *presentationLocalizationBindings {
	return &presentationLocalizationBindings{
		byID: make(map[string]*presentationLocalizationBinding),
	}
}

func (bindings *presentationLocalizationBindings) add(
	kind localization.OwnerKind,
	ownerID string,
	name localization.FieldName,
	text string,
	protected []localization.ProtectedValue,
	setter func(*ReportData, string) bool,
) error {
	return bindings.addAddress(
		presentationTextAddress(kind, ownerID, name),
		text,
		protected,
		setter,
	)
}

func (bindings *presentationLocalizationBindings) addAddress(
	address string,
	text string,
	protected []localization.ProtectedValue,
	setter func(*ReportData, string) bool,
) error {
	if text == "" {
		return nil
	}
	protected = append(
		append([]localization.ProtectedValue(nil), protected...),
		presentationOpaqueValuesInText(text)...,
	)
	id, err := localization.FieldID(
		localization.OwnerPresentationText,
		address,
		localization.FieldText,
	)
	if err != nil {
		return err
	}
	spec := localization.FieldSpec{
		OwnerKind:      localization.OwnerPresentationText,
		OwnerID:        address,
		Name:           localization.FieldText,
		Text:           text,
		ProtectedTerms: presentProtectedValues(text, protected),
	}
	if existing := bindings.byID[id]; existing != nil {
		left, leftErr := localization.NewCanonical([]localization.FieldSpec{existing.spec})
		right, rightErr := localization.NewCanonical([]localization.FieldSpec{spec})
		if leftErr != nil || rightErr != nil ||
			len(left.Fields) != 1 || len(right.Fields) != 1 ||
			!bytes.Equal(localizationBindingJSON(left.Fields[0]), localizationBindingJSON(right.Fields[0])) {
			return fmt.Errorf("report localization: stable owner %q has conflicting canonical prose", id)
		}
		existing.setters = append(existing.setters, setter)
		return nil
	}
	bindings.ordered = append(bindings.ordered, id)
	bindings.byID[id] = &presentationLocalizationBinding{
		spec:    spec,
		setters: []func(*ReportData, string) bool{setter},
	}
	return nil
}

func presentationOpaqueValuesInText(text string) []localization.ProtectedValue {
	tokens := strings.Fields(text)
	result := make([]localization.ProtectedValue, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, raw := range tokens {
		token := strings.Trim(raw, "\"'`()[]{}<>,;!?")
		token = strings.TrimSuffix(token, ".")
		if token == "" {
			continue
		}
		kind := localization.ProtectedKind("")
		switch {
		case strings.Contains(token, "://"):
			kind = localization.ProtectedURL
		case strings.HasPrefix(token, "--"):
			kind = localization.ProtectedIdentifier
		case strings.Contains(token, "/") &&
			token != "and/or" &&
			token != "input/output":
			kind = localization.ProtectedPath
		case strings.Contains(token, "::"):
			kind = localization.ProtectedSymbol
		case looksLikePresentationFileToken(token):
			kind = localization.ProtectedPath
		}
		if kind == "" {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, localization.ProtectedValue{
			Kind:  kind,
			Value: token,
		})
	}
	return result
}

func looksLikePresentationFileToken(token string) bool {
	lower := strings.ToLower(token)
	for _, suffix := range []string{
		".c", ".cc", ".cpp", ".cs", ".go", ".h", ".hpp", ".html", ".java",
		".js", ".json", ".jsx", ".kt", ".md", ".php", ".proto", ".py",
		".rb", ".rs", ".sh", ".sql", ".toml", ".ts", ".tsx", ".yaml", ".yml",
	} {
		if strings.Contains(lower, suffix) {
			return true
		}
	}
	return false
}

func (bindings *presentationLocalizationBindings) specs() []localization.FieldSpec {
	specs := make([]localization.FieldSpec, 0, len(bindings.ordered))
	for _, id := range bindings.ordered {
		specs = append(specs, bindings.byID[id].spec)
	}
	return specs
}

// PreparePresentationLocalization extracts the complete typed inventory of
// terminal-presentation prose. Repository evidence, source, identities, and
// navigation remain outside the provider-visible translation input.
func PreparePresentationLocalization(
	data *ReportData,
	targetLocale string,
) (PreparedPresentationLocalization, error) {
	bindings, err := buildPresentationLocalizationBindings(data)
	if err != nil {
		return PreparedPresentationLocalization{}, err
	}
	canonical, err := localization.NewCanonical(bindings.specs())
	if err != nil {
		return PreparedPresentationLocalization{}, fmt.Errorf(
			"report localization: build canonical presentation: %w",
			err,
		)
	}
	input, err := localization.BuildInput(canonical, targetLocale)
	if err != nil {
		return PreparedPresentationLocalization{}, fmt.Errorf(
			"report localization: build translation input: %w",
			err,
		)
	}
	return PreparedPresentationLocalization{
		Version:   PresentationTextInventoryVersion,
		Canonical: canonical,
		Input:     input,
	}, nil
}

func presentationTextAddress(
	kind localization.OwnerKind,
	ownerID string,
	name localization.FieldName,
) string {
	return string(kind) + "/" + ownerID + "/" + string(name)
}

// ApplyPresentationLocalization rebinds the supplied canonical/input pair to
// the current report before applying an atomic projection. Any rejected field
// rejects the whole RU presentation; callers can then render canonical EN.
func ApplyPresentationLocalization(
	data *ReportData,
	prepared PreparedPresentationLocalization,
	projection localization.Projection,
) (*ReportData, localization.Result, error) {
	if data == nil {
		return nil, localization.Result{}, fmt.Errorf("report localization: data is required")
	}
	expected, err := PreparePresentationLocalization(data, prepared.Input.TargetLocale)
	if err != nil {
		return nil, localization.Result{}, err
	}
	canonicalJSON, err := localization.MarshalCanonical(prepared.Canonical)
	if err != nil {
		return nil, localization.Result{}, fmt.Errorf("report localization: invalid canonical input")
	}
	expectedCanonicalJSON, err := localization.MarshalCanonical(expected.Canonical)
	if err != nil {
		return nil, localization.Result{}, err
	}
	inputJSON, err := json.Marshal(prepared.Input)
	if err != nil {
		return nil, localization.Result{}, fmt.Errorf("report localization: encode input: %w", err)
	}
	expectedInputJSON, err := json.Marshal(expected.Input)
	if err != nil {
		return nil, localization.Result{}, fmt.Errorf("report localization: encode current input: %w", err)
	}
	if !bytes.Equal(canonicalJSON, expectedCanonicalJSON) ||
		!bytes.Equal(inputJSON, expectedInputJSON) {
		return nil, localization.Result{}, fmt.Errorf(
			"report localization: canonical input does not match current report",
		)
	}
	result, err := localization.Apply(prepared.Canonical, prepared.Input, projection)
	if err != nil {
		return nil, localization.Result{}, fmt.Errorf("report localization: apply projection: %w", err)
	}
	if result.Fallback || len(result.Diagnostics) != 0 ||
		result.Locale != localization.LocaleRussian {
		return nil, result, fmt.Errorf("report localization: projection was not fully accepted")
	}
	if !presentationLocalizationChanged(prepared.Canonical, result) {
		return nil, result, fmt.Errorf(
			"report localization: Russian projection repeated canonical English",
		)
	}

	projected, err := cloneReportData(data)
	if err != nil {
		return nil, result, err
	}
	bindings, err := buildPresentationLocalizationBindings(projected)
	if err != nil {
		return nil, result, err
	}
	if len(result.Fields) != len(bindings.byID) {
		return nil, result, fmt.Errorf("report localization: projected field set mismatch")
	}
	for _, field := range result.Fields {
		binding := bindings.byID[field.ID]
		if binding == nil || len(binding.setters) == 0 {
			return nil, result, fmt.Errorf("report localization: projected field set mismatch")
		}
		for _, setter := range binding.setters {
			if !setter(projected, field.Text) {
				return nil, result, fmt.Errorf("report localization: semantic owner disappeared")
			}
		}
	}
	projected.ReportLanguage = localization.LocaleRussian
	projected.presentationLocalizationState = PresentationLocalizationSucceeded
	projected.presentationLocalizationMessageID = "main.localization.ru_active"
	return projected, result, nil
}

func presentationLocalizationChanged(
	canonical localization.CanonicalArtifact,
	result localization.Result,
) bool {
	if len(result.Fields) != len(canonical.Fields) {
		return false
	}
	for index := range canonical.Fields {
		if result.Fields[index].ID != canonical.Fields[index].ID {
			return false
		}
		if result.Fields[index].Text != canonical.Fields[index].Text {
			return true
		}
	}
	return false
}

func buildPresentationLocalizationBindings(
	data *ReportData,
) (*presentationLocalizationBindings, error) {
	if data == nil {
		return nil, fmt.Errorf("report localization: data is required")
	}
	bindings := newPresentationLocalizationBindings()
	repositoryProtected := repositoryPresentationProtectedValues(data)
	add := func(
		kind localization.OwnerKind,
		ownerID string,
		name localization.FieldName,
		text string,
		protected []localization.ProtectedValue,
		setter func(*ReportData, string) bool,
	) error {
		return bindings.add(kind, ownerID, name, text, protected, setter)
	}

	if err := add(
		localization.OwnerRepository, "repository", localization.FieldProjectGuess,
		data.ProjectGuess, repositoryProtected,
		func(target *ReportData, text string) bool { target.ProjectGuess = text; return true },
	); err != nil {
		return nil, err
	}
	if err := add(
		localization.OwnerRepository, "repository", localization.FieldDocumentedPurpose,
		data.DocumentedPurpose, repositoryProtected,
		func(target *ReportData, text string) bool { target.DocumentedPurpose = text; return true },
	); err != nil {
		return nil, err
	}

	if canvas := data.ArchitectureCanvas; canvas != nil {
		components := make(map[string]ArchitectureComponent, len(canvas.Components))
		for _, component := range canvas.Components {
			components[string(component.ID)] = component
		}
		for _, subsystem := range canvas.Subsystems {
			id := string(subsystem.ID)
			protected := presentationSubsystemProtectedValues(subsystem, componentMapByID(canvas.Components))
			if err := add(
				localization.OwnerSubsystem, id, localization.FieldNameText,
				subsystem.Name, protected,
				func(target *ReportData, text string) bool {
					if target.ArchitectureCanvas == nil {
						return false
					}
					for index := range target.ArchitectureCanvas.Subsystems {
						if string(target.ArchitectureCanvas.Subsystems[index].ID) == id {
							target.ArchitectureCanvas.Subsystems[index].Name = text
							return true
						}
					}
					return false
				},
			); err != nil {
				return nil, err
			}
			if err := add(
				localization.OwnerSubsystem, id, localization.FieldDescription,
				subsystem.Description, protected,
				func(target *ReportData, text string) bool {
					if target.ArchitectureCanvas == nil {
						return false
					}
					for index := range target.ArchitectureCanvas.Subsystems {
						if string(target.ArchitectureCanvas.Subsystems[index].ID) == id {
							target.ArchitectureCanvas.Subsystems[index].Description = text
							return true
						}
					}
					return false
				},
			); err != nil {
				return nil, err
			}
		}
		for _, component := range canvas.Components {
			id := string(component.ID)
			protected := presentationComponentProtectedValues(components[id])
			if err := add(
				localization.OwnerComponent, id, localization.FieldNameText,
				component.Name, protected,
				func(target *ReportData, text string) bool {
					if target.ArchitectureCanvas == nil {
						return false
					}
					for index := range target.ArchitectureCanvas.Components {
						if string(target.ArchitectureCanvas.Components[index].ID) == id {
							target.ArchitectureCanvas.Components[index].Name = text
							return true
						}
					}
					return false
				},
			); err != nil {
				return nil, err
			}
			if err := add(
				localization.OwnerComponent, id, localization.FieldDescription,
				component.Description, protected,
				func(target *ReportData, text string) bool {
					if target.ArchitectureCanvas == nil {
						return false
					}
					for index := range target.ArchitectureCanvas.Components {
						if string(target.ArchitectureCanvas.Components[index].ID) == id {
							target.ArchitectureCanvas.Components[index].Description = text
							return true
						}
					}
					return false
				},
			); err != nil {
				return nil, err
			}
		}
	}

	if story := data.GuidedTour; story != nil {
		candidateID := story.CandidateID
		storyProtected := guidedTourPresentationProtectedValues(data, *story)
		if err := bindings.addAddress(
			"guided_tour/"+candidateID+"/trigger",
			story.Trigger,
			storyProtected,
			func(target *ReportData, text string) bool {
				if target.GuidedTour == nil || target.GuidedTour.CandidateID != candidateID {
					return false
				}
				target.GuidedTour.Trigger = text
				return true
			},
		); err != nil {
			return nil, err
		}
		if err := add(
			localization.OwnerGuidedTour, candidateID, localization.FieldTitle,
			story.Title, storyProtected,
			func(target *ReportData, text string) bool {
				if target.GuidedTour == nil || target.GuidedTour.CandidateID != candidateID {
					return false
				}
				target.GuidedTour.Title = text
				return true
			},
		); err != nil {
			return nil, err
		}
		if err := add(
			localization.OwnerGuidedTour, candidateID, localization.FieldSummary,
			story.Summary, storyProtected,
			func(target *ReportData, text string) bool {
				if target.GuidedTour == nil || target.GuidedTour.CandidateID != candidateID {
					return false
				}
				target.GuidedTour.Summary = text
				return true
			},
		); err != nil {
			return nil, err
		}
		for _, step := range story.Steps {
			stepID := guidedStepLocalizationOwner(candidateID, step.BeatIDs)
			if err := addGuidedStepBindings(bindings, stepID, step, storyProtected); err != nil {
				return nil, err
			}
		}
		for _, gap := range story.GapSummary {
			gapID := guidedGapLocalizationOwner(candidateID, gap.GapIDs)
			if err := add(
				localization.OwnerGuidedGap, gapID, localization.FieldExplanation,
				gap.Explanation, storyProtected,
				func(target *ReportData, text string) bool {
					if target.GuidedTour == nil {
						return false
					}
					for index := range target.GuidedTour.GapSummary {
						current := &target.GuidedTour.GapSummary[index]
						if guidedGapLocalizationOwner(target.GuidedTour.CandidateID, current.GapIDs) == gapID {
							current.Explanation = text
							return true
						}
					}
					return false
				},
			); err != nil {
				return nil, err
			}
			for _, item := range gap.Gaps {
				itemID := item.ID
				itemOwner := "guided_tour/" + candidateID + "/gaps/" + itemID
				if err := bindings.addAddress(
					itemOwner+"/label",
					item.Label,
					storyProtected,
					func(target *ReportData, text string) bool {
						current := findGuidedTourGap(target, candidateID, itemID)
						if current == nil {
							return false
						}
						current.Label = text
						return true
					},
				); err != nil {
					return nil, err
				}
				if err := bindings.addAddress(
					itemOwner+"/detail",
					item.Detail,
					storyProtected,
					func(target *ReportData, text string) bool {
						current := findGuidedTourGap(target, candidateID, itemID)
						if current == nil {
							return false
						}
						current.Detail = text
						return true
					},
				); err != nil {
					return nil, err
				}
			}
		}
	}

	if data.StudyMap != nil {
		studyProtected := append([]localization.ProtectedValue(nil), repositoryProtected...)
		if err := addStudyMapLocalizationBindings(bindings, data.StudyMap, studyProtected); err != nil {
			return nil, err
		}
	}
	if data.IncompleteStudy != nil {
		studyProtected := append([]localization.ProtectedValue(nil), repositoryProtected...)
		for _, direction := range data.IncompleteStudy.Directions {
			if err := addStudyDirectionLocalizationBindings(
				bindings, direction, studyProtected,
			); err != nil {
				return nil, err
			}
		}
	}
	for _, mechanism := range data.UserMechanisms {
		if err := addMechanismLocalizationBindings(bindings, mechanism, nil); err != nil {
			return nil, err
		}
	}
	if err := addRemainingPresentationTextInventory(
		bindings,
		data,
		nil,
	); err != nil {
		return nil, err
	}
	return bindings, nil
}

func findGuidedTourGap(
	data *ReportData,
	candidateID string,
	gapID string,
) *guidedtour.Gap {
	if data == nil || data.GuidedTour == nil ||
		data.GuidedTour.CandidateID != candidateID {
		return nil
	}
	for summaryIndex := range data.GuidedTour.GapSummary {
		for gapIndex := range data.GuidedTour.GapSummary[summaryIndex].Gaps {
			gap := &data.GuidedTour.GapSummary[summaryIndex].Gaps[gapIndex]
			if gap.ID == gapID {
				return gap
			}
		}
	}
	return nil
}

func componentMapByID(
	components []ArchitectureComponent,
) map[componentmap.ComponentID]ArchitectureComponent {
	result := make(map[componentmap.ComponentID]ArchitectureComponent, len(components))
	for _, component := range components {
		result[component.ID] = component
	}
	return result
}

func addGuidedStepBindings(
	bindings *presentationLocalizationBindings,
	ownerID string,
	step guidedtour.StoryStep,
	protected []localization.ProtectedValue,
) error {
	add := func(name localization.FieldName, text string, update func(*guidedtour.StoryStep, string)) error {
		return bindings.add(
			localization.OwnerGuidedStep, ownerID, name, text, protected,
			func(target *ReportData, translated string) bool {
				if target.GuidedTour == nil {
					return false
				}
				for index := range target.GuidedTour.Steps {
					current := &target.GuidedTour.Steps[index]
					if guidedStepLocalizationOwner(target.GuidedTour.CandidateID, current.BeatIDs) == ownerID {
						update(current, translated)
						return true
					}
				}
				return false
			},
		)
	}
	if err := add(localization.FieldTitle, step.Title, func(step *guidedtour.StoryStep, text string) {
		step.Title = text
	}); err != nil {
		return err
	}
	return add(localization.FieldExplanation, step.Explanation, func(step *guidedtour.StoryStep, text string) {
		step.Explanation = text
	})
}

func addStudyMapLocalizationBindings(
	bindings *presentationLocalizationBindings,
	study *RepositoryStudyMap,
	protected []localization.ProtectedValue,
) error {
	briefFields := []struct {
		name localization.FieldName
		text string
		set  func(*RepositoryBrief, string)
	}{
		{localization.FieldWhatItIs, study.Brief.WhatItIs, func(brief *RepositoryBrief, text string) { brief.WhatItIs = text }},
		{localization.FieldProblem, study.Brief.Problem, func(brief *RepositoryBrief, text string) { brief.Problem = text }},
		{localization.FieldMainInput, study.Brief.MainInput, func(brief *RepositoryBrief, text string) { brief.MainInput = text }},
		{localization.FieldCentralResponsibility, study.Brief.CentralResponsibility, func(brief *RepositoryBrief, text string) { brief.CentralResponsibility = text }},
		{localization.FieldObservableResult, study.Brief.ObservableResult, func(brief *RepositoryBrief, text string) { brief.ObservableResult = text }},
	}
	for _, field := range briefFields {
		field := field
		if err := bindings.add(
			localization.OwnerStudyBrief, "repository-brief", field.name, field.text, protected,
			func(target *ReportData, text string) bool {
				if target.StudyMap == nil {
					return false
				}
				field.set(&target.StudyMap.Brief, text)
				return true
			},
		); err != nil {
			return err
		}
	}
	for _, term := range study.Brief.DomainTerms {
		termName := term.Term
		ownerID := "repository-brief:domain-term:" + presentationOwnerDigest(termName)
		termProtected := append(protected, localization.ProtectedValue{
			Kind: localization.ProtectedIdentifier, Value: termName,
		})
		if err := bindings.add(
			localization.OwnerBriefDomainTerm, ownerID, localization.FieldDomainTermMeaning,
			term.Meaning, termProtected,
			func(target *ReportData, text string) bool {
				if target.StudyMap == nil {
					return false
				}
				for index := range target.StudyMap.Brief.DomainTerms {
					if target.StudyMap.Brief.DomainTerms[index].Term == termName {
						target.StudyMap.Brief.DomainTerms[index].Meaning = text
						return true
					}
				}
				return false
			},
		); err != nil {
			return err
		}
	}
	for _, area := range study.Shape {
		if err := addStudyAreaLocalizationBindings(bindings, area, protected); err != nil {
			return err
		}
	}
	for _, direction := range study.Directions {
		if err := addStudyDirectionLocalizationBindings(bindings, direction, protected); err != nil {
			return err
		}
	}
	for _, direction := range study.HiddenDirections {
		if err := addStudyDirectionLocalizationBindings(bindings, direction, protected); err != nil {
			return err
		}
	}
	return nil
}

func addStudyAreaLocalizationBindings(
	bindings *presentationLocalizationBindings,
	area RepositoryStudyArea,
	protected []localization.ProtectedValue,
) error {
	protected = append(protected, studyAreaProtectedValues(area)...)
	add := func(name localization.FieldName, text string, update func(*RepositoryStudyArea, string)) error {
		return bindings.add(
			localization.OwnerConceptualArea, area.ID, name, text, protected,
			func(target *ReportData, translated string) bool {
				found := false
				updateAreas := func(areas []RepositoryStudyArea) {
					for index := range areas {
						if areas[index].ID == area.ID {
							update(&areas[index], translated)
							found = true
						}
					}
				}
				if target.StudyMap != nil {
					updateAreas(target.StudyMap.Shape)
					for index := range target.StudyMap.Directions {
						updateAreas(target.StudyMap.Directions[index].Areas)
					}
					for index := range target.StudyMap.HiddenDirections {
						updateAreas(target.StudyMap.HiddenDirections[index].Areas)
					}
				}
				if target.IncompleteStudy != nil {
					for index := range target.IncompleteStudy.Directions {
						updateAreas(target.IncompleteStudy.Directions[index].Areas)
					}
				}
				return found
			},
		)
	}
	if err := add(localization.FieldNameText, area.Name, func(area *RepositoryStudyArea, text string) {
		area.Name = text
	}); err != nil {
		return err
	}
	return add(localization.FieldResponsibility, area.Responsibility, func(area *RepositoryStudyArea, text string) {
		area.Responsibility = text
	})
}

func addStudyDirectionLocalizationBindings(
	bindings *presentationLocalizationBindings,
	direction StudyDirection,
	protected []localization.ProtectedValue,
) error {
	protected = append(protected, studyDirectionProtectedValues(direction)...)
	addDirection := func(name localization.FieldName, text string, update func(*StudyDirection, string)) error {
		return bindings.add(
			localization.OwnerStudyDirection, direction.ID, name, text, protected,
			func(target *ReportData, translated string) bool {
				found := false
				apply := func(directions []StudyDirection) {
					for index := range directions {
						if directions[index].ID == direction.ID {
							update(&directions[index], translated)
							found = true
						}
					}
				}
				if target.StudyMap != nil {
					apply(target.StudyMap.Directions)
					apply(target.StudyMap.HiddenDirections)
				}
				if target.IncompleteStudy != nil {
					apply(target.IncompleteStudy.Directions)
				}
				return found
			},
		)
	}
	if err := addDirection(localization.FieldQuestion, direction.Question, func(direction *StudyDirection, text string) {
		direction.Question = text
	}); err != nil {
		return err
	}
	if err := addDirection(localization.FieldWhy, direction.WhyItMatters, func(direction *StudyDirection, text string) {
		direction.WhyItMatters = text
	}); err != nil {
		return err
	}
	if err := addDirection(localization.FieldOutcome, direction.LearningOutcome, func(direction *StudyDirection, text string) {
		direction.LearningOutcome = text
	}); err != nil {
		return err
	}
	for _, anchor := range direction.ReadingAnchors {
		ownerID := studyReadingLocalizationOwner(direction.ID, anchor)
		if err := bindings.add(
			localization.OwnerStudyReading, ownerID, localization.FieldWhatToLookFor,
			anchor.WhatToLookFor, protected,
			func(target *ReportData, text string) bool {
				found := false
				apply := func(directions []StudyDirection) {
					for directionIndex := range directions {
						currentDirection := &directions[directionIndex]
						if currentDirection.ID != direction.ID {
							continue
						}
						for anchorIndex := range currentDirection.ReadingAnchors {
							current := &currentDirection.ReadingAnchors[anchorIndex]
							if studyReadingLocalizationOwner(currentDirection.ID, *current) == ownerID {
								current.WhatToLookFor = text
								found = true
							}
						}
					}
				}
				if target.StudyMap != nil {
					apply(target.StudyMap.Directions)
					apply(target.StudyMap.HiddenDirections)
				}
				if target.IncompleteStudy != nil {
					apply(target.IncompleteStudy.Directions)
				}
				return found
			},
		); err != nil {
			return err
		}
	}
	for _, area := range direction.Areas {
		if err := addStudyAreaLocalizationBindings(bindings, area, protected); err != nil {
			return err
		}
	}
	return nil
}

func addMechanismLocalizationBindings(
	bindings *presentationLocalizationBindings,
	mechanism UserMechanism,
	protected []localization.ProtectedValue,
) error {
	protected = append(protected, userMechanismProtectedValues(mechanism)...)
	addMechanism := func(name localization.FieldName, text string, update func(*UserMechanism, string)) error {
		return bindings.add(
			localization.OwnerMechanism, mechanism.ArtifactID, name, text, protected,
			func(target *ReportData, translated string) bool {
				for index := range target.UserMechanisms {
					if target.UserMechanisms[index].ArtifactID == mechanism.ArtifactID {
						update(&target.UserMechanisms[index], translated)
						return true
					}
				}
				return false
			},
		)
	}
	if err := addMechanism(localization.FieldTitle, mechanism.Title, func(mechanism *UserMechanism, text string) {
		mechanism.Title = text
	}); err != nil {
		return err
	}
	if err := addMechanism(localization.FieldQuestion, mechanism.Question, func(mechanism *UserMechanism, text string) {
		mechanism.Question = text
	}); err != nil {
		return err
	}
	if err := addMechanism(localization.FieldAnswer, mechanism.Answer, func(mechanism *UserMechanism, text string) {
		mechanism.Answer = text
	}); err != nil {
		return err
	}
	for _, step := range mechanism.Steps {
		ownerID := mechanism.ArtifactID + ":step:" + userLocationsDigest(step.Locations)
		if err := addMechanismStepLocalizationBindings(
			bindings, localization.OwnerMechanismStep, ownerID,
			mechanism.ArtifactID, step, protected,
		); err != nil {
			return err
		}
	}
	for _, phase := range mechanism.Phases {
		ownerID := mechanism.ArtifactID + ":phase:" + userLocationsDigest(phase.Locations)
		if err := addMechanismPhaseLocalizationBindings(
			bindings, ownerID, mechanism.ArtifactID, phase, protected,
		); err != nil {
			return err
		}
	}
	return nil
}

func studyAreaProtectedValues(area RepositoryStudyArea) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(localization.ProtectedIdentifier, area.ID)
	if area.CodeLocation != nil {
		builder.add(localization.ProtectedPath, area.CodeLocation.Path)
	}
	if area.Source != nil {
		builder.addSource(*area.Source)
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
	return builder.values
}

func studyDirectionProtectedValues(
	direction StudyDirection,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		direction.ID,
		direction.MechanismID,
		string(direction.TargetUserJob),
		string(direction.LearningStage),
	)
	for _, anchor := range direction.PrincipalAnchors {
		builder.add(localization.ProtectedPath, anchor.Path)
		builder.add(localization.ProtectedSymbol, anchor.Symbol)
	}
	for _, anchor := range direction.ReadingAnchors {
		builder.add(localization.ProtectedPath, anchor.Location.Path)
		builder.addSource(anchor.Source)
	}
	for _, document := range direction.Documents {
		builder.add(localization.ProtectedPath, document.Location.Path)
		if document.Source != nil {
			builder.addSource(*document.Source)
		}
	}
	for _, area := range direction.Areas {
		builder.values = append(builder.values, studyAreaProtectedValues(area)...)
	}
	return builder.values
}

func userMechanismProtectedValues(
	mechanism UserMechanism,
) []localization.ProtectedValue {
	var builder objectProtectedValueBuilder
	builder.add(
		localization.ProtectedIdentifier,
		mechanism.ArtifactID,
		string(mechanism.Role),
		string(mechanism.OpportunityKind),
		string(mechanism.TargetUserJob),
	)
	appendStep := func(step UserMechanismStep) {
		for _, location := range step.Locations {
			builder.add(localization.ProtectedPath, location.Path)
		}
		for _, source := range step.Sources {
			builder.addSource(source)
		}
		if step.MapTarget != nil {
			builder.add(
				localization.ProtectedIdentifier,
				string(step.MapTarget.Kind),
				string(step.MapTarget.ComponentID),
				string(step.MapTarget.FlowID),
				step.MapTarget.SurfaceID,
			)
		}
	}
	for _, location := range mechanism.Files {
		builder.add(localization.ProtectedPath, location.Path)
	}
	for _, step := range mechanism.Steps {
		appendStep(step)
	}
	for _, phase := range mechanism.Phases {
		for _, location := range phase.Locations {
			builder.add(localization.ProtectedPath, location.Path)
		}
		for _, source := range phase.Sources {
			builder.addSource(source)
		}
		for _, detail := range phase.ImplementationDetails {
			appendStep(detail)
		}
	}
	for _, context := range mechanism.Context {
		if context.CodeLocation != nil {
			builder.add(localization.ProtectedPath, context.CodeLocation.Path)
		}
		if context.MapTarget != nil {
			builder.add(
				localization.ProtectedIdentifier,
				string(context.MapTarget.Kind),
				string(context.MapTarget.ComponentID),
				string(context.MapTarget.FlowID),
				context.MapTarget.SurfaceID,
			)
		}
	}
	for _, target := range mechanism.ReadNext {
		builder.add(localization.ProtectedPath, target.Path)
		builder.add(localization.ProtectedSymbol, target.Symbol)
	}
	return builder.values
}

func addMechanismStepLocalizationBindings(
	bindings *presentationLocalizationBindings,
	kind localization.OwnerKind,
	ownerID,
	artifactID string,
	step UserMechanismStep,
	protected []localization.ProtectedValue,
) error {
	add := func(name localization.FieldName, text string, update func(*UserMechanismStep, string)) error {
		return bindings.add(
			kind, ownerID, name, text, protected,
			func(target *ReportData, translated string) bool {
				for mechanismIndex := range target.UserMechanisms {
					mechanism := &target.UserMechanisms[mechanismIndex]
					if mechanism.ArtifactID != artifactID {
						continue
					}
					for stepIndex := range mechanism.Steps {
						current := &mechanism.Steps[stepIndex]
						if artifactID+":step:"+userLocationsDigest(current.Locations) == ownerID {
							update(current, translated)
							return true
						}
					}
				}
				return false
			},
		)
	}
	if err := add(localization.FieldTitle, step.Title, func(step *UserMechanismStep, text string) {
		step.Title = text
	}); err != nil {
		return err
	}
	return add(localization.FieldExplanation, step.Explanation, func(step *UserMechanismStep, text string) {
		step.Explanation = text
	})
}

func addMechanismPhaseLocalizationBindings(
	bindings *presentationLocalizationBindings,
	ownerID,
	artifactID string,
	phase UserMechanismPhase,
	protected []localization.ProtectedValue,
) error {
	add := func(name localization.FieldName, text string, update func(*UserMechanismPhase, string)) error {
		return bindings.add(
			localization.OwnerMechanismPhase, ownerID, name, text, protected,
			func(target *ReportData, translated string) bool {
				for mechanismIndex := range target.UserMechanisms {
					mechanism := &target.UserMechanisms[mechanismIndex]
					if mechanism.ArtifactID != artifactID {
						continue
					}
					for phaseIndex := range mechanism.Phases {
						current := &mechanism.Phases[phaseIndex]
						if artifactID+":phase:"+userLocationsDigest(current.Locations) == ownerID {
							update(current, translated)
							return true
						}
					}
				}
				return false
			},
		)
	}
	if err := add(localization.FieldTitle, phase.Title, func(phase *UserMechanismPhase, text string) {
		phase.Title = text
	}); err != nil {
		return err
	}
	if err := add(localization.FieldExplanation, phase.Explanation, func(phase *UserMechanismPhase, text string) {
		phase.Explanation = text
	}); err != nil {
		return err
	}
	for detailIndex, detail := range phase.ImplementationDetails {
		detailOwnerID := ownerID +
			":implementation-detail:" +
			strconv.Itoa(detailIndex) +
			":" +
			userLocationsDigest(detail.Locations)
		if err := addMechanismPhaseDetailLocalizationBindings(
			bindings,
			ownerID,
			detailOwnerID,
			artifactID,
			detailIndex,
			detail,
			protected,
		); err != nil {
			return err
		}
	}
	return nil
}

func addMechanismPhaseDetailLocalizationBindings(
	bindings *presentationLocalizationBindings,
	phaseOwnerID,
	detailOwnerID,
	artifactID string,
	detailIndex int,
	detail UserMechanismStep,
	protected []localization.ProtectedValue,
) error {
	add := func(name localization.FieldName, text string, update func(*UserMechanismStep, string)) error {
		return bindings.add(
			localization.OwnerMechanismStep, detailOwnerID, name, text, protected,
			func(target *ReportData, translated string) bool {
				for mechanismIndex := range target.UserMechanisms {
					mechanism := &target.UserMechanisms[mechanismIndex]
					if mechanism.ArtifactID != artifactID {
						continue
					}
					for phaseIndex := range mechanism.Phases {
						phase := &mechanism.Phases[phaseIndex]
						if artifactID+":phase:"+userLocationsDigest(phase.Locations) != phaseOwnerID {
							continue
						}
						if detailIndex < 0 || detailIndex >= len(phase.ImplementationDetails) {
							return false
						}
						current := &phase.ImplementationDetails[detailIndex]
						currentOwnerID := phaseOwnerID +
							":implementation-detail:" +
							strconv.Itoa(detailIndex) +
							":" +
							userLocationsDigest(current.Locations)
						if currentOwnerID == detailOwnerID {
							update(current, translated)
							return true
						}
						return false
					}
				}
				return false
			},
		)
	}
	if err := add(localization.FieldTitle, detail.Title, func(detail *UserMechanismStep, text string) {
		detail.Title = text
	}); err != nil {
		return err
	}
	return add(localization.FieldExplanation, detail.Explanation, func(detail *UserMechanismStep, text string) {
		detail.Explanation = text
	})
}

func repositoryPresentationProtectedValues(
	data *ReportData,
) []localization.ProtectedValue {
	if data == nil {
		return nil
	}
	var builder objectProtectedValueBuilder
	builder.add(localization.ProtectedProduct, data.RepoName)
	// RepoName may be a module or forge path while model-authored prose uses
	// the repository's display spelling. Derive that spelling from the exact
	// repository path and its canonical project sentence, rather than from a
	// report-wide product-word dictionary.
	base := path.Base(strings.TrimSuffix(data.RepoName, "/"))
	for _, raw := range strings.Fields(data.ProjectGuess) {
		candidate := strings.Trim(raw, "\"'`()[]{}<>,;:!?.")
		if candidate != "" && strings.EqualFold(candidate, base) {
			builder.add(localization.ProtectedProduct, candidate)
			break
		}
	}
	builder.add(localization.ProtectedIdentifier, data.CapturedRevision)
	return builder.values
}

func guidedStepLocalizationOwner(candidateID string, beatIDs []string) string {
	return candidateID + ":step:" + presentationOwnerDigest(sortedStrings(beatIDs)...)
}

func guidedGapLocalizationOwner(candidateID string, gapIDs []string) string {
	return candidateID + ":gap:" + presentationOwnerDigest(sortedStrings(gapIDs)...)
}

func studyReadingLocalizationOwner(directionID string, anchor StudyReadingAnchor) string {
	location := anchor.Location
	return directionID + ":reading:" + presentationOwnerDigest(
		location.Path,
		strconv.Itoa(location.Line),
		strconv.Itoa(location.Column),
		anchor.Source.PresentationSHA256,
	)
}

func userLocationsDigest(locations []UserCodeLocation) string {
	parts := make([]string, 0, len(locations))
	for _, location := range locations {
		parts = append(parts, location.Path+":"+strconv.Itoa(location.Line)+":"+strconv.Itoa(location.Column))
	}
	sort.Strings(parts)
	return presentationOwnerDigest(parts...)
}

func presentationOwnerDigest(parts ...string) string {
	hash := sha256.New()
	hash.Write([]byte(PresentationLocalizationContractVersion))
	for _, part := range parts {
		hash.Write([]byte{0})
		hash.Write([]byte(part))
	}
	return fmt.Sprintf("%x", hash.Sum(nil))[:24]
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func cloneReportData(data *ReportData) (*ReportData, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("report localization: encode canonical report: %w", err)
	}
	var cloned ReportData
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("report localization: clone canonical report: %w", err)
	}
	cloned.evidenceLocations = append(cloned.evidenceLocations, data.evidenceLocations...)
	cloned.sourceSignals = append(cloned.sourceSignals, data.sourceSignals...)
	cloned.studyDocumentSourceRoot = data.studyDocumentSourceRoot
	cloned.standaloneLocalRoots = append([]string(nil), data.standaloneLocalRoots...)
	cloned.externalImports = append(cloned.externalImports, data.externalImports...)
	cloned.repositoryGoFacts = data.repositoryGoFacts
	cloned.repositoryEntrypointFacts = data.repositoryEntrypointFacts
	if len(data.architectureDebugPresentation) > 0 {
		cloned.architectureDebugPresentation = make(
			map[string]string,
			len(data.architectureDebugPresentation),
		)
		for address, text := range data.architectureDebugPresentation {
			cloned.architectureDebugPresentation[address] = text
		}
	}
	cloned.semanticAttempted = data.semanticAttempted
	cloned.semanticInvestigated = data.semanticInvestigated
	cloned.runWarningDiagnostics = append(
		[]runWarningDiagnostic(nil),
		data.runWarningDiagnostics...,
	)
	if data.TaskInvestigation != nil && cloned.TaskInvestigation != nil {
		cloned.TaskInvestigation.warningDiagnostics = append(
			[]tasklens.WarningDiagnostic(nil),
			data.TaskInvestigation.warningDiagnostics...,
		)
	}
	if data.presentationSourceEpisode != nil {
		episodeJSON, err := json.Marshal(data.presentationSourceEpisode)
		if err != nil {
			return nil, fmt.Errorf(
				"report localization: encode source episode presentation: %w",
				err,
			)
		}
		var episode sourceEpisodeProjection
		if err := json.Unmarshal(episodeJSON, &episode); err != nil {
			return nil, fmt.Errorf(
				"report localization: clone source episode presentation: %w",
				err,
			)
		}
		for index := range episode.Claims {
			episode.Claims[index].ID = data.presentationSourceEpisode.Claims[index].ID
		}
		for index := range episode.Uncertainties {
			episode.Uncertainties[index].ID = data.presentationSourceEpisode.Uncertainties[index].ID
		}
		cloned.presentationSourceEpisode = &episode
	}
	return &cloned, nil
}

func localizationBindingJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}
