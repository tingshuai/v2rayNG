package collector

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recordingStore struct {
	pruned   bool
	pruneErr error
	sources  []Source
	outcomes []RunOutcome
}

func (store *recordingStore) RegisterSource(context.Context, SourceRegistration) error { return nil }
func (store *recordingStore) UpdateSource(context.Context, SourceRegistration, string) error {
	return nil
}
func (store *recordingStore) PauseSource(context.Context, string, string) error  { return nil }
func (store *recordingStore) EnableSource(context.Context, string, string) error { return nil }
func (store *recordingStore) DeleteSource(context.Context, string, string) error { return nil }
func (store *recordingStore) RecordRun(_ context.Context, _ string, outcome RunOutcome, _ int, _ string) (Source, error) {
	store.outcomes = append(store.outcomes, outcome)
	return Source{}, nil
}
func (store *recordingStore) RecordPublication(context.Context, []string, PublishedSnapshot) error {
	return nil
}
func (store *recordingStore) EnabledSources(context.Context) ([]Source, error) {
	return store.sources, nil
}
func (store *recordingStore) PruneExpired(context.Context, time.Time) error {
	store.pruned = true
	return store.pruneErr
}

func TestPublishAtomicallyKeepsPreviousOutputAsRollback(t *testing.T) {
	workDir := t.TempDir()
	if _, err := publishAtomically(workDir, []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := publishAtomically(workDir, []string{"two"}); err != nil {
		t.Fatal(err)
	}
	previous, err := os.ReadFile(filepath.Join(workDir, "published", "subscription.previous.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(previous) != "b25l" {
		t.Fatalf("previous output = %q, want base64(one)", previous)
	}
}

func TestPublicationRollbackRemovesFirstOutputWhenAuditCannotBeRecorded(t *testing.T) {
	workDir := t.TempDir()
	published, err := publishAtomically(workDir, []string{"one"})
	if err != nil {
		t.Fatal(err)
	}
	if published.snapshot.BackupPath != "" {
		t.Fatalf("first publication backup path = %q, want empty", published.snapshot.BackupPath)
	}
	if err := published.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workDir, "published", "subscription.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first publication survived rollback: %v", err)
	}
}

func TestRemoveExpiredBackupRetainsOnlyRecentRollbackOutput(t *testing.T) {
	workDir := t.TempDir()
	if _, err := publishAtomically(workDir, []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if _, err := publishAtomically(workDir, []string{"two"}); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(workDir, "published", "subscription.previous.txt")
	now := time.Now()
	if err := removeExpiredBackup(workDir, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("recent backup was removed: %v", err)
	}
	if err := os.Chtimes(backup, now.Add(-backupRetention).Add(-time.Nanosecond), now.Add(-backupRetention).Add(-time.Nanosecond)); err != nil {
		t.Fatal(err)
	}
	if err := removeExpiredBackup(workDir, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expired backup still exists or could not be checked: %v", err)
	}
}

func TestBatchRunPrunesExpiredRecordsBeforeReadingSources(t *testing.T) {
	store := &recordingStore{}
	runner := BatchRunner{Store: store, FingerprintKey: make([]byte, 32)}
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded without candidates")
	}
	if !store.pruned {
		t.Fatal("Run did not prune expired records")
	}
}

func TestBatchRunStopsWhenPruningFails(t *testing.T) {
	store := &recordingStore{pruneErr: errors.New("database unavailable")}
	runner := BatchRunner{Store: store, FingerprintKey: make([]byte, 32)}
	if err := runner.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded after pruning failed")
	}
}

func TestEffectiveTCPConcurrencyIsBounded(t *testing.T) {
	if got := (BatchRunner{}).effectiveTCPConcurrency(); got != defaultTCPConcurrency {
		t.Fatalf("default concurrency = %d, want %d", got, defaultTCPConcurrency)
	}
	if got := (BatchRunner{TCPConcurrency: maxTCPConcurrency + 1}).effectiveTCPConcurrency(); got != maxTCPConcurrency {
		t.Fatalf("capped concurrency = %d, want %d", got, maxTCPConcurrency)
	}
}

func TestEffectiveFetchConcurrencyIsBounded(t *testing.T) {
	if got := (BatchRunner{}).effectiveFetchConcurrency(); got != defaultFetchConcurrency {
		t.Fatalf("default fetch concurrency = %d, want %d", got, defaultFetchConcurrency)
	}
	if got := (BatchRunner{FetchConcurrency: maxFetchConcurrency + 1}).effectiveFetchConcurrency(); got != maxFetchConcurrency {
		t.Fatalf("capped fetch concurrency = %d, want %d", got, maxFetchConcurrency)
	}
}

type fixedFetcher struct{ body []byte }

func (fetcher fixedFetcher) Fetch(context.Context, Source) ([]byte, error) { return fetcher.body, nil }

type successfulProber struct{}

func (successfulProber) Probe(context.Context, string) error { return nil }

func TestBatchPublishesAcceptedCandidatesAfterScreeningDeadline(t *testing.T) {
	store := &recordingStore{sources: []Source{{ID: "source-one", MinFetchInterval: time.Minute}}}
	candidates := "trojan://credential@first.example:443\ntrojan://credential@second.example:443"
	var mutex sync.Mutex
	checks := 0
	runner := BatchRunner{
		Store:            store,
		Fetcher:          fixedFetcher{body: []byte(candidates)},
		FingerprintKey:   make([]byte, 32),
		WorkDir:          t.TempDir(),
		FastDeadline:     20 * time.Millisecond,
		FetchConcurrency: 1,
		TCPConcurrency:   1,
		Prober:           successfulProber{},
		TCPChecker: func(ctx context.Context, _ Resolver, _ string) error {
			mutex.Lock()
			checks++
			check := checks
			mutex.Unlock()
			if check == 1 {
				return nil
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(runner.WorkDir, "published", "subscription.txt"))
	if err != nil {
		t.Fatal(err)
	}
	published, err := base64.StdEncoding.DecodeString(string(contents))
	if err != nil {
		t.Fatal(err)
	}
	if string(published) != "trojan://credential@first.example:443" {
		t.Fatalf("published candidates = %q", published)
	}
	if len(store.outcomes) != 1 || store.outcomes[0] != RunDeferred {
		t.Fatalf("outcomes = %#v, want one deferred outcome", store.outcomes)
	}
}

func TestScreenSourcesSharesGlobalProbeConcurrency(t *testing.T) {
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	var inFlight, peak int32
	runner := BatchRunner{
		Fetcher:          fixedFetcher{body: []byte("trojan://credential@node.example:443")},
		FetchConcurrency: 3,
		TCPConcurrency:   maxTCPConcurrency,
		Prober:           successfulProber{},
		TCPChecker: func(ctx context.Context, _ Resolver, _ string) error {
			current := atomic.AddInt32(&inFlight, 1)
			defer atomic.AddInt32(&inFlight, -1)
			for {
				observed := atomic.LoadInt32(&peak)
				if current <= observed || atomic.CompareAndSwapInt32(&peak, observed, current) {
					break
				}
			}
			entered <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	results := runner.screenSources(context.Background(), []Source{{ID: "one"}, {ID: "two"}, {ID: "three"}})
	<-entered
	<-entered
	if got := atomic.LoadInt32(&peak); got != maxTCPConcurrency {
		t.Fatalf("concurrent probes = %d, want %d", got, maxTCPConcurrency)
	}
	close(release)
	count := 0
	for result := range results {
		if result.err != nil || len(result.candidates) != 1 {
			t.Fatalf("screen result = %#v", result)
		}
		count++
	}
	if count != 3 {
		t.Fatalf("screened source count = %d, want 3", count)
	}
}
