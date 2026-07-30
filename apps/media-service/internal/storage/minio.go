// Package storage wraps the MinIO client used by media-service. Buckets are
// always private; bytes are exchanged with clients exclusively by proxying
// through media-service, never by presigned URL — MinIO is a shared cluster
// service and is not exposed outside the cluster. Object keys are namespaced
// by fleet so a single bucket can hold every fleet's media without
// collisions.
package storage

import (
	"context"
	"io"
	"path"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

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

// PutObject uploads bytes to the bucket under key (used by the variant worker).
func (c *Client) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	_, err := c.mc.PutObject(ctx, c.bucket, key, r, size, minio.PutObjectOptions{ContentType: contentType})
	return err
}

// GetObject opens an object for reading (used by the variant worker to download
// the original before resizing). The caller must Close the returned reader.
func (c *Client) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := c.mc.GetObject(ctx, c.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// RemoveObject deletes an object from the bucket (used by the purge job).
func (c *Client) RemoveObject(ctx context.Context, key string) error {
	return c.mc.RemoveObject(ctx, c.bucket, key, minio.RemoveObjectOptions{})
}
