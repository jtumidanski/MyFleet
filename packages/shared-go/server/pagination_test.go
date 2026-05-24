package server

import (
	"net/http/httptest"
	"testing"
)

func TestParsePage_defaultsAndClamps(t *testing.T) {
	p := ParsePage(httptest.NewRequest("GET", "/x", nil))
	if p.Number != 1 || p.Size != 25 {
		t.Fatalf("defaults want 1/25 got %d/%d", p.Number, p.Size)
	}
	p = ParsePage(httptest.NewRequest("GET", "/x?page[number]=3&page[size]=500", nil))
	if p.Number != 3 || p.Size != 100 {
		t.Fatalf("clamp want 3/100 got %d/%d", p.Number, p.Size)
	}
}

func TestPageMeta_computesTotalPages(t *testing.T) {
	m := Page{Number: 1, Size: 10}.Meta(95)
	if m.TotalPages != 10 || m.Total != 95 {
		t.Fatalf("want 10 pages/95 total got %d/%d", m.TotalPages, m.Total)
	}
}
