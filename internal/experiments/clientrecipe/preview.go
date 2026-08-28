package clientrecipe

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"sort"
	"strings"
)

const PreviewVersion = 1

//go:embed preview_template.html
var previewTemplateText string

//go:embed preview_style.css
var previewStyle string

//go:embed preview_script.js
var previewScript string

type PreviewModel struct {
	Version  int                `json:"version"`
	Target   PreviewTarget      `json:"target"`
	Scope    PreviewScope       `json:"scope"`
	Summary  PreviewSummary     `json:"summary"`
	Tasks    []PreviewTask      `json:"tasks"`
	Roles    []PreviewRole      `json:"roles"`
	Steps    []PreviewStep      `json:"steps"`
	Examples []PreviewExample   `json:"examples"`
	Audit    []PreviewExclusion `json:"audit"`
}

type PreviewTarget struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Kind     string `json:"kind"`
}

type PreviewScope struct {
	Evidence       string `json:"evidence"`
	Generalization string `json:"generalization"`
}

type PreviewSummary struct {
	Observed               int `json:"observed"`
	Boundaries             int `json:"boundaries"`
	Complete               int `json:"complete"`
	Excluded               int `json:"excluded"`
	ObservedUniversalRoles int `json:"observed_universal_roles"`
	ObservedCommonRoles    int `json:"observed_common_roles"`
	CallbackClosed         int `json:"callback_closed"`
	CallbackFrontier       int `json:"callback_frontier"`
}

type PreviewTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
}

type PreviewRole struct {
	ID                string `json:"id"`
	Label             string `json:"label"`
	TaskRequired      bool   `json:"task_required"`
	ObservedComplete  int    `json:"observed_complete"`
	CompleteExamples  int    `json:"complete_examples"`
	ObservedNecessity string `json:"observed_necessity"`
}

type PreviewEvidence struct {
	ID         string `json:"id"`
	ExampleID  string `json:"example_id"`
	Example    string `json:"example"`
	Role       string `json:"role"`
	Locator    string `json:"locator"`
	Path       string `json:"path"`
	Line       int    `json:"line"`
	Symbol     string `json:"symbol"`
	Authority  string `json:"authority"`
	Provenance string `json:"provenance"`
	SourceHref string `json:"source_href"`
}

type PreviewStep struct {
	ID               string            `json:"id"`
	Number           int               `json:"number"`
	Title            string            `json:"title"`
	Purpose          string            `json:"purpose"`
	Roles            []PreviewRole     `json:"roles"`
	CoveredExamples  []string          `json:"covered_examples"`
	Evidence         []PreviewEvidence `json:"evidence"`
	EvidenceCount    int               `json:"evidence_count"`
	CompleteCoverage int               `json:"complete_coverage"`
	PartialCoverage  int               `json:"partial_coverage"`
}

type PreviewSlot struct {
	StepID       string            `json:"step_id"`
	Title        string            `json:"title"`
	Status       string            `json:"status"`
	Roles        []PreviewRole     `json:"roles"`
	CoveredRoles []PreviewRole     `json:"covered_roles"`
	Missing      []string          `json:"missing"`
	Evidence     []PreviewEvidence `json:"evidence"`
}

type PreviewExample struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Summary          string        `json:"summary"`
	Status           string        `json:"status"`
	Complete         bool          `json:"complete"`
	MostComplete     bool          `json:"most_complete"`
	VerificationKind string        `json:"verification_kind"`
	Missing          []string      `json:"missing"`
	EvidenceCount    int           `json:"evidence_count"`
	RoleCoverage     int           `json:"role_coverage"`
	Slots            []PreviewSlot `json:"slots"`
}

type PreviewExclusion struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	Reason  string `json:"reason"`
	Locator string `json:"locator"`
}

type previewTemplateData struct {
	ModelJSON template.JS
	Style     template.CSS
	Script    template.JS
}

func BuildPreviewModel(h1 H1Result, h2 H2Result, evaluation EvaluationResult) (PreviewModel, error) {
	if err := h1.Validate(); err != nil {
		return PreviewModel{}, err
	}
	if err := h2.ValidateAgainst(h1); err != nil {
		return PreviewModel{}, err
	}
	if err := evaluation.Validate(); err != nil {
		return PreviewModel{}, err
	}
	if evaluation.Verdict != EvaluationPass || evaluation.H1.Verdict != EvaluationPass ||
		evaluation.H1SHA256 != h1.SHA256 || evaluation.H0SHA256 != h1.H0SHA256 {
		return PreviewModel{}, fmt.Errorf("client recipe preview: evaluation/H1 binding or verdict mismatch")
	}
	if len(h1.Instances) != 4 || len(h1.Excluded) != 6 {
		return PreviewModel{}, fmt.Errorf("client recipe preview: experiment accounting changed")
	}

	completeExamples := 0
	for _, instance := range h1.Instances {
		if instance.Complete {
			completeExamples++
		}
	}
	roles, roleByID := previewRoles(h1, completeExamples)

	stepCopy := make(map[string]H2StepCopy, len(h2.Steps))
	for _, row := range h2.Steps {
		stepCopy[row.Ref] = row
	}
	exampleCopy := make(map[string]H2ExampleCopy, len(h2.Examples))
	for _, row := range h2.Examples {
		exampleCopy[row.Ref] = row
	}
	definitions := h2Examples(h1)
	mostCompleteIDs := previewMostCompleteExamples(h1, definitions)
	examples := make([]PreviewExample, 0, len(definitions))
	for _, definition := range definitions {
		example := buildPreviewExample(definition, stepCopy, exampleCopy[definition.Ref], roleByID)
		_, example.MostComplete = mostCompleteIDs[definition.Ref]
		examples = append(examples, example)
	}
	sort.SliceStable(examples, func(i, j int) bool {
		if examples[i].MostComplete != examples[j].MostComplete {
			return examples[i].MostComplete
		}
		if examples[i].Complete != examples[j].Complete {
			return examples[i].Complete
		}
		return examples[i].Name < examples[j].Name
	})

	required, common := 0, 0
	for _, row := range h1.Roles {
		switch row.Necessity {
		case H1Required:
			required++
		case H1Common:
			common++
		}
	}
	steps := make([]PreviewStep, 0, len(h2StepDefinitions))
	for index, definition := range h2StepDefinitions {
		copyRow := stepCopy[definition.Ref]
		step := PreviewStep{
			ID: definition.Ref, Number: index + 1, Title: copyRow.Title, Purpose: copyRow.Purpose,
			Roles: []PreviewRole{}, CoveredExamples: []string{}, Evidence: []PreviewEvidence{},
		}
		for _, role := range definition.Roles {
			step.Roles = append(step.Roles, roleByID[role])
		}
		for _, example := range examples {
			slot := previewSlot(example, definition.Ref)
			switch slot.Status {
			case "covered":
				step.CompleteCoverage++
				step.CoveredExamples = append(step.CoveredExamples, example.Name)
			case "partial":
				step.PartialCoverage++
			}
			step.Evidence = append(step.Evidence, slot.Evidence...)
		}
		step.Evidence = uniquePreviewEvidence(step.Evidence)
		step.EvidenceCount = len(step.Evidence)
		steps = append(steps, step)
	}

	audit := make([]PreviewExclusion, 0, len(h1.Excluded))
	for index, excluded := range h1.Excluded {
		first := excluded.Evidence[0]
		audit = append(audit, PreviewExclusion{
			ID: fmt.Sprintf("a%d", index+1), Kind: previewKind(excluded.Kind), Label: first.Symbol,
			Reason: previewExclusionReason(excluded.Reason), Locator: fmt.Sprintf("%s:%d", first.Path, first.Line),
		})
	}
	model := PreviewModel{
		Version: PreviewVersion,
		Target:  PreviewTarget{Name: "launch-service", Language: "Go", Kind: "Service"},
		Scope:   PreviewScope{Evidence: "Controlled fixture only", Generalization: "Generalization not established"},
		Summary: PreviewSummary{
			Observed: h1.Ledger.Observed, Boundaries: h1.Ledger.Admitted, Complete: completeExamples,
			Excluded: h1.Ledger.Excluded, ObservedUniversalRoles: required, ObservedCommonRoles: common,
			CallbackClosed: h1.Callbacks.Closed, CallbackFrontier: h1.Callbacks.Frontier,
		},
		Tasks: []PreviewTask{
			{ID: "add_client", Title: "Add an external client", Description: "Follow the repository's proven boundary shape from configuration to verification.", Available: true},
		},
		Roles: roles, Steps: steps, Examples: examples, Audit: audit,
	}
	return model, nil
}

func RenderClientRecipePreview(h1 H1Result, h2 H2Result, evaluation EvaluationResult) ([]byte, error) {
	model, err := BuildPreviewModel(h1, h2, evaluation)
	if err != nil {
		return nil, err
	}
	modelRaw, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("client recipe preview: encode model: %w", err)
	}
	tmpl, err := template.New("client-recipe-preview").Parse(previewTemplateText)
	if err != nil {
		return nil, fmt.Errorf("client recipe preview: parse template: %w", err)
	}
	var output bytes.Buffer
	err = tmpl.Execute(&output, previewTemplateData{
		ModelJSON: template.JS(modelRaw), Style: template.CSS(previewStyle), Script: template.JS(previewScript),
	})
	if err != nil {
		return nil, fmt.Errorf("client recipe preview: execute template: %w", err)
	}
	return output.Bytes(), nil
}

func buildPreviewExample(
	definition h2ExampleDefinition,
	stepCopy map[string]H2StepCopy,
	copyRow H2ExampleCopy,
	roleByID map[H1Role]PreviewRole,
) PreviewExample {
	instance := definition.Instance
	roleRows := make(map[H1Role]H1RoleEvidence, len(instance.Roles))
	for _, row := range instance.Roles {
		roleRows[row.Role] = row
	}
	example := PreviewExample{
		ID: definition.Ref, Name: definition.Name, Summary: copyRow.Summary,
		Complete: instance.Complete, VerificationKind: previewVerification(instance.VerificationKind),
		Missing: []string{}, RoleCoverage: len(instance.Roles), Slots: []PreviewSlot{},
	}
	if instance.Complete {
		example.Status = "Complete"
	} else {
		example.Status = fmt.Sprintf("Needs %d roles", len(instance.Missing))
	}
	for _, role := range instance.Missing {
		example.Missing = append(example.Missing, previewRoleLabel(role))
	}
	for _, stepDefinition := range h2StepDefinitions {
		slot := PreviewSlot{
			StepID: stepDefinition.Ref, Title: stepCopy[stepDefinition.Ref].Title, Status: "covered",
			Roles: []PreviewRole{}, CoveredRoles: []PreviewRole{}, Missing: []string{}, Evidence: []PreviewEvidence{},
		}
		for _, role := range stepDefinition.Roles {
			projectedRole := roleByID[role]
			slot.Roles = append(slot.Roles, projectedRole)
			row, present := roleRows[role]
			if !present {
				slot.Missing = append(slot.Missing, previewRoleLabel(role))
				continue
			}
			slot.CoveredRoles = append(slot.CoveredRoles, projectedRole)
			for _, evidence := range row.Evidence {
				slot.Evidence = append(slot.Evidence, previewEvidence(definition.Ref, definition.Name, role, evidence))
				example.EvidenceCount++
			}
		}
		switch {
		case len(slot.Missing) == 0:
			slot.Status = "covered"
		case len(slot.CoveredRoles) != 0:
			slot.Status = "partial"
		default:
			slot.Status = "missing"
		}
		example.Slots = append(example.Slots, slot)
	}
	return example
}

func previewMostCompleteExamples(h1 H1Result, definitions []h2ExampleDefinition) map[string]struct{} {
	necessities := make(map[H1Role]H1Necessity, len(h1.Roles))
	for _, row := range h1.Roles {
		necessities[row.Role] = row.Necessity
	}
	best := make(map[string]struct{})
	bestScore := -1
	for _, definition := range definitions {
		if !definition.Instance.Complete {
			continue
		}
		score := 0
		for _, row := range definition.Instance.Roles {
			if necessities[row.Role] == H1Required || necessities[row.Role] == H1Common {
				score++
			}
		}
		switch {
		case score > bestScore:
			clear(best)
			best[definition.Ref] = struct{}{}
			bestScore = score
		case score == bestScore:
			best[definition.Ref] = struct{}{}
		}
	}
	return best
}

func previewRoles(h1 H1Result, completeExamples int) ([]PreviewRole, map[H1Role]PreviewRole) {
	roles := make([]PreviewRole, 0, len(h1.Roles))
	byID := make(map[H1Role]PreviewRole, len(h1.Roles))
	for _, frequency := range h1.Roles {
		_, taskRequired := h1MandatoryRoles[frequency.Role]
		role := PreviewRole{
			ID: string(frequency.Role), Label: previewRoleLabel(frequency.Role), TaskRequired: taskRequired,
			ObservedComplete: frequency.CompleteInstances, CompleteExamples: completeExamples,
			ObservedNecessity: previewObservedNecessity(frequency.Necessity),
		}
		roles = append(roles, role)
		byID[frequency.Role] = role
	}
	return roles, byID
}

func previewSlot(example PreviewExample, stepID string) PreviewSlot {
	for _, slot := range example.Slots {
		if slot.StepID == stepID {
			return slot
		}
	}
	return PreviewSlot{}
}

func previewEvidence(exampleID, exampleName string, role H1Role, evidence H1Evidence) PreviewEvidence {
	authority := "Local syntax"
	provenance := "AST-backed source fact"
	switch {
	case strings.HasPrefix(evidence.AuthorityID, "program-object-"):
		authority, provenance = "Typed object", "ProgramIndex object authority"
	case strings.HasPrefix(evidence.AuthorityID, "program-relation-"):
		authority, provenance = "Typed relation", "ProgramIndex relation authority"
	}
	return PreviewEvidence{
		ID: fmt.Sprintf("%s-%s-%d", exampleID, role, evidence.Line), ExampleID: exampleID, Example: exampleName,
		Role: previewRoleLabel(role), Locator: fmt.Sprintf("%s:%d", evidence.Path, evidence.Line),
		Path: evidence.Path, Line: evidence.Line, Symbol: evidence.Symbol,
		Authority: authority, Provenance: provenance,
		SourceHref: "../repo/" + previewEscapePath(evidence.Path) + fmt.Sprintf("#L%d", evidence.Line),
	}
}

func previewEscapePath(value string) string {
	parts := strings.Split(value, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func previewRoleLabel(role H1Role) string {
	labels := map[H1Role]string{
		H1RoleConfiguration: "Configuration", H1RoleConstruction: "Construction",
		H1RoleLocalWrapper: "Local wrapper", H1RoleConsumerBoundary: "Consumer boundary",
		H1RoleApplicationWiring: "Application wiring", H1RoleProductionOperation: "Production operation",
		H1RoleVerification: "Verification", H1RoleObservability: "Observability",
		H1RoleFailurePolicy: "Failure policy",
	}
	return labels[role]
}

func previewObservedNecessity(value H1Necessity) string {
	switch value {
	case H1Required:
		return "Observed in all"
	case H1Common:
		return "Common pattern"
	default:
		return "Occasional pattern"
	}
}

func previewVerification(value string) string {
	switch value {
	case "unit_test":
		return "Unit verified"
	case "integration_test":
		return "Integration verified"
	default:
		return "Not verified"
	}
}

func previewKind(value string) string {
	switch value {
	case "external_dependency":
		return "External dependency"
	case "test_type":
		return "Test double"
	case "stdlib_helper":
		return "Standard library helper"
	case "prose":
		return "Repository prose"
	default:
		return value
	}
}

func previewExclusionReason(value H1ExclusionReason) string {
	switch value {
	case H1ExcludedGenerated:
		return "Generated code"
	case H1ExcludedTestOnly:
		return "Test-only implementation"
	case H1ExcludedNotProductionReachable:
		return "Not reachable from production startup"
	case H1ExcludedNotExternalBoundary:
		return "Not an external client boundary"
	default:
		return string(value)
	}
}

func uniquePreviewEvidence(values []PreviewEvidence) []PreviewEvidence {
	seen := make(map[string]struct{}, len(values))
	result := make([]PreviewEvidence, 0, len(values))
	for _, value := range values {
		key := value.Path + "\x00" + fmt.Sprint(value.Line) + "\x00" + value.Symbol
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}
