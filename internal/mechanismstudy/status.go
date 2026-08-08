package mechanismstudy

import (
	"fmt"
	"reflect"
)

type StatusState string

const (
	StatusComplete StatusState = "complete"
	StatusPartial  StatusState = "partial"
	StatusFailed   StatusState = "failed"
)

type BatchExecutionState string

const (
	BatchAccepted            BatchExecutionState = "accepted"
	BatchResponseInvalid     BatchExecutionState = "response_invalid"
	BatchOutputLimit         BatchExecutionState = "output_limit"
	BatchProviderFailed      BatchExecutionState = "provider_failed"
	BatchCanceled            BatchExecutionState = "canceled"
	BatchConfigurationFailed BatchExecutionState = "configuration_failed"
)

func (state BatchExecutionState) valid() bool {
	switch state {
	case BatchAccepted, BatchResponseInvalid, BatchOutputLimit, BatchProviderFailed,
		BatchCanceled, BatchConfigurationFailed:
		return true
	default:
		return false
	}
}

// BatchExecution contains only closed accounting for one attempted batch.
// ProviderCalls counts semantic calls (zero or one); TransportAttempts may be
// greater only when the shared transport replayed identical request bytes.
type BatchExecution struct {
	RequestRef        string              `json:"request_ref"`
	RequestSHA256     string              `json:"request_sha256"`
	State             BatchExecutionState `json:"state"`
	ProviderCalls     int                 `json:"provider_calls"`
	TransportAttempts int                 `json:"transport_attempts"`
}

func (execution BatchExecution) validAccounting() bool {
	switch execution.State {
	case BatchAccepted, BatchResponseInvalid, BatchOutputLimit, BatchProviderFailed:
		return execution.ProviderCalls == 1 &&
			execution.TransportAttempts >= 1 && execution.TransportAttempts <= 8
	case BatchConfigurationFailed:
		return execution.ProviderCalls == 0 && execution.TransportAttempts == 0
	case BatchCanceled:
		return (execution.ProviderCalls == 0 && execution.TransportAttempts == 0) ||
			(execution.ProviderCalls == 1 &&
				execution.TransportAttempts >= 0 && execution.TransportAttempts <= 8)
	default:
		return false
	}
}

type StatusExecution struct {
	Batches []BatchExecution
}

// Status is the closed, prose-free last record in the artifact family. Every
// count and digest except per-batch transport accounting is rederived from the
// other three canonical artifacts.
type Status struct {
	Version                int              `json:"version"`
	State                  StatusState      `json:"state"`
	PromptVersion          string           `json:"prompt_version"`
	FactsSHA256            string           `json:"facts_sha256"`
	CandidatesSHA256       string           `json:"candidates_sha256"`
	ResultSHA256           string           `json:"result_sha256"`
	CatalogRef             string           `json:"catalog_ref"`
	CatalogSHA256          string           `json:"catalog_sha256"`
	CardCount              int              `json:"card_count"`
	MechanismCardCount     int              `json:"mechanism_card_count"`
	PreparedCardCount      int              `json:"prepared_card_count"`
	MechanismCount         int              `json:"mechanism_count"`
	CandidateCount         int              `json:"candidate_count"`
	RejectedCandidateCount int              `json:"rejected_candidate_count"`
	PlannedBatchCount      int              `json:"planned_batch_count"`
	AttemptedBatchCount    int              `json:"attempted_batch_count"`
	AcceptedBatchCount     int              `json:"accepted_batch_count"`
	FailedBatchCount       int              `json:"failed_batch_count"`
	UnattemptedBatchCount  int              `json:"unattempted_batch_count"`
	FailedCardCount        int              `json:"failed_card_count"`
	UnrequestedCardCount   int              `json:"unrequested_card_count"`
	ProviderCallCount      int              `json:"provider_call_count"`
	TransportAttemptCount  int              `json:"transport_attempt_count"`
	Batches                []BatchExecution `json:"batches"`
}

func BuildStatus(
	factsData, candidatesData, resultData []byte,
	execution StatusExecution,
) (Status, error) {
	facts, err := DecodeFacts(factsData)
	if err != nil {
		return Status{}, err
	}
	candidates, err := DecodeCandidates(factsData, candidatesData)
	if err != nil {
		return Status{}, err
	}
	result, err := DecodeResult(factsData, candidatesData, resultData)
	if err != nil {
		return Status{}, err
	}
	status := Status{
		Version:              ArtifactVersion,
		PromptVersion:        PromptVersion,
		FactsSHA256:          facts.SHA256,
		CandidatesSHA256:     candidates.SHA256,
		ResultSHA256:         result.SHA256,
		CatalogRef:           facts.Compilation.CatalogRef,
		CatalogSHA256:        facts.Compilation.CatalogSHA256,
		CardCount:            len(result.Cards),
		PlannedBatchCount:    len(facts.Plan.Batches),
		UnrequestedCardCount: len(facts.Plan.UnrequestedCardRefs),
		Batches:              append([]BatchExecution{}, execution.Batches...),
	}
	for _, batch := range candidates.Batches {
		for _, card := range batch.Response.Cards {
			status.CandidateCount += len(card.Mechanisms)
		}
	}
	for _, card := range result.Cards {
		switch card.State {
		case OutcomeMechanism:
			status.MechanismCardCount++
			status.MechanismCount += len(card.Mechanisms)
		case OutcomePrepared:
			status.PreparedCardCount++
		default:
			return Status{}, fmt.Errorf("mechanism study status artifact: invalid result outcome")
		}
		for _, frontier := range card.Frontier {
			if frontier.Reason == FrontierResponseInvalid {
				status.RejectedCandidateCount += frontier.Count
			}
		}
	}
	if status.MechanismCardCount+status.PreparedCardCount != status.CardCount {
		return Status{}, fmt.Errorf("mechanism study status artifact: result counts do not close")
	}
	if err := deriveExecutionStatus(&status, facts, candidates); err != nil {
		return Status{}, err
	}
	return status, nil
}

func deriveExecutionStatus(status *Status, facts RestoredFacts, candidates RestoredCandidates) error {
	if len(status.Batches) > len(facts.Plan.Batches) {
		return fmt.Errorf("mechanism study status artifact: too many batch executions")
	}
	candidateByRef := make(map[string]struct{}, len(candidates.Batches))
	for _, candidate := range candidates.Batches {
		candidateByRef[candidate.RequestRef] = struct{}{}
	}
	failureSeen := false
	for position, execution := range status.Batches {
		batch := facts.Plan.Batches[position]
		if execution.RequestRef != batch.Request.RequestRef || execution.RequestSHA256 != batch.WireSHA256 ||
			!execution.State.valid() || !execution.validAccounting() || failureSeen {
			return fmt.Errorf("mechanism study status artifact: invalid canonical batch execution")
		}
		status.ProviderCallCount += execution.ProviderCalls
		status.TransportAttemptCount += execution.TransportAttempts
		_, hasCandidates := candidateByRef[execution.RequestRef]
		if execution.State == BatchAccepted {
			if !hasCandidates {
				return fmt.Errorf("mechanism study status artifact: accepted batch has no candidates")
			}
			delete(candidateByRef, execution.RequestRef)
			status.AcceptedBatchCount++
			continue
		}
		if hasCandidates {
			return fmt.Errorf("mechanism study status artifact: failed batch retained candidates")
		}
		failureSeen = true
		status.FailedBatchCount++
		status.FailedCardCount += len(batch.Request.Cards)
	}
	if len(candidateByRef) != 0 {
		return fmt.Errorf("mechanism study status artifact: candidates lack accepted execution")
	}
	status.AttemptedBatchCount = len(status.Batches)
	status.UnattemptedBatchCount = status.PlannedBatchCount - status.AttemptedBatchCount
	if status.ProviderCallCount > MaxProviderCalls || status.FailedBatchCount > 1 {
		return fmt.Errorf("mechanism study status artifact: execution bounds exceeded")
	}
	switch {
	case status.PlannedBatchCount == 0:
		if status.AttemptedBatchCount != 0 {
			return fmt.Errorf("mechanism study status artifact: zero-call plan was executed")
		}
		status.State = StatusComplete
	case status.FailedBatchCount == 0 && status.AttemptedBatchCount == status.PlannedBatchCount:
		if status.UnrequestedCardCount > 0 {
			status.State = StatusPartial
		} else {
			status.State = StatusComplete
		}
	case status.FailedBatchCount == 1 && status.AcceptedBatchCount > 0:
		status.State = StatusPartial
	case status.FailedBatchCount == 1:
		status.State = StatusFailed
	default:
		return fmt.Errorf("mechanism study status artifact: execution does not close planned prefix")
	}
	return nil
}

func EncodeStatus(
	factsData, candidatesData, resultData []byte,
	execution StatusExecution,
) ([]byte, error) {
	status, err := BuildStatus(factsData, candidatesData, resultData, execution)
	if err != nil {
		return nil, err
	}
	return encodeCanonicalArtifact("mechanism study status", MaxStatusArtifactBytes, status)
}

func DecodeStatus(factsData, candidatesData, resultData, data []byte) (Status, error) {
	var artifact Status
	if err := decodeCanonicalArtifact("mechanism study status", data, MaxStatusArtifactBytes, &artifact); err != nil {
		return Status{}, err
	}
	want, err := BuildStatus(
		factsData, candidatesData, resultData,
		StatusExecution{Batches: append([]BatchExecution{}, artifact.Batches...)},
	)
	if err != nil {
		return Status{}, err
	}
	if !statusesEqual(artifact, want) {
		return Status{}, fmt.Errorf("mechanism study status artifact: derived counts or bindings mismatch")
	}
	return artifact, nil
}

func statusesEqual(first, second Status) bool {
	return reflect.DeepEqual(first, second)
}
