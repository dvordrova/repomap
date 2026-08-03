package atlasstudy

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestReplayResponseRecordRecoversExactSavedD208Response(t *testing.T) {
	requestRaw, err := os.ReadFile("testdata/casdoor_20260803_075743_request_v5.json")
	if err != nil {
		t.Fatalf("read request fixture: %v", err)
	}
	request, err := DecodeRequestRecord(requestRaw)
	if err != nil {
		t.Fatalf("decode request fixture: %v", err)
	}
	responseRaw, err := os.ReadFile("testdata/casdoor_20260803_075743_response.json")
	if err != nil {
		t.Fatalf("read response fixture: %v", err)
	}

	result, status, diagnostics, err := ReplayResponseRecord(request, responseRaw)
	if err != nil {
		t.Fatalf("ReplayResponseRecord: %v", err)
	}
	if result.Version != ResultVersion || result.State != ProductStateAccepted {
		t.Fatalf("result identity = version %d state %q", result.Version, result.State)
	}
	if status.Version != ResultVersion || status.State != ProductStateAccepted || status.DirectionCount != 5 {
		t.Fatalf("status = %+v", status)
	}
	if diagnostics.DirectionsReceived != 5 || diagnostics.DirectionsAccepted != 5 ||
		diagnostics.DirectionsRejected != 0 {
		t.Fatalf("direction diagnostics = %+v", diagnostics)
	}
	readingCounts := make([]int, len(result.Directions))
	questions := make([]string, len(result.Directions))
	for index, direction := range result.Directions {
		readingCounts[index] = len(direction.Reading)
		questions[index] = direction.Question
	}
	if !slices.Equal(readingCounts, []int{1, 3, 4, 1, 1}) {
		t.Fatalf("reading cardinalities = %v", readingCounts)
	}
	for _, concise := range []string{
		"Как обрабатываются ECC-ключи?",
		"Как запускается прокси-сервис?",
	} {
		if !slices.Contains(questions, concise) {
			t.Fatalf("missing recovered concise question %q in %v", concise, questions)
		}
	}
}

func TestReplayResponseRecordRejectsRequestMaterialMismatch(t *testing.T) {
	requestRaw, err := os.ReadFile("testdata/casdoor_20260803_075743_request_v5.json")
	if err != nil {
		t.Fatal(err)
	}
	request, err := DecodeRequestRecord(requestRaw)
	if err != nil {
		t.Fatal(err)
	}
	request.CatalogSHA256 = strings.Repeat("0", 64)
	request.CatalogRef = "atlas-study-v5-" + request.CatalogSHA256

	_, _, _, err = ReplayResponseRecord(request, []byte(`{}`))
	if err == nil || !strings.Contains(err.Error(), "catalog hash mismatch") {
		t.Fatalf("mismatched request error = %v", err)
	}
}
