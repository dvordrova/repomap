package llm

import (
	"reflect"
	"testing"
)

type acceptedTestRows []string

func (rows acceptedTestRows) AcceptedRowKeys() []string { return []string(rows) }

type rowAcceptanceAdapter struct {
	*testProvider
	accepted [][]string
}

func (adapter *rowAcceptanceAdapter) AdaptResponse(_, _, response []byte) (AdaptedResponse, error) {
	return AdaptedResponse{Domain: response, Accept: func(rows []string) {
		adapter.accepted = append(adapter.accepted, rows)
	}}, nil
}

func TestResponseMetadataUsesAcceptedRowKeysOnLiveAndCache(t *testing.T) {
	for _, rows := range []acceptedTestRows{nil, {}, {"r1", "r3"}} {
		provider := &rowAcceptanceAdapter{testProvider: baseTestProvider()}
		base := baseTestCall("accepted-rows", "input")
		call := Call[acceptedTestRows]{State: base.State, Prompt: base.Prompt, Limits: base.Limits,
			DecodeValidate: func([]byte) (acceptedTestRows, error) { return rows, nil },
		}
		executor := Executor{Enabled: true, RootDir: t.TempDir()}
		for i := 0; i < 2; i++ {
			outcome, err := ExecuteJSON(t.Context(), executor, provider, call)
			if err != nil || outcome.Cached != (i == 1) {
				t.Fatalf("execute: cached=%t error=%v", outcome.Cached, err)
			}
		}
		if !reflect.DeepEqual(provider.accepted, [][]string{[]string(rows), []string(rows)}) {
			t.Fatalf("metadata acceptance lost nil/empty/selected distinction: %#v, expected %#v", provider.accepted, rows)
		}
	}
}
