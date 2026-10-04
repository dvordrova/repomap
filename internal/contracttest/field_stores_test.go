package contracttest

import (
	"fmt"
	"slices"
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
		Arguments: map[string]reading.ArgumentChoice{"net/http.Get": {Position: 1}, "database/sql.Open": {Position: 2}},
		Talks:     map[string]string{"database/sql.Open": "db", "database/sql.DB.Exec": "db", "database/sql.DB.Ping": "db", "net/http.Get": "client_request"},
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
				found["ping"] = ends(reader.Read(place, call))
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
	if !slices.Equal(found["pingPool"], []string{"|initializer: file:pool.db"}) {
		t.Fatalf("the unrelated pool's ping = %q, want the pool database as a possible origin only", found["pingPool"])
	}
	if !slices.Equal(found["ping"], []string{"http://localhost:8080/status|", "|written by a call it is handed to"}) {
		t.Fatalf("the status ping = %q, want its default and the flag's unknown write", found["ping"])
	}
}
