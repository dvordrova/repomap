package reading

import (
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
)

// wordCallRecord is one call outside tests giving words to, or calling without
// words, an outside symbol asked what its words become, with what the
// reading made of it: an entry of kind, or unsure (undecided, no_words).
type wordCallRecord struct {
	targets      []string
	objectID     string
	path         string
	line, column int
	symbol       string
	outcome      string
	kind         string
}

const (
	wordEntry     = "entry"
	wordUndecided = "undecided"
	wordNoWords   = "no_words"
)

func (r *reader) recordWordCall(place atlas.Place, objectID string, line, column int, symbol, outcome, kind string) {
	r.wordCalls = append(r.wordCalls, wordCallRecord{targets: slices.Clone(place.TargetIDs), objectID: objectID, path: place.Path, line: line, column: column, symbol: symbol, outcome: outcome, kind: kind})
}

// launchEvidence is a target's unsure calls and idioms, in source order.
func (r *reader) launchEvidence(targetID string) ([]atlas.UnsureCall, []atlas.Idiom) {
	var unsure []atlas.UnsureCall
	type idiom struct {
		kind           string
		entries, calls int
		objects        []string
	}
	idioms := map[string]*idiom{}
	seen := map[sourceSite]bool{}
	for _, call := range r.wordCalls {
		if !slices.Contains(call.targets, targetID) {
			continue
		}
		site := sourceSite{call.path, call.line, call.column}
		if seen[site] {
			continue
		}
		seen[site] = true
		if call.outcome != wordEntry {
			unsure = append(unsure, atlas.UnsureCall{ObjectID: call.objectID, Path: call.path, LineNo: call.line, Column: call.column, Symbol: call.symbol, Reason: call.outcome})
		}
		if call.kind == "" {
			continue
		}
		at := idioms[call.symbol]
		if at == nil {
			at = &idiom{kind: call.kind}
			idioms[call.symbol] = at
		}
		at.calls++
		if call.outcome == wordEntry {
			at.entries++
			if call.objectID != "" && !slices.Contains(at.objects, call.objectID) {
				at.objects = append(at.objects, call.objectID)
			}
		}
	}
	sort.Slice(unsure, func(i, j int) bool {
		a, b := unsure[i], unsure[j]
		return sourceSite{a.Path, a.LineNo, a.Column}.compare(sourceSite{b.Path, b.LineNo, b.Column}) < 0
	})
	var result []atlas.Idiom
	for symbol, at := range idioms {
		if at.entries == 0 {
			continue
		}
		result = append(result, atlas.Idiom{Symbol: symbol, Kind: at.kind, Entries: at.entries, Calls: at.calls, ObjectIDs: at.objects})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Symbol < result[j].Symbol })
	return unsure, result
}
