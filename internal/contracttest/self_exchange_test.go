package contracttest

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// A program's request to a route it serves itself is an exchange with
// itself: the Go fixture's fetchLevels gets /api/levels, a path with no
// host, and the same program's registerLevelRoute serves /api/levels, so
// the request is joined to that route (an integration connection within
// the program), no outside system. litestream's subcommands post to
// http://localhost/start and /info, which its own Server serves, and had
// stood as an Outside "Litestream". A request to another host with the
// same path stays another server's. The other languages share the rule
// (reading self_joints.go) over their own requests and routes; their
// fixtures request no route their own program serves.
func TestAProgramsRequestToItsOwnRouteJoinsThatRoute(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	app := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "self exchange")
	index, err := goadapter.Build(repository, app.target, app.origins, app.direct, app.external, app.core, app.dynamic, app.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph := graphWithFacts(t, repository, places.TargetInput{Index: index, Root: "."})
	preset := &inputsPreset{decide: func(column string, item map[string]any, _ []string) (string, bool) {
		symbol, _ := item["symbol"].(string)
		switch {
		case column == "talks" && symbol == "net/http.Get":
			return "client_request", true
		case column == "binds" && strings.HasSuffix(symbol, "net/http.HandleFunc"):
			return "request", true
		}
		return "", false
	}}
	projected := readInputs(t, graph, index, reading.TargetMeta{ID: index.Target.ID, Language: "go", Kind: "executable", Name: index.Target.Name, Root: "."}, root, preset)
	names := map[string]string{}
	for _, subject := range projected.Subjects {
		if subject.Object != nil {
			names[subject.ID] = subject.Object.Name
		}
	}
	var joined []string
	for _, connection := range projected.Connections {
		if connection.SourceKind == "integration" && connection.From.TargetID == connection.To.TargetID {
			joined = append(joined, fmt.Sprintf("%s -> %s %s", names[connection.FromSubjectID], names[connection.ToSubjectID], connection.Summary))
		}
	}
	if !slices.Contains(joined, "fetchLevels -> getLevel GET /api/levels") {
		t.Fatalf("the program's own request joins %q, want fetchLevels to getLevel, the handler of its /api/levels route", joined)
	}
}
