package mediaobject

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/jtumidanski/myfleet/packages/shared-go/auth"
	sharedevents "github.com/jtumidanski/myfleet/packages/shared-go/events"
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
	router, proc, hook, _ := buildTestRouter(t, store, variants, maxUploadBytes)
	return router, proc, hook
}

// testRouterWithDB is testRouter plus the *gorm.DB, for the tests that inspect
// the outbox or rewrite a stored content type behind the processor's back —
// the only way to reproduce a pre-allowlist row now that InitUpload normalises.
func testRouterWithDB(t *testing.T, store ObjectStore, maxUploadBytes int64) (http.Handler, *Processor, *gorm.DB) {
	t.Helper()
	router, proc, _, db := buildTestRouter(t, store, &fakeVariants{}, maxUploadBytes)
	return router, proc, db
}

// buildTestRouter is the single construction point every testRouter* helper
// delegates to, so the allowlist and the variant lookup are wired identically
// no matter which shape a test asks for.
func buildTestRouter(t *testing.T, store ObjectStore, variants VariantLookup, maxUploadBytes int64) (http.Handler, *Processor, *logrusTestHook, *gorm.DB) {
	t.Helper()
	db := newConfirmTestDB(t)
	log := logrus.New()
	log.SetOutput(io.Discard)
	hook := &logrusTestHook{}
	log.AddHook(hook)
	allow, err := ParseAllowlist(DefaultAllowedContentTypes)
	if err != nil {
		t.Fatalf("ParseAllowlist: %v", err)
	}
	r := chi.NewRouter()
	r.Group(InitializeRoutes(log, db, store, variants, maxUploadBytes, allow))
	return r, NewProcessor(log, NewProvider(db), NewAdministrator(db), store, variants, allow), hook, db
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

func TestGetContent_pdfIsAttachmentWithNosniff(t *testing.T) {
	store := &fakeStore{bucket: "myfleet-media", getBody: []byte("%PDF-1.7")}
	router, proc, _ := testRouterWithDB(t, store, 1024)

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
	router, proc, _ := testRouterWithDB(t, store, 1024)

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
	router, proc, db := testRouterWithDB(t, store, 1024)

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

// forceContentType rewrites a stored content type directly, which is the only
// way to reproduce a pre-allowlist row now that InitUpload normalises.
func forceContentType(t *testing.T, db *gorm.DB, id, contentType string) {
	t.Helper()
	if err := db.Model(&Entity{}).Where("id = ?", id).
		Update("content_type", contentType).Error; err != nil {
		t.Fatalf("force content type: %v", err)
	}
}

// initBody builds the JSON:API envelope POST /media expects.
func initBody(contentType, filename string) io.Reader {
	return strings.NewReader(`{"data":{"attributes":{"contentType":"` + contentType +
		`","originalFilename":"` + filename + `"}}}`)
}

func TestInitUpload_pdfIsAccepted(t *testing.T) {
	router, _, _ := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

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
	router, _, _ := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

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
	router, _, _ := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodPost, "/media", initBody("", "mystery")))

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

// text/csv; charset=utf-8 is what a browser actually sends. The parameters are
// discarded and the bare type is what gets stored (design D10).
func TestInitUpload_normalizesAndStoresBareType(t *testing.T) {
	_, proc, _ := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	m, err := proc.InitUpload("fleet-a", "u1", "TEXT/CSV; charset=utf-8", "mileage.csv")
	if err != nil {
		t.Fatalf("InitUpload: %v", err)
	}
	if m.ContentType() != "text/csv" {
		t.Fatalf("stored contentType = %q, want text/csv", m.ContentType())
	}
}

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
	_, proc, db := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

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
	_, proc, db := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

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
	_, proc, db := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

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
	_, proc, db := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)
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
	_, _, db := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	rec := getInternal(t, internalRouter(t, db), "/internal/media?ids=x")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
}

func TestInternalMedia_emptyIdsReturnsEmptyList(t *testing.T) {
	_, _, db := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

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
	_, _, db := testRouterWithDB(t, &fakeStore{bucket: "myfleet-media"}, 1024)

	ids := make([]string, MaxInternalLookupIDs+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	rec := getInternal(t, internalRouter(t, db), "/internal/media?fleet_id=fleet-a&ids="+strings.Join(ids, ","))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
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

// A variant response carries the same download hardening as the original.
// The two features landed on separate branches — variants, and the
// allowlist/nosniff/Content-Disposition work — so nothing on either branch
// alone covered their intersection. nosniff in particular has to be on EVERY
// response: a variant is bytes served from the application's origin like any
// other, and "it was re-encoded by our own worker" is an argument about how it
// got there, not about what a browser will do with it.
func TestGetContent_variantIsHardenedLikeTheOriginal(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	// A thumbnail is a renderable image, so it is served inline under the
	// ORIGINAL's filename — the variant has no filename of its own, and
	// offering "photo.png" is what the user recognises.
	if got := rec.Header().Get("Content-Disposition"); got != `inline; filename="photo.png"` {
		t.Fatalf("Content-Disposition = %q", got)
	}
}

// A variant whose recorded content type is not on the allowlist must degrade
// exactly like a legacy original: octet-stream plus attachment. The worker only
// ever writes jpeg today, so this is a guard against a future variant format
// being served as a type the browser is willing to execute.
func TestGetContent_offAllowlistVariantDegradesToAttachment(t *testing.T) {
	store := &fakeStore{
		bucket:    "myfleet-media",
		getBody:   []byte("original-bytes"),
		getBodies: map[string][]byte{"fleet-a/odd.bin": []byte("<script>alert(1)</script>")},
	}
	variants := &fakeVariants{refs: map[string]VariantRef{
		"thumbnail": {ObjectKey: "fleet-a/odd.bin", ContentType: "text/html"},
	}}
	router, proc := testRouterWithVariants(t, store, variants, 1024)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=thumbnail", nil))

	if got := rec.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q, want application/octet-stream", got)
	}
	if got := rec.Header().Get("Content-Disposition"); !strings.HasPrefix(got, "attachment;") {
		t.Fatalf("Content-Disposition = %q, want attachment", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
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

// TestGetContent_downgradedCardIsNotStored is the whole point of the cache
// change. thumbnailRouter seeds thumbnail and display but no card, so
// ?variant=card downgrades. Those soft bytes must not be stored under the
// sharp image's URL: the card generation the downgrade schedules usually
// completes within seconds, and nothing can invalidate a cache entry that
// recorded no substitution.
func TestGetContent_downgradedCardIsNotStored(t *testing.T) {
	router, proc, _ := thumbnailRouter(t)
	id := seedStoredObject(t, proc, "fleet-a", []byte("original-bytes"))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, memberRequest(http.MethodGet, "/media/"+id+"/content?variant=card", nil))

	if cc := rec.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store — a soft image must never be stored under the card URL", cc)
	}
	// Everything else is byte-identical to what this request returns today:
	// the substitution stays undetectable by the client (FR-DG-4).
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "thumb-bytes" {
		t.Fatalf("body = %q, want thumb-bytes", rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want the thumbnail row's own image/jpeg", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `inline; filename="photo.png"` {
		t.Fatalf("Content-Disposition = %q, want inline with the original's filename", cd)
	}
	if xcto := rec.Header().Get("X-Content-Type-Options"); xcto != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff on every response", xcto)
	}
	if cl := rec.Header().Get("Content-Length"); cl != "" {
		t.Fatalf("Content-Length = %q, want it omitted — a variant records no byte count", cl)
	}
	// No new response header may be introduced: the four above are the entire
	// header set a content response carries, downgraded or not.
	if n := len(rec.Header()); n != 4 {
		t.Fatalf("response carries %d headers (%v), want exactly Content-Type, X-Content-Type-Options, "+
			"Content-Disposition and Cache-Control — the downgrade must stay invisible to clients", n, rec.Header())
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
