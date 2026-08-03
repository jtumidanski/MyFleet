package mailer

import (
	"bytes"
	_ "embed"
	htmltemplate "html/template"
	"strings"
	texttemplate "text/template"
	"time"
)

//go:embed templates/invite.html.tmpl
var inviteHTMLSource string

//go:embed templates/invite.txt.tmpl
var inviteTextSource string

// Parsed once at package init. template.Must is correct here: an unparseable
// embedded template is a build-time mistake, not a runtime condition.
var (
	inviteHTML = htmltemplate.Must(htmltemplate.New("invite.html").Parse(inviteHTMLSource))
	inviteText = texttemplate.Must(texttemplate.New("invite.txt").Parse(inviteTextSource))
)

// InviteData is the render input. Every field is already resolved — mailer does
// no lookups. FleetName may be empty; the copy degrades rather than rendering a
// hole (FR-TPL-3).
//
// SECURITY: AcceptURL contains the token. This struct is never logged whole.
type InviteData struct {
	To        string
	FleetName string
	Role      string
	AcceptURL string
	ExpiresAt time.Time
}

// viewModel is what the templates actually see: pre-resolved copy fragments, so
// neither template contains a conditional and both stay trivially reviewable.
type viewModel struct {
	Greeting   string
	FleetLabel string
	Role       string
	AcceptURL  htmltemplate.URL
	Expires    string
}

// RenderInvite produces both MIME parts plus the subject.
//
// Escaping is contextual by construction (FR-TPL-5): html/template for the HTML
// part, text/template for the plain part. FleetName and Role are user-influenced
// input and are escaped by the template engine, not by hand.
//
// AcceptURL is typed htmltemplate.URL so html/template passes an https:// URL
// through an href intact instead of mangling it. That is safe because
// PUBLIC_WEB_URL comes from config and never from an inbound request header
// (FR-TPL-2), so the scheme is trusted by construction.
func RenderInvite(d InviteData) (Message, error) {
	label := strings.TrimSpace(d.FleetName)
	subject := "You're invited to " + label + " on MyFleet"
	if label == "" {
		label = "a fleet"
		subject = "You're invited to a fleet on MyFleet"
	}

	vm := viewModel{
		Greeting:   "You've been invited to MyFleet",
		FleetLabel: label,
		Role:       d.Role,
		AcceptURL:  htmltemplate.URL(d.AcceptURL),
		Expires:    d.ExpiresAt.UTC().Format("January 2, 2006"),
	}

	var html, text bytes.Buffer
	if err := inviteHTML.Execute(&html, vm); err != nil {
		return Message{}, err
	}
	if err := inviteText.Execute(&text, vm); err != nil {
		return Message{}, err
	}

	return Message{To: d.To, Subject: subject, HTML: html.String(), Text: text.String()}, nil
}
