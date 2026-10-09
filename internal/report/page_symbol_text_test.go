package report

import (
	"testing"

	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/programindex"
)

// A variable's tile says what follows its name in the adapter's words, and
// a Clojure def's words are its name alone: othello's alpha-min had read
// "alpha-min: alpha-min". Python's value and Go's type still stand.
func TestAVariableTileSaysNoNameTwice(t *testing.T) {
	for _, c := range []struct{ name, signature, want string }{
		{"othello.ai.search/alpha-min", "alpha-min", ""},
		{"fixture_app.levels.READ_LIMIT", "READ_LIMIT = 8", " = 8"},
		{"Model", "Model gorm.Model", ": gorm.Model"},
		{"a", "any", ": any"},
		{"i", "int", ": int"},
		{"n", "number", ": number"},
		{"body", "body: string;", ": string"},
		{"a", "a = 8", " = 8"},
	} {
		object := &groupindex.ObjectFacts{Name: c.name, Kind: programindex.ObjectVariable, Signature: c.signature}
		if got := symbolText(object, c.name); got != c.want {
			t.Errorf("%s (%q) reads %q, want %q", c.name, c.signature, got, c.want)
		}
	}
}
