package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestObjectKey_namespacedByFleet(t *testing.T) {
	k := ObjectKey("f1", "id-1", "My Receipt.pdf")
	if !strings.HasPrefix(k, "f1/id-1/") || strings.Contains(k, " ") {
		t.Fatalf("unexpected key %q", k)
	}
}

func TestObjectKey_slugifiesUnsafeChars(t *testing.T) {
	k := ObjectKey("fleet-a", "obj-2", "Some Weird/Name*?.JPG")
	if !strings.HasPrefix(k, "fleet-a/obj-2/") {
		t.Fatalf("expected fleet/obj prefix, got %q", k)
	}
	slug := strings.TrimPrefix(k, "fleet-a/obj-2/")
	for _, c := range []string{" ", "/", "*", "?"} {
		if strings.Contains(slug, c) {
			t.Fatalf("slug %q still contains unsafe char %q", slug, c)
		}
	}
	if slug != strings.ToLower(slug) {
		t.Fatalf("slug should be lowercase, got %q", slug)
	}
}

func TestObjectKey_emptyFilenameFallsBack(t *testing.T) {
	k := ObjectKey("f1", "id-1", "")
	if !strings.HasPrefix(k, "f1/id-1/") || strings.HasSuffix(k, "/") {
		t.Fatalf("empty filename must fall back to a non-empty slug, got %q", k)
	}
}

// --- upload allocation bound (see uploadPartSize) ----------------------------

// TestPutOptions_partSizeBoundsUnknownLengthAllocation pins the one part of the
// allocation that is observable without a live MinIO: the PartSize we hand the
// SDK. PartSize == 0 is what makes minio-go pick a 528 MiB buffer for an
// unknown-length (chunked) body, which OOM-kills a 256 MiB pod before a single
// body byte is read. The consequences of the value are then measured through
// minio's own exported OptimalPartInfo rather than trusted from its doc
// comment.
func TestPutOptions_partSizeBoundsUnknownLengthAllocation(t *testing.T) {
	opts := putOptions("image/jpeg")
	if opts.ContentType != "image/jpeg" {
		t.Fatalf("ContentType = %q, want image/jpeg", opts.ContentType)
	}
	if opts.PartSize == 0 {
		t.Fatal("PartSize must be set explicitly; 0 makes minio-go allocate a 528 MiB part for an unknown-length body")
	}

	// The buffer minio-go allocates up front on the size=-1 path.
	totalParts, partSize, _, err := minio.OptimalPartInfo(-1, opts.PartSize)
	if err != nil {
		t.Fatalf("OptimalPartInfo(-1, %d): %v", opts.PartSize, err)
	}

	// Comfortably inside the container's 256 MiB limit, with room for several
	// concurrent uploads (the buffer is per in-flight upload).
	const maxAcceptableBuffer = 8 << 20 // 8 MiB
	if partSize > maxAcceptableBuffer {
		t.Fatalf("unknown-length part buffer = %d bytes, want <= %d", partSize, maxAcceptableBuffer)
	}

	// ...but still enough parts to carry an object of at least the upload cap
	// (MEDIA_MAX_UPLOAD_BYTES), or legitimate chunked uploads would break.
	const uploadCapBytes = 26214400 // 25 MiB
	if capacity := int64(totalParts) * partSize; capacity < uploadCapBytes {
		t.Fatalf("unknown-length capacity = %d bytes across %d parts, want >= %d", capacity, totalParts, uploadCapBytes)
	}

	// A known-size body at the cap must also stay within the same bound.
	_, knownPartSize, _, err := minio.OptimalPartInfo(uploadCapBytes, opts.PartSize)
	if err != nil {
		t.Fatalf("OptimalPartInfo(%d, %d): %v", uploadCapBytes, opts.PartSize, err)
	}
	if knownPartSize > maxAcceptableBuffer {
		t.Fatalf("known-size part buffer = %d bytes, want <= %d", knownPartSize, maxAcceptableBuffer)
	}
}

// --- GetObject must not report success for an object that is not there -------

// newTestClient points a storage.Client at an httptest stand-in for MinIO. The
// region is pinned so the SDK never issues a GetBucketLocation round trip.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}
	mc, err := minio.New(u.Host, &minio.Options{
		Creds:  credentials.NewStaticV4("ak", "sk", ""),
		Secure: false,
		Region: "us-east-1",
	})
	if err != nil {
		t.Fatalf("minio.New: %v", err)
	}
	return &Client{mc: mc, bucket: "media"}
}

// TestGetObject_missingKeyIsErrObjectNotFound drives the real minio-go client
// against a server that 404s, which is what happens whenever a media row was
// created by POST /media but its bytes were never successfully PUT.
//
// minio.Client.GetObject is lazy — it returns a handle without contacting the
// server — so before the priming read this returned (reader, nil) and the HTTP
// layer committed a 200 with an empty body. It must be an error instead.
func TestGetObject_missingKeyIsErrObjectNotFound(t *testing.T) {
	var gets int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gets++
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?>`+
			`<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message>`+
			`<Key>fleet-a/obj-1/photo.jpg</Key><BucketName>media</BucketName></Error>`)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	rc, err := c.GetObject(context.Background(), "fleet-a/obj-1/photo.jpg")
	if !errors.Is(err, ErrObjectNotFound) {
		if rc != nil {
			_ = rc.Close()
		}
		t.Fatalf("GetObject on a missing key = %v, want ErrObjectNotFound", err)
	}
	if rc != nil {
		t.Fatal("GetObject must not return a reader alongside an error")
	}
	if gets == 0 {
		t.Fatal("GetObject must actually contact the server before reporting success")
	}
}

// TestGetObject_streamsExistingObjectIntact covers the other half: priming must
// not consume, duplicate, or reorder bytes. The payload is deliberately larger
// than getProbeSize so the assertion spans the seam between the primed head and
// the rest of the stream.
func TestGetObject_streamsExistingObjectIntact(t *testing.T) {
	payload := make([]byte, getProbeSize*3+7)
	for i := range payload {
		payload[i] = byte(i % 251)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("ETag", `"deadbeef"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(payload)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	rc, err := c.GetObject(context.Background(), "fleet-a/obj-1/photo.jpg")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("streamed %d bytes, want %d (identical content)", len(got), len(payload))
	}
}

// TestGetObject_emptyObjectIsNotAnError distinguishes "the key exists and holds
// zero bytes" (a legitimate 200) from "the key is not there" (404). The probing
// read sees io.EOF in both shapes, so the distinction has to come from the
// server's status, not from the byte count.
func TestGetObject_emptyObjectIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("ETag", `"d41d8cd98f00b204e9800998ecf8427e"`)
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	rc, err := c.GetObject(context.Background(), "fleet-a/obj-1/empty.bin")
	if err != nil {
		t.Fatalf("GetObject on a zero-byte object: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("read empty object: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("read %d bytes from a zero-byte object", len(got))
	}
}
