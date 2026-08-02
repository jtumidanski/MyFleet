// Package mailer composes and delivers transactional email. It is
// INFRASTRUCTURE, not a DDD domain — no Model/Entity/Provider/Administrator,
// matching internal/inbox and internal/fleetclient.
//
// It knows nothing about invites beyond a struct of already-resolved fields;
// mailconsumer knows nothing about SMTP. That split is what lets every test run
// without a socket (FR-MAIL-1, FR-DEV-4).
package mailer

import "context"

// Message is one already-rendered email. Both parts are required: a single-part
// HTML-only body materially raises the spam score (FR-TPL-1).
//
// SECURITY: HTML/Text contain the accept URL, which contains the invite token.
// This type deliberately has NO String() or LogValue() method, and no code path
// passes a Message to a logger (FR-OBS-2, design §6.6).
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Sender is the transport seam. The SMTP implementation and the in-memory fake
// both satisfy it; every test uses the fake.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// PermanentError marks a failure that retrying cannot fix: a relay rejecting
// the recipient address, or a malformed address caught before any dial. The
// consumer marks the ledger and stops — retrying a rejected mailbox forever is
// what wedges a partition (FR-MAIL-5).
//
// It wraps the SMTP error only, never the message body.
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }
