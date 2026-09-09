package orientation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"

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
	summary        string
	summaryRefs    []string
	roles          []Role
	recipe         []RecipeStep
	flow           MainFlow
	rejected       []RejectedRow
	ambiguousRoles map[string]bool
	accepted       map[string]bool
	roleRows       map[string][]string
}

// Slots identify the original output position, even when earlier rows were refused.
func (result normalized) AcceptedRowKeys() []string {
	keys := make([]string, 0, len(result.accepted))
	for key := range result.accepted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
	fields, err := decodeResponse(raw)
	if err != nil {
		return normalized{}, err
	}
	result := normalized{rejected: []RejectedRow{}, accepted: make(map[string]bool), roleRows: make(map[string][]string)}
	var response modelResponse
	summaryOK := result.decodeField(sectionSummary, "summary", fields, &response.Summary)
	refsOK := result.decodeField(sectionSummary, "summary_refs", fields, &response.SummaryRefs)
	if summaryOK && refsOK {
		result.acceptSummary(response, cat)
	}
	if result.decodeField(sectionRoles, "roles", fields, &response.Roles) {
		for i, row := range response.Roles {
			result.acceptRole(row, cat, fmt.Sprintf("roles[%d]", i))
		}
	}
	if result.decodeField(sectionRunRecipe, "run_recipe", fields, &response.RunRecipe) {
		for i, row := range response.RunRecipe {
			result.acceptRecipe(row, cat, fmt.Sprintf("run_recipe[%d]", i))
		}
	}
	var flow map[string]json.RawMessage
	if result.decodeField(sectionMainFlow, "main_flow", fields, &flow) {
		result.decodeField(sectionMainFlow, "title", flow, &response.MainFlow.Title)
		result.decodeField(sectionMainFlow, "steps", flow, &response.MainFlow.Steps)
		result.acceptFlow(response.MainFlow, cat)
	}
	if len(result.rejected) > 0 && len(result.accepted) == 0 {
		return result, fmt.Errorf("orientation: no output accepted: %s", result.rejected[0].Reason)
	}
	return result, nil
}

func (result *normalized) decodeField(section, name string, fields map[string]json.RawMessage, value any) bool {
	raw, present := fields[name]
	if !present {
		return true
	}
	if err := json.Unmarshal(raw, value); err != nil {
		result.reject(section, raw, fmt.Sprintf("%s has an invalid shape: %v", name, err))
		return false
	}
	return true
}

func decodeResponse(raw []byte) (map[string]json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > llm.ProviderResponseByteLimit {
		return nil, fmt.Errorf("orientation: response exceeds bounded envelope")
	}
	normalizedJSON, err := llm.NormalizeJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("orientation: invalid response JSON: %w", err)
	}
	var response map[string]json.RawMessage
	if err := decodeStrict(normalizedJSON, &response); err != nil || response == nil {
		return nil, fmt.Errorf("orientation: response must be a JSON object")
	}
	return response, nil
}

func decodeStrict(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
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
	response.Summary = strings.TrimSpace(response.Summary)
	if response.Summary == "" && len(response.SummaryRefs) == 0 {
		return
	}
	raw, _ := json.Marshal(map[string]any{"summary": response.Summary, "summary_refs": response.SummaryRefs})
	if !validSentence(response.Summary) {
		result.reject(sectionSummary, raw, sentenceReason("summary"))
		return
	}
	refs, ignored, err := cat.resolve(response.SummaryRefs, classFact, classClaim, classSubject)
	if err != nil {
		result.reject(sectionSummary, raw, err.Error())
		return
	}
	result.rejectRefs(sectionSummary, ignored)
	result.summary = response.Summary
	result.summaryRefs = ids(refs)
	result.accepted["summary"] = true
}

func (result *normalized) acceptRole(raw json.RawMessage, cat catalog, slot string) {
	var row roleResponse
	if err := decodeStrict(raw, &row); err != nil {
		result.reject(sectionRoles, raw, "row does not match the requested shape: "+err.Error())
		return
	}
	row.Role = strings.Join(strings.Fields(row.Role), " ")
	row.Purpose = strings.TrimSpace(row.Purpose)
	targetID, known := cat.targets[row.Target]
	if !known {
		result.reject(sectionRoles, raw, fmt.Sprintf("unknown target ref %q", row.Target))
		return
	}
	if !validSentence(row.Role) {
		result.reject(sectionRoles, raw, sentenceReason("role"))
		return
	}
	if !validSentence(row.Purpose) {
		result.reject(sectionRoles, raw, sentenceReason("purpose"))
		return
	}
	refs, ignored, err := cat.resolve(row.Refs, classFact, classClaim, classSubject)
	if err != nil {
		result.reject(sectionRoles, raw, err.Error())
		return
	}
	result.rejectRefs(sectionRoles, ignored)
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
	if result.ambiguousRoles[targetID] {
		result.reject(sectionRoles, raw, fmt.Sprintf("target %q has conflicting roles", row.Target))
		return
	}
	for i, accepted := range result.roles {
		if accepted.TargetID != targetID {
			continue
		}
		if accepted.Role == role.Role && accepted.Purpose == role.Purpose {
			result.roles[i].FactIDs = unionRefs(accepted.FactIDs, role.FactIDs)
			result.roles[i].ClaimIDs = unionRefs(accepted.ClaimIDs, role.ClaimIDs)
			result.roles[i].SubjectIDs = unionRefs(accepted.SubjectIDs, role.SubjectIDs)
			result.accepted[slot] = true
			result.roleRows[targetID] = append(result.roleRows[targetID], slot)
			return
		}
		result.roles = append(result.roles[:i], result.roles[i+1:]...)
		if result.ambiguousRoles == nil {
			result.ambiguousRoles = make(map[string]bool)
		}
		result.ambiguousRoles[targetID] = true
		for _, slot := range result.roleRows[targetID] {
			delete(result.accepted, slot)
		}
		result.reject(sectionRoles, raw, fmt.Sprintf("conflicting roles for target %q; neither role is used", row.Target))
		return
	}
	result.roles = append(result.roles, role)
	result.accepted[slot] = true
	result.roleRows[targetID] = append(result.roleRows[targetID], slot)
}

func unionRefs(left, right []string) []string {
	for _, ref := range right {
		if !slices.Contains(left, ref) {
			left = append(left, ref)
		}
	}
	return left
}

func (result *normalized) acceptRecipe(raw json.RawMessage, cat catalog, slot string) {
	var row recipeResponse
	if err := decodeStrict(raw, &row); err != nil {
		result.reject(sectionRunRecipe, raw, "row does not match the requested shape: "+err.Error())
		return
	}
	row.Command = strings.TrimSpace(row.Command)
	row.Note = strings.TrimSpace(row.Note)
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
	refs, ignored, err := cat.resolve(row.Refs, classFact)
	if err != nil {
		result.reject(sectionRunRecipe, raw, err.Error())
		return
	}
	result.rejectRefs(sectionRunRecipe, ignored)
	if !citesRunEvidence(refs) {
		result.reject(sectionRunRecipe, raw, "a run step must cite at least one manifest or entrypoint fact")
		return
	}
	result.recipe = append(result.recipe, RecipeStep{
		TargetID: targetID, Command: row.Command, Cwd: row.Cwd, Note: row.Note, FactIDs: ids(refs),
	})
	result.accepted[slot] = true
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
	flow.Title = strings.Join(strings.Fields(flow.Title), " ")
	for i, raw := range flow.Steps {
		result.acceptFlowStep(raw, cat, fmt.Sprintf("main_flow.steps[%d]", i))
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
		result.accepted["main_flow.title"] = true
	}
}

func (result *normalized) acceptFlowStep(raw json.RawMessage, cat catalog, slot string) {
	var row flowStepResponse
	if err := decodeStrict(raw, &row); err != nil {
		result.reject(sectionMainFlow, raw, "row does not match the requested shape: "+err.Error())
		return
	}
	row.Explanation = strings.TrimSpace(row.Explanation)
	targetID, known := cat.targets[row.Target]
	if !known {
		result.reject(sectionMainFlow, raw, fmt.Sprintf("unknown target ref %q", row.Target))
		return
	}
	if !validSentence(row.Explanation) {
		result.reject(sectionMainFlow, raw, sentenceReason("explanation"))
		return
	}
	refs, _, err := cat.resolve([]string{row.Ref}, classFact, classSubject)
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
	result.accepted[slot] = true
}

// resolve keeps the advertised set. An unusable additional ref does not undo
// valid evidence; a row with no remaining evidence still has no answer.
func (cat catalog) resolve(refs []string, allowed ...byte) ([]resolvedRef, []string, error) {
	seen := make(map[string]struct{}, len(refs))
	resolved := make([]resolvedRef, 0, len(refs))
	var ignored []string
	for _, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		if ref == "" || !bytes.ContainsRune(allowed, rune(ref[0])) {
			ignored = append(ignored, ref)
			continue
		}
		entry, err := cat.lookup(ref)
		if err != nil {
			ignored = append(ignored, ref)
			continue
		}
		resolved = append(resolved, entry)
	}
	if len(resolved) == 0 {
		return nil, ignored, fmt.Errorf("the row cites no advertised %s", classNames(allowed))
	}
	return resolved, ignored, nil
}

func (result *normalized) rejectRefs(section string, refs []string) {
	for _, ref := range refs {
		raw, _ := json.Marshal(ref)
		result.reject(section, raw, "unsupported reference ignored; accepted evidence is kept")
	}
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
	return fmt.Sprintf("%s must contain non-empty text", field)
}
