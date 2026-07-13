package catalog

import (
	"strings"
	"testing"
)

func TestBuiltin(t *testing.T) {
	t.Parallel()

	catalog, err := Builtin()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Seeds) != 20 {
		t.Fatalf("seed count = %d, want 20", len(catalog.Seeds))
	}
	seeds := make(map[string]Seed, len(catalog.Seeds))
	for _, seed := range catalog.Seeds {
		seeds[seed.ID] = seed
	}
	for _, id := range []string{
		"echo-v4-echo-add",
		"echo-v4-group-add",
		"caddy-admin-load-routes",
		"caddy-http-route-list-compile",
		"gin-router-group-handle",
		"quic-go-http3-server-serve-listener",
	} {
		if _, ok := seeds[id]; !ok {
			t.Errorf("built-in catalog is missing %q", id)
		}
	}
	group := seeds["echo-v4-group-add"]
	if group.Symbol.PackagePath != "github.com/labstack/echo/v4" ||
		group.Symbol.Receiver != "*Group" || group.Projections["path_prefix"].Field != "prefix" {
		t.Fatalf("Echo group seed = %#v", group)
	}
}

func TestDecodeRejectsInvalidCatalog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "unknown version", input: `{"version":2,"seeds":[{}]}`, want: "unsupported version"},
		{name: "empty seeds", input: `{"version":1,"seeds":[]}`, want: "must not be empty"},
		{name: "name only match", input: `{"version":1,"seeds":[{"id":"bad","operation":"call","symbol":{"name":"Get"},"effect":{"kind":"http_route_registration","transport":"http","framework":"bad"},"projections":{"path":{"source":"argument","index":0}},"scenario":{},"reference":"","fixture_coverage":[]}]}`, want: "exact package"},
		{name: "receiver projection on function", input: `{"version":1,"seeds":[{"id":"bad","operation":"call","symbol":{"package":"p","name":"Add"},"effect":{"kind":"http_route_registration","transport":"http","framework":"bad"},"projections":{"dispatcher":{"source":"receiver"}},"scenario":{},"reference":"","fixture_coverage":[]}]}`, want: "uses a receiver"},
		{name: "receiver field without field", input: `{"version":1,"seeds":[{"id":"bad","operation":"call","symbol":{"package":"p","receiver":"*R","name":"Add"},"effect":{"kind":"http_route_registration","transport":"http","framework":"bad"},"projections":{"prefix":{"source":"receiver_field"}},"scenario":{},"reference":"","fixture_coverage":[]}]}`, want: "needs a receiver and field"},
		{name: "unknown effect", input: `{"version":1,"seeds":[{"id":"bad","operation":"call","symbol":{"package":"p","name":"Add"},"effect":{"kind":"imaginary","transport":"http","framework":"bad"},"projections":{"path":{"source":"argument","index":0}},"scenario":{},"reference":"","fixture_coverage":[]}]}`, want: "unsupported effect kind"},
		{name: "return field outside provider", input: `{"version":1,"seeds":[{"id":"bad","operation":"call","symbol":{"package":"p","name":"Add"},"effect":{"kind":"http_route_registration","transport":"http","framework":"bad"},"projections":{"path":{"source":"return_field","field":"Path"}},"scenario":{},"reference":"","fixture_coverage":[]}]}`, want: "outside route provider"},
		{name: "provider missing handler", input: `{"version":1,"seeds":[{"id":"bad","operation":"call","symbol":{"package":"p","receiver":"R","name":"Routes"},"effect":{"kind":"http_route_provider","transport":"http","framework":"bad"},"projections":{"path":{"source":"return_field","field":"Path"}},"scenario":{},"reference":"","fixture_coverage":[]}]}`, want: "requires \"handler\""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Decode(strings.NewReader(test.input))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}
