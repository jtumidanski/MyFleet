package main

import (
	"fmt"
	"os"
	"strings"
)

// envFirst returns the first non-empty value among keys, else def. Object
// store settings are read under their S3_* names with the pre-2026-09
// MINIO_* names as a one-release fallback.
func envFirst(def string, keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return def
}

func mustEnvFirst(keys ...string) (string, error) {
	if v := envFirst("", keys...); v != "" {
		return v, nil
	}
	return "", fmt.Errorf("missing required env: one of %s", strings.Join(keys, ", "))
}
