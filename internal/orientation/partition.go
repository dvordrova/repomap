package orientation

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

//go:embed context-prompt.md
var contextPrompt string

//go:embed partition-prompt.md
var partitionPrompt string

//go:embed partition-roles-prompt.md
var partitionRolesPrompt string

//go:embed partition-recipe-prompt.md
var partitionRecipePrompt string

//go:embed partition-repository-prompt.md
var partitionRepositoryPrompt string

func sectionPrompt(section string) string {
	var decision string
	switch section {
	case "roles":
		decision = partitionRolesPrompt
	case "run_recipe":
		decision = partitionRecipePrompt
	case "repository":
		decision = partitionRepositoryPrompt
	}
	return partitionPrompt + "\n" + decision
}

type orientationRecord struct {
	Ref     string   `json:"ref"`
	Layer   string   `json:"layer"`
	Kind    string   `json:"kind"`
	Value   any      `json:"value"`
	Sources []string `json:"sources,omitempty"`
}

type orientationObservation struct {
	Text           string              `json:"text"`
	Sources        []string            `json:"supports,omitempty"`
	Question       string              `json:"question,omitempty"`
	Status         string              `json:"status,omitempty"`
	Basis          []orientationBasis  `json:"basis,omitempty"`
	SupportRecords []string            `json:"supporting_records,omitempty"`
	Native         []orientationRecord `json:"-"`
}

type orientationReading struct {
	Task    string              `json:"task"`
	Target  targetWire          `json:"target"`
	Records []orientationRecord `json:"records"`
}

type orientationScope struct {
	Ref     string `json:"ref"`
	Meaning string `json:"meaning"`
	First   string `json:"first"`
	Last    string `json:"last"`
}

const contextTask = "repomap.orientation.context.v4"

// Records preserve complete source values. A seed's ordered calls may be
// partitioned, each beside its declaration and referenced native evidence.
func orientationRecords(wire overviewRequest, target targetWire) []orientationRecord {
	var records []orientationRecord
	add := func(layer, kind string, value any, sources ...string) {
		records = append(records, orientationRecord{fmt.Sprintf("d%d", len(records)+1), layer, kind, value, sources})
	}
	add("native", "omitted_fact_counts", struct {
		Scope      string         `json:"scope"`
		Repository string         `json:"repository"`
		Counts     map[string]int `json:"counts"`
	}{"repository", wire.Repository, wire.OmittedFactCounts})
	factsByRef := map[string]factWire{}
	groupsByRef := map[string]groupWire{}
	targetsByRef := map[string]targetWire{}
	for _, value := range wire.Facts {
		factsByRef[value.Ref] = value
	}
	for _, value := range wire.Groups {
		groupsByRef[value.Ref] = value
	}
	for _, value := range wire.Targets {
		targetsByRef[value.Ref] = value
	}
	for _, fact := range wire.Facts {
		if len(fact.Targets) == 0 || slices.Contains(fact.Targets, target.Ref) {
			var related []factWire
			var targets []targetWire
			for _, ref := range fact.Links {
				if value, known := factsByRef[ref]; known {
					related = append(related, value)
				}
			}
			for _, ref := range append(append([]string(nil), fact.Targets...), fact.Peer) {
				if value, known := targetsByRef[ref]; known {
					targets = append(targets, value)
				}
			}
			add("native", "fact", struct {
				Fact    factWire     `json:"fact"`
				Related []factWire   `json:"related_facts,omitempty"`
				Targets []targetWire `json:"related_targets,omitempty"`
			}{fact, related, targets}, fact.Ref)
		}
	}
	for _, group := range wire.Groups {
		if group.Target == target.Ref {
			add("model_interpretation", "group", group)
		}
	}
	for _, connection := range wire.Connections {
		if strings.HasPrefix(connection.From, target.Ref+".") || strings.HasPrefix(connection.To, target.Ref+".") {
			add("model_interpretation", "connection", struct {
				Connection connectionWire `json:"connection"`
				From       groupWire      `json:"from_group"`
				To         groupWire      `json:"to_group"`
				FromTarget targetWire     `json:"from_target"`
				ToTarget   targetWire     `json:"to_target"`
			}{connection, groupsByRef[connection.From], groupsByRef[connection.To], targetsByRef[groupsByRef[connection.From].Target], targetsByRef[groupsByRef[connection.To].Target]})
		}
	}
	for _, seed := range wire.Seeds {
		if !strings.HasPrefix(seed.Ref, target.Ref+".") {
			continue
		}
		header := seed
		header.Calls, header.Evidence = nil, nil
		add("native", "seed", header, seed.Ref)
		used := map[string]bool{}
		for i, call := range seed.Calls {
			evidence := map[string][]any{}
			// The wire already owns the call's tuple vocabulary. Inspect only
			// its evidence_refs field, without guessing effects from its name.
			raw, _ := encodeWire(call)
			var visit func(any)
			visit = func(value any) {
				switch v := value.(type) {
				case []any:
					for _, x := range v {
						visit(x)
					}
				case map[string]any:
					for key, x := range v {
						if key == "evidence_refs" {
							list, _ := x.([]any)
							for _, ref := range list {
								if id, ok := ref.(string); ok {
									if row, known := seed.Evidence[id]; known {
										evidence[id] = row
										used[id] = true
									}
								}
							}
						} else {
							visit(x)
						}
					}
				}
			}
			var value any
			_ = json.Unmarshal(raw, &value)
			visit(value)
			add("native", "seed_call", struct {
				Seed     memberRow        `json:"seed"`
				Position int              `json:"position"`
				Call     any              `json:"call"`
				Evidence map[string][]any `json:"evidence,omitempty"`
			}{header, i + 1, call, evidence}, seed.Ref)
		}
		for _, ref := range slices.Sorted(slices.Values(mapKeys(seed.Evidence))) {
			if !used[ref] {
				add("native", "seed_evidence", struct {
					Seed  memberRow `json:"seed"`
					Ref   string    `json:"ref"`
					Value []any     `json:"value"`
				}{header, ref, seed.Evidence[ref]}, seed.Ref)
			}
		}
	}
	return records
}

func mapKeys[V any](m map[string]V) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Duplicate required fields are compared before any observation is accepted.
func contextObject(raw []byte) (map[string][]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("orientation context: expected object")
	}
	fields := map[string][]json.RawMessage{}
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return nil, err
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		name := strings.ToLower(strings.TrimSpace(key.(string)))
		fields[name] = append(fields[name], value)
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	return fields, nil
}

func contextString(values []json.RawMessage) (string, bool) {
	first, seen := "", false
	for _, raw := range values {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return "", false
		}
		value = strings.TrimSpace(value)
		if seen && strings.Join(strings.Fields(first), " ") != strings.Join(strings.Fields(value), " ") {
			return "", false
		}
		if !seen {
			first, seen = value, true
		}
	}
	return first, seen && first != ""
}

func readOrientationContext(ctx context.Context, executor llm.Executor, provider llm.Provider, input Input, digests []string, target targetWire, records []orientationRecord) (orientationContext, []RejectedRow, error) {
	item := orientationReading{contextTask, target, records}
	build := func(item orientationReading) (llm.Call[orientationContext], error) {
		if len(item.Records) == 0 {
			return llm.Call[orientationContext]{}, fmt.Errorf("orientation context: empty input")
		}
		wire, err := encodeContextReading(item)
		if err != nil {
			return llm.Call[orientationContext]{}, err
		}
		readingLimits := limits()
		readingLimits.MaxOutputTokens = llm.DefaultMaxOutputTokens
		return llm.Call[orientationContext]{State: cubeState(input, digests, "context", wire), Prompt: llm.Prompt{System: contextPrompt, User: string(wire), ResponseFormatJSON: true, NoResponseAdjunct: true, ResponseExample: contextExample}, Limits: readingLimits, DecodeValidate: func(raw []byte) (orientationContext, error) { return decodeOrientationContext(raw, item) }}, nil
	}
	split := func(item orientationReading) (orientationReading, orientationReading, bool) {
		if len(item.Records) < 2 {
			return orientationReading{}, orientationReading{}, false
		}
		a, b := item, item
		half := len(item.Records) / 2
		a.Records, b.Records = item.Records[:half], item.Records[half:]
		return a, b, true
	}
	leaves, err := llm.ExecuteAdaptiveJSONEachResults(ctx, executor, provider, []orientationReading{item}, build, split)
	if err != nil {
		return orientationContext{}, nil, err
	}
	result := orientationContext{Provenance: records, Questions: map[string]bool{}}
	for _, q := range contextQuestions {
		result.Questions[q] = true
	}
	for _, leaf := range leaves {
		if leaf.Err != nil {
			wire, _ := encodeWire(leaf.Item)
			outcome := llm.Outcome[normalized]{Request: leaf.Outcome.Request, Response: leaf.Outcome.Response, ResponseRejections: leaf.Outcome.ResponseRejections}
			rows, ok := refusal(ctx, leaf.Err, outcome, "context", len(wire))
			if !ok {
				return orientationContext{}, nil, leaf.Err
			}
			result.Rejected = append(result.Rejected, rows...)
			result.Gaps = append(result.Gaps, orientationGap{Question: "all_questions", Reason: "complete reader window unavailable", Target: target.Ref, Scope: orientationScope{Ref: "w1", Meaning: "all_supplied_records", First: leaf.Item.Records[0].Ref, Last: leaf.Item.Records[len(leaf.Item.Records)-1].Ref}})
			for _, q := range contextQuestions {
				result.Questions[q] = false
			}
			continue
		}
		result.Observations = append(result.Observations, leaf.Outcome.Value.Observations...)
		result.Rejected = append(result.Rejected, leaf.Outcome.Value.Rejected...)
		result.Gaps = append(result.Gaps, leaf.Outcome.Value.Gaps...)
		for _, q := range contextQuestions {
			result.Questions[q] = result.Questions[q] && leaf.Outcome.Value.Questions[q]
		}
	}
	return result, result.Rejected, nil
}

const contextExample = `{"scope":"w1","signals":{"responsibilities":[{"basis":["d1"],"text":"Offers the supplied database interface.","status":"interpretation","supports":[]}],"interfaces":[],"launch_requirements":[],"uncertainty":[]}}`

func sectionExample(section string) string {
	switch section {
	case "roles":
		return `{"roles":[{"target":"t1","role":"Database engine","purpose":"Implements database operations.","refs":["a1"]}]}`
	case "run_recipe":
		return `{"run_recipe":[{"target":"t1","command":"./program <input>","cwd":".","note":"Supply the required input.","refs":["a1"]}]}`
	default:
		return `{"summary":"The repository implements database operations.","summary_refs":["a1"],"main_flow_target":"t1"}`
	}
}

func finalContextWire(repository string, targets []targetWire, observations []orientationObservation, cat catalog, section string, gaps ...orientationGap) ([]byte, error) {
	type sourceChoice struct {
		Ref      string   `json:"ref"`
		Kind     string   `json:"kind"`
		Targets  []string `json:"targets,omitempty"`
		Evidence any      `json:"evidence,omitempty"`
	}
	// Selected native values are emitted once, including full related evidence
	// and ordered call uses. Their local identity is closed for this final request.
	var nativeSupport []orientationRecord
	nativeRefs := map[string]string{}
	observations = slices.Clone(observations)
	for i := range observations {
		if len(observations[i].Native) == 0 {
			continue
		}
		observations[i].SupportRecords = nil
		for _, record := range observations[i].Native {
			raw, err := encodeWire(record)
			if err != nil {
				return nil, err
			}
			ref, known := nativeRefs[string(raw)]
			if !known {
				ref = fmt.Sprintf("s%d", len(nativeSupport)+1)
				nativeRefs[string(raw)] = ref
				record.Ref = ref
				nativeSupport = append(nativeSupport, record)
			}
			observations[i].SupportRecords = unionRefs(observations[i].SupportRecords, []string{ref})
		}
	}
	var choices []sourceChoice
	seen := make(map[string]struct{})
	for _, observation := range observations {
		for _, ref := range observation.Sources {
			if _, exists := seen[ref]; exists {
				continue
			}
			seen[ref] = struct{}{}
			if source, ok := cat.facts[ref]; ok {
				kind := string(source.kind)
				if source.export {
					kind = exportKind
				}
				targets := mapKeys(source.byTarget)
				slices.SortFunc(targets, func(a, b string) int {
					if a == b {
						return 0
					}
					if compactRefLess(a, b) {
						return -1
					}
					return 1
				})
				choices = append(choices, sourceChoice{ref, kind, targets, cat.evidence[ref]})
			} else if source, ok := cat.subjects[ref]; ok {
				choices = append(choices, sourceChoice{ref, "seed", []string{source.targetRef}, cat.evidence[ref]})
			}
		}
	}
	return encodeWire(struct {
		Task          string                   `json:"task"`
		Repository    string                   `json:"repository"`
		Targets       []targetWire             `json:"targets"`
		Layer         string                   `json:"layer"`
		Observations  []orientationObservation `json:"observations"`
		Result        string                   `json:"result"`
		Sources       []sourceChoice           `json:"citation_sources"`
		NativeSupport []orientationRecord      `json:"native_support"`
		Gaps          []orientationGap         `json:"reader_gaps"`
	}{"repomap.orientation.final.v1", repository, targets, "model_interpretation", observations, section, choices, nativeSupport, gaps})
}

// Final fields are occurrence-aware too: a second answer never overwrites
// the first, and a known omitted role remains an explicit row refusal.
func finalFields(raw []byte) (map[string][]json.RawMessage, error) {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		merged := map[string][]json.RawMessage{}
		objectSeen := false
		for _, value := range values {
			nested, err := finalFields(value)
			if err != nil {
				continue
			}
			objectSeen = true
			for k, rows := range nested {
				merged[k] = append(merged[k], rows...)
			}
		}
		if !objectSeen {
			return nil, fmt.Errorf("orientation final: array has no answer object")
		}
		return merged, nil
	}
	fields, err := contextObject(raw)
	if err != nil {
		return nil, err
	}
	merged := map[string][]json.RawMessage{}
	for _, key := range []string{"roles", "run_recipe", "summary", "summary_refs", "main_flow_target"} {
		if len(fields[key]) > 0 {
			merged[key] = append(merged[key], fields[key]...)
		}
	}
	for _, key := range slices.Sorted(slices.Values(mapKeys(fields))) {
		for _, value := range fields[key] {
			nested, err := finalFields(value)
			if err != nil {
				continue
			}
			for k, rows := range nested {
				merged[k] = append(merged[k], rows...)
			}
		}
	}
	return merged, nil
}
func uniqueFinalText(values []json.RawMessage, fold bool) (string, bool) {
	first, canonical, seen := "", "", false
	for _, raw := range values {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", false
		}
		value = strings.TrimSpace(value)
		normalized := strings.Join(strings.Fields(value), " ")
		if fold {
			normalized = strings.ToLower(normalized)
		}
		if seen && normalized != canonical {
			return "", false
		}
		if !seen {
			first, canonical, seen = value, normalized, true
		}
	}
	return first, true
}
func combinedFinalRefs(values []json.RawMessage) (refList, bool) {
	var combined refList
	for _, value := range values {
		var refs refList
		if err := json.Unmarshal(value, &refs); err != nil {
			return nil, false
		}
		for _, ref := range refs {
			ref = strings.ToLower(strings.TrimSpace(ref))
			if !slices.Contains(combined, ref) {
				combined = append(combined, ref)
			}
		}
	}
	return combined, true
}
func normalizeSection(raw []byte, cat catalog, section string) (normalized, error) {
	return normalizeSectionWithCover(raw, cat, section, true)
}
func normalizeSectionWithCover(raw []byte, cat catalog, section string, requireRoleCover bool) (normalized, error) {
	normalizedJSON, err := llm.NormalizeJSON(raw)
	if err != nil {
		return normalized{}, err
	}
	fields, err := finalFields(normalizedJSON)
	if err != nil {
		return normalized{}, err
	}
	result := newNormalized()
	if section == "repository" {
		summary, valid := uniqueFinalText(fields["summary"], false)
		refs, refsValid := combinedFinalRefs(fields["summary_refs"])
		if valid && refsValid {
			result.acceptSummary(modelResponse{Summary: summary, SummaryRefs: refs}, cat)
		} else {
			result.reject(sectionSummary, json.RawMessage(raw), "conflicting or invalid required summary decision")
		}
		target, valid := uniqueFinalText(fields["main_flow_target"], true)
		if valid {
			encoded, _ := json.Marshal(target)
			result.acceptFlowTarget(encoded, strings.ToLower(target), cat)
		} else {
			result.reject(sectionMainFlow, json.RawMessage(raw), "conflicting or invalid main flow decision")
		}
		return result.done()
	}
	var rows []json.RawMessage
	for _, value := range fields[section] {
		var listed []json.RawMessage
		if err := json.Unmarshal(value, &listed); err != nil {
			result.reject(section, value, "invalid result list")
			continue
		}
		rows = append(rows, listed...)
	}
	type acceptedRow struct {
		raw  json.RawMessage
		slot int
	}
	var acceptedRows []acceptedRow
	for slot, row := range rows {
		values, err := contextObject(row)
		if err != nil {
			result.reject(section, row, "invalid result row")
			continue
		}
		target, targetValid := uniqueFinalText(values["target"], true)
		target = strings.ToLower(target)
		addressed := map[string]bool{}
		for _, value := range values["target"] {
			var ref string
			_ = json.Unmarshal(value, &ref)
			if id, known := cat.targets[strings.ToLower(strings.TrimSpace(ref))]; known {
				addressed[id] = true
			}
		}
		if section == sectionRoles && len(addressed) == 0 {
			continue
		}
		keys := []string{"target", "command", "cwd"}
		optional := []string{"note"}
		if section == sectionRoles {
			keys = []string{"target", "role"}
			optional = []string{"purpose"}
		}
		valid := targetValid
		prepared := map[string]any{}
		for _, key := range keys {
			text, ok := uniqueFinalText(values[key], key == "target" || key == "role")
			valid = valid && ok
			prepared[key] = text
		}
		prepared["target"] = target
		refs, ok := combinedFinalRefs(values["refs"])
		valid = valid && ok
		prepared["refs"] = refs
		for _, key := range optional {
			text, ok := uniqueFinalText(values[key], false)
			if !ok || text != "" && !validSentence(text) {
				refused := json.RawMessage(`null`)
				if len(values[key]) > 0 {
					refused = values[key][0]
				}
				result.reject(section, refused, "invalid or conflicting optional "+key+"; only this cell is dropped")
				text = ""
			}
			prepared[key] = text
		}
		if !valid {
			result.reject(section, row, "conflicting or invalid required decision")
			if section == sectionRoles {
				if result.ambiguousRoles == nil {
					result.ambiguousRoles = map[string]bool{}
				}
				for id := range addressed {
					result.ambiguousRoles[id] = true
				}

			}
			continue
		}
		encoded, _ := encodeWire(prepared)
		if section == sectionRoles {
			probe := newNormalized()
			probe.acceptRole(encoded, cat, fmt.Sprintf("roles[%d]", slot))
			if len(probe.roles) == 0 {
				for _, rejected := range probe.rejected {
					result.reject(rejected.Section, row, rejected.Reason)
				}
				if result.ambiguousRoles == nil {
					result.ambiguousRoles = map[string]bool{}
				}
				for id := range addressed {
					result.ambiguousRoles[id] = true
				}
				continue
			}
		}
		acceptedRows = append(acceptedRows, acceptedRow{encoded, slot})
	}
	for _, row := range acceptedRows {
		if section == sectionRoles {
			result.acceptRole(row.raw, cat, fmt.Sprintf("roles[%d]", row.slot))
		} else {
			result.acceptRecipe(row.raw, cat, fmt.Sprintf("run_recipe[%d]", row.slot))
		}
	}
	if section == sectionRoles && requireRoleCover {
		for ref, id := range cat.targets {
			found := false
			for _, role := range result.roles {
				found = found || role.TargetID == id
			}
			if !found {
				encoded, _ := json.Marshal(ref)
				result.reject(sectionRoles, encoded, "known target has no accepted role decision")
			}
		}
	}
	if section == sectionRunRecipe {
		var unique []RecipeStep
		for _, step := range result.recipe {
			found := false
			for i, prior := range unique {
				if prior.TargetID == step.TargetID && prior.Command == step.Command && prior.Cwd == step.Cwd && prior.Note == step.Note {
					unique[i].FactIDs = unionRefs(prior.FactIDs, step.FactIDs)
					found = true
					break
				}
			}
			if !found {
				unique = append(unique, step)
			}
		}
		result.recipe = unique
	}
	return result.done()
}

// The final request may choose only sources actually advertised beside the
// accepted observations. Original evidence obligations survive this closure.
func contextCatalog(original catalog, targets []targetWire, observations []orientationObservation) catalog {
	closed := newCatalog()
	closed.requiredEvidence = map[string]bool{}
	for _, target := range targets {
		closed.targets[target.Ref] = original.targets[target.Ref]
		closed.requiredEvidence[target.Ref] = original.holdsEvidence(target.Ref)
	}
	for _, observation := range observations {
		for _, ref := range observation.Sources {
			if entry, ok := original.facts[ref]; ok {
				closed.facts[ref] = entry
			}
			if entry, ok := original.subjects[ref]; ok {
				closed.subjects[ref] = entry
			}
		}
	}
	return closed
}

// Evidence obligations come from the original request; only model-selected
// original sources may satisfy them. No source is borrowed to fill a gap.
func roleCitationUnavailable(original, selected catalog, target string) bool {
	if !original.holdsEvidence(target) {
		return false
	}
	for _, entry := range selected.facts {
		if selected.ownFact(entry, target) {
			return false
		}
	}
	for _, entry := range selected.subjects {
		if entry.targetRef == target {
			return false
		}
	}
	return true
}

func askPartitionedOverview(ctx context.Context, executor llm.Executor, provider llm.Provider, input Input, digests []string, overview overviewRequest, cat catalog) (normalized, error) {
	accepted := newNormalized()
	var contexts []struct {
		Target  targetWire
		Context orientationContext
	}
	complete := true
	var originalGaps []orientationGap
	for _, target := range overview.Targets {
		records := orientationRecords(overview, target)
		reading, rejected, err := readOrientationContext(ctx, executor, provider, input, digests, target, records)
		if err != nil {
			return normalized{}, err
		}
		accepted.rejected = append(accepted.rejected, rejected...)
		originalGaps = append(originalGaps, reading.Gaps...)
		complete = complete && reading.permits("repository")
		contexts = append(contexts, struct {
			Target  targetWire
			Context orientationContext
		}{target, reading})
		for _, section := range []string{"roles", "run_recipe"} {
			if !reading.permits(section) {
				accepted.rejected = append(accepted.rejected, RejectedRow{Stage: StageName, Section: section, Raw: json.RawMessage(`null`), Reason: "required complete reader question unavailable; independent decisions are kept"})
				continue
			}
			observations := reading.forSection(section)
			local := contextCatalog(cat, []targetWire{target}, observations)
			if section == "roles" && roleCitationUnavailable(cat, local, target.Ref) {
				raw, _ := json.Marshal(map[string]string{"target": target.Ref})
				accepted.rejected = append(accepted.rejected, RejectedRow{Stage: StageName, Section: section, Raw: raw, Reason: "required own source citations unavailable in selected context; independent decisions are kept"})
				continue
			}
			wire, err := finalContextWire(input.RepositoryName, []targetWire{target}, observations, cat, section, reading.gapsForSection(section)...)
			if err != nil {
				return normalized{}, err
			}
			outcome, err := ask(ctx, executor, provider, input, digests, wire, target.Ref+"/"+section, sectionPrompt(section), sectionExample(section), func(raw []byte) (normalized, error) { return normalizeSection(raw, local, section) })
			if err != nil {
				rows, ok := refusal(ctx, err, outcome, section, len(wire))
				if !ok {
					return normalized{}, err
				}
				accepted.rejected = append(accepted.rejected, rows...)
				continue
			}
			accepted.roles = append(accepted.roles, outcome.Value.roles...)
			accepted.recipe = append(accepted.recipe, outcome.Value.recipe...)
			accepted.rejected = append(accepted.rejected, outcome.Value.rejected...)
		}
	}
	if !complete {
		accepted.rejected = append(accepted.rejected, RejectedRow{Stage: StageName, Section: "repository", Raw: json.RawMessage(`null`), Reason: "complete repository interpretation unavailable; independent target decisions are kept"})
		return accepted, nil
	}
	// A global interpreter sees typed signals as model interpretations, beside
	// separately retained original native support. No signal inherits its input
	// inventory as proof. Empty local questions never establish global absence.
	global := contextSignalRecords(contexts)
	if len(global) == 0 {
		accepted.rejected = append(accepted.rejected, RejectedRow{Stage: StageName, Section: "repository", Raw: json.RawMessage(`null`), Reason: "complete input acknowledged without selected reader signals; repository interpretation unavailable"})
		return accepted, nil
	}
	reading, rejected, err := readOrientationContext(ctx, executor, provider, input, digests, targetWire{Name: input.RepositoryName}, global)
	if err != nil {
		return normalized{}, err
	}
	accepted.rejected = append(accepted.rejected, rejected...)
	if !reading.permits("repository") {
		accepted.rejected = append(accepted.rejected, RejectedRow{Stage: StageName, Section: "repository", Raw: json.RawMessage(`null`), Reason: "required complete global reader question unavailable"})
		return accepted, nil
	}
	// The global interpreter cannot erase original target refusals by omitting
	// them from its selected signals. Keep those exact diagnostic values once.
	reading.Gaps = append(originalGaps, reading.Gaps...)
	observations := reading.forSection("repository")
	wire, err := finalContextWire(input.RepositoryName, overview.Targets, observations, cat, "repository", reading.gapsForSection("repository")...)
	if err != nil {
		return normalized{}, err
	}
	outcome, err := ask(ctx, executor, provider, input, digests, wire, "repository", sectionPrompt("repository"), sectionExample("repository"), func(raw []byte) (normalized, error) {
		return normalizeSection(raw, contextCatalog(cat, overview.Targets, observations), "repository")
	})
	if err != nil {
		rows, ok := refusal(ctx, err, outcome, "repository", len(wire))
		if !ok {
			return normalized{}, err
		}
		accepted.rejected = append(accepted.rejected, rows...)
		return accepted, nil
	}
	accepted.summary, accepted.summaryRefs, accepted.flowTarget = outcome.Value.summary, outcome.Value.summaryRefs, outcome.Value.flowTarget
	accepted.rejected = append(accepted.rejected, outcome.Value.rejected...)
	return accepted, nil
}

func isEnvelopeRefusal(err error) bool {
	var resource *llm.ResourceLimitError
	return errors.As(err, &resource)
}
