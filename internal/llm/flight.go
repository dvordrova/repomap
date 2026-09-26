package llm

import (
	"context"
	"fmt"
)

// flight is one exact request a caller is answering. Every identical request
// that arrives before it lands waits for that one answer instead of asking the
// provider again: identical bytes asked at once would otherwise draw
// different answers, of which the cache keeps whichever landed last, so the
// next run would read another answer than this one did.
type flight struct {
	done chan struct{}
	// answer is what the provider returned for the request, or the accepted
	// record the leader read; nil when the leader stopped without one.
	answer *flightAnswer
}

// flightAnswer is the leader's raw answer. A follower reads it through its own
// limits, response adapter and decoder, as it would read a cache record.
type flightAnswer struct {
	completion Completion
	// err is the provider's failure, the one error a follower shares as it
	// is; an envelope or decoder refusal is decided again by each follower.
	err error
}

// joinFlight returns the flight answering key and whether this caller leads
// it. A caller that only recalls never leads: it waits for a flight in the
// air, then reads the cache. Without a controller nothing is shared.
func (controller *BatchController) joinFlight(key string, lead bool) (*flight, bool) {
	if controller == nil || key == "" {
		return nil, false
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if current := controller.flights[key]; current != nil {
		return current, false
	}
	if !lead {
		return nil, false
	}
	if controller.flights == nil {
		controller.flights = make(map[string]*flight)
	}
	current := &flight{done: make(chan struct{})}
	controller.flights[key] = current
	return current, true
}

// land ends the leader's flight. An answer is published even when the leader
// then refused it; a leader whose own context ended its call leaves no answer,
// and its followers ask again.
func (controller *BatchController) land(key string, current *flight, ctx context.Context, err error) {
	controller.mu.Lock()
	delete(controller.flights, key)
	controller.mu.Unlock()
	if err != nil && ctx.Err() != nil {
		current.answer = nil
	}
	close(current.done)
}

func (current *flight) wait(ctx context.Context) (*flightAnswer, error) {
	select {
	case <-current.done:
		return current.answer, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// followFlight gives a follower the leader's answer, validated by the
// follower's own limits and decoder. It made no provider call, so its outcome
// is Cached like a cache hit and its events carry the cache source.
func followFlight[T any](
	executor Executor,
	answer flightAnswer,
	decodeValidate DecodeValidate[T],
	limits Limits,
	outcome Outcome[T],
	adapted *AdaptedResponse,
) (Outcome[T], error) {
	completion := answer.completion
	outcome.HTTPResponse = completion.HTTPResponse.Clone()
	setOutcomeResponse(&outcome, completion.Response)
	outcome.FinishReason = completion.FinishReason
	outcome.ChoiceCount = completion.ChoiceCount
	outcome.Metrics = completion.Metrics
	outcome.Cached = true
	if answer.err != nil {
		outcome.ResponseRejections = []ResponseRejection{{Kind: "provider_failed", Count: 1, Reason: answer.err.Error()}}
		return outcome, answer.err
	}
	if err := validateLiveCompletion(completion, limits); err != nil {
		outcome.ResponseRejections = []ResponseRejection{{Kind: "response_envelope", Count: 1, Reason: err.Error()}}
		return outcome, err
	}
	value, err := decodeAcceptedJSON(decodeValidate, completion.Response)
	if err != nil {
		outcome.ResponseRejections = []ResponseRejection{{Kind: "response_validation", Count: 1, Reason: err.Error()}}
		outcome.Issues = observe(executor.Observer, eventForOutcome(
			EventFailure, SourceCache, FailureValidation, outcome,
		), outcome.Issues)
		return outcome, fmt.Errorf("llm: reject response: %w", err)
	}
	outcome.Value = value
	outcome.ResponseRejections = acceptedResponseRejections(value, adapted.Rejections)
	outcome.Issues = observe(executor.Observer, eventForOutcome(
		EventCacheHit, SourceCache, FailureNone, outcome,
	), outcome.Issues)
	return outcome, nil
}
