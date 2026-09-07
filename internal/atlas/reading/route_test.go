package reading

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
	"github.com/dvordrova/repomap/internal/atlas/table"
)

func TestGuideOrdersRestoredAnchorsAndKeepsCandidateEvidence(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through = lines.StageRoute
	provider.routeFor = func(row map[string]any) table.Answer { return table.Answer{"order": "c4 c2"} }
	result, route := readQuestionResult(t, opts)
	if result.Complete || result.Through != lines.StageRoute || len(result.Uses) != 2 {
		t.Fatalf("unexpected stages: %+v", result.Uses)
	}
	if route.Guide.State != "ready" || len(route.Guide.Steps) != 2 || route.Guide.Candidates != 4 || len(route.Stops) != 4 {
		t.Fatalf("guide: %+v", route.Guide)
	}
	if route.Guide.Steps[0].Path != route.Stops[3].Path || route.Guide.Steps[1].Path != route.Stops[1].Path {
		t.Fatal("model order not restored")
	}
	for _, stop := range route.Stops {
		if len(stop.Evidence) == 0 {
			t.Fatal("original evidence lost")
		}
	}
	calls := provider.calls
	opts.OwnerRunDir = t.TempDir()
	_, warm := readQuestionResult(t, opts)
	if provider.calls != calls || warm.Guide.Source != atlas.SourceCache {
		t.Fatal("warm route did not reuse same evidence")
	}
	opts.OwnerRunDir, opts.Prompt = t.TempDir(), "Change the route prompt only."
	result, _ = readQuestionResult(t, opts)
	for _, use := range result.Uses {
		if use.Stage == lines.StageQuestion && use.Live > 0 {
			t.Fatal("route edit invalidated retrieval")
		}
	}
}

func TestGuideConsidersTheTailAcrossReductionRounds(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through, opts.WindowRows = lines.StageRoute, 1
	opts.InputBytes = 16000
	file := &opts.Graph.Places[0]
	for i := range opts.Graph.Places {
		if opts.Graph.Places[i].Path == "pkg/a/x.go" && opts.Graph.Places[i].File != nil {
			file = &opts.Graph.Places[i]
			break
		}
	}
	// One relevant declaration per complete chunk makes > 3 candidate pools.
	file.File.Decls = nil
	for i := 0; i < 80*lines.QuestionChunkAnchors; i++ {
		file.File.Decls = append(file.File.Decls, atlas.Decl{Name: fmt.Sprintf("Storage%d", i), Kind: "function", LineNo: i + 1})
	}
	provider.routeFor = func(row map[string]any) table.Answer {
		options := row["candidate_options"].([]any)
		// Always retain the last candidate, plus the first, to test that the
		// tail survives comparison rather than deterministic top-N truncation.
		limit := int(row["preferred_steps"].(float64))
		order := options[len(options)-1].(string)
		if len(options) > 1 && limit > 1 {
			order += " " + options[0].(string)
		}
		return table.Answer{"order": order}
	}
	_, route := readQuestionResult(t, opts)
	if route.Guide.Candidates != 83 || len(route.Guide.Rounds) < 2 || route.Guide.Rounds[0].Candidates != 83 {
		t.Fatalf("coverage: %+v", route.Guide)
	}
	if route.Guide.State != "ready" || len(route.Guide.Steps) > lines.RouteSteps {
		t.Fatalf("guide: %+v", route.Guide)
	}
	requests, _ := filepath.Glob(filepath.Join(opts.OwnerRunDir, atlas.TablesDir, "atlas_route-*.input.ref.json"))
	seen := map[string]bool{}
	for _, filename := range requests {
		raw, _ := readWindowPayload(filename)
		if strings.Contains(string(raw), "file:") || strings.Contains(string(raw), "sym:") {
			t.Fatal("internal IDs sent")
		}
		if len(raw)+len(lines.Route().System) > opts.InputBytes {
			t.Fatal("context budget exceeded")
		}
		var request struct {
			Rows []struct {
				Candidates []struct {
					Path string `json:"path"`
					Line int    `json:"line"`
				} `json:"candidates"`
			} `json:"rows"`
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
		for _, row := range request.Rows {
			for _, candidate := range row.Candidates {
				seen[fmt.Sprintf("%s:%d", candidate.Path, candidate.Line)] = true
			}
		}
	}
	if len(seen) != 83 {
		t.Fatalf("only %d candidates reached the route model", len(seen))
	}
}

func TestIntermediateGuideKeepsAdditionalKnownLocationsAndStopsAtAFixedPoint(t *testing.T) {
	for _, keepAll := range []bool{false, true} {
		t.Run(fmt.Sprintf("keep_all_%t", keepAll), func(t *testing.T) {
			opts, provider := questionFixture(t)
			opts.Through, opts.InputBytes = lines.StageRoute, 16000
			for i := range opts.Graph.Places {
				file := &opts.Graph.Places[i]
				if file.Path != "pkg/a/x.go" || file.File == nil {
					continue
				}
				file.File.Decls = nil
				for j := 0; j < 80*lines.QuestionChunkAnchors; j++ {
					file.File.Decls = append(file.File.Decls, atlas.Decl{Name: fmt.Sprintf("Storage%d", j), Kind: "function", LineNo: j + 1})
				}
			}
			provider.routeFor = func(row map[string]any) table.Answer {
				options := row["candidate_options"].([]any)
				count := len(options)
				if !keepAll {
					count = min(count, 8)
				}
				var refs []string
				for _, ref := range options[:count] {
					refs = append(refs, ref.(string))
				}
				return table.Answer{"order": strings.Join(refs, " "), "open_question": "Inspect how this evidence fits with the other sources."}
			}
			result, route := readQuestionResult(t, opts)
			guide := route.Guide
			if len(result.Rejected) > 0 || guide.Rounds[0].Pools < 2 || guide.Rounds[0].Selected != guide.Rounds[0].Pools*8 && !keepAll {
				t.Fatalf("valid intermediate sources were rejected: %+v / %+v", guide, result.Rejected)
			}
			if !keepAll {
				if guide.State != "ready" || len(guide.Steps) != 8 || len(guide.Parts) != 0 {
					t.Fatalf("complementary evidence did not reach final reading: %+v", guide)
				}
				// A valid empty choice in every independent pool is not a
				// failed provider response and must remain distinguishable.
				opts.OwnerRunDir, opts.Prompt = t.TempDir(), "Select no useful reading sources."
				provider.routeFor = func(map[string]any) table.Answer { return table.Answer{"order": "none"} }
				result, empty := readQuestionResult(t, opts)
				if len(result.Rejected) > 0 || empty.Guide.State != "empty" || len(empty.Guide.Steps) != 0 || empty.Guide.Rounds[0].Pools < 2 {
					t.Fatalf("valid empty selections became a failure: %+v", empty.Guide)
				}
				return
			}
			if guide.State != "partitioned" || len(guide.Rounds) != 1 || len(guide.Steps) != 83 || len(guide.Parts) != guide.Rounds[0].Pools {
				t.Fatalf("a fixed point lost evidence or was retried: %+v", guide)
			}
			seen := make(map[string]bool)
			for _, part := range guide.Parts {
				if part.Source != atlas.SourceModel || part.OpenQuestion == "" {
					t.Fatal("independent order lost its model origin or gap")
				}
				for _, step := range part.Steps {
					key := fmt.Sprintf("%s:%d", step.Path, step.Line)
					if seen[key] {
						t.Fatalf("duplicated source %s", key)
					}
					seen[key] = true
				}
			}
			if len(seen) != 83 {
				t.Fatalf("preserved %d of 83 sources", len(seen))
			}
			calls := provider.calls
			opts.OwnerRunDir = t.TempDir()
			_, warm := readQuestionResult(t, opts)
			if provider.calls != calls || warm.Guide.Parts[0].Source != atlas.SourceCache {
				t.Fatal("partitioned readings did not reuse the exact cache")
			}
		})
	}
}

func TestGuideDeduplicatesLocationsWithoutLosingProducerObservations(t *testing.T) {
	stops := []atlas.QuestionStop{
		{PlaceID: "entity:one", Path: "schema.sql", Line: 1, Evidence: map[string]any{"extractor": "sqlc"}},
		{PlaceID: "entity:two", Path: "schema.sql", Line: 1, Evidence: map[string]any{"extractor": "company"}},
	}
	anchors := uniqueRouteAnchors(stops)
	if len(anchors) != 1 || len(anchors[0].StopIndexes) != 2 {
		t.Fatal("duplicate source location or dropped producer")
	}
	row := routePoolRow(&atlas.QuestionRoute{Stops: stops}, anchors, true)
	raw, _ := table.Request(lines.Route(), table.Window{Stage: lines.StageRoute, Rows: []table.Row{row}})
	if !strings.Contains(string(raw), "sqlc") || !strings.Contains(string(raw), "company") || strings.Contains(string(raw), "entity:") {
		t.Fatal("producer evidence or identity handling changed")
	}
}

func TestFinalGuideKeepsAdditionalKnownLocations(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through = lines.StageRoute
	for i := range opts.Graph.Places {
		place := &opts.Graph.Places[i]
		if place.Path != "pkg/a/x.go" || place.File == nil {
			continue
		}
		place.File.Decls = nil
		for j := 0; j < 10*lines.QuestionChunkAnchors; j++ {
			place.File.Decls = append(place.File.Decls, atlas.Decl{Name: fmt.Sprintf("Storage%d", j), Kind: "function", LineNo: j + 1})
		}
	}
	provider.routeFor = func(row map[string]any) table.Answer {
		options := row["candidate_options"].([]any)
		var order []string
		for _, ref := range options[:min(8, len(options))] {
			order = append(order, ref.(string))
		}
		return table.Answer{"order": strings.Join(order, " ")}
	}
	result, route := readQuestionResult(t, opts)
	if len(result.Rejected) != 0 || route.Guide.State != "ready" || len(route.Guide.Steps) != 8 {
		t.Fatalf("valid anchors were lost due to a presentation preference: %+v / %+v", route.Guide, result.Rejected)
	}
}

func TestRefusedRouteDoesNotInventAReadingOrder(t *testing.T) {
	opts, provider := questionFixture(t)
	opts.Through = lines.StageRoute
	provider.routeFor = func(map[string]any) table.Answer { return table.Answer{"order": "c999"} }
	result, route := readQuestionResult(t, opts)
	if len(result.Rejected) != 1 || route.Guide.State != "unavailable" || len(route.Guide.Steps) != 0 || len(route.Stops) != 4 || route.Guide.Rounds[0].UnresolvedPools != 1 {
		t.Fatalf("refusal became a route: %+v", route.Guide)
	}
	opts.OwnerRunDir, opts.Provider = t.TempDir(), nil
	_, route = readQuestionResult(t, opts)
	if route.Guide.State != "unavailable" || len(route.Guide.Steps) != 0 {
		t.Fatal("no-provider mode invented a guide")
	}
}

func TestRouteBudgetSplitsWholeCandidates(t *testing.T) {
	route := &atlas.QuestionRoute{}
	for i := 0; i < 9; i++ {
		route.Stops = append(route.Stops, atlas.QuestionStop{Path: fmt.Sprintf("f%d.go", i), Line: 1, Evidence: map[string]any{"author_doc": strings.Repeat("word ", 250)}})
	}
	def := lines.Route()
	def.MaxInputBytes = 6500
	pools, err := planRoutePools(def, nil, route, uniqueRouteAnchors(route.Stops))
	if err != nil {
		t.Fatal(err)
	}
	if len(pools) < 2 {
		t.Fatal("did not split oversized pool")
	}
	count := 0
	for _, pool := range pools {
		count += len(pool.candidates)
		raw, _ := table.Request(def, table.Window{Stage: def.Stage, Rows: []table.Row{pool.row}})
		if len(raw)+len(def.System) > def.MaxInputBytes {
			t.Fatal("oversized pool")
		}
	}
	if count != 9 {
		t.Fatal("lost candidates while splitting")
	}
	def.MaxInputBytes = 100
	if _, err := planRoutePools(def, nil, route, uniqueRouteAnchors(route.Stops)); err == nil {
		t.Fatal("oversized singleton accepted")
	}
}

func TestRouteContextAggregatesFileCallsAndKeepsOriginalWitnesses(t *testing.T) {
	route := &atlas.QuestionRoute{Stops: []atlas.QuestionStop{
		{PlaceID: "a", Path: "a.go", Line: 1, Name: "SelectedA"},
		{PlaceID: "b", Path: "b.go", Line: 1, Name: "SelectedB"},
	}, Connections: []atlas.QuestionConnection{{FromID: "a", ToID: "b", Kind: "calls", Count: 4000, Witnesses: []atlas.Witness{{Caller: "UnrelatedCaller", Callee: "UnrelatedCallee", Path: "a.go", LineNo: 500}}}}}
	row := routePoolRow(route, uniqueRouteAnchors(route.Stops), true)
	raw, err := table.Request(lines.Route(), table.Window{Rows: []table.Row{row}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "UnrelatedCaller") || !strings.Contains(string(raw), "files_or_observed_entities") || !strings.Contains(string(raw), "4000") {
		t.Fatalf("file connection is not a compact, labelled observation: %s", raw)
	}
	if route.Connections[0].Witnesses[0].Caller != "UnrelatedCaller" {
		t.Fatal("source evidence was removed from the question artifact")
	}
}
