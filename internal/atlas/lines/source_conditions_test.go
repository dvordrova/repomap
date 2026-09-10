package lines

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestEmptyCallCatalogueDoesNotAskOutboundOrRemoveKeyAndActivation(t *testing.T) {
	def := SymbolSelection(false)
	rows := []table.Row{{Fields: []table.Field{{Name: "call_options", Value: []string{}}}}, {Fields: []table.Field{{Name: "call_options", Value: []string{"c1"}}, {Name: "call_count", Value: 1}}}}
	window := table.Window{Rows: rows}
	raw, err := table.Request(def, window)
	if err != nil || !strings.Contains(string(raw), `"when_options_nonempty":"call_options"`) {
		t.Fatalf("missing input condition: %s %v", raw, err)
	}
	result, err := table.DecodeResult(def, window, []byte(`{"rows":[{"key":"r1","key_symbol":"yes","activation":"unassessed","outbound":42},{"key":"r2","key_symbol":"no","activation":"none","outbound":"c1"}]}`))
	if err != nil || len(result.Rejections) != 0 || result.Answers[0]["key_symbol"] != "yes" || result.Answers[0]["activation"] != "unassessed" || len(result.Answers[0]) != 2 || result.Answers[1]["outbound"] != "c1" {
		t.Fatalf("input condition changed supported decisions: %+v %v", result, err)
	}
	missing, err := table.DecodeResult(def, window, []byte(`{"rows":[{"key":"r1","key_symbol":"yes","activation":"unassessed"},{"key":"r2","key_symbol":"no","activation":"none"}]}`))
	if err != nil || missing.Answers[0] == nil || missing.Answers[1] != nil {
		t.Fatalf("empty calls requested outbound or real calls stopped requiring it: %+v %v", missing, err)
	}
}

func TestSingletonBindingIsOneCompleteObject(t *testing.T) {
	binding := atlas.SymbolBinding{From: "Routes", To: "Proxy", Path: "server/routes.go", Line: 19, Invocation: "callback_transfer", Resolution: "alternatives",
		Arguments: []atlas.RegistrationArgument{{Kind: "literal_string", Value: "/proxy", Path: "server/routes.go", Line: 19}}, Evidence: []atlas.EdgeEvidence{{Path: "server/routes.go", LineNo: 19, Extractor: "binding"}}}
	before, _ := json.Marshal(binding)
	var catalogue EvidenceCatalog
	projected := catalogue.Bindings([]atlas.SymbolBinding{binding})
	raw, err := json.Marshal(projected)
	if err != nil || strings.Contains(string(raw), `"rows"`) || strings.Contains(string(raw), `"columns"`) {
		t.Fatalf("singleton retained tabular shell: %s %v", raw, err)
	}
	var restored struct {
		atlas.SymbolBinding
		EvidenceRefs []string `json:"evidence_refs"`
	}
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	for _, ref := range restored.EvidenceRefs {
		restored.Evidence = append(restored.Evidence, catalogue.byRef[ref])
	}
	after, _ := json.Marshal(binding)
	if !reflect.DeepEqual(restored.SymbolBinding, binding) || string(before) != string(after) {
		t.Fatal("singleton lost evidence or changed source")
	}
}
