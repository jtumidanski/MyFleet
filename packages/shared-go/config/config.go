// Package config centralizes environment-variable access. Handlers must never
// call os.Getenv directly (design §6: "env only; no os.Getenv in handlers").
package config

import (
	"fmt"
	"os"
	"strconv"
)

func Get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func MustGet(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		panic(fmt.Sprintf("required env var %q is not set", key))
	}
	return v
}

func GetInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
