package debugdump

import (
	"sync"

	"github.com/dvordrova/repomap/internal/llm"
)

// SemanticObserver adapts provider-neutral LLM events into the diagnostic
// semantic exchange journal. Recording is best-effort and never participates
// in the semantic result returned by a domain cube.
type SemanticObserver struct {
	writer *Writer

	mu              sync.Mutex
	ordinals        map[string]int
	instanceOrdinal int
	pending         []SemanticExchange
}

// SemanticOrdinalScaleWarning reports a former journal ordinal ceiling that
// a complete ordinary observer crossed. The warning is aggregate: callers do
// not need to print one line for every retained exchange.
type SemanticOrdinalScaleWarning struct {
	Kind         string
	Retained     int
	AdvisorySize int
}

const (
	SemanticScaleWarningAttemptOrdinal  = "semantic_attempt_ordinal"
	SemanticScaleWarningInstanceOrdinal = "semantic_exchange_instance_ordinal"
)

func NewSemanticObserver(writer *Writer) *SemanticObserver {
	return &SemanticObserver{writer: writer, ordinals: make(map[string]int)}
}

// Observe satisfies llm.Observer. A stage owner must bind the observer before
// a call; an event without that request-local owner is deliberately omitted.
func (observer *SemanticObserver) Observe(llm.Event) error { return nil }

// ObserveStage records one event under its already-resolved diagnostic stage.
func (observer *SemanticObserver) ObserveStage(stage string, event llm.Event) error {
	if observer == nil {
		return nil
	}
	observer.mu.Lock()
	ordinal := observer.ordinals[stage] + 1
	instanceOrdinal := observer.instanceOrdinal + 1
	exchange, record := semanticExchangeForStageEventAt(
		stage,
		instanceOrdinal,
		ordinal,
		event,
	)
	if record {
		observer.ordinals[stage] = ordinal
		observer.instanceOrdinal = instanceOrdinal
	}
	writer := observer.writer
	if record && writer == nil {
		observer.pending = append(observer.pending, exchange)
	}
	observer.mu.Unlock()
	if record && writer != nil {
		writer.RecordSemanticExchange(exchange)
	}
	return nil
}

// OrdinalScaleWarnings returns at most one warning for each former journal
// ordinal ceiling. It snapshots only exchanges the observer actually retained;
// redacted or otherwise unrecordable events do not inflate the measurement.
func (observer *SemanticObserver) OrdinalScaleWarnings() []SemanticOrdinalScaleWarning {
	if observer == nil {
		return nil
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	maximumAttemptOrdinal := 0
	for _, ordinal := range observer.ordinals {
		if ordinal > maximumAttemptOrdinal {
			maximumAttemptOrdinal = ordinal
		}
	}
	warnings := make([]SemanticOrdinalScaleWarning, 0, 2)
	if maximumAttemptOrdinal > MaxSemanticAttemptOrdinal {
		warnings = append(warnings, SemanticOrdinalScaleWarning{
			Kind: SemanticScaleWarningAttemptOrdinal, Retained: maximumAttemptOrdinal,
			AdvisorySize: MaxSemanticAttemptOrdinal,
		})
	}
	if observer.instanceOrdinal > MaxSemanticExchangeInstanceOrdinal {
		warnings = append(warnings, SemanticOrdinalScaleWarning{
			Kind: SemanticScaleWarningInstanceOrdinal, Retained: observer.instanceOrdinal,
			AdvisorySize: MaxSemanticExchangeInstanceOrdinal,
		})
	}
	return warnings
}

// Flush persists events buffered before the ordinary run directory existed.
// Selection executes inside snapshot construction, so journaling must not
// invent a second run directory or race the authoritative run writer.
func (observer *SemanticObserver) Flush(writer *Writer) {
	if observer == nil || writer == nil {
		return
	}
	observer.mu.Lock()
	pending := append([]SemanticExchange(nil), observer.pending...)
	observer.pending = nil
	observer.mu.Unlock()
	for _, exchange := range pending {
		writer.RecordSemanticExchange(exchange)
	}
}

// BindStage returns a request-local executor whose observer owns the supplied
// diagnostic stage. Observers without the stage-aware seam are left intact.
func BindStage(executor llm.Executor, stage string) llm.Executor {
	observer, ok := executor.Observer.(interface {
		ObserveStage(string, llm.Event) error
	})
	if !ok || observer == nil {
		return executor
	}
	executor.Observer = llm.ObserverFunc(func(event llm.Event) error {
		return observer.ObserveStage(stage, event)
	})
	return executor
}

func semanticExchangeForStageEvent(
	stage string,
	ordinal int,
	event llm.Event,
) (SemanticExchange, bool) {
	return semanticExchangeForStageEventAt(stage, 1, ordinal, event)
}

func semanticExchangeForStageEventAt(
	stage string,
	instanceOrdinal int,
	semanticAttemptOrdinal int,
	event llm.Event,
) (SemanticExchange, bool) {
	if stage == "" || instanceOrdinal < 1 || semanticAttemptOrdinal < 1 ||
		event.RequestRedacted || len(event.Request) == 0 {
		return SemanticExchange{}, false
	}
	exchange := SemanticExchange{
		Stage: stage, InstanceOrdinal: instanceOrdinal,
		SemanticAttemptOrdinal: semanticAttemptOrdinal,
		Request:                event.Request, Response: event.Response,
	}
	if event.Source == llm.SourceLive {
		exchange.RequestProvenance = SemanticRequestExactSent
		exchange.SemanticCalls = 1
		exchange.TransportAttempts = event.Metrics.Attempts
		if exchange.TransportAttempts < 0 || exchange.TransportAttempts > MaxSemanticTransportAttempts {
			return SemanticExchange{}, false
		}
	} else if event.Source == llm.SourceCache {
		exchange.RequestProvenance = SemanticRequestPrepared
	} else {
		return SemanticExchange{}, false
	}

	switch event.Kind {
	case llm.EventLive:
		if event.Source != llm.SourceLive || event.Failure != llm.FailureNone {
			return SemanticExchange{}, false
		}
		exchange.State = SemanticStateAccepted
		exchange.ValidationCode = SemanticValidationAccepted
	case llm.EventCacheHit:
		if event.Source != llm.SourceCache || event.Failure != llm.FailureNone {
			return SemanticExchange{}, false
		}
		exchange.State = SemanticStateCacheHit
		exchange.ValidationCode = SemanticValidationCache
	case llm.EventFailure:
		if !classifySemanticFailure(&exchange, event) {
			return SemanticExchange{}, false
		}
	default:
		return SemanticExchange{}, false
	}
	if len(exchange.Response) == 0 {
		code := SemanticUnavailableNoContent
		if event.ResponseRedacted {
			code = SemanticUnavailableOmitted
		} else if event.Source == llm.SourceCache {
			code = SemanticUnavailableCache
		}
		exchange.ResponseUnavailable = &SemanticUnavailable{
			Code: code, OriginalSHA256: event.ResponseSHA256, OriginalBytes: event.ResponseBytes,
		}
	}
	return exchange, true
}

func classifySemanticFailure(exchange *SemanticExchange, event llm.Event) bool {
	if exchange == nil {
		return false
	}
	exchange.State = SemanticStateRejected
	switch event.Failure {
	case llm.FailureProvider:
		if event.Source != llm.SourceLive {
			return false
		}
		exchange.State = SemanticStateProviderFailed
		exchange.ValidationCode = SemanticValidationProvider
	case llm.FailureResponse:
		if event.Source != llm.SourceLive {
			return false
		}
		if event.ResponseRedacted {
			exchange.ValidationCode = SemanticValidationSecret
		} else {
			exchange.ValidationCode = SemanticValidationResponse
		}
	case llm.FailureValidation:
		if event.ResponseRedacted {
			exchange.ValidationCode = SemanticValidationSecret
		} else if _, err := llm.NormalizeJSON(event.Response); err != nil {
			exchange.ValidationCode = SemanticValidationDecode
		} else {
			exchange.ValidationCode = SemanticValidationResponse
		}
	default:
		// Preparation and operational cache failures do not own a complete
		// semantic exchange. A later live attempt is observed independently.
		return false
	}
	return true
}
