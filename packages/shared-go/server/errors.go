package server

import "errors"

var (
	ErrBadRequest            = errors.New("bad request")              // 400
	ErrUnauthorized          = errors.New("unauthorized")             // 401
	ErrForbidden             = errors.New("forbidden")                // 403
	ErrNotFound              = errors.New("not found")                // 404
	ErrConflict              = errors.New("conflict")                 // 409
	ErrGone                  = errors.New("gone")                     // 410
	ErrRequestEntityTooLarge = errors.New("request entity too large") // 413
	ErrUnsupportedMediaType  = errors.New("unsupported media type")   // 415
	ErrValidation            = errors.New("validation")               // 422
)

func StatusFor(err error) int {
	switch {
	case errors.Is(err, ErrBadRequest):
		return 400
	case errors.Is(err, ErrUnauthorized):
		return 401
	case errors.Is(err, ErrForbidden):
		return 403
	case errors.Is(err, ErrNotFound):
		return 404
	case errors.Is(err, ErrConflict):
		return 409
	case errors.Is(err, ErrGone):
		return 410
	case errors.Is(err, ErrRequestEntityTooLarge):
		return 413
	case errors.Is(err, ErrUnsupportedMediaType):
		return 415
	case errors.Is(err, ErrValidation):
		return 422
	default:
		return 500
	}
}

// Detailed wraps a status sentinel with a human-readable JSON:API `detail`.
//
// Error() returns the BASE sentinel's message, not a concatenation: the
// envelope's `title` is `err.Error()`, so wrapping with fmt.Errorf("...: %w")
// would turn every 409 title into a sentence and change the response shape for
// every existing caller. The detail rides in the `detail` field instead.
//
// errors.Is matches both the wrapper (pointer identity, so a handler can tell
// two sentinels over the same base apart) and the base sentinel (via Unwrap, so
// StatusFor keeps mapping it to the right status with no new mapping code).
func Detailed(base error, detail string) error {
	return &detailedError{base: base, detail: detail}
}

type detailedError struct {
	base   error
	detail string
}

func (e *detailedError) Error() string  { return e.base.Error() }
func (e *detailedError) Unwrap() error  { return e.base }
func (e *detailedError) Detail() string { return e.detail }

// APIError is one entry in the standard error envelope (design §6).
type APIError struct {
	Status string       `json:"status"`
	Code   string       `json:"code"`
	Title  string       `json:"title"`
	Detail string       `json:"detail,omitempty"`
	Source *ErrorSource `json:"source,omitempty"`
}

type ErrorSource struct {
	Pointer string `json:"pointer,omitempty"`
}
