package report

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	PresentationLocalizationContractVersion = "report-presentation-localization-v8"
	PresentationTextInventoryVersion        = "presentation-text-inventory-v5"
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
	// Report-wide values may protect prose only when their spelling makes the
	// technical identity self-authenticating (a path, URL, acronym, version, or
	// punctuated identifier). Semantic identities with ordinary word spelling
	// are attached to the presentation object that owns them below. This keeps
	// repository or framework names such as "main", "Echo", or "Server" from
	// masking unrelated prose elsewhere in the report.
	globalProtected := globallyUnambiguousPresentationProtectedValues(
		reportLocalizationProtectedValues(data),
	)
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
		data.ProjectGuess, append(repositoryProtected, globalProtected...),
		func(target *ReportData, text string) bool { target.ProjectGuess = text; return true },
	); err != nil {
		return nil, err
	}
	if err := add(
		localization.OwnerRepository, "repository", localization.FieldDocumentedPurpose,
		data.DocumentedPurpose, append(repositoryProtected, globalProtected...),
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
		if err := bindings.addAddress(
			"guided_tour/"+candidateID+"/trigger",
			story.Trigger,
			globalProtected,
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
			story.Title, globalProtected,
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
			story.Summary, globalProtected,
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
			if err := addGuidedStepBindings(bindings, stepID, step, globalProtected); err != nil {
				return nil, err
			}
		}
		for _, gap := range story.GapSummary {
			gapID := guidedGapLocalizationOwner(candidateID, gap.GapIDs)
			if err := add(
				localization.OwnerGuidedGap, gapID, localization.FieldExplanation,
				gap.Explanation, globalProtected,
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
					globalProtected,
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
					globalProtected,
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
		if err := addStudyMapLocalizationBindings(bindings, data.StudyMap, globalProtected); err != nil {
			return nil, err
		}
	}
	if data.IncompleteStudy != nil {
		for _, direction := range data.IncompleteStudy.Directions {
			if err := addStudyDirectionLocalizationBindings(
				bindings, direction, globalProtected,
			); err != nil {
				return nil, err
			}
		}
	}
	for _, mechanism := range data.UserMechanisms {
		if err := addMechanismLocalizationBindings(bindings, mechanism, globalProtected); err != nil {
			return nil, err
		}
	}
	if err := addRemainingPresentationTextInventory(
		bindings,
		data,
		globalProtected,
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

func reportLocalizationProtectedValues(data *ReportData) []localization.ProtectedValue {
	values := []localization.ProtectedValue{
		{Kind: localization.ProtectedProduct, Value: data.RepoName},
		{Kind: localization.ProtectedIdentifier, Value: data.CapturedRevision},
	}
	appendValue := func(kind localization.ProtectedKind, value string) {
		values = append(values, localization.ProtectedValue{Kind: kind, Value: value})
	}
	appendLocation := func(location UserCodeLocation) {
		appendValue(localization.ProtectedPath, location.Path)
	}
	appendSource := func(source SourceSnippet) {
		appendValue(localization.ProtectedPath, source.Path)
		appendValue(localization.ProtectedSymbol, source.EnclosingSymbol)
	}
	appendSurfaceLocation := func(location *SurfaceLocation) {
		if location != nil {
			appendValue(localization.ProtectedPath, location.Path)
		}
	}
	appendSurfaceSymbol := func(symbol SurfaceSymbol) {
		appendValue(localization.ProtectedIdentifier, symbol.ID)
		appendValue(localization.ProtectedPackage, symbol.Package)
		appendValue(localization.ProtectedSymbol, symbol.Name)
		appendSurfaceLocation(symbol.Location)
	}
	appendSurfaceValue := func(value SurfaceValue) {
		appendValue(localization.ProtectedIdentifier, value.Text)
		for _, candidate := range value.Candidates {
			appendValue(localization.ProtectedIdentifier, candidate)
		}
	}
	for _, path := range data.OpenablePaths {
		appendValue(localization.ProtectedPath, path)
	}
	for _, item := range data.FirstFilesToOpen {
		appendValue(localization.ProtectedPath, item.Path)
	}
	for _, item := range data.OrientationUnverifiedPaths {
		appendValue(localization.ProtectedPath, item.Path)
	}
	for _, direction := range data.CandidateDirections {
		appendValue(localization.ProtectedIdentifier, direction.ID)
		appendValue(localization.ProtectedIdentifier, direction.FlowType)
		appendValue(localization.ProtectedSymbol, direction.LikelyEntrypoint)
		appendValue(localization.ProtectedIdentifier, direction.Disposition)
		appendValue(localization.ProtectedIdentifier, direction.CandidateBasis)
		for _, filePath := range direction.LikelyFiles {
			appendValue(localization.ProtectedPath, filePath)
		}
	}
	for _, flow := range data.Flows {
		appendValue(localization.ProtectedIdentifier, flow.ID)
		appendValue(localization.ProtectedIdentifier, flow.FlowType)
		appendValue(localization.ProtectedIdentifier, flow.FlowStatus)
		appendValue(localization.ProtectedIdentifier, flow.CandidateBasis)
		for _, step := range flow.LikelyChain {
			for _, filePath := range step.EvidenceFiles {
				appendValue(localization.ProtectedPath, filePath)
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
				appendValue(localization.ProtectedPath, item.Path)
			}
		}
		for _, item := range flow.UnverifiedPaths {
			appendValue(localization.ProtectedPath, item.Path)
		}
		for _, packagePath := range flow.BundlePackages {
			appendValue(localization.ProtectedPackage, packagePath)
		}
		for _, edge := range flow.BundleEdges {
			appendValue(localization.ProtectedPackage, edge.From)
			appendValue(localization.ProtectedPackage, edge.To)
		}
	}
	if data.RepositoryGraph != nil {
		for _, module := range data.RepositoryGraph.Modules {
			appendValue(localization.ProtectedIdentifier, module.ID)
			appendValue(localization.ProtectedModule, module.Path)
			appendValue(localization.ProtectedPath, module.Dir)
			appendValue(localization.ProtectedModule, module.DisplayName)
		}
		for _, packageInfo := range data.RepositoryGraph.Packages {
			appendValue(localization.ProtectedPackage, packageInfo.CanonicalPath)
			appendValue(localization.ProtectedIdentifier, packageInfo.Name)
			appendValue(localization.ProtectedIdentifier, packageInfo.ModuleID)
			appendValue(localization.ProtectedModule, packageInfo.ModulePath)
			appendValue(localization.ProtectedPath, packageInfo.Dir)
			appendValue(localization.ProtectedPath, packageInfo.ModuleRelativeDir)
			appendValue(localization.ProtectedPackage, packageInfo.DisplayPath)
			for _, path := range packageInfo.Files {
				appendValue(localization.ProtectedPath, path)
			}
		}
	}
	for _, word := range data.ImportantDomainWords {
		appendValue(localization.ProtectedIdentifier, word.Word)
	}
	for _, component := range data.Components {
		appendValue(localization.ProtectedIdentifier, component.ID)
		appendValue(localization.ProtectedPackage, component.PrimaryPackage)
		for _, packagePath := range component.Packages {
			appendValue(localization.ProtectedPackage, packagePath)
		}
		for _, group := range component.AnchorGroups {
			appendValue(localization.ProtectedIdentifier, group.ID)
			appendValue(localization.ProtectedPath, group.Path)
			for _, location := range group.Locations {
				appendValue(localization.ProtectedPath, location.Path)
			}
		}
	}
	if data.ArchitectureCanvas != nil {
		for _, anchor := range data.ArchitectureCanvas.BehaviorAnchors {
			appendValue(localization.ProtectedIdentifier, anchor.ID)
			appendValue(localization.ProtectedPath, anchor.Location.Path)
			for _, memberID := range anchor.MemberIDs {
				appendValue(localization.ProtectedIdentifier, memberID.Value)
			}
		}
		for _, subsystem := range data.ArchitectureCanvas.Subsystems {
			appendValue(localization.ProtectedIdentifier, string(subsystem.ID))
			for _, componentID := range subsystem.ComponentIDs {
				appendValue(localization.ProtectedIdentifier, string(componentID))
			}
		}
		for _, component := range data.ArchitectureCanvas.Components {
			values = append(values, presentationComponentProtectedValues(component)...)
		}
		for _, surface := range data.ArchitectureCanvas.Surfaces {
			appendValue(localization.ProtectedIdentifier, surface.ID)
			appendValue(localization.ProtectedIdentifier, surface.Source)
			appendValue(localization.ProtectedIdentifier, surface.Kind)
			appendValue(localization.ProtectedIdentifier, surface.Category)
			appendValue(localization.ProtectedPackage, surface.OwningExecutable)
			appendValue(localization.ProtectedIdentifier, string(surface.OwningComponentID))
			appendValue(localization.ProtectedIdentifier, string(surface.RelatedTraceID))
			for _, componentID := range surface.ParticipatingComponentIDs {
				appendValue(localization.ProtectedIdentifier, string(componentID))
			}
			for index := range surface.Evidence {
				appendSurfaceLocation(&surface.Evidence[index])
			}
		}
		for _, suggestion := range data.ArchitectureCanvas.Suggestions {
			appendValue(localization.ProtectedIdentifier, suggestion.ID)
			for _, reference := range suggestion.EvidenceReferences {
				appendValue(localization.ProtectedIdentifier, reference)
			}
			for _, anchorID := range suggestion.RelevantAnchorIDs {
				appendValue(localization.ProtectedIdentifier, anchorID)
			}
			for _, componentID := range suggestion.RelevantComponentIDs {
				appendValue(localization.ProtectedIdentifier, string(componentID))
			}
			if suggestion.StartLocation != nil {
				appendSurfaceLocation(suggestion.StartLocation)
			}
		}
		for _, flow := range data.ArchitectureCanvas.Flows {
			appendValue(localization.ProtectedIdentifier, string(flow.ID))
			appendValue(localization.ProtectedIdentifier, string(flow.Archetype))
			appendValue(localization.ProtectedIdentifier, flow.Command)
			appendValue(localization.ProtectedIdentifier, flow.Status)
			appendValue(localization.ProtectedIdentifier, flow.EvidenceBasis)
			appendValue(localization.ProtectedIdentifier, flow.StartSurfaceID)
			appendValue(localization.ProtectedIdentifier, flow.SeedSurfaceID)
			for _, step := range flow.Steps {
				appendValue(localization.ProtectedIdentifier, step.ID)
				appendValue(localization.ProtectedSymbol, step.QualifiedName)
				appendValue(localization.ProtectedIdentifier, step.BranchID)
				appendValue(localization.ProtectedIdentifier, string(step.ComponentID))
				if step.Location != nil {
					appendValue(localization.ProtectedPath, step.Location.Path)
				}
			}
			for _, componentID := range flow.ParticipatingComponentIDs {
				appendValue(localization.ProtectedIdentifier, string(componentID))
			}
		}
		for _, frontier := range data.ArchitectureCanvas.Frontiers {
			appendValue(localization.ProtectedIdentifier, frontier.ID)
			appendValue(localization.ProtectedIdentifier, string(frontier.FlowID))
			appendValue(localization.ProtectedIdentifier, frontier.Kind)
			appendValue(localization.ProtectedIdentifier, frontier.AnchorID)
			appendValue(localization.ProtectedIdentifier, frontier.TransitionID)
			if frontier.Evidence != nil {
				appendValue(localization.ProtectedPath, frontier.Evidence.Path)
			}
		}
		for _, diagnostic := range data.ArchitectureCanvas.Diagnostics {
			appendValue(localization.ProtectedIdentifier, diagnostic.ID)
			appendValue(localization.ProtectedIdentifier, diagnostic.Source)
			appendValue(localization.ProtectedIdentifier, diagnostic.Code)
			appendValue(localization.ProtectedIdentifier, string(diagnostic.FlowID))
			if diagnostic.Member != nil {
				appendValue(
					protectedMemberKind(diagnostic.Member.Kind),
					diagnostic.Member.Value,
				)
			}
		}
	}
	if data.GuidedTour != nil {
		appendValue(localization.ProtectedIdentifier, data.GuidedTour.CandidateID)
		for _, step := range data.GuidedTour.Steps {
			for _, beatID := range step.BeatIDs {
				appendValue(localization.ProtectedIdentifier, beatID)
			}
			for _, componentID := range step.ComponentIDs {
				appendValue(localization.ProtectedIdentifier, componentID)
			}
			for _, surfaceID := range step.SurfaceIDs {
				appendValue(localization.ProtectedIdentifier, surfaceID)
			}
			for _, flowID := range step.FlowIDs {
				appendValue(localization.ProtectedIdentifier, flowID)
			}
			for _, flowStepID := range step.FlowStepIDs {
				appendValue(localization.ProtectedIdentifier, flowStepID)
			}
			for _, reference := range step.Evidence {
				appendValue(localization.ProtectedIdentifier, reference.ID)
				if reference.Location != nil {
					appendValue(localization.ProtectedPath, reference.Location.Path)
				}
			}
		}
	}
	if data.StudyMap != nil {
		for _, term := range data.StudyMap.Brief.DomainTerms {
			appendValue(localization.ProtectedIdentifier, term.Term)
		}
		appendDirectionValues := func(directions []StudyDirection) {
			for _, direction := range directions {
				appendValue(localization.ProtectedIdentifier, direction.ID)
				for _, anchor := range direction.PrincipalAnchors {
					appendValue(localization.ProtectedPath, anchor.Path)
					appendValue(localization.ProtectedSymbol, anchor.Symbol)
				}
				for _, anchor := range direction.ReadingAnchors {
					appendLocation(anchor.Location)
					appendSource(anchor.Source)
				}
				for _, document := range direction.Documents {
					appendLocation(document.Location)
					if document.Source != nil {
						appendSource(*document.Source)
					}
				}
			}
		}
		appendDirectionValues(data.StudyMap.Directions)
		appendDirectionValues(data.StudyMap.HiddenDirections)
	}
	if data.IncompleteStudy != nil {
		for _, direction := range data.IncompleteStudy.Directions {
			appendValue(localization.ProtectedIdentifier, direction.ID)
			for _, anchor := range direction.PrincipalAnchors {
				appendValue(localization.ProtectedPath, anchor.Path)
				appendValue(localization.ProtectedSymbol, anchor.Symbol)
			}
			for _, anchor := range direction.ReadingAnchors {
				appendLocation(anchor.Location)
				appendSource(anchor.Source)
			}
			for _, document := range direction.Documents {
				appendLocation(document.Location)
				if document.Source != nil {
					appendSource(*document.Source)
				}
			}
		}
	}
	for _, mechanism := range data.UserMechanisms {
		appendValue(localization.ProtectedIdentifier, mechanism.ArtifactID)
		for _, location := range mechanism.Files {
			appendLocation(location)
		}
		for _, step := range mechanism.Steps {
			for _, location := range step.Locations {
				appendLocation(location)
			}
			for _, source := range step.Sources {
				appendSource(source)
			}
		}
		for _, phase := range mechanism.Phases {
			for _, location := range phase.Locations {
				appendLocation(location)
			}
			for _, source := range phase.Sources {
				appendSource(source)
			}
			for _, detail := range phase.ImplementationDetails {
				for _, location := range detail.Locations {
					appendLocation(location)
				}
				for _, source := range detail.Sources {
					appendSource(source)
				}
			}
		}
		for _, context := range mechanism.Context {
			if context.CodeLocation != nil {
				appendLocation(*context.CodeLocation)
			}
		}
		for _, target := range mechanism.ReadNext {
			appendValue(localization.ProtectedPath, target.Path)
			appendValue(localization.ProtectedSymbol, target.Symbol)
		}
	}
	if data.Operations != nil {
		appendReference := func(reference OperationalReference) {
			appendLocation(reference.Location)
			appendSource(reference.Source)
			appendValue(localization.ProtectedIdentifier, reference.Role)
		}
		for _, path := range data.Operations.Paths {
			appendValue(localization.ProtectedIdentifier, path.ID)
			for _, relatedID := range path.RelatedStudyIDs {
				appendValue(localization.ProtectedIdentifier, relatedID)
			}
			for _, reference := range path.Prerequisites {
				appendReference(reference)
			}
			for _, action := range path.Actions {
				appendValue(localization.ProtectedIdentifier, action.Command)
				appendValue(localization.ProtectedIdentifier, action.CopyText)
				appendValue(localization.ProtectedURL, action.Endpoint)
				appendReference(action.Reference)
			}
			for _, result := range path.ExpectedResults {
				appendValue(localization.ProtectedIdentifier, string(result.Kind))
				appendValue(localization.ProtectedIdentifier, result.Value)
				for _, evidenceID := range result.ResultEvidenceIDs {
					appendValue(localization.ProtectedIdentifier, evidenceID)
				}
				appendReference(result.Reference)
			}
			for _, reference := range path.Expected {
				appendReference(reference)
			}
			for _, reference := range path.Troubleshooting {
				appendReference(reference)
			}
		}
		for _, landmark := range data.Operations.Landmarks {
			appendValue(localization.ProtectedIdentifier, landmark.ID)
			appendValue(localization.ProtectedIdentifier, landmark.Role)
			appendValue(localization.ProtectedIdentifier, landmark.Command)
			appendValue(localization.ProtectedIdentifier, landmark.CopyText)
			appendValue(localization.ProtectedURL, landmark.Endpoint)
			appendReference(landmark.Reference)
		}
	}
	if catalog := data.DiscoveredSurfaces; catalog != nil {
		appendValue(localization.ProtectedIdentifier, catalog.AnalyzerVersion)
		appendValue(localization.ProtectedIdentifier, catalog.ScenarioID)
		for _, entrypoint := range catalog.EntrypointsConsidered {
			appendSurfaceSymbol(entrypoint)
		}
		for _, seed := range catalog.ConfiguredSeedsMatched {
			appendValue(localization.ProtectedIdentifier, seed)
		}
		for _, trigger := range catalog.Triggers {
			appendValue(localization.ProtectedIdentifier, trigger.ID)
			appendValue(localization.ProtectedIdentifier, trigger.Kind)
			appendValue(localization.ProtectedIdentifier, trigger.Producer)
			appendValue(localization.ProtectedProtocol, trigger.Transport)
			appendValue(localization.ProtectedProduct, trigger.Framework)
			appendSurfaceSymbol(trigger.ProcessEntrypoint)
			appendSurfaceValue(trigger.Dispatcher)
			appendSurfaceSymbol(trigger.Constructor)
			appendSurfaceLocation(trigger.RegistrationSite)
			appendSurfaceLocation(trigger.DescriptorSite)
			appendSurfaceLocation(trigger.ServerStartSite)
			appendSurfaceValue(trigger.Handler)
			appendSurfaceLocation(trigger.HandlerLocation)
			for _, middleware := range trigger.Middleware {
				appendSurfaceValue(middleware)
			}
			for _, wrapper := range trigger.WrapperChain {
				appendSurfaceSymbol(wrapper.Symbol)
				appendSurfaceLocation(wrapper.Callsite)
				appendValue(localization.ProtectedIdentifier, wrapper.Origin)
			}
			appendValue(localization.ProtectedIdentifier, trigger.FinalSeed)
			appendValue(localization.ProtectedIdentifier, trigger.DiscoveryBasis)
			appendValue(localization.ProtectedIdentifier, trigger.OwningExecutable)
			appendValue(localization.ProtectedIdentifier, string(trigger.OwningComponentID))
			appendValue(localization.ProtectedIdentifier, string(trigger.RelatedTraceID))
			for _, componentID := range trigger.ParticipatingComponentIDs {
				appendValue(localization.ProtectedIdentifier, string(componentID))
			}
			for _, evidence := range trigger.Evidence {
				appendValue(localization.ProtectedIdentifier, evidence.ID)
				appendValue(localization.ProtectedIdentifier, evidence.Kind)
				appendSurfaceLocation(evidence.Location)
			}
			for _, frontier := range trigger.DynamicFrontier {
				appendValue(localization.ProtectedIdentifier, frontier.Kind)
				appendSurfaceLocation(frontier.Location)
			}
		}
		for _, signal := range catalog.LoopSignals {
			appendValue(localization.ProtectedIdentifier, signal.Kind)
			appendValue(localization.ProtectedSymbol, signal.FunctionID)
			appendValue(localization.ProtectedIdentifier, signal.TerminalSeed)
			appendSurfaceLocation(signal.Location)
		}
		for _, collection := range [][]SurfaceFrontier{
			catalog.DynamicFrontiers,
			catalog.UnsupportedDispatch,
		} {
			for _, frontier := range collection {
				appendValue(localization.ProtectedIdentifier, frontier.Kind)
				appendSurfaceLocation(frontier.Location)
			}
		}
		for _, diagnostic := range catalog.PackageDiagnostics {
			appendValue(localization.ProtectedIdentifier, diagnostic.ID)
			appendValue(localization.ProtectedIdentifier, diagnostic.Kind)
			appendValue(localization.ProtectedPackage, diagnostic.Package)
			appendValue(localization.ProtectedPackage, diagnostic.PackageName)
			appendValue(localization.ProtectedPackage, diagnostic.OwningExecutable)
			appendSurfaceLocation(diagnostic.Location)
		}
		for _, unavailable := range catalog.UnavailablePackages {
			appendValue(localization.ProtectedPackage, unavailable.Package)
			appendValue(localization.ProtectedPackage, unavailable.PackageName)
			appendValue(localization.ProtectedPackage, unavailable.OwningExecutable)
			for _, diagnosticID := range unavailable.DiagnosticIDs {
				appendValue(localization.ProtectedIdentifier, diagnosticID)
			}
		}
	}
	if episode := data.presentationSourceEpisode; episode != nil {
		appendValue(localization.ProtectedIdentifier, episode.EpisodeID)
		appendValue(localization.ProtectedProduct, episode.Repository)
		appendValue(localization.ProtectedIdentifier, episode.Revision)
		for _, claim := range episode.Claims {
			appendValue(localization.ProtectedIdentifier, claim.ID)
			for _, source := range claim.Sources {
				appendValue(localization.ProtectedPath, source.Path)
			}
		}
		for _, gap := range episode.Uncertainties {
			appendValue(localization.ProtectedIdentifier, gap.ID)
			for _, source := range gap.Sources {
				appendValue(localization.ProtectedPath, source.Path)
			}
		}
	}
	if data.GitLabSourceLinks != nil {
		appendValue(localization.ProtectedURL, data.GitLabSourceLinks.RepositoryURL)
	}
	if data.GitHubSourceLinks != nil {
		appendValue(localization.ProtectedURL, data.GitHubSourceLinks.RepositoryURL)
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]localization.ProtectedValue, 0, len(values))
	for _, value := range values {
		if value.Value == "" {
			continue
		}
		if _, exists := seen[value.Value]; exists {
			continue
		}
		seen[value.Value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func globallyUnambiguousPresentationProtectedValues(
	values []localization.ProtectedValue,
) []localization.ProtectedValue {
	result := make([]localization.ProtectedValue, 0, len(values))
	for _, value := range values {
		switch value.Kind {
		case localization.ProtectedPath,
			localization.ProtectedURL:
			result = append(result, value)
		default:
			if isGloballyUnambiguousTechnicalSpelling(value.Value) {
				result = append(result, value)
			}
		}
	}
	return result
}

func isGloballyUnambiguousTechnicalSpelling(value string) bool {
	if strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	asciiLetterIndex := 0
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
			return true
		case character >= 'A' && character <= 'Z':
			if asciiLetterIndex > 0 {
				return true
			}
			asciiLetterIndex++
		case character >= 'a' && character <= 'z':
			asciiLetterIndex++
		case character == ' ':
			// A plain multi-word phrase is not an opaque identity merely
			// because it appears elsewhere in the report.
		default:
			return true
		}
	}
	return false
}

func repositoryPresentationProtectedValues(
	data *ReportData,
) []localization.ProtectedValue {
	if data == nil {
		return nil
	}
	var builder objectProtectedValueBuilder
	builder.add(localization.ProtectedProduct, data.RepoName)
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
