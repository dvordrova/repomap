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

type contentError struct {
	kind  ErrorKind
	limit LimitKind
}

func (err *contentError) Error() string {
	if err == nil {
		return "workspace content: read failed"
	}
	return "workspace content: " + string(err.kind)
}

func workspaceContentError(kind ErrorKind) error {
	return &contentError{kind: kind}
}

func workspaceContentLimit(limit LimitKind) error {
	return &contentError{kind: ErrorLimitExceeded, limit: limit}
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
