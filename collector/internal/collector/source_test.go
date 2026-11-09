package collector

import (
	"testing"
	"time"
)

func TestSourceIsDueUsesMostRecentAttempt(t *testing.T) {
	now := time.Date(2026, time.November, 9, 0, 0, 0, 0, time.UTC)
	lastSuccess := now.Add(-2 * time.Minute)
	lastFailure := now.Add(-30 * time.Second)
	source := Source{
		MinFetchInterval: time.Minute,
		LastSuccessAt:    &lastSuccess,
		LastFailureAt:    &lastFailure,
	}
	if source.IsDue(now) {
		t.Fatal("source was due before its failed-attempt interval elapsed")
	}
	if !source.IsDue(lastFailure.Add(time.Minute)) {
		t.Fatal("source was not due at its fetch-interval boundary")
	}
}
