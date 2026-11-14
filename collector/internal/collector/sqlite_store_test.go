package collector

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestThirdFailedRunPausesSourceAndExplicitEnableRestoresIt(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	registration := SourceRegistration{ID: "source-one", FetchURL: "https://source.example/subscription", MinFetchInterval: time.Minute}
	if err := store.RegisterSource(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	for range FailurePauseLimit {
		source, err := store.RecordRun(context.Background(), registration.ID, RunFailed, 0, "timeout")
		if err != nil {
			t.Fatal(err)
		}
		if source.ConsecutiveFailures < FailurePauseLimit && !source.Enabled {
			t.Fatal("source paused before failure threshold")
		}
	}
	sources, err := store.EnabledSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 {
		t.Fatalf("enabled sources = %d, want 0", len(sources))
	}
	if err := store.EnableSource(context.Background(), registration.ID, "operator_request"); err != nil {
		t.Fatal(err)
	}
	sources, err = store.EnabledSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("enabled sources after explicit enable = %d, want 1", len(sources))
	}
}

func TestSourceScreeningCursorPersistsAcrossStoreReopen(t *testing.T) {
	database := filepath.Join(t.TempDir(), "collector.sqlite")
	store, err := OpenSQLite(database)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	registration := SourceRegistration{ID: "source-cursor", FetchURL: "https://source.example/subscription", MinFetchInterval: time.Minute}
	if err := store.RegisterSource(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	if err := store.AdvanceScreeningCursor(context.Background(), registration.ID, 42); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = OpenSQLite(database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	offset, err := store.ScreeningCursor(context.Background(), registration.ID)
	if err != nil || offset != 42 {
		t.Fatalf("cursor = %d, error = %v; want 42", offset, err)
	}
}

func TestCandidateProbeStateAppliesCooldownAndRepeatedFailureIsolation(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	due, err := store.CandidateProbeDue(context.Background(), "candidate", now)
	if err != nil || !due {
		t.Fatalf("new candidate due=%v err=%v", due, err)
	}
	for index := range 3 {
		if err := store.RecordCandidateProbe(context.Background(), "candidate", false, now.Add(time.Duration(index)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	due, err = store.CandidateProbeDue(context.Background(), "candidate", now.Add(2*time.Hour+probeCooldown-time.Second))
	if err != nil || due {
		t.Fatalf("isolated candidate due=%v err=%v before TTL", due, err)
	}
	due, err = store.CandidateProbeDue(context.Background(), "candidate", now.Add(2*time.Hour+probeCooldown))
	if err != nil || !due {
		t.Fatalf("isolated candidate due=%v err=%v at TTL", due, err)
	}
	if err := store.RecordCandidateProbe(context.Background(), "candidate", true, now.Add(2*time.Hour+probeCooldown)); err != nil {
		t.Fatal(err)
	}
	due, err = store.CandidateProbeDue(context.Background(), "candidate", now.Add(2*time.Hour+probeCooldown+healthyRecheck-time.Second))
	if err != nil || due {
		t.Fatalf("healthy candidate due=%v err=%v before recheck interval", due, err)
	}
	due, err = store.CandidateProbeDue(context.Background(), "candidate", now.Add(2*time.Hour+probeCooldown+healthyRecheck))
	if err != nil || !due {
		t.Fatalf("healthy candidate due=%v err=%v at recheck interval", due, err)
	}
}

func TestScreenedOutAndDeferredRunsDoNotPauseSource(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	registration := SourceRegistration{ID: "source-screening", FetchURL: "https://source.example/subscription", MinFetchInterval: time.Minute}
	if err := store.RegisterSource(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []RunOutcome{RunScreenedOut, RunDeferred, RunScreenedOut} {
		source, err := store.RecordRun(context.Background(), registration.ID, outcome, 0, "test")
		if err != nil {
			t.Fatal(err)
		}
		if !source.Enabled || source.ConsecutiveFailures != 0 {
			t.Fatalf("source after %s = %#v", outcome, source)
		}
	}
}

func TestRecordPublicationStoresOnlyFingerprintsAndSnapshotMetadata(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	snapshot := PublishedSnapshot{ContentSHA256: "content-hash", ActivePath: "published/subscription.txt", BackupPath: "published/subscription.previous.txt", PublishedAt: time.Now()}
	if err := store.RecordPublication(context.Background(), []string{"fingerprint-one", "fingerprint-two"}, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordPublication(context.Background(), []string{"fingerprint-one"}, snapshot); err != nil {
		t.Fatal(err)
	}
	var fingerprints, snapshots, successes int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM candidate_fingerprints").Scan(&fingerprints); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM published_snapshots").Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT success_count FROM candidate_fingerprints WHERE fingerprint='fingerprint-one'").Scan(&successes); err != nil {
		t.Fatal(err)
	}
	if fingerprints != 2 || snapshots != 2 || successes != 0 {
		t.Fatalf("fingerprints=%d snapshots=%d successful_probes=%d", fingerprints, snapshots, successes)
	}
}

func TestCandidateQualityAndAssignmentPressureAreAggregated(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := store.RecordCandidateProbe(context.Background(), "candidate-good", true, now); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCandidateProbe(context.Background(), "candidate-good", true, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCandidateProbe(context.Background(), "candidate-good", false, now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordCandidateAssignments(context.Background(), []string{"candidate-good", "candidate-good"}); err != nil {
		t.Fatal(err)
	}
	qualities, err := store.CandidateQuality(context.Background(), []string{"candidate-good", "candidate-missing"})
	if err != nil {
		t.Fatal(err)
	}
	quality := qualities["candidate-good"]
	if quality.SuccessCount != 2 || quality.FailureCount != 1 || quality.ConsecutiveFails != 1 || quality.AssignmentCount != 2 || quality.LastSuccessAt == nil {
		t.Fatalf("candidate quality = %#v", quality)
	}
	if _, exists := qualities["candidate-missing"]; exists {
		t.Fatal("missing candidate unexpectedly had persisted quality")
	}
}

func TestPruneExpiredRemovesOnlyExpiredRecords(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.November, 9, 0, 0, 0, 0, time.UTC)
	oldAudit := now.Add(-auditRetention).Add(-time.Nanosecond).Format(time.RFC3339Nano)
	currentAudit := now.Add(-auditRetention).Format(time.RFC3339Nano)
	oldRun := now.Add(-runRetention).Add(-time.Nanosecond).Format(time.RFC3339Nano)
	currentRun := now.Add(-runRetention).Format(time.RFC3339Nano)
	for _, timestamp := range []string{oldAudit, currentAudit} {
		if _, err := store.db.Exec("INSERT INTO source_audit(source_id, action, reason, occurred_at) VALUES('source', 'test', 'test', ?)", timestamp); err != nil {
			t.Fatal(err)
		}
	}
	for _, timestamp := range []string{oldRun, currentRun} {
		if _, err := store.db.Exec("INSERT INTO source_runs(source_id, started_at, finished_at, outcome, candidate_count) VALUES('source', ?, ?, 'failed', 0)", timestamp, timestamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PruneExpired(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var audits, runs int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM source_audit").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM source_runs").Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if audits != 1 || runs != 1 {
		t.Fatalf("retained audits=%d runs=%d, want 1 each", audits, runs)
	}
}

func TestDeleteSourceRemovesConfigurationAndKeepsAuditSummary(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	registration := SourceRegistration{ID: "source-delete", FetchURL: "https://source.example/subscription", MinFetchInterval: time.Minute}
	if err := store.RegisterSource(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteSource(context.Background(), registration.ID, "operator_request"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.source(context.Background(), registration.ID); !errors.Is(err, ErrSourceNotFound) {
		t.Fatalf("deleted source lookup error = %v, want ErrSourceNotFound", err)
	}
	var deletionAudits int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM source_audit WHERE source_id = ? AND action = 'deleted' AND reason = 'operator_request'", registration.ID).Scan(&deletionAudits); err != nil {
		t.Fatal(err)
	}
	if deletionAudits != 1 {
		t.Fatalf("deletion audits = %d, want 1", deletionAudits)
	}
}

func TestUpdateSourceChangesConfigurationAndAuditsReason(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "collector.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(context.Background(), filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	registration := SourceRegistration{ID: "source-update", FetchURL: "https://source.example/first", MinFetchInterval: time.Minute}
	if err := store.RegisterSource(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	updated := SourceRegistration{ID: registration.ID, FetchURL: "https://source.example/second", MinFetchInterval: 2 * time.Minute}
	if err := store.UpdateSource(context.Background(), updated, "operator_request"); err != nil {
		t.Fatal(err)
	}
	source, err := store.source(context.Background(), registration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if source.FetchURL != updated.FetchURL || source.MinFetchInterval != updated.MinFetchInterval {
		t.Fatalf("source after update = %#v", source)
	}
	var updates int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM source_audit WHERE source_id = ? AND action = 'updated' AND reason = 'operator_request'", registration.ID).Scan(&updates); err != nil {
		t.Fatal(err)
	}
	if updates != 1 {
		t.Fatalf("update audits = %d, want 1", updates)
	}
}
