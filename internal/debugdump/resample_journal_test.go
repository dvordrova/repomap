package debugdump

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

// drawProvider answers each call with its next response.
type drawProvider struct{ responses []string }

func (*drawProvider) State() []byte { return []byte(`{"model":"draws"}`) }
func (*drawProvider) Prepare(prompt llm.Prompt, _ llm.Limits) (llm.Prepared, error) {
	return llm.NewPrepared([]byte(prompt.User))
}
func (provider *drawProvider) Complete(context.Context, llm.Prepared) (llm.Completion, error) {
	response := provider.responses[0]
	provider.responses = provider.responses[1:]
	return llm.Completion{Response: []byte(response), FinishReason: llm.FinishStop, ChoiceCount: 1,
		Metrics: llm.Metrics{Attempts: 1, InputTokens: 10, OutputTokens: 4}}, nil
}

// A resampled request is two exchanges in the journal: the refused draw and
// the accepted one, with one request SHA and their own instance ordinals.
func TestResampledRequestJournalsBothDraws(t *testing.T) {
	writer, err := NewWriter(t.TempDir(), "resample")
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	executor := BindStage(llm.Executor{Observer: NewSemanticObserver(writer)}, SemanticStageAtlasZones)
	provider := &drawProvider{responses: []string{`{"groups":{}}`, `{"groups":{"Parts":"Draws the map."}}`}}
	call := llm.Call[map[string]any]{
		Resample: true,
		Prompt:   llm.Prompt{User: `{"task":"parts"}`, ResponseFormatJSON: true},
		Limits:   llm.Limits{MaxRequestBytes: 1 << 10, MaxResponseBytes: 1 << 10, MaxOutputTokens: 100},
		Validate: func(value map[string]any) error {
			if groups, _ := value["groups"].(map[string]any); len(groups) == 0 {
				return errors.New("no groups")
			}
			return nil
		},
	}
	if _, err := llm.ExecuteJSON(t.Context(), executor, provider, call); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(writer.BaseDir, writer.RunID)
	matches, err := filepath.Glob(filepath.Join(runDir, SemanticExchangesDir, "*", SemanticExchangeMetaFile))
	if err != nil || len(matches) != 2 {
		t.Fatalf("journal holds %d exchanges, want 2: %v", len(matches), err)
	}
	var records []SemanticExchangeRecord
	for _, match := range matches {
		raw, err := os.ReadFile(match)
		if err != nil {
			t.Fatal(err)
		}
		var record SemanticExchangeRecord
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].InstanceOrdinal < records[j].InstanceOrdinal })
	first, second := records[0], records[1]
	if first.RequestSHA256 != second.RequestSHA256 || first.InstanceOrdinal == second.InstanceOrdinal ||
		first.State != SemanticStateRejected || second.State != SemanticStateAccepted ||
		first.SemanticCalls != 1 || second.SemanticCalls != 1 {
		t.Fatalf("draws are not told apart: %+v / %+v", first, second)
	}
}
