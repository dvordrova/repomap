package debugdump

import (
	"encoding/json"
	"path/filepath"
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
	pending         []observedResponse
	failureNotice   func(SemanticFailureReceipt)
}

// SemanticFailureReceipt points at one committed diagnostic exchange. An
// unavailable response is identified explicitly; its marker is not a raw body.
type SemanticFailureReceipt struct {
	Stage, Reason                          string
	JournalPath, RequestPath, ResponsePath string
	ResponseUnavailable                    string
	HTTPResponse                           *llm.HTTPResponse
	TransportAttempts                      int
	LatencyMS                              int64
}

type observedResponse struct {
	exchange   SemanticExchange
	rejections []llm.ResponseRejection
	failed     bool
	reason     string
}

// Rejections use the existing rejected.jsonl shape. ResponseRef names the
// ordinary exchange record, which links both exact request and response bytes.
func recordObservedResponse(writer *Writer, value observedResponse, notice func(SemanticFailureReceipt)) error {
	ref, record := writer.recordSemanticExchange(value.exchange)
	if value.failed && record != nil && notice != nil {
		journal, err := filepath.Abs(filepath.Join(writer.runDir, filepath.FromSlash(ref)))
		if err == nil {
			receipt := SemanticFailureReceipt{
				Stage: value.exchange.Stage, Reason: value.reason, JournalPath: journal,
				RequestPath:         filepath.Join(filepath.Dir(journal), filepath.FromSlash(record.Request.File)),
				ResponseUnavailable: record.Response.UnavailableCode,
				HTTPResponse:        record.HTTPResponse,
				TransportAttempts:   record.TransportAttempts,
				LatencyMS:           record.LatencyMS,
			}
			if receipt.ResponseUnavailable == "" {
				receipt.ResponsePath = filepath.Join(filepath.Dir(journal), filepath.FromSlash(record.Response.File))
			}
			notice(receipt)
		}
	}
	if len(value.rejections) == 0 {
		return nil
	}
	var rows []byte
	for _, rejection := range value.rejections {
		raw, err := json.Marshal(struct {
			Stage string `json:"stage"`
			llm.ResponseRejection
			ResponseRef string `json:"response_ref,omitempty"`
		}{value.exchange.Stage, rejection, ref})
		if err != nil {
			return err
		}
		rows = append(rows, raw...)
		rows = append(rows, '\n')
	}
	return writer.AppendFile("rejected.jsonl", rows)
}

func NewSemanticObserver(writer *Writer) *SemanticObserver {
	return &SemanticObserver{writer: writer, ordinals: make(map[string]int)}
}

// SetFailureNotice installs a console-only notification after journal commit.
// It cannot change acceptance, cache state or the result of recording.
func (observer *SemanticObserver) SetFailureNotice(notice func(SemanticFailureReceipt)) {
	if observer == nil {
		return
	}
	observer.mu.Lock()
	defer observer.mu.Unlock()
	observer.failureNotice = notice
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
	notice := observer.failureNotice
	value := observedResponse{exchange: exchange, rejections: event.ResponseRejections, failed: event.Kind == llm.EventFailure, reason: string(event.Failure)}
	if len(event.ResponseRejections) > 0 {
		value.reason = event.ResponseRejections[0].Reason
	}
	if record && writer == nil {
		observer.pending = append(observer.pending, value)
	}
	observer.mu.Unlock()
	if record && writer != nil {
		return recordObservedResponse(writer, value, notice)
	}
	return nil
}

// Flush persists events buffered before the ordinary run directory existed.
// Selection executes inside snapshot construction, so journaling must not
// invent a second run directory or race the authoritative run writer.
func (observer *SemanticObserver) Flush(writer *Writer) {
	if observer == nil || writer == nil {
		return
	}
	observer.mu.Lock()
	pending := append([]observedResponse(nil), observer.pending...)
	observer.pending = nil
	notice := observer.failureNotice
	observer.mu.Unlock()
	for _, value := range pending {
		if err := recordObservedResponse(writer, value, notice); err != nil {
			writer.warnSemanticExchange(value.exchange.Stage)
		}
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
		event.CacheRoot = executor.RootDir
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
		len(event.Request) == 0 {
		return SemanticExchange{}, false
	}
	exchange := SemanticExchange{
		CacheRoot: event.CacheRoot,
		Stage:     stage, InstanceOrdinal: instanceOrdinal,
		SemanticAttemptOrdinal: semanticAttemptOrdinal,
		Request:                event.Request, Response: event.Response,
		HTTPResponse: event.HTTPResponse.Clone(),
		Latency:      event.Metrics.Latency,
		InputTokens:  event.Metrics.InputTokens,
		OutputTokens: event.Metrics.OutputTokens,
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
		if event.Source == llm.SourceCache {
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
		exchange.ValidationCode = SemanticValidationResponse
	case llm.FailureValidation:
		if _, err := llm.NormalizeJSON(event.Response); err != nil {
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
