package orientation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
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
	Summary        string            `json:"summary"`
	SummaryRefs    refList           `json:"summary_refs"`
	Roles          []json.RawMessage `json:"roles"`
	RunRecipe      []json.RawMessage `json:"run_recipe"`
	MainFlowTarget string            `json:"main_flow_target"`
}

type roleResponse struct {
	Target  string  `json:"target"`
	Role    string  `json:"role"`
	Purpose string  `json:"purpose"`
	Refs    refList `json:"refs"`
}

type recipeResponse struct {
	Target  string  `json:"target,omitempty"`
	Command string  `json:"command"`
	Cwd     string  `json:"cwd,omitempty"`
	Note    string  `json:"note,omitempty"`
	Refs    refList `json:"refs"`
}

// refList is a list of request-local refs. One ref written as a bare string
// is the same one-element list; any other non-list value keeps its type error.
type refList []string

func (refs *refList) UnmarshalJSON(raw []byte) error {
	if string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		*refs = refList{single}
		return nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	*refs = list
	return nil
}

// normalized is the accepted, restored part of one model response together
// with every row that was refused.
type normalized struct {
	summary        string
	summaryRefs    []string
	roles          []Role
	recipe         []RecipeStep
	flowTarget     string // the target id whose Main flow is walked
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

// normalizeOverview is the overview's pure decoder: strict JSON in, exact
// ids out. A broken row is refused with its raw JSON and a reason; the
// response as a whole fails only when it is not the requested JSON shape.
func normalizeOverview(raw []byte, cat catalog) (normalized, error) {
	fields, err := decodeResponse(raw)
	if err != nil {
		return normalized{}, err
	}
	result := newNormalized()
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
	if result.decodeField(sectionMainFlow, "main_flow_target", fields, &response.MainFlowTarget) {
		result.acceptFlowTarget(fields["main_flow_target"], response.MainFlowTarget, cat)
	}
	return result.done()
}

func newNormalized() normalized {
	return normalized{rejected: []RejectedRow{}, accepted: make(map[string]bool), roleRows: make(map[string][]string)}
}

// done refuses a response whose every row was refused.
func (result normalized) done() (normalized, error) {
	if len(result.rejected) > 0 && len(result.accepted) == 0 {
		return result, &noOutputError{rejected: result.rejected}
	}
	return result, nil
}

// acceptFlowTarget keeps the one target the Main flow is asked for. None is
// a legitimate answer: no flow is asked.
func (result *normalized) acceptFlowTarget(raw json.RawMessage, ref string, cat catalog) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return
	}
	targetID, known := cat.targets[ref]
	if !known {
		result.reject(sectionMainFlow, raw, fmt.Sprintf("unknown main_flow_target %q", ref))
		return
	}
	result.flowTarget = targetID
	result.accepted["main_flow_target"] = true
}

// noOutputError refuses a response in which every row was refused. It keeps
// each row's own rejection so the journal names every reason, not the first.
type noOutputError struct {
	rejected []RejectedRow
}

func (err *noOutputError) Error() string {
	if len(err.rejected) == 1 {
		return fmt.Sprintf("orientation: no output accepted: %s", err.rejected[0].Reason)
	}
	return fmt.Sprintf("orientation: no output accepted: %d rows refused, first: %s", len(err.rejected), err.rejected[0].Reason)
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
	if ref := cat.proseRef(response.Summary); ref != "" {
		result.reject(sectionSummary, raw, proseRefReason("summary", ref, "summary_refs"))
		return
	}
	refs, ignored, err := cat.resolve(response.SummaryRefs, "", classFact, classClaim, classSubject)
	if err != nil {
		result.reject(sectionSummary, raw, err.Error())
		return
	}
	result.rejectRefs(sectionSummary, ignored)
	result.summary = response.Summary
	result.summaryRefs = qualifiedIDs(refs)
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
	row.Target = strings.TrimSpace(row.Target)
	targetID, known := cat.targets[row.Target]
	if !known {
		result.reject(sectionRoles, raw, fmt.Sprintf("unknown target ref %q", row.Target))
		return
	}
	if !validSentence(row.Role) {
		result.reject(sectionRoles, raw, sentenceReason("role"))
		return
	}
	if ref := cat.proseRef(row.Role); ref != "" {
		result.reject(sectionRoles, raw, proseRefReason("role", ref, "refs"))
		return
	}
	// The label is the decision; an empty purpose stays empty, never filled.
	if row.Purpose != "" && !validSentence(row.Purpose) {
		result.reject(sectionRoles, raw, sentenceReason("purpose"))
		return
	}
	// A purpose writing a ref in its prose is refused alone: the role stands
	// without it, as a refused note leaves its step.
	if ref := cat.proseRef(row.Purpose); ref != "" {
		purpose, _ := json.Marshal(row.Purpose)
		result.reject(sectionRoles, purpose, proseRefReason("purpose", ref, "refs")+"; the purpose is dropped and the role kept")
		row.Purpose = ""
	}
	refs, ignored, err := cat.resolve(row.Refs, row.Target, classFact, classClaim, classSubject)
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
			role.SubjectIDs = append(role.SubjectIDs, ref.ref)
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
		if accepted.Role == role.Role && samePurpose(accepted.Purpose, role.Purpose) {
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

// samePurpose treats purposes that differ only in whitespace as one answer.
// The first accepted spelling is kept; any other difference is a conflict.
func samePurpose(left, right string) bool {
	return strings.Join(strings.Fields(left), " ") == strings.Join(strings.Fields(right), " ")
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
	row.Cwd = strings.TrimSpace(row.Cwd)
	row.Target = strings.TrimSpace(row.Target)
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
	if row.Cwd != "" && !validText(row.Cwd) {
		result.reject(sectionRunRecipe, raw, "cwd must be one non-empty single-line path")
		return
	}
	refs, ignored, err := cat.resolve(row.Refs, row.Target, classFact)
	if err != nil {
		result.reject(sectionRunRecipe, raw, err.Error())
		return
	}
	result.rejectRefs(sectionRunRecipe, ignored)
	if !citesRunEvidence(refs) {
		result.reject(sectionRunRecipe, raw, "a run step must cite at least one manifest or entrypoint fact")
		return
	}
	if row.Note != "" && !validSentence(row.Note) {
		// The note is optional: only it is dropped, and the step stays.
		note, _ := json.Marshal(row.Note)
		result.reject(sectionRunRecipe, note, sentenceReason("note")+"; the note is dropped and the step kept")
		row.Note = ""
	}
	if ref := cat.proseRef(row.Note); ref != "" {
		note, _ := json.Marshal(row.Note)
		result.reject(sectionRunRecipe, note, proseRefReason("note", ref, "refs")+"; the note is dropped and the step kept")
		row.Note = ""
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

// resolve keeps the advertised set. An unusable additional ref does not undo
// valid evidence; a row with no remaining evidence still has no answer. A
// fact row several targets share restores the named target's own fact id.
func (cat catalog) resolve(refs []string, targetRef string, allowed ...byte) ([]resolvedRef, []string, error) {
	seen := make(map[string]struct{}, len(refs))
	resolved := make([]resolvedRef, 0, len(refs))
	var ignored []string
	for _, ref := range refs {
		if _, duplicate := seen[ref]; duplicate {
			continue
		}
		seen[ref] = struct{}{}
		if ref == "" {
			ignored = append(ignored, ref)
			continue
		}
		entry, err := cat.lookup(ref, targetRef)
		if err != nil || !bytes.ContainsRune(allowed, rune(entry.class)) {
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

func (cat catalog) lookup(ref, targetRef string) (resolvedRef, error) {
	if entry, known := cat.facts[ref]; known {
		return resolvedRef{ref: ref, class: classFact, id: entry.idFor(targetRef), fact: entry}, nil
	}
	if id, known := cat.claims[ref]; known {
		return resolvedRef{ref: ref, class: classClaim, id: id}, nil
	}
	if entry, known := cat.subjects[ref]; known {
		return resolvedRef{ref: ref, class: classSubject, id: entry.id, subject: entry}, nil
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
			names += "facts a*"
		case classClaim:
			names += "claims h*"
		case classSubject:
			names += "qualified members"
		}
	}
	return names
}

// qualifiedIDs keeps a subject's target-qualified ref: a bare subject id
// repeats across targets (t1.n3 and t2.n3), and the report resolves the
// qualified form. Facts and claims keep their own artifact ids.
func qualifiedIDs(refs []resolvedRef) []string {
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.class == classSubject {
			result = append(result, ref.ref)
			continue
		}
		result = append(result, ref.id)
	}
	return result
}

func ids(refs []resolvedRef) []string {
	result := make([]string, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ref.id)
	}
	return result
}

// refToken is the shape of a request-local ref written as a word: a
// target (t7), a fact (a12), a claim (h3) or a target-qualified member or
// group (t1.n22, t1.g3).
var refToken = regexp.MustCompile(`\b(?:t[0-9]+(?:\.[a-z][0-9]+)?|[ah][0-9]+)\b`)

// proseRef is the first request-local ref a prose value writes, or "": a
// word of the ref shape that the request advertises, or a member or group
// of an advertised target. Refs belong in the refs fields; a sentence
// carrying one ("its main program (t1)") is refused at its own cell, never
// rewritten (owner, 2026-09-30: freqtrade's summary read "main program
// (t1)" and "REST client (t7)").
func (cat catalog) proseRef(text string) string {
	for _, token := range refToken.FindAllString(text, -1) {
		if _, known := cat.targets[token]; known {
			return token
		}
		if _, err := cat.lookup(token, ""); err == nil {
			return token
		}
		if target, _, qualified := strings.Cut(token, "."); qualified {
			if _, known := cat.targets[target]; known {
				return token
			}
		}
	}
	return ""
}

func proseRefReason(field, ref, refsField string) string {
	return fmt.Sprintf("%s writes the request-local ref %q in its prose; refs go only in %s", field, ref, refsField)
}

func sentenceReason(field string) string {
	return fmt.Sprintf("%s must contain non-empty text", field)
}
