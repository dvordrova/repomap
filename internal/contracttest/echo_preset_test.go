package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/extractors"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/groupindex/flowtest"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
	"github.com/dvordrova/repomap/internal/typesafe/typesafetest"
)

// The Echo service is read end to end with a preset in place of the model:
// the answers a reader would give to the tables the code asks. Nothing in the
// pipeline names Echo, sqlc or PostgreSQL; the route, the database read and the
// configuration key reach the overlay from the shapes of the calls alone.
func TestEchoPresetReadingTurnsRegistrationsIntoOperations(t *testing.T) {
	provider := &echoPreset{}
	index, layer, result, indexes := readEchoPreset(t, provider)
	registrations := map[string]facts.Fact{}
	for _, fact := range layer.OfKind(facts.KindRegistration) {
		registrations[fact.Key+" "+fact.Path] = fact
	}
	route, ok := registrations["GET /users/:id"]
	if !ok || route.Symbol != "GetUser" || route.Method != "GET" || route.Resolution != facts.ResolutionExact || route.Text != "github.com/labstack/echo/v4.Echo.GET" {
		t.Fatalf("route registration = %+v (all: %v)", route, registrations)
	}
	if queries := layer.OfKind(facts.KindSQLQuery); len(queries) != 1 || queries[0].Key != "users" || queries[0].Symbol != "GetUser" {
		t.Fatalf("sql queries = %+v", queries)
	}
	overlay := indexes[0]
	handler := methodOf(t, index, "Handler", "GetUser")
	var request *groupindex.Operation
	for i, operation := range overlay.Operations {
		if operation.FactID == route.ID {
			request = &overlay.Operations[i]
		}
	}
	if request == nil || request.Kind != "request" || request.SubjectID != handler.ID || request.Name != "GET /users/:id" || request.Address != ":8080" || request.Source != "fact" {
		t.Fatalf("route did not become the handler's request operation on the started address: %+v (operations %+v)", request, overlay.Operations)
	}
	core := 0
	for _, group := range overlay.Groups {
		if group.Core {
			core++
		}
	}
	if !provider.sawDomainPart || core != 1 {
		t.Fatalf("the model's core part did not reach the groups: %d of %d", core, len(overlay.Groups))
	}
	// Only the part holding cmd/api's main is the entry; the handler's
	// part, which the route calls into, is not.
	entries := 0
	for _, group := range overlay.Groups {
		holdsMain := slices.ContainsFunc(group.MemberSubjectIDs, func(id string) bool { return id == index.Target.Seeds[0].ObjectID })
		if holdsMain != (group.Lane == groupindex.LaneTriggers) || slices.Contains(group.MemberSubjectIDs, handler.ID) && group.Lane == groupindex.LaneTriggers {
			t.Fatalf("part %q is %s (holds main: %v)", group.Title, group.Lane, holdsMain)
		}
		if holdsMain {
			entries++
		}
	}
	if entries != 1 || len(index.Target.Seeds) != 1 {
		t.Fatalf("%d parts hold cmd/api's %d seeds", entries, len(index.Target.Seeds))
	}
	kinds := map[string]int{}
	for _, call := range overlay.Outbound {
		kinds[call.Kind]++
		if call.Kind == "db" && call.Location.Path == "internal/database/sqlc/users.sql.go" && (len(call.Values) == 0 || call.Values[0] != "users") {
			t.Fatalf("database read lost its table: %+v", call)
		}
	}
	// The driver opened and the statement sent: two database boundaries.
	if kinds["db"] != 2 || kinds["client_request"] != 0 {
		t.Fatalf("outbound kinds = %v (%+v)", kinds, overlay.Outbound)
	}
	if !provider.sawRegistration || !provider.sawSQL {
		t.Fatalf("the preset was never asked about the route symbol (%v) or the query (%v)", provider.sawRegistration, provider.sawSQL)
	}
	if len(result.Atlas.API) != 3 {
		t.Fatalf("api roles = %+v", result.Atlas.API)
	}
	// Each field whose tag names a key is asked on its own, the reply's
	// with the call it is given to; keys of data the service stores and
	// replies with are no settings.
	if len(provider.askedFields) != 4 || provider.askedFields["UserResponse.ID"] != "given to github.com/labstack/echo/v4.Context.JSON in Handler.GetUser: ctx.JSON(http.StatusOK, handler.converter.ToResponse(user))" {
		t.Fatalf("tagged fields asked: %q", provider.askedFields)
	}
	for _, operation := range overlay.Operations {
		if operation.Kind == "setting" {
			t.Fatalf("a key of data became a setting: %+v", operation)
		}
	}
	// The extracted schema reaches the overlay, the statement names its table
	// and the route walks handler → service → repository → query to it.
	var users string
	for _, record := range overlay.Data {
		if record.Data != nil && record.Data.Kind == "table" && record.Data.Name == "users" {
			users = record.ID
		}
	}
	if users == "" {
		t.Fatalf("users table missing from data: %+v", overlay.Data)
	}
	var query groupindex.OutboundCall
	for _, call := range overlay.Outbound {
		if call.Kind == "db" && call.Location.Path == "internal/database/sqlc/users.sql.go" && len(call.Values) > 0 && call.Values[0] == "users" {
			query = call
		}
	}
	if len(query.DataIDs) != 1 || query.DataIDs[0] != users {
		t.Fatalf("statement not joined to its table: %+v", query)
	}
	names := map[string]string{}
	for _, subject := range overlay.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	// The route's reach walks its handler, the service, the repository
	// method and the sqlc query that sends the statement naming the users
	// table: the whole way from the route to the database call.
	flowtest.Check(t, index, overlay)
	for position, operation := range overlay.Operations {
		if operation.ID != request.ID {
			continue
		}
		located := map[string]string{}
		for _, subject := range overlay.Subjects {
			if subject.Object != nil && subject.Object.Location != nil {
				located[subject.ID] = subject.Object.Name + "@" + subject.Object.Location.Path
			}
		}
		reached := map[string]int{}
		for _, subject := range overlay.Reach[position].Subjects {
			reached[located[subject.SubjectID]] = subject.Depth + 1
		}
		for _, want := range []string{"GetUser@internal/users/handler/handler.go", "GetUser@internal/users/service/service.go", "GetByID@internal/users/repository/postgres.go", "GetUser@internal/database/sqlc/users.sql.go"} {
			if reached[want] == 0 {
				t.Fatalf("the route does not reach %s: %v", want, reached)
			}
		}
		if reached[located[query.SubjectID]] == 0 {
			t.Fatalf("the route does not reach the call sending its statement (%s): %v", located[query.SubjectID], reached)
		}
	}
	// Initialization is what main reaches by calls; runtime is the route's reach.
	phases := map[string]string{}
	for _, subject := range overlay.Subjects {
		if subject.Phase != "" && subject.Object != nil && subject.Object.Location != nil {
			phases[names[subject.ID]+"@"+subject.Object.Location.Path] = subject.Phase
		}
	}
	for name, want := range map[string]string{
		"main@cmd/api/main.go": "init", "NewUsers@internal/app/users.go": "init", "New@internal/users/handler/handler.go": "init",
		"GetUser@internal/users/handler/handler.go": "runtime", "GetUser@internal/database/sqlc/users.sql.go": "runtime",
	} {
		if phases[name] != want {
			t.Fatalf("phase of %s = %q, want %q (all: %v)", name, phases[name], want, phases)
		}
	}
	// The reach is derived: the saved overlay carries none, and the decoded
	// index derives the same one again.
	encoded, err := groupindex.Encode(overlay)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := groupindex.Decode(encoded, index)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.Reach, overlay.Reach) || !reflect.DeepEqual(restored.Dispatch, overlay.Dispatch) ||
		!reflect.DeepEqual(restored.Operations, overlay.Operations) || !reflect.DeepEqual(restored.Subjects, overlay.Subjects) {
		t.Fatal("the decoded index derives another reach, or other operations or subjects")
	}
}

// readEchoPreset reads the Echo service's cmd/api end to end with a preset
// in place of the model and projects its GroupsIndex.
func readEchoPreset(t *testing.T, provider *echoPreset) (programindex.Index, facts.Result, reading.Result, []groupindex.Index) {
	t.Helper()
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	repositoryPath, repository := materializeRepository(t, filepath.Join("testdata", "echo-sqlc-service"))
	authorities := analyzeGoFixture(t, repositoryPath, repository, "example.com/echo-sqlc-service/cmd/api", "echo")
	input, err := goadapter.BuildInput(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
	if err != nil {
		t.Fatal(err)
	}
	index, err := programindex.New(input)
	if err != nil {
		t.Fatal(err)
	}
	extracted, err := extractors.Run(context.Background(), repositoryPath, repository)
	if err != nil {
		t.Fatal(err)
	}
	layer, err := facts.Build(facts.Input{Repository: repository, Targets: []facts.TargetInput{{Index: index, Root: "."}}, Extractions: extracted.Extractions})
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	result, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Repository: "echo", Revision: "test",
		Targets:  []reading.TargetMeta{{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}},
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: provider, Categorizer: provider.categorizer(), OwnerRunDir: t.TempDir(),
		ReadSource: func(path string) ([]byte, error) {
			id, ok := repository.ID(path)
			if !ok {
				return nil, fmt.Errorf("%s is not in the corpus", path)
			}
			content, err := repository.ReadFileAll(id)
			if err != nil {
				return nil, err
			}
			return content.Bytes, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertNoSourceBodies(t, repository, provider.requests)
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 {
		t.Fatalf("indexes = %d", len(indexes))
	}
	return index, layer, result, indexes
}

// The query call sqlc generated gives QueryRowContext its statement, and
// the SQL fact already names that call: its words are not asked what they
// become, so even a model that would take them for a command makes no
// entry there (nor could the statement, whose line breaks name nothing).
// The call stays the one database boundary it is, and no input stands in
// users.sql.go.
func TestNoEntryAtACallAFactAlreadyNames(t *testing.T) {
	provider := &echoPreset{enters: map[string]string{"database/sql.DB.QueryRowContext": "command"}}
	_, _, _, indexes := readEchoPreset(t, provider)
	if provider.askedEnters["database/sql.DB.QueryRowContext"] {
		t.Fatalf("the words of a call the SQL fact names were asked about: %v", provider.askedEnters)
	}
	for _, operation := range indexes[0].Operations {
		if operation.Location.Path == "internal/database/sqlc/users.sql.go" {
			t.Fatalf("an input stands at the query call: %+v", operation)
		}
	}
	at := 0
	for _, call := range indexes[0].Outbound {
		if call.Location.Path == "internal/database/sqlc/users.sql.go" && call.Location.Line == 13 {
			if call.Kind != "db" {
				t.Fatalf("the query call is %s", call.Kind)
			}
			at++
		}
	}
	if at != 1 {
		t.Fatalf("%d boundaries at the query call, want the one database boundary", at)
	}
}

func methodOf(t *testing.T, index programindex.Index, owner, name string) programindex.Object {
	t.Helper()
	owners := make(map[string]string, len(index.Objects))
	for _, object := range index.Objects {
		owners[object.ID] = object.Name
	}
	for _, object := range index.Objects {
		if object.Kind == programindex.ObjectMethod && object.Name == name && owners[object.OwnerID] == owner {
			return object
		}
	}
	t.Fatalf("method %s.%s is absent", owner, name)
	return programindex.Object{}
}

func materializeRepository(t *testing.T, relative string) (string, *corpus.Corpus) {
	t.Helper()
	isolateFixtureGitEnvironment(t)
	destination := filepath.Join(t.TempDir(), "repository")
	copyFixtureTree(t, filepath.Join(repositoryRoot(t), relative), destination)
	runFixtureGit(t, destination, "init", "--quiet")
	runFixtureGit(t, destination, "add", "--all", "--")
	repository, err := corpus.Open(t.Context(), destination)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repository.Close() })
	return destination, repository
}

// echoPreset answers every text-model table the reading asks the way a
// careful reader would, from the row alone. Text cells get a placeholder; choices get the
// reader's decision, or the first option where any answer is fine.
type echoPreset struct {
	sawRegistration, sawSQL, sawDomainPart bool
	mu                                     sync.Mutex
	requests                               [][]byte
	// enters answers what the words given to these symbols become; every
	// other symbol's are none. askedEnters are the symbols asked.
	enters      map[string]string
	askedEnters map[string]bool
	// askedFields are the tagged fields asked what their key becomes, by
	// structure and field, with the structure's use.
	askedFields map[string]string
}

func (*echoPreset) State() []byte { return []byte(`{"provider":"echo-preset"}`) }

func (*echoPreset) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}

func (preset *echoPreset) Complete(_ context.Context, prepared llm.Prepared) (llm.Completion, error) {
	var request struct {
		Task  string           `json:"task"`
		Table string           `json:"table"`
		Fill  []map[string]any `json:"fill"`
		Rows  []map[string]any `json:"rows"`
		Units []struct {
			Ref  string `json:"ref"`
			Path string `json:"path"`
			Box  string `json:"box"`
		} `json:"units"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	preset.mu.Lock()
	preset.requests = append(preset.requests, append([]byte(nil), prepared.Bytes()...))
	preset.mu.Unlock()
	var response []byte
	switch {
	case request.Task == "repomap.atlas.parts.v2":
		// The parts a reader would draw: what serves requests, what holds
		// the data, and the program's setup around them.
		parts := map[string][]string{}
		for _, file := range request.Units {
			part := "Program setup"
			switch {
			case strings.Contains(file.Path, "/handler/"):
				part = "Request handling"
			case strings.Contains(file.Path, "/database/"):
				part = "Stored users"
			}
			parts[part] = append(parts[part], file.Ref)
		}
		var groups []map[string]any
		for _, name := range []string{"Request handling", "Stored users", "Program setup"} {
			if len(parts[name]) > 0 {
				groups = append(groups, map[string]any{"name": name, "units": parts[name]})
			}
		}
		var err error
		if response, err = json.Marshal(map[string]any{"groups": groups}); err != nil {
			return llm.Completion{}, err
		}
	case request.Task == "repomap.atlas.describe.v1":
		response = []byte(`{"description":"Preset description."}`)
	case request.Task == "repomap.atlas.areas.v1":
		response = []byte(`{"areas":[]}`)
	default:
		rows := make([]map[string]any, 0, len(request.Rows))
		for _, row := range request.Rows {
			rows = append(rows, preset.answer(request.Table, request.Fill, row))
		}
		var err error
		if response, err = json.Marshal(map[string]any{"rows": rows}); err != nil {
			return llm.Completion{}, err
		}
	}
	return llm.Completion{Response: response, FinishReason: llm.FinishStop, ChoiceCount: 1, Metrics: llm.Metrics{Attempts: 1}}, nil
}

func (preset *echoPreset) answer(table string, fill []map[string]any, row map[string]any) map[string]any {
	answer := map[string]any{"key": row["key"]}
	values := stringsOf(row["values"])
	if table == "atlas_boundaries" && row["kind_given"] == "db" && hasPrefix(values, "-- name: GetUser") {
		// The SQL statement is a fact; only its wording is asked.
		preset.sawSQL = true
	}
	for _, column := range fill {
		name, _ := column["name"].(string)
		if !conditionHolds(column, answer, row) {
			continue
		}
		switch {
		case table == "atlas_boundaries" && name == "name":
			// The route is named by the verb and the path its registration
			// wrote, in the order a client reads them.
			answer["name"] = wordRefs(row, "GET", "/users/:id")
		case table == "atlas_boundaries" && name == "destination":
			answer["destination"] = "other: PostgreSQL"
		case table == "atlas_boundaries" && name == "address":
			answer["address"] = "unknown"
		case column["kind"] == "text":
			answer[name] = fmt.Sprintf("preset %s", name)
		case column["kind"] == "choice":
			options := stringsOf(column["options"])
			if len(options) > 0 {
				answer[name] = options[0]
			}
		}
	}
	return answer
}

// categorizer decides the closed tables as a reader would: the part that
// holds the stored users is this program's domain and the others serve it;
// every candidate explains its part and is a key. Of the outside symbols,
// the route registration binds a request, the server's start serves and
// the driver's open talks to the database; every other symbol is none.
func (preset *echoPreset) categorizer() *typesafetest.Categorizer {
	decide := typesafetest.ByColumn(map[string]llm.Verdict{"explains": typesafetest.Yes(0.9), "key_symbol": typesafetest.Choose("yes")})
	var mu sync.Mutex
	return &typesafetest.Categorizer{Decide: func(key string, question llm.Question) (llm.Verdict, bool) {
		symbol, _ := question.Item["symbol"].(string)
		switch column := key[strings.LastIndex(key, "|")+1:]; {
		case column == "binds" && strings.HasSuffix(symbol, "echo/v4.Echo.GET"):
			mu.Lock()
			preset.sawRegistration = true
			mu.Unlock()
			return typesafetest.Choose("request"), true
		case column == "talks" && strings.HasSuffix(symbol, "echo/v4.Echo.Start"):
			return typesafetest.Choose("serves"), true
		case column == "talks" && symbol == "database/sql.Open":
			return typesafetest.Choose("db"), true
		case column == "enters":
			mu.Lock()
			if preset.askedEnters == nil {
				preset.askedEnters = map[string]bool{}
			}
			preset.askedEnters[symbol] = true
			mu.Unlock()
			if answer := preset.enters[symbol]; answer != "" {
				return typesafetest.Choose(answer), true
			}
			return typesafetest.Choose("none"), true
		case column == "binds" || column == "publishes" || column == "talks":
			return typesafetest.Choose("none"), true
		case column == "becomes" && question.Item["tag"] != nil:
			structure, _ := question.Item["structure"].(string)
			field, _ := question.Item["field"].(string)
			name, _, _ := strings.Cut(structure, ",")
			var use []string
			lines, _ := question.Item["structure_use"].([]any)
			for _, line := range lines {
				use = append(use, line.(string))
			}
			mu.Lock()
			if preset.askedFields == nil {
				preset.askedFields = map[string]string{}
			}
			preset.askedFields[name+"."+strings.Fields(field)[0]] = strings.Join(use, "; ")
			mu.Unlock()
			return typesafetest.Choose("none"), true
		}
		if !strings.HasSuffix(key, "|role") {
			return decide(key, question)
		}
		if question.Item["reaches"] == nil {
			return typesafetest.Choose("interface"), true
		}
		mu.Lock()
		preset.sawDomainPart = true
		mu.Unlock()
		return typesafetest.Choose("domain"), true
	}}
}

// conditionHolds reads a column's when clauses: other cells already
// answered, and row fields that must not be empty.
func conditionHolds(column map[string]any, answer, row map[string]any) bool {
	if when, ok := column["when"].(map[string]any); ok {
		for cell, wanted := range when {
			if answer[cell] != wanted {
				return false
			}
		}
	}
	if field, ok := column["when_options_nonempty"].(string); ok {
		if len(stringsOf(row[field])) == 0 {
			return false
		}
	}
	return true
}

// wordRefs are the refs of an entry row's words with these values, in this
// order; a value the row did not write has no ref.
func wordRefs(row map[string]any, values ...string) []string {
	words, _ := row["words"].([]any)
	var refs []string
	for _, value := range values {
		for _, item := range words {
			word, _ := item.(map[string]any)
			if word["value"] == value {
				refs = append(refs, word["ref"].(string))
			}
		}
	}
	return refs
}

func stringsOf(value any) []string {
	list, _ := value.([]any)
	result := make([]string, 0, len(list))
	for _, item := range list {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func hasPrefix(values []string, prefix string) bool {
	for _, value := range values {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

// assertNoSourceBodies fails when a request carries a function's source as
// written, its declaration line and the line after it, numbered or not:
// source bodies do not enter provider requests (PROGRAM_INDEX). A call
// site's one line, a literal and a signature are not bodies.
func assertNoSourceBodies(t *testing.T, repository *corpus.Corpus, requests [][]byte) {
	t.Helper()
	numbered := regexp.MustCompile(`^\s*\d+\s+`)
	bodies := map[[2]string]string{}
	for _, path := range repository.VisiblePaths() {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		id, _ := repository.ID(path)
		content, err := repository.ReadFileAll(id)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(content.Bytes), "\n")
		for i := 0; i+1 < len(lines); i++ {
			if first, next := strings.TrimSpace(lines[i]), strings.TrimSpace(lines[i+1]); strings.HasPrefix(first, "func ") && next != "" {
				bodies[[2]string{first, next}] = fmt.Sprintf("%s:%d", path, i+1)
			}
		}
	}
	var visit func(value any)
	visit = func(value any) {
		switch value := value.(type) {
		case string:
			lines := strings.Split(value, "\n")
			for i := 0; i+1 < len(lines); i++ {
				pair := [2]string{strings.TrimSpace(numbered.ReplaceAllString(lines[i], "")), strings.TrimSpace(numbered.ReplaceAllString(lines[i+1], ""))}
				if at, ok := bodies[pair]; ok {
					t.Fatalf("a request carries the source body of %s: %q", at, pair[0])
				}
			}
		case []any:
			for _, item := range value {
				visit(item)
			}
		case map[string]any:
			for _, item := range value {
				visit(item)
			}
		}
	}
	if len(requests) == 0 {
		t.Fatal("the reading sent no request")
	}
	for _, request := range requests {
		var decoded any
		if err := json.Unmarshal(request, &decoded); err != nil {
			t.Fatal(err)
		}
		visit(decoded)
	}
}
