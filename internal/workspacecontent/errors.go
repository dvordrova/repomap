package workspacecontent

import "errors"

// ErrorKind is the closed failure contract for authorized content reads.
type ErrorKind string

const (
	ErrorInvalidRequest  ErrorKind = "invalid_request"
	ErrorUnauthorized    ErrorKind = "unauthorized"
	ErrorUnavailable     ErrorKind = "unavailable"
	ErrorSourceChanged   ErrorKind = "source_changed"
	ErrorUnsupportedText ErrorKind = "unsupported_text"
	ErrorLimitExceeded   ErrorKind = "limit_exceeded"
	ErrorCanceled        ErrorKind = "canceled"
	ErrorReadFailed      ErrorKind = "read_failed"
)

// LimitKind identifies which fixed bound rejected a request without exposing
// request content or local filesystem details.
type LimitKind string

const (
	LimitNone  LimitKind = ""
	LimitFile  LimitKind = "file"
	LimitLines LimitKind = "lines"
	LimitText  LimitKind = "text"
	LimitLine  LimitKind = "line"
)

// FailureStage identifies the closed phase that rejected the read. Adapters
// may use it to preserve an existing error message without inspecting raw
// filesystem errors.
type FailureStage string

const (
	StageRequest   FailureStage = "request"
	StageAuthority FailureStage = "authority"
	StageRead      FailureStage = "read"
	StageRange     FailureStage = "range"
)

type contentError struct {
	kind  ErrorKind
	limit LimitKind
	stage FailureStage
}

func (err *contentError) Error() string {
	if err == nil {
		return "workspace content: read failed"
	}
	return "workspace content: " + string(err.kind)
}

func workspaceContentError(kind ErrorKind, stage FailureStage) error {
	return &contentError{kind: kind, stage: stage}
}

func workspaceContentLimit(limit LimitKind) error {
	stage := StageRange
	if limit == LimitFile {
		stage = StageRead
	}
	return &contentError{kind: ErrorLimitExceeded, limit: limit, stage: stage}
}

// ErrorKindOf returns the closed kind for an error produced by this package.
func ErrorKindOf(err error) ErrorKind {
	var target *contentError
	if errors.As(err, &target) {
		return target.kind
	}
	return ""
}

// LimitKindOf returns the closed limit discriminator for a limit error.
func LimitKindOf(err error) LimitKind {
	var target *contentError
	if errors.As(err, &target) {
		return target.limit
	}
	return LimitNone
}

// FailureStageOf returns the closed phase for an error produced by this
// package.
func FailureStageOf(err error) FailureStage {
	var target *contentError
	if errors.As(err, &target) {
		return target.stage
	}
	return ""
}
