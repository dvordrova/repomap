package orientation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/llm"
)

var contextQuestions = []string{"responsibilities", "interfaces", "launch_requirements", "uncertainty"}

type orientationBasis struct {
	Ref   string `json:"ref"`
	Layer string `json:"layer"`
}

// Provenance is the complete original logical input, retained once. It is
// neither a model-selected support set nor a claim that every job was explained.
// A gap describes a refused interpretation, never a repaired claim or native
// evidence. Addresses remain explicitly tied to the ORIGINAL prepared scope.
// They are not current request-local supporting choices in a later phase.
type orientationGap struct {
	Question string             `json:"question"`
	Reason   string             `json:"reason"`
	Target   string             `json:"original_target,omitempty"`
	Scope    orientationScope   `json:"original_scope"`
	Basis    []orientationBasis `json:"original_basis,omitempty"`
	Supports []orientationBasis `json:"attempted_support,omitempty"`
}

type orientationContext struct {
	Observations []orientationObservation `json:"signals"`
	Provenance   []orientationRecord      `json:"-"`
	Questions    map[string]bool          `json:"questions"`
	Rejected     []RejectedRow            `json:"-"`
	Gaps         []orientationGap         `json:"-"`
}

func (value orientationContext) ResponseRejections() []llm.ResponseRejection {
	var out []llm.ResponseRejection
	for _, row := range value.Rejected {
		out = append(out, llm.ResponseRejection{Kind: "orientation_signal_refused", Count: 1, Reason: row.Reason, Samples: []string{row.Section}})
	}
	return out
}

func (value orientationContext) permits(section string) bool {
	questions := contextQuestions
	if section == "roles" {
		questions = []string{"responsibilities", "interfaces", "uncertainty"}
	}
	if section == "run_recipe" {
		questions = []string{"launch_requirements", "uncertainty"}
	}
	for _, q := range questions {
		if !value.Questions[q] {
			return false
		}
	}
	return true
}

func (value orientationContext) forSection(section string) []orientationObservation {
	var out []orientationObservation
	for _, row := range value.Observations {
		if section == "roles" && row.Question == "launch_requirements" {
			continue
		}
		if section == "run_recipe" && row.Question != "launch_requirements" && row.Question != "uncertainty" {
			continue
		}
		out = append(out, row)
	}
	return out
}

func (value orientationContext) gapsForSection(section string) []orientationGap {
	var out []orientationGap
	for _, gap := range value.Gaps {
		if section == "roles" && gap.Question == "launch_requirements" {
			continue
		}
		if section == "run_recipe" && gap.Question != "launch_requirements" && gap.Question != "uncertainty" && gap.Question != "all_questions" {
			continue
		}
		out = append(out, gap)
	}
	return out
}

func signalGaps(input orientationReading, rejected []RejectedRow) []orientationGap {
	known := map[string]orientationRecord{}
	for _, r := range input.Records {
		known[strings.ToLower(r.Ref)] = r
	}
	var out []orientationGap
	for _, row := range rejected {
		gap := orientationGap{Question: strings.TrimPrefix(row.Section, "context/"), Reason: row.Reason, Target: input.Target.Ref, Scope: orientationScope{Ref: "w1", Meaning: "all_supplied_records"}}
		if len(input.Records) > 0 {
			gap.Scope.First = input.Records[0].Ref
			gap.Scope.Last = input.Records[len(input.Records)-1].Ref
		}
		fields, err := contextObject(row.Raw)
		if err == nil {
			_, basis, _ := signalRefs(fields["basis"], known)
			_, supports, _ := signalRefs(fields["supports"], known)
			for _, record := range input.Records {
				ref := strings.ToLower(record.Ref)
				if slices.Contains(basis, ref) {
					gap.Basis = append(gap.Basis, orientationBasis{record.Ref, record.Layer})
				}
				if slices.Contains(supports, ref) {
					gap.Supports = append(gap.Supports, orientationBasis{record.Ref, record.Layer})
				}
			}
		}
		out = append(out, gap)
	}
	return out
}

func signalWrappers(raw []byte) ([]map[string][]json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		var out []map[string][]json.RawMessage
		for _, value := range values {
			nested, _ := signalWrappers(value)
			out = append(out, nested...)
		}
		return out, nil
	}
	fields, err := contextObject(raw)
	if err != nil {
		return nil, err
	}
	var out []map[string][]json.RawMessage
	if len(fields["scope"]) > 0 {
		out = append(out, fields)
	}
	for _, key := range slices.Sorted(slices.Values(mapKeys(fields))) {
		for _, value := range fields[key] {
			nested, _ := signalWrappers(value)
			out = append(out, nested...)
		}
	}
	return out, nil
}

// Unknown refs are discarded. Every occurrence of a required list is checked;
// its first value never hides an addressed malformed/conflicting later value.
func signalRefs(values []json.RawMessage, known map[string]orientationRecord) ([]string, []string, bool) {
	var first, addressed []string
	valid, seen := true, false
	for _, raw := range values {
		var refs []string
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			if json.Unmarshal(raw, &refs) != nil {
				var text string
				if json.Unmarshal(raw, &text) != nil {
					valid = false
					// A malformed array may still address known refs. Preserve that
					// union only to refuse the attempted row, never to accept its list.
					for _, ref := range refs {
						ref = strings.ToLower(strings.TrimSpace(ref))
						if _, ok := known[ref]; ok {
							addressed = unionRefs(addressed, []string{ref})
						}
					}
					continue
				}
				refs = strings.Fields(text)
			}
		}
		var normalized []string
		for _, ref := range refs {
			ref = strings.ToLower(strings.TrimSpace(ref))
			if _, ok := known[ref]; ok {
				normalized = unionRefs(normalized, []string{ref})
				addressed = unionRefs(addressed, []string{ref})
			}
		}
		slices.Sort(normalized)
		if seen && !slices.Equal(first, normalized) {
			valid = false
		}
		if !seen {
			first = normalized
			seen = true
		}
	}
	return first, addressed, valid && seen
}

func signalRows(raw json.RawMessage, question string, input orientationReading) ([]orientationObservation, []RejectedRow, error) {
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, nil, err
	}
	known := map[string]orientationRecord{}
	for _, row := range input.Records {
		known[strings.ToLower(row.Ref)] = row
	}
	var accepted []orientationObservation
	var rejected []RejectedRow
	for _, raw := range rows {
		fields, err := contextObject(raw)
		if err != nil {
			continue
		}
		basis, addressed, basisOK := signalRefs(fields["basis"], known)
		supports, supportAddressed, supportOK := signalRefs(fields["supports"], known)
		if len(addressed) == 0 && len(supportAddressed) == 0 {
			continue
		}
		basisOK = basisOK && len(basis) > 0
		text, textOK := contextString(fields["text"])
		status, statusOK := signalClosedText(fields["status"])
		statusOK = statusOK && (status == "supported" || status == "interpretation" || status == "uncertain")
		observation := orientationObservation{Text: text, Question: question, Status: status}
		for _, ref := range basis {
			observation.Basis = append(observation.Basis, orientationBasis{ref, known[ref].Layer})
		}
		for _, record := range input.Records {
			if !slices.Contains(supports, strings.ToLower(record.Ref)) {
				continue
			}
			if record.Layer != "native" || record.Value == nil || len(record.Sources) == 0 {
				supportOK = false
				continue
			}
			// Keep the complete actual selected native value, including related
			// facts, endpoints, seed headers, call position and evidence uses.
			observation.Native = append(observation.Native, record)
			observation.SupportRecords = append(observation.SupportRecords, record.Ref)
			observation.Sources = unionRefs(observation.Sources, nativeCitationRefs(record))
		}
		if status == "supported" && len(observation.Native) == 0 {
			supportOK = false
		}
		if !basisOK || !textOK || !statusOK || !supportOK {
			rejected = append(rejected, RejectedRow{Stage: StageName, Section: "context/" + question, Raw: slices.Clone(raw), Reason: "known signal has missing, conflicting or invalid basis, text, status or native support"})
			continue
		}
		identity := signalIdentity([]orientationObservation{observation}, nil)
		if !slices.ContainsFunc(accepted, func(previous orientationObservation) bool {
			return bytes.Equal(signalIdentity([]orientationObservation{previous}, nil), identity)
		}) {
			accepted = append(accepted, observation)
		}
	}
	return accepted, rejected, nil
}

func signalClosedText(values []json.RawMessage) (string, bool) {
	var normalized []json.RawMessage
	for _, raw := range values {
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return "", false
		}
		encoded, _ := json.Marshal(strings.ToLower(strings.TrimSpace(value)))
		normalized = append(normalized, encoded)
	}
	return contextString(normalized)
}

func signalIdentity(rows []orientationObservation, rejected []RejectedRow) []byte {
	// Signal-array order is presentation, not execution order. Compare the
	// normalized set of owning decisions, preserving first accepted order.
	var identities []string
	for _, row := range rows {
		row.Text = strings.Join(strings.Fields(row.Text), " ")
		raw, _ := encodeWire(row)
		identities = unionRefs(identities, []string{string(raw)})
	}
	slices.Sort(identities)
	// Invalid rows remain refused. Irrelevant extra metadata and harmless
	// repetitions cannot poison independently identical accepted neighbours.
	var gaps []string
	for _, row := range rejected {
		gaps = unionRefs(gaps, []string{row.Section + "\x00" + row.Reason})
	}
	slices.Sort(gaps)
	raw, _ := encodeWire(struct{ Rows, Gaps []string }{identities, gaps})
	return raw
}

func decodeOrientationContext(raw []byte, input orientationReading) (orientationContext, error) {
	raw, err := llm.NormalizeJSON(raw)
	if err != nil {
		return orientationContext{}, err
	}
	wrappers, err := signalWrappers(raw)
	if err != nil {
		return orientationContext{}, err
	}
	result := orientationContext{Provenance: input.Records, Questions: map[string]bool{}}
	seenScope := false
	identities := map[string][]byte{}
	poison := map[string]bool{}
	byQuestion := map[string][]orientationObservation{}
	for _, fields := range wrappers {
		addressed := false
		for _, value := range fields["scope"] {
			var ref string
			if json.Unmarshal(value, &ref) == nil {
				ref = strings.ToLower(strings.TrimSpace(ref))
				addressed = addressed || ref == "w1"
			}
		}
		if !addressed {
			continue
		}
		scope, valid := signalClosedText(fields["scope"])
		if !valid || !strings.EqualFold(scope, "w1") {
			return result, fmt.Errorf("orientation context: invalid complete input scope")
		}
		seenScope = true
		var signalFields []map[string][]json.RawMessage
		for _, value := range fields["signals"] {
			f, err := contextObject(value)
			if err != nil {
				return result, fmt.Errorf("orientation context: malformed signals object")
			}
			signalFields = append(signalFields, f)
		}
		for _, q := range contextQuestions {
			var values []json.RawMessage
			for _, f := range signalFields {
				values = append(values, f[q]...)
			}
			if len(values) == 0 {
				poison[q] = true
				result.Rejected = append(result.Rejected, RejectedRow{Stage: StageName, Section: "context/" + q, Raw: json.RawMessage(`null`), Reason: "required reader question missing"})
				continue
			}
			for _, value := range values {
				rows, rejected, err := signalRows(value, q, input)
				if err != nil {
					poison[q] = true
					result.Rejected = append(result.Rejected, RejectedRow{Stage: StageName, Section: "context/" + q, Raw: slices.Clone(value), Reason: "required reader question is not an array"})
					continue
				}
				identity := signalIdentity(rows, rejected)
				if previous, seen := identities[q]; seen && !bytes.Equal(previous, identity) {
					poison[q] = true
					result.Rejected = append(result.Rejected, RejectedRow{Stage: StageName, Section: "context/" + q, Raw: slices.Clone(value), Reason: "conflicting required reader question"})
				}
				if _, seen := identities[q]; !seen {
					identities[q] = identity
					byQuestion[q] = rows
				}
				result.Rejected = append(result.Rejected, rejected...)
			}
		}
	}
	if !seenScope || len(input.Records) == 0 {
		return result, fmt.Errorf("orientation context: missing complete input scope")
	}
	for _, q := range contextQuestions {
		result.Questions[q] = !poison[q]
		if result.Questions[q] {
			result.Observations = append(result.Observations, byQuestion[q]...)
		}
	}
	result.Gaps = signalGaps(input, result.Rejected)
	if !slices.ContainsFunc(contextQuestions, func(q string) bool { return result.Questions[q] }) {
		return result, &noOutputError{rejected: result.Rejected}
	}
	return result, nil
}

// Native support is advertised as original native records, not inherited from
// the provenance of a previous model statement. Every actual source value stays
// beside its use; request-local refs are newly closed for this complete input.
func contextSignalRecords(contexts []struct {
	Target  targetWire
	Context orientationContext
}) []orientationRecord {
	var records []orientationRecord
	add := func(layer, kind string, value any, sources []string) {
		records = append(records, orientationRecord{fmt.Sprintf("d%d", len(records)+1), layer, kind, value, sources})
	}
	seen := map[string]bool{}
	for _, item := range contexts {
		if len(item.Context.Gaps) > 0 {
			add("model_interpretation", "reader_gaps", struct {
				Target targetWire       `json:"target"`
				Gaps   []orientationGap `json:"refused_signals"`
			}{item.Target, item.Context.Gaps}, nil)
		}
		for _, signal := range item.Context.Observations {
			layers := []string{}
			for _, basis := range signal.Basis {
				layers = unionRefs(layers, []string{basis.Layer})
			}
			add("model_interpretation", "reader_signal", struct {
				Target   targetWire `json:"target"`
				Question string     `json:"question"`
				Text     string     `json:"text"`
				Status   string     `json:"status"`
				Layers   []string   `json:"original_basis_layers"`
			}{item.Target, signal.Question, signal.Text, signal.Status, layers}, nil)
			for _, native := range signal.Native {
				identity, _ := encodeWire(native)
				if seen[string(identity)] {
					continue
				}
				seen[string(identity)] = true
				add("native", native.Kind, native.Value, native.Sources)
			}
		}
	}
	return records
}

// Only full native values actually present in this selected record expand the
// final citation choices. A links-only ref, target metadata or earlier model
// inventory never supplies a fact. The final decision still selects support.
func nativeRecordFacts(record orientationRecord) []factWire {
	if record.Layer != "native" || record.Kind != "fact" {
		return nil
	}
	raw, err := encodeWire(record.Value)
	if err != nil {
		return nil
	}
	var value struct {
		Fact    factWire   `json:"fact"`
		Related []factWire `json:"related_facts"`
	}
	if json.Unmarshal(raw, &value) != nil {
		return nil
	}
	var facts []factWire
	if value.Fact.Ref != "" {
		facts = append(facts, value.Fact)
	}
	for _, fact := range value.Related {
		if fact.Ref != "" {
			facts = append(facts, fact)
		}
	}
	return facts
}
func nativeCitationRefs(record orientationRecord) []string {
	refs := slices.Clone(record.Sources)
	for _, fact := range nativeRecordFacts(record) {
		refs = unionRefs(refs, []string{fact.Ref})
	}
	return refs
}
