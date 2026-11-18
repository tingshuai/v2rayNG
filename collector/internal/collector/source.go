package collector

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	ContractVersionV1 = 1
	FailurePauseLimit = 3
)

var ErrSourceNotFound = errors.New("source not found")
var ErrCandidateNotRevoked = errors.New("candidate is not revoked")
var ErrNoScreenedCandidates = errors.New("no candidates passed bounded screening")

type Source struct {
	ID                  string
	FetchURL            string
	Enabled             bool
	ContractVersion     int
	MinFetchInterval    time.Duration
	LastSuccessAt       *time.Time
	LastFailureAt       *time.Time
	ConsecutiveFailures int
	PausedReason        string
}

type SourceRegistration struct {
	ID               string
	FetchURL         string
	MinFetchInterval time.Duration
}

// IsDue reports whether a source may be fetched without violating its per-source
// rate limit. A failed attempt also consumes the interval budget.
func (source Source) IsDue(now time.Time) bool {
	mostRecent := source.LastSuccessAt
	if source.LastFailureAt != nil && (mostRecent == nil || source.LastFailureAt.After(*mostRecent)) {
		mostRecent = source.LastFailureAt
	}
	return mostRecent == nil || !now.Before(mostRecent.Add(source.MinFetchInterval))
}

func (registration SourceRegistration) Validate() error {
	if strings.TrimSpace(registration.ID) == "" {
		return errors.New("source ID is required")
	}
	if registration.MinFetchInterval <= 0 {
		return errors.New("source fetch interval must be positive")
	}
	parsed, err := url.Parse(registration.FetchURL)
	if err != nil {
		return fmt.Errorf("parse source URL: %w", err)
	}
	if parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("source URL must be an unauthenticated HTTPS URL without a fragment")
	}
	return nil
}

type Store interface {
	RegisterSource(ctx context.Context, registration SourceRegistration) error
	UpdateSource(ctx context.Context, registration SourceRegistration, reason string) error
	PauseSource(ctx context.Context, sourceID, reason string) error
	EnableSource(ctx context.Context, sourceID, reason string) error
	DeleteSource(ctx context.Context, sourceID, reason string) error
	RecordRun(ctx context.Context, sourceID string, outcome RunOutcome, candidateCount int, errorCode string) (Source, error)
	RecordPublication(ctx context.Context, fingerprints []string, snapshot PublishedSnapshot) error
	RecordSourceCandidates(ctx context.Context, sourceID string, fingerprints []string, fetchedAt time.Time) error
	EnabledSources(ctx context.Context) ([]Source, error)
	ScreeningCursor(ctx context.Context, sourceID string) (int, error)
	AdvanceScreeningCursor(ctx context.Context, sourceID string, offset int) error
	CandidateProbeDue(ctx context.Context, fingerprint string, now time.Time) (bool, error)
	RecordCandidateProbe(ctx context.Context, fingerprint string, succeeded bool, now time.Time) error
	CandidateQuality(ctx context.Context, fingerprints []string) (map[string]CandidateQuality, error)
	CandidateSources(ctx context.Context, fingerprints []string) (map[string][]string, error)
	CandidateRevocations(ctx context.Context) (map[string]string, error)
	RevokeCandidate(ctx context.Context, fingerprint, reason string) error
	RestoreCandidate(ctx context.Context, fingerprint, reason string) error
	RecordFeedback(ctx context.Context, fingerprint, outcome, latencyBucket string, reportedAt time.Time) error
	RecordFeedbackBatch(ctx context.Context, reports []FeedbackReport, reportedAt time.Time) error
	RecordCandidateAssignments(ctx context.Context, fingerprints []string) error
	PruneExpired(ctx context.Context, now time.Time) error
}

type PublishedSnapshot struct {
	ContentSHA256 string
	ActivePath    string
	BackupPath    string
	PublishedAt   time.Time
}

type FeedbackReport struct {
	CandidateID   string `json:"candidate_id"`
	Outcome       string `json:"outcome"`
	LatencyBucket string `json:"latency_bucket"`
}

func (report FeedbackReport) Validate() error {
	if len(report.CandidateID) != 64 {
		return errors.New("feedback candidate ID is invalid")
	}
	for _, char := range report.CandidateID {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f')) {
			return errors.New("feedback candidate ID is invalid")
		}
	}
	if report.Outcome != "success" && report.Outcome != "failure" {
		return errors.New("feedback outcome is invalid")
	}
	switch report.LatencyBucket {
	case "none", "lt_100", "100_300", "300_1000", "gte_1000":
	default:
		return errors.New("feedback latency bucket is invalid")
	}
	if report.Outcome == "failure" && report.LatencyBucket != "none" || report.Outcome == "success" && report.LatencyBucket == "none" {
		return errors.New("feedback latency bucket does not match outcome")
	}
	return nil
}

type RunOutcome string

const (
	RunSucceeded   RunOutcome = "succeeded"
	RunFailed      RunOutcome = "failed"
	RunScreenedOut RunOutcome = "screened_out"
	RunDeferred    RunOutcome = "deferred"
)
