package reading

import (
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

// An arrow folded only from import edges has no witness call. Asked anyway,
// the model invents one: Morfeu arrows r7 and r10 came back as "store or
// fetch cached data". Such an arrow keeps its fallback sentence unasked.
func TestArrowsWithoutWitnessesTakeTheFallbackWithoutAModelRow(t *testing.T) {
	provider := &mutatedTableProvider{}
	var asked []string
	provider.mutate = func(input map[string]any, _ []map[string]any) {
		if input["table"] != lines.StageArrows {
			return
		}
		for _, row := range input["rows"].([]any) {
			fields := row.(map[string]any)
			asked = append(asked, fields["from"].(string)+" -> "+fields["to"].(string))
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.opts.Targets = []TargetMeta{{ID: "t"}}
	r.opts.Graph = atlas.Graph{Edges: []atlas.Edge{
		{From: "file:a", To: "file:b", Kind: "calls", Count: 2, Witnesses: []atlas.Witness{{Caller: "Main", Callee: "Help", Path: "a/x.go", LineNo: 4}}},
		{From: "file:a", To: "file:c", Kind: "imports", Count: 3},
	}}
	r.places, r.boxes, r.boxOf = map[string]atlas.Place{}, map[string]*boxState{}, map[string]string{}
	for _, box := range []struct{ id, title string }{{"a", "Alpha"}, {"b", "Beta"}, {"c", "Gamma"}} {
		fileID := "file:" + box.id
		r.places[fileID] = atlas.Place{ID: fileID, Kind: atlas.PlaceFile, Path: box.id + "/x.go", TargetIDs: []string{"t"}}
		r.boxes[box.id] = &boxState{id: box.id, dir: box.id, title: box.title, line: box.title + " does things.", files: []string{fileID}, open: true}
		r.boxOf[fileID] = box.id
	}
	r.knowledge, r.knowledgeSubjects = map[string]*Knowledge{}, map[string]*Knowledge{}
	r.responseTables = map[string]rememberedTable{}
	if err := r.readArrows(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asked, []string{"Alpha: Alpha does things. -> Beta: Beta does things."}) {
		t.Fatalf("rows sent to the model = %q, want only the witnessed arrow", asked)
	}
	sentences := map[string]string{}
	for _, arrow := range r.arrows["t"] {
		sentences[arrow.from+"->"+arrow.to] = arrow.sentence
	}
	if !strings.HasPrefix(sentences["a->b"], "Text for") {
		t.Fatalf("witnessed arrow lost the model's sentence: %q", sentences["a->b"])
	}
	if sentences["a->c"] != "Alpha uses Gamma." {
		t.Fatalf("unwitnessed arrow sentence = %q, want the fallback", sentences["a->c"])
	}
}
