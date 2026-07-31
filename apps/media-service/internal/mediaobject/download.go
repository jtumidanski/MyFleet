package mediaobject

import (
	"fmt"
	"strings"
)

// ContentDisposition renders the Content-Disposition header value for a
// download. filename is untrusted input supplied by whoever uploaded the file.
//
// This is a pure function on purpose (design D14): "a filename containing
// quotes, newlines or non-ASCII characters cannot corrupt or inject the header"
// is only cheap to prove if the logic is not buried in an HTTP handler.
//
// Sanitisation order matters:
//  1. Strip every character below 0x20 plus 0x7F. This is what makes header
//     injection structurally impossible, and it runs first so nothing
//     downstream ever sees a CR or LF.
//  2. Take the base name only, so a filename cannot suggest a path.
//  3. Build the ASCII form: non-ASCII runes become '_', and '\' and '"' are
//     escaped for the quoted-string.
//  4. If nothing usable is left, fall back (the caller passes the media ID).
//  5. When the original held non-ASCII, additionally emit the RFC 5987
//     filename* form, keeping the ASCII filename= as the older-client fallback.
func ContentDisposition(class Class, filename, fallback string) string {
	disposition := "attachment"
	if class == ClassImage {
		disposition = "inline"
	}

	name := baseName(stripControl(filename))
	if name == "" {
		name = baseName(stripControl(fallback))
	}
	if name == "" {
		name = "download"
	}

	out := disposition + `; filename="` + asciiFold(name) + `"`
	if hasNonASCII(name) {
		out += "; filename*=UTF-8''" + encodeRFC5987(name)
	}
	return out
}

// stripControl removes every rune below 0x20 plus 0x7F (DEL).
func stripControl(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7F {
			return -1
		}
		return r
	}, s)
}

// baseName drops everything up to the last '/' or '\'.
func baseName(s string) string {
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		return s[i+1:]
	}
	return s
}

// asciiFold replaces every non-ASCII rune with '_' and escapes the two
// characters that are special inside a quoted-string.
func asciiFold(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r > 0x7E:
			b.WriteByte('_')
		case r == '\\' || r == '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 0x7E {
			return true
		}
	}
	return false
}

// encodeRFC5987 percent-encodes every byte outside RFC 5987's attr-char set.
// Hand-written because url.QueryEscape encodes space as '+' and url.PathEscape
// leaves '?', '&' and '=' unescaped.
func encodeRFC5987(s string) string {
	const attrChar = "!#$+-.^_`|~"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			strings.IndexByte(attrChar, c) >= 0:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
