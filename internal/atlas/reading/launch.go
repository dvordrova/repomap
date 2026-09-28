package reading

import (
	"slices"
	"sort"

	"github.com/dvordrova/repomap/internal/atlas"
)

// wordCallRecord is one call outside tests giving words to an outside
// symbol, or giving none to a symbol whose words are an entry at another
// call, with what the reading made of it: an entry of kind, none, or
// unsure (undecided, no_words).
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
	wordNone      = "none"
	wordUndecided = atlas.UnsureUndecided
	wordNoWords   = atlas.UnsureNoWords
)

func (r *reader) recordWordCall(place atlas.Place, objectID string, line, column int, symbol, outcome, kind string) {
	r.wordCalls = append(r.wordCalls, wordCallRecord{targets: slices.Clone(place.TargetIDs), objectID: objectID, path: place.Path, line: line, column: column, symbol: symbol, outcome: outcome, kind: kind})
}

// launchEvidence is a target's unsure calls and idioms, in source order.
// An idiom is one outside symbol's word calls that made entries of one
// kind, counted against every call of the symbol the reading recorded.
func (r *reader) launchEvidence(targetID string) ([]atlas.UnsureCall, []atlas.Idiom) {
	var unsure []atlas.UnsureCall
	type idiom struct {
		symbol, kind string
		entries      int
		objects      []string
	}
	idioms := map[[2]string]*idiom{}
	calls := map[string]int{}
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
		calls[call.symbol]++
		switch call.outcome {
		case wordUndecided, wordNoWords:
			unsure = append(unsure, atlas.UnsureCall{ObjectID: call.objectID, Path: call.path, LineNo: call.line, Column: call.column, Symbol: call.symbol, Reason: call.outcome})
		case wordEntry:
			key := [2]string{call.symbol, call.kind}
			at := idioms[key]
			if at == nil {
				at = &idiom{symbol: call.symbol, kind: call.kind}
				idioms[key] = at
			}
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
	for _, at := range idioms {
		result = append(result, atlas.Idiom{Symbol: at.symbol, Kind: at.kind, Entries: at.entries, Calls: calls[at.symbol], ObjectIDs: at.objects})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Symbol != result[j].Symbol {
			return result[i].Symbol < result[j].Symbol
		}
		return result[i].Kind < result[j].Kind
	})
	return unsure, result
}
