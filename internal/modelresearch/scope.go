// Package modelresearch contains the bounded contracts shared by adaptive
// provider research stages. It does not grant repository access: callers must
// assemble every local scope from already authorized deterministic facts.
package modelresearch

import "github.com/dvordrova/repomap/internal/gofacts"

// LocalRepositoryUniverse is the complete deterministic local authority for a
// run. It is never serialized into a provider request.
type LocalRepositoryUniverse struct {
	AuthorizedPaths []string
	CommandTraces   []gofacts.CommandTrace
	ScenarioID      string
}

// ProviderVisibleBundle identifies the exact bounded material exposed by one
// provider request. AllowedPaths has provider-only meaning.
type ProviderVisibleBundle struct {
	Stage        string   `json:"stage"`
	AllowedPaths []string `json:"provider_allowed_paths"`
	EvidenceIDs  []string `json:"evidence_ids,omitempty"`
	RequestBytes int      `json:"request_bytes"`
}

// FocusedInvestigationScope identifies one bounded local expansion. It may
// contain authorized paths that were absent from initial orientation.
type FocusedInvestigationScope struct {
	QuestionID          string   `json:"question_id"`
	FocusedEvidenceIDs  []string `json:"focused_evidence_ids"`
	LocallyInspected    []string `json:"locally_inspected_paths"`
	ProviderEvidenceIDs []string `json:"provider_evidence_ids,omitempty"`
}
