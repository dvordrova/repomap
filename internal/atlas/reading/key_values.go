package reading

import (
	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A key's values (owner's rule K3, values.go). A word call comparing a
// literal element of a value compares what follows a key when a call
// written before it in the same declaration compares a lower element of
// that value: Redis's strcasecmp(argv[1],"debug") after
// strcasecmp(argv[0],"loglevel"). Such a call waits for its key's answer
// (readCalls): when the key is an entry, the call is its value, of its kind,
// and is not asked, since the step before decided it and foldValues nests
// it; otherwise it is asked on its own, with its key as written.

// keyCall is, for a word call comparing a literal element of a value, the
// last call written before it in the same declaration that compares a lower
// element of that value, as written, and its site; "" when there is none.
func (r *reader) keyCall(files map[string]*lines.CallFile, place atlas.Place, call atlas.SymbolCall) (string, sourceSite) {
	element := comparedElement(call.SourceArguments)
	if element == nil || place.Symbol == nil {
		return "", sourceSite{}
	}
	site := sourceSite{place.Path, call.Line, call.Column}
	var key *atlas.SymbolCall
	var keySite sourceSite
	for i := range place.Symbol.Calls {
		other := &place.Symbol.Calls[i]
		if len(intersectTargets(callTargets(place, call), callTargets(place, *other))) != len(callTargets(place, call)) {
			continue
		}
		at := sourceSite{place.Path, other.Line, other.Column}
		if other.Kind != string(programindex.RelationInvokesExternal) || len(other.Values) == 0 || at.compare(site) >= 0 {
			continue
		}
		if lower := comparedElement(other.SourceArguments); lower != nil && lower.base == element.base && lower.index < element.index && (key == nil || at.compare(keySite) > 0) {
			key, keySite = other, at
		}
	}
	if key == nil {
		return "", sourceSite{}
	}
	text := r.sourceText(files, place.Path, key.Line, key.Column)
	if text == "" {
		text = key.Name
	}
	return text, keySite
}
