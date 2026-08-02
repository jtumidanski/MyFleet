package mailer

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
	"time"

	"github.com/google/uuid"
)

// compose renders msg as an RFC 5322 multipart/alternative message.
//
// Hand-composed rather than pulled from a third-party mail library: the header
// set is small and fixed, and mime/multipart plus net/textproto cover the rest.
//
// date, messageID and boundary are parameters rather than generated inside, so
// the output is deterministic under test.
func compose(fromName, fromAddress string, msg Message, date time.Time, messageID, boundary string) ([]byte, error) {
	var buf bytes.Buffer

	from := fromAddress
	if n := sanitizeHeader(fromName); n != "" {
		from = fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", n), fromAddress)
	}

	headers := [][2]string{
		{"From", from},
		{"To", sanitizeHeader(msg.To)},
		// The fleet name reaches the Subject and is user-controlled, so it is
		// RFC 2047 encoded. QEncoding also neutralises any control character
		// that reached the database before invite-creation validation existed.
		{"Subject", mime.QEncoding.Encode("utf-8", sanitizeHeader(msg.Subject))},
		{"Date", date.Format(time.RFC1123Z)},
		{"Message-ID", sanitizeHeader(messageID)},
		{"MIME-Version", "1.0"},
		{"Content-Type", fmt.Sprintf("multipart/alternative; boundary=%q", boundary)},
	}
	for _, h := range headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", h[0], h[1])
	}
	buf.WriteString("\r\n")

	w := multipart.NewWriter(&buf)
	if err := w.SetBoundary(boundary); err != nil {
		return nil, err
	}

	// Least-capable part first, per RFC 2046: a text-only client renders the
	// plain part rather than raw markup.
	parts := []struct{ contentType, body string }{
		{`text/plain; charset="utf-8"`, msg.Text},
		{`text/html; charset="utf-8"`, msg.HTML},
	}
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", p.contentType)
		h.Set("Content-Transfer-Encoding", "8bit")
		pw, err := w.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := pw.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sanitizeHeader strips CR and LF from a header value. Invite-creation already
// rejects them (ValidateInviteEmail), and QEncoding would encode them anyway;
// this is the belt to that pair of braces, and it covers rows written before
// the validation existed.
func sanitizeHeader(s string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// newBoundary returns a random MIME boundary. It must not collide with body
// content — a collision truncates the message at the collision point.
func newBoundary() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "myfleet-" + hex.EncodeToString(b), nil
}

// newMessageID builds a Message-ID whose domain half matches the From domain.
// A Message-ID whose domain does not match From is a spam signal.
func newMessageID(fromAddress string) (string, error) {
	at := strings.LastIndex(fromAddress, "@")
	if at < 0 || at == len(fromAddress)-1 {
		return "", fmt.Errorf("from address %q has no domain", fromAddress)
	}
	return fmt.Sprintf("<%s@%s>", uuid.NewString(), fromAddress[at+1:]), nil
}
