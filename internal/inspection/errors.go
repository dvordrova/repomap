package inspection

import (
	"errors"
	"fmt"
)

// ErrorKind is the closed set of presentation-neutral failure classes exposed
// by the inspection service.
type ErrorKind string

const (
	ErrorInvalidRequest      ErrorKind = "invalid_request"
	ErrorUnauthorized        ErrorKind = "unauthorized"
	ErrorSourceChanged       ErrorKind = "source_changed"
	ErrorNotFound            ErrorKind = "not_found"
	ErrorAnalyzerUnavailable ErrorKind = "analyzer_unavailable"
	ErrorAnalysisFailed      ErrorKind = "analysis_failed"
)

// Error carries a neutral failure kind and operation. Its text deliberately
// omits analyzer prose and local paths; adapters may inspect Cause through
// errors.Is without exposing it to users.
type Error struct {
	Kind      ErrorKind
	Operation string
	Cause     error
}

func (e *Error) Error() string {
	if e == nil {
		return "inspection: unknown failure"
	}
	if e.Operation == "" {
		return fmt.Sprintf("inspection: %s", e.Kind)
	}
	return fmt.Sprintf("inspection: %s: %s", e.Operation, e.Kind)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// ErrorKindOf returns the neutral kind of err, or the empty string when err is
// not an inspection error.
func ErrorKindOf(err error) ErrorKind {
	var inspectionError *Error
	if !errors.As(err, &inspectionError) {
		return ""
	}
	return inspectionError.Kind
}

func inspectionError(kind ErrorKind, operation string, cause error) error {
	return &Error{Kind: kind, Operation: operation, Cause: cause}
}
