package reading

import (
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/sourcevalue"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// keySource is a configuration loader comparing each line's words.
const keySource = `void loadConfig(char **argv) {
    strcasecmp(argv[1],"early");
    strcasecmp(argv[0],"appendfsync");
    strcasecmp(argv[1],"always");
    strcasecmp(argv[1],"no");
    strcasecmp(argv[0],"logfile");
    strcasecmp(argv[1],"stdout");
    strcasecmp(other[1],"elsewhere");
}
`

// A word compared with what follows a key is that key's value (K3): when
// the key's answer made it an entry, the value is its entry's, of its kind,
// and never asked, since the step before decided it (appendfsync's
// "always" and "no"). A value whose key was answered none is asked on its
// own with its key as written, and so is a word compared with a later
// element and no key before it, or with another value.
func TestAKeysValueIsNotAskedWhenItsKeyIsAnEntry(t *testing.T) {
	line := &sourcevalue.Anchor{Path: "main.c", Line: 1, Column: 1}
	other := &sourcevalue.Anchor{Path: "main.c", Line: 1, Column: 20}
	element := func(of *sourcevalue.Anchor, index, line int, word string) atlas.SymbolCall {
		call := wordCall("string.h.strcasecmp", line, 5, word)
		call.SourceArguments = []atlas.SourceArgument{
			{Position: 1, Origin: &sourcevalue.Value{Kind: "index", Parts: []sourcevalue.Value{{Kind: "call_result", Anchor: of}, {Kind: "literal", Text: string(rune('0' + index))}}}},
			{Position: 2, Origin: &sourcevalue.Value{Kind: "literal", Text: word}},
		}
		return call
	}
	places := apiGraph(
		element(line, 1, 2, "early"), element(line, 0, 3, "appendfsync"), element(line, 1, 4, "always"), element(line, 1, 5, "no"),
		element(line, 0, 6, "logfile"), element(line, 1, 7, "stdout"), element(other, 1, 8, "elsewhere"),
	)
	answers := map[string]string{"appendfsync": atlas.BoundarySetting}
	var mu sync.Mutex
	asked := map[string]any{}
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		if !strings.HasSuffix(key, "|enters") {
			return typesafetest.Choose(lines.APINone), true
		}
		words, _ := question.Item["literals"].([]any)
		word, _ := words[0].(string)
		mu.Lock()
		asked[word] = question.Item["compared_after"]
		mu.Unlock()
		if answer := answers[word]; answer != "" {
			return typesafetest.Choose(answer), true
		}
		return typesafetest.Choose(lines.APINone), true
	}}
	r := apiReader(t, t.TempDir(), places, categorizer)
	r.opts.ReadSource = func(string) ([]byte, error) { return []byte(keySource), nil }
	if err := r.readAPI(t.Context()); err != nil {
		t.Fatal(err)
	}
	var words []string
	for word := range asked {
		words = append(words, word)
	}
	slices.Sort(words)
	if want := []string{"appendfsync", "early", "elsewhere", "logfile", "stdout"}; !slices.Equal(words, want) {
		t.Fatalf("calls asked = %v, want %v", words, want)
	}
	if asked["stdout"] != `strcasecmp(argv[0],"logfile")` || asked["early"] != nil || asked["elsewhere"] != nil {
		t.Fatalf("keys shown: %v", asked)
	}
	for line, want := range map[int]string{4: atlas.BoundarySetting, 5: atlas.BoundarySetting, 7: lines.APINone} {
		if got := r.callEnters[sourceSite{"main.c", line, 5}]; got != want {
			t.Fatalf("the call on line %d became %q, want %q", line, got, want)
		}
	}
}
