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
	EnabledSources(ctx context.Context) ([]Source, error)
	PruneExpired(ctx context.Context, now time.Time) error
}

type PublishedSnapshot struct {
	ContentSHA256 string
	ActivePath    string
	BackupPath    string
	PublishedAt   time.Time
}

type RunOutcome string

const (
	RunSucceeded   RunOutcome = "succeeded"
	RunFailed      RunOutcome = "failed"
	RunScreenedOut RunOutcome = "screened_out"
	RunDeferred    RunOutcome = "deferred"
)
