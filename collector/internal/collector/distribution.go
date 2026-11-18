package collector

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	mathrand "math/rand"
	"sort"
	"strings"
	"time"
)

const (
	DefaultDistributionLimit      = 32
	MaxCandidateAssignmentsPerDay = 500
)

type CandidateQuality struct {
	SuccessCount         int
	FailureCount         int
	ConsecutiveFails     int
	AssignmentCount      int
	LastSuccessAt        *time.Time
	FeedbackSuccess      int
	FeedbackFailure      int
	DailyAssignmentCount int
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
	feedbackSuccess := min(quality.FeedbackSuccess, 5)
	feedbackFailure := min(quality.FeedbackFailure, 5)
	feedbackMultiplier := 1.0
	if feedbackSuccess+feedbackFailure > 0 {
		feedbackRate := float64(feedbackSuccess+1) / float64(feedbackSuccess+feedbackFailure+2)
		feedbackMultiplier = 0.9 + 0.2*feedbackRate
	}
	return (0.25 + 0.75*successRate) * stability * freshness * pressure * feedbackMultiplier
}

func selectWeightedCandidates(candidates []string, qualities map[string]CandidateQuality, key []byte, limit int, now time.Time, random *mathrand.Rand) ([]string, []string) {
	return selectWeightedCandidatesWithSources(candidates, qualities, nil, key, limit, now, random)
}

func selectWeightedCandidatesWithSources(candidates []string, qualities map[string]CandidateQuality, sourceSets map[string][]string, key []byte, limit int, now time.Time, random *mathrand.Rand) ([]string, []string) {
	if limit <= 0 || len(candidates) == 0 {
		return nil, nil
	}
	type weightedCandidate struct {
		config      string
		fingerprint string
		sources     []string
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
		weighted = append(weighted, weightedCandidate{config: candidate, fingerprint: fingerprint, sources: sourceSets[fingerprint], weight: candidateDistributionWeight(qualities[fingerprint], now)})
	}
	selected := make([]string, 0, min(limit, len(weighted)))
	selectedFingerprints := make([]string, 0, min(limit, len(weighted)))
	sourceCounts := make(map[string]int)
	availableSources := make(map[string]struct{})
	for _, candidate := range weighted {
		for _, sourceID := range candidate.sources {
			availableSources[sourceID] = struct{}{}
		}
	}
	diversityTarget := min(3, min(limit, len(availableSources)))
	for len(weighted) > 0 && len(selected) < limit {
		representedSources := make(map[string]struct{}, len(sourceCounts))
		for sourceID, count := range sourceCounts {
			if count > 0 {
				representedSources[sourceID] = struct{}{}
			}
		}
		needNewSource := len(representedSources) < diversityTarget
		total := 0.0
		for _, candidate := range weighted {
			newSource := false
			for _, sourceID := range candidate.sources {
				if _, represented := representedSources[sourceID]; !represented {
					newSource = true
					break
				}
			}
			if needNewSource && !newSource {
				continue
			}
			pressure := 1
			for _, sourceID := range candidate.sources {
				if sourceCounts[sourceID] > pressure {
					pressure = sourceCounts[sourceID]
				}
			}
			total += candidate.weight / float64(pressure)
		}
		if total <= 0 && needNewSource {
			needNewSource = false
			for _, candidate := range weighted {
				pressure := 1
				for _, sourceID := range candidate.sources {
					if sourceCounts[sourceID] > pressure {
						pressure = sourceCounts[sourceID]
					}
				}
				total += candidate.weight / float64(pressure)
			}
		}
		if total <= 0 {
			return selected, selectedFingerprints
		}
		target := random.Float64() * total
		chosen := len(weighted) - 1
		for index, candidate := range weighted {
			newSource := false
			for _, sourceID := range candidate.sources {
				if _, represented := representedSources[sourceID]; !represented {
					newSource = true
					break
				}
			}
			if needNewSource && !newSource {
				continue
			}
			pressure := 1
			for _, sourceID := range candidate.sources {
				if sourceCounts[sourceID] > pressure {
					pressure = sourceCounts[sourceID]
				}
			}
			target -= candidate.weight / float64(pressure)
			if target < 0 {
				chosen = index
				break
			}
		}
		selected = append(selected, weighted[chosen].config)
		selectedFingerprints = append(selectedFingerprints, weighted[chosen].fingerprint)
		for _, sourceID := range weighted[chosen].sources {
			sourceCounts[sourceID]++
		}
		weighted = append(weighted[:chosen], weighted[chosen+1:]...)
	}
	return selected, selectedFingerprints
}

// DistributeSubscription returns a bounded weighted sample of the last valid
// publication and records only aggregate candidate assignments.
func DistributeSubscription(ctx context.Context, store Store, key, published []byte, limit int) ([]byte, error) {
	selected, _, _, err := selectPublishedCandidates(ctx, store, key, published, limit)
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return []byte{}, nil
	}
	return []byte(base64.StdEncoding.EncodeToString([]byte(strings.Join(selected, "\n")))), nil
}

type FeedCandidate struct {
	ID     string `json:"id"`
	Config string `json:"config"`
}

type CandidateManifest struct {
	Version     int             `json:"version"`
	GeneratedAt time.Time       `json:"generated_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	ContentHash string          `json:"content_sha256"`
	Candidates  []FeedCandidate `json:"candidates"`
	Revoked     []string        `json:"revoked"`
}

func BuildCandidateManifest(ctx context.Context, store Store, key, published []byte, limit int, publishedAt, now time.Time) ([]byte, error) {
	expiresAt := publishedAt.UTC().Add(2 * time.Hour)
	if publishedAt.IsZero() || !expiresAt.After(now.UTC()) || publishedAt.After(now.UTC().Add(5*time.Minute)) {
		return nil, errors.New("published feed is stale or has an invalid timestamp")
	}
	selected, selectedFingerprints, revoked, err := selectPublishedCandidates(ctx, store, key, published, limit)
	if err != nil {
		return nil, err
	}
	manifest := CandidateManifest{Version: 2, GeneratedAt: publishedAt.UTC(), ExpiresAt: expiresAt, Candidates: make([]FeedCandidate, 0, len(selected)), Revoked: make([]string, 0)}
	for index, config := range selected {
		manifest.Candidates = append(manifest.Candidates, FeedCandidate{ID: selectedFingerprints[index], Config: config})
	}
	for id := range revoked {
		manifest.Revoked = append(manifest.Revoked, id)
	}
	sort.Strings(manifest.Revoked)
	content := make([]string, 0, len(manifest.Candidates)+len(manifest.Revoked)+1)
	for _, candidate := range manifest.Candidates {
		content = append(content, candidate.ID+"\t"+candidate.Config)
	}
	content = append(content, "--revoked--")
	content = append(content, manifest.Revoked...)
	digest := sha256.Sum256([]byte(strings.Join(content, "\n")))
	manifest.ContentHash = hex.EncodeToString(digest[:])
	return json.Marshal(manifest)
}

func selectPublishedCandidates(ctx context.Context, store Store, key, published []byte, limit int) ([]string, []string, map[string]string, error) {
	if len(key) < 32 {
		return nil, nil, nil, errors.New("fingerprint key must contain at least 32 bytes")
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(published)))
	if err != nil {
		return nil, nil, nil, errors.New("published subscription is invalid")
	}
	allCandidates := strings.Fields(string(decoded))
	revoked, err := store.CandidateRevocations(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	candidates := make([]string, 0, len(allCandidates))
	fingerprints := make([]string, 0, len(allCandidates))
	for _, candidate := range allCandidates {
		id := fingerprint(key, candidate)
		if _, isRevoked := revoked[id]; isRevoked {
			continue
		}
		candidates = append(candidates, candidate)
		fingerprints = append(fingerprints, id)
	}
	qualities, err := store.CandidateQuality(ctx, fingerprints)
	if err != nil {
		return nil, nil, nil, err
	}
	sources, err := store.CandidateSources(ctx, fingerprints)
	if err != nil {
		return nil, nil, nil, err
	}
	eligibleCandidates := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if qualities[fingerprint(key, candidate)].DailyAssignmentCount < MaxCandidateAssignmentsPerDay {
			eligibleCandidates = append(eligibleCandidates, candidate)
		}
	}
	var seedBytes [8]byte
	if _, err := rand.Read(seedBytes[:]); err != nil {
		return nil, nil, nil, err
	}
	rng := mathrand.New(mathrand.NewSource(int64(binary.LittleEndian.Uint64(seedBytes[:]))))
	selected, selectedFingerprints := selectWeightedCandidatesWithSources(eligibleCandidates, qualities, sources, key, limit, time.Now().UTC(), rng)
	if len(selected) > 0 {
		if err := store.RecordCandidateAssignments(ctx, selectedFingerprints); err != nil {
			return nil, nil, nil, err
		}
	}
	return selected, selectedFingerprints, revoked, nil
}
