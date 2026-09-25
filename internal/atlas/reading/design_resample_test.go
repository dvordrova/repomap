package reading

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

// A parts answer refused whole is asked once more with the same request: a
// good second draw draws the map, and a second refusal leaves the target an
// explicit map failure after exactly two parts requests. The other target,
// answered well the first time, is asked once.
func TestRefusedPartsAnswerIsAskedOnceMore(t *testing.T) {
	for _, refused := range []int{1, 2} {
		var mu sync.Mutex
		asked := map[string]int{}
		provider := &tableProvider{}
		provider.partsResponse = func(files []map[string]any) string {
			target := strings.SplitN(fmt.Sprint(files[0]["path"]), "/", 2)[0]
			mu.Lock()
			asked[target]++
			draw := asked[target]
			mu.Unlock()
			switch {
			case target != "svc" || draw > refused:
				var groups []string
				for _, file := range files {
					groups = append(groups, fmt.Sprintf(`{"name":%q,"files":[%q]}`, file["path"], file["ref"]))
				}
				return `{"groups":[` + strings.Join(groups, ",") + `]}`
			default:
				return `{"groups":[]}`
			}
		}
		result, err := Read(t.Context(), twoTargetOptions(t, twoTargetGraph(t), provider))
		if err != nil {
			t.Fatal(err)
		}
		if asked["svc"] != 2 || asked["web"] != 1 {
			t.Fatalf("refused %d: parts requests %v", refused, asked)
		}
		svc := targetOf(t, result, "svc")
		if drawn := len(svc.Boxes) > 0 && svc.MapFailure == ""; drawn != (refused == 1) {
			t.Fatalf("refused %d: map drawn %v, failure %q", refused, drawn, svc.MapFailure)
		}
		if refused == 2 {
			for _, entry := range svc.OffMap {
				if entry.Reason != atlas.OffMapFailure {
					t.Fatalf("%s: %s", entry.File.Path, entry.Reason)
				}
			}
			rejected := 0
			for _, row := range result.Rejected {
				if row.Stage == lines.StageZones && row.Kind == "window_rejected" && row.Target == "svc" {
					rejected++
				}
			}
			if rejected != 1 {
				t.Fatalf("refusals: %+v", result.Rejected)
			}
		}
		if web := targetOf(t, result, "web"); web.MapFailure != "" || len(web.Boxes) == 0 {
			t.Fatalf("refused %d: web failure %q boxes %d", refused, web.MapFailure, len(web.Boxes))
		}
		if err := atlas.Validate(result.Atlas); err != nil {
			t.Fatal(err)
		}
	}
}
