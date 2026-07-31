package server

import "errors"

var (
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
