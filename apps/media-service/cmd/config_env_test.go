package main

import "testing"

func TestEnvFirstPrefersS3ThenMinio(t *testing.T) {
	t.Setenv("S3_ENDPOINT", "new:9000")
	t.Setenv("MINIO_ENDPOINT", "old:9000")
	if got := envFirst("", "S3_ENDPOINT", "MINIO_ENDPOINT"); got != "new:9000" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("S3_ENDPOINT", "")
	if got := envFirst("", "S3_ENDPOINT", "MINIO_ENDPOINT"); got != "old:9000" {
		t.Fatalf("fallback: got %q", got)
	}
	t.Setenv("MINIO_ENDPOINT", "")
	if got := envFirst("false", "S3_USE_SSL", "MINIO_USE_SSL"); got != "false" {
		t.Fatalf("default: got %q", got)
	}
}

func TestMustEnvFirstFailsWhenBothUnset(t *testing.T) {
	t.Setenv("S3_ACCESS_KEY", "")
	t.Setenv("MINIO_ACCESS_KEY", "")
	if _, err := mustEnvFirst("S3_ACCESS_KEY", "MINIO_ACCESS_KEY"); err == nil {
		t.Fatal("expected error when neither name is set")
	}
}
