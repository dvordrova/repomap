package report

import (
	"regexp"
	"strings"

	"github.com/dvordrova/repomap/internal/facts"
)

// "Where do the parts talk and on which port" is one of the questions this
// page exists to answer, and the answer was on it — buried in a target's
// configuration table, several screens below the question. These rows lift the
// port-carrying facts to the place the question is asked. Nothing new is
// derived: each row is one existing fact at its own anchor.
type pageAddress struct {
	Target string
	Key    string
	Value  string
	Note   string
	Anchor *pageAnchor
}

// hostPortValue matches a value that names a port, whether written as a URL,
// a host:port pair, or a bare listen address.
var hostPortValue = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9+.-]*://)?[A-Za-z0-9_.-]*:\d{2,5}(?:/\S*)?$`)

func (builder *pageBuilder) addresses(view *pageView) {
	if builder.data.Facts == nil {
		return
	}
	for _, kind := range []facts.Kind{
		facts.KindListenAddress, facts.KindManifest, facts.KindConfigRead,
	} {
		for _, fact := range builder.data.Facts.OfKind(kind) {
			if kind != facts.KindListenAddress && !addressFact(fact) {
				continue
			}
			row := pageAddress{
				Key: fact.Key, Value: fact.Value, Anchor: builder.links.factAnchor(fact),
			}
			if kind == facts.KindListenAddress {
				row.Key = "listens on"
				if fact.Symbol != "" {
					row.Note = "in " + fact.Symbol
				}
			}
			if section, known := builder.byFacts[fact.TargetID]; known {
				row.Target = section.Label
			}
			if row.Value == "" {
				// Saying "no default" would be a claim this page cannot make:
				// a Python `Field(default=8080, env='APP_PORT')` has one, and
				// the index records no value for a numeric literal, so there
				// is nothing to read. The anchor is the answer; it stands on
				// its own without a sentence that might be wrong.
				row.Note = ""
			}
			view.Addresses = append(view.Addresses, row)
		}
	}
}

// addressFact keeps a fact that names where something listens or connects: a
// value carrying a port, or a key whose name is a port with no default.
func addressFact(fact facts.Fact) bool {
	if hostPortValue.MatchString(fact.Value) {
		return true
	}
	key := strings.ToLower(fact.Key)
	return fact.Value == "" &&
		(key == "port" || strings.HasSuffix(key, "_port") || strings.HasSuffix(key, ".port"))
}
