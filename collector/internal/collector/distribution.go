package collector

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"math"
	mathrand "math/rand"
	"strings"
	"time"
)

const DefaultDistributionLimit = 32

type CandidateQuality struct {
	SuccessCount     int
	FailureCount     int
	ConsecutiveFails int
	AssignmentCount  int
	LastSuccessAt    *time.Time
}

func candidateDistributionWeight(quality CandidateQuality, now time.Time) float64 {
	attempts := quality.SuccessCount + quality.FailureCount
	if attempts == 0 {
		return 0.20 // Small exploration weight for candidates without probe history.
	}
	successRate := float64(quality.SuccessCount+1) / float64(attempts+2)
	stability := 1 / float64(1+quality.ConsecutiveFails)
	freshness := 0.25
	if quality.LastSuccessAt != nil {
		age := now.Sub(*quality.LastSuccessAt)
		if age < 0 {
			age = 0
		}
		freshness = 0.25 + 0.75*math.Exp(-age.Hours()/24)
	}
	pressure := 1 / math.Sqrt(float64(quality.AssignmentCount+1))
	return (0.25 + 0.75*successRate) * stability * freshness * pressure
}

func selectWeightedCandidates(candidates []string, qualities map[string]CandidateQuality, key []byte, limit int, now time.Time, random *mathrand.Rand) ([]string, []string) {
	if limit <= 0 || len(candidates) == 0 {
		return nil, nil
	}
	type weightedCandidate struct {
		config      string
		fingerprint string
		weight      float64
	}
	weighted := make([]weightedCandidate, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		fingerprint := fingerprint(key, candidate)
		if _, exists := seen[fingerprint]; exists {
			continue
		}
		seen[fingerprint] = struct{}{}
		weighted = append(weighted, weightedCandidate{config: candidate, fingerprint: fingerprint, weight: candidateDistributionWeight(qualities[fingerprint], now)})
	}
	selected := make([]string, 0, min(limit, len(weighted)))
	selectedFingerprints := make([]string, 0, min(limit, len(weighted)))
	for len(weighted) > 0 && len(selected) < limit {
		total := 0.0
		for _, candidate := range weighted {
			total += candidate.weight
		}
		if total <= 0 {
			return selected, selectedFingerprints
		}
		target := random.Float64() * total
		chosen := len(weighted) - 1
		for index, candidate := range weighted {
			target -= candidate.weight
			if target < 0 {
				chosen = index
				break
			}
		}
		selected = append(selected, weighted[chosen].config)
		selectedFingerprints = append(selectedFingerprints, weighted[chosen].fingerprint)
		weighted = append(weighted[:chosen], weighted[chosen+1:]...)
	}
	return selected, selectedFingerprints
}

// DistributeSubscription returns a bounded weighted sample of the last valid
// publication and records only aggregate candidate assignments.
func DistributeSubscription(ctx context.Context, store Store, key, published []byte, limit int) ([]byte, error) {
	if len(key) < 32 {
		return nil, errors.New("fingerprint key must contain at least 32 bytes")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(published)))
	if err != nil {
		return nil, errors.New("published subscription is invalid")
	}
	candidates := strings.Fields(string(decoded))
	fingerprints := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		fingerprints = append(fingerprints, fingerprint(key, candidate))
	}
	selected, selectedFingerprints := candidates, fingerprints
	if len(candidates) > limit {
		qualities, err := store.CandidateQuality(ctx, fingerprints)
		if err != nil {
			return nil, err
		}
		var seedBytes [8]byte
		if _, err := rand.Read(seedBytes[:]); err != nil {
			return nil, err
		}
		rng := mathrand.New(mathrand.NewSource(int64(binary.LittleEndian.Uint64(seedBytes[:]))))
		selected, selectedFingerprints = selectWeightedCandidates(candidates, qualities, key, limit, time.Now().UTC(), rng)
	}
	if len(selected) == 0 {
		return published, nil
	}
	if err := store.RecordCandidateAssignments(ctx, selectedFingerprints); err != nil {
		return nil, err
	}
	if len(selected) == len(candidates) {
		return published, nil
	}
	return []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(selected, "\n")))), nil
}
