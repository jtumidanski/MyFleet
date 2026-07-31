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
// store, and injects the identity the JWT middleware would normally put on the
// context so the handlers can be exercised end to end.
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
	router, proc, _ := testRouter(t, store, cap)

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
	router, proc, _ := testRouter(t, store, 1024)

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
	router, proc, _ := testRouter(t, store, 1024)

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
	router, proc, _ := testRouter(t, store, 1024)

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
	router, proc, _ := testRouter(t, store, 1024)

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
	router, proc, _ := testRouter(t, store, 1024)

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
