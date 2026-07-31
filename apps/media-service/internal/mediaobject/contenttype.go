package mediaobject

import (
	"errors"
	"fmt"
	"mime"
	"sort"
	"strings"
)

// Class is what a content type means to the rest of the service: whether the
// variant worker can decode it, whether it is served inline, whether it is on
// the allowlist at all. Classification happens once, at the door
// (POST /media), and every later decision is a pure function of the stored
// type and the allowlist (design §2).
type Class int

const (
	// ClassUnknown is anything not on the allowlist, including legacy rows
	// created before the allowlist existed. It is never handed to image.Decode
	// and is always served as an attachment.
	ClassUnknown Class = iota
	// ClassImage is a renderable image: one the variant worker's image.Decode
	// can actually handle.
	ClassImage
	// ClassDocument is everything else on the allowlist. No variants, served
	// as an attachment.
	ClassDocument
)

// DefaultAllowedContentTypes is the built-in value of MEDIA_ALLOWED_CONTENT_TYPES
// (PRD FR-MEDIA-2). Adding a document type is a ConfigMap edit; adding a
// renderable type is a code change, because it needs a decoder.
const DefaultAllowedContentTypes = "image/jpeg,image/png,application/pdf," +
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document," +
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"

// renderableImages is deliberately NOT configurable (design D11). It is a
// statement about what processing/worker.go can decode, not a deployment
// preference: a config key that could add image/heic here would let an operator
// hand the worker bytes it cannot read.
var renderableImages = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
}

// Allowlist is an immutable value object: no database, no HTTP, no clock. It is
// built once in cmd/main.go and injected, which is what lets Classify and
// Normalize be unit-tested directly.
type Allowlist struct {
	allowed map[string]Class
}

// ParseAllowlist builds an Allowlist from a comma-separated list of media
// types. Entries are normalised the same way an upload's content type is, so
// the config and the request are compared on identical terms. A malformed or
// empty list is an error: a malformed allowlist must not boot into "allow
// nothing" or "allow everything".
func ParseAllowlist(csv string) (Allowlist, error) {
	allowed := make(map[string]Class)
	for _, part := range strings.Split(csv, ",") {
		raw := strings.TrimSpace(part)
		if raw == "" {
			continue
		}
		mt, _, err := mime.ParseMediaType(raw)
		if err != nil {
			return Allowlist{}, fmt.Errorf("allowlist entry %q: %w", raw, err)
		}
		mt = strings.ToLower(mt)
		if renderableImages[mt] {
			allowed[mt] = ClassImage
			continue
		}
		allowed[mt] = ClassDocument
	}
	if len(allowed) == 0 {
		return Allowlist{}, errors.New("allowlist is empty")
	}
	return Allowlist{allowed: allowed}, nil
}

// Normalize parses raw, discards its parameters, lowercases the bare media
// type, and reports whether the result is on the allowlist. The normalised
// value is what callers persist, so no arbitrary client string is ever stored
// (design D10).
func (a Allowlist) Normalize(raw string) (string, bool) {
	mt, _, err := mime.ParseMediaType(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	mt = strings.ToLower(mt)
	if _, ok := a.allowed[mt]; !ok {
		return "", false
	}
	return mt, true
}

// Classify reports the class of a content type. Anything unparseable or off the
// allowlist is ClassUnknown.
func (a Allowlist) Classify(contentType string) Class {
	mt, ok := a.Normalize(contentType)
	if !ok {
		return ClassUnknown
	}
	return a.allowed[mt]
}

// Resolve returns the Content-Type to write on a download response and its
// class. It is called on every read rather than trusting the stored value, so
// shrinking the allowlist retroactively downgrades already-stored objects to
// attachment + octet-stream on their next request (design D15).
func (a Allowlist) Resolve(stored string) (string, Class) {
	mt, ok := a.Normalize(stored)
	if !ok {
		return "application/octet-stream", ClassUnknown
	}
	return mt, a.allowed[mt]
}

// Accepted returns the allowlist entries in a stable order, for the message
// that accompanies a 415 so the client can render something actionable.
func (a Allowlist) Accepted() []string {
	out := make([]string, 0, len(a.allowed))
	for mt := range a.allowed {
		out = append(out, mt)
	}
	sort.Strings(out)
	return out
}
