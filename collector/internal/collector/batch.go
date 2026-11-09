package collector

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultBatchDeadline    = 30 * time.Second
	finalizationDeadline    = 5 * time.Second
	defaultFetchConcurrency = 4
	maxFetchConcurrency     = 4
	defaultTCPConcurrency   = 2
	maxTCPConcurrency       = 2
	maxScreenedCandidates   = 50
	backupRetention         = 7 * 24 * time.Hour
)

type BatchRunner struct {
	Store            Store
	Fetcher          SourceFetcher
	Resolver         Resolver
	TCPChecker       func(context.Context, Resolver, string) error
	WorkDir          string
	FingerprintKey   []byte
	FastDeadline     time.Duration
	FetchConcurrency int
	TCPConcurrency   int
	Prober           CandidateProber
}

type SourceFetcher interface {
	Fetch(context.Context, Source) ([]byte, error)
}

type sourceScreenResult struct {
	source     Source
	candidates []string
	err        error
	deferred   bool
}

func (runner BatchRunner) Run(ctx context.Context) error {
	if len(runner.FingerprintKey) < 32 {
		return errors.New("fingerprint key must contain at least 32 bytes")
	}
	if runner.FastDeadline <= 0 {
		runner.FastDeadline = defaultBatchDeadline
	}
	runner.TCPConcurrency = runner.effectiveTCPConcurrency()
	screeningCtx, cancel := context.WithTimeout(ctx, runner.FastDeadline)
	defer cancel()
	if err := removeExpiredBackup(runner.WorkDir, time.Now()); err != nil {
		return fmt.Errorf("remove expired subscription backup: %w", err)
	}
	if err := cleanupProbeArtifacts(runner.WorkDir); err != nil {
		return fmt.Errorf("clean stale probe artifacts: %w", err)
	}
	if err := runner.Store.PruneExpired(screeningCtx, time.Now()); err != nil {
		return fmt.Errorf("prune expired collector records: %w", err)
	}
	sources, err := runner.Store.EnabledSources(screeningCtx)
	if err != nil {
		return err
	}
	due := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source.IsDue(time.Now()) {
			due = append(due, source)
		}
	}
	var accepted []string
	var acceptedFingerprints []string
	seen := map[string]struct{}{}
	deferredSources := 0
	for result := range runner.screenSources(screeningCtx, due) {
		outcome, errorCode := sourceRunOutcome(result)
		if outcome == RunDeferred {
			deferredSources++
		}
		if err := runner.recordRun(ctx, result.source.ID, outcome, len(result.candidates), errorCode); err != nil {
			return err
		}
		if result.err != nil {
			continue
		}
		for _, candidate := range result.candidates {
			fingerprint := fingerprint(runner.FingerprintKey, candidate)
			if _, exists := seen[fingerprint]; !exists {
				seen[fingerprint] = struct{}{}
				accepted = append(accepted, candidate)
				acceptedFingerprints = append(acceptedFingerprints, fingerprint)
			}
		}
	}
	if len(accepted) == 0 {
		return errors.New("batch produced no screened candidates; retained previous output")
	}
	sort.Strings(accepted)
	finalizationCtx, finalizationCancel := context.WithTimeout(ctx, finalizationDeadline)
	defer finalizationCancel()
	if err := finalizationCtx.Err(); err != nil {
		return fmt.Errorf("finalize batch: %w", err)
	}
	publication, err := publishAtomically(runner.WorkDir, accepted)
	if err != nil {
		return err
	}
	if err := runner.Store.RecordPublication(finalizationCtx, acceptedFingerprints, publication.snapshot); err != nil {
		if rollbackErr := publication.rollback(); rollbackErr != nil {
			return fmt.Errorf("record publication: %w; rollback published output: %v", err, rollbackErr)
		}
		return fmt.Errorf("record publication: %w", err)
	}
	slog.Info("collector batch published", "component", "batch", "enabled_source_count", len(sources), "due_source_count", len(due), "deferred_source_count", deferredSources, "published_candidate_count", len(accepted), "screening_deadline_exceeded", screeningCtx.Err() != nil)
	return nil
}

func (runner BatchRunner) screenSources(ctx context.Context, sources []Source) <-chan sourceScreenResult {
	results := make(chan sourceScreenResult, len(sources))
	jobs := make(chan Source)
	workers := runner.effectiveFetchConcurrency()
	probeSlots := make(chan struct{}, runner.effectiveTCPConcurrency())
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for source := range jobs {
				candidates, deferred, err := runner.fetchAndScreen(ctx, source, probeSlots)
				results <- sourceScreenResult{source: source, candidates: candidates, deferred: deferred, err: err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, source := range sources {
			jobs <- source
		}
	}()
	go func() {
		group.Wait()
		close(results)
	}()
	return results
}

func (runner BatchRunner) recordRun(ctx context.Context, sourceID string, outcome RunOutcome, candidateCount int, errorCode string) error {
	finalizationCtx, cancel := context.WithTimeout(ctx, finalizationDeadline)
	defer cancel()
	_, err := runner.Store.RecordRun(finalizationCtx, sourceID, outcome, candidateCount, errorCode)
	return err
}

func sourceRunOutcome(result sourceScreenResult) (RunOutcome, string) {
	if result.deferred {
		return RunDeferred, "batch_deadline_exceeded"
	}
	if errors.Is(result.err, ErrNoScreenedCandidates) {
		return RunScreenedOut, "no_candidates_passed"
	}
	if result.err != nil {
		return RunFailed, "fetch_or_parse_failed"
	}
	return RunSucceeded, ""
}

func (runner BatchRunner) effectiveTCPConcurrency() int {
	if runner.TCPConcurrency <= 0 {
		return defaultTCPConcurrency
	}
	if runner.TCPConcurrency > maxTCPConcurrency {
		return maxTCPConcurrency
	}
	return runner.TCPConcurrency
}

func (runner BatchRunner) effectiveFetchConcurrency() int {
	if runner.FetchConcurrency <= 0 {
		return defaultFetchConcurrency
	}
	if runner.FetchConcurrency > maxFetchConcurrency {
		return maxFetchConcurrency
	}
	return runner.FetchConcurrency
}

func (runner BatchRunner) fetchAndScreen(ctx context.Context, source Source, probeSlots chan struct{}) ([]string, bool, error) {
	body, err := runner.Fetcher.Fetch(ctx, source)
	if err != nil {
		return nil, errors.Is(ctx.Err(), context.DeadlineExceeded), err
	}
	candidates, err := splitCandidates(body)
	if err != nil {
		return nil, errors.Is(ctx.Err(), context.DeadlineExceeded), err
	}
	screened := candidates
	if runner.Prober != nil {
		screened = selectProbeCandidates(candidates)
	}
	accepted, failures := runner.screenCandidates(ctx, candidates, probeSlots)
	slog.Info("candidate screening summary", "component", "batch", "source_id", source.ID, "received_count", len(candidates), "eligible_count", len(screened), "unsupported_count", len(candidates)-len(screened), "accepted_count", len(accepted), "tcp_failed_count", failures["tcp_failed"], "proxy_http_failed_count", failures["proxy_http_failed"], "http_status_count", failures["http_status"], "invalid_count", failures["invalid"])
	if len(accepted) == 0 {
		return nil, errors.Is(ctx.Err(), context.DeadlineExceeded), ErrNoScreenedCandidates
	}
	return accepted, errors.Is(ctx.Err(), context.DeadlineExceeded), nil
}

func (runner BatchRunner) screenCandidates(ctx context.Context, candidates []string, probeSlots chan struct{}) ([]string, map[string]int) {
	if runner.Prober != nil {
		candidates = selectProbeCandidates(candidates)
	} else if len(candidates) > maxScreenedCandidates {
		candidates = candidates[:maxScreenedCandidates]
	}
	limit := len(candidates)
	accepted := make(chan string, limit)
	failures := make(chan string, limit)
	jobs := make(chan string)
	var workers sync.WaitGroup
	for range runner.TCPConcurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for candidate := range jobs {
				endpoint, err := candidateEndpoint(candidate)
				if err != nil {
					failures <- "invalid"
					continue
				}
				{
					select {
					case probeSlots <- struct{}{}:
					case <-ctx.Done():
						return
					}
					probeContext, cancel := context.WithTimeout(ctx, 3*time.Second)
					err = runner.tcpReachable(probeContext, endpoint)
					if err == nil && runner.Prober != nil {
						err = runner.Prober.Probe(probeContext, candidate)
					}
					cancel()
					<-probeSlots
					if err == nil {
						select {
						case accepted <- candidate:
						case <-ctx.Done():
						}
					} else if failure, ok := err.(probeFailure); ok {
						failures <- failure.category
					} else {
						failures <- "tcp_failed"
					}
				}
			}
		}()
	}
	go func() {
		workers.Wait()
		close(accepted)
		close(failures)
	}()
	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case jobs <- candidate:
			case <-ctx.Done():
				return
			}
		}
	}()
	result := make([]string, 0, limit)
	counts := map[string]int{}
	for accepted != nil || failures != nil {
		select {
		case candidate, open := <-accepted:
			if !open {
				accepted = nil
			} else {
				result = append(result, candidate)
			}
		case category, open := <-failures:
			if !open {
				failures = nil
			} else {
				counts[category]++
			}
		case <-ctx.Done():
			return result, counts
		}
	}
	return result, counts
}

func (runner BatchRunner) tcpReachable(ctx context.Context, endpoint string) error {
	if runner.TCPChecker != nil {
		return runner.TCPChecker(ctx, runner.Resolver, endpoint)
	}
	return tcpReachable(ctx, runner.Resolver, endpoint)
}

func selectProbeCandidates(candidates []string) []string {
	eligible := make([]string, 0, maxScreenedCandidates)
	for _, candidate := range candidates {
		if !supportsBoundedProbe(candidate) {
			continue
		}
		eligible = append(eligible, candidate)
	}
	if len(eligible) <= maxScreenedCandidates {
		return eligible
	}
	selected := make([]string, 0, maxScreenedCandidates)
	for index := range maxScreenedCandidates {
		selected = append(selected, eligible[index*len(eligible)/maxScreenedCandidates])
	}
	return selected
}

func candidateEndpoint(candidate string) (string, error) {
	parsed, err := url.Parse(candidate)
	if err != nil {
		return "", err
	}
	if parsed.Hostname() == "" || parsed.Port() == "" {
		return "", errors.New("candidate lacks host or port")
	}
	switch parsed.Scheme {
	case "vless", "trojan", "ss":
	default:
		return "", errors.New("unsupported candidate scheme")
	}
	return net.JoinHostPort(parsed.Hostname(), parsed.Port()), nil
}

func tcpReachable(ctx context.Context, resolver Resolver, endpoint string) error {
	dial := safeDialContext(resolver, &net.Dialer{Timeout: 5 * time.Second})
	connection, err := dial(ctx, "tcp", endpoint)
	if err != nil {
		return err
	}
	return connection.Close()
}

func fingerprint(key []byte, candidate string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(candidate))
	return fmt.Sprintf("%x", mac.Sum(nil))
}

type publication struct {
	active      string
	backup      string
	hadPrevious bool
	snapshot    PublishedSnapshot
}

func (publication publication) rollback() error {
	if err := os.Remove(publication.active); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if publication.hadPrevious {
		return os.Rename(publication.backup, publication.active)
	}
	return nil
}

func publishAtomically(workDir string, candidates []string) (publication, error) {
	directory := filepath.Join(workDir, "published")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return publication{}, err
	}
	decoded := []byte(strings.Join(candidates, "\n"))
	contents := []byte(base64.StdEncoding.EncodeToString(decoded))
	temporary, err := os.CreateTemp(directory, ".subscription-*")
	if err != nil {
		return publication{}, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return publication{}, err
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return publication{}, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return publication{}, err
	}
	if err := temporary.Close(); err != nil {
		return publication{}, err
	}
	active := filepath.Join(directory, "subscription.txt")
	backup := filepath.Join(directory, "subscription.previous.txt")
	published := publication{active: active, backup: backup, snapshot: PublishedSnapshot{ContentSHA256: fmt.Sprintf("%x", sha256.Sum256(decoded)), ActivePath: filepath.Join("published", "subscription.txt"), PublishedAt: time.Now().UTC()}}
	if _, err := os.Stat(active); err == nil {
		_ = os.Remove(backup)
		if err := os.Rename(active, backup); err != nil {
			return publication{}, err
		}
		published.hadPrevious = true
		published.snapshot.BackupPath = filepath.Join("published", "subscription.previous.txt")
		if err := os.Chtimes(backup, time.Now(), time.Now()); err != nil {
			return publication{}, err
		}
	}
	if err := os.Rename(temporaryName, active); err != nil {
		if published.hadPrevious {
			_ = os.Rename(backup, active)
		}
		return publication{}, err
	}
	return published, nil
}

func removeExpiredBackup(workDir string, now time.Time) error {
	backup := filepath.Join(workDir, "published", "subscription.previous.txt")
	info, err := os.Stat(backup)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if now.Before(info.ModTime().Add(backupRetention)) {
		return nil
	}
	return os.Remove(backup)
}
