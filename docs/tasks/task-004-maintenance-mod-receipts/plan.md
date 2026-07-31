# Maintenance & Modification Logging with Receipts — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the receipt loop — attach, view and download documents on maintenance
records — and introduce modifications as a kind of maintenance category, while making
media-service accept non-image files safely.

**Architecture:** Four slices from design §1. **B** (media-service content-type allowlist,
document class, terminal processing state) and **C** (download hardening) land first
because everything else depends on the allowlist. **A** (category `kind`, record
`description`, kind filtering) is independent. **D** (the attachment lifecycle) integrates
them and carries the one new cross-service seam: a single batch internal endpoint on
media-service (`GET /internal/media`) called by a single new HTTP client in fleet-service.
No shared database access; no change to the existing image path.

**Tech Stack:** Go 1.25 (chi v5, GORM, logrus, kafka-go), React 19 + TypeScript + Vite,
TanStack React Query, react-hook-form + Zod, Tailwind + shadcn/ui, vitest, kustomize/Traefik.

## Global Constraints

- **Read `context.md` first.** It documents the DDD layering, the JSON:API envelope, the
  internal-endpoint precedent, and the four deviations this plan makes from the design.
- Immutable domain models: unexported fields, value receivers, `WithX` copy-mutators. No
  domain package imports another domain's provider — cross-domain access is a one-method
  interface declared in the consumer.
- Cross-service data moves over HTTP, never over the database.
- `ClassImage` is exactly `{image/jpeg, image/png}` and is **not** configurable.
- The allowlist default value, verbatim:
  `image/jpeg,image/png,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv`
- `MEDIA_MAX_UPLOAD_BYTES` stays `26214400`; the client mirror `MEDIA_MAX_UPLOAD_BYTES` in
  `apps/web/src/lib/hooks/api/media.ts` stays in sync.
- `MaxDescriptionRunes = 200` (runes, not bytes). `MaxDocuments = 10`.
- Every unauthenticated `/internal/*` route ships in the same change as its priority-200
  `internal-deny` Traefik rule. Never separately.
- Commit after every task. Conventional-commit prefixes (`feat:`, `fix:`, `refactor:`,
  `test:`, `chore:`).
- `make ci` must pass at the end (Task 26). Do not claim a task is done without running the
  command in its verification step and reading the output.

---

## Task 1: `415` sentinel in shared-go

**Files:**
- Modify: `packages/shared-go/server/errors.go`
- Test: `packages/shared-go/server/errors_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.ErrUnsupportedMediaType` (an `error`), mapped to `415` by
  `server.StatusFor`. Consumed by Task 5.

- [ ] **Step 1: Add the failing test case**

In `packages/shared-go/server/errors_test.go`, add `ErrUnsupportedMediaType: 415` to the
existing map and add the two statuses the map is currently missing so the table matches
the sentinel set:

```go
func TestStatusFor_mapsDomainErrors(t *testing.T) {
	cases := map[error]int{
		ErrUnauthorized:          401,
		ErrForbidden:             403,
		ErrNotFound:              404,
		ErrConflict:              409,
		ErrGone:                  410,
		ErrRequestEntityTooLarge: 413,
		ErrUnsupportedMediaType:  415,
		ErrValidation:            422,
	}
	for err, want := range cases {
		if got := StatusFor(err); got != want {
			t.Fatalf("StatusFor(%v)=%d want %d", err, got, want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd packages/shared-go && go test ./server/... -run TestStatusFor -v`
Expected: FAIL — compile error, `undefined: ErrUnsupportedMediaType`.

- [ ] **Step 3: Add the sentinel and its `StatusFor` arm**

In `packages/shared-go/server/errors.go`:

```go
var (
	ErrUnauthorized          = errors.New("unauthorized")             // 401
	ErrForbidden             = errors.New("forbidden")                // 403
	ErrNotFound              = errors.New("not found")                // 404
	ErrConflict              = errors.New("conflict")                 // 409
	ErrGone                  = errors.New("gone")                     // 410
	ErrRequestEntityTooLarge = errors.New("request entity too large")  // 413
	ErrUnsupportedMediaType  = errors.New("unsupported media type")    // 415
	ErrValidation            = errors.New("validation")               // 422
)
```

and, in `StatusFor`, between the `413` and `422` arms:

```go
	case errors.Is(err, ErrUnsupportedMediaType):
		return 415
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd packages/shared-go && go test ./server/... -run TestStatusFor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add packages/shared-go/server/errors.go packages/shared-go/server/errors_test.go
git commit -m "feat(shared-go): add ErrUnsupportedMediaType (415) sentinel"
```

---

## Task 2: content-type allowlist value object

**Files:**
- Create: `apps/media-service/internal/mediaobject/contenttype.go`
- Create: `apps/media-service/internal/mediaobject/contenttype_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces, all in package `mediaobject`:
  - `type Class int` with `ClassUnknown`, `ClassImage`, `ClassDocument`
  - `const DefaultAllowedContentTypes string`
  - `type Allowlist struct{ … }`
  - `func ParseAllowlist(csv string) (Allowlist, error)`
  - `func (a Allowlist) Normalize(raw string) (string, bool)`
  - `func (a Allowlist) Classify(contentType string) Class`
  - `func (a Allowlist) Resolve(stored string) (string, Class)`
  - `func (a Allowlist) Accepted() []string`
  Consumed by Tasks 3, 5, 6, 7.

- [ ] **Step 1: Write the failing tests**

Create `apps/media-service/internal/mediaobject/contenttype_test.go`:

```go
package mediaobject

import (
	"strings"
	"testing"
)

func testAllowlist(t *testing.T) Allowlist {
	t.Helper()
	a, err := ParseAllowlist(DefaultAllowedContentTypes)
	if err != nil {
		t.Fatalf("ParseAllowlist(default): %v", err)
	}
	return a
}

// The two renderable types are hard-coded, not configured: they are a statement
// about what processing/worker.go's image.Decode can actually handle.
func TestClassify_rendersOnlyJPEGAndPNGAsImages(t *testing.T) {
	a := testAllowlist(t)
	for _, ct := range []string{"image/jpeg", "image/png"} {
		if got := a.Classify(ct); got != ClassImage {
			t.Fatalf("Classify(%q)=%v want ClassImage", ct, got)
		}
	}
	for _, ct := range []string{
		"application/pdf",
		"text/csv",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	} {
		if got := a.Classify(ct); got != ClassDocument {
			t.Fatalf("Classify(%q)=%v want ClassDocument", ct, got)
		}
	}
}

func TestClassify_offListAndMalformedAreUnknown(t *testing.T) {
	a := testAllowlist(t)
	for _, ct := range []string{"text/html", "image/heic", "", "not a media type", "///"} {
		if got := a.Classify(ct); got != ClassUnknown {
			t.Fatalf("Classify(%q)=%v want ClassUnknown", ct, got)
		}
	}
}

// text/csv; charset=utf-8 is what browsers actually send. A bare map lookup
// would reject it (design D10).
func TestNormalize_dropsParametersAndLowercases(t *testing.T) {
	a := testAllowlist(t)
	cases := map[string]string{
		"text/csv; charset=utf-8": "text/csv",
		"APPLICATION/PDF":         "application/pdf",
		"image/jpeg":              "image/jpeg",
		" image/png ":             "image/png",
	}
	for in, want := range cases {
		got, ok := a.Normalize(in)
		if !ok || got != want {
			t.Fatalf("Normalize(%q)=(%q,%v) want (%q,true)", in, got, ok, want)
		}
	}
}

func TestNormalize_rejectsEmptyAndOffList(t *testing.T) {
	a := testAllowlist(t)
	for _, in := range []string{"", "   ", "text/html", "application/x-msdownload"} {
		if got, ok := a.Normalize(in); ok {
			t.Fatalf("Normalize(%q)=(%q,true) want ok=false", in, got)
		}
	}
}

// Re-resolving on read is what protects rows created before the allowlist
// existed (design D15).
func TestResolve_offListFallsBackToOctetStream(t *testing.T) {
	a := testAllowlist(t)

	ct, class := a.Resolve("application/pdf")
	if ct != "application/pdf" || class != ClassDocument {
		t.Fatalf("Resolve(pdf)=(%q,%v) want (application/pdf, ClassDocument)", ct, class)
	}

	ct, class = a.Resolve("text/html")
	if ct != "application/octet-stream" || class != ClassUnknown {
		t.Fatalf("Resolve(text/html)=(%q,%v) want (application/octet-stream, ClassUnknown)", ct, class)
	}

	ct, class = a.Resolve("")
	if ct != "application/octet-stream" || class != ClassUnknown {
		t.Fatalf("Resolve(\"\")=(%q,%v) want (application/octet-stream, ClassUnknown)", ct, class)
	}
}

func TestParseAllowlist_rejectsMalformedAndEmpty(t *testing.T) {
	if _, err := ParseAllowlist("image/jpeg,not a media type"); err == nil {
		t.Fatal("ParseAllowlist accepted a malformed entry; want error")
	}
	if _, err := ParseAllowlist("   ,  "); err == nil {
		t.Fatal("ParseAllowlist accepted an empty list; want error")
	}
}

func TestParseAllowlist_toleratesWhitespaceAndTrailingCommas(t *testing.T) {
	a, err := ParseAllowlist(" image/jpeg , application/pdf ,")
	if err != nil {
		t.Fatalf("ParseAllowlist: %v", err)
	}
	if a.Classify("image/jpeg") != ClassImage || a.Classify("application/pdf") != ClassDocument {
		t.Fatal("whitespace-padded entries were not parsed")
	}
}

// Accepted() feeds the 415 message, so it must be stable and complete.
func TestAccepted_isSortedAndComplete(t *testing.T) {
	a := testAllowlist(t)
	got := strings.Join(a.Accepted(), ",")
	want := "application/pdf," +
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet," +
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document," +
		"image/jpeg,image/png,text/csv"
	if got != want {
		t.Fatalf("Accepted()=%q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run 'TestClassify|TestNormalize|TestResolve|TestParseAllowlist|TestAccepted' -v`
Expected: FAIL — compile error, `undefined: ParseAllowlist`.

- [ ] **Step 3: Write the implementation**

Create `apps/media-service/internal/mediaobject/contenttype.go`:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run 'TestClassify|TestNormalize|TestResolve|TestParseAllowlist|TestAccepted' -v`
Expected: PASS, 8 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/mediaobject/contenttype.go apps/media-service/internal/mediaobject/contenttype_test.go
git commit -m "feat(media-service): add content-type allowlist value object"
```

---

## Task 3: `Content-Disposition` builder

**Files:**
- Create: `apps/media-service/internal/mediaobject/download.go`
- Create: `apps/media-service/internal/mediaobject/download_test.go`

**Interfaces:**
- Consumes: `Class`, `ClassImage`, `ClassDocument`, `ClassUnknown` (Task 2).
- Produces: `func ContentDisposition(class Class, filename, fallback string) string`.
  Consumed by Task 7.

- [ ] **Step 1: Write the failing tests**

Create `apps/media-service/internal/mediaobject/download_test.go`:

```go
package mediaobject

import (
	"strings"
	"testing"
)

func TestContentDisposition_imagesInlineDocumentsAttachment(t *testing.T) {
	if got := ContentDisposition(ClassImage, "photo.jpg", "m1"); got != `inline; filename="photo.jpg"` {
		t.Fatalf("image: %q", got)
	}
	if got := ContentDisposition(ClassDocument, "invoice.pdf", "m1"); got != `attachment; filename="invoice.pdf"` {
		t.Fatalf("document: %q", got)
	}
	// Legacy rows whose stored type nobody recognises are attachments too.
	if got := ContentDisposition(ClassUnknown, "unknown.bin", "m1"); got != `attachment; filename="unknown.bin"` {
		t.Fatalf("unknown: %q", got)
	}
}

// The acceptance criterion: a filename must never be able to inject a header.
func TestContentDisposition_stripsControlCharacters(t *testing.T) {
	got := ContentDisposition(ClassDocument, "in\r\nX-Evil: 1\x00voice\x7f.pdf", "m1")
	if strings.ContainsAny(got, "\r\n\x00\x7f") {
		t.Fatalf("control characters survived: %q", got)
	}
	if got != `attachment; filename="inX-Evil: 1voice.pdf"` {
		t.Fatalf("got %q", got)
	}
}

func TestContentDisposition_escapesQuotesAndBackslashes(t *testing.T) {
	got := ContentDisposition(ClassDocument, `in"voice.pdf`, "m1")
	if got != `attachment; filename="in\"voice.pdf"` {
		t.Fatalf("got %q", got)
	}
}

// A filename must not suggest a path to the client.
func TestContentDisposition_takesBaseNameOnly(t *testing.T) {
	for _, in := range []string{"../../etc/passwd", `C:\Users\bob\invoice.pdf`, "/tmp/invoice.pdf"} {
		got := ContentDisposition(ClassDocument, in, "m1")
		if strings.Contains(got, "/") || strings.Contains(got, `\\`) {
			t.Fatalf("path survived for %q: %q", in, got)
		}
	}
	if got := ContentDisposition(ClassDocument, "/tmp/invoice.pdf", "m1"); got != `attachment; filename="invoice.pdf"` {
		t.Fatalf("got %q", got)
	}
}

func TestContentDisposition_nonASCIIGetsRFC5987Form(t *testing.T) {
	got := ContentDisposition(ClassDocument, "facturé señal.pdf", "m1")
	want := `attachment; filename="factur_ se_al.pdf"; filename*=UTF-8''factur%C3%A9%20se%C3%B1al.pdf`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// url.QueryEscape encodes space as '+' and url.PathEscape leaves '?', '&' and
// '=' alone; both produce headers that mostly work, which is the worst outcome
// for a security-adjacent function (design D14).
func TestContentDisposition_rfc5987EncodesReservedCharacters(t *testing.T) {
	got := ContentDisposition(ClassDocument, "a?b&c=d é.pdf", "m1")
	if !strings.Contains(got, "filename*=UTF-8''a%3Fb%26c%3Dd%20%C3%A9.pdf") {
		t.Fatalf("reserved characters not encoded: %q", got)
	}
	if strings.Contains(got, "+") {
		t.Fatalf("space encoded as '+': %q", got)
	}
}

func TestContentDisposition_emptyNameFallsBackToMediaID(t *testing.T) {
	if got := ContentDisposition(ClassDocument, "", "m-123"); got != `attachment; filename="m-123"` {
		t.Fatalf("empty: %q", got)
	}
	// Fully stripped by sanitisation is the same case.
	if got := ContentDisposition(ClassDocument, "\r\n\x00", "m-123"); got != `attachment; filename="m-123"` {
		t.Fatalf("stripped: %q", got)
	}
	// And if even the fallback is unusable, the header is still well-formed.
	if got := ContentDisposition(ClassDocument, "", ""); got != `attachment; filename="download"` {
		t.Fatalf("no fallback: %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run TestContentDisposition -v`
Expected: FAIL — compile error, `undefined: ContentDisposition`.

- [ ] **Step 3: Write the implementation**

Create `apps/media-service/internal/mediaobject/download.go`:

```go
package mediaobject

import (
	"fmt"
	"strings"
)

// ContentDisposition renders the Content-Disposition header value for a
// download. filename is untrusted input supplied by whoever uploaded the file.
//
// This is a pure function on purpose (design D14): "a filename containing
// quotes, newlines or non-ASCII characters cannot corrupt or inject the header"
// is only cheap to prove if the logic is not buried in an HTTP handler.
//
// Sanitisation order matters:
//  1. Strip every character below 0x20 plus 0x7F. This is what makes header
//     injection structurally impossible, and it runs first so nothing
//     downstream ever sees a CR or LF.
//  2. Take the base name only, so a filename cannot suggest a path.
//  3. Build the ASCII form: non-ASCII runes become '_', and '\' and '"' are
//     escaped for the quoted-string.
//  4. If nothing usable is left, fall back (the caller passes the media ID).
//  5. When the original held non-ASCII, additionally emit the RFC 5987
//     filename* form, keeping the ASCII filename= as the older-client fallback.
func ContentDisposition(class Class, filename, fallback string) string {
	disposition := "attachment"
	if class == ClassImage {
		disposition = "inline"
	}

	name := baseName(stripControl(filename))
	if name == "" {
		name = baseName(stripControl(fallback))
	}
	if name == "" {
		name = "download"
	}

	out := disposition + `; filename="` + asciiFold(name) + `"`
	if hasNonASCII(name) {
		out += "; filename*=UTF-8''" + encodeRFC5987(name)
	}
	return out
}

// stripControl removes every rune below 0x20 plus 0x7F (DEL).
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}

// baseName drops everything up to the last '/' or '\'.
func baseName(s string) string {
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		return s[i+1:]
	}
	return s
}

// asciiFold replaces every non-ASCII rune with '_' and escapes the two
// characters that are special inside a quoted-string.
func asciiFold(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r > 0x7E:
			b.WriteByte('_')
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 0x7E {
			return true
		}
	}
	return false
}

// encodeRFC5987 percent-encodes every byte outside RFC 5987's attr-char set.
// Hand-written because url.QueryEscape encodes space as '+' and url.PathEscape
// leaves '?', '&' and '=' unescaped.
func encodeRFC5987(s string) string {
	const attrChar = "!#$&+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			strings.IndexByte(attrChar, c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
```

Note: `&` and `+` are attr-chars per RFC 5987, so they are emitted literally; the test
above only asserts `?`, `=` and space are encoded.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run TestContentDisposition -v`
Expected: PASS, 7 tests.

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/mediaobject/download.go apps/media-service/internal/mediaobject/download_test.go
git commit -m "feat(media-service): add injection-safe Content-Disposition builder"
```

---

## Task 4: terminal media statuses and their transition guards

**Files:**
- Modify: `apps/media-service/internal/mediaobject/model.go`
- Modify: `apps/media-service/internal/mediaobject/processor.go`
- Test: `apps/media-service/internal/mediaobject/processor_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `mediaobject.StatusFailed Status = "failed"`,
  `func MarkReadyDirect(m Model) (Model, error)`, `func MarkFailed(m Model) (Model, error)`.
  Consumed by Tasks 6 and 8.

- [ ] **Step 1: Write the failing tests**

Append to `apps/media-service/internal/mediaobject/processor_test.go` (it already imports
`testing`, `errors` and `server`; add imports only if the file does not already have them):

```go
// MarkReadyDirect is the documents-only shortcut: uploaded → ready with no
// worker in between (design D12). Any other source state is a conflict.
func TestMarkReadyDirect_requiresUploaded(t *testing.T) {
	uploaded := Model{status: StatusUploaded}
	got, err := MarkReadyDirect(uploaded)
	if err != nil {
		t.Fatalf("MarkReadyDirect(uploaded): %v", err)
	}
	if got.Status() != StatusReady {
		t.Fatalf("status = %q, want ready", got.Status())
	}

	for _, s := range []Status{StatusProcessing, StatusReady, StatusFailed} {
		if _, err := MarkReadyDirect(Model{status: s}); !errors.Is(err, server.ErrConflict) {
			t.Fatalf("MarkReadyDirect(%q) err = %v, want ErrConflict", s, err)
		}
	}
}

// MarkFailed is the only terminal transition out of the pipeline that is not
// ready. It accepts uploaded or processing (design D13).
func TestMarkFailed_acceptsUploadedAndProcessing(t *testing.T) {
	for _, s := range []Status{StatusUploaded, StatusProcessing} {
		got, err := MarkFailed(Model{status: s})
		if err != nil {
			t.Fatalf("MarkFailed(%q): %v", s, err)
		}
		if got.Status() != StatusFailed {
			t.Fatalf("MarkFailed(%q) status = %q, want failed", s, got.Status())
		}
	}
	for _, s := range []Status{StatusReady, StatusFailed} {
		if _, err := MarkFailed(Model{status: s}); !errors.Is(err, server.ErrConflict) {
			t.Fatalf("MarkFailed(%q) err = %v, want ErrConflict", s, err)
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run 'TestMarkReadyDirect|TestMarkFailed' -v`
Expected: FAIL — compile error, `undefined: StatusFailed`.

- [ ] **Step 3: Add the status and the two guards**

In `apps/media-service/internal/mediaobject/model.go`, extend the `Status` block:

```go
const (
	StatusUploaded   Status = "uploaded"
	StatusProcessing Status = "processing"
	StatusReady      Status = "ready"
	// StatusFailed is terminal. It exists because "ready with zero variants"
	// would be a promise the object cannot keep: every consumer of ready
	// assumes displayable bytes, and an object that lies about its state is a
	// bug that surfaces far from its cause (design D13).
	StatusFailed Status = "failed"
)
```

In `apps/media-service/internal/mediaobject/processor.go`, immediately after `MarkReady`:

```go
// MarkReadyDirect transitions uploaded → ready for objects that need no
// processing (documents). Any other source state is a conflict (409). MarkReady
// is deliberately left untouched so the worker's behaviour and tests are
// unchanged (design D12).
func MarkReadyDirect(m Model) (Model, error) {
	if m.Status() != StatusUploaded {
		return Model{}, server.ErrConflict
	}
	return m.WithStatus(StatusReady), nil
}

// MarkFailed is the terminal failure transition. It accepts uploaded or
// processing; anything else is a conflict. It is what guarantees no object
// stays in processing forever and no Kafka partition is blocked by one bad
// file (design D13, PRD FR-MEDIA-5).
func MarkFailed(m Model) (Model, error) {
	if m.Status() != StatusUploaded && m.Status() != StatusProcessing {
		return Model{}, server.ErrConflict
	}
	return m.WithStatus(StatusFailed), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run 'TestMark' -v`
Expected: PASS — the two new tests plus the existing `TestMarkReady_requiresProcessing`
and `TestMarkProcessing_requiresUploaded`.

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/mediaobject/model.go apps/media-service/internal/mediaobject/processor.go apps/media-service/internal/mediaobject/processor_test.go
git commit -m "feat(media-service): add failed status and terminal transition guards"
```

---

## Task 5: enforce the allowlist on `POST /media`

**Files:**
- Modify: `apps/media-service/internal/mediaobject/processor.go` (`Processor`, `NewProcessor`, `InitUpload`)
- Modify: `apps/media-service/internal/mediaobject/resource.go` (`InitializeRoutes` signature)
- Modify: `apps/media-service/cmd/main.go`
- Modify: `apps/media-service/internal/mediaobject/processor_test.go` (call sites)
- Modify: `apps/media-service/internal/mediaobject/resource_test.go` (call sites)
- Modify: `deploy/k8s/base/media-service/configmap.yaml`
- Modify: `deploy/compose/docker-compose.yml`, `deploy/compose/.env.example`

**Interfaces:**
- Consumes: `Allowlist`, `DefaultAllowedContentTypes`, `ParseAllowlist` (Task 2);
  `server.ErrUnsupportedMediaType` (Task 1).
- Produces: `NewProcessor(log, p, a, st, allow Allowlist) *Processor` and
  `InitializeRoutes(log, db, st, maxUploadBytes int64, allow Allowlist) func(chi.Router)`.
  Consumed by Tasks 6, 7, 9.

- [ ] **Step 1: Write the failing handler tests**

Append to `apps/media-service/internal/mediaobject/resource_test.go`:

```go
// initBody builds the JSON:API envelope POST /media expects.
func initBody(contentType, filename string) io.Reader {
	return strings.NewReader(`{"data":{"attributes":{"contentType":"` + contentType +
		`","originalFilename":"` + filename + `"}}}`)
}

func TestInitUpload_pdfIsAccepted(t *testing.T) {
	router, _, _ := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodPost, "/media", initBody("application/pdf", "invoice.pdf")))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", rec.Code, rec.Body.String())
	}
}

// The allowlist is a security control, not a UX affordance: broadening the
// accepted file types without it would turn media download into a same-origin
// stored-XSS vector.
func TestInitUpload_htmlIs415(t *testing.T) {
	router, _, _ := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodPost, "/media", initBody("text/html", "evil.html")))

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "application/pdf") {
		t.Fatalf("415 body does not name the accepted types: %s", rec.Body.String())
	}
}

func TestInitUpload_emptyContentTypeIs415(t *testing.T) {
	router, _, _ := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodPost, "/media", initBody("", "mystery")))

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

// text/csv; charset=utf-8 is what a browser actually sends. The parameters are
// discarded and the bare type is what gets stored (design D10).
func TestInitUpload_normalizesAndStoresBareType(t *testing.T) {
	_, proc, _ := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	m, err := proc.InitUpload("fleet-a", "u1", "TEXT/CSV; charset=utf-8", "mileage.csv")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}
	if m.ContentType() != "text/csv" {
		t.Fatalf("stored contentType = %q, want text/csv", m.ContentType())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run TestInitUpload -v`
Expected: FAIL — `TestInitUpload_htmlIs415` gets 201, `TestInitUpload_normalizesAndStoresBareType`
stores the raw string.

- [ ] **Step 3: Thread the allowlist through the processor**

In `apps/media-service/internal/mediaobject/processor.go`, add the field and parameter:

```go
type Processor struct {
	log     logrus.FieldLogger
	p       Provider
	a       Administrator
	storage ObjectStore
	allow   Allowlist
}

func NewProcessor(log logrus.FieldLogger, p Provider, a Administrator, st ObjectStore, allow Allowlist) *Processor {
	return &Processor{log: log, p: p, a: a, storage: st, allow: allow}
}
```

and replace the body of `InitUpload` up to the builder call:

```go
// InitUpload creates a media-object row in the uploaded state. The client then
// PUTs the bytes to /media/{id}/content; this service proxies them to object
// storage so MinIO is never reachable from the browser.
//
// The client-supplied content type is validated against the server-side
// allowlist and stored NORMALISED (parameters discarded, lowercased), so no
// arbitrary client string is ever persisted and GET /media/{id}/content cannot
// echo one back (PRD FR-MEDIA-1, design D10).
func (pr *Processor) InitUpload(fleetID, userID, contentType, filename string) (Model, error) {
	normalized, ok := pr.allow.Normalize(contentType)
	if !ok {
		// Log the offending type, never the bytes and never the filename.
		pr.log.WithFields(logrus.Fields{
			"content_type": contentType,
			"fleet_id":     fleetID,
			"user_id":      userID,
		}).Warn("upload rejected: content type is not on the allowlist")
		return Model{}, fmt.Errorf("%w: accepted types are %s",
			server.ErrUnsupportedMediaType, strings.Join(pr.allow.Accepted(), ", "))
	}

	id := uuid.NewString()
	key := storage.ObjectKey(fleetID, id, filename)
	m, err := NewBuilder().
		SetID(id).
		SetFleetID(fleetID).
		SetUploadedByUserID(userID).
		SetBucket(pr.storage.Bucket()).
		SetObjectKey(key).
		SetContentType(normalized).
		SetOriginalFilename(filename).
		SetStatus(StatusUploaded).
		Build()
	if err != nil {
		return Model{}, err
	}
	return pr.a.Insert(m)
}
```

Add `"fmt"` and `"strings"` to the import block.

- [ ] **Step 4: Thread it through the route initializer and `main.go`**

In `apps/media-service/internal/mediaobject/resource.go`:

```go
func InitializeRoutes(log logrus.FieldLogger, db *gorm.DB, st ObjectStore, maxUploadBytes int64, allow Allowlist) func(chi.Router) {
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db), st, allow)
	return func(r chi.Router) {
```

In `apps/media-service/cmd/main.go`, after `maxUploadBytes` is read:

```go
	// The allowlist is a security control (PRD FR-MEDIA-1). Fatal on a parse
	// error: a malformed list must not boot into "allow nothing" or "allow
	// everything".
	allow, err := mediaobject.ParseAllowlist(
		config.Get("MEDIA_ALLOWED_CONTENT_TYPES", mediaobject.DefaultAllowedContentTypes))
	if err != nil {
		log.WithError(err).Fatal("parse MEDIA_ALLOWED_CONTENT_TYPES")
	}
```

and update the route wiring:

```go
				mediaobject.InitializeRoutes(log, db, store, maxUploadBytes, allow)(pr)
```

- [ ] **Step 5: Fix the existing test call sites**

In `apps/media-service/internal/mediaobject/resource_test.go`, `testRouter` must build and
pass an allowlist. It also starts returning the `*gorm.DB` it built, which Tasks 6, 7 and 9
need in order to inspect the outbox and rewrite a row directly:

```go
func testRouter(t *testing.T, store ObjectStore, maxUploadBytes int64) (http.Handler, *Processor, *gorm.DB) {
	t.Helper()
	db := newConfirmTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	allow, err := ParseAllowlist(DefaultAllowedContentTypes)
	if err != nil {
		t.Fatalf("ParseAllowlist: %v", err)
	}
	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, store, maxUploadBytes, allow))
	return r, NewProcessor(log, NewProvider(db), NewAdministrator(db), store, allow), db
}
```

Update the five existing `testRouter` call sites in this file to the three-value form,
discarding what they do not use — e.g.
`router, proc, _ := testRouter(t, store, cap)`. Update the four new
`TestInitUpload_*` tests from Step 1 the same way.

Then run `cd apps/media-service && go build ./...` and give every remaining
`NewProcessor(` call in `processor_test.go` the same allowlist. `contenttype_test.go`
already defines `testAllowlist(t)` in this package (Task 2), so reuse it rather than
adding a second helper:

```go
	proc := NewProcessor(log, NewProvider(db), NewAdministrator(db), store, testAllowlist(t))
```

- [ ] **Step 6: Add the config key**

`deploy/k8s/base/media-service/configmap.yaml` — append:

```yaml
  # Server-side upload allowlist (PRD FR-MEDIA-2). This is a security control,
  # not a UX affordance: the client's `accept` attribute is a convenience only.
  # Adding a document type here is safe; renderable image types are hard-coded
  # in mediaobject/contenttype.go because they need a decoder.
  MEDIA_ALLOWED_CONTENT_TYPES: "image/jpeg,image/png,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv"
```

`deploy/compose/docker-compose.yml`, in the `media-service` `environment:` block after
`MEDIA_WORKERS`:

```yaml
      MEDIA_ALLOWED_CONTENT_TYPES: ${MEDIA_ALLOWED_CONTENT_TYPES:-image/jpeg,image/png,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv}
```

`deploy/compose/.env.example`, after `MEDIA_WORKERS=2`:

```
MEDIA_ALLOWED_CONTENT_TYPES=image/jpeg,image/png,application/pdf,application/vnd.openxmlformats-officedocument.wordprocessingml.document,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet,text/csv
```

- [ ] **Step 7: Run the tests to verify they pass**

Run: `cd apps/media-service && go build ./... && go test ./... -v 2>&1 | tail -40`
Expected: PASS, including the four new `TestInitUpload_*` tests and every pre-existing test.

Run: `make manifests`
Expected: both overlays render; no invariant violations.

- [ ] **Step 8: Commit**

```bash
git add apps/media-service deploy/k8s/base/media-service/configmap.yaml deploy/compose/docker-compose.yml deploy/compose/.env.example
git commit -m "feat(media-service): enforce content-type allowlist on upload init"
```

---

## Task 6: documents short-circuit `Confirm` to `ready`

**Files:**
- Modify: `apps/media-service/internal/mediaobject/processor.go` (`Confirm`)
- Test: `apps/media-service/internal/mediaobject/processor_test.go`

**Interfaces:**
- Consumes: `Allowlist.Classify`, `ClassImage` (Task 2); `MarkReadyDirect` (Task 4).
- Produces: no new symbols. Behaviour: `Confirm` publishes no outbox row for
  `ClassDocument` / `ClassUnknown`.

- [ ] **Step 1: Write the failing test**

Append to `apps/media-service/internal/mediaobject/resource_test.go` (that is where
`testRouter` and `fakeStore` live). The outbox assertion uses `sharedevents.OutboxRow`, the
same type `TestConfirm_enqueuesOutboxAtomically` already queries:

```go
// countOutboxRows returns the number of unsent outbox rows, which is how
// "published a media.uploaded event" is observed without standing up Kafka.
func countOutboxRows(t *testing.T, db *gorm.DB) int {
	t.Helper()
	var rows []sharedevents.OutboxRow
	if err := db.Where("sent_at IS NULL").Find(&rows).Error; err != nil {
		t.Fatalf("read outbox: %v", err)
	}
	return len(rows)
}

// A document needs no worker, so Confirm takes it straight to ready and
// publishes nothing. The client's poll-until-ready loop therefore resolves on
// the first read rather than waiting on a worker that would never run
// (design D12, api-contracts §6).
func TestConfirm_documentGoesStraightToReadyWithNoOutboxRow(t *testing.T) {
	_, proc, db := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "application/pdf", "invoice.pdf")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}

	confirmed, err := proc.Confirm(context.Background(), created.ID(), "fleet-a")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.Status() != StatusReady {
		t.Fatalf("status = %q, want ready", confirmed.Status())
	}
	if n := countOutboxRows(t, db); n != 0 {
		t.Fatalf("outbox rows = %d, want 0 — a document must not enqueue media.uploaded", n)
	}
}

// The image path is unchanged: uploaded → processing plus exactly one outbox row.
func TestConfirm_imageStillEnqueuesProcessing(t *testing.T) {
	_, proc, db := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}

	confirmed, err := proc.Confirm(context.Background(), created.ID(), "fleet-a")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.Status() != StatusProcessing {
		t.Fatalf("status = %q, want processing", confirmed.Status())
	}
	if n := countOutboxRows(t, db); n != 1 {
		t.Fatalf("outbox rows = %d, want 1", n)
	}
}

// A legacy row whose stored type nobody recognises must never be handed to
// image.Decode, so it confirms like a document (design D12).
func TestConfirm_unknownContentTypeConfirmsLikeADocument(t *testing.T) {
	_, proc, db := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "image/png", "legacy.png")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}
	if err := db.Model(&Entity{}).Where("id = ?", created.ID()).
		Update("content_type", "application/x-legacy").Error; err != nil {
		t.Fatalf("force content type: %v", err)
	}

	confirmed, err := proc.Confirm(context.Background(), created.ID(), "fleet-a")
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	if confirmed.Status() != StatusReady {
		t.Fatalf("status = %q, want ready", confirmed.Status())
	}
	if n := countOutboxRows(t, db); n != 0 {
		t.Fatalf("outbox rows = %d, want 0", n)
	}
}
```

Add `"context"` and the `sharedevents` import
(`sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"` — match the
alias `processor_test.go` already uses) plus `"gorm.io/gorm"` to `resource_test.go`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run TestConfirm -v`
Expected: the document test FAILs — status is `processing` and one outbox row exists.

- [ ] **Step 3: Branch `Confirm` on class**

In `apps/media-service/internal/mediaobject/processor.go`, replace the body of `Confirm`
from the `MarkProcessing` call onwards:

```go
	// Classification decides everything downstream (design §2). ClassUnknown is
	// deliberately folded in with documents: a pre-allowlist row whose content
	// type nobody recognises must never be handed to image.Decode. Legacy
	// JPEG/PNG rows still classify as ClassImage — their stored type is on the
	// allowlist — so nothing regresses.
	if pr.allow.Classify(m.ContentType()) != ClassImage {
		ready, err := MarkReadyDirect(m)
		if err != nil {
			return Model{}, err
		}
		return pr.a.Update(ready)
	}

	processing, err := MarkProcessing(m)
	if err != nil {
		return Model{}, err
	}
```

Everything from the existing `// Build the envelope before opening the transaction…` comment
onwards — `env := events.Envelope{…}`, the `pr.a.UpdateInTx(processing, …)` call and the
`return updated, nil` — stays byte-for-byte as it is. The only edit to `Confirm` is
inserting the classification branch above and leaving the rest untouched.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -v 2>&1 | tail -30`
Expected: PASS, including the pre-existing `TestConfirm_enqueuesOutboxAtomically` and
`TestConfirm_outboxRollsBackOnEnqueueError` (both use JPEG models, so they take the
unchanged image path).

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/mediaobject/processor.go apps/media-service/internal/mediaobject/processor_test.go
git commit -m "feat(media-service): confirm documents straight to ready with no processing event"
```

---

## Task 7: harden `GET /media/{id}/content`

**Files:**
- Modify: `apps/media-service/internal/mediaobject/resource.go` (download handler)
- Test: `apps/media-service/internal/mediaobject/resource_test.go`

**Interfaces:**
- Consumes: `Allowlist.Resolve` (Task 2), `ContentDisposition` (Task 3).
- Produces: no new symbols. Response headers per api-contracts §7.

- [ ] **Step 1: Write the failing tests**

Append to `apps/media-service/internal/mediaobject/resource_test.go`. Follow the setup in
the existing `TestGetContent_presentObjectStreams200` for putting bytes in the fake store:

```go
func TestGetContent_pdfIsAttachmentWithNosniff(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("%PDF-1.7")}
	router, proc, _ := testRouter(t, store, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "application/pdf", "invoice.pdf")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+created.ID()+"/content", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="invoice.pdf"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "private, max-age=300" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

// inline + a correct image/jpeg still renders in an <img>; nosniff only stops
// the browser second-guessing the declared type.
func TestGetContent_jpegIsInlineWithNosniff(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("\xff\xd8\xff")}
	router, proc, _ := testRouter(t, store, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+created.ID()+"/content", nil))

	if got := rec.Header().Get("Content-Disposition"); got != `inline; filename="photo.jpg"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

// Rows created before the allowlist existed may hold arbitrary strings. They
// are served as octet-stream + attachment (PRD FR-DL-4). InitUpload can no
// longer create such a row, so the row is written directly.
func TestGetContent_legacyContentTypeIsOctetStreamAttachment(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("<script>alert(1)</script>")}
	router, proc, db := testRouter(t, store, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "image/png", "legacy.png")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}
	// Simulate a pre-allowlist row by rewriting the stored type behind the
	// processor's back, exactly as an old row in the database would look.
	forceContentType(t, db, created.ID(), "text/html")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+created.ID()+"/content", nil))

	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment;") {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
}
```

Add the helper beside them (Task 5 already made `testRouter` return the `*gorm.DB`):

```go
// forceContentType rewrites a stored content type directly, which is the only
// way to reproduce a pre-allowlist row now that InitUpload normalises.
func forceContentType(t *testing.T, db *gorm.DB, id, contentType string) {
	t.Helper()
	if err := db.Model(&Entity{}).Where("id = ?", id).
		Update("content_type", contentType).Error; err != nil {
		t.Fatalf("force content type: %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run TestGetContent -v`
Expected: FAIL — no `Content-Disposition`, no `X-Content-Type-Options`, and the legacy row
serves `text/html`.

- [ ] **Step 3: Rewrite the response headers**

In `apps/media-service/internal/mediaobject/resource.go`, inside the
`r.Get("/media/{id}/content", …)` handler, replace the header block (currently the
`if ct := m.ContentType(); ct != "" { … }` through the `Cache-Control` line) with:

```go
				// The Content-Type is re-resolved through the allowlist on every
				// read rather than trusting the stored value, so shrinking the
				// allowlist retroactively downgrades already-stored objects and
				// rows created before the allowlist existed are covered too
				// (design D15, PRD FR-DL-4).
				ct, class := allow.Resolve(m.ContentType())
				w.Header().Set("Content-Type", ct)
				// nosniff on EVERY response, both classes (PRD FR-DL-1). Together
				// with attachment on documents this is what prevents an uploaded
				// file from executing in the application's origin; neither alone
				// is sufficient.
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Content-Disposition",
					ContentDisposition(class, m.OriginalFilename(), m.ID()))
				if size := m.Size(); size > 0 {
					w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
				}
				// Per-fleet authorized bytes — never store in a shared cache.
				w.Header().Set("Cache-Control", "private, max-age=300")
```

`allow` is already in scope: it is the `InitializeRoutes` parameter added in Task 5.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -v 2>&1 | tail -30`
Expected: PASS, including the pre-existing `TestGetContent_presentObjectStreams200`,
`TestGetContent_missingObjectIs404NotEmpty200` and `TestGetContent_crossFleetIs404`.

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/mediaobject/resource.go apps/media-service/internal/mediaobject/resource_test.go
git commit -m "feat(media-service): add Content-Disposition and nosniff to media download"
```

---

## Task 8: permanent vs transient processing failures

**Files:**
- Modify: `apps/media-service/internal/processing/worker.go`
- Test: `apps/media-service/internal/processing/worker_test.go`

**Interfaces:**
- Consumes: `mediaobject.MarkFailed`, `mediaobject.StatusFailed` (Task 4).
- Produces: `processing.ErrPermanent`. Not consumed elsewhere; it exists so `handle` can
  distinguish a decode failure from a storage failure.

**Why this shape:** the PRD says "after the existing consumer retry budget is exhausted".
There is no retry budget — `packages/shared-go/events/consumer.go` `continue`s without
committing, so a failing handler loops forever *and blocks the partition for every other
media object behind it*. Adding a budget would mean touching a consumer shared by four
services. Retrying only errors that can plausibly succeed later is both smaller and better
(design D13).

- [ ] **Step 1: Write the failing tests**

Append to `apps/media-service/internal/processing/worker_test.go`, reusing the helpers the
file already defines (`newWorkerTestDB`, `buildProcessingObj`, `fakeObjectStore`,
`fakeProvider`, `fakeObjectAdmin`, `fakeVariantAdmin`). The transient case is already
covered by `TestHandle_failedVariantGeneration_doesNotMarkProcessed`, so these two tests
cover only the permanent side of the split:

```go
// bytesStore returns fixed bytes from GetObject so the decode path can be
// exercised with content that is not a valid image.
type bytesStore struct{ data []byte }

func (b *bytesStore) GetObject(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b.data)), nil
}

func (b *bytesStore) PutObject(_ context.Context, _ string, _ io.Reader, _ int64, _ string) error {
	return nil
}

// A corrupt or mislabelled file will never become decodable, so retrying it
// forever blocks the partition for every other object behind it. It must reach
// a terminal state AND have its event recorded (PRD FR-MEDIA-5, design D13).
func TestHandle_undecodableBytesMarkFailedAndProcessed(t *testing.T) {
	db := newWorkerTestDB(t)
	dedupe := processedevents.New(logrus.New(), db)

	obj := buildProcessingObj(t)
	objStore := &bytesStore{data: []byte("this is definitely not a jpeg")}
	objAdmin := &fakeObjectAdmin{}

	worker := NewWorker(logrus.New(), objStore, &fakeProvider{m: obj}, objAdmin, &fakeVariantAdmin{}, dedupe)

	env := events.Envelope{
		EventID: "evt-undecodable",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-1"},
	}

	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle returned %v; a permanent failure must be committed, not retried", err)
	}

	if len(objAdmin.updated) != 1 || objAdmin.updated[0].Status() != mediaobject.StatusFailed {
		t.Fatalf("object was not moved to failed: %+v", objAdmin.updated)
	}

	recorded, err := dedupe.Exists("evt-undecodable")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !recorded {
		t.Fatal("event was not marked processed; it would redeliver forever and block the partition")
	}
}

// An original whose bytes were never stored is permanent too: the PUT that
// would have created them already failed and will not retry itself.
func TestHandle_missingOriginalMarksFailedAndProcessed(t *testing.T) {
	db := newWorkerTestDB(t)
	dedupe := processedevents.New(logrus.New(), db)

	obj := buildProcessingObj(t)
	objStore := &fakeObjectStore{errGet: storage.ErrObjectNotFound}
	objAdmin := &fakeObjectAdmin{}

	worker := NewWorker(logrus.New(), objStore, &fakeProvider{m: obj}, objAdmin, &fakeVariantAdmin{}, dedupe)

	env := events.Envelope{
		EventID: "evt-missing-original",
		Type:    mediaobject.EventTypeMediaUploaded,
		Data:    map[string]any{"media_id": "media-1"},
	}

	if err := worker.handle(context.Background(), env); err != nil {
		t.Fatalf("handle returned %v; a missing original is permanent", err)
	}
	if len(objAdmin.updated) != 1 || objAdmin.updated[0].Status() != mediaobject.StatusFailed {
		t.Fatalf("object was not moved to failed: %+v", objAdmin.updated)
	}
	recorded, err := dedupe.Exists("evt-missing-original")
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !recorded {
		t.Fatal("event was not marked processed")
	}
}
```

Add `"bytes"` and the `storage` package
(`"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"`) to the test file's
import block. `processedevents` and `logrus` are already imported.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/processing/... -run 'TestHandle_undecodableBytes|TestHandle_missingOriginal' -v`
Expected: both FAIL — `handle` returns a non-nil error and nothing is marked failed or
processed.

- [ ] **Step 3: Classify the failures in `worker.go`**

Add the sentinel near the top of `apps/media-service/internal/processing/worker.go`:

```go
// ErrPermanent marks a processing failure that cannot plausibly succeed on a
// later delivery. events.Consume has no retry budget — it re-delivers without
// committing until the handler stops erroring — so returning an error for a
// file that will never decode blocks the partition for every other media object
// behind it. Wrapping permanent failures lets handle commit them instead
// (design D13).
var ErrPermanent = errors.New("permanent processing failure")
```

Add `"errors"` to the import block.

Change `decodeOriginal` to classify:

```go
// decodeOriginal downloads and decodes the original image (jpeg or png).
// Both failure modes it can produce are permanent: bytes that do not decode
// will not start decoding, and an original that was never stored will not
// appear. Everything else (a transport error from the store, for instance)
// passes through unwrapped and stays retryable.
func (w *Worker) decodeOriginal(ctx context.Context, key string) (image.Image, error) {
	rc, err := w.store.GetObject(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrObjectNotFound) {
			return nil, fmt.Errorf("%w: original bytes were never stored: %w", ErrPermanent, err)
		}
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	img, _, err := image.Decode(rc)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPermanent, err)
	}
	return img, nil
}
```

In `handle`, replace the `decodeOriginal` error branch:

```go
	src, err := w.decodeOriginal(ctx, obj.ObjectKey())
	if err != nil {
		if errors.Is(err, ErrPermanent) {
			return w.failPermanently(e, obj, err)
		}
		return fmt.Errorf("decode original %s: %w", obj.ObjectKey(), err)
	}
```

and add the helper at the end of the file:

```go
// failPermanently moves the object to the terminal failed state and records the
// event as processed, so a file that can never be decoded does not redeliver
// forever. It returns nil on success: the delivery is complete, just not
// successfully, and committing the offset is the whole point.
func (w *Worker) failPermanently(e events.Envelope, obj mediaobject.Model, cause error) error {
	w.log.WithField("media_id", obj.ID()).WithError(cause).
		Error("media processing failed permanently; marking object failed")

	failed, err := mediaobject.MarkFailed(obj)
	if err != nil {
		// The object is in a state MarkFailed rejects (already ready or already
		// failed). Nothing left to do, and retrying will not change it.
		w.log.WithField("media_id", obj.ID()).WithError(err).
			Warn("cannot mark media object failed; recording event anyway")
	} else if _, err := w.objectAdmin.Update(failed); err != nil {
		// Persisting the terminal state is a database failure, which IS
		// transient — retry rather than committing a half-applied outcome.
		return fmt.Errorf("persist failed media object: %w", err)
	}

	if _, err := w.dedupe.MarkProcessed(e.EventID); err != nil {
		return fmt.Errorf("record processed event (permanent failure): %w", err)
	}
	return nil
}
```

Leave the variant-generation and `ReplaceForMediaObject` error paths untouched: a MinIO or
database failure there is transient and must keep redelivering.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/media-service && go test ./internal/processing/... -v`
Expected: PASS, including the pre-existing
`TestHandle_failedVariantGeneration_doesNotMarkProcessed`,
`TestHandle_alreadyReady_marksProcessedAndSkipsVariants`,
`TestHandle_alreadyProcessed_skipsWithoutWork` and the four `TestResizeDims_*` tests.

- [ ] **Step 5: Commit**

```bash
git add apps/media-service/internal/processing
git commit -m "fix(media-service): commit permanent processing failures instead of blocking the partition"
```

---

## Task 9: `GET /internal/media` batch ownership lookup

**Files:**
- Modify: `apps/media-service/internal/mediaobject/provider.go`
- Modify: `apps/media-service/internal/mediaobject/rest.go`
- Modify: `apps/media-service/internal/mediaobject/resource.go`
- Modify: `apps/media-service/cmd/main.go`
- Modify: `apps/media-service/internal/processing/worker_test.go` (`fakeProvider`)
- Test: `apps/media-service/internal/mediaobject/resource_test.go`

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `Provider.ListActiveByFleetAndIDs(fleetID string, ids []string) ([]Model, error)`
  - `type InternalMedia struct{ ID, Status, ContentType string }` with snake_case JSON tags
  - `type InternalMediaResponse struct{ Media []InternalMedia `json:"media"` }`
  - `func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router)`
  - `const MaxInternalLookupIDs = 50`
  Consumed by Task 15 (`mediaclient`) over HTTP, not by import.

- [ ] **Step 1: Write the failing tests**

Append to `apps/media-service/internal/mediaobject/resource_test.go`:

```go
// internalRouter mounts the no-JWT internal routes over the same DB the
// authenticated router uses, so a test can create objects through the processor
// and then query them the way fleet-service will.
func internalRouter(t *testing.T, db *gorm.DB) http.Handler {
	t.Helper()
	log := logrus.New()
	log.SetOutput(io.Discard)
	r := chi.NewRouter()
	r.Group(InitializeInternalRoutes(log, db))
	return r
}

// getInternal issues an unauthenticated GET — this route has no JWT middleware,
// so no identity is attached to the context.
func getInternal(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// The response contains only the requested IDs that are active AND in the
// fleet. Whether a missing ID does not exist, was deleted, or belongs to
// someone else is indistinguishable — that non-disclosure property falls out of
// the endpoint's shape rather than needing handler-side care (design D6).
func TestInternalMedia_returnsOnlySameFleetActiveIDs(t *testing.T) {
	_, proc, db := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)
	h := internalRouter(t, db)

	mine, err := proc.InitUpload("fleet-a", "u1", "application/pdf", "a.pdf")
	if err != nil {
		t.Fatalf("InitUpload(mine): %v", err)
	}
	theirs, err := proc.InitUpload("fleet-b", "u2", "application/pdf", "b.pdf")
	if err != nil {
		t.Fatalf("InitUpload(theirs): %v", err)
	}
	deleted, err := proc.InitUpload("fleet-a", "u1", "application/pdf", "c.pdf")
	if err != nil {
		t.Fatalf("InitUpload(deleted): %v", err)
	}
	if err := proc.SoftDelete(deleted.ID(), "fleet-a"); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	ids := strings.Join([]string{mine.ID(), theirs.ID(), deleted.ID(), "does-not-exist"}, ",")
	rec := getInternal(t, h, "/internal/media?fleet_id=fleet-a&ids="+ids)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got InternalMediaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Media) != 1 || got.Media[0].ID != mine.ID() {
		t.Fatalf("got %+v, want exactly the caller's own active object", got.Media)
	}
	if got.Media[0].ContentType != "application/pdf" {
		t.Fatalf("content_type = %q", got.Media[0].ContentType)
	}
}

func TestInternalMedia_missingFleetIDIs422(t *testing.T) {
	_, _, db := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	rec := getInternal(t, internalRouter(t, db), "/internal/media?ids=x")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestInternalMedia_emptyIdsReturnsEmptyList(t *testing.T) {
	_, _, db := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	rec := getInternal(t, internalRouter(t, db), "/internal/media?fleet_id=fleet-a")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var got InternalMediaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Media) != 0 {
		t.Fatalf("got %+v, want an empty list", got.Media)
	}
}

// The endpoint is unauthenticated, so its input must be bounded.
func TestInternalMedia_tooManyIdsIs422(t *testing.T) {
	_, _, db := testRouter(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	ids := make([]string, MaxInternalLookupIDs+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	rec := getInternal(t, internalRouter(t, db), "/internal/media?fleet_id=fleet-a&ids="+strings.Join(ids, ","))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}
```

Add `"encoding/json"` and `"fmt"` to the test file's imports (`strings`, `net/http`,
`net/http/httptest`, `chi`, `logrus`, `io` and `gorm` are already there after Task 5).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/media-service && go test ./internal/mediaobject/... -run TestInternalMedia -v`
Expected: FAIL — compile error, `undefined: InitializeInternalRoutes`.

- [ ] **Step 3: Add the provider method**

In `apps/media-service/internal/mediaobject/provider.go`:

```go
type Provider interface {
	GetByID(id string) (Model, error)
	GetByIDIncludingDeleted(id string) (Model, error)
	// ListActiveByFleetAndIDs returns the subset of ids that are active (not
	// soft-deleted) AND belong to fleetID. fleetID is a filter, never a trusted
	// assertion: the result set is never widened on the caller's say-so.
	ListActiveByFleetAndIDs(fleetID string, ids []string) ([]Model, error)
}

func (p *dbProvider) ListActiveByFleetAndIDs(fleetID string, ids []string) ([]Model, error) {
	if len(ids) == 0 {
		return []Model{}, nil
	}
	var es []Entity
	if err := p.db.Where("fleet_id = ? AND deleted_at IS NULL AND id IN ?", fleetID, ids).
		Find(&es).Error; err != nil {
		return nil, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, nil
}
```

Then add the same method to `fakeProvider` in
`apps/media-service/internal/processing/worker_test.go` so the interface is still satisfied:

```go
func (f *fakeProvider) ListActiveByFleetAndIDs(_ string, _ []string) ([]mediaobject.Model, error) {
	return nil, nil
}
```

- [ ] **Step 4: Add the flat internal payload**

Append to `apps/media-service/internal/mediaobject/rest.go`:

```go
// InternalMedia is the flat (deliberately NOT JSON:API) payload the
// network-restricted GET /internal/media returns, matching the shape
// fleet-service's other internal clients already consume.
type InternalMedia struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ContentType string `json:"content_type"`
}

// InternalMediaResponse wraps the list so the payload can gain fields later
// without becoming a breaking change for the client.
type InternalMediaResponse struct {
	Media []InternalMedia `json:"media"`
}

// TransformInternalMedia converts Models to the internal payload.
func TransformInternalMedia(ms []Model) []InternalMedia {
	out := make([]InternalMedia, 0, len(ms))
	for _, m := range ms {
		out = append(out, InternalMedia{
			ID:          m.ID(),
			Status:      string(m.Status()),
			ContentType: m.ContentType(),
		})
	}
	return out
}
```

- [ ] **Step 5: Add the route**

Append to `apps/media-service/internal/mediaobject/resource.go`:

```go
// MaxInternalLookupIDs bounds a single /internal/media lookup. The endpoint is
// unauthenticated, so its input must be bounded; fleet-service's own
// maintenancerecord.MaxDocuments (10) is the authoritative per-record cap and
// this is the defensive ceiling behind it.
const MaxInternalLookupIDs = 50

// InitializeInternalRoutes wires the network-restricted internal endpoint.
// Register this initializer WITHOUT JWT middleware, exactly as fleet-service
// does for membership.InitializeInternalRoutes.
//
// GET /internal/media?fleet_id=<uuid>&ids=<id>,<id>,… returns the requested IDs
// that are active AND belong to fleet_id. fleet-service compares the returned
// set against the requested set to prove a record's documentMediaIds are its
// caller's own media (design D6, PRD FR-DOC-6). A missing ID is
// indistinguishable between "does not exist", "was deleted" and "belongs to
// another fleet" — which is the non-disclosure property api-contracts §3 asks
// for, and it falls out of the shape rather than needing handler-side care.
//
// SECURITY: this route has no authentication. The priority-200 `internal-deny`
// rule in deploy/k8s/overlays/main/ingressroute.yaml is what keeps it off the
// public internet (design D20). Without it this is an unauthenticated
// cross-fleet media-existence oracle. The two ship together; never separately.
func InitializeInternalRoutes(log logrus.FieldLogger, db *gorm.DB) func(chi.Router) {
	prov := NewProvider(db)
	return func(r chi.Router) {
		r.Get("/internal/media", func(w http.ResponseWriter, req *http.Request) {
			fleetID := req.URL.Query().Get("fleet_id")
			if fleetID == "" {
				server.WriteError(w, server.ErrValidation)
				return
			}

			ids := splitInternalIDs(req.URL.Query().Get("ids"))
			if len(ids) > MaxInternalLookupIDs {
				server.WriteError(w, server.ErrValidation)
				return
			}
			if len(ids) == 0 {
				server.WriteJSON(w, http.StatusOK, InternalMediaResponse{Media: []InternalMedia{}})
				return
			}

			ms, err := prov.ListActiveByFleetAndIDs(fleetID, ids)
			if err != nil {
				log.WithError(err).Error("internal media lookup")
				server.WriteError(w, err)
				return
			}
			server.WriteJSON(w, http.StatusOK, InternalMediaResponse{Media: TransformInternalMedia(ms)})
		})
	}
}

// splitInternalIDs parses the comma-separated ids parameter, dropping empty
// segments so a trailing comma or a doubled separator is not a lookup for "".
func splitInternalIDs(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out
}
```

Add `"strings"` to the import block.

- [ ] **Step 6: Register it in `main.go`**

In `apps/media-service/cmd/main.go`, add an initializer **outside** the JWT group,
mirroring fleet-service's wiring:

```go
		// Internal route: no JWT, network-restricted (consumed by fleet-service
		// to validate documentMediaIds). Kept off the public internet by the
		// priority-200 internal-deny rule in the main overlay's ingressroute.
		AddRouteInitializer(mediaobject.InitializeInternalRoutes(log, db)).
```

Place it immediately before the existing `AddRouteInitializer(func(r chi.Router) { r.Group(...JWT...) })`.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `cd apps/media-service && go build ./... && go test ./... 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/media-service
git commit -m "feat(media-service): add internal batch media ownership lookup"
```

---

## Task 10: deny media-service's `/internal` surface at the edge

**Files:**
- Modify: `deploy/k8s/overlays/main/ingressroute.yaml`
- Modify: `deploy/k8s/base/fleet-service/configmap.yaml`
- Modify: `deploy/compose/docker-compose.yml`, `deploy/compose/.env.example`

**Interfaces:**
- Consumes: the route added in Task 9.
- Produces: `MEDIA_INTERNAL_URL` config key for fleet-service, consumed by Task 16.

**This is security-critical and non-optional.** Without the deny rule, Task 9's endpoint is
an unauthenticated cross-fleet media-existence oracle on the public internet (design D20).

- [ ] **Step 1: Add the mirrored deny route**

In `deploy/k8s/overlays/main/ingressroute.yaml`, in the `myfleet-routes` object's `routes:`
list, immediately after the existing fleet-service priority-200 rule and before the
`/api/auth` rule:

```yaml
    # Same obligation as the fleet-service rule above, for the same reasons.
    # media-service registers GET /internal/media WITHOUT JWT
    # (apps/media-service/cmd/main.go) on the assumption that it is only
    # reachable from inside the cluster; exposing media-service on the internet
    # invalidates that assumption, and /internal/media is a cross-fleet
    # media-existence oracle for anyone who can guess a fleet ID.
    #
    # PathRegexp, not PathPrefix: Traefik normalises the path before matching,
    # and media-stripprefix removes the literal string `/api` rather than a path
    # segment, so `/api/mediainternal/media?...` would otherwise reach the
    # handler as `/internal/media`. Hence `[^/]*/*` — the separator is optional.
    #
    # The service reference is inert: internal-deny short-circuits with 403
    # before anything is proxied.
    - match: (Host(`myfleet.tumidanski.com`) || Host(`myfleet.tumidanski.me`) || Host(`myfleet.home`)) && PathRegexp(`(?i)^/+api/+media[^/]*/*internal`)
      kind: Rule
      priority: 200
      middlewares:
        - name: internal-deny
      services:
        - name: media-service
          port: 8080
```

Do **not** touch `myfleet-routes-tls` — its `routes: []` is populated by the kustomize
`replacements` block, which is what makes the two entrypoints unable to drift.

- [ ] **Step 2: Add `MEDIA_INTERNAL_URL` for fleet-service**

`deploy/k8s/base/fleet-service/configmap.yaml` — append:

```yaml
  # media-service's network-restricted internal API, used to prove a
  # maintenance record's documentMediaIds belong to the caller's fleet
  # (PRD FR-DOC-6). Cluster-internal ClusterVIP; never traverses the edge.
  MEDIA_INTERNAL_URL: "http://media-service:8080"
```

`deploy/compose/docker-compose.yml`, in the `fleet-service` `environment:` block after
`KAFKA_BROKERS`:

```yaml
      MEDIA_INTERNAL_URL: http://media-service:8080
```

and add `media-service` to fleet-service's `depends_on` is **not** required — fleet-service
only calls media-service on a request that carries attachments, and a hard dependency would
create a startup ordering constraint for no benefit. Leave `depends_on` alone.

`deploy/compose/.env.example`, next to the existing `FLEET_INTERNAL_URL` line:

```
MEDIA_INTERNAL_URL=http://media-service:8080
```

- [ ] **Step 3: Verify both overlays render and the rule is present on both entrypoints**

```bash
make manifests
kustomize build deploy/k8s/overlays/main | grep -c 'api/+media\[\^/\]\*/\*internal'
```

Expected: `make manifests` succeeds; the grep prints `2` — once in `myfleet-routes` and
once in the TLS twin that `replacements` populates. If it prints `1`, the replacement did
not copy the route and the rule is missing from an entrypoint; stop and fix that before
continuing.

```bash
kustomize build deploy/k8s/overlays/local | head -5
```

Expected: renders cleanly. The local overlay needs no equivalent rule — it has no
`internal-deny` for fleet-service either and is not internet-facing.

- [ ] **Step 4: Commit**

```bash
git add deploy/k8s/overlays/main/ingressroute.yaml deploy/k8s/base/fleet-service/configmap.yaml deploy/compose/docker-compose.yml deploy/compose/.env.example
git commit -m "feat(deploy): deny media-service /internal at the edge and wire MEDIA_INTERNAL_URL"
```

---

## Task 11: category `kind` — model, entity, seeds, transport

**Files:**
- Modify: `apps/fleet-service/internal/maintenancecategory/model.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/entity.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/rest.go`
- Test: `apps/fleet-service/internal/maintenancecategory/entity_test.go`

**Interfaces:**
- Consumes: `server.ErrValidation`.
- Produces, in package `maintenancecategory`:
  - `type Kind string`, `KindMaintenance`, `KindModification`
  - `func ParseKind(s string) (Kind, error)`
  - `Model.Kind() Kind`
  - `Entity.Kind string`
  - `Attributes.Kind string` (JSON `kind`, no `omitempty`)
  Consumed by Tasks 12, 16, 17.

**Deviation from design D1:** the GORM tag is `default:'maintenance'` (quoted). GORM copies
the tag value verbatim into the DDL, so the unquoted `default:maintenance` emits
`DEFAULT maintenance`, which PostgreSQL reads as a column reference and rejects.

- [ ] **Step 1: Write the failing tests**

Append to `apps/fleet-service/internal/maintenancecategory/entity_test.go`:

```go
// ParseKind is the single place the permitted ?kind= values are defined, so the
// category and record endpoints cannot drift on what they accept.
func TestParseKind(t *testing.T) {
	if k, err := ParseKind(""); err != nil || k != "" {
		t.Fatalf(`ParseKind("")=(%q,%v) want ("",nil) — empty means "no filter"`, k, err)
	}
	if k, err := ParseKind("maintenance"); err != nil || k != KindMaintenance {
		t.Fatalf("ParseKind(maintenance)=(%q,%v)", k, err)
	}
	if k, err := ParseKind("modification"); err != nil || k != KindModification {
		t.Fatalf("ParseKind(modification)=(%q,%v)", k, err)
	}
	// An unrecognised value is a validation error, never a silent empty result.
	for _, in := range []string{"bogus", "MAINTENANCE", "mod", " maintenance"} {
		if _, err := ParseKind(in); !errors.Is(err, server.ErrValidation) {
			t.Fatalf("ParseKind(%q) err = %v, want ErrValidation", in, err)
		}
	}
}

// The eight pre-existing rows must read as maintenance after migration with no
// manual backfill; the twelve modification categories must seed alongside them.
func TestSeed_classifiesEveryCategory(t *testing.T) {
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	var maintenance, modification int64
	if err := db.Model(&Entity{}).Where("kind = ?", string(KindMaintenance)).
		Count(&maintenance).Error; err != nil {
		t.Fatalf("count maintenance: %v", err)
	}
	if err := db.Model(&Entity{}).Where("kind = ?", string(KindModification)).
		Count(&modification).Error; err != nil {
		t.Fatalf("count modification: %v", err)
	}

	if maintenance != 8 {
		t.Fatalf("maintenance categories = %d, want 8", maintenance)
	}
	if modification != 12 {
		t.Fatalf("modification categories = %d, want 12", modification)
	}
	if int(maintenance+modification) != len(seeds) {
		t.Fatalf("kinds sum to %d but seeds has %d rows", maintenance+modification, len(seeds))
	}
}

// No modification name may collide with an existing maintenance name — Seed is
// keyed by Name, so a collision would silently reclassify nothing and leave a
// category missing.
func TestSeed_namesAreUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range seeds {
		if seen[s.Name] {
			t.Fatalf("duplicate seed name %q", s.Name)
		}
		seen[s.Name] = true
	}
}
```

Add `"errors"` and `"github.com/jtumidanski/myfleet/packages/shared-go/server"` to the test
file's imports.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/fleet-service && go test ./internal/maintenancecategory/... -v`
Expected: FAIL — compile error, `undefined: ParseKind`.

- [ ] **Step 3: Add `Kind` to the model**

In `apps/fleet-service/internal/maintenancecategory/model.go`:

```go
package maintenancecategory

import "github.com/jtumidanski/myfleet/packages/shared-go/server"

// Kind discriminates repair/service work from modifications. It lives on the
// category rather than on the record so a record's kind cannot disagree with
// its category's — the category a record points at is the single source of
// truth, and a record stores no kind at all (design D1).
type Kind string

const (
	KindMaintenance  Kind = "maintenance"
	KindModification Kind = "modification"
)

// ParseKind maps a ?kind= query-parameter value to a Kind. The empty string
// means "no filter" and yields ("", nil); anything else unrecognised is a
// validation error, never a silent empty result (PRD FR-KIND-4).
func ParseKind(s string) (Kind, error) {
	switch Kind(s) {
	case "":
		return "", nil
	case KindMaintenance:
		return KindMaintenance, nil
	case KindModification:
		return KindModification, nil
	default:
		return "", server.ErrValidation
	}
}

// Model is the immutable maintenance category domain model. Categories are
// global/system data (not fleet-scoped); see design §8.2.
type Model struct {
	id            string
	name          string
	description   string
	systemDefined bool
	kind          Kind
}

func (m Model) ID() string          { return m.id }
func (m Model) Name() string        { return m.name }
func (m Model) Description() string { return m.description }
func (m Model) SystemDefined() bool { return m.systemDefined }
func (m Model) Kind() Kind          { return m.kind }
```

- [ ] **Step 4: Add the column and the twelve seeds**

In `apps/fleet-service/internal/maintenancecategory/entity.go`:

```go
// Entity maps to fleet.maintenance_categories (PRD §6, design §8.2).
type Entity struct {
	ID            string `gorm:"type:uuid;primaryKey"`
	Name          string `gorm:"not null"`
	Description   string
	SystemDefined bool `gorm:"not null;default:false"`
	// The DEFAULT is what classifies the eight pre-existing rows in the same
	// ALTER TABLE that adds the column — no backfill step (PRD FR-KIND-1).
	// The literal is quoted because GORM copies the tag value verbatim into the
	// DDL; unquoted, PostgreSQL reads `maintenance` as a column reference.
	Kind string `gorm:"type:varchar(20);not null;default:'maintenance'"`
}
```

Update `Make`:

```go
func Make(e Entity) Model {
	return Model{
		id:            e.ID,
		name:          e.Name,
		description:   e.Description,
		systemDefined: e.SystemDefined,
		kind:          Kind(e.Kind),
	}
}
```

Replace `seeds` with the full twenty:

```go
// seeds is the canonical list of system-defined categories (FR-MAINT-1,
// FR-KIND-2). Seeding is keyed by Name so it is idempotent; no modification
// name collides with a maintenance one.
var seeds = []Entity{
	{Name: "Oil Change", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Tire Rotation", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Brake Service", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Air Filter", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Transmission Service", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Coolant Flush", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Battery", SystemDefined: true, Kind: string(KindMaintenance)},
	{Name: "Inspection", SystemDefined: true, Kind: string(KindMaintenance)},

	{Name: "Performance / Tune", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Suspension", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Wheels & Tires", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Exhaust", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Intake", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Brake Upgrade", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Exterior / Body", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Interior", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Audio & Electronics", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Lighting", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Towing", SystemDefined: true, Kind: string(KindModification)},
	{Name: "Other Modification", SystemDefined: true, Kind: string(KindModification)},
}
```

and carry `Kind` in `Seed`'s `Attrs` literal so existing rows are untouched while new ones
are classified:

```go
			if err := db.Where("name = ?", s.Name).
				Attrs(Entity{
					ID:            uuid.NewString(),
					Name:          s.Name,
					Description:   s.Description,
					SystemDefined: s.SystemDefined,
					Kind:          s.Kind,
				}).
				FirstOrCreate(&e).Error; err != nil {
```

- [ ] **Step 5: Expose `kind` on the wire**

In `apps/fleet-service/internal/maintenancecategory/rest.go`:

```go
type Attributes struct {
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	SystemDefined bool   `json:"systemDefined"`
	// No omitempty: the column is NOT NULL, so kind is always present and
	// never null (api-contracts §1).
	Kind string `json:"kind"`
}
```

and in `Transform`, add `Kind: string(m.Kind()),`.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd apps/fleet-service && go test ./internal/maintenancecategory/... -v`
Expected: PASS — `TestSeedIsIdempotent` (unchanged; it asserts `len(seeds)`, now 20),
`TestParseKind`, `TestSeed_classifiesEveryCategory`, `TestSeed_namesAreUnique`.

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service/internal/maintenancecategory
git commit -m "feat(fleet-service): add kind to maintenance categories and seed modifications"
```

---

## Task 12: category `kind` filter on the provider and the route

**Files:**
- Modify: `apps/fleet-service/internal/maintenancecategory/provider.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/processor.go`
- Modify: `apps/fleet-service/internal/maintenancecategory/resource.go`
- Create: `apps/fleet-service/internal/maintenancecategory/provider_test.go`

**Interfaces:**
- Consumes: `Kind`, `ParseKind` (Task 11).
- Produces:
  - `Provider.List(kind Kind, page server.Page) ([]Model, int, error)`
  - `Provider.IDsByKind(kind Kind) ([]string, error)`
  - `(*Processor).List(kind Kind, page server.Page) ([]Model, int, error)`
  - `(*Processor).IDsByKind(kind Kind) ([]string, error)` — this is what satisfies
    `maintenancerecord.CategoryAccessor` in Task 16.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/maintenancecategory/provider_test.go`:

```go
package maintenancecategory

import (
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func seededProvider(t *testing.T) Provider {
	t.Helper()
	db := newTestDB(t)
	if err := Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return NewProvider(db)
}

func TestList_filtersByKindAndCountsAfterFiltering(t *testing.T) {
	p := seededProvider(t)
	page := server.Page{Number: 1, Size: 100}

	all, allTotal, err := p.List("", page)
	if err != nil {
		t.Fatalf("List(all): %v", err)
	}
	if allTotal != 20 || len(all) != 20 {
		t.Fatalf("unfiltered: len=%d total=%d, want 20/20", len(all), allTotal)
	}

	mods, modTotal, err := p.List(KindModification, page)
	if err != nil {
		t.Fatalf("List(modification): %v", err)
	}
	if modTotal != 12 || len(mods) != 12 {
		t.Fatalf("modification: len=%d total=%d, want 12/12", len(mods), modTotal)
	}
	for _, m := range mods {
		if m.Kind() != KindModification {
			t.Fatalf("%q leaked into the modification filter with kind %q", m.Name(), m.Kind())
		}
	}

	maint, maintTotal, err := p.List(KindMaintenance, page)
	if err != nil {
		t.Fatalf("List(maintenance): %v", err)
	}
	if maintTotal != 8 || len(maint) != 8 {
		t.Fatalf("maintenance: len=%d total=%d, want 8/8", len(maint), maintTotal)
	}
}

// The total must reflect the count AFTER the filter, across more than one page.
func TestList_filteredTotalSurvivesPaging(t *testing.T) {
	p := seededProvider(t)

	first, total, err := p.List(KindModification, server.Page{Number: 1, Size: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 12 {
		t.Fatalf("total = %d, want the filtered count 12", total)
	}
	if len(first) != 5 {
		t.Fatalf("page 1 len = %d, want 5", len(first))
	}

	third, _, err := p.List(KindModification, server.Page{Number: 3, Size: 5})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(third) != 2 {
		t.Fatalf("page 3 len = %d, want 2", len(third))
	}
}

func TestIDsByKind(t *testing.T) {
	p := seededProvider(t)

	ids, err := p.IDsByKind(KindModification)
	if err != nil {
		t.Fatalf("IDsByKind: %v", err)
	}
	if len(ids) != 12 {
		t.Fatalf("len(ids) = %d, want 12", len(ids))
	}
}

// A kind with no rows must yield an empty NON-NIL slice: the record provider
// reads nil as "no filter" and empty-non-nil as "match nothing" (design D3).
func TestIDsByKind_emptyResultIsNonNil(t *testing.T) {
	db := newTestDB(t) // no Seed — the table is empty
	ids, err := NewProvider(db).IDsByKind(KindModification)
	if err != nil {
		t.Fatalf("IDsByKind: %v", err)
	}
	if ids == nil {
		t.Fatal("IDsByKind returned nil for an empty result; nil means 'no filter' downstream")
	}
	if len(ids) != 0 {
		t.Fatalf("len(ids) = %d, want 0", len(ids))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/fleet-service && go test ./internal/maintenancecategory/... -run 'TestList|TestIDsByKind' -v`
Expected: FAIL — `List` takes one argument, `IDsByKind` is undefined.

- [ ] **Step 3: Implement the provider**

Replace the body of `apps/fleet-service/internal/maintenancecategory/provider.go` below the
imports:

```go
// Provider is the read-only interface for maintenance category data access.
type Provider interface {
	// List returns a page of categories. An empty kind means no filter.
	List(kind Kind, page server.Page) ([]Model, int, error)
	// IDsByKind returns every category ID of a kind. It always returns a
	// non-nil slice, because the record provider reads nil as "no filter"
	// and empty-non-nil as "match nothing" (design D3).
	IDsByKind(kind Kind) ([]string, error)
}

type dbProvider struct{ db *gorm.DB }

// NewProvider returns a read-only Provider backed by the given database.
func NewProvider(db *gorm.DB) Provider { return &dbProvider{db: db} }

func (p *dbProvider) List(kind Kind, page server.Page) ([]Model, int, error) {
	// Two independent query builders: reusing one after Count() carries the
	// aggregate's state into the Find.
	count := p.db.Model(&Entity{})
	find := p.db.Model(&Entity{})
	if kind != "" {
		count = count.Where("kind = ?", string(kind))
		find = find.Where("kind = ?", string(kind))
	}

	var total int64
	if err := count.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var es []Entity
	if err := find.Order("name asc").Offset(page.Offset()).Limit(page.Size).
		Find(&es).Error; err != nil {
		return nil, 0, err
	}
	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e))
	}
	return out, int(total), nil
}

func (p *dbProvider) IDsByKind(kind Kind) ([]string, error) {
	var ids []string
	if err := p.db.Model(&Entity{}).Where("kind = ?", string(kind)).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}
```

- [ ] **Step 4: Pass both through the processor**

In `apps/fleet-service/internal/maintenancecategory/processor.go`:

```go
// List returns a page of maintenance categories, optionally filtered by kind.
func (pr *Processor) List(kind Kind, page server.Page) ([]Model, int, error) {
	return pr.p.List(kind, page)
}

// IDsByKind returns every category ID of a kind. It is what satisfies
// maintenancerecord.CategoryAccessor, so the record list can filter by kind
// without importing this package's data access.
func (pr *Processor) IDsByKind(kind Kind) ([]string, error) {
	return pr.p.IDsByKind(kind)
}
```

- [ ] **Step 5: Parse `?kind=` on the route**

In `apps/fleet-service/internal/maintenancecategory/resource.go`, inside the GET handler,
before `page := server.ParsePage(req)`:

```go
			// An unrecognised kind is 422, not a silent empty list
			// (PRD FR-KIND-4). 422 rather than 400 because shared-go has no 400
			// sentinel and ErrValidation is the established mapping.
			kind, err := ParseKind(req.URL.Query().Get("kind"))
			if err != nil {
				server.WriteError(w, err)
				return
			}
```

and change the call to `proc.List(kind, page)`. The existing
`ms, total, err := proc.List(...)` line keeps its `:=` — `ms` and `total` are still new
declarations, so redeclaring `err` alongside them is legal.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd apps/fleet-service && go build ./... && go test ./internal/maintenancecategory/... -v`
Expected: PASS, all eight tests in the package.

- [ ] **Step 7: Commit**

```bash
git add apps/fleet-service/internal/maintenancecategory
git commit -m "feat(fleet-service): filter maintenance categories by kind"
```

---

## Task 13: record `description`, validation, and the attachment cap

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/model.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/entity.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/builder.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/administrator.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/rest.go`
- Create: `apps/fleet-service/internal/maintenancerecord/model_test.go`

**Interfaces:**
- Consumes: `server.ErrValidation`.
- Produces, in package `maintenancerecord`:
  - `const MaxDescriptionRunes = 200`, `const MaxDocuments = 10`
  - `func Validate(m Model) error`
  - `Model.Description() string`, `Model.WithDescription(string) Model`
  - `(*Builder).SetDescription(string) *Builder`
  - `Attributes.Description string` (JSON `description,omitempty`)
  Consumed by Tasks 14, 16, 17.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/maintenancerecord/model_test.go`:

```go
package maintenancerecord

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func validBuilder() *Builder {
	return NewBuilder().
		SetVehicleID("v1").
		SetCategoryID("c1").
		SetPerformedAt(time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
}

func TestBuild_requiresVehicleCategoryAndDate(t *testing.T) {
	if _, err := NewBuilder().SetCategoryID("c1").SetPerformedAt(time.Now()).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing vehicleID err = %v, want ErrValidation", err)
	}
	if _, err := NewBuilder().SetVehicleID("v1").SetPerformedAt(time.Now()).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("missing categoryID err = %v, want ErrValidation", err)
	}
	// A maintenance log with a silently-guessed date is worse than one that
	// refuses to save (PRD FR-REC-5).
	if _, err := NewBuilder().SetVehicleID("v1").SetCategoryID("c1").Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("zero performedAt err = %v, want ErrValidation", err)
	}
}

// The limit is 200 RUNES, not bytes: a 200-character limit that rejects 60
// emoji is a bug, not a security control (design D4).
func TestValidate_descriptionLimitIsCountedInRunes(t *testing.T) {
	okMultibyte := strings.Repeat("é", MaxDescriptionRunes)
	if _, err := validBuilder().SetDescription(okMultibyte).Build(); err != nil {
		t.Fatalf("200 multi-byte runes rejected: %v", err)
	}

	tooLong := strings.Repeat("a", MaxDescriptionRunes+1)
	m, err := validBuilder().SetDescription(tooLong).Build()
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("201 runes err = %v, want ErrValidation", err)
	}
	if m.Description() != "" {
		t.Fatal("an over-length description must be rejected, never truncated")
	}
}

// Surrounding whitespace is trimmed at the setter so measurement and storage
// agree, matching the client's z.string().trim().
func TestSetDescription_trims(t *testing.T) {
	m, err := validBuilder().SetDescription("  Cat-back exhaust  ").Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if m.Description() != "Cat-back exhaust" {
		t.Fatalf("Description() = %q, want trimmed", m.Description())
	}
	// A description of only whitespace is empty, not a 1-rune value.
	padded := strings.Repeat(" ", 50) + strings.Repeat("a", MaxDescriptionRunes) + strings.Repeat(" ", 50)
	if _, err := validBuilder().SetDescription(padded).Build(); err != nil {
		t.Fatalf("whitespace-padded 200 runes rejected: %v", err)
	}
}

// The cap bounds the ids= query string on media-service's internal endpoint,
// the per-record fan-out when an attachment list is expanded, and the InsertTx
// document loop (design D9).
func TestValidate_capsAttachments(t *testing.T) {
	ids := make([]string, MaxDocuments)
	for i := range ids {
		ids[i] = "m" + string(rune('a'+i))
	}
	if _, err := validBuilder().SetDocumentMediaIDs(ids).Build(); err != nil {
		t.Fatalf("%d attachments rejected: %v", MaxDocuments, err)
	}
	if _, err := validBuilder().SetDocumentMediaIDs(append(ids, "one-too-many")).Build(); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("%d attachments err = %v, want ErrValidation", MaxDocuments+1, err)
	}
}

// Validate is called from both write paths, so PATCH is guarded too — the
// builder alone would leave Processor.Update unchecked (design D4).
func TestValidate_rejectsAnOverLongUpdate(t *testing.T) {
	m, err := validBuilder().Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if err := Validate(m.WithDescription(strings.Repeat("a", MaxDescriptionRunes+1))); !errors.Is(err, server.ErrValidation) {
		t.Fatalf("Validate(over-long update) = %v, want ErrValidation", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/fleet-service && go test ./internal/maintenancerecord/... -v`
Expected: FAIL — compile error, `undefined: MaxDescriptionRunes`.

- [ ] **Step 3: Add the field, the limits and `Validate`**

In `apps/fleet-service/internal/maintenancerecord/model.go`, add to the imports
`"strings"` and `"unicode/utf8"` plus the shared-go server package, add the field
`description string` to `Model` (after `categoryID`), and add:

```go
// MaxDescriptionRunes bounds the record's short summary (PRD FR-REC-1).
// Measured in runes, not bytes.
const MaxDescriptionRunes = 200

// MaxDocuments bounds attachments per record (design D9). It bounds three
// things at once: the ids= query string on media-service's internal endpoint,
// the per-record fan-out when an attachment list is expanded, and the size of a
// single InsertTx document loop.
const MaxDocuments = 10

func (m Model) Description() string { return m.description }

// WithDescription returns a copy with the description changed. The value is
// trimmed here so measurement and storage always agree.
func (m Model) WithDescription(d string) Model {
	m.description = strings.TrimSpace(d)
	return m
}

// Validate enforces the model's invariants. It is called by Builder.Build and
// by Processor.Update after the mutation function is applied, so every write
// path is covered by construction — putting the check in the handler would
// duplicate it, and putting it only in the builder would leave PATCH unguarded
// (design D4).
func Validate(m Model) error {
	if m.vehicleID == "" || m.categoryID == "" || m.performedAt.IsZero() {
		return server.ErrValidation
	}
	if utf8.RuneCountInString(m.description) > MaxDescriptionRunes {
		return server.ErrValidation
	}
	if len(m.documentMediaIDs) > MaxDocuments {
		return server.ErrValidation
	}
	return nil
}
```

- [ ] **Step 4: Persist it**

In `apps/fleet-service/internal/maintenancerecord/entity.go`, add to `Entity` after
`CategoryID`:

```go
	Description     string    `gorm:"type:varchar(200)"`
```

map it in `Make` (`description: e.Description,`) and in `ToEntity`
(`Description: m.description,`).

In `apps/fleet-service/internal/maintenancerecord/administrator.go`, add
`"description": e.Description,` to the `Updates(map[string]any{…})` literal — without it
the column round-trips through the model and silently does not persist.

- [ ] **Step 5: Wire the builder to `Validate`**

In `apps/fleet-service/internal/maintenancerecord/builder.go`, add `"strings"` to the
imports and:

```go
// SetDescription sets the short summary, trimmed of surrounding whitespace.
func (b *Builder) SetDescription(d string) *Builder {
	b.m.description = strings.TrimSpace(d)
	return b
}

// Build validates invariants and returns the model or a validation error.
func (b *Builder) Build() (Model, error) {
	if err := Validate(b.m); err != nil {
		return Model{}, err
	}
	return b.m, nil
}
```

The `server` import in `builder.go` becomes unused once `Build` delegates — remove it.

- [ ] **Step 6: Expose it on the wire**

In `apps/fleet-service/internal/maintenancerecord/rest.go`, add to `Attributes` after
`CategoryID`:

```go
	// omitempty, consistent with the existing vendor/notes fields. Clients treat
	// an absent description as empty and fall back to the category name
	// (PRD FR-REC-2, api-contracts §2).
	Description string `json:"description,omitempty"`
```

and `Description: m.Description(),` in `Transform`.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `cd apps/fleet-service && go build ./... && go test ./internal/maintenancerecord/... -v`
Expected: PASS, six tests.

- [ ] **Step 8: Commit**

```bash
git add apps/fleet-service/internal/maintenancerecord
git commit -m "feat(fleet-service): add description and validation to maintenance records"
```

---

## Task 14: kind-filtered listing with a batched document fetch

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/provider.go`
- Modify: `apps/fleet-service/internal/maintenancerecord/processor.go`
- Create: `apps/fleet-service/internal/maintenancerecord/provider_test.go`

**Interfaces:**
- Consumes: `Validate` (Task 13).
- Produces:
  - `Provider.ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error)`
  - `(*Processor).ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error)`
  - `(*Processor).Update` now runs `Validate` on the applied model.
  Consumed by Task 16.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/maintenancerecord/provider_test.go`:

```go
package maintenancerecord

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// TableName is schema-qualified (fleet.maintenance_records) for Postgres.
	// SQLite has no schemas, so attach an in-memory database aliased "fleet".
	if err := db.Exec("ATTACH DATABASE ':memory:' AS fleet").Error; err != nil {
		t.Fatalf("attach fleet schema: %v", err)
	}
	if err := Migration(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// insertRecord writes one record plus docCount document rows and returns its ID.
func insertRecord(t *testing.T, a Administrator, vehicleID, categoryID string, day int, docCount int) string {
	t.Helper()
	ids := make([]string, 0, docCount)
	for i := 0; i < docCount; i++ {
		ids = append(ids, uuid.NewString())
	}
	m, err := NewBuilder().
		SetVehicleID(vehicleID).
		SetCategoryID(categoryID).
		SetPerformedAt(time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)).
		SetDocumentMediaIDs(ids).
		Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	created, err := a.Insert(m)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return created.ID()
}

// nil means "no filter".
func TestListByVehicle_nilCategoryIDsReturnsEverything(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "maint-1", 1, 0)
	insertRecord(t, a, "v1", "mod-1", 2, 0)
	insertRecord(t, a, "v2", "maint-1", 3, 0)

	ms, total, err := NewProvider(db).ListByVehicle("v1", nil, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 2 || len(ms) != 2 {
		t.Fatalf("len=%d total=%d, want 2/2", len(ms), total)
	}
}

// Empty-but-non-nil means "match nothing" — NOT "match everything". This is the
// difference between a fleet with no modifications seeing an empty tab and
// seeing every maintenance record labelled as a modification (design D3).
func TestListByVehicle_emptyCategoryIDsMatchesNothing(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "maint-1", 1, 0)
	insertRecord(t, a, "v1", "maint-2", 2, 0)

	ms, total, err := NewProvider(db).ListByVehicle("v1", []string{}, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 0 || len(ms) != 0 {
		t.Fatalf("len=%d total=%d, want 0/0", len(ms), total)
	}
}

func TestListByVehicle_filtersByCategoryIDs(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "maint-1", 1, 0)
	insertRecord(t, a, "v1", "mod-1", 2, 0)
	insertRecord(t, a, "v1", "mod-2", 3, 0)

	ms, total, err := NewProvider(db).ListByVehicle("v1", []string{"mod-1", "mod-2"}, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 2 || len(ms) != 2 {
		t.Fatalf("len=%d total=%d, want 2/2", len(ms), total)
	}
	for _, m := range ms {
		if m.CategoryID() == "maint-1" {
			t.Fatal("a maintenance record leaked through the modification filter")
		}
	}
}

// meta.total must be the count AFTER filtering, verified with more records than
// fit on one page (PRD FR-LIST-2).
func TestListByVehicle_filteredTotalSurvivesPaging(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	for i := 1; i <= 7; i++ {
		insertRecord(t, a, "v1", "mod-1", i, 0)
	}
	for i := 8; i <= 12; i++ {
		insertRecord(t, a, "v1", "maint-1", i, 0)
	}

	ms, total, err := NewProvider(db).ListByVehicle("v1", []string{"mod-1"}, server.Page{Number: 1, Size: 5})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	if total != 7 {
		t.Fatalf("total = %d, want the filtered count 7 (not the unfiltered 12)", total)
	}
	if len(ms) != 5 {
		t.Fatalf("page 1 len = %d, want 5", len(ms))
	}
}

// The page's documents are fetched in one query and grouped in memory (D21).
// The observable contract is that every record still carries exactly its own.
func TestListByVehicle_attachesEachRecordsOwnDocuments(t *testing.T) {
	db := newTestDB(t)
	a := NewAdministrator(db)
	insertRecord(t, a, "v1", "c1", 1, 2)
	insertRecord(t, a, "v1", "c1", 2, 0)
	insertRecord(t, a, "v1", "c1", 3, 3)

	ms, _, err := NewProvider(db).ListByVehicle("v1", nil, server.Page{Number: 1, Size: 25})
	if err != nil {
		t.Fatalf("ListByVehicle: %v", err)
	}
	// Newest first: day 3 (3 docs), day 2 (0), day 1 (2).
	want := []int{3, 0, 2}
	for i, m := range ms {
		if len(m.DocumentMediaIDs()) != want[i] {
			t.Fatalf("record %d has %d documents, want %d", i, len(m.DocumentMediaIDs()), want[i])
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/fleet-service && go test ./internal/maintenancerecord/... -run TestListByVehicle -v`
Expected: FAIL — `ListByVehicle` takes two arguments.

- [ ] **Step 3: Rewrite the provider**

In `apps/fleet-service/internal/maintenancerecord/provider.go`:

```go
// Provider is the read-only interface for maintenance record data access.
type Provider interface {
	GetByID(id string) (Model, error)
	// ListByVehicle returns a page of a vehicle's records, newest first.
	//
	// categoryIDs is a three-state filter (design D3):
	//   nil            → no filter
	//   empty non-nil  → match nothing
	//   populated      → category_id IN (…)
	//
	// The empty case is not a corner: `IN ()` is not valid SQL, and skipping
	// the clause when the slice is empty would silently degrade a filtered
	// request into an unfiltered one.
	ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error)
}

func (p *dbProvider) ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error) {
	if categoryIDs != nil && len(categoryIDs) == 0 {
		return []Model{}, 0, nil
	}

	count := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
	find := p.db.Model(&Entity{}).Where("vehicle_id = ? AND deleted_at IS NULL", vehicleID)
	if categoryIDs != nil {
		count = count.Where("category_id IN ?", categoryIDs)
		find = find.Where("category_id IN ?", categoryIDs)
	}

	var total int64
	if err := count.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var es []Entity
	if err := find.Order("performed_at desc").Offset(page.Offset()).Limit(page.Size).
		Find(&es).Error; err != nil {
		return nil, 0, err
	}

	// One query for the whole page's documents, grouped in memory (design D21).
	// This loop used to issue one query per record — 26 for a 25-record page.
	// It was harmless while no record had documents; this task makes
	// attachments the point, so it stops being harmless. Bounded by page size.
	ids := make([]string, 0, len(es))
	for _, e := range es {
		ids = append(ids, e.ID)
	}
	byRecord := make(map[string][]DocumentEntity, len(ids))
	if len(ids) > 0 {
		var docs []DocumentEntity
		if err := p.db.Where("maintenance_record_id IN ?", ids).Find(&docs).Error; err != nil {
			return nil, 0, err
		}
		for _, d := range docs {
			byRecord[d.MaintenanceRecordID] = append(byRecord[d.MaintenanceRecordID], d)
		}
	}

	out := make([]Model, 0, len(es))
	for _, e := range es {
		out = append(out, Make(e, byRecord[e.ID]))
	}
	return out, int(total), nil
}
```

`GetByID` is left alone — it is already one record, one query.

- [ ] **Step 4: Update the processor**

In `apps/fleet-service/internal/maintenancerecord/processor.go`:

```go
// ListByVehicle returns a page of records for a vehicle, optionally constrained
// to a set of category IDs (design D3 for the nil/empty semantics).
func (pr *Processor) ListByVehicle(vehicleID string, categoryIDs []string, page server.Page) ([]Model, int, error) {
	return pr.p.ListByVehicle(vehicleID, categoryIDs, page)
}

// Update applies a partial update to an existing maintenance record. The
// applied model is validated before it reaches the administrator, so PATCH is
// guarded by the same invariants as create (design D4).
func (pr *Processor) Update(id string, apply func(Model) Model) (Model, error) {
	m, err := pr.GetByID(id)
	if err != nil {
		return Model{}, err
	}
	updated := apply(m)
	if err := Validate(updated); err != nil {
		return Model{}, err
	}
	return pr.a.Update(updated)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd apps/fleet-service && go build ./... 2>&1 | head` (expect one error in
`resource.go`, fixed in Task 16 — if you want a clean build now, temporarily pass `nil` for
`categoryIDs` at the single call site in `resource.go`; Task 16 replaces that line anyway).

Run: `cd apps/fleet-service && go test ./internal/maintenancerecord/... -v`
Expected: PASS, eleven tests (six from Task 13, five here).

- [ ] **Step 6: Commit**

```bash
git add apps/fleet-service/internal/maintenancerecord
git commit -m "feat(fleet-service): filter records by category and batch the document fetch"
```

---

## Task 15: `mediaclient` — fleet-service's media ownership client

**Files:**
- Create: `apps/fleet-service/internal/mediaclient/client.go`
- Create: `apps/fleet-service/internal/mediaclient/client_test.go`

**Interfaces:**
- Consumes: media-service's `GET /internal/media` (Task 9) over HTTP.
- Produces: `mediaclient.NewClient(base string) *Client` and
  `(*Client).ValidateOwnership(ctx context.Context, fleetID string, mediaIDs []string) error`.
  Consumed by Task 16 through the `maintenancerecord.DocumentValidator` interface.

- [ ] **Step 1: Write the failing tests**

Create `apps/fleet-service/internal/mediaclient/client_test.go`:

```go
package mediaclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

func TestValidateOwnership_fullMatchPasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("fleet_id"); got != "fleet-a" {
			t.Errorf("fleet_id = %q", got)
		}
		if got := r.URL.Query().Get("ids"); got != "m1,m2" {
			t.Errorf("ids = %q", got)
		}
		_, _ = w.Write([]byte(`{"media":[{"id":"m1","status":"ready","content_type":"application/pdf"},{"id":"m2","status":"uploaded","content_type":"image/jpeg"}]}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1", "m2"}); err != nil {
		t.Fatalf("ValidateOwnership: %v", err)
	}
}

// A missing ID is 422 and is indistinguishable between "does not exist",
// "was deleted" and "belongs to another fleet" — 403 would confirm the ID
// exists somewhere else (api-contracts §3).
func TestValidateOwnership_shortSetIsValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"media":[{"id":"m1","status":"ready","content_type":"application/pdf"}]}`))
	}))
	defer srv.Close()

	err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1", "m2"})
	if !errors.Is(err, server.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}

// Ownership is the security property; readiness is a UX property. Requiring
// ready here would reject a legitimate save when a JPEG's variant worker is a
// second behind the user's click (design D8).
func TestValidateOwnership_doesNotRequireReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"media":[{"id":"m1","status":"processing","content_type":"image/jpeg"}]}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1"}); err != nil {
		t.Fatalf("a processing object was rejected: %v", err)
	}
}

// media-service unreachable means the record is not created: the alternative
// trades a visible failure for a silent one on the exact path the check exists
// to protect (design D7). The transport error propagates, so StatusFor maps it
// to 500 — NOT to a validation error.
func TestValidateOwnership_transportErrorPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", []string{"m1"})
	if err == nil {
		t.Fatal("a 500 from media-service must not be treated as success")
	}
	if errors.Is(err, server.ErrValidation) {
		t.Fatal("a transport failure must not masquerade as a validation error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("error does not name the status: %v", err)
	}
}

// The common case — logging an oil change with no receipt — makes no
// cross-service call at all.
func TestValidateOwnership_emptyInputMakesNoRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	defer srv.Close()

	if err := NewClient(srv.URL).ValidateOwnership(context.Background(), "fleet-a", nil); err != nil {
		t.Fatalf("ValidateOwnership(nil): %v", err)
	}
	if called {
		t.Fatal("an empty id list must not issue a request")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/fleet-service && go test ./internal/mediaclient/... -v`
Expected: FAIL — the package does not exist.

- [ ] **Step 3: Write the client**

Create `apps/fleet-service/internal/mediaclient/client.go`:

```go
// Package mediaclient is the internal HTTP client fleet-service uses to prove
// that a maintenance record's documentMediaIds belong to the caller's active
// fleet, via media-service's network-restricted (no-JWT) internal endpoint.
//
// Cross-service data is fetched over the API, never via a cross-service DB read
// (design D6). Modelled directly on
// apps/notification-service/internal/fleetclient.
package mediaclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jtumidanski/myfleet/packages/shared-go/server"
)

// Media is one active media object returned by GET /internal/media.
type Media struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ContentType string `json:"content_type"`
}

type response struct {
	Media []Media `json:"media"`
}

// Client calls media-service's internal endpoints. base is the service base URL
// (e.g. http://media-service:8080), from MEDIA_INTERNAL_URL.
type Client struct {
	base string
	hc   *http.Client
}

// NewClient returns a Client targeting the given media-service base URL.
func NewClient(base string) *Client { return &Client{base: base, hc: http.DefaultClient} }

// ValidateOwnership returns nil when every requested ID came back from
// media-service, which means every one is active AND in fleetID.
//
// A short set is server.ErrValidation (422). Whether an ID does not exist, was
// deleted, or belongs to another fleet is deliberately indistinguishable: a 403
// would confirm to the caller that the ID exists somewhere else.
//
// A transport failure or a non-200 propagates unchanged, so StatusFor maps it
// to 500 and no record is written. Failing closed is correct here even though
// it couples record creation to media-service availability, because the only
// requests affected are those that carry attachments (design D7).
//
// fleetID is a filter media-service applies, not an assertion it trusts.
func (c *Client) ValidateOwnership(ctx context.Context, fleetID string, mediaIDs []string) error {
	if len(mediaIDs) == 0 {
		return nil
	}

	q := url.Values{}
	q.Set("fleet_id", fleetID)
	q.Set("ids", strings.Join(mediaIDs, ","))
	endpoint := c.base + "/internal/media?" + q.Encode()

	var out response
	if err := c.getJSON(ctx, endpoint, &out); err != nil {
		return err
	}

	found := make(map[string]struct{}, len(out.Media))
	for _, m := range out.Media {
		found[m.ID] = struct{}{}
	}
	for _, id := range mediaIDs {
		if _, ok := found[id]; !ok {
			return fmt.Errorf("%w: attachment %s is not available to this fleet", server.ErrValidation, id)
		}
	}
	return nil
}

func (c *Client) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("media internal %s: status %d", endpoint, res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(dst)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/fleet-service && go test ./internal/mediaclient/... -v`
Expected: PASS, five tests.

Note: `q.Encode()` sorts and percent-encodes, so the `ids` assertion in the first test
compares the decoded value via `r.URL.Query().Get("ids")` — which is why the test reads it
that way rather than string-matching the raw URL.

- [ ] **Step 5: Commit**

```bash
git add apps/fleet-service/internal/mediaclient
git commit -m "feat(fleet-service): add media ownership client for attachment validation"
```

---

## Task 16: wire it all into the record routes

**Files:**
- Modify: `apps/fleet-service/internal/maintenancerecord/resource.go`
- Modify: `apps/fleet-service/cmd/main.go`

**Interfaces:**
- Consumes: `maintenancecategory.Kind`/`ParseKind` (Task 11),
  `(*maintenancecategory.Processor).IDsByKind` (Task 12), `Validate`/`SetDescription`
  (Task 13), `Provider.ListByVehicle` (Task 14), `*mediaclient.Client` (Task 15).
- Produces:
  - `type CategoryAccessor interface { IDsByKind(kind maintenancecategory.Kind) ([]string, error) }`
  - `type DocumentValidator interface { ValidateOwnership(ctx context.Context, fleetID string, mediaIDs []string) error }`
  - `InitializeRoutes(log, db, vehicleAccessor VehicleAccessor, categoryAccessor CategoryAccessor, docs DocumentValidator) func(chi.Router)`

- [ ] **Step 1: Declare the two consumer-side interfaces**

In `apps/fleet-service/internal/maintenancerecord/resource.go`, below `VehicleAccessor`:

```go
// CategoryAccessor resolves the category IDs of a kind so the record list can
// be filtered by kind without importing another domain's data access. It
// mirrors VehicleAccessor exactly; satisfied by *maintenancecategory.Processor.
//
// Importing maintenancecategory.Kind/ParseKind for the type keeps parsing and
// the permitted value set in the domain that owns them, so the category and
// record endpoints cannot drift on what ?kind= accepts (design D2).
type CategoryAccessor interface {
	IDsByKind(kind maintenancecategory.Kind) ([]string, error)
}

// DocumentValidator proves every documentMediaId belongs to the caller's active
// fleet (PRD FR-DOC-6). Satisfied by *mediaclient.Client. A nil value is legal
// and means "no validator wired" — used by unit tests and by any future caller
// that has already validated.
type DocumentValidator interface {
	ValidateOwnership(ctx context.Context, fleetID string, mediaIDs []string) error
}
```

Add `"context"` and the `maintenancecategory` import to the file.

- [ ] **Step 2: Extend the initializer signature**

```go
// InitializeRoutes wires the JWT-protected maintenance record endpoints.
// vehicleAccessor resolves the owning vehicle's fleetID for authz;
// categoryAccessor backs the ?kind= filter; docs validates attachment
// ownership on create and may be nil.
func InitializeRoutes(
	log logrus.FieldLogger,
	db *gorm.DB,
	vehicleAccessor VehicleAccessor,
	categoryAccessor CategoryAccessor,
	docs DocumentValidator,
) func(chi.Router) {
```

- [ ] **Step 3: Add the `?kind=` filter to the list handler**

Inside `r.Get("/vehicles/{id}/maintenance-records", …)`, replace the block from
`page := server.ParsePage(req)` through the `proc.ListByVehicle` call with:

```go
				kind, err := maintenancecategory.ParseKind(req.URL.Query().Get("kind"))
				if err != nil {
					server.WriteError(w, err)
					return
				}

				// nil means "no filter"; a resolved-but-empty set means "match
				// nothing". Never collapse the two (design D3).
				var categoryIDs []string
				if kind != "" {
					categoryIDs, err = categoryAccessor.IDsByKind(kind)
					if err != nil {
						log.WithError(err).Error("resolve category ids by kind")
						server.WriteError(w, err)
						return
					}
					if categoryIDs == nil {
						categoryIDs = []string{}
					}
				}

				page := server.ParsePage(req)
				ms, total, err := proc.ListByVehicle(vehicleID, categoryIDs, page)
```

`err` is already declared earlier in this handler by `v, err := vehicleAccessor.GetByID`.
`kind, err := …` is still legal because `kind` is a new declaration; but the later
`categoryIDs, err = …` must use plain `=`, since both variables already exist.

- [ ] **Step 4: Make `performedAt` required and accept `description` on POST**

In the POST handler's attribute struct, add `Description string \`json:"description"\`` after
`CategoryID`. Then replace the `performedAt` block:

```go
				// performedAt is required (PRD FR-REC-5). This used to default
				// to time.Now().UTC() when empty; a maintenance log with a
				// silently-guessed date is worse than one that refuses to save.
				// The builder rejects a zero time as well, so the invariant is
				// enforced twice on purpose: the handler gives the accurate
				// status code, the builder guarantees no code path can construct
				// a dateless record.
				if attrs.PerformedAt == "" {
					server.WriteError(w, server.ErrValidation)
					return
				}
				performedAt, err := time.Parse(time.RFC3339, attrs.PerformedAt)
				if err != nil {
					server.WriteError(w, server.ErrValidation)
					return
				}

				// Prove the attachments are the caller's own BEFORE anything is
				// written, so a rejection leaves nothing to roll back
				// (PRD FR-DOC-6). Skipped entirely when there are no
				// attachments, so the common case makes no cross-service call.
				if len(attrs.DocumentMediaIDs) > 0 && docs != nil {
					if err := docs.ValidateOwnership(req.Context(), identity.ActiveFleetID, attrs.DocumentMediaIDs); err != nil {
						log.WithError(err).Warn("attachment ownership validation failed")
						server.WriteError(w, err)
						return
					}
				}
```

and add `SetDescription(attrs.Description).` to the builder chain, after `SetCategoryID`.

- [ ] **Step 5: Accept `description` on PATCH**

Add `Description *string \`json:"description"\`` to the PATCH attribute struct, and inside
the `proc.Update` mutation function:

```go
					if attrs.Description != nil {
						m = m.WithDescription(*attrs.Description)
					}
```

`documentMediaIds` is deliberately **not** declared on PATCH: attachments are immutable in
this task, and `RegisterInputHandler` ignores undeclared attributes, so sending the field
is ignored rather than an error (api-contracts §4).

- [ ] **Step 6: Wire `main.go`**

In `apps/fleet-service/cmd/main.go`, add the import
`"github.com/jtumidanski/myfleet/apps/fleet-service/internal/mediaclient"`, and before the
`server.New(log)` chain:

```go
	// Category accessor for the record list's ?kind= filter. The processor is
	// stateless, so constructing a second one here rather than reshaping
	// maintenancecategory.InitializeRoutes costs nothing.
	categoryProc := maintenancecategory.NewProcessor(log, maintenancecategory.NewProvider(db))

	// Attachment ownership validation (PRD FR-DOC-6). Cluster-internal; the
	// endpoint it calls is kept off the public internet by the priority-200
	// internal-deny rule in the main overlay's ingressroute.
	mediaClient := mediaclient.NewClient(config.Get("MEDIA_INTERNAL_URL", "http://media-service:8080"))
```

and update the route line:

```go
				maintenancerecord.InitializeRoutes(log, db, vehicleProc, categoryProc, mediaClient)(pr)
```

- [ ] **Step 7: Verify**

Run: `cd apps/fleet-service && go build ./... && go vet ./...`
Expected: clean.

Run: `make test 2>&1 | tail -20`
Expected: PASS across every module.

- [ ] **Step 8: Commit**

```bash
git add apps/fleet-service
git commit -m "feat(fleet-service): kind filter, required performedAt, and attachment validation on records"
```

---

## Task 17: frontend types and the record Zod schema

**Files:**
- Modify: `apps/web/src/types/models/maintenanceCategory.ts`
- Modify: `apps/web/src/types/models/maintenanceRecord.ts`
- Modify: `apps/web/src/types/models/media.ts`
- Modify: `apps/web/src/lib/schemas/maintenanceRecord.ts`

**Interfaces:**
- Consumes: the Go `rest.go` shapes from Tasks 11 and 13.
- Produces:
  - `type MaintenanceCategoryKind = 'maintenance' | 'modification'`
  - `MaintenanceCategoryAttributes.kind: MaintenanceCategoryKind`
  - `description?: string` on all three maintenance-record interfaces
  - `type MediaStatus = 'uploaded' | 'processing' | 'ready' | 'failed'`
  - `maintenanceRecordSchema` with `description`, and `mileage`/`cost` optional
  Consumed by Tasks 18, 20, 22, 23, 24, 25.

- [ ] **Step 1: Update the category type**

`apps/web/src/types/models/maintenanceCategory.ts`:

```ts
import type { JsonApiResource } from '@myfleet/shared-ts';

/** Discriminates repair/service work from modifications (PRD FR-KIND-1). */
export type MaintenanceCategoryKind = 'maintenance' | 'modification';

/**
 * Mirrors apps/fleet-service/internal/maintenancecategory/rest.go Attributes.
 */
export interface MaintenanceCategoryAttributes {
  name: string;
  description?: string;
  systemDefined: boolean;
  /** Always present: the column is NOT NULL, so it is never omitted or null. */
  kind: MaintenanceCategoryKind;
}

export type MaintenanceCategory = JsonApiResource<MaintenanceCategoryAttributes>;
```

- [ ] **Step 2: Update the record types**

`apps/web/src/types/models/maintenanceRecord.ts` — add `description` to all three
interfaces. `MaintenanceRecordAttributes` gets:

```ts
  /**
   * Short summary. Absent when empty (the server emits it with omitempty), so
   * callers fall back to the category name (PRD FR-REC-2).
   */
  description?: string;
```

`CreateMaintenanceRecordAttributes` and `UpdateMaintenanceRecordAttributes` both get
`description?: string;`. Also relax the create shape to match the server, where mileage and
cost are optional:

```ts
export interface CreateMaintenanceRecordAttributes {
  categoryId: string;
  /** RFC3339. Required — the server no longer defaults it to now. */
  performedAt: string;
  description?: string;
  mileage?: number;
  cost?: number;
  vendor?: string;
  notes?: string;
  documentMediaIds?: string[];
}
```

- [ ] **Step 3: Update the media status type**

`apps/web/src/types/models/media.ts` — replace the `status` field:

```ts
/**
 * Lifecycle states of a media object, mirroring
 * apps/media-service/internal/mediaobject/model.go.
 *
 * 'failed' is terminal: it means the bytes could not be processed and never
 * will be. Documents skip 'processing' entirely and go straight to 'ready'.
 */
export type MediaStatus = 'uploaded' | 'processing' | 'ready' | 'failed';

export interface MediaObjectAttributes {
  fleetId: string;
  uploadedByUserId: string;
  bucket: string;
  objectKey: string;
  contentType?: string;
  size?: number;
  originalFilename?: string;
  status: MediaStatus;
}
```

- [ ] **Step 4: Update the Zod schema**

`apps/web/src/lib/schemas/maintenanceRecord.ts`:

```ts
import { z } from 'zod';

export const maintenanceRecordSchema = z.object({
  categoryId: z.string().min(1, 'Category is required'),
  performedAt: z.string().min(1, 'Date performed is required'),
  description: z
    .string()
    .trim()
    .max(200, 'Description must be 200 characters or fewer')
    .optional()
    .or(z.literal('')),
  // Optional because the form writes `undefined` when the input is cleared and
  // the server treats both as optional (PRD FR-REC-5). Without .optional() a
  // user logging an oil change with no cost cannot submit: zodResolver reports
  // "Cost must be a number" on an untouched field (design D22).
  mileage: z
    .number({ invalid_type_error: 'Mileage must be a number' })
    .int('Mileage must be a whole number')
    .min(0, 'Mileage cannot be negative')
    .optional(),
  cost: z
    .number({ invalid_type_error: 'Cost must be a number' })
    .min(0, 'Cost cannot be negative')
    .optional(),
  vendor: z.string().trim().max(200).optional().or(z.literal('')),
  notes: z.string().trim().max(2000).optional().or(z.literal('')),
  // Kept on the schema for shape compatibility, but populated from
  // usePendingAttachments.commit() at submit time rather than as a controlled
  // field — a useFieldArray of IDs would put upload state into form state
  // (design D16).
  documentMediaIds: z.array(z.string()).optional(),
});

export type MaintenanceRecordFormInput = z.infer<typeof maintenanceRecordSchema>;
```

- [ ] **Step 5: Verify the types still compile**

Run: `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22 && npm run -w apps/web build 2>&1 | tail -30`
Expected: this will FAIL — `MaintenanceScheduleForm` and any other consumer of
`MaintenanceCategoryAttributes` now needs `kind`, and mocks in existing tests may be
missing it. Fix **only** the type errors: add `kind: 'maintenance'` to test fixtures and
mock objects. Do not change behaviour here.

Run it again until clean.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/types apps/web/src/lib/schemas
git commit -m "feat(web): add category kind, record description and failed media status types"
```

---

## Task 18: category and record services + hooks take `kind`

**Files:**
- Modify: `apps/web/src/services/api/MaintenanceCategoryService.ts`
- Modify: `apps/web/src/services/api/MaintenanceRecordService.ts`
- Modify: `apps/web/src/lib/hooks/api/maintenance.ts`

**Interfaces:**
- Consumes: `MaintenanceCategoryKind` (Task 17).
- Produces:
  - `maintenanceCategoryService.list(kind?: MaintenanceCategoryKind)`
  - `maintenanceRecordService.listByVehicle(vehicleId: string, kind?: MaintenanceCategoryKind)`
  - `useMaintenanceCategories(kind?: MaintenanceCategoryKind)`
  - `useMaintenanceRecords(vehicleId, kind?: MaintenanceCategoryKind)`
  - `maintenanceCategoryKeys.list({ kind })`, `maintenanceRecordKeys.list({ vehicleId, kind })`
  Consumed by Tasks 23, 25.

- [ ] **Step 1: Add `page[size]` and `kind` to the category service**

`apps/web/src/services/api/MaintenanceCategoryService.ts`:

```ts
import type {
  MaintenanceCategoryAttributes,
  MaintenanceCategoryKind,
} from '../../types/models/maintenanceCategory';
import { BaseService, type ListResult } from './BaseService';

/**
 * Maintenance Category service.
 *
 * Routes (apps/fleet-service/internal/maintenancecategory/resource.go, gateway-prefixed):
 *   GET /api/fleet/maintenance-categories[?kind=maintenance|modification] — list (paged)
 *
 * Categories are global/system data — any authenticated caller may list them.
 */
class MaintenanceCategoryService extends BaseService<MaintenanceCategoryAttributes> {
  protected readonly resourceType = 'maintenanceCategories';
  protected readonly basePath = '/api/fleet/maintenance-categories';

  /**
   * page[size] is explicit because server.ParsePage defaults to 25 and the
   * seeded list is now 20 rows — five rows from silently truncating the picker
   * the next time a category is added, in a way that would look like "the new
   * category didn't seed" (design D23). 100 is ParsePage's hard ceiling.
   */
  list(kind?: MaintenanceCategoryKind): Promise<ListResult<MaintenanceCategoryAttributes>> {
    const params = new URLSearchParams({ 'page[size]': '100' });
    if (kind) {
      params.set('kind', kind);
    }
    return this.listAt(`${this.basePath}?${params.toString()}`);
  }
}

export const maintenanceCategoryService = new MaintenanceCategoryService();
```

`ListResult` is already exported from `BaseService.ts`; `listAt` is `protected`, so calling
it from the subclass is fine.

- [ ] **Step 2: Add `kind` to the record service**

In `apps/web/src/services/api/MaintenanceRecordService.ts`, replace `listByVehicle`:

```ts
  /** GET /api/fleet/vehicles/{vehicleId}/maintenance-records[?kind=…] */
  listByVehicle(vehicleId: string, kind?: MaintenanceCategoryKind) {
    const path = `/api/fleet/vehicles/${vehicleId}/maintenance-records`;
    return this.listAt(kind ? `${path}?kind=${kind}` : path);
  }
```

and import `MaintenanceCategoryKind` from `../../types/models/maintenanceCategory`.

- [ ] **Step 3: Thread `kind` through the hooks**

In `apps/web/src/lib/hooks/api/maintenance.ts`, update the two key factories and the two
hooks. `kind` must be part of both query keys or a filtered list would serve a cached
unfiltered one:

```ts
export const maintenanceRecordKeys = {
  all: ['maintenanceRecords'] as const,
  lists: () => [...maintenanceRecordKeys.all, 'list'] as const,
  list: (params: { vehicleId: string; kind?: MaintenanceCategoryKind }) =>
    [...maintenanceRecordKeys.lists(), params] as const,
  details: () => [...maintenanceRecordKeys.all, 'detail'] as const,
  detail: (id: string) => [...maintenanceRecordKeys.details(), id] as const,
};

export const maintenanceCategoryKeys = {
  all: ['maintenanceCategories'] as const,
  lists: () => [...maintenanceCategoryKeys.all, 'list'] as const,
  list: (params: { kind?: MaintenanceCategoryKind }) =>
    [...maintenanceCategoryKeys.lists(), params] as const,
};

/** GET /api/fleet/maintenance-categories — all categories, or one kind. */
export function useMaintenanceCategories(kind?: MaintenanceCategoryKind) {
  return useQuery({
    queryKey: maintenanceCategoryKeys.list({ kind }),
    queryFn: () => maintenanceCategoryService.list(kind),
    staleTime: 10 * 60 * 1000, // Categories are relatively static
    gcTime: 30 * 60 * 1000,
    select: (result) => result.data,
  });
}

/** GET /api/fleet/vehicles/{vehicleId}/maintenance-records[?kind=…] */
export function useMaintenanceRecords(
  vehicleId: string | null | undefined,
  kind?: MaintenanceCategoryKind,
) {
  return useQuery({
    queryKey: maintenanceRecordKeys.list({ vehicleId: vehicleId ?? '', kind }),
    queryFn: () => maintenanceRecordService.listByVehicle(vehicleId as string, kind),
    enabled: !!vehicleId,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    select: (result) => result.data,
  });
}
```

Add the `MaintenanceCategoryKind` import. `maintenanceRecordKeys.lists()` is what the
mutations invalidate, and it is a prefix of every `list({...})` key, so kind-filtered
caches are still invalidated on create/delete — no change needed there.

- [ ] **Step 4: Verify**

Run: `npm run -w apps/web test 2>&1 | tail -20`
Expected: PASS. If a test asserts the old `maintenanceCategoryKeys.lists()` shape, update
it to `maintenanceCategoryKeys.list({ kind: undefined })`.

Run: `npm run -w apps/web build 2>&1 | tail -20`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/services/api apps/web/src/lib/hooks/api/maintenance.ts
git commit -m "feat(web): thread category kind through maintenance services and hooks"
```

---

## Task 19: fleet-agnostic media upload and delete hooks

**Files:**
- Modify: `apps/web/src/lib/hooks/api/media.ts`

**Interfaces:**
- Consumes: `performMediaUpload` (existing).
- Produces:
  - `export const ACCEPTED_UPLOAD_TYPES: string`
  - `export function useMediaUpload(options?: { invalidateKeys?: readonly unknown[][] })`
  - `export function useDeleteMediaObject()`
  `useUploadMedia(vehicleId)` and `useDeleteMedia(vehicleId)` keep their signatures and
  become thin wrappers. Consumed by Tasks 20, 22.

- [ ] **Step 1: Add the accepted-types mirror**

In `apps/web/src/lib/hooks/api/media.ts`, next to `MEDIA_MAX_UPLOAD_BYTES`:

```ts
/**
 * Client-side mirror of the media-service `MEDIA_ALLOWED_CONTENT_TYPES` config
 * key, formatted for a file input's `accept` attribute.
 *
 * Like `MEDIA_MAX_UPLOAD_BYTES` this is a UX affordance, NOT a security
 * control: the server validates the content type against its own allowlist and
 * answers 415 regardless of what this string says.
 */
export const ACCEPTED_UPLOAD_TYPES = [
  'image/jpeg',
  'image/png',
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'text/csv',
].join(',');
```

- [ ] **Step 2: Extract the upload mutation**

Replace `useUploadMedia` with the pair:

```ts
export interface MediaUploadOptions {
  /**
   * Query keys to invalidate once the upload settles. Empty by default: a
   * receipt attached to a maintenance record has no gallery to refresh, and the
   * previous hard-coded vehicle-media invalidation was exactly what made the
   * old hook unusable for one.
   */
  invalidateKeys?: ReadonlyArray<readonly unknown[]>;
}

/**
 * Full upload flow: init → putContent → confirm. The mutation variable is a
 * File; the result is the confirmed media resource.
 *
 * Documents come back `ready` from confirm; images come back `processing` and
 * finish asynchronously. Callers do NOT need to wait for `ready` — the server
 * validates attachment ownership, not readiness (design D8).
 */
export function useMediaUpload(options: MediaUploadOptions = {}) {
  const queryClient = useQueryClient();
  const { invalidateKeys } = options;
  return useMutation({
    mutationFn: (file: File) =>
      performMediaUpload(file, {
        initUpload: (attrs) => mediaService.initUpload(attrs),
        putContent: (id, f) => mediaService.putContent(id, f),
        confirm: (id) => mediaService.confirm(id),
      }),
    onSettled: () => {
      for (const key of invalidateKeys ?? []) {
        void queryClient.invalidateQueries({ queryKey: key });
      }
    },
  });
}

/** Upload a file and refresh a vehicle's media gallery. */
export function useUploadMedia(vehicleId: string) {
  return useMediaUpload({ invalidateKeys: [mediaKeys.vehicleMedia(vehicleId)] });
}
```

- [ ] **Step 3: Add the fleet-agnostic delete sibling**

Below the existing `useDeleteMedia`:

```ts
/**
 * DELETE /api/media/{id} — soft delete, with no gallery coupling. Used to clean
 * up an attachment that was uploaded but never attached to a saved record
 * (PRD FR-DOC-2/FR-DOC-3). Best-effort: the 5-day purge_after sweep is the
 * authoritative backstop.
 */
export function useDeleteMediaObject() {
  return useMutation({
    mutationFn: (mediaId: string) => mediaService.remove(mediaId),
  });
}
```

- [ ] **Step 4: Verify**

Run: `npm run -w apps/web test -- src/lib/hooks/api/media.test.ts 2>&1 | tail -20`
Expected: PASS — the existing tests exercise `performMediaUpload` and
`useMediaContentUrl`, neither of which changed.

Run: `npm run -w apps/web build 2>&1 | tail -10`
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/lib/hooks/api/media.ts
git commit -m "refactor(web): extract fleet-agnostic media upload and delete hooks"
```

---

## Task 20: `usePendingAttachments`

**Files:**
- Create: `apps/web/src/lib/hooks/usePendingAttachments.ts`
- Create: `apps/web/src/lib/hooks/usePendingAttachments.test.ts`

**Interfaces:**
- Consumes: `performMediaUpload`, `MAX_ATTACHMENTS` peer constant, `mediaService`.
- Produces:
  - `export const MAX_ATTACHMENTS = 10`
  - `export interface PendingAttachment { localId; file; status: 'uploading'|'ready'|'failed'; mediaId?; error? }`
  - `export function usePendingAttachments(): { items, add, remove, commit, mediaIds, isUploading, isFull }`
  Consumed by Tasks 22, 23.

**Why a hook:** `MaintenanceRecordForm` is already 180 lines of react-hook-form wiring.
Upload orchestration, per-file status, per-file error text and orphan cleanup inline would
roughly double it and make none of it testable without rendering a form (design D16).

`'ready'` here means **"the three-step upload completed"**, not `media.status === 'ready'`.
Images are still `processing` server-side at that point and that is fine — ownership is the
security property, readiness is a UX one, and they are enforced in different places on
purpose (design D8).

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/lib/hooks/usePendingAttachments.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act, waitFor } from '@testing-library/react';
import { usePendingAttachments, MAX_ATTACHMENTS } from './usePendingAttachments';
import { mediaService } from '../../services/api/MediaService';

vi.mock('../../services/api/MediaService', () => ({
  mediaService: {
    initUpload: vi.fn(),
    putContent: vi.fn(),
    confirm: vi.fn(),
    remove: vi.fn(),
  },
}));

function file(name: string): File {
  return new File(['x'], name, { type: 'application/pdf' });
}

function mockUploadSucceeds(mediaId: string) {
  vi.mocked(mediaService.initUpload).mockResolvedValue({
    id: mediaId,
    type: 'media-objects',
    attributes: { status: 'uploaded' },
  } as never);
  vi.mocked(mediaService.putContent).mockResolvedValue({} as never);
  vi.mocked(mediaService.confirm).mockResolvedValue({
    id: mediaId,
    type: 'media-objects',
    attributes: { status: 'ready' },
  } as never);
}

describe('usePendingAttachments', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(mediaService.remove).mockResolvedValue(undefined);
  });

  it('uploads an added file and exposes its media id once confirmed', async () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));

    expect(result.current.items).toHaveLength(1);
    expect(result.current.isUploading).toBe(true);

    await waitFor(() => expect(result.current.isUploading).toBe(false));
    expect(result.current.items[0].status).toBe('ready');
    expect(result.current.mediaIds).toEqual(['m1']);
  });

  // A failed upload keeps its row with the filename and the reason and is
  // simply absent from mediaIds — the save proceeds with what succeeded
  // (PRD FR-DOC-4).
  it('keeps a failed upload visible and excludes it from mediaIds', async () => {
    vi.mocked(mediaService.initUpload).mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('bad.pdf')]));

    await waitFor(() => expect(result.current.items[0].status).toBe('failed'));
    expect(result.current.items[0].file.name).toBe('bad.pdf');
    expect(result.current.items[0].error).toBeTruthy();
    expect(result.current.mediaIds).toEqual([]);
    expect(result.current.isUploading).toBe(false);
  });

  // Removing a pending attachment soft-deletes the uploaded media object so it
  // does not linger (PRD FR-DOC-2).
  it('soft-deletes the media object when a ready item is removed', async () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));
    await waitFor(() => expect(result.current.mediaIds).toEqual(['m1']));

    act(() => result.current.remove(result.current.items[0].localId));

    expect(result.current.items).toHaveLength(0);
    expect(mediaService.remove).toHaveBeenCalledWith('m1');
  });

  it('does not call remove for an item that never uploaded', async () => {
    vi.mocked(mediaService.initUpload).mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('bad.pdf')]));
    await waitFor(() => expect(result.current.items[0].status).toBe('failed'));

    act(() => result.current.remove(result.current.items[0].localId));
    expect(mediaService.remove).not.toHaveBeenCalled();
  });

  // Abandoning the form soft-deletes everything uploaded but never attached
  // (PRD FR-DOC-3).
  it('deletes uncommitted media on unmount', async () => {
    mockUploadSucceeds('m1');
    const { result, unmount } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));
    await waitFor(() => expect(result.current.mediaIds).toEqual(['m1']));

    unmount();
    expect(mediaService.remove).toHaveBeenCalledWith('m1');
  });

  // commit() disarms the unmount cleanup, so a successful save never deletes
  // the media it just attached.
  it('commit disarms the unmount cleanup and returns the media ids', async () => {
    mockUploadSucceeds('m1');
    const { result, unmount } = renderHook(() => usePendingAttachments());

    act(() => result.current.add([file('invoice.pdf')]));
    await waitFor(() => expect(result.current.mediaIds).toEqual(['m1']));

    let committed: string[] = [];
    act(() => {
      committed = result.current.commit();
    });
    expect(committed).toEqual(['m1']);

    unmount();
    expect(mediaService.remove).not.toHaveBeenCalled();
  });

  it('stops accepting files at the cap', () => {
    mockUploadSucceeds('m1');
    const { result } = renderHook(() => usePendingAttachments());

    const many = Array.from({ length: MAX_ATTACHMENTS + 3 }, (_, i) => file(`f${i}.pdf`));
    act(() => result.current.add(many));

    expect(result.current.items).toHaveLength(MAX_ATTACHMENTS);
    expect(result.current.isFull).toBe(true);
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run -w apps/web test -- src/lib/hooks/usePendingAttachments.test.ts 2>&1 | tail -20`
Expected: FAIL — the module does not exist.

- [ ] **Step 3: Write the hook**

Create `apps/web/src/lib/hooks/usePendingAttachments.ts`:

```ts
import { useCallback, useEffect, useRef, useState } from 'react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { mediaService } from '../../services/api/MediaService';
import { performMediaUpload } from './api/media';

/**
 * Hard cap on attachments per record, mirroring
 * apps/fleet-service/internal/maintenancerecord.MaxDocuments. It bounds the
 * ids= query string on media-service's internal endpoint, the fan-out when an
 * attachment list is expanded, and the insert loop (design D9).
 */
export const MAX_ATTACHMENTS = 10;

export interface PendingAttachment {
  /** Stable across re-renders; the file itself is not a usable key. */
  localId: string;
  file: File;
  /**
   * 'ready' means the three-step upload completed — NOT that the media object
   * reached status 'ready'. An image is still 'processing' server-side at that
   * point, which is fine: the server validates ownership, not readiness
   * (design D8).
   */
  status: 'uploading' | 'ready' | 'failed';
  mediaId?: string;
  error?: string;
}

/**
 * Owns the upload lifecycle for files a user picks while filling in a form,
 * before any record exists to attach them to.
 *
 * Cleanup has three layers, in order of reliability (design D17):
 *  1. remove() deletes an uploaded object explicitly.
 *  2. An unmount effect deletes everything uploaded but never committed.
 *  3. The 5-day purge_after sweep catches the rest.
 *
 * Deliberately no beforeunload handler: it cannot reliably issue authenticated
 * requests during teardown, and layer 3 already covers the case.
 */
export function usePendingAttachments() {
  const [items, setItems] = useState<PendingAttachment[]>([]);
  // A ref mirrors the list so async upload callbacks and the unmount cleanup
  // read the current value without re-subscribing on every state change.
  const itemsRef = useRef<PendingAttachment[]>([]);
  const committedRef = useRef(false);

  const write = useCallback((next: PendingAttachment[]) => {
    itemsRef.current = next;
    setItems(next);
  }, []);

  const patch = useCallback(
    (localId: string, changes: Partial<PendingAttachment>) => {
      write(itemsRef.current.map((i) => (i.localId === localId ? { ...i, ...changes } : i)));
    },
    [write],
  );

  const add = useCallback(
    (files: FileList | File[]) => {
      const room = Math.max(MAX_ATTACHMENTS - itemsRef.current.length, 0);
      const accepted = Array.from(files).slice(0, room);
      if (accepted.length === 0) {
        return;
      }

      const created: PendingAttachment[] = accepted.map((file) => ({
        localId: crypto.randomUUID(),
        file,
        status: 'uploading',
      }));
      write([...itemsRef.current, ...created]);

      for (const item of created) {
        void performMediaUpload(item.file, {
          initUpload: (attrs) => mediaService.initUpload(attrs),
          putContent: (id, f) => mediaService.putContent(id, f),
          confirm: (id) => mediaService.confirm(id),
        })
          .then((media) => patch(item.localId, { status: 'ready', mediaId: media.id }))
          .catch((err) =>
            patch(item.localId, {
              status: 'failed',
              // Reported by name with the reason; the rest of the form is
              // unaffected (PRD FR-DOC-4).
              error: createErrorFromUnknown(err).message || 'Upload failed',
            }),
          );
      }
    },
    [patch, write],
  );

  const remove = useCallback(
    (localId: string) => {
      const target = itemsRef.current.find((i) => i.localId === localId);
      write(itemsRef.current.filter((i) => i.localId !== localId));
      if (target?.mediaId) {
        void mediaService.remove(target.mediaId).catch(() => undefined);
      }
    },
    [write],
  );

  /**
   * Returns the media IDs to submit and disarms the unmount cleanup, so a
   * successful save never deletes the media it just attached.
   */
  const commit = useCallback((): string[] => {
    committedRef.current = true;
    return itemsRef.current
      .filter((i): i is PendingAttachment & { mediaId: string } => !!i.mediaId)
      .map((i) => i.mediaId);
  }, []);

  useEffect(
    () => () => {
      if (committedRef.current) {
        return;
      }
      for (const item of itemsRef.current) {
        if (item.mediaId) {
          void mediaService.remove(item.mediaId).catch(() => undefined);
        }
      }
    },
    [],
  );

  const mediaIds = items.filter((i) => !!i.mediaId).map((i) => i.mediaId as string);

  return {
    items,
    add,
    remove,
    commit,
    mediaIds,
    isUploading: items.some((i) => i.status === 'uploading'),
    isFull: items.length >= MAX_ATTACHMENTS,
  };
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/lib/hooks/usePendingAttachments.test.ts 2>&1 | tail -20`
Expected: PASS, seven tests.

If `crypto.randomUUID` is undefined under jsdom, add a polyfill to
`apps/web/src/test/setup.ts` rather than weakening the hook:

```ts
if (!globalThis.crypto?.randomUUID) {
  Object.defineProperty(globalThis.crypto, 'randomUUID', {
    value: () => `test-${Math.random().toString(16).slice(2)}`,
  });
}
```

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/lib/hooks/usePendingAttachments.ts apps/web/src/lib/hooks/usePendingAttachments.test.ts apps/web/src/test/setup.ts
git commit -m "feat(web): add usePendingAttachments upload lifecycle hook"
```

---

## Task 21: `downloadBlob`

**Files:**
- Create: `apps/web/src/lib/utils/download.ts`
- Create: `apps/web/src/lib/utils/download.test.ts`

**Interfaces:**
- Consumes: nothing.
- Produces: `export function downloadBlob(blob: Blob, filename: string): void`.
  Consumed by Task 24.

**Deviation from PRD §7:** this lives in `apps/web`, not `packages/shared-ts`.
`shared-ts` is transport and typing, and its only DOM dependency is `fetch`, which exists
outside browsers. `downloadBlob` needs `document.createElement`, `URL.createObjectURL` and a
click — putting it in `shared-ts` would make the package browser-only for eleven lines
(design D18).

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/lib/utils/download.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { downloadBlob } from './download';

describe('downloadBlob', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    URL.createObjectURL = vi.fn(() => 'blob:test-url');
    URL.revokeObjectURL = vi.fn();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it('clicks a detached anchor carrying the object URL and the filename', () => {
    const clicks: HTMLAnchorElement[] = [];
    const realCreate = document.createElement.bind(document);
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag) as HTMLAnchorElement;
      if (tag === 'a') {
        el.click = () => clicks.push(el);
      }
      return el;
    });

    downloadBlob(new Blob(['x']), 'invoice.pdf');

    expect(clicks).toHaveLength(1);
    expect(clicks[0].href).toContain('blob:test-url');
    expect(clicks[0].download).toBe('invoice.pdf');
  });

  it('leaves no anchor behind in the document', () => {
    downloadBlob(new Blob(['x']), 'invoice.pdf');
    expect(document.querySelectorAll('a[download]')).toHaveLength(0);
  });

  // Revoking synchronously can cancel the download in some browsers, so it is
  // deferred to a macrotask — but it must still happen.
  it('revokes the object URL, but not before the click', () => {
    downloadBlob(new Blob(['x']), 'invoice.pdf');
    expect(URL.revokeObjectURL).not.toHaveBeenCalled();

    vi.runAllTimers();
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:test-url');
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run -w apps/web test -- src/lib/utils/download.test.ts 2>&1 | tail -20`
Expected: FAIL — the module does not exist.

- [ ] **Step 3: Write the implementation**

Create `apps/web/src/lib/utils/download.ts`:

```ts
/**
 * Saves a Blob to disk under `filename`.
 *
 * A plain `<a href>` cannot be used for media downloads: GET
 * /api/media/{id}/content requires an Authorization header, which the browser
 * does not send for a navigation. So the bytes are fetched through the
 * authenticated API client and handed to a detached anchor via an object URL
 * (PRD FR-VIEW-3).
 *
 * The revoke is deferred to a macrotask so the click-driven download has
 * started before the URL is invalidated; revoking synchronously cancels it in
 * some browsers.
 */
export function downloadBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  anchor.rel = 'noopener';
  document.body.appendChild(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 0);
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/lib/utils/download.test.ts 2>&1 | tail -20`
Expected: PASS, three tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/lib/utils/download.ts apps/web/src/lib/utils/download.test.ts
git commit -m "feat(web): add authenticated blob download helper"
```

---

## Task 22: `AttachmentPicker`

**Files:**
- Create: `apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.tsx`

**Interfaces:**
- Consumes: `PendingAttachment`, `MAX_ATTACHMENTS` (Task 20); `ACCEPTED_UPLOAD_TYPES`,
  `MEDIA_MAX_UPLOAD_BYTES`, `formatUploadSize` (Task 19 / existing).
- Produces: `export function AttachmentPicker(props: { items, onAdd, onRemove, disabled? })`.
  Consumed by Task 23.

- [ ] **Step 1: Write the component**

Create `apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.tsx`:

```tsx
import { useRef } from 'react';
import { FileText, ImageIcon, Loader2, X } from 'lucide-react';
import { Button } from '../../../ui/button';
import {
  ACCEPTED_UPLOAD_TYPES,
  MEDIA_MAX_UPLOAD_BYTES,
  formatUploadSize,
} from '../../../../lib/hooks/api/media';
import { MAX_ATTACHMENTS, type PendingAttachment } from '../../../../lib/hooks/usePendingAttachments';

interface AttachmentPickerProps {
  items: PendingAttachment[];
  onAdd: (files: FileList | File[]) => void;
  onRemove: (localId: string) => void;
  /** Hides the picker for viewers (PRD FR-VIEW-5). */
  disabled?: boolean;
}

/**
 * Receipt picker for the log form: choose files, watch them upload, remove any
 * one before saving.
 *
 * The `accept` attribute mirrors the server allowlist as a convenience only —
 * the server answers 415 regardless of what a client offers.
 */
export function AttachmentPicker({ items, onAdd, onRemove, disabled }: AttachmentPickerProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const isFull = items.length >= MAX_ATTACHMENTS;

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Receipts &amp; documents</span>
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={disabled || isFull}
          onClick={() => inputRef.current?.click()}
        >
          Add files
        </Button>
      </div>

      <input
        ref={inputRef}
        type="file"
        multiple
        className="hidden"
        accept={ACCEPTED_UPLOAD_TYPES}
        onChange={(e) => {
          if (e.target.files) {
            onAdd(e.target.files);
          }
          // Reset so re-picking the same file fires change again.
          e.target.value = '';
        }}
      />

      <p className="text-xs text-muted-foreground">
        {isFull
          ? `Maximum ${MAX_ATTACHMENTS} attachments per record.`
          : `PDF, image, Word, Excel or CSV. Up to ${formatUploadSize(MEDIA_MAX_UPLOAD_BYTES)} each, ${MAX_ATTACHMENTS} per record.`}
      </p>

      {items.length > 0 && (
        <ul className="space-y-1">
          {items.map((item) => (
            <li
              key={item.localId}
              className="flex items-center gap-2 rounded-md border px-2 py-1.5 text-sm"
            >
              {item.file.type.startsWith('image/') ? (
                <ImageIcon className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              ) : (
                <FileText className="h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
              )}

              <span className="min-w-0 flex-1 truncate">{item.file.name}</span>

              {item.status === 'uploading' && (
                <Loader2 className="h-4 w-4 animate-spin" aria-label="Uploading" />
              )}
              {item.status === 'failed' && (
                <span className="truncate text-xs text-destructive" title={item.error}>
                  {item.error ?? 'Upload failed'}
                </span>
              )}

              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="h-6 w-6 p-0"
                aria-label={`Remove ${item.file.name}`}
                onClick={() => onRemove(item.localId)}
              >
                <X className="h-4 w-4" />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
```

If the shadcn `Button` in this repo does not support `variant="ghost"`, read
`src/components/ui/button.tsx` and use a variant it does define — do not add a variant.

- [ ] **Step 2: Verify it compiles**

Run: `npm run -w apps/web build 2>&1 | tail -20`
Expected: clean.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/AttachmentPicker.tsx
git commit -m "feat(web): add attachment picker for the maintenance log form"
```

---

## Task 23: description field, attachments and submit gating in the form

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx`
- Create: `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx`

**Interfaces:**
- Consumes: `maintenanceRecordSchema` (Task 17), `usePendingAttachments` (Task 20),
  `AttachmentPicker` (Task 22), `MaintenanceCategoryKind` (Task 17).
- Produces: `MaintenanceRecordForm` props gain `kind?: MaintenanceCategoryKind`, and
  `onSubmit` now receives
  `(values: MaintenanceRecordFormInput, documentMediaIds: string[])`.
  No `canWrite` prop: the whole form is already rendered only behind `canWrite` in
  `VehicleMaintenanceSection`, so a second gate here would be dead code.
  Consumed by Task 25.

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx`:

```tsx
import { describe, it, expect, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MaintenanceRecordForm } from './MaintenanceRecordForm';
import type { MaintenanceCategory } from '../../../../types/models/maintenanceCategory';

const categories: MaintenanceCategory[] = [
  {
    id: 'c1',
    type: 'maintenanceCategories',
    attributes: { name: 'Oil Change', systemDefined: true, kind: 'maintenance' },
  },
  {
    id: 'c2',
    type: 'maintenanceCategories',
    attributes: { name: 'Exhaust', systemDefined: true, kind: 'modification' },
  },
];

describe('MaintenanceRecordForm', () => {
  it('offers only the categories of the requested kind', () => {
    render(<MaintenanceRecordForm categories={categories} kind="modification" onSubmit={vi.fn()} />);
    // The picker holds one option; the maintenance category must not be offered
    // when logging a modification.
    expect(screen.queryByText('Oil Change')).not.toBeInTheDocument();
  });

  it('rejects a description over 200 characters', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<MaintenanceRecordForm categories={categories} onSubmit={onSubmit} />);

    await user.type(screen.getByLabelText(/description/i), 'a'.repeat(201));
    await user.click(screen.getByRole('button', { name: /log record/i }));

    await waitFor(() => expect(screen.getByText(/200 characters or fewer/i)).toBeInTheDocument());
    expect(onSubmit).not.toHaveBeenCalled();
  });

  // A record must never be saved referencing a media object that has not been
  // confirmed (PRD FR-DOC-5).
  it('disables submit while an attachment upload is in flight', async () => {
    const user = userEvent.setup();
    render(<MaintenanceRecordForm categories={categories} onSubmit={vi.fn()} />);

    const submit = screen.getByRole('button', { name: /log record/i });
    expect(submit).toBeEnabled();

    const input = document.querySelector('input[type="file"]') as HTMLInputElement;
    await user.upload(input, new File(['x'], 'invoice.pdf', { type: 'application/pdf' }));

    await waitFor(() => expect(submit).toBeDisabled());
  });
});
```

Mock `../../../../services/api/MediaService` in this file the same way
`usePendingAttachments.test.ts` does, so the upload never resolves in the third test (use a
`new Promise(() => {})` for `initUpload`) and the submit button stays disabled.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run -w apps/web test -- src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx 2>&1 | tail -20`
Expected: FAIL — no description field, no file input, no `kind` prop.

- [ ] **Step 3: Update the form**

In `apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx`:

Props and setup:

```tsx
interface MaintenanceRecordFormProps {
  categories: MaintenanceCategory[];
  defaultMileage?: number;
  /**
   * Restricts the category picker to one kind and relabels the submit button.
   * The picker is not grouped by kind because it never shows more than one kind
   * at a time (design D19).
   */
  kind?: MaintenanceCategoryKind;
  onSubmit: (
    values: MaintenanceRecordFormInput,
    documentMediaIds: string[],
  ) => Promise<void> | void;
  onCancel?: () => void;
  submitting?: boolean;
}

export function MaintenanceRecordForm({
  categories,
  defaultMileage,
  kind,
  onSubmit,
  onCancel,
  submitting,
}: MaintenanceRecordFormProps) {
  const now = new Date().toISOString().slice(0, 16); // YYYY-MM-DDTHH:MM for datetime-local
  const attachments = usePendingAttachments();

  const visibleCategories = kind
    ? categories.filter((c) => c.attributes.kind === kind)
    : categories;

  const form = useForm<MaintenanceRecordFormInput>({
    resolver: zodResolver(maintenanceRecordSchema),
    defaultValues: {
      categoryId: '',
      performedAt: now,
      description: '',
      mileage: defaultMileage,
      cost: undefined,
      vendor: '',
      notes: '',
      documentMediaIds: [],
    },
  });
```

Submit handler — `commit()` supplies the IDs, so upload state never enters form state:

```tsx
      <form
        onSubmit={form.handleSubmit((values) => onSubmit(values, attachments.commit()))}
        className="space-y-4"
      >
```

Category picker — map `visibleCategories` instead of `categories`.

Description field, immediately after the `performedAt` field:

```tsx
        <FormField
          control={form.control}
          name="description"
          render={({ field }) => (
            <FormItem>
              <FormLabel>Description</FormLabel>
              <FormControl>
                <Input
                  type="text"
                  placeholder="Cat-back exhaust, Borla S-Type"
                  maxLength={200}
                  {...field}
                  value={field.value ?? ''}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
```

Attachment picker, after the `notes` field:

```tsx
        <AttachmentPicker
          items={attachments.items}
          onAdd={attachments.add}
          onRemove={attachments.remove}
        />
```

Submit gating and the kind-aware label:

```tsx
          <Button type="submit" disabled={submitting || attachments.isUploading}>
            {(submitting || attachments.isUploading) && (
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
            )}
            {kind === 'modification' ? 'Log Modification' : 'Log Record'}
          </Button>
```

Add the imports: `usePendingAttachments`, `AttachmentPicker`, `MaintenanceCategoryKind`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx 2>&1 | tail -20`
Expected: PASS, three tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.tsx apps/web/src/components/features/vehicles/maintenance/MaintenanceRecordForm.test.tsx
git commit -m "feat(web): add description field and receipt picker to the record form"
```

---

## Task 24: `RecordAttachmentList`

**Files:**
- Create: `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx`
- Create: `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx`

**Interfaces:**
- Consumes: `useMediaObject` (existing), `MediaThumbnail` (existing), `downloadBlob`
  (Task 21), `mediaService.getContentBlob` (existing).
- Produces: `export function RecordAttachmentList({ mediaIds }: { mediaIds: string[] })`.
  Consumed by Task 25.

**Why only for the expanded record:** rendering a 25-record page must not issue 25 × N
metadata requests (the performance NFR). This component is mounted only for the record the
user expanded, and `useMediaObject` is cached by the existing `mediaKeys` factory.

- [ ] **Step 1: Write the failing tests**

Create `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import React from 'react';
import { RecordAttachmentList } from './RecordAttachmentList';
import { mediaService } from '../../../../services/api/MediaService';
import { downloadBlob } from '../../../../lib/utils/download';

vi.mock('../../../../services/api/MediaService', () => ({
  mediaService: { get: vi.fn(), getContentBlob: vi.fn() },
}));
vi.mock('../../../../lib/utils/download', () => ({ downloadBlob: vi.fn() }));

function wrapper({ children }: { children: React.ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

function mediaResource(id: string, contentType: string, status = 'ready') {
  return {
    id,
    type: 'media-objects',
    attributes: { contentType, status, originalFilename: `${id}.file` },
  };
}

describe('RecordAttachmentList', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders a document as a working download action', async () => {
    const user = userEvent.setup();
    vi.mocked(mediaService.get).mockResolvedValue(
      mediaResource('m1', 'application/pdf') as never,
    );
    const blob = new Blob(['%PDF']);
    vi.mocked(mediaService.getContentBlob).mockResolvedValue(blob);

    render(<RecordAttachmentList mediaIds={['m1']} />, { wrapper });

    const button = await screen.findByRole('button', { name: /m1\.file/i });
    await user.click(button);

    await waitFor(() => expect(downloadBlob).toHaveBeenCalledWith(blob, 'm1.file'));
  });

  it('renders an image attachment as a thumbnail, not a download button', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(mediaResource('m2', 'image/jpeg') as never);

    render(<RecordAttachmentList mediaIds={['m2']} />, { wrapper });

    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /m2\.file/i })).not.toBeInTheDocument(),
    );
    expect(await screen.findByText('m2.file')).toBeInTheDocument();
  });

  // A media object that is missing, soft-deleted, or in a fleet the caller
  // cannot read renders an explicit unavailable row — never a broken control or
  // a silently empty list (PRD FR-VIEW-4).
  it('renders an explicit unavailable row when the media object cannot be read', async () => {
    vi.mocked(mediaService.get).mockRejectedValue(new Error('404'));

    render(<RecordAttachmentList mediaIds={['gone']} />, { wrapper });

    expect(await screen.findByText(/unavailable/i)).toBeInTheDocument();
  });

  // A terminal processing failure is the same user-visible outcome as a deleted
  // attachment — no new state to handle (design D8/D13).
  it('treats a failed media object as unavailable', async () => {
    vi.mocked(mediaService.get).mockResolvedValue(
      mediaResource('m3', 'image/jpeg', 'failed') as never,
    );

    render(<RecordAttachmentList mediaIds={['m3']} />, { wrapper });

    expect(await screen.findByText(/unavailable/i)).toBeInTheDocument();
  });
});
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `npm run -w apps/web test -- src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx 2>&1 | tail -20`
Expected: FAIL — the module does not exist.

- [ ] **Step 3: Write the component**

Create `apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx`:

```tsx
import { useState } from 'react';
import { toast } from 'sonner';
import { Download, FileText, Loader2 } from 'lucide-react';
import { createErrorFromUnknown } from '@myfleet/shared-ts';
import { Button } from '../../../ui/button';
import { Skeleton } from '../../../ui/skeleton';
import { useMediaObject } from '../../../../lib/hooks/api/media';
import { mediaService } from '../../../../services/api/MediaService';
import { downloadBlob } from '../../../../lib/utils/download';
import { MediaThumbnail } from '../media/MediaThumbnail';

/**
 * The two types media-service classifies as renderable images (its
 * mediaobject.renderableImages set). Everything else on the allowlist is a
 * document and is served as an attachment.
 */
function isRenderableImage(contentType?: string): boolean {
  return contentType === 'image/jpeg' || contentType === 'image/png';
}

function AttachmentRow({ mediaId }: { mediaId: string }) {
  const { data, isLoading, isError } = useMediaObject(mediaId);
  const [downloading, setDownloading] = useState(false);

  if (isLoading) {
    return <Skeleton className="h-10 w-full" />;
  }

  // Missing, soft-deleted, cross-fleet, or a terminal processing failure all
  // render the same explicit row rather than a broken control (PRD FR-VIEW-4).
  if (isError || !data || data.attributes.status === 'failed') {
    return (
      <div className="rounded-md border border-dashed px-2 py-1.5 text-xs text-muted-foreground">
        Attachment unavailable
      </div>
    );
  }

  const filename = data.attributes.originalFilename || mediaId;

  if (isRenderableImage(data.attributes.contentType)) {
    return (
      <div className="flex items-center gap-2 rounded-md border px-2 py-1.5">
        <MediaThumbnail mediaId={mediaId} className="h-12 w-12" />
        <span className="min-w-0 flex-1 truncate text-sm">{filename}</span>
      </div>
    );
  }

  const handleDownload = async () => {
    setDownloading(true);
    try {
      // Fetched through the authenticated API client: GET
      // /api/media/{id}/content needs an Authorization header, so a plain
      // <a href> cannot be used (PRD FR-VIEW-3).
      const blob = await mediaService.getContentBlob(mediaId);
      downloadBlob(blob, filename);
    } catch (err) {
      toast.error(createErrorFromUnknown(err).message || 'Could not download attachment');
    } finally {
      setDownloading(false);
    }
  };

  return (
    <Button
      type="button"
      variant="outline"
      className="w-full justify-start gap-2"
      disabled={downloading}
      onClick={() => void handleDownload()}
    >
      {downloading ? (
        <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />
      ) : (
        <FileText className="h-4 w-4" aria-hidden="true" />
      )}
      <span className="min-w-0 flex-1 truncate text-left">{filename}</span>
      <Download className="h-4 w-4 shrink-0" aria-hidden="true" />
    </Button>
  );
}

/**
 * The attachments of one record. Rendered only for the expanded record, which
 * is what keeps a 25-record page from issuing 25 × N metadata requests.
 *
 * Everything here renders for viewers too — only uploading, attaching and
 * deleting are gated on write access (PRD FR-VIEW-5).
 */
export function RecordAttachmentList({ mediaIds }: { mediaIds: string[] }) {
  if (mediaIds.length === 0) {
    return <p className="text-xs text-muted-foreground">No attachments.</p>;
  }
  return (
    <div className="space-y-1.5">
      {mediaIds.map((id) => (
        <AttachmentRow key={id} mediaId={id} />
      ))}
    </div>
  );
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `npm run -w apps/web test -- src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx 2>&1 | tail -20`
Expected: PASS, four tests.

- [ ] **Step 5: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.tsx apps/web/src/components/features/vehicles/maintenance/RecordAttachmentList.test.tsx
git commit -m "feat(web): render and download maintenance record attachments"
```

---

## Task 25: kind-aware maintenance section

**Files:**
- Modify: `apps/web/src/components/features/vehicles/maintenance/VehicleMaintenanceSection.tsx`

**Interfaces:**
- Consumes: `useMaintenanceCategories(kind?)`, `useMaintenanceRecords(vehicleId, kind?)`
  (Task 18); the new `MaintenanceRecordForm` props (Task 23); `RecordAttachmentList`
  (Task 24).
- Produces: no exported symbols beyond the unchanged `VehicleMaintenanceSection`.

- [ ] **Step 1: Add kind state, the category map and the filter**

In `apps/web/src/components/features/vehicles/maintenance/VehicleMaintenanceSection.tsx`,
inside the component, replace the record/category data wiring:

```tsx
  const [showRecordForm, setShowRecordForm] = useState(false);
  /** Which kind the open log form is for. */
  const [formKind, setFormKind] = useState<MaintenanceCategoryKind>('maintenance');
  /** History filter: undefined = everything. */
  const [historyKind, setHistoryKind] = useState<MaintenanceCategoryKind | undefined>();
  const [expandedRecordId, setExpandedRecordId] = useState<string | null>(null);

  const { data: categories, isLoading: categoriesLoading } = useMaintenanceCategories();
  const { data: records, isLoading: recordsLoading } = useMaintenanceRecords(vehicleId, historyKind);

  // Resolve categoryId → category once. The full list is already cached for ten
  // minutes, so every badge, group header and filter label reads from this map
  // instead of issuing a per-row fetch (design D19).
  const categoryById = useMemo(
    () => new Map((categories ?? []).map((c) => [c.id, c])),
    [categories],
  );

  // maintenance_schedules stays maintenance-only (PRD §2 non-goals), so the
  // schedule picker must not offer the seeded modification categories.
  const maintenanceCategories = useMemo(
    () => (categories ?? []).filter((c) => c.attributes.kind === 'maintenance'),
    [categories],
  );
```

Add the imports: `useMemo` from React, `MaintenanceCategoryKind` from the category type
module, and `RecordAttachmentList`.

- [ ] **Step 2: Pass `documentMediaIds` through on create**

Replace `handleCreateRecord`:

```tsx
  const handleCreateRecord = async (
    values: MaintenanceRecordFormInput,
    documentMediaIds: string[],
  ) => {
    try {
      await createRecord.mutateAsync({
        categoryId: values.categoryId,
        performedAt: new Date(values.performedAt).toISOString(),
        description: values.description || undefined,
        mileage: values.mileage,
        cost: values.cost,
        vendor: values.vendor || undefined,
        notes: values.notes || undefined,
        documentMediaIds,
      });
      toast.success(
        formKind === 'modification' ? 'Modification logged' : 'Maintenance record logged',
      );
      setShowRecordForm(false);
    } catch (err) {
      const apiError = createErrorFromUnknown(err);
      toast.error(apiError.message || 'Could not log maintenance record');
    }
  };
```

`mileage`/`cost` are now passed straight through rather than coerced with `?? 0`: both are
optional server-side (FR-REC-5), and sending `0` for a cost the user did not enter is a
lie about the record.

- [ ] **Step 3: Give the schedule form maintenance categories only**

Change the schedule form's prop:

```tsx
              <MaintenanceScheduleForm
                categories={maintenanceCategories}
```

- [ ] **Step 4: Two entry points and the history filter**

Replace the "Maintenance History" header block:

```tsx
          <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-base font-semibold">History</h2>
            <div className="flex items-center gap-2">
              {(['all', 'maintenance', 'modification'] as const).map((option) => {
                const value = option === 'all' ? undefined : option;
                const isActive = historyKind === value;
                return (
                  <Button
                    key={option}
                    size="sm"
                    variant={isActive ? 'default' : 'outline'}
                    onClick={() => setHistoryKind(value)}
                  >
                    {option === 'all' ? 'All' : option === 'maintenance' ? 'Maintenance' : 'Mods'}
                  </Button>
                );
              })}
              {canWrite && !showRecordForm && (
                <>
                  <Button
                    size="sm"
                    onClick={() => {
                      setFormKind('maintenance');
                      setShowRecordForm(true);
                    }}
                  >
                    Log Record
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setFormKind('modification');
                      setShowRecordForm(true);
                    }}
                  >
                    Log Modification
                  </Button>
                </>
              )}
            </div>
          </div>
```

and the form invocation:

```tsx
          {showRecordForm && categories && (
            <div className="mb-4 rounded-md border p-4">
              <MaintenanceRecordForm
                categories={categories}
                kind={formKind}
                defaultMileage={autoFillMileage}
                onSubmit={handleCreateRecord}
                onCancel={() => setShowRecordForm(false)}
                submitting={createRecord.isPending}
              />
            </div>
          )}
```

- [ ] **Step 5: Description as the primary line, kind badge, attachment expansion**

Replace the record row body:

```tsx
              {records.map((record) => {
                const category = categoryById.get(record.attributes.categoryId);
                const documentCount = record.attributes.documentMediaIds?.length ?? 0;
                const isExpanded = expandedRecordId === record.id;
                return (
                  <div key={record.id} className="rounded-md border p-3">
                    <div className="flex items-start justify-between gap-2">
                      <div className="min-w-0 flex-1 space-y-0.5">
                        <div className="flex items-center gap-2">
                          {/* description is the primary line; existing records
                              with no description fall back to the category name
                              so history stays readable (PRD FR-REC-2). */}
                          <p className="truncate text-sm font-medium">
                            {record.attributes.description || category?.attributes.name ||
                              record.attributes.categoryId}
                          </p>
                          {category?.attributes.kind === 'modification' && (
                            <span className="inline-flex items-center rounded-full border border-violet-200 bg-violet-100 px-2.5 py-0.5 text-xs font-medium text-violet-800">
                              Mod
                            </span>
                          )}
                        </div>
                        <p className="text-xs text-muted-foreground">
                          {new Date(record.attributes.performedAt).toLocaleDateString()}
                          {record.attributes.description && category
                            ? ` · ${category.attributes.name}`
                            : ''}
                          {record.attributes.mileage
                            ? ` · ${record.attributes.mileage.toLocaleString()} mi`
                            : ''}
                          {record.attributes.cost > 0
                            ? ` · $${record.attributes.cost.toFixed(2)}`
                            : ''}
                        </p>
                        {record.attributes.vendor && (
                          <p className="text-xs text-muted-foreground">
                            {record.attributes.vendor}
                          </p>
                        )}
                      </div>

                      {documentCount > 0 && (
                        <Button
                          size="sm"
                          variant="outline"
                          aria-expanded={isExpanded}
                          onClick={() => setExpandedRecordId(isExpanded ? null : record.id)}
                        >
                          <Paperclip className="mr-1 h-3.5 w-3.5" aria-hidden="true" />
                          {documentCount}
                        </Button>
                      )}
                    </div>

                    {/* Mounted only when expanded, which is what keeps a
                        25-record page from issuing 25 × N metadata requests. */}
                    {isExpanded && (
                      <div className="mt-3 border-t pt-3">
                        <RecordAttachmentList
                          mediaIds={record.attributes.documentMediaIds ?? []}
                        />
                      </div>
                    )}
                  </div>
                );
              })}
```

Add `Paperclip` to the `lucide-react` import.

- [ ] **Step 6: Verify**

Run: `npm run -w apps/web build 2>&1 | tail -20`
Expected: clean.

Run: `npm run -w apps/web test 2>&1 | tail -20`
Expected: PASS across the whole suite.

- [ ] **Step 7: Commit**

```bash
git add apps/web/src/components/features/vehicles/maintenance/VehicleMaintenanceSection.tsx
git commit -m "feat(web): kind filtering, badges and attachment viewing in the maintenance section"
```

---

## Task 26: full verification

**Files:** none modified unless a gate fails.

**Interfaces:** none.

- [ ] **Step 1: Run the full CI gate**

```bash
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
make ci
```

Expected: `lint-check`, `vet`, `test`, `build`, `fe-test`, `fe-build`, `manifests` all pass.
Fix anything that fails and re-run the whole target — not just the failing step.

- [ ] **Step 2: Assert the main overlay's invariants**

```bash
kustomize build deploy/k8s/overlays/main > /tmp/main.yaml
grep -c 'kind: PersistentVolumeClaim' /tmp/main.yaml || true
grep -c 'kind: Secret' /tmp/main.yaml || true
grep -c 'kind: ClusterRole' /tmp/main.yaml || true
grep -c 'api/+media\[\^/\]\*/\*internal' /tmp/main.yaml
grep -c 'MEDIA_ALLOWED_CONTENT_TYPES' /tmp/main.yaml
grep -c 'MEDIA_INTERNAL_URL' /tmp/main.yaml
```

Expected: `0` PVCs, `0` Secrets, `0` ClusterRoles, **`2`** internal-deny media rules (one
per entrypoint — a `1` means the TLS twin lost it and an entrypoint is exposed), `1`
`MEDIA_ALLOWED_CONTENT_TYPES`, `1` `MEDIA_INTERNAL_URL`.

- [ ] **Step 3: Server dry-runs against a reachable cluster**

```bash
kustomize build deploy/k8s/overlays/main  | kubectl apply --dry-run=server -f -
kustomize build deploy/k8s/overlays/local | kubectl apply --dry-run=server -f -
```

Expected: both succeed. Rendering alone does not catch namespace or cross-resource-reference
errors, and the local overlay is not exempt — a missing `namespace:` there once slipped
through ten reviews because only the `main` dry-run was ever run. If no cluster is
reachable, say so explicitly in the completion report rather than silently skipping.

- [ ] **Step 4: Manual end-to-end checks**

Bring the stack up (`make up`) and confirm, recording the actual result of each:

1. Log a maintenance record with a real PDF attached; expand it and download the PDF —
   the saved file has its original name and opens.
2. Log a modification with a photo; it shows a **Mod** badge and an inline thumbnail.
3. Filter the history to Maintenance-only and Mods-only; each shows only its kind.
4. Upload a file at 25 MiB − 1 (succeeds) and 25 MiB + 1 (rejected with the size message).
5. Upload an HEIC file — a clear `415` naming the accepted types, not today's silent hang.
6. Attach a receipt, remove it before saving, and confirm the media object is soft-deleted.
7. Open the form, attach a receipt, cancel — the media object is soft-deleted.
8. Sign in as a viewer: records and downloads work; no Log/Add/Remove control renders.
9. Confirm the existing vehicle photo gallery still renders (the image path is unchanged).

- [ ] **Step 5: Code review before the PR**

Per CLAUDE.md, run `superpowers:requesting-code-review`, which dispatches
`plan-adherence-reviewer`, `backend-guidelines-reviewer` and `frontend-guidelines-reviewer`.
Do not skip this even though the plan looks complete. Address the findings, then open the PR.

- [ ] **Step 6: Commit any fixes**

```bash
git add -A
git commit -m "chore: address verification and review findings"
```

---

## Coverage map

Every PRD requirement, mapped to the task that implements it.

| Requirement | Task |
|---|---|
| FR-KIND-1 (`kind` column, NOT NULL, default classifies existing rows) | 11 |
| FR-KIND-2 (twelve modification seeds) | 11 |
| FR-KIND-3 (`Seed` stays idempotent) | 11 |
| FR-KIND-4 (`?kind=` on categories, 422 on a bad value) | 12 |
| FR-KIND-5 (`kind` in category attributes) | 11 |
| FR-REC-1 (`description`, ≤200, server-enforced) | 13 |
| FR-REC-2 (description is the primary line, falls back to category) | 25 |
| FR-REC-3 (`notes` unchanged) | 13 (no change made) |
| FR-REC-4 (`description` on create and PATCH) | 13, 16 |
| FR-REC-5 (`performedAt` required; the rest optional) | 13, 16, 17 |
| FR-LIST-1 (`?kind=` on records, no cross-service join) | 12, 14, 16 |
| FR-LIST-2 (filtered `meta.total`) | 14 |
| FR-LIST-3 (client resolves kind; no new record field) | 25 |
| FR-DOC-1 (pick files, upload, submit IDs) | 20, 22, 23, 25 |
| FR-DOC-2 (per-item removal soft-deletes) | 20, 22 |
| FR-DOC-3 (abandoning the form soft-deletes orphans) | 20 |
| FR-DOC-4 (a failed upload does not block saving) | 20, 22 |
| FR-DOC-5 (submit disabled while uploading) | 23 |
| FR-DOC-6 (server-side fleet validation of `documentMediaIds`) | 9, 15, 16 |
| FR-VIEW-1 (attachment count; expand to a filename list) | 24, 25 |
| FR-VIEW-2 (images as thumbnails) | 24 |
| FR-VIEW-3 (documents as authenticated downloads) | 21, 24 |
| FR-VIEW-4 (explicit unavailable row) | 24 |
| FR-VIEW-5 (viewers can view and download only) | 22, 23, 24, 25 |
| FR-MEDIA-1 (allowlist enforced, 415 sentinel) | 1, 2, 5 |
| FR-MEDIA-2 (config-driven allowlist, default value) | 2, 5 |
| FR-MEDIA-3 (renderable vs document classification) | 2 |
| FR-MEDIA-4 (documents confirm straight to ready, no event) | 6 |
| FR-MEDIA-5 (every confirmed upload reaches a terminal state) | 4, 8 |
| FR-MEDIA-6 (25 MiB cap unchanged, client mirror in sync) | 19 (no change made) |
| FR-DL-1 (`nosniff` on every response) | 7 |
| FR-DL-2 (inline for images, attachment for documents) | 3, 7 |
| FR-DL-3 (escaped filename, RFC 5987, no injection) | 3 |
| FR-DL-4 (stored type re-resolved; octet-stream fallback) | 2, 7 |
| FR-DL-5 (`Cache-Control` retained) | 7 |
| §8 Security (allowlist server-side, disposition + nosniff, write-side scoping) | 5, 7, 16 |
| §8 Performance (bounded requests per page, no variant work for documents) | 6, 14, 24 |
| §8 Observability (`warn` on a rejected type, `error` on a terminal failure) | 5, 8 |
| §8 Compatibility (additive schema, legacy rows defined) | 7, 11, 13 |
| Design D9 (attachment cap of 10) | 13, 20 |
| Design D18 (`downloadBlob` in `apps/web`) | 21 |
| Design D20 (`internal-deny` at the edge) | 10 |
| Design D21 (batched document fetch) | 14 |
| Design D22 (`mileage`/`cost` optional in Zod) | 17 |
| Design D23 (`page[size]=100` on categories) | 18 |
| PRD §2 non-goal: schedules stay maintenance-only (`context.md` deviation 3) | 25 |
| PRD §2 non-goal: attachments immutable on `PATCH` (field not declared) | 16 |
| PRD §10 Project gates (`make ci`, overlays, dry-runs, code review) | 26 |
