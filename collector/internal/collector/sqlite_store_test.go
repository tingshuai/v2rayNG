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
