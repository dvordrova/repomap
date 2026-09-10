package reading

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dvordrova/repomap/internal/atlas"
	"github.com/dvordrova/repomap/internal/atlas/lines"
)

func TestAnswerScopesAreSharedExactlyWithoutBroadeningPartialParts(t *testing.T) {
	routes := []atlas.QuestionRoute{{Question: "First?", Scope: []string{"Original complete scope.", "No runtime observations."}}, {Question: "Second?", Scope: []string{"Original complete scope.", "No runtime observations."}}, {Question: "Partial?", Scope: []string{"Only selected sources inspected."}}}
	window, err := makeAnswerWindow(lines.Answer(), routes, []answerQuestion{{index: 0, complete: true}, {index: 1, complete: true}, {index: 2, complete: false}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(window.table.Request), "Original complete scope.") != 1 {
		t.Fatal("identical scope repeated")
	}
	var request struct {
		Context struct{ Scopes map[string][]string }
		Rows    []struct {
			Question         string
			ScopeRef         string `json:"scope_ref"`
			EvidenceComplete bool   `json:"evidence_complete"`
		}
	}
	if err := json.Unmarshal(window.table.Request, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.Context.Scopes) != 2 {
		t.Fatal("distinct scopes merged")
	}
	for i, row := range request.Rows {
		if !reflect.DeepEqual(request.Context.Scopes[row.ScopeRef], routes[i].Scope) || row.EvidenceComplete != (i < 2) {
			t.Fatal("scope or source-part completeness changed")
		}
	}
}

func TestLearnOwnsItsSmallerOutputAllowance(t *testing.T) {
	call, err := learningCall(learningRequest{}, learningPrompt)
	if err != nil || call.Limits.MaxOutputTokens != 16000 {
		t.Fatalf("learn output planning: %+v %v", call.Limits, err)
	}
}
