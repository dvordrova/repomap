package contracttest

import (
	"fmt"
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
	found := map[string][]string{}
	// callers are, by the deliverBoth line a walk passed, the addresses
	// deliver's write read there.
	callers := map[int][]string{}
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
								callers[step.Line] = append(callers[step.Line], strings.TrimPrefix(use.Frontier, "initializer: "))
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
	// A write of a field the address walk reaches, made by a function the
	// walk passed through, is walked as any value: deliver's write reads its
	// parameter, so each caller's request reads its own address and never
	// the other's, beside the unknown instances. A write made anywhere else
	// is named by its site and value: fire's Endpoint is c.URL, written in
	// copyEndpoint, not followed further.
	if !slices.Equal(found["deliver"], []string{"|initializer: https://a.example", "|initializer: https://b.example", "|main.endpoints[\"a\"].URL", "|main.endpoints[\"b\"].URL"}) {
		t.Fatalf("deliver = %q", found["deliver"])
	}
	for line, addresses := range callers {
		if slices.Sort(addresses); len(slices.Compact(addresses)) != 1 {
			t.Fatalf("deliverBoth line %d reads %q, want its own address alone", line, addresses)
		}
	}
	if len(callers) != 2 {
		t.Fatalf("deliver's addresses by caller = %v", callers)
	}
	if !slices.Equal(found["fire"], []string{"|initializer: c.URL", "|w.Endpoint"}) {
		t.Fatalf("fire = %q", found["fire"])
	}
	if !slices.Contains(found["send"], "|initializer: https://service.example") || len(found["send"]) < 2 {
		t.Fatalf("send's URL = %q, want the possible write beside its unresolved configurations", found["send"])
	}
	if !slices.Equal(found["statusEndpoint.ping"], []string{"http://localhost:8080/status|", "|written by a call it is handed to"}) {
		t.Fatalf("the status ping = %q, want its default and the flag's unknown write", found["statusEndpoint.ping"])
	}
}
