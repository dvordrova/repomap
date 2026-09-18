package contracttest

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/corpus"
	"github.com/dvordrova/repomap/internal/extractors"
	"github.com/dvordrova/repomap/internal/facts"
	"github.com/dvordrova/repomap/internal/groupindex"
	"github.com/dvordrova/repomap/internal/llm"
	"github.com/dvordrova/repomap/internal/programindex"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// The Echo service is read end to end with a preset in place of the model:
// the answers a reader would give to the tables the code asks. Nothing in the
// pipeline names Echo, sqlc or PostgreSQL; the route, the database read and the
// configuration key reach the overlay from the shapes of the calls alone.
func TestEchoPresetReadingTurnsRegistrationsIntoOperations(t *testing.T) {
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
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index, Root: "."}}, Facts: layer})
	if err != nil {
		t.Fatal(err)
	}
	provider := &echoPreset{}
	result, err := reading.Read(context.Background(), reading.Options{
		Graph: graph, Repository: "echo", Revision: "test",
		Targets:  []reading.TargetMeta{{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}},
		Executor: llm.Executor{BatchConcurrency: 1, BatchController: &llm.BatchController{}},
		Provider: provider, OwnerRunDir: t.TempDir(),
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
	indexes, err := groupindex.ProjectAtlas(map[string]programindex.Index{index.Target.ID: index}, result.Atlas)
	if err != nil {
		t.Fatal(err)
	}
	if len(indexes) != 1 {
		t.Fatalf("indexes = %d", len(indexes))
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
	if !provider.sawEntryPart || core != 1 {
		t.Fatalf("the model's core part did not reach the groups: %d of %d", core, len(overlay.Groups))
	}
	kinds := map[string]int{}
	for _, call := range overlay.Outbound {
		kinds[call.Kind]++
		if call.Kind == "db" && call.Location.Path == "internal/database/sqlc/users.sql.go" && (len(call.Values) == 0 || call.Values[0] != "users") {
			t.Fatalf("database read lost its table: %+v", call)
		}
	}
	// The driver opened and the statement sent: two database boundaries.
	if kinds["db"] != 2 || kinds["http_client"] != 0 {
		t.Fatalf("outbound kinds = %v (%+v)", kinds, overlay.Outbound)
	}
	if !provider.sawRegistration || !provider.sawSQL {
		t.Fatalf("the preset was never asked about the route symbol (%v) or the query (%v)", provider.sawRegistration, provider.sawSQL)
	}
	if len(result.Atlas.API) != 3 {
		t.Fatalf("api roles = %+v", result.Atlas.API)
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
	var walked []string
	for _, chain := range overlay.Chains {
		if chain.OperationID == request.ID && chain.OutboundID == query.ID {
			for _, id := range chain.SubjectIDs {
				walked = append(walked, names[id])
			}
		}
	}
	if strings.Join(walked, " → ") != "GetUser → GetUser → GetByID → GetUser" {
		t.Fatalf("route chain to the users table = %v (chains %+v)", walked, overlay.Chains)
	}
	typeNames := func(ids []string) []string {
		var result []string
		for _, id := range ids {
			result = append(result, names[id])
		}
		return result
	}
	if !reflect.DeepEqual(typeNames(request.RequestTypeIDs), []string(nil)) || !reflect.DeepEqual(typeNames(request.ResponseTypeIDs), []string{"UserResponse"}) {
		t.Fatalf("request/response types = %v / %v", typeNames(request.RequestTypeIDs), typeNames(request.ResponseTypeIDs))
	}
	for _, chain := range overlay.Chains {
		if chain.OperationID == request.ID && chain.OutboundID == query.ID && !reflect.DeepEqual(typeNames(chain.TypeIDs), []string{"User", "User"}) {
			t.Fatalf("types carried by the route chain = %v", typeNames(chain.TypeIDs))
		}
	}
	// The layers table read each declaration on the chain from its source and
	// the preset answered from what it saw.
	roles := map[string]string{}
	for _, subject := range overlay.Subjects {
		if subject.Interpretation != nil && subject.Interpretation.Role != "" {
			roles[names[subject.ID]+"@"+subject.Object.Location.Path] = subject.Interpretation.Role
		}
	}
	wantRoles := map[string]string{
		"GetUser@internal/users/handler/handler.go": "logic", "GetUser@internal/users/service/service.go": "passthrough",
		"GetByID@internal/users/repository/postgres.go": "adapter", "GetUser@internal/database/sqlc/users.sql.go": "access",
	}
	if !reflect.DeepEqual(roles, wantRoles) || provider.sawQuerySource || !provider.sawRepositorySource {
		t.Fatalf("roles = %v (query asked: %v, repository read: %v)", roles, provider.sawQuerySource, provider.sawRepositorySource)
	}
	// Initialization is what main reaches by calls; runtime is the route's chain.
	phases := map[string]string{}
	for _, subject := range overlay.Subjects {
		if subject.Phase != "" {
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
	// Chains are derived: the saved overlay has none, the hydrated index has them again.
	encoded, err := groupindex.Encode(overlay)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"chains"`) {
		t.Fatal("derived chains were persisted")
	}
	restored, err := groupindex.Decode(encoded, index)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Chains) != len(overlay.Chains) || !reflect.DeepEqual(restored.Operations, overlay.Operations) || !reflect.DeepEqual(restored.Subjects, overlay.Subjects) {
		t.Fatalf("hydrated chains = %d, projected %d; operations equal: %v", len(restored.Chains), len(overlay.Chains), reflect.DeepEqual(restored.Operations, overlay.Operations))
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

// echoPreset answers every table the reading asks the way a careful reader
// would, from the row alone. Text cells get a placeholder; choices get the
// reader's decision, or the first option where any answer is fine.
type echoPreset struct {
	sawRegistration, sawSQL, sawQuerySource, sawRepositorySource, sawEntryPart bool
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
		Mode  string           `json:"mode"`
		Items []struct {
			Ref  string `json:"ref"`
			Path string `json:"path"`
		} `json:"items"`
	}
	if err := json.Unmarshal(prepared.Bytes(), &request); err != nil {
		return llm.Completion{}, err
	}
	var response []byte
	switch {
	case request.Task != "" && request.Mode == "parts":
		// Two parts a reader would draw: what serves requests and what holds
		// the data. Every other declaration stays source inventory.
		parts := map[string][]string{}
		for _, item := range request.Items {
			switch {
			case strings.Contains(item.Path, "/handler/"):
				parts["Request handling"] = append(parts["Request handling"], item.Ref)
			case strings.Contains(item.Path, "/database/"):
				parts["Stored users"] = append(parts["Stored users"], item.Ref)
			}
		}
		var groups []map[string]any
		for _, title := range []string{"Request handling", "Stored users"} {
			if len(parts[title]) > 0 {
				groups = append(groups, map[string]any{"title": title, "purpose": title + ".", "members": parts[title]})
			}
		}
		var err error
		if response, err = json.Marshal(map[string]any{"groups": groups}); err != nil {
			return llm.Completion{}, err
		}
	case request.Task != "":
		response = []byte(`{"groups":[]}`)
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
		symbol, _ := row["symbol"].(string)
		switch {
		case table == "atlas_api":
			// The roles a reader gives the three symbols that matter here;
			// every other symbol gets no cell.
			switch {
			case name == "binds" && strings.HasSuffix(symbol, "echo/v4.Echo.GET"):
				preset.sawRegistration = true
				answer["binds"] = "http_server"
			case name == "publishes" && strings.HasSuffix(symbol, "echo/v4.Echo.Start"):
				answer["publishes"] = "yes"
			case name == "talks" && symbol == "database/sql.Open":
				answer["talks"] = "db"
			}
		case table == "atlas_layers" && name == "role":
			// The query itself is never asked: the code names the access.
			source, _ := row["source"].(string)
			switch {
			case strings.Contains(source, "QueryRowContext"):
				preset.sawQuerySource = true
			case strings.Contains(source, ".JSON("):
				answer["role"] = "logic"
			case strings.Contains(source, "model.User{"):
				preset.sawRepositorySource = true
				answer["role"] = "adapter"
			default:
				answer["role"] = "passthrough"
			}
		case table == "atlas_core":
			// The part requests enter is what this program exists for; every
			// other cell stays empty, and an empty cell is no.
			if name == "core" && row["entries"] != nil {
				preset.sawEntryPart = true
				answer["core"] = "yes"
			}
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
