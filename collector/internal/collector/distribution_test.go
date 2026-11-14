package collector

import (
	"context"
	"encoding/base64"
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
