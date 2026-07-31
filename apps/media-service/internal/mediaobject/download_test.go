package mediaobject

import (
	"strings"
	"testing"
)

func TestContentDisposition_imagesInlineDocumentsAttachment(t *testing.T) {
	if got := ContentDisposition(ClassImage, "photo.jpg", "m1"); got != `inline; filename="photo.jpg"` {
		t.Fatalf("image: %q", got)
	}
	if got := ContentDisposition(ClassDocument, "invoice.pdf", "m1"); got != `attachment; filename="invoice.pdf"` {
		t.Fatalf("document: %q", got)
	}
	// Legacy rows whose stored type nobody recognises are attachments too.
	if got := ContentDisposition(ClassUnknown, "unknown.bin", "m1"); got != `attachment; filename="unknown.bin"` {
		t.Fatalf("unknown: %q", got)
	}
}

// The acceptance criterion: a filename must never be able to inject a header.
func TestContentDisposition_stripsControlCharacters(t *testing.T) {
	got := ContentDisposition(ClassDocument, "in\r\nX-Evil: 1\x00voice\x7f.pdf", "m1")
	if strings.ContainsAny(got, "\r\n\x00\x7f") {
		t.Fatalf("control characters survived: %q", got)
	}
	if got != `attachment; filename="inX-Evil: 1voice.pdf"` {
		t.Fatalf("got %q", got)
	}
}

func TestContentDisposition_escapesQuotesAndBackslashes(t *testing.T) {
	got := ContentDisposition(ClassDocument, `in"voice.pdf`, "m1")
	if got != `attachment; filename="in\"voice.pdf"` {
		t.Fatalf("got %q", got)
	}
}

// A filename must not suggest a path to the client.
func TestContentDisposition_takesBaseNameOnly(t *testing.T) {
	for _, in := range []string{"../../etc/passwd", `C:\Users\bob\invoice.pdf`, "/tmp/invoice.pdf"} {
		got := ContentDisposition(ClassDocument, in, "m1")
		if strings.Contains(got, "/") || strings.Contains(got, `\\`) {
			t.Fatalf("path survived for %q: %q", in, got)
		}
	}
	if got := ContentDisposition(ClassDocument, "/tmp/invoice.pdf", "m1"); got != `attachment; filename="invoice.pdf"` {
		t.Fatalf("got %q", got)
	}
}

func TestContentDisposition_nonASCIIGetsRFC5987Form(t *testing.T) {
	got := ContentDisposition(ClassDocument, "facturé señal.pdf", "m1")
	want := `attachment; filename="factur_ se_al.pdf"; filename*=UTF-8''factur%C3%A9%20se%C3%B1al.pdf`
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// url.QueryEscape encodes space as '+' and url.PathEscape leaves '?', '&' and
// '=' alone; both produce headers that mostly work, which is the worst outcome
// for a security-adjacent function (design D14).
func TestContentDisposition_rfc5987EncodesReservedCharacters(t *testing.T) {
	got := ContentDisposition(ClassDocument, "a?b&c=d é.pdf", "m1")
	if !strings.Contains(got, "filename*=UTF-8''a%3Fb%26c%3Dd%20%C3%A9.pdf") {
		t.Fatalf("reserved characters not encoded: %q", got)
	}
	if strings.Contains(got, "+") {
		t.Fatalf("space encoded as '+': %q", got)
	}
}

func TestContentDisposition_emptyNameFallsBackToMediaID(t *testing.T) {
	if got := ContentDisposition(ClassDocument, "", "m-123"); got != `attachment; filename="m-123"` {
		t.Fatalf("empty: %q", got)
	}
	// Fully stripped by sanitisation is the same case.
	if got := ContentDisposition(ClassDocument, "\r\n\x00", "m-123"); got != `attachment; filename="m-123"` {
		t.Fatalf("stripped: %q", got)
	}
	// And if even the fallback is unusable, the header is still well-formed.
	if got := ContentDisposition(ClassDocument, "", ""); got != `attachment; filename="download"` {
		t.Fatalf("no fallback: %q", got)
	}
}
