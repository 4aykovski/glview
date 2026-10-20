package gitlab

import (
	"errors"
	"fmt"
	"time"
)

// Error kinds. Use errors.Is(err, gitlab.ErrUnauthorized) etc.
var (
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrInsufficientScope = errors.New("insufficient token scope")
	ErrNotFound          = errors.New("not found or no access")
	ErrConflict          = errors.New("conflict")
	ErrNotPossible       = errors.New("operation not possible")
	ErrRateLimited       = errors.New("rate limited")
	ErrNetwork           = errors.New("host unreachable")
	ErrTLS               = errors.New("TLS error")
	ErrComplexity        = errors.New("GraphQL query too complex")
	ErrServer            = errors.New("server error")
	// ErrNotReady means the server has not computed the result yet (CI config
	// variables); the caller may retry later.
	ErrNotReady = errors.New("not ready")
)

// Error is an API error with its kind, HTTP status and server message.
type Error struct {
	Kind       error // one of the Err* values above
	Status     int   // HTTP status, 0 for transport errors
	Message    string
	RetryAfter time.Duration // set for ErrRateLimited when known
	Err        error         // underlying error, if any
}

func (e *Error) Error() string {
	switch {
	case e.Message != "" && e.Status != 0:
		return fmt.Sprintf("%v (HTTP %d): %s", e.Kind, e.Status, e.Message)
	case e.Status != 0:
		return fmt.Sprintf("%v (HTTP %d)", e.Kind, e.Status)
	case e.Message != "":
		return fmt.Sprintf("%v: %s", e.Kind, e.Message)
	case e.Err != nil:
		return fmt.Sprintf("%v: %v", e.Kind, e.Err)
	default:
		return e.Kind.Error()
	}
}

// Unwrap lets errors.Is match both the kind and the underlying error.
// ErrInsufficientScope also matches ErrForbidden.
func (e *Error) Unwrap() []error {
	errs := []error{e.Kind}
	if e.Kind == ErrInsufficientScope {
		errs = append(errs, ErrForbidden)
	}
	if e.Err != nil {
		errs = append(errs, e.Err)
	}
	return errs
}
