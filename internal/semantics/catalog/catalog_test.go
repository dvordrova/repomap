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
	if len(catalog.Seeds) != 12 {
		t.Fatalf("seed count = %d, want 12", len(catalog.Seeds))
	}
	if catalog.Seeds[0].ID != "gin-router-group-handle" {
		t.Fatalf("first sorted seed = %q", catalog.Seeds[0].ID)
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
