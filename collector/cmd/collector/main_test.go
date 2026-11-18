package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tingshuai/FreeV2ray/collector/internal/collector"
)

type memoryFeedbackRecorder struct {
	reports []collector.FeedbackReport
}

func (recorder *memoryFeedbackRecorder) RecordFeedbackBatch(_ context.Context, reports []collector.FeedbackReport, _ time.Time) error {
	recorder.reports = append(recorder.reports, reports...)
	return nil
}

func TestServeMuxHealthAndSubscriptionAvailability(t *testing.T) {
	workDir := t.TempDir()
	mux := newServeMux(workDir, time.Hour)
	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	mux.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing health status = %d, want %d", healthResponse.Code, http.StatusServiceUnavailable)
	}
	missingRequest := httptest.NewRequest(http.MethodGet, "/subscription", nil)
	missingResponse := httptest.NewRecorder()
	mux.ServeHTTP(missingResponse, missingRequest)
	if missingResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("missing subscription status = %d, want %d", missingResponse.Code, http.StatusServiceUnavailable)
	}
	published := filepath.Join(workDir, "published")
	if err := os.MkdirAll(published, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(published, "subscription.txt"), []byte("c3Vic2NyaXB0aW9u"), 0o600); err != nil {
		t.Fatal(err)
	}
	readyRequest := httptest.NewRequest(http.MethodGet, "/subscription", nil)
	readyResponse := httptest.NewRecorder()
	mux.ServeHTTP(readyResponse, readyRequest)
	if readyResponse.Code != http.StatusOK {
		t.Fatalf("published subscription status = %d, want %d", readyResponse.Code, http.StatusOK)
	}
	if got := readyResponse.Body.String(); got != "c3Vic2NyaXB0aW9u" {
		t.Fatalf("published subscription = %q", got)
	}
	healthResponse = httptest.NewRecorder()
	mux.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusNoContent {
		t.Fatalf("fresh health status = %d, want %d", healthResponse.Code, http.StatusNoContent)
	}
	stale := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(published, "subscription.txt"), stale, stale); err != nil {
		t.Fatal(err)
	}
	healthResponse = httptest.NewRecorder()
	mux.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("stale health status = %d, want %d", healthResponse.Code, http.StatusServiceUnavailable)
	}
}

func TestServeMuxUsesSelectedWeightedSubscriptionResponse(t *testing.T) {
	workDir := t.TempDir()
	published := filepath.Join(workDir, "published")
	if err := os.MkdirAll(published, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(published, "subscription.txt"), []byte("cHVibGlzaGVk"), 0o600); err != nil {
		t.Fatal(err)
	}
	selectorCalled := false
	mux := newServeMux(workDir, time.Hour, func(_ context.Context, _ []byte) ([]byte, error) {
		selectorCalled = true
		return []byte("c2VsZWN0ZWQ="), nil
	})
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/subscription", nil))
	if response.Code != http.StatusOK || response.Body.String() != "c2VsZWN0ZWQ=" || !selectorCalled {
		t.Fatalf("response status=%d body=%q selectorCalled=%v", response.Code, response.Body.String(), selectorCalled)
	}
}

func TestServeMuxExposesManifestWithoutCaching(t *testing.T) {
	workDir := t.TempDir()
	published := filepath.Join(workDir, "published")
	if err := os.MkdirAll(published, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(published, "subscription.txt"), []byte("cHVibGlzaGVk"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifestCalled := false
	mux := newServeMuxWithManifest(workDir, time.Hour,
		func(_ context.Context, _ []byte) ([]byte, error) { return []byte("c2VsZWN0ZWQ="), nil },
		func(_ context.Context, _ []byte) ([]byte, error) {
			manifestCalled = true
			return []byte(`{"version":2}`), nil
		}, nil,
	)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/manifest", nil))
	if response.Code != http.StatusOK || !manifestCalled || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("manifest response status=%d headers=%v called=%v", response.Code, response.Header(), manifestCalled)
	}
	if response.Body.String() != `{"version":2}` {
		t.Fatalf("manifest body = %q", response.Body.String())
	}
}

func TestFeedbackEndpointValidatesAndPassesOnlyCoarseAggregateFields(t *testing.T) {
	workDir := t.TempDir()
	recorder := &memoryFeedbackRecorder{}
	mux := newServeMuxInternal(workDir, time.Hour, nil, nil, recorder)
	candidateID := strings.Repeat("a", 64)
	body, err := json.Marshal([]collector.FeedbackReport{{CandidateID: candidateID, Outcome: "success", LatencyBucket: "100_300"}})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/feedback", bytes.NewReader(body))
	request.RemoteAddr = "203.0.113.10:1234"
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || len(recorder.reports) != 1 || recorder.reports[0].CandidateID != candidateID || recorder.reports[0].Outcome != "success" || recorder.reports[0].LatencyBucket != "100_300" {
		t.Fatalf("feedback status=%d recorder=%+v", response.Code, recorder)
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("feedback cache-control = %q", got)
	}
	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/feedback", strings.NewReader(`{"candidate_id":"short","outcome":"failure","latency_bucket":"none"}`)))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid feedback status = %d", bad.Code)
	}
}

func TestWindowRateLimiterBoundsFeedbackRequests(t *testing.T) {
	limiter := &windowRateLimiter{limit: 2, window: time.Minute}
	now := time.Now()
	if !limiter.allow(now) || !limiter.allow(now.Add(time.Second)) || limiter.allow(now.Add(2*time.Second)) {
		t.Fatal("feedback limiter did not enforce request window")
	}
	if !limiter.allow(now.Add(time.Minute + time.Second)) {
		t.Fatal("feedback limiter did not reset after its time window")
	}
}
