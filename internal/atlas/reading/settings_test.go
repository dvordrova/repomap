package reading

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/sourcevalue"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// settingsSource is a configuration structure a program decodes its file
// into, a structure nested in it, and a reply it sends.
const settingsSource = `package config

type Config struct {
	Addr string      ` + "`yaml:\"addr\"`" + `
	DBs  []*DBConfig ` + "`yaml:\"dbs\"`" + `
}

type DBConfig struct {
	Path string ` + "`yaml:\"path\" json:\"path\"`" + `
}

type Reply struct {
	Count int ` + "`json:\"count\"`" + `
}

func DefaultConfig() Config { return Config{} }

func ParseConfig(buf []byte) (Config, error) {
	config := DefaultConfig()
	err := yaml.Unmarshal(buf, &config)
	return config, err
}

func reply(w io.Writer, n int) {
	json.NewEncoder(w).Encode(Reply{Count: n})
}

func load(buf []byte) Config {
	config, err := ParseConfig(buf)
	log.Print(err)
	return config
}
`

// settingsAt is the line and column of the first mark in settingsSource.
func settingsAt(t *testing.T, mark string) (int, int) {
	t.Helper()
	offset := strings.Index(settingsSource, mark)
	if offset < 0 {
		t.Fatalf("no %q in the source", mark)
	}
	return strings.Count(settingsSource[:offset], "\n") + 1, offset - strings.LastIndex(settingsSource[:offset], "\n")
}

// A field whose tag names a key is asked on its own, with its structure, its
// tag as written and what the facts show the structure is used for: the
// outside call given a value of it, by where the types the argument's
// value is of are declared (an error beside it names none), or the field
// typed with it with that structure's own use. The
// key of a field answered setting is an entry whose handler is not
// established, declared by its structure, and on the one call decoding it;
// a field answered none makes nothing.
func TestATaggedFieldIsAskedWithItsStructureAndAnsweredSettingIsAnEntry(t *testing.T) {
	// declared is where a structure is declared, as the adapter anchors a
	// type a value or a field names.
	declared := func(name string) []sourcevalue.Anchor {
		line, column := settingsAt(t, "type "+name+" ")
		return []sourcevalue.Anchor{{Path: "config.go", Line: line, Column: column}}
	}
	member := func(mark, name, signature, aliases string, types ...sourcevalue.Anchor) atlas.TypeMember {
		line, column := settingsAt(t, mark)
		return atlas.TypeMember{Path: "config.go", Decl: atlas.Decl{Name: name, Kind: "field", Signature: signature, Aliases: aliases, Types: types, LineNo: line, Column: column}}
	}
	structure := func(id, name, object string, members ...atlas.TypeMember) atlas.Place {
		line, column := settingsAt(t, "type "+name+" ")
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: "config.go", LineNo: line, Column: column, Parent: "f1", TargetIDs: []string{"t1"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name, Kind: string(programindex.ObjectType), Signature: "struct", ObjectID: object}, Members: members}}
	}
	function := func(id, name, signature string, calls ...atlas.SymbolCall) atlas.Place {
		line, column := settingsAt(t, "func "+name+"(")
		return atlas.Place{ID: id, Kind: atlas.PlaceSymbol, Path: "config.go", LineNo: line, Column: column, Parent: "f1", TargetIDs: []string{"t1"},
			Symbol: &atlas.SymbolFacts{Decl: atlas.Decl{Name: name, Kind: "function", Signature: signature, ObjectID: "o" + id}, Calls: calls}}
	}
	defaultLine, defaultColumn := settingsAt(t, "DefaultConfig()\n")
	unmarshalLine, unmarshalColumn := settingsAt(t, "yaml.Unmarshal")
	encodeLine, encodeColumn := settingsAt(t, "json.NewEncoder")
	recordLine, recordColumn := settingsAt(t, "Reply{Count")
	parseLine, parseColumn := settingsAt(t, "ParseConfig(buf)")
	printLine, printColumn := settingsAt(t, "log.Print")
	places := []atlas.Place{
		{ID: "f1", Kind: atlas.PlaceFile, Path: "config.go", TargetIDs: []string{"t1"}, File: &atlas.FileFacts{}},
		structure("s:config", "Config", "n:config", member("Addr string", "Addr", "Addr string", "yaml:addr"), member("DBs  []", "DBs", "DBs []*DBConfig", "yaml:dbs", declared("DBConfig")...)),
		structure("s:db", "DBConfig", "n:db", member("Path string", "Path", "Path string", "json:path yaml:path")),
		structure("s:reply", "Reply", "n:reply", member("Count int", "Count", "Count int", "json:count")),
		function("s:default", "DefaultConfig", "func() Config"),
		function("s:parse", "ParseConfig", "func(buf []byte) (Config, error)",
			atlas.SymbolCall{Kind: string(programindex.RelationCalls), Name: "DefaultConfig", Line: defaultLine, Column: defaultColumn, CalleeIDs: []string{"s:default"}},
			atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: "yaml.Unmarshal", Line: unmarshalLine, Column: unmarshalColumn,
				API: &atlas.CallAPI{Package: "gopkg.in/yaml.v2", Name: "Unmarshal"},
				SourceArguments: []atlas.SourceArgument{
					{Position: 1, Origin: &sourcevalue.Value{Kind: "parameter", Text: "buf"}},
					{Position: 2, Origin: &sourcevalue.Value{Kind: "call_result", Text: "DefaultConfig", Types: declared("Config"), Anchor: &sourcevalue.Anchor{Path: "config.go", Line: defaultLine, Column: defaultColumn}}},
				}}),
		function("s:reply-fn", "reply", "func(w io.Writer, n int)",
			atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: "json.Encoder.Encode", Line: encodeLine, Column: encodeColumn,
				API:             &atlas.CallAPI{Package: "encoding/json", Receiver: "*Encoder", Name: "Encode"},
				SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "record", Types: declared("Reply"), Anchor: &sourcevalue.Anchor{Path: "config.go", Line: recordLine, Column: recordColumn}}}}}),
		// The error ParseConfig returns beside the Config is no Config.
		function("s:load", "load", "func(buf []byte) Config",
			atlas.SymbolCall{Kind: string(programindex.RelationCalls), Name: "ParseConfig", Line: parseLine, Column: parseColumn, CalleeIDs: []string{"s:parse"}},
			atlas.SymbolCall{Kind: string(programindex.RelationInvokesExternal), Name: "log.Print", Line: printLine, Column: printColumn,
				API:             &atlas.CallAPI{Package: "log", Name: "Print"},
				SourceArguments: []atlas.SourceArgument{{Position: 1, Origin: &sourcevalue.Value{Kind: "call_result", Text: "ParseConfig", Anchor: &sourcevalue.Anchor{Path: "config.go", Line: parseLine, Column: parseColumn}}}}}),
	}
	var mu sync.Mutex
	asked := map[string]map[string]any{}
	categorizer := &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		if !strings.HasSuffix(key, "|becomes") {
			return llm.Verdict{}, false
		}
		structure, _ := question.Item["structure"].(string)
		field, _ := question.Item["field"].(string)
		mu.Lock()
		name, _, _ := strings.Cut(structure, ",")
		asked[name+"."+strings.Fields(field)[0]] = question.Item
		mu.Unlock()
		// The reader's decision: a key the configuration file writes is a
		// setting, a key of the reply is none.
		if tag, _ := question.Item["tag"].(string); strings.Contains(tag, "yaml:") {
			return typesafetest.Choose(atlas.BoundarySetting), true
		}
		return typesafetest.Choose(lines.APINone), true
	}}
	r := apiReader(t, t.TempDir(), places, categorizer)
	r.opts.ReadSource = func(string) ([]byte, error) { return []byte(settingsSource), nil }
	r.boundaries = map[string]*boundaryState{}
	if err := r.readInputs(t.Context()); err != nil {
		t.Fatal(err)
	}
	uses := func(key string) []string {
		used, _ := asked[key]["structure_use"].([]any)
		var result []string
		for _, line := range used {
			result = append(result, line.(string))
		}
		return result
	}
	if item := asked["Config.DBs"]; item["field"] != "DBs []*DBConfig" || item["structure"] != "Config, declared in config.go" || item["tag"] != `yaml:"dbs"` {
		t.Fatalf("Config.DBs was asked with %v (all %v)", item, asked)
	}
	if got, want := uses("Config.DBs"), []string{"given to gopkg.in/yaml.v2.Unmarshal in ParseConfig: yaml.Unmarshal(buf, &config)"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Config's use = %q, want %q", got, want)
	}
	if got, want := uses("DBConfig.Path"), []string{`the type of field DBs (yaml:"dbs") of Config, which is given to gopkg.in/yaml.v2.Unmarshal in ParseConfig: yaml.Unmarshal(buf, &config)`}; !reflect.DeepEqual(got, want) || asked["DBConfig.Path"]["tag"] != `json:"path" yaml:"path"` {
		t.Fatalf("DBConfig's use = %q (tag %v), want %q", got, asked["DBConfig.Path"]["tag"], want)
	}
	if got := uses("Reply.Count"); len(got) != 1 || !strings.HasPrefix(got[0], "given to encoding/json.Encoder.Encode in reply: ") {
		t.Fatalf("Reply's use = %q", got)
	}
	if len(asked) != 4 {
		t.Fatalf("asked %d fields: %v", len(asked), asked)
	}
	r.bindTableRows()
	settings := map[string]*boundaryState{}
	for _, state := range r.boundaries {
		b := state.place.Boundary
		if state.kind != atlas.BoundarySetting || !state.handlerUnknown || b.GivenKind != atlas.BoundarySetting || b.Direction != atlas.DirectionIn {
			t.Fatalf("a tagged field made %+v", state)
		}
		settings[b.Caller+" "+strings.Join(b.Words, " ")] = state
	}
	if keys := slices.Sorted(func(yield func(string) bool) {
		for key := range settings {
			if !yield(key) {
				return
			}
		}
	}); !reflect.DeepEqual(keys, []string{"Config addr", "Config dbs", "DBConfig path"}) {
		t.Fatalf("settings = %q", keys)
	}
	if on := settings["Config dbs"].on; on == nil || on.LineNo != unmarshalLine || on.Text != "yaml.Unmarshal(buf, &config)" {
		t.Fatalf("Config's settings are declared on %+v", on)
	}
	if on := settings["DBConfig path"].on; on != nil {
		t.Fatalf("DBConfig, decoded by no call of its own, is declared on %+v", on)
	}
	if b := settings["Config addr"].place.Boundary; b.ObjectID != "n:config" || settings["Config addr"].place.LineNo != 4 {
		t.Fatalf("addr is declared by %s at line %d", b.ObjectID, settings["Config addr"].place.LineNo)
	}
}
