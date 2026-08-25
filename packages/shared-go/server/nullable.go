package server

import (
	"bytes"
	"encoding/json"
)

// Nullable distinguishes the three states a JSON PATCH field can be in:
// absent (Present == false), explicit null (Present && !Valid), and a value
// (Present && Valid).
//
// A *T cannot express the middle state: encoding/json sets a settable pointer
// to nil for an explicit null, making it byte-identical to an omitted key. Any
// PATCH field whose "cleared" state is a NULL column — as opposed to a zero
// value that already means "unset" — needs this instead of a pointer.
type Nullable[T any] struct {
	// Present reports whether the key appeared in the request body at all.
	Present bool
	// Valid reports whether the key carried a value rather than null. It is
	// meaningless unless Present.
	Valid bool
	// Value is the decoded value. It is the zero value unless Present && Valid.
	Value T
}

// UnmarshalJSON records that the key was present and decodes the value unless
// it was null. It is only ever called when the key IS present, which is what
// makes Present reliable.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Present = true
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	if err := json.Unmarshal(b, &n.Value); err != nil {
		return err
	}
	n.Valid = true
	return nil
}
