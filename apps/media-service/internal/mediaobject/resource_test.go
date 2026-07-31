package mediaobject

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	"github.com/jtumidanski/myfleet/packages/shared-go/server"

	"github.com/jtumidanski/myfleet/apps/media-service/internal/storage"
)

// TestClassifyUploadError_mapsMaxBytesTo413 exercises the real
// *http.MaxBytesError produced by http.MaxBytesReader (the same mechanism
// PUT /media/{id}/content uses to bound uploads), rather than a hand-rolled
// stand-in, so the mapping is verified against actual SDK behavior.
func TestClassifyUploadError_mapsMaxBytesTo413(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/media/x/content", strings.NewReader("more bytes than the cap allows"))
	body := http.MaxBytesReader(rec, req.Body, 4)

	_, readErr := io.ReadAll(body)
	if readErr == nil {
		t.Fatal("expected MaxBytesReader to reject a body over its cap")
	}

	mapped := classifyUploadError(readErr)
	if !errors.Is(mapped, server.ErrRequestEntityTooLarge) {
		t.Fatalf("classifyUploadError(%v) = %v, want ErrRequestEntityTooLarge", readErr, mapped)
	}
}

// TestClassifyUploadError_passesOtherErrorsThrough verifies the mapping is
// scoped to the MaxBytesError case only: any other error (404/409/500, ...)
// must come back unchanged so the caller's existing error handling applies.
func TestClassifyUploadError_passesOtherErrorsThrough(t *testing.T) {
	other := errors.New("simulated store failure")
	if got := classifyUploadError(other); got != other {
		t.Fatalf("classifyUploadError(%v) = %v, want passthrough", other, got)
	}

	if got := classifyUploadError(server.ErrNotFound); !errors.Is(got, server.ErrNotFound) {
		t.Fatalf("classifyUploadError(ErrNotFound) = %v, want ErrNotFound unchanged", got)
	}
}

// testRouter mounts the real routes over an in-memory DB and the supplied
// store, with no stored variants — the shape every pre-variant test needs.
func testRouter(t *testing.T, store ObjectStore, maxUploadBytes int64) (http.Handler, *Processor) {
	t.Helper()
	return testRouterWithVariants(t, store, &fakeVariants{}, maxUploadBytes)
}

// testRouterWithVariants mounts the real routes with a variant lookup under the
// test's control, and injects the identity the JWT middleware would normally put
// on the context so the handlers can be exercised end to end.
func testRouterWithVariants(t *testing.T, store ObjectStore, variants VariantLookup, maxUploadBytes int64) (http.Handler, *Processor) {
	t.Helper()
	router, proc, _ := testRouterCapturingLogs(t, store, variants, maxUploadBytes)
	return router, proc
}

// testRouterCapturingLogs is testRouterWithVariants plus the hook, for the tests
// that assert an operator-facing log was actually emitted rather than only that
// the right status reached the client.
func testRouterCapturingLogs(t *testing.T, store ObjectStore, variants VariantLookup, maxUploadBytes int64) (http.Handler, *Processor, *logrusTestHook) {
	t.Helper()
	db := newConfirmTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	hook := &logrusTestHook{}
	log.AddHook(hook)
	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, store, variants, maxUploadBytes))
	return r, NewProcessor(log, NewProvider(db), NewAdministrator(db), store, variants), hook
}

func memberRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	return req.WithContext(auth.WithIdentity(req.Context(), auth.Identity{
		UserID:        "u1",
		ActiveFleetID: "fleet-a",
		Role:          "member",
	}))
}

// TestPutContent_overCapContentLengthIs413BeforeStorage covers the request that
// used to be the most expensive one this service could receive: a client that
// advertises a Content-Length above the cap. That header alone is grounds for a
// 413 — it must be answered without reading the body and without opening an
// upload against object storage. (Previously the advertised size was silently
// downgraded to the unknown-length sentinel, which routed the request into
// minio-go's size-guessing path.)
func TestPutContent_overCapContentLengthIs413BeforeStorage(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media"}
	const cap = 1024
	router, proc := testRouter(t, store, cap)

	created, err := proc.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	// One byte on the wire, a wildly overstated Content-Length — the shape that
	// costs a client nothing to send.
	req := memberRequest(http.MethodPut, "/media/"+created.ID()+"/content", strings.NewReader("x"))
	req.ContentLength = 99999999
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rec.Code)
	}
	if store.putCalls != 0 {
		t.Fatalf("object storage was called %d time(s); an over-cap Content-Length must be rejected before any storage work", store.putCalls)
	}
}

// TestPutContent_unknownLengthStillUploads guards the other side of the same
// check: a chunked body has no Content-Length at all, which is legitimate and
// must still reach the store with the unknown-length sentinel.
func TestPutContent_unknownLengthStillUploads(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media"}
	router, proc := testRouter(t, store, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	req := memberRequest(http.MethodPut, "/media/"+created.ID()+"/content", strings.NewReader("jpeg-bytes"))
	req.ContentLength = -1 // chunked: length unknown
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (a chunked upload is legitimate); body: %s", rec.Code, rec.Body.String())
	}
	if store.putSize != -1 {
		t.Fatalf("store received size %d, want the -1 unknown-length sentinel", store.putSize)
	}
	if string(store.putBody) != "jpeg-bytes" {
		t.Fatalf("store received %q, want %q", store.putBody, "jpeg-bytes")
	}
}

// TestPutContent_withinCapContentLengthIsPassedThrough confirms the new guard
// does not fire on ordinary uploads: a truthful Content-Length at or below the
// cap reaches the store unchanged, so minio-go can take its cheap known-size
// path.
func TestPutContent_withinCapContentLengthIsPassedThrough(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media"}
	router, proc := testRouter(t, store, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	body := "jpeg-bytes"
	req := memberRequest(http.MethodPut, "/media/"+created.ID()+"/content", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if store.putSize != int64(len(body)) {
		t.Fatalf("store received size %d, want %d", store.putSize, len(body))
	}
}

// TestGetContent_missingObjectIs404NotEmpty200 is the HTTP-level statement of
// the bug: a media row whose bytes were never stored must not answer 200 with
// an empty body. The status has to be decided before any header is committed.
func TestGetContent_missingObjectIs404NotEmpty200(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getErr: storage.ErrObjectNotFound}
	router, proc := testRouter(t, store, 1024)

	// Exactly the state POST /media leaves behind: row created, nothing PUT.
	created, err := proc.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	req := memberRequest(http.MethodGet, "/media/"+created.ID()+"/content", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d (body %q), want 404 — an object with no stored bytes must not look like a successful empty download", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct == "image/jpeg" {
		t.Fatal("the stored row's Content-Type must not be committed on the error path")
	}
}

// TestGetContent_presentObjectStreams200 pins the happy path so the fix above
// cannot regress into rejecting real downloads: Content-Type comes from the
// stored row, Content-Length from its recorded size, and the bytes stream out.
func TestGetContent_presentObjectStreams200(t *testing.T) {
	payload := []byte("jpeg-bytes")
	store := &fakeStore{bucket: "myfleet-media", getBody: payload}
	router, proc := testRouter(t, store, 1024)

	created, err := proc.InitUpload("fleet-a", "u1", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	if _, err := proc.StoreContent(context.Background(), created.ID(), "fleet-a", bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("store content: %v", err)
	}

	req := memberRequest(http.MethodGet, "/media/"+created.ID()+"/content", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg (from the stored row)", ct)
	}
	if rec.Body.String() != string(payload) {
		t.Fatalf("body = %q, want %q", rec.Body.String(), payload)
	}
}

// TestGetContent_crossFleetIs404 keeps tenancy semantics intact alongside the
// new not-found mapping: a foreign fleet must still get a 404 and storage must
// never be touched.
func TestGetContent_crossFleetIs404(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("jpeg-bytes")}
	router, proc := testRouter(t, store, 1024)

	created, err := proc.InitUpload("fleet-b", "u2", "image/jpeg", "photo.jpg")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}

	req := memberRequest(http.MethodGet, "/media/"+created.ID()+"/content", nil) // identity is fleet-a
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a cross-fleet read", rec.Code)
	}
	if store.getKey != "" {
		t.Fatalf("cross-fleet read touched storage (key %q)", store.getKey)
	}
}

// seedStoredObject creates a media object and records its bytes, returning the
// media id — the state a completed upload leaves behind.
func seedStoredObject(t *testing.T, pr *Processor, fleetID string, payload []byte) string {
	t.Helper()
	created, err := pr.InitUpload(fleetID, "u1", "image/png", "photo.png")
	if err != nil {
		t.Fatalf("init upload: %v", err)
	}
	if _, err := pr.StoreContent(context.Background(), created.ID(), fleetID, bytes.NewReader(payload), int64(len(payload))); err != nil {
		t.Fatalf("store content: %v", err)
	}
	return created.ID()
}

func thumbnailRouter(t *testing.T) (http.Handler, *Processor, *fakeStore) {
	t.Helper()
	store := &fakeStore{
		bucket:  "myfleet-media",
		getBody: []byte("original-bytes"),
		getBodies: map[string][]byte{
			"fleet-a/thumb.jpg":   []byte("thumb-bytes"),
			"fleet-a/display.jpg": []byte("display-bytes"),
		},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-a/thumb.jpg", ContentType: "image/jpeg"},
		"display":   {ObjectKey: "fleet-a/display.jpg", ContentType: "image/jpeg"},
	}}
	router, proc := testRouterWithVariants(t, store, variants, 1024)
	return router, proc, store
}

// TestGetContent_noVariantParamIsUnchanged is the backwards-compatibility gate:
// a request with no ?variant= must behave exactly as it did before this feature,
// Content-Length included. Every existing caller depends on it.
func TestGetContent_noVariantParamIsUnchanged(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "original-bytes" {
		t.Fatalf("body = %q, want original-bytes", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", ct)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "14" {
		t.Fatalf("Content-Length = %q, want 14 (len(\"original-bytes\"))", cl)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=300" {
		t.Fatalf("Cache-Control = %q, want private, max-age=300", cc)
	}
}

// TestGetContent_thumbnailServesVariantWithoutContentLength is the request the
// vehicles list makes. media_variants records no byte count, so sending the
// ORIGINAL's Content-Length here would truncate or hang the response.
func TestGetContent_thumbnailServesVariantWithoutContentLength(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "thumb-bytes" {
		t.Fatalf("body = %q, want thumb-bytes", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want the variant's own image/jpeg", ct)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		t.Fatalf("Content-Length = %q, want it omitted — that value describes the original, not the variant", cl)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "private, max-age=300" {
		t.Fatalf("Cache-Control = %q, want private, max-age=300 on a variant response too", cc)
	}
}

func TestGetContent_displayAndOriginalVariantsAreAccepted(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	for _, tc := range []struct{ variant, want string }{
		{"display", "display-bytes"},
		{"original", "original-bytes"},
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant="+tc.variant, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("?variant=%s status = %d, want 200; body: %s", tc.variant, rec.Code, rec.Body.String())
		}
		if rec.Body.String() != tc.want {
			t.Fatalf("?variant=%s body = %q, want %q", tc.variant, rec.Body.String(), tc.want)
		}
	}
}

// TestGetContent_variantWithNoRowIs404WithNoOriginalBytes: the normal state for
// a media object whose processing has not completed yet is now a 404 over the
// wire, not the original. A 12-card grid must not be able to turn 12 thumbnail
// requests into 12 full-size downloads.
func TestGetContent_variantWithNoRowIs404WithNoOriginalBytes(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	router, proc := testRouterWithVariants(t, store, &fakeVariants{}, 1024)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))
	callsBefore := len(store.getCalls)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "original-bytes") {
		t.Fatalf("the 404 body carried the original's bytes: %s", rec.Body.String())
	}
	if len(store.getCalls) != callsBefore {
		t.Fatalf("an unservable variant read the object store: %v", store.getCalls[callsBefore:])
	}
}

// TestGetContent_originalIsUnaffectedByAnUnservableVariant is the
// backwards-compatibility half of the same change: making a derived variant 404
// must leave ?variant=original and the bare request serving the original with
// its Content-Length, on the very same object that 404s for a thumbnail.
func TestGetContent_originalIsUnaffectedByAnUnservableVariant(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	router, proc := testRouterWithVariants(t, store, &fakeVariants{}, 1024)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	for _, target := range []string{
		"/media/" + id + "/content",
		"/media/" + id + "/content?variant=original",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, memberRequest(http.MethodGet, target, nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200; body: %s", target, rec.Code, rec.Body.String())
		}
		if rec.Body.String() != "original-bytes" {
			t.Fatalf("GET %s body = %q, want original-bytes", target, rec.Body.String())
		}
		if cl := rec.Header().Get("Content-Length"); cl != "14" {
			t.Fatalf("GET %s Content-Length = %q, want 14 — the original's contract is unchanged", target, cl)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "image/png" {
			t.Fatalf("GET %s Content-Type = %q, want image/png", target, ct)
		}
	}
}

// TestGetContent_lookupFailureIs500AndIsLogged: a broken variant query is a
// server fault, and a 500 that leaves no server-side trace is undebuggable. Its
// sibling handlers in this file log every failure they turn into an error
// response; this one must too.
func TestGetContent_lookupFailureIs500AndIsLogged(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{err: errors.New("simulated variant query failure")}
	router, proc, hook := testRouterCapturingLogs(t, store, variants, 1024)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rec.Code, rec.Body.String())
	}
	if !hook.hasError() {
		t.Fatal("a 500 from the variant lookup was returned with no server-side log")
	}
}

// TestGetContent_expectedErrorsAreNotLoggedAsFaults: a thumbnail that has not
// been generated yet is the normal state of freshly uploaded media. Logging each
// one at Error would bury the genuine faults the test above pins.
func TestGetContent_expectedErrorsAreNotLoggedAsFaults(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	router, proc, hook := testRouterCapturingLogs(t, store, &fakeVariants{}, 1024)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if hook.hasError() {
		t.Fatal("an expected 404 was logged at Error level")
	}
}

// TestGetContent_bogusVariantIs400WithNoBytes: a typo must be loud, not a
// silent multi-megabyte download.
func TestGetContent_bogusVariantIs400WithNoBytes(t *testing.T) {
	router, proc, store := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))
	callsBefore := len(store.getCalls)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=bogus", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"errors"`) || !strings.Contains(rec.Body.String(), `"bad_request"`) {
		t.Fatalf("body = %s, want a JSON:API error envelope with code bad_request", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "original-bytes") || strings.Contains(rec.Body.String(), "thumb-bytes") {
		t.Fatalf("a rejected variant returned image bytes: %s", rec.Body.String())
	}
	if len(store.getCalls) != callsBefore {
		t.Fatalf("a rejected variant touched storage: %v", store.getCalls[callsBefore:])
	}
}

// TestGetContent_variantCrossFleetIs404WithNoStoreRead: a variant must never be
// reachable by a caller who could not read the original, and the 404 (not 403)
// keeps cross-fleet existence unleakable.
func TestGetContent_variantCrossFleetIs404WithNoStoreRead(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("original-bytes")}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-b/thumb.jpg", ContentType: "image/jpeg"},
	}}
	router, proc := testRouterWithVariants(t, store, variants, 1024)
	id := seedStoredObject(t, proc, "fleet-b", []byte("original-bytes")) // other fleet
	callsBefore := len(store.getCalls)

	rec := httptest.NewRecorder()
	// memberRequest carries ActiveFleetID "fleet-a".
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for a cross-fleet variant read", rec.Code)
	}
	if len(store.getCalls) != callsBefore {
		t.Fatalf("cross-fleet variant read touched storage: %v", store.getCalls[callsBefore:])
	}
	if len(variants.calls) != 0 {
		t.Fatalf("cross-fleet variant read ran the variant lookup: %v", variants.calls)
	}
}
