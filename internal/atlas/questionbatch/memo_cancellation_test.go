package questionbatch

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dvordrova/repomap/internal/llm"
)

type cancelingMemoProvider struct {
	*testProvider
	cancel   context.CancelFunc
	point    string
	accepted int
	unwraps  int
}

func (provider *cancelingMemoProvider) Prepare(prompt llm.Prompt, limits llm.Limits) (llm.Prepared, error) {
	prepared, err := provider.testProvider.Prepare(prompt, limits)
	var request modelRequest
	if json.Unmarshal([]byte(prompt.User), &request) == nil &&
		(provider.point == "identity" && len(request.Evidence) == 0 || provider.point == "prepared window" && len(request.Evidence) > 0) {
		provider.cancel()
	}
	return prepared, err
}

func (provider *cancelingMemoProvider) AdaptResponse(_, response []byte) (llm.AdaptedResponse, error) {
	provider.unwraps++
	if provider.point == "cached decoder" {
		provider.cancel()
	}
	return llm.AdaptedResponse{Domain: response, Accept: func([]string) {
		provider.accepted++
		if provider.point == "accepted callback" {
			provider.cancel()
		}
	}}, nil
}

func TestFullyWarmQuestionMemoHonorsCancellationWithoutNewCompletion(t *testing.T) {
	for _, point := range []string{"identity", "prepared window", "cached decoder", "accepted callback", "before remember"} {
		t.Run(point, func(t *testing.T) {
			input := testInput(3, 2)
			base := &testProvider{}
			executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
			if _, err := Run(t.Context(), executor, base, input, Options{}); err != nil {
				t.Fatal(err)
			}
			if len(base.requests) != 1 {
				t.Fatal("cold questions were not stored in one shared response")
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			provider := &cancelingMemoProvider{testProvider: base, cancel: cancel, point: point}
			if point == "before remember" {
				executor.PlanNotice = func(count int) {
					if count != 0 {
						t.Errorf("fully warm reading planned %d completions", count)
					}
					cancel()
				}
			}
			result, err := Run(ctx, executor, provider, input, Options{})
			if !errors.Is(err, context.Canceled) || len(base.requests) != 1 || len(result.Questions) != 0 || len(result.Exchanges) != 0 {
				t.Fatalf("canceled warm reading returned success/authority or called provider: calls=%d result=%+v err=%v", len(base.requests), result, err)
			}
			if point != "accepted callback" && point != "before remember" && provider.accepted != 0 {
				t.Fatal("canceled cached response supplied accepted adjunct metadata")
			}
			if point == "cached decoder" && provider.unwraps != 1 {
				t.Fatalf("canceled decoder was repeated: %d", provider.unwraps)
			}
		})
	}
}

func TestWarmQuestionsParseSharedAdjunctOnlyOnce(t *testing.T) {
	base := &testProvider{}
	input := testInput(5, 3)
	executor := llm.Executor{Enabled: true, RootDir: t.TempDir()}
	if _, err := Run(t.Context(), executor, base, input, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(base.requests) != 1 {
		t.Fatal("fixture did not form one shared response")
	}
	adapter := &cancelingMemoProvider{testProvider: base, cancel: func() {}}
	result, err := Run(t.Context(), executor, adapter, input, Options{})
	if err != nil || len(result.Questions) != len(input.Questions) || len(base.requests) != 1 {
		t.Fatalf("warm result lost or called provider: %+v / %v", result, err)
	}
	if adapter.unwraps != 1 || adapter.accepted != 1 {
		t.Fatalf("one shared adjunct repeatedly parsed/collected: %d / %d", adapter.unwraps, adapter.accepted)
	}
}
