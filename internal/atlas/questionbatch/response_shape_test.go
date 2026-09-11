package questionbatch

import (
	"encoding/json"
	"strings"
	"testing"
)

// A missing question's reason describes the entries that named no asked
// question, so the journal shows the provider's shape.
func TestResponseShapeDescribesUnmatchedEntries(t *testing.T) {
	shape := responseShape([]json.RawMessage{json.RawMessage(`{"question_ref":"q1","rows":[]}`), json.RawMessage(`{"key":"q9"}`)})
	if !strings.Contains(shape, "2 unmatched") || !strings.Contains(shape, "question_ref") || !strings.Contains(shape, "key=q9") {
		t.Fatalf("shape does not describe the provider's fields: %q", shape)
	}
	if responseShape(nil) != "" {
		t.Fatal("no unmatched entries produced a shape")
	}
	if _, err := questionEntriesOf([]byte(`{"results":[]}`)); err == nil {
		t.Fatal("a response without questions was accepted")
	}
}
