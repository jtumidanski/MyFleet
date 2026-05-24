package database

import (
	"errors"
	"testing"
)

func TestQuery_lazilyInvokesFetcherOnce(t *testing.T) {
	calls := 0
	p := Query(func() (string, error) { calls++; return "value", nil })
	got, err := p()
	if err != nil || got != "value" {
		t.Fatalf("want value,nil got %q,%v", got, err)
	}
	if _, _ = p(); calls != 2 {
		t.Fatalf("provider should re-invoke fetcher each call, calls=%d", calls)
	}
}

func TestSliceQuery_propagatesError(t *testing.T) {
	sentinel := errors.New("boom")
	p := SliceQuery(func() ([]int, error) { return nil, sentinel })
	if _, err := p(); !errors.Is(err, sentinel) {
		t.Fatalf("want sentinel error, got %v", err)
	}
}
