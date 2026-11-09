package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServeMuxHealthAndSubscriptionAvailability(t *testing.T) {
	workDir := t.TempDir()
	mux := newServeMux(workDir)
	healthRequest := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthResponse := httptest.NewRecorder()
	mux.ServeHTTP(healthResponse, healthRequest)
	if healthResponse.Code != http.StatusNoContent {
		t.Fatalf("health status = %d, want %d", healthResponse.Code, http.StatusNoContent)
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
}
