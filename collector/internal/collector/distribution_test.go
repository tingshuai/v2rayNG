package collector

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math/rand"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCandidateDistributionWeightRewardsRecentSuccessAndLimitsPressure(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-time.Hour)
	good := candidateDistributionWeight(CandidateQuality{SuccessCount: 9, FailureCount: 1, LastSuccessAt: &recent}, now)
	newCandidate := candidateDistributionWeight(CandidateQuality{}, now)
	contended := candidateDistributionWeight(CandidateQuality{SuccessCount: 9, FailureCount: 1, AssignmentCount: 99, LastSuccessAt: &recent}, now)
	if !(good > newCandidate && newCandidate > contended) {
		t.Fatalf("weights good=%f new=%f contended=%f", good, newCandidate, contended)
	}
	feedbackFailure := candidateDistributionWeight(CandidateQuality{SuccessCount: 9, FailureCount: 1, LastSuccessAt: &recent, FeedbackFailure: 1}, now)
	if feedbackFailure >= good || feedbackFailure < good*0.95 {
		t.Fatalf("single feedback should only weakly change weight: baseline=%f feedback=%f", good, feedbackFailure)
	}
}

func TestFingerprintCanonicalizesNonConnectionURLDifferences(t *testing.T) {
	key := make([]byte, 32)
	first := "VLESS://uuid@EXAMPLE.com:443?type=ws&security=tls#one"
	second := "vless://uuid@example.com:443?security=tls&type=ws#two"
	if fingerprint(key, first) != fingerprint(key, second) {
		t.Fatal("connection-equivalent candidates received different fingerprints")
	}
}

func TestWeightedCandidateSelectionIsBoundedUniqueAndReproducible(t *testing.T) {
	key := make([]byte, 32)
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	candidates := make([]string, 200)
	for index := range candidates {
		candidates[index] = "candidate-" + strconv.Itoa(index)
	}
	first, firstFingerprints := selectWeightedCandidates(candidates, nil, key, DefaultDistributionLimit, now, rand.New(rand.NewSource(7)))
	second, secondFingerprints := selectWeightedCandidates(candidates, nil, key, DefaultDistributionLimit, now, rand.New(rand.NewSource(7)))
	if len(first) != DefaultDistributionLimit || !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstFingerprints, secondFingerprints) {
		t.Fatalf("selections are not bounded and reproducible: %d %d", len(first), len(second))
	}
	seen := make(map[string]struct{}, len(first))
	for _, candidate := range first {
		if _, exists := seen[candidate]; exists {
			t.Fatal("weighted selection returned duplicates")
		}
		seen[candidate] = struct{}{}
	}
}

func TestWeightedSelectionReducesConcentrationFromOneSource(t *testing.T) {
	key := make([]byte, 32)
	candidates := make([]string, 21)
	sources := make(map[string][]string, len(candidates))
	for index := range candidates {
		candidates[index] = "candidate-" + strconv.Itoa(index)
		sourceID := "source-a"
		if index == len(candidates)-1 {
			sourceID = "source-b"
		}
		sources[fingerprint(key, candidates[index])] = []string{sourceID}
	}
	selected, _ := selectWeightedCandidatesWithSources(candidates, nil, sources, key, 12, time.Now().UTC(), rand.New(rand.NewSource(7)))
	for _, candidate := range selected {
		if candidate == candidates[len(candidates)-1] {
			return
		}
	}
	t.Fatal("selection omitted the only candidate from a distinct source")
}

func TestWeightedSelectionGuaranteesThreeSourcesWhenAvailable(t *testing.T) {
	key := make([]byte, 32)
	now := time.Now().UTC()
	candidates := []string{"source-a-1", "source-a-2", "source-a-3", "source-b", "source-c", "source-d"}
	sources := map[string][]string{}
	for index, candidate := range candidates {
		sourceID := "source-a"
		if index == 3 {
			sourceID = "source-b"
		} else if index == 4 {
			sourceID = "source-c"
		} else if index == 5 {
			sourceID = "source-d"
		}
		sources[fingerprint(key, candidate)] = []string{sourceID}
	}
	selected, _ := selectWeightedCandidatesWithSources(candidates, nil, sources, key, 3, now, rand.New(rand.NewSource(1)))
	selectedSources := map[string]struct{}{}
	for _, candidate := range selected {
		selectedSources[sources[fingerprint(key, candidate)][0]] = struct{}{}
	}
	if len(selectedSources) != 3 {
		t.Fatalf("selected source count = %d, want 3: %v", len(selectedSources), selectedSources)
	}
}

func TestDistributeSubscriptionReturnsBoundedSampleAndRecordsOnlyAggregateAssignments(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	candidates := make([]string, 40)
	for index := range candidates {
		candidates[index] = "trojan://credential@node-" + strconv.Itoa(index) + ".example:443"
	}
	encoded := []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(candidates, "\n"))))
	output, err := DistributeSubscription(context.Background(), store, key, encoded, 12)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	if err != nil {
		t.Fatal(err)
	}
	selected := strings.Fields(string(decoded))
	if len(selected) != 12 {
		t.Fatalf("selected candidate count = %d, want 12", len(selected))
	}
	fingerprints := make([]string, 0, len(selected))
	for _, candidate := range selected {
		fingerprints = append(fingerprints, fingerprint(key, candidate))
	}
	qualities, err := store.CandidateQuality(context.Background(), fingerprints)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidateFingerprint := range fingerprints {
		if qualities[candidateFingerprint].AssignmentCount != 1 {
			t.Fatalf("assignment count for selected candidate is %d, want 1", qualities[candidateFingerprint].AssignmentCount)
		}
	}
}

func TestDistributionEnforcesDailyCandidateCapacityEvenBelowFeedLimit(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	capped := "trojan://secret-a@a.example:443"
	available := "trojan://secret-b@b.example:443"
	now := time.Now().UTC()
	for _, candidate := range []string{capped, available} {
		if err := store.RecordCandidateProbe(context.Background(), fingerprint(key, candidate), true, now); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec("INSERT INTO candidate_daily_assignments(fingerprint, day_utc, assignment_count) VALUES(?, ?, ?)", fingerprint(key, capped), now.Format("2006-01-02"), MaxCandidateAssignmentsPerDay); err != nil {
		t.Fatal(err)
	}
	published := []byte(base64.StdEncoding.EncodeToString([]byte(capped + "\n" + available)))
	output, err := DistributeSubscription(context.Background(), store, key, published, DefaultDistributionLimit)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.StdEncoding.DecodeString(string(output))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(decoded)); got != available {
		t.Fatalf("distributed candidates = %q, want only uncapped candidate", got)
	}
}

func TestCandidateManifestCarriesBoundedCandidatesDigestAndRevocation(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	candidates := []string{"trojan://secret-a@a.example:443", "vless://secret-b@b.example:443"}
	if err := store.RevokeCandidate(context.Background(), fingerprint(key, candidates[1]), "unsafe_address"); err != nil {
		t.Fatal(err)
	}
	published := []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(candidates, "\n"))))
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	raw, err := BuildCandidateManifest(context.Background(), store, key, published, 32, now, now)
	if err != nil {
		t.Fatal(err)
	}
	var manifest CandidateManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != 2 || len(manifest.Candidates) != 1 || len(manifest.Revoked) != 1 || manifest.Candidates[0].Config != candidates[0] {
		t.Fatalf("unexpected manifest fields: version=%d candidates=%d revoked=%d", manifest.Version, len(manifest.Candidates), len(manifest.Revoked))
	}
	if manifest.GeneratedAt != now || !manifest.ExpiresAt.Equal(now.Add(2*time.Hour)) || len(manifest.ContentHash) != 64 {
		t.Fatalf("manifest metadata invalid: %+v", manifest)
	}
}

func TestCandidateManifestEncodesEmptyRevocationsAsArray(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	published := []byte(base64.StdEncoding.EncodeToString([]byte("ss://secret@example.com:443")))
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	raw, err := BuildCandidateManifest(context.Background(), store, key, published, 32, now, now)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["revoked"]) != "[]" {
		t.Fatalf("empty revoked field = %s, want []", fields["revoked"])
	}
}
