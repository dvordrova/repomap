package orientation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/llm"
)

const (
	sectionSummary   = "summary"
	sectionRoles     = "roles"
	sectionRunRecipe = "run_recipe"
	sectionMainFlow  = "main_flow"
	sectionRequest   = "request"

	classFact    = 'f'
	classClaim   = 'c'
	classSubject = 's'
)

type modelResponse struct {
	Summary     string            `json:"summary"`
	SummaryRefs []string          `json:"summary_refs"`
	Roles       []json.RawMessage `json:"roles"`
	RunRecipe   []json.RawMessage `json:"run_recipe"`
	MainFlow    flowResponse      `json:"main_flow"`
}

type flowResponse struct {
	Title string            `json:"title"`
	Steps []json.RawMessage `json:"steps"`
}

type roleResponse struct {
	Target  string   `json:"target"`
	Role    string   `json:"role"`
	Purpose string   `json:"purpose"`
	Refs    []string `json:"refs"`
}

type recipeResponse struct {
	Target  string   `json:"target,omitempty"`
	Command string   `json:"command"`
	Cwd     string   `json:"cwd,omitempty"`
	Note    string   `json:"note,omitempty"`
	Refs    []string `json:"refs"`
}

type flowStepResponse struct {
	Target      string `json:"target"`
	Ref         string `json:"ref"`
	Explanation string `json:"explanation"`
}

// normalized is the accepted, restored part of one model response together
// with every row that was refused.
type normalized struct {
	summary     string
	summaryRefs []string
	roles       []Role
	recipe      []RecipeStep
	flow        MainFlow
	rejected    []RejectedRow
}

type resolvedRef struct {
	ref     string
	class   byte
	id      string
	fact    factEntry
	subject subjectEntry
}

// normalize is the pure decoder: strict JSON in, exact ids out. A broken row
// is refused with its raw JSON and a reason; the response as a whole fails
// only when it is not the requested JSON shape.
func normalize(raw []byte, cat catalog) (normalized, error) {
	response, err := decodeResponse(raw)
	if err != nil {
		return normalized{}, err
	}
	result := normalized{rejected: []RejectedRow{}}
	result.acceptSummary(response, cat)
	for _, row := range response.Roles {
		result.acceptRole(row, cat)
	}
	for _, row := range response.RunRecipe {
		result.acceptRecipe(row, cat)
	}
	result.acceptFlow(response.MainFlow, cat)
	return result, nil
}

func decodeResponse(raw []byte) (modelResponse, error) {
	if len(raw) == 0 || len(raw) > llm.ProviderResponseByteLimit {
		return modelResponse{}, fmt.Errorf("orientation: response exceeds bounded envelope")
	}
	normalizedJSON, err := llm.NormalizeJSON(raw)
	if err != nil {
		return modelResponse{}, fmt.Errorf("orientation: invalid response JSON: %w", err)
	}
	var response modelResponse
	if err := decodeStrict(normalizedJSON, &response); err != nil {
		return modelResponse{}, fmt.Errorf("orientation: decode response: %w", err)
	}
	return response, nil
}

func decodeStrict(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("trailing data")
	}
	return nil
}

func (result *normalized) reject(section string, raw json.RawMessage, reason string) {
	compact := &bytes.Buffer{}
	if err := json.Compact(compact, raw); err != nil {
		compact.Reset()
		compact.WriteString("null")
	}
	result.rejected = append(result.rejected, RejectedRow{
		Stage: StageName, Section: section, Raw: json.RawMessage(compact.Bytes()), Reason: reason,
	})
}

func (result *normalized) acceptSummary(response modelResponse, cat catalog) {
	if response.Summary == "" && len(response.SummaryRefs) == 0 {
		return
	}
	raw, _ := json.Marshal(map[string]any{"summary": response.Summary, "summary_refs": response.SummaryRefs})
	if !validSentence(response.Summary) {
		result.reject(sectionSummary, raw, sentenceReason("summary"))
		return
	}
	refs, err := cat.resolve(response.SummaryRefs, classFact, classClaim, classSubject)
	if err != nil {
		result.reject(sectionSummary, raw, err.Error())
		return
	}
	result.summary = response.Summary
	result.summaryRefs = ids(refs)
}

func (result *normalized) acceptRole(raw json.RawMessage, cat catalog) {
	var row roleResponse
	if err := decodeStrict(raw, &row); err != nil {
		result.reject(sectionRoles, raw, "row does not match the requested shape: "+err.Error())
		return
	}
	targetID, known := cat.targets[row.Target]
	if !known {
		result.reject(sectionRoles, raw, fmt.Sprintf("unknown target ref %q", row.Target))
		return
	}
	for _, accepted := range result.roles {
		if accepted.TargetID == targetID {
			result.reject(sectionRoles, raw, fmt.Sprintf("target %q already has a role", row.Target))
			return
		}
	}
	if !validSentence(row.Role) {
		result.reject(sectionRoles, raw, sentenceReason("role"))
		return
	}
	if !validSentence(row.Purpose) {
		result.reject(sectionRoles, raw, sentenceReason("purpose"))
		return
	}
	refs, err := cat.resolve(row.Refs, classFact, classClaim, classSubject)
	if err != nil {
		result.reject(sectionRoles, raw, err.Error())
		return
	}
	role := Role{TargetID: targetID, Role: row.Role, Purpose: row.Purpose, FactIDs: []string{}}
	for _, ref := range refs {
		switch ref.class {
		case classFact:
			role.FactIDs = append(role.FactIDs, ref.id)
		case classClaim:
			role.ClaimIDs = append(role.ClaimIDs, ref.id)
		case classSubject:
			role.SubjectIDs = append(role.SubjectIDs, ref.id)
		}
	}
	result.roles = append(result.roles, role)
}

func (result *normalized) acceptRecipe(raw json.RawMessage, cat catalog) {
	var row recipeResponse
	if err := decodeStrict(raw, &row); err != nil {
		result.reject(sectionRunRecipe, raw, "row does not match the requested shape: "+err.Error())
		return
	}
	targetID := ""
	if row.Target != "" {
		known := false
		if targetID, known = cat.targets[row.Target]; !known {
			result.reject(sectionRunRecipe, raw, fmt.Sprintf("unknown target ref %q", row.Target))
			return
		}
	}
	if !validSentence(row.Command) {
		result.reject(sectionRunRecipe, raw, sentenceReason("command"))
		return
	}
	if row.Note != "" && !validSentence(row.Note) {
		result.reject(sectionRunRecipe, raw, sentenceReason("note"))
		return
	}
	if row.Cwd != "" && !validText(row.Cwd) {
		result.reject(sectionRunRecipe, raw, "cwd must be one non-empty single-line path")
		return
	}
	refs, err := cat.resolve(row.Refs, classFact)
	if err != nil {
		result.reject(sectionRunRecipe, raw, err.Error())
		return
	}
	if !citesRunEvidence(refs) {
		result.reject(sectionRunRecipe, raw, "a run step must cite at least one manifest or entrypoint fact")
		return
	}
	result.recipe = append(result.recipe, RecipeStep{
		TargetID: targetID, Command: row.Command, Cwd: row.Cwd, Note: row.Note, FactIDs: ids(refs),
	})
}

func citesRunEvidence(refs []resolvedRef) bool {
	for _, ref := range refs {
		if ref.fact.kind == facts.KindManifest || ref.fact.kind == facts.KindEntrypoint {
			return true
		}
	}
	return false
}

func (result *normalized) acceptFlow(flow flowResponse, cat catalog) {
	for _, raw := range flow.Steps {
		result.acceptFlowStep(raw, cat)
	}
	if flow.Title == "" {
		return
	}
	raw, _ := json.Marshal(map[string]any{"title": flow.Title})
	switch {
	case !validSentence(flow.Title):
		result.reject(sectionMainFlow, raw, sentenceReason("title"))
	case len(result.flow.Steps) == 0:
		result.reject(sectionMainFlow, raw, "the main flow has no accepted steps")
	default:
		result.flow.Title = flow.Title
	}
}

func (result *normalized) acceptFlowStep(raw json.RawMessage, cat catalog) {
	var row flowStepResponse
	if err := decodeStrict(raw, &row); err != nil {
		result.reject(sectionMainFlow, raw, "row does not match the requested shape: "+err.Error())
		return
	}
	targetID, known := cat.targets[row.Target]
	if !known {
		result.reject(sectionMainFlow, raw, fmt.Sprintf("unknown target ref %q", row.Target))
		return
	}
	if !validSentence(row.Explanation) {
		result.reject(sectionMainFlow, raw, sentenceReason("explanation"))
		return
	}
	refs, err := cat.resolve([]string{row.Ref}, classFact, classSubject)
	if err != nil {
		result.reject(sectionMainFlow, raw, err.Error())
		return
	}
	ref := refs[0]
	step := FlowStep{TargetID: targetID, Explanation: row.Explanation}
	switch ref.class {
	case classFact:
		step.FactID = ref.id
	case classSubject:
		if ref.subject.targetRef != row.Target {
			result.reject(sectionMainFlow, raw, fmt.Sprintf("member %q does not belong to target %q", row.Ref, row.Target))
			return
		}
		step.SubjectID = ref.id
	}
	result.flow.Steps = append(result.flow.Steps, step)
}

// resolve checks one row's refs against the advertised catalog: every ref
// must be present, unique, and of an allowed class.
func (cat catalog) resolve(refs []string, allowed ...byte) ([]resolvedRef, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("the row cites nothing")
	}
	seen := make(map[string]struct{}, len(refs))
	resolved := make([]resolvedRef, 0, len(refs))
	for _, ref := range refs {
		if ref == "" {
			return nil, fmt.Errorf("the row cites an empty ref")
		}
		if _, duplicate := seen[ref]; duplicate {
			return nil, fmt.Errorf("duplicate ref %q", ref)
		}
		seen[ref] = struct{}{}
		if !bytes.ContainsRune(allowed, rune(ref[0])) {
			return nil, fmt.Errorf("ref %q is not allowed here (allowed: %s)", ref, classNames(allowed))
		}
		entry, err := cat.lookup(ref)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, entry)
	}
	return resolved, nil
}

func (cat catalog) lookup(ref string) (resolvedRef, error) {
	switch ref[0] {
	case classFact:
		if entry, known := cat.facts[ref]; known {
			return resolvedRef{ref: ref, class: classFact, id: entry.id, fact: entry}, nil
		}
	case classClaim:
		if id, known := cat.claims[ref]; known {
			return resolvedRef{ref: ref, class: classClaim, id: id}, nil
		}
	case classSubject:
		if entry, known := cat.subjects[ref]; known {
			return resolvedRef{ref: ref, class: classSubject, id: entry.id, subject: entry}, nil
		}
	}
	return resolvedRef{}, fmt.Errorf("unknown ref %q", ref)
}

func classNames(classes []byte) string {
	names := ""
	for position, class := range classes {
		if position > 0 {
			names += ", "
		}
		switch class {
		case classFact:
			names += "facts f*"
		case classClaim:
			names += "claims c*"
		case classSubject:
			names += "members s*"
		}
	}
	return names
}

func ids(refs []resolvedRef) []string {
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ref.id)
	}
	return result
}

func sentenceReason(field string) string {
	return fmt.Sprintf("%s must be one non-empty single-line sentence of at most %d characters", field, MaxSentenceRunes)
}
