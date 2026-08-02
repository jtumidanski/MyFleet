package mailer

import (
	"strings"
	"testing"
	"time"
)

func testInviteData() InviteData {
	return InviteData{
		To:        "a@b.com",
		FleetName: "The Smiths",
		Role:      "member",
		AcceptURL: "https://myfleet.example.com/invites/deadbeef/accept",
		ExpiresAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
	}
}

func TestRenderInvite(t *testing.T) {
	msg, err := RenderInvite(testInviteData())
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if msg.To != "a@b.com" {
		t.Fatalf("To=%q", msg.To)
	}
	// FR-TPL-4: fixed string plus the fleet name, and NEVER the token.
	if msg.Subject != "You're invited to The Smiths on MyFleet" {
		t.Fatalf("Subject=%q", msg.Subject)
	}
	if strings.Contains(msg.Subject, "deadbeef") {
		t.Fatal("the token must never appear in the subject")
	}
	// FR-TPL-1: both parts exist and both are usable on their own.
	for name, part := range map[string]string{"html": msg.HTML, "text": msg.Text} {
		if part == "" {
			t.Fatalf("%s part is empty", name)
		}
		if !strings.Contains(part, "https://myfleet.example.com/invites/deadbeef/accept") {
			t.Fatalf("%s part is missing the accept URL: %s", name, part)
		}
		if !strings.Contains(part, "The Smiths") {
			t.Fatalf("%s part is missing the fleet name", name)
		}
		if !strings.Contains(part, "member") {
			t.Fatalf("%s part is missing the role", name)
		}
		// FR-TPL-3: the expiry must be legible, not a raw timestamp.
		if !strings.Contains(part, "August 9, 2026") {
			t.Fatalf("%s part is missing a legible expiry: %s", name, part)
		}
		// FR-TPL-6.
		if !strings.Contains(strings.ToLower(part), "ignore") {
			t.Fatalf("%s part is missing the 'ignore if unexpected' line", name)
		}
	}
}

// FR-TPL-5: the fleet name is user-controlled input. html/template must escape
// it contextually; the raw script tag must not survive into the HTML part.
func TestRenderInvite_escapesUserControlledFleetName(t *testing.T) {
	d := testInviteData()
	d.FleetName = `<script>alert(1)</script>`
	msg, err := RenderInvite(d)
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if strings.Contains(msg.HTML, "<script>") {
		t.Fatalf("unescaped script tag survived into the HTML part: %s", msg.HTML)
	}
	if !strings.Contains(msg.HTML, "&lt;script&gt;") {
		t.Fatalf("expected an escaped fleet name, got: %s", msg.HTML)
	}
}

// The accept URL is the one value that must NOT be escaped into uselessness.
// html/template treats href as a URL context and passes an https:// URL through
// intact — but would rewrite a javascript: scheme, which is the protection we
// want to keep.
func TestRenderInvite_keepsTheAcceptURLUsableInHref(t *testing.T) {
	msg, err := RenderInvite(testInviteData())
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if !strings.Contains(msg.HTML, `href="https://myfleet.example.com/invites/deadbeef/accept"`) {
		t.Fatalf("href was mangled: %s", msg.HTML)
	}
}

// An empty fleet name degrades to a generic subject rather than rendering
// "invited to  on MyFleet" (FR-TPL-3).
func TestRenderInvite_emptyFleetNameDegradesGracefully(t *testing.T) {
	d := testInviteData()
	d.FleetName = ""
	msg, err := RenderInvite(d)
	if err != nil {
		t.Fatalf("RenderInvite: %v", err)
	}
	if msg.Subject != "You're invited to a fleet on MyFleet" {
		t.Fatalf("Subject=%q", msg.Subject)
	}
	if strings.Contains(msg.Text, "  ") {
		t.Fatalf("empty fleet name left a hole in the text part: %s", msg.Text)
	}
}
