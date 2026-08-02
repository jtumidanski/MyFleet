package mailer

import (
	"fmt"
	"time"

	"github.com/jtumidanski/myfleet/packages/shared-go/config"
)

// The closed set of TLS modes (Open Q3, design §6.2).
//
//	starttls — plaintext connect, STARTTLS, FAIL if the server does not offer it
//	tls      — implicit TLS from the first byte
//	none     — plaintext; the ONLY mode that permits an unauthenticated session,
//	           and legal only for a local relay (Mailpit). tools/check-manifests.sh
//	           keeps it out of the main overlay (FR-DEV-2).
const (
	TLSModeStartTLS = "starttls"
	TLSModeTLS      = "tls"
	TLSModeNone     = "none"
)

// Config is the mail transport configuration. Every value is read through
// packages/shared-go/config; nothing here calls os.Getenv (FR-CFG-3).
type Config struct {
	Enabled      bool
	Host         string
	Port         int
	TLSMode      string
	FromAddress  string
	FromName     string
	Username     string
	Password     string
	PublicWebURL string
	Timeout      time.Duration
	SendAttempts int
	RetryBase    time.Duration
}

// ConfigFromEnv builds the transport config, failing at STARTUP on anything
// misconfigured rather than surfacing it as a per-message failure hours later
// (FR-CFG-5).
//
// When SMTP_ENABLED is false it reads nothing else and returns a disabled
// config: a cluster without relay credentials is a documented no-op, not a
// crash loop or a retry storm (FR-CFG-4).
func ConfigFromEnv() Config {
	if config.Get("SMTP_ENABLED", "false") != "true" {
		return Config{Enabled: false}
	}

	cfg := Config{
		Enabled:      true,
		Host:         config.MustGet("SMTP_HOST"),
		Port:         config.GetInt("SMTP_PORT", 587),
		TLSMode:      config.Get("SMTP_TLS_MODE", TLSModeStartTLS),
		FromAddress:  config.MustGet("SMTP_FROM_ADDRESS"),
		FromName:     config.Get("SMTP_FROM_NAME", "MyFleet"),
		Username:     config.Get("SMTP_USERNAME", ""),
		Password:     config.Get("SMTP_PASSWORD", ""),
		PublicWebURL: config.MustGet("PUBLIC_WEB_URL"),
		Timeout:      time.Duration(config.GetInt("SMTP_TIMEOUT_SECONDS", 10)) * time.Second,
		SendAttempts: config.GetInt("SMTP_SEND_ATTEMPTS", 4),
		RetryBase:    time.Duration(config.GetInt("SMTP_RETRY_BASE_SECONDS", 2)) * time.Second,
	}

	switch cfg.TLSMode {
	case TLSModeStartTLS, TLSModeTLS, TLSModeNone:
	default:
		panic(fmt.Sprintf("SMTP_TLS_MODE %q must be one of %q, %q, %q",
			cfg.TLSMode, TLSModeStartTLS, TLSModeTLS, TLSModeNone))
	}

	// Unauthenticated submission to a real relay is rejected on every message.
	// Fail now rather than emitting one permanent failure per invite forever.
	if cfg.TLSMode != TLSModeNone && (cfg.Username == "" || cfg.Password == "") {
		panic("SMTP_USERNAME and SMTP_PASSWORD are required unless SMTP_TLS_MODE is \"none\"")
	}
	// The reverse combination is just as broken: net/smtp's PlainAuth refuses to
	// transmit credentials over an unencrypted connection, so every send would
	// fail. That failure classifies as TRANSIENT, burning the full retry budget
	// before the mail is permanently dropped — a confusing, expensive way to
	// discover a config mistake. Fail now instead. Empty credentials with
	// TLSModeNone stay legal: that is exactly how compose and the k3s-local
	// overlay point at the unauthenticated Mailpit sink (FR-DEV-2).
	if cfg.TLSMode == TLSModeNone && (cfg.Username != "" || cfg.Password != "") {
		panic("SMTP_USERNAME/SMTP_PASSWORD cannot be set when SMTP_TLS_MODE is \"none\": " +
			"net/smtp refuses to send credentials over an unencrypted connection, so every message would fail")
	}
	if cfg.SendAttempts < 1 {
		panic("SMTP_SEND_ATTEMPTS must be at least 1")
	}

	return cfg
}
