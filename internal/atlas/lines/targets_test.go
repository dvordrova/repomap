package lines

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
)

func TestTargetRowRendersTheLabelsThePromptDefines(t *testing.T) {
	row := TargetRow(TargetSummary{ID: "svc", Name: "svc", Root: "cmd/svc", Language: "go", Kind: "executable", Readme: "A service.", Entrypoint: "cmd/svc/main.go",
		Files: 12, Dirs: 3, Boundaries: map[string]int{"http_client out": 3, "http_server in": 2, "config out": 1}, Operations: []string{"request: submit job"}})
	fields := make(map[string]any)
	for _, field := range row.Fields {
		fields[field.Name] = field.Value
	}
	if fields["kind"] != "executable" || !reflect.DeepEqual(fields["boundaries"], []string{"config out 1", "http_client out 3", "http_server in 2"}) {
		t.Fatalf("target row labels: %+v", fields)
	}
	prompt := Targets().System
	for _, defined := range []string{"`shared_code`", "`module_library`", "`http_client out 3`", "`config`", "`listen_address`"} {
		if !strings.Contains(prompt, defined) {
			t.Fatalf("targets prompt does not define %s", defined)
		}
	}
	for _, role := range atlas.Roles() {
		if !strings.Contains(prompt, "`"+role+"`") {
			t.Fatalf("targets prompt does not define role %s", role)
		}
	}
	if !strings.HasPrefix(Targets().Contract, "repomap.atlas.targets.v3") {
		t.Fatalf("targets contract not raised: %s", Targets().Contract)
	}
}
