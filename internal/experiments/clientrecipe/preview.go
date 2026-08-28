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
	Summary  PreviewSummary     `json:"summary"`
	Tasks    []PreviewTask      `json:"tasks"`
	Steps    []PreviewStep      `json:"steps"`
	Examples []PreviewExample   `json:"examples"`
	Audit    []PreviewExclusion `json:"audit"`
}

type PreviewTarget struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Kind     string `json:"kind"`
}

type PreviewSummary struct {
	Observed         int `json:"observed"`
	Boundaries       int `json:"boundaries"`
	Complete         int `json:"complete"`
	Excluded         int `json:"excluded"`
	RequiredRoles    int `json:"required_roles"`
	CommonRoles      int `json:"common_roles"`
	CallbackClosed   int `json:"callback_closed"`
	CallbackFrontier int `json:"callback_frontier"`
}

type PreviewTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Available   bool   `json:"available"`
}

type PreviewRole struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Necessity string `json:"necessity"`
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
}

type PreviewSlot struct {
	StepID   string            `json:"step_id"`
	Title    string            `json:"title"`
	Status   string            `json:"status"`
	Roles    []PreviewRole     `json:"roles"`
	Missing  []string          `json:"missing"`
	Evidence []PreviewEvidence `json:"evidence"`
}

type PreviewExample struct {
	ID               string        `json:"id"`
	Name             string        `json:"name"`
	Summary          string        `json:"summary"`
	Status           string        `json:"status"`
	Complete         bool          `json:"complete"`
	Recommended      bool          `json:"recommended"`
	VerificationKind string        `json:"verification_kind"`
	Missing          []string      `json:"missing"`
	EvidenceCount    int           `json:"evidence_count"`
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

	stepCopy := make(map[string]H2StepCopy, len(h2.Steps))
	for _, row := range h2.Steps {
		stepCopy[row.Ref] = row
	}
	exampleCopy := make(map[string]H2ExampleCopy, len(h2.Examples))
	for _, row := range h2.Examples {
		exampleCopy[row.Ref] = row
	}
	definitions := h2Examples(h1)
	recommendedID := previewBestExample(h1, definitions)
	examples := make([]PreviewExample, 0, len(definitions))
	for _, definition := range definitions {
		example := buildPreviewExample(h1, definition, stepCopy, exampleCopy[definition.Ref])
		example.Recommended = definition.Ref == recommendedID
		examples = append(examples, example)
	}
	sort.SliceStable(examples, func(i, j int) bool {
		if examples[i].Recommended != examples[j].Recommended {
			return examples[i].Recommended
		}
		if examples[i].Complete != examples[j].Complete {
			return examples[i].Complete
		}
		return examples[i].Name < examples[j].Name
	})

	necessities := make(map[H1Role]H1Necessity, len(h1.Roles))
	required, common := 0, 0
	for _, row := range h1.Roles {
		necessities[row.Role] = row.Necessity
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
		step := PreviewStep{ID: definition.Ref, Number: index + 1, Title: copyRow.Title, Purpose: copyRow.Purpose}
		for _, role := range definition.Roles {
			step.Roles = append(step.Roles, previewRole(role, necessities[role]))
		}
		for _, example := range examples {
			slot := previewSlot(example, definition.Ref)
			if slot.Status == "covered" {
				step.CompleteCoverage++
				step.CoveredExamples = append(step.CoveredExamples, example.Name)
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
	complete := 0
	for _, example := range examples {
		if example.Complete {
			complete++
		}
	}
	model := PreviewModel{
		Version: PreviewVersion,
		Target:  PreviewTarget{Name: "launch-service", Language: "Go", Kind: "Service"},
		Summary: PreviewSummary{
			Observed: h1.Ledger.Observed, Boundaries: h1.Ledger.Admitted, Complete: complete,
			Excluded: h1.Ledger.Excluded, RequiredRoles: required, CommonRoles: common,
			CallbackClosed: h1.Callbacks.Closed, CallbackFrontier: h1.Callbacks.Frontier,
		},
		Tasks: []PreviewTask{
			{ID: "add_client", Title: "Add an external client", Description: "Follow the repository's proven boundary shape from configuration to verification.", Available: true},
			{ID: "trace_startup", Title: "Trace service startup", Description: "See how the live application graph is assembled."},
			{ID: "add_endpoint", Title: "Add an endpoint", Description: "Find the local request-handling pattern."},
			{ID: "change_storage", Title: "Change persistence", Description: "Locate storage contracts and implementations."},
			{ID: "add_metric", Title: "Add observability", Description: "Follow metrics and logging conventions."},
			{ID: "inspect_tests", Title: "Extend verification", Description: "Find the repository's testing boundaries."},
		},
		Steps: steps, Examples: examples, Audit: audit,
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
	h1 H1Result,
	definition h2ExampleDefinition,
	stepCopy map[string]H2StepCopy,
	copyRow H2ExampleCopy,
) PreviewExample {
	instance := definition.Instance
	necessities := make(map[H1Role]H1Necessity, len(h1.Roles))
	for _, role := range h1.Roles {
		necessities[role.Role] = role.Necessity
	}
	roleRows := make(map[H1Role]H1RoleEvidence, len(instance.Roles))
	for _, row := range instance.Roles {
		roleRows[row.Role] = row
	}
	example := PreviewExample{
		ID: definition.Ref, Name: definition.Name, Summary: copyRow.Summary,
		Complete: instance.Complete, VerificationKind: previewVerification(instance.VerificationKind),
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
		slot := PreviewSlot{StepID: stepDefinition.Ref, Title: stepCopy[stepDefinition.Ref].Title, Status: "covered"}
		for _, role := range stepDefinition.Roles {
			slot.Roles = append(slot.Roles, previewRole(role, necessities[role]))
			row, present := roleRows[role]
			if !present {
				slot.Missing = append(slot.Missing, previewRoleLabel(role))
				continue
			}
			for _, evidence := range row.Evidence {
				slot.Evidence = append(slot.Evidence, previewEvidence(definition.Ref, definition.Name, role, evidence))
				example.EvidenceCount++
			}
		}
		if len(slot.Missing) != 0 {
			slot.Status = "missing"
		}
		example.Slots = append(example.Slots, slot)
	}
	return example
}

func previewBestExample(h1 H1Result, definitions []h2ExampleDefinition) string {
	necessities := make(map[H1Role]H1Necessity, len(h1.Roles))
	for _, row := range h1.Roles {
		necessities[row.Role] = row.Necessity
	}
	bestRef, bestPath, bestScore := "", "", -1
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
		if score > bestScore || (score == bestScore && (bestPath == "" || definition.Instance.ImporterRepositoryPath < bestPath)) {
			bestRef, bestPath, bestScore = definition.Ref, definition.Instance.ImporterRepositoryPath, score
		}
	}
	return bestRef
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

func previewRole(role H1Role, necessity H1Necessity) PreviewRole {
	return PreviewRole{ID: string(role), Label: previewRoleLabel(role), Necessity: previewNecessity(necessity)}
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

func previewNecessity(value H1Necessity) string {
	switch value {
	case H1Required:
		return "Required"
	case H1Common:
		return "Common"
	default:
		return "Optional"
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
