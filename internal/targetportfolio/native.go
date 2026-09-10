package targetportfolio

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/dvordrova/repomap/internal/corpus"
)

// Observation is source evidence supplied by an adapter or a guidance
// extractor. It never carries an architectural decision.
type Observation struct {
	Kind   string   `json:"kind"`
	Path   string   `json:"path,omitempty"`
	Line   int      `json:"line,omitempty"`
	Values []string `json:"values,omitempty"`
}

type NativeOwner struct {
	Ref  string `json:"ref"`
	Name string `json:"name"`
	Kind string `json:"kind"`
	// SameLaunch is adapter-observed equality of the complete argument-free
	// launch callable. The original declarations remain in row evidence.
	SameLaunch bool `json:"same_launch,omitempty"`
}

// NativeCandidate is one exact native target, even when its representative
// file also represents another target. Refs are assigned locally for this
// selection; native IDs and compiler edges never cross the provider boundary.
type NativeCandidate struct {
	Ref          string        `json:"ref"`
	FileRef      corpus.FileID `json:"file_ref"`
	Language     string        `json:"language"`
	Kind         string        `json:"kind"`
	Name         string        `json:"name"`
	Root         string        `json:"root"`
	Evidence     []Observation `json:"evidence,omitempty"`
	EvidenceRefs []string      `json:"evidence_refs,omitempty"`
	SeedOwners   []NativeOwner `json:"seed_owners,omitempty"`
}

type NativeDecision struct {
	Ref      string `json:"ref"`
	Decision string `json:"decision"`
}

// Placement includes the exact evidence and any refused decision for the
// journal. Missing or invalid answers retain a standalone target; only a
// positive, structurally valid seed_of can remove an independent run.
type Placement struct {
	Candidate NativeCandidate `json:"candidate"`
	Decision  string          `json:"decision"`
	Rejected  string          `json:"rejected,omitempty"`
	Reason    string          `json:"reason,omitempty"`
}

func CompileWithNativeAuthority(snapshot corpus.Snapshot, candidates []Candidate, required []corpus.FileID, native []NativeCandidate) (Compilation, error) {
	return compile(snapshot, candidates, false, nil, true, required, native)
}

func cloneNative(values []NativeCandidate) []NativeCandidate {
	if values == nil {
		return nil
	}
	wire, _ := json.Marshal(values)
	var result []NativeCandidate
	_ = json.Unmarshal(wire, &result)
	return result
}

func validateNative(compilation Compilation) error {
	rows, evidence := nativeRequest(compilation.native)
	if !reflect.DeepEqual(rows, compilation.Request.NativeTargets) || !reflect.DeepEqual(evidence, compilation.Request.Observations) {
		return fmt.Errorf("target portfolio: native evidence authority mismatch")
	}
	files := make(map[corpus.FileID]bool)
	for _, candidate := range compilation.candidates {
		files[candidate.FileRef] = true
	}
	seen := make(map[string]bool)
	for _, row := range compilation.native {
		if !files[row.FileRef] || row.Ref == "" || seen[row.Ref] || row.Name == "" || row.Language == "" || row.Kind == "" {
			return fmt.Errorf("target portfolio: invalid native target authority")
		}
		seen[row.Ref] = true
		for _, owner := range row.SeedOwners {
			if owner.Ref == row.Ref || owner.Ref == "" || owner.Name == "" {
				return fmt.Errorf("target portfolio: invalid seed owner authority")
			}
		}
	}
	return nil
}

func nativeSubset(compilation Compilation, candidates []Candidate) []NativeCandidate {
	files := make(map[corpus.FileID]bool)
	for _, candidate := range candidates {
		files[candidate.FileRef] = true
	}
	var rows []NativeCandidate
	for _, row := range compilation.native {
		if files[row.FileRef] {
			rows = append(rows, row)
		}
	}
	return rows
}

func nativeDecisions(rows []NativeCandidate, answers []NativeDecision, final bool) []Placement {
	known := make(map[string]NativeCandidate)
	for _, row := range rows {
		known[row.Ref] = row
	}
	byRef := make(map[string][]string)
	for _, answer := range answers {
		if _, ok := known[answer.Ref]; !ok {
			continue
		}
		if !slices.Contains(byRef[answer.Ref], answer.Decision) {
			byRef[answer.Ref] = append(byRef[answer.Ref], answer.Decision)
		}
	}
	result := make([]Placement, 0, len(rows))
	for _, row := range rows {
		answers := byRef[row.Ref]
		p := Placement{Candidate: row, Decision: "standalone"}
		if len(answers) != 1 {
			p.Reason = "decision not received: missing or conflicting target decision"
			p.Rejected = strings.Join(answers, ", ")
		} else {
			answer := answers[0]
			valid := slices.Contains([]string{"standalone", "shared_code", "tool", "example"}, answer)
			if answer == "shared_code" && row.Kind != "library" && row.Kind != "module_library" {
				valid = false
			}
			if ownerRef, seed := strings.CutPrefix(answer, "seed_of:"); seed {
				for _, owner := range row.SeedOwners {
					if owner.Ref == ownerRef {
						valid = true
					}
				}
				if final && (len(byRef[ownerRef]) != 1 || byRef[ownerRef][0] != "standalone") {
					valid = false
				}
			}
			if valid {
				p.Decision = answer
			} else {
				p.Rejected, p.Reason = answer, "decision not received: invalid role or seed owner (must be an advertised standalone target)"
			}
		}
		result = append(result, p)
	}
	return result
}

// nativeRequest encodes repeated declarations and guide quotations once per
// complete request window. Each child rebuilds its own closed evidence refs.
type NamedObservation struct {
	Ref string `json:"ref"`
	Observation
}

func nativeRequest(native []NativeCandidate) ([]NativeCandidate, []NamedObservation) {
	rows := cloneNative(native)
	var evidence []NamedObservation
	byWire := make(map[string]string)
	for i := range rows {
		for _, observation := range rows[i].Evidence {
			wire, _ := json.Marshal(observation)
			ref, ok := byWire[string(wire)]
			if !ok {
				ref = fmt.Sprintf("e%d", len(evidence)+1)
				byWire[string(wire)] = ref
				evidence = append(evidence, NamedObservation{Ref: ref, Observation: observation})
			}
			rows[i].EvidenceRefs = append(rows[i].EvidenceRefs, ref)
		}
		rows[i].Evidence = nil
	}
	return rows, evidence
}
