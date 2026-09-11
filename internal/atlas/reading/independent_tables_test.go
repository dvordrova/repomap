package reading

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
	"github.com/dvordrova/repomap/internal/llm"
)

func TestIndependentRelationsKeepNeighboursAndExactResponseCache(t *testing.T) {
	for _, def := range []table.Definition{lines.Arrows(), lines.Targets(), lines.Targets(true), lines.ZoneAssign([]string{"Core"}), lines.ZoneLines(), lines.Joints(), lines.Peers()} {
		t.Run(def.Contract, func(t *testing.T) {
			rows := []table.Row{{ID: "first"}, {ID: "second"}, {ID: "third"}}
			var wireRows []map[string]any
			for i := range rows {
				rows[i].Fields = []table.Field{{Name: "peer_options", Value: []string{"none", "p1"}}}
				wire := map[string]any{"key": table.Key(i), "extra": []int{1}}
				for _, column := range def.Columns {
					value := "Accepted prose."
					if column.Kind == table.Choice {
						value = "p1"
						if len(column.Options) > 0 {
							value = column.Options[0]
						}
					}
					wire[column.Name] = value
				}
				wireRows = append(wireRows, wire)
			}
			column := def.Columns[0].Name
			wireRows[1][column] = 42
			response, err := json.Marshal(map[string]any{"rows": wireRows, "extra": true})
			if err != nil {
				t.Fatal(err)
			}
			base := &independentResponseProvider{response: response}
			provider := &parsedRowAdapter{Provider: base}
			r := answerTestReader(t, nil, provider)
			r.opts.Through = ""
			// These rows have no knowledge entities. Their shared response is the
			// cache unit even though validation accepts each row separately.
			first, err := r.runTable(t.Context(), def, 1, rows)
			if err != nil || base.calls != 1 || len(first) != 3 || first[0].answer == nil || first[1].answer != nil || first[2].answer == nil {
				t.Fatalf("independent rows used knowledge routing or lost neighbours: %+v / %v", first, err)
			}
			if len(r.knowledge) != 0 || first[1].source != atlas.SourceGiven || len(r.rejected) != 1 || r.rejected[0].Samples[0] != "r2" {
				t.Fatalf("invalid row gained an answer or lost diagnostics: %+v", r.rejected)
			}
			exchange, found, err := llm.CachedExchange(r.opts.Executor.RootDir, first[0].requestKey)
			if err != nil || !found || !bytes.Equal(exchange.Response, response) || first[2].requestKey != first[0].requestKey {
				t.Fatalf("partial response lost exact cache: %v", err)
			}
			windows, err := table.Windows(def, 1, rows)
			if err != nil {
				t.Fatal(err)
			}
			current, _ := table.State(def, windows[0])
			old := def
			old.Independent, old.Memoize = false, false
			previous, _ := table.State(old, windows[0])
			if !bytes.Equal(current, previous) {
				t.Fatal("row isolation changed the existing request identity")
			}
			warm, err := r.runTable(t.Context(), def, 1, rows)
			if err != nil || base.calls != 1 || warm[0].source != atlas.SourceCache || warm[1].answer != nil || warm[2].source != atlas.SourceCache {
				t.Fatalf("same accepted neighbours requested again: %+v / %v", warm, err)
			}
			// The same exact request can repair its refused row through replay.
			wireRows[1][column] = wireRows[0][column]
			base.response, _ = json.Marshal(map[string]any{"rows": wireRows})
			prepared, err := llm.NewPrepared(exchange.Request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := llm.ReplayJSON(t.Context(), r.opts.Executor, base, prepared); err != nil {
				t.Fatal(err)
			}
			updated, err := r.runTable(t.Context(), def, 1, rows)
			if err != nil || base.calls != 2 || updated[1].answer == nil || updated[1].source != atlas.SourceCache || updated[0].responseSHA == first[0].responseSHA {
				t.Fatalf("replayed exact response was not revalidated: %+v / %v", updated, err)
			}
			if !reflect.DeepEqual(provider.accepted, [][]string{{"r1", "r3"}, {"r1", "r3"}, {"r1", "r2", "r3"}}) {
				t.Fatalf("rejected-row metadata accepted: %v", provider.accepted)
			}
		})
	}
}

type mutatedTableProvider struct {
	tableProvider
	mutate func(map[string]any, []map[string]any)
}

func (provider *mutatedTableProvider) Complete(ctx context.Context, request llm.Prepared) (llm.Completion, error) {
	completion, err := provider.tableProvider.Complete(ctx, request)
	if err != nil {
		return completion, err
	}
	var input map[string]any
	var response struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(request.Bytes(), &input); err != nil {
		return completion, err
	}
	if err := json.Unmarshal(completion.Response, &response); err != nil {
		return completion, err
	}
	provider.mutate(input, response.Rows)
	completion.Response, err = json.Marshal(response)
	return completion, err
}

func TestRefusedZoneAssignmentDoesNotAcquireMatchingNameOrAncestor(t *testing.T) {
	provider := &mutatedTableProvider{tableProvider: tableProvider{partNames: []string{"Rejected title", "Accepted title", "Other part", "Spare part"}}}
	provider.mutate = func(input map[string]any, rows []map[string]any) {
		mode := input["context"].(map[string]any)["question"]
		if mode == "assign" {
			for i, row := range rows {
				row["part"] = "Accepted title"
				if i == 1 {
					row["part"] = "unknown part"
				}
			}
		}
	}
	r := answerTestReader(t, nil, provider)
	r.opts.Through = ""
	r.places, r.boxes, r.zones = make(map[string]atlas.Place), make(map[string]*boxState), make(map[string][]*zoneState)
	for i, dir := range []string{"a", "a/b", "c", "d", "a/b/child", "a/kept"} {
		fileID := fmt.Sprintf("file-%d", i)
		r.places[fileID] = atlas.Place{ID: fileID, TargetIDs: []string{"t"}}
		title := dir
		if i == 1 {
			title = "Rejected title"
		}
		r.boxes[dir] = &boxState{id: dir, dir: dir, title: title, top: i < 4, files: []string{fileID}, zoneID: make(map[string]string)}
	}
	var journal []string
	r.opts.State = func(_, _ string, details ...string) { journal = append(journal, details...) }
	if err := r.readTargetZones(t.Context(), "t"); err != nil {
		t.Fatal(err)
	}
	if len(r.zones["t"]) != 1 || r.zones["t"][0].title != "Accepted title" || len(r.zones["t"][0].boxes) != 4 || r.boxes["a/b"].zoneID["t"] != "" || r.boxes["a/b/child"].zoneID["t"] != "" {
		t.Fatalf("refused row gained a semantic zone: %+v / %v", r.zones["t"], r.boxes["a/b"].zoneID)
	}
	// The three named parts no box chose are dropped, and the run says so.
	if text := strings.Join(journal, "\n"); !strings.Contains(text, "target t: 3 named parts held no box after assignment and were dropped: Rejected title, Other part, Spare part") {
		t.Fatalf("dropped parts are not journaled: %v", journal)
	}
	if r.boxes["a"].zoneID["t"] != "accepted-title" || len(r.rejected) != 1 || r.rejected[0].Samples[0] != "r2" {
		t.Fatal("valid neighbouring assignments or local rejection lost")
	}
}

func TestRefusedJointAndPeerRowsDoNotCreateConnections(t *testing.T) {
	for _, mode := range []string{"joints", "peers"} {
		t.Run(mode, func(t *testing.T) {
			provider := &mutatedTableProvider{}
			provider.mutate = func(input map[string]any, rows []map[string]any) {
				question := input["context"].(map[string]any)["question"]
				for _, row := range rows {
					if question == "joints" {
						if row["key"] == "r2" {
							row["same"] = "unsupported"
						}
					} else if mode == "joints" {
						row["peer"] = "none"
					} else if row["key"] == "r2" {
						row["peer"] = "p999"
					}
				}
			}
			r := answerTestReader(t, nil, provider)
			r.opts.Through = ""
			r.opts.Targets = []TargetMeta{{ID: "client"}, {ID: "service"}}
			r.targets = map[string]*targetState{"client": {role: atlas.RoleProduct}, "service": {role: atlas.RoleProduct}}
			r.boundaries = make(map[string]*boundaryState)
			for i, id := range []string{"out1", "out2", "in"} {
				target, direction, kind := "client", atlas.DirectionOut, atlas.BoundaryHTTPClient
				if id == "in" {
					target, direction, kind = "service", atlas.DirectionIn, atlas.BoundaryHTTPServer
				}
				value := "shared-value"
				if mode == "peers" {
					value = fmt.Sprintf("distinct-%d", i)
				}
				// Distinct semantic inputs keep two independently rejectable rows.
				r.boundaries[id] = &boundaryState{line: id + " behavior", kind: kind, place: atlas.Place{ID: id, TargetIDs: []string{target}, Boundary: &atlas.BoundaryFacts{Direction: direction, Values: []string{value}}}}
			}
			if err := r.readJoints(t.Context()); err != nil {
				t.Fatal(err)
			}
			if len(r.joints) != 1 || r.joints[0].From.BoundaryID != "out1" || r.joints[0].Blind != (mode == "peers") || len(r.rejected) != 1 || r.rejected[0].Samples[0] != "r2" {
				t.Fatalf("refused connection gained authority or removed its neighbour: %+v / %+v", r.joints, r.rejected)
			}
		})
	}
}
