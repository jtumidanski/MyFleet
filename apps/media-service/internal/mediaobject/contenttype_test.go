package mediaobject

import (
	"strings"
	"testing"
)

func testAllowlist(t *testing.T) Allowlist {
	t.Helper()
	a, err := ParseAllowlist(DefaultAllowedContentTypes)
	if err != nil {
		t.Fatalf("ParseAllowlist(default): %v", err)
	}
	return a
}

// The two renderable types are hard-coded, not configured: they are a statement
// about what processing/worker.go's image.Decode can actually handle.
func TestClassify_rendersOnlyJPEGAndPNGAsImages(t *testing.T) {
	a := testAllowlist(t)
	for _, ct := range []string{"image/jpeg", "image/png"} {
		if got := a.Classify(ct); got != ClassImage {
			t.Fatalf("Classify(%q)=%v want ClassImage", ct, got)
		}
	}
	for _, ct := range []string{
		"application/pdf",
		"text/csv",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	} {
		if got := a.Classify(ct); got != ClassDocument {
			t.Fatalf("Classify(%q)=%v want ClassDocument", ct, got)
		}
	}
}

func TestClassify_offListAndMalformedAreUnknown(t *testing.T) {
	a := testAllowlist(t)
	for _, ct := range []string{"text/html", "image/heic", "", "not a media type", "///"} {
		if got := a.Classify(ct); got != ClassUnknown {
			t.Fatalf("Classify(%q)=%v want ClassUnknown", ct, got)
		}
	}
}

// text/csv; charset=utf-8 is what browsers actually send. A bare map lookup
// would reject it (design D10).
func TestNormalize_dropsParametersAndLowercases(t *testing.T) {
	a := testAllowlist(t)
	cases := map[string]string{
		"text/csv; charset=utf-8": "text/csv",
		"APPLICATION/PDF":         "application/pdf",
		"image/jpeg":              "image/jpeg",
		" image/png ":             "image/png",
	}
	for in, want := range cases {
		got, ok := a.Normalize(in)
		if !ok || got != want {
			t.Fatalf("Normalize(%q)=(%q,%v) want (%q,true)", in, got, ok, want)
		}
	}
}

func TestNormalize_rejectsEmptyAndOffList(t *testing.T) {
	a := testAllowlist(t)
	for _, in := range []string{"", "   ", "text/html", "application/x-msdownload"} {
		if got, ok := a.Normalize(in); ok {
			t.Fatalf("Normalize(%q)=(%q,true) want ok=false", in, got)
		}
	}
}

// Re-resolving on read is what protects rows created before the allowlist
// existed (design D15).
func TestResolve_offListFallsBackToOctetStream(t *testing.T) {
	a := testAllowlist(t)

	ct, class := a.Resolve("application/pdf")
	if ct != "application/pdf" || class != ClassDocument {
		t.Fatalf("Resolve(pdf)=(%q,%v) want (application/pdf, ClassDocument)", ct, class)
	}

	ct, class = a.Resolve("text/html")
	if ct != "application/octet-stream" || class != ClassUnknown {
		t.Fatalf("Resolve(text/html)=(%q,%v) want (application/octet-stream, ClassUnknown)", ct, class)
	}

	ct, class = a.Resolve("")
	if ct != "application/octet-stream" || class != ClassUnknown {
		t.Fatalf("Resolve(\"\")=(%q,%v) want (application/octet-stream, ClassUnknown)", ct, class)
	}
}

func TestParseAllowlist_rejectsMalformedAndEmpty(t *testing.T) {
	if _, err := ParseAllowlist("image/jpeg,not a media type"); err == nil {
		t.Fatal("ParseAllowlist accepted a malformed entry; want error")
	}
	if _, err := ParseAllowlist("   ,  "); err == nil {
		t.Fatal("ParseAllowlist accepted an empty list; want error")
	}
}

func TestParseAllowlist_toleratesWhitespaceAndTrailingCommas(t *testing.T) {
	a, err := ParseAllowlist(" image/jpeg , application/pdf ,")
	if err != nil {
		t.Fatalf("ParseAllowlist: %v", err)
	}
	if a.Classify("image/jpeg") != ClassImage || a.Classify("application/pdf") != ClassDocument {
		t.Fatal("whitespace-padded entries were not parsed")
	}
}

// Accepted() feeds the 415 message, so it must be stable and complete.
func TestAccepted_isSortedAndComplete(t *testing.T) {
	a := testAllowlist(t)
	got := strings.Join(a.Accepted(), ",")
	want := "application/pdf," +
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet," +
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document," +
		"image/jpeg,image/png,text/csv"
	if got != want {
		t.Fatalf("Accepted()=%q want %q", got, want)
	}
}
