package mailer

import (
	"testing"
	"time"
)

// FR-CFG-4: disabled is the default, and a disabled config reads nothing else.
// A cluster with no relay credentials must be a no-op, not a crash loop.
func TestConfigFromEnv_disabledByDefault(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.Enabled {
		t.Fatal("SMTP must default to disabled")
	}
}

func TestConfigFromEnv_enabledReadsEverything(t *testing.T) {
	t.Setenv("SMTP_ENABLED", "true")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_TLS_MODE", "starttls")
	t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
	t.Setenv("SMTP_FROM_NAME", "MyFleet")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "pass")
	t.Setenv("PUBLIC_WEB_URL", "https://example.com")

	cfg := ConfigFromEnv()
	if !cfg.Enabled || cfg.Host != "smtp.example.com" || cfg.Port != 587 {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.TLSMode != TLSModeStartTLS || cfg.FromAddress != "invites@example.com" {
		t.Fatalf("cfg=%+v", cfg)
	}
	if cfg.PublicWebURL != "https://example.com" {
		t.Fatalf("cfg=%+v", cfg)
	}
	// Defaults that keep a black-holed relay from hanging the consumer.
	if cfg.Timeout != 10*time.Second || cfg.SendAttempts != 4 || cfg.RetryBase != 2*time.Second {
		t.Fatalf("retry defaults wrong: %+v", cfg)
	}
}

// FR-CFG-5: a missing required key is a STARTUP failure, not a per-message
// failure discovered hours later.
func TestConfigFromEnv_missingRequiredKeyPanics(t *testing.T) {
	for _, missing := range []string{"SMTP_HOST", "SMTP_FROM_ADDRESS", "PUBLIC_WEB_URL"} {
		t.Run(missing, func(t *testing.T) {
			t.Setenv("SMTP_ENABLED", "true")
			t.Setenv("SMTP_HOST", "smtp.example.com")
			t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
			t.Setenv("PUBLIC_WEB_URL", "https://example.com")
			t.Setenv("SMTP_TLS_MODE", "starttls")
			t.Setenv("SMTP_USERNAME", "user")
			t.Setenv("SMTP_PASSWORD", "pass")
			t.Setenv(missing, "")

			defer func() {
				if recover() == nil {
					t.Fatalf("missing %s must panic at startup", missing)
				}
			}()
			ConfigFromEnv()
		})
	}
}

// Open Q3: the mode set is closed. An unknown value is a startup panic, not a
// runtime surprise on the first invite.
func TestConfigFromEnv_rejectsUnknownTLSMode(t *testing.T) {
	t.Setenv("SMTP_ENABLED", "true")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
	t.Setenv("PUBLIC_WEB_URL", "https://example.com")
	t.Setenv("SMTP_USERNAME", "user")
	t.Setenv("SMTP_PASSWORD", "pass")
	t.Setenv("SMTP_TLS_MODE", "ssl")

	defer func() {
		if recover() == nil {
			t.Fatal("an unknown SMTP_TLS_MODE must panic at startup")
		}
	}()
	ConfigFromEnv()
}

// Empty credentials against a real relay mean every message is rejected. Fail
// at startup instead. They are legal only for the plaintext local relay.
func TestConfigFromEnv_credentialsRequiredUnlessModeIsNone(t *testing.T) {
	base := func(t *testing.T, mode string) {
		t.Setenv("SMTP_ENABLED", "true")
		t.Setenv("SMTP_HOST", "smtp.example.com")
		t.Setenv("SMTP_FROM_ADDRESS", "invites@example.com")
		t.Setenv("PUBLIC_WEB_URL", "https://example.com")
		t.Setenv("SMTP_TLS_MODE", mode)
		t.Setenv("SMTP_USERNAME", "")
		t.Setenv("SMTP_PASSWORD", "")
	}

	t.Run("starttls without credentials panics", func(t *testing.T) {
		base(t, TLSModeStartTLS)
		defer func() {
			if recover() == nil {
				t.Fatal("expected a panic")
			}
		}()
		ConfigFromEnv()
	})

	t.Run("none without credentials is legal", func(t *testing.T) {
		base(t, TLSModeNone)
		cfg := ConfigFromEnv()
		if cfg.Username != "" || cfg.TLSMode != TLSModeNone {
			t.Fatalf("cfg=%+v", cfg)
		}
	})
}
