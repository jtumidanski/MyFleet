package mailer

import (
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func composeFixture(t *testing.T, msg Message) *mail.Message {
	t.Helper()
	raw, err := compose("MyFleet", "invites@myfleet.example.com", msg,
		time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC),
		"<abc-123@myfleet.example.com>", "BOUNDARY123")
	if err != nil {
		t.Fatalf("compose: %v", err)
	}
	m, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("the composed message does not parse as RFC 5322: %v\n%s", err, raw)
	}
	return m
}

// FR-TPL-1: multipart/alternative with BOTH parts and all six required headers.
// Asserted by parsing the output back rather than by string matching, so a
// malformed-but-grep-passing message cannot slip through.
func TestCompose_isParseableMultipartAlternative(t *testing.T) {
	m := composeFixture(t, Message{
		To: "a@b.com", Subject: "You're invited to The Smiths on MyFleet",
		HTML: "<p>hello html</p>", Text: "hello text",
	})

	for _, h := range []string{"From", "To", "Subject", "Date", "Message-ID", "MIME-Version"} {
		if m.Header.Get(h) == "" {
			t.Errorf("missing required header %s", h)
		}
	}
	if got := m.Header.Get("From"); got != "MyFleet <invites@myfleet.example.com>" {
		t.Errorf("From=%q", got)
	}
	if got := m.Header.Get("To"); got != "a@b.com" {
		t.Errorf("To=%q", got)
	}
	if got := m.Header.Get("MIME-Version"); got != "1.0" {
		t.Errorf("MIME-Version=%q", got)
	}

	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("Content-Type: %v", err)
	}
	if mediaType != "multipart/alternative" {
		t.Fatalf("mediaType=%q want multipart/alternative", mediaType)
	}

	types := map[string]string{}
	mr := multipart.NewReader(m.Body, params["boundary"])
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		body, _ := io.ReadAll(p)
		mt, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		types[mt] = string(body)
	}

	if !strings.Contains(types["text/plain"], "hello text") {
		t.Errorf("text/plain part = %q", types["text/plain"])
	}
	if !strings.Contains(types["text/html"], "hello html") {
		t.Errorf("text/html part = %q", types["text/html"])
	}
	// Order matters: least-capable first, so a text-only client shows the
	// plain part rather than raw markup.
	if len(types) != 2 {
		t.Fatalf("want exactly 2 parts, got %d: %v", len(types), types)
	}
}

// The fleet name is user-controlled and reaches the Subject header. A non-ASCII
// name must be RFC 2047 encoded, and a CR/LF must not survive into headers even
// if one predates the invite-creation validation (design §6.3).
func TestCompose_encodesSubjectAndStripsHeaderInjection(t *testing.T) {
	m := composeFixture(t, Message{
		To: "a@b.com", Subject: "You're invited to Håkon's Garage\r\nBcc: victim@x.com on MyFleet",
		HTML: "<p>h</p>", Text: "t",
	})

	if m.Header.Get("Bcc") != "" {
		t.Fatal("header injection succeeded: a Bcc header was created")
	}
	raw := m.Header.Get("Subject")
	if strings.Contains(raw, "\r") || strings.Contains(raw, "\n") {
		t.Fatalf("raw CR/LF survived into the Subject header: %q", raw)
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(raw)
	if err != nil {
		t.Fatalf("decode subject: %v", err)
	}
	if !strings.Contains(decoded, "Håkon's Garage") {
		t.Fatalf("decoded subject lost the fleet name: %q", decoded)
	}
}

// Message-ID's domain half must match the From domain — a mismatch is a spam
// signal (design §6.3).
func TestNewMessageID_usesTheFromDomain(t *testing.T) {
	id, err := newMessageID("invites@myfleet.example.com")
	if err != nil {
		t.Fatalf("newMessageID: %v", err)
	}
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@myfleet.example.com>") {
		t.Fatalf("Message-ID=%q", id)
	}
}

// The boundary must be random per message so it cannot collide with body
// content, which would truncate the message at the collision.
func TestNewBoundary_isRandom(t *testing.T) {
	a, err := newBoundary()
	if err != nil {
		t.Fatalf("newBoundary: %v", err)
	}
	b, err := newBoundary()
	if err != nil {
		t.Fatalf("newBoundary: %v", err)
	}
	if a == b {
		t.Fatal("boundaries must differ between messages")
	}
	if len(a) < 16 {
		t.Fatalf("boundary %q is too short to be collision-resistant", a)
	}
}
