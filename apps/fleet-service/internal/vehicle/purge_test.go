package vehicle

import (
	"testing"
	"time"
)

func TestPurgeAfter_isFiveDaysAfterDelete(t *testing.T) {
	deletedAt := time.Date(2026, 5, 24, 0, 0, 0, 0, time.UTC)
	if got := ComputePurgeAfter(deletedAt); !got.Equal(deletedAt.Add(5 * 24 * time.Hour)) {
		t.Fatalf("purge_after want +5d, got %v", got)
	}
}

func TestIsPurgeable_onlyPastWindow(t *testing.T) {
	past := time.Now().Add(-time.Minute)
	future := time.Now().Add(time.Hour)
	if !IsPurgeable(&past) {
		t.Fatal("row past purge_after should be purgeable")
	}
	if IsPurgeable(&future) {
		t.Fatal("row before purge_after must not be purged")
	}
}
