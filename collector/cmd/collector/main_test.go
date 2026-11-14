package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
