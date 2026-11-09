package collector

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type recordingStore struct {
	pruned   bool
	pruneErr error
}

func (store *recordingStore) RegisterSource(context.Context, SourceRegistration) error { return nil }
func (store *recordingStore) UpdateSource(context.Context, SourceRegistration, string) error {
	return nil
}
func (store *recordingStore) PauseSource(context.Context, string, string) error  { return nil }
func (store *recordingStore) EnableSource(context.Context, string, string) error { return nil }
func (store *recordingStore) DeleteSource(context.Context, string, string) error { return nil }
func (store *recordingStore) RecordRun(context.Context, string, RunOutcome, int, string) (Source, error) {
	return Source{}, nil
}
func (store *recordingStore) EnabledSources(context.Context) ([]Source, error) { return nil, nil }
func (store *recordingStore) PruneExpired(context.Context, time.Time) error {
	store.pruned = true
	return store.pruneErr
}

func TestPublishAtomicallyKeepsPreviousOutputAsRollback(t *testing.T) {
	workDir := t.TempDir()
	if err := publishAtomically(workDir, []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if err := publishAtomically(workDir, []string{"two"}); err != nil {
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

func TestRemoveExpiredBackupRetainsOnlyRecentRollbackOutput(t *testing.T) {
	workDir := t.TempDir()
	if err := publishAtomically(workDir, []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if err := publishAtomically(workDir, []string{"two"}); err != nil {
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
