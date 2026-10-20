package llm

import (
	"errors"
)

func adaptiveFailureMemo[Value any](call Call[Value], outcome Outcome[Value], err error) (adaptiveSplitMemo, bool) {
	memo := adaptiveSplitMemo{Version: 1}
	var resourceErr *ResourceLimitError
	providerErr, providerFailure := err.(*ProviderError)
	if providerFailure && providerErr.Operation == "complete" && errors.As(err, &resourceErr) && adaptiveSplitKind(resourceErr.Kind) {
		memo.Kind = resourceErr.Kind
	} else if len(outcome.Request) > 0 && errors.As(err, &resourceErr) &&
		(resourceErr.Kind == ResourceLimitOutputTokens || resourceErr.Kind == ResourceLimitResponseBytes) &&
		hasResponseEnvelopeRefusal(outcome.ResponseRejections) {
		// A provider-neutral completion can pass transport, then exceed the
		// executor's output envelope. This is still a completed parent that the
		// owner can divide; preparation/configuration failures have no such proof.
		memo.Kind = resourceErr.Kind
	} else if providerFailure && providerErr.Operation == "complete" && call.SplitHTTP500 &&
		providerErr.ProviderFailure().Kind == ProviderFailureHTTPStatus && providerErr.ProviderFailure().HTTPStatus == 500 {
		memo.RejectionReason = adaptiveHTTP500
	} else if call.SplitRejectedResponse && rejectedAdaptiveResponse(outcome) {
		memo.RejectionReason = adaptiveResponseRejected
	} else {
		return memo, false
	}
	return memo, true
}

func hasResponseEnvelopeRefusal(rejections []ResponseRejection) bool {
	for _, rejection := range rejections {
		if rejection.Kind == "response_envelope" {
			return true
		}
	}
	return false
}

func rejectedAdaptiveResponse[Value any](outcome Outcome[Value]) bool {
	// This reason is set by ExecuteJSON only after the provider envelope passed
	// and the original response failed JSON/domain validation. Metadata-only,
	// preparation, transport and cancellation failures never acquire it.
	for _, rejection := range outcome.ResponseRejections {
		if rejection.Kind == adaptiveResponseRejected {
			return true
		}
	}
	return false
}

// RequestBytes proves Prepare completed and executeLive was entered. A
// canceled or queued zero slot is not a live provider call, while a
// provider resource failure may legitimately report zero Attempts.
