package contracttest

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/places"
	"github.com/dvordrova/repomap/internal/atlas/reading"
	"github.com/dvordrova/repomap/internal/programindex/goadapter"
)

// What a Go field holds (cmd/app/field_stores.go). The guarded count's
// instance is followed to the store its composite literal copied, connected
// to the items database: an established address. pingPool's configuration
// no caller names, so what any write of poolConfig.DB stored (the pool
// database; the nil write stores nothing) is only a possible origin, an
// unresolved end that joins no engine's destination. statusEndpoint's url is
// its default or what flag.StringVar writes through its address: the ping
// reads both, the second unknown.
func TestGoFieldReadsHoldEveryWriteOfTheField(t *testing.T) {
	t.Setenv("CGO_ENABLED", "0")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("GOWORK", "off")
	root, repository := materializeFixtureRepository(t, "go")
	writePublishedGoFixtureModule(t, root)
	authorities := analyzeGoFixture(t, root, repository, goFixtureAppPackage, "field-stores")
	index, err := goadapter.Build(repository, authorities.target, authorities.origins, authorities.direct, authorities.external, authorities.core, authorities.dynamic, authorities.tests)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := places.Build(places.Input{Repository: repository, Targets: []places.TargetInput{{Index: index}}})
	if err != nil {
		t.Fatal(err)
	}
	reader := reading.NewDestinationReader(graph.Places, reading.DestinationChoices{
		Arguments:   map[string]reading.ArgumentChoice{"net/http.Get": {Position: 1}, "database/sql.Open": {Position: 2}},
		FieldWrites: graph.FieldWrites,
		Talks:       map[string]string{"database/sql.Open": "db", "database/sql.DB.Exec": "db", "database/sql.DB.Ping": "db", "net/http.Get": "client_request"},
	})
	ends := func(uses []atlas.DestinationUse) []string {
		var result []string
		for _, use := range uses {
			result = append(result, fmt.Sprintf("%s|%s", use.Address, use.Frontier))
		}
		slices.Sort(result)
		return slices.Compact(result)
	}
	// callerOf is the deliverBoth line a walk passed, by the address its
	// argument writes there.
	callers := map[string]string{}
	found := map[string][]string{}
	for _, place := range graph.Places {
		if place.Symbol == nil || place.Path != "cmd/app/field_stores.go" {
			continue
		}
		for _, call := range place.Symbol.Calls {
			if call.API == nil {
				continue
			}
			switch call.API.Name {
			case "Get":
				uses := reader.Read(place, call)
				found[place.Symbol.Decl.Name] = ends(uses)
				if place.Symbol.Decl.Name == "deliver" {
					for _, use := range uses {
						for _, step := range use.Steps {
							if step.Name == "deliverBoth" && strings.HasPrefix(use.Frontier, "initializer: ") {
								callers[fmt.Sprint(step.Line)] += strings.TrimPrefix(use.Frontier, "initializer: ") + " "
							}
						}
					}
				}
			case "DB.Exec", "Exec":
				found["count"] = ends(reader.Exchange(place, call, "db"))
			case "DB.Ping", "Ping":
				found["pingPool"] = ends(reader.Exchange(place, call, "db"))
			}
		}
	}
	if !slices.Equal(found["count"], []string{"file:items.db|"}) {
		t.Fatalf("the guarded count's exchange = %q, want the items database it was connected to", found["count"])
	}
	if !slices.Contains(found["pingPool"], "|initializer: file:pool.db") || slices.ContainsFunc(found["pingPool"], func(end string) bool { return !strings.HasPrefix(end, "|") }) {
		t.Fatalf("the unrelated pool's ping = %q, want the pool database as a possible origin only", found["pingPool"])
	}
	// Each caller's request reads its own address as the write's value,
	// never the other caller's.
	if len(callers) != 2 || !slices.ContainsFunc(slices.Collect(maps.Values(callers)), func(v string) bool { return v == "https://a.example " }) ||
		!slices.ContainsFunc(slices.Collect(maps.Values(callers)), func(v string) bool { return v == "https://b.example " }) {
		t.Fatalf("deliver's writes by caller = %q", callers)
	}
	// The webhook's Endpoint is a serviceConfig's URL, whose write is read
	// on too; the unknown webhook stays.
	if !slices.Contains(found["fire"], "|initializer: https://service.example") || !slices.ContainsFunc(found["fire"], func(end string) bool { return strings.HasPrefix(end, "|w.Endpoint") }) {
		t.Fatalf("fire = %q", found["fire"])
	}
	if !slices.Contains(found["send"], "|initializer: https://service.example") || len(found["send"]) < 2 {
		t.Fatalf("send's URL = %q, want the possible write beside its unresolved configurations", found["send"])
	}
	if !slices.Equal(found["statusEndpoint.ping"], []string{"http://localhost:8080/status|", "|written by a call it is handed to"}) {
		t.Fatalf("the status ping = %q, want its default and the flag's unknown write", found["statusEndpoint.ping"])
	}
}
