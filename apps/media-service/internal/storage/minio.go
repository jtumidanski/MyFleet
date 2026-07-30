// Package storage wraps the MinIO client used by media-service. Buckets are
// always private; bytes are exchanged with clients exclusively by proxying
// through media-service, never by presigned URL — MinIO is a shared cluster
// service and is not exposed outside the cluster. Object keys are namespaced
// by fleet so a single bucket can hold every fleet's media without
// collisions.
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ErrObjectNotFound reports that the requested key does not exist in the
// bucket. GetObject returns it in place of the SDK's raw ErrorResponse so
// callers can map "the bytes were never stored" to a 404 without importing
// minio's error types.
var ErrObjectNotFound = errors.New("object not found")

// uploadPartSize is the multipart part size handed to minio-go, and therefore
// the size of the single buffer it allocates per in-flight upload.
//
// It must be set explicitly. With PartSize left at 0 and an unknown-length
// body (size == -1, i.e. a chunked request), minio-go calls
// OptimalPartInfo(-1, 0), which assumes a 5 TiB object and picks a 528 MiB
// part — allocated with `make([]byte, partSize)` *before* the first body byte
// is read. The pod's memory limit is 256 MiB, so that single allocation is an
// immediate OOM kill. http.MaxBytesReader bounds bytes *read*; it cannot
// bound a buffer allocated up front. Measured against minio-go v7.1.0:
//
//	OptimalPartInfo(-1, 0)      -> totalParts=9930  partSize=553648128 (528.0 MiB)
//	OptimalPartInfo(-1, 5 MiB)  -> totalParts=10000 partSize=5242880   (5.0 MiB)
//	OptimalPartInfo(-1, 16 MiB) -> totalParts=10000 partSize=16777216  (16.0 MiB)
//
// 5 MiB is `absMinPartSize`, the smallest value S3 (and OptimalPartInfo)
// accepts. It is chosen over the SDK's 16 MiB default because the buffer is
// per concurrent upload: at 5 MiB, 20 simultaneous uploads cost 100 MiB of the
// 256 MiB limit; at 16 MiB the same 20 uploads would cost 320 MiB and blow it.
// The 10000-part ceiling still allows a ~48.8 GiB object on the unknown-length
// path, ~2000x the 25 MiB upload cap (MEDIA_MAX_UPLOAD_BYTES = 26214400), so a
// legitimate upload never runs out of parts — MaxBytesReader trips at 25 MiB,
// during part 5 of 10000.
const uploadPartSize = 5 * 1024 * 1024

// getProbeSize is how many bytes GetObject pulls eagerly to force the lazy
// minio reader to actually issue its GET. It matches io.Copy's default buffer
// size, so priming costs no more than the first copy iteration would.
const getProbeSize = 32 * 1024

// ObjectKey builds the canonical object key for a media object:
// `fleetID/id/<slug>`. The filename is slugified (spaces and unsafe characters
// replaced with '-', lowercased) so the key is URL- and path-safe. An empty or
// fully-stripped filename falls back to "file".
func ObjectKey(fleetID, id, filename string) string {
	return path.Join(fleetID, id, slugify(filename))
}

// slugify lowercases the filename and replaces every character that is not an
// ASCII letter, digit, dot, dash, or underscore with a single dash. Runs of
// dashes are collapsed and leading/trailing dashes trimmed. The file extension
// (if any) is preserved through the same sanitization.
func slugify(filename string) string {
	s := strings.ToLower(strings.TrimSpace(filename))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_':
			b.WriteRune(r)
			prevDash = false
		default:
			// Collapse any run of unsafe chars into a single '-'.
			if !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "file"
	}
	return out
}

// Client wraps *minio.Client with the media-service bucket bound in, exposing
// only the operations the service needs.
type Client struct {
	mc     *minio.Client
	bucket string
}

// Config holds the connection settings for the MinIO client.
type Config struct {
	Endpoint  string // host:port, no scheme
	AccessKey string
	SecretKey string
	UseSSL    bool
	Bucket    string
}

// New constructs the wrapped client and ensures the bucket exists (private).
// It never sets any public-read policy; bytes are exchanged with clients
// exclusively by proxying through media-service, never by presigned URL —
// MinIO is a shared cluster service and is not exposed outside the cluster.
func New(ctx context.Context, cfg Config) (*Client, error) {
	mc, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, err
	}
	c := &Client{mc: mc, bucket: cfg.Bucket}
	if err := c.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// Bucket returns the bucket name this client is bound to.
func (c *Client) Bucket() string { return c.bucket }

// ensureBucket creates the bucket if it does not already exist. The bucket is
// left private (no anonymous policy is applied).
func (c *Client) ensureBucket(ctx context.Context) error {
	exists, err := c.mc.BucketExists(ctx, c.bucket)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	return c.mc.MakeBucket(ctx, c.bucket, minio.MakeBucketOptions{})
}

// PutObject uploads bytes to the bucket under key. size may be -1 for a body
// of unknown length (a chunked request); PartSize is always set so the SDK's
// buffer stays bounded on that path — see uploadPartSize.
func (c *Client) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, r, size, putOptions(contentType))
	return err
}

// putOptions builds the options every upload uses. Split out from PutObject so
// the PartSize that bounds minio-go's buffer is assertable without a live
// MinIO — it is the one thing about the allocation that is observable in a
// unit test.
func putOptions(contentType string) minio.PutObjectOptions {
	return minio.PutObjectOptions{
		ContentType: contentType,
		PartSize:    uploadPartSize,
	}
}

// GetObject opens an object for reading. The caller must Close the returned
// reader.
//
// minio.Client.GetObject is lazy: it validates the bucket/object *names* and
// hands back a handle without contacting the server, so a key that does not
// exist still comes back with err == nil and only fails on the first Read. A
// caller that writes its status line before copying therefore commits a 200 to
// an object that is not there. GetObject closes that gap by pulling the first
// chunk here, while the caller can still choose a status code, and replaying
// those bytes ahead of the rest of the stream. The response stays streamed —
// only getProbeSize bytes are ever held — and no byte is read twice or
// dropped. A missing key becomes ErrObjectNotFound; every other failure is
// returned unchanged.
func (c *Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	head := make([]byte, getProbeSize)
	// io.EOF (empty object) and io.ErrUnexpectedEOF (object shorter than the
	// probe) both mean "readable, just small" — only a real transport/S3
	// failure aborts.
	n, rerr := io.ReadFull(obj, head)
	if rerr != nil && !errors.Is(rerr, io.EOF) && !errors.Is(rerr, io.ErrUnexpectedEOF) {
		_ = obj.Close()
		if minio.ToErrorResponse(rerr).Code == minio.NoSuchKey {
			return nil, ErrObjectNotFound
		}
		return nil, rerr
	}
	return &primedObject{r: io.MultiReader(bytes.NewReader(head[:n]), obj), c: obj}, nil
}

// primedObject re-attaches the bytes GetObject read eagerly to the front of the
// underlying object stream, while still closing the real object.
type primedObject struct {
	r io.Reader
	c io.Closer
}

func (p *primedObject) Read(b []byte) (int, error) { return p.r.Read(b) }
func (p *primedObject) Close() error               { return p.c.Close() }

// RemoveObject deletes an object from the bucket (used by the purge job).
func (c *Client) RemoveObject(ctx context.Context, key string) error {
	return c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
}
