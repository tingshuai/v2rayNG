package collector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	auditRetention = 180 * 24 * time.Hour
	runRetention   = 30 * 24 * time.Hour
)

type SQLiteStore struct{ db *sql.DB }

func OpenSQLite(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open collector database: %w", err)
	}
	db.SetMaxOpenConns(1) // WAL has one writer; serialize mutations intentionally.
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("configure collector database: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (store *SQLiteStore) Close() error { return store.db.Close() }

func (store *SQLiteStore) Migrate(ctx context.Context, directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		versionText := strings.SplitN(entry.Name(), "_", 2)[0]
		version, err := strconv.Atoi(versionText)
		if err != nil {
			return fmt.Errorf("migration %q has invalid version: %w", entry.Name(), err)
		}
		var applied bool
		if err := store.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", version).Scan(&applied); err != nil && !strings.Contains(err.Error(), "no such table") {
			return err
		}
		if applied {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(contents)); err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", version, time.Now().UTC().Format(time.RFC3339Nano))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %q: %w", entry.Name(), err)
		}
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %q: %w", entry.Name(), err)
		}
	}
	return nil
}

func (store *SQLiteStore) RegisterSource(ctx context.Context, registration SourceRegistration) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO sources(source_id, fetch_url, enabled, contract_version, min_fetch_interval_seconds, created_at, updated_at) VALUES(?, ?, 1, ?, ?, ?, ?)`, registration.ID, registration.FetchURL, ContractVersionV1, int(registration.MinFetchInterval.Seconds()), now, now)
	if err != nil {
		return fmt.Errorf("register source: %w", err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO source_audit(source_id, action, reason, occurred_at) VALUES(?, 'registered', 'distribution_allowed', ?)", registration.ID, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (store *SQLiteStore) UpdateSource(ctx context.Context, registration SourceRegistration, reason string) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return errors.New("source update reason is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE sources SET fetch_url=?, min_fetch_interval_seconds=?, updated_at=? WHERE source_id=?`, registration.FetchURL, int(registration.MinFetchInterval.Seconds()), now, registration.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrSourceNotFound
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO source_audit(source_id, action, reason, occurred_at) VALUES(?, 'updated', ?, ?)", registration.ID, reason, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *SQLiteStore) PauseSource(ctx context.Context, sourceID, reason string) error {
	return store.setSourceEnabled(ctx, sourceID, false, reason, "paused")
}
func (store *SQLiteStore) EnableSource(ctx context.Context, sourceID, reason string) error {
	return store.setSourceEnabled(ctx, sourceID, true, reason, "enabled")
}

func (store *SQLiteStore) DeleteSource(ctx context.Context, sourceID, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("source deletion reason is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "DELETE FROM sources WHERE source_id = ?", sourceID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrSourceNotFound
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO source_audit(source_id, action, reason, occurred_at) VALUES(?, 'deleted', ?, ?)", sourceID, reason, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *SQLiteStore) setSourceEnabled(ctx context.Context, sourceID string, enabled bool, reason, action string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("source state change reason is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, "UPDATE sources SET enabled = ?, paused_reason = ?, updated_at = ? WHERE source_id = ?", enabled, nullablePauseReason(enabled, reason), now, sourceID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrSourceNotFound
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO source_audit(source_id, action, reason, occurred_at) VALUES(?, ?, ?, ?)", sourceID, action, reason, now); err != nil {
		return err
	}
	return tx.Commit()
}

func nullablePauseReason(enabled bool, reason string) any {
	if enabled {
		return nil
	}
	return reason
}

func (store *SQLiteStore) RecordRun(ctx context.Context, sourceID string, outcome RunOutcome, candidateCount int, errorCode string) (Source, error) {
	if outcome != RunSucceeded && outcome != RunFailed {
		return Source{}, errors.New("invalid source run outcome")
	}
	if candidateCount < 0 {
		return Source{}, errors.New("candidate count cannot be negative")
	}
	now := time.Now().UTC()
	nowText := now.Format(time.RFC3339Nano)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return Source{}, err
	}
	defer tx.Rollback()
	var failures int
	err = tx.QueryRowContext(ctx, "SELECT consecutive_failures FROM sources WHERE source_id = ?", sourceID).Scan(&failures)
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, ErrSourceNotFound
	}
	if err != nil {
		return Source{}, err
	}
	enabled := true
	pauseReason := any(nil)
	if outcome == RunSucceeded {
		failures = 0
		_, err = tx.ExecContext(ctx, "UPDATE sources SET last_success_at=?, consecutive_failures=0, updated_at=? WHERE source_id=?", nowText, nowText, sourceID)
	} else {
		failures++
		if failures >= FailurePauseLimit {
			enabled = false
			pauseReason = "consecutive_fetch_failures"
		}
		_, err = tx.ExecContext(ctx, "UPDATE sources SET last_failure_at=?, consecutive_failures=?, enabled=?, paused_reason=?, updated_at=? WHERE source_id=?", nowText, failures, enabled, pauseReason, nowText, sourceID)
	}
	if err != nil {
		return Source{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO source_runs(source_id, started_at, finished_at, outcome, candidate_count, error_code) VALUES(?, ?, ?, ?, ?, ?)", sourceID, nowText, nowText, outcome, candidateCount, nullableString(errorCode)); err != nil {
		return Source{}, err
	}
	if !enabled {
		if _, err = tx.ExecContext(ctx, "INSERT INTO source_audit(source_id, action, reason, occurred_at) VALUES(?, 'paused', 'consecutive_fetch_failures', ?)", sourceID, nowText); err != nil {
			return Source{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return Source{}, err
	}
	return store.source(ctx, sourceID)
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (store *SQLiteStore) EnabledSources(ctx context.Context) ([]Source, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT source_id, fetch_url, enabled, contract_version, min_fetch_interval_seconds, last_success_at, last_failure_at, consecutive_failures, COALESCE(paused_reason, '') FROM sources WHERE enabled=1 ORDER BY source_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []Source
	for rows.Next() {
		source, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

func (store *SQLiteStore) source(ctx context.Context, sourceID string) (Source, error) {
	row := store.db.QueryRowContext(ctx, `SELECT source_id, fetch_url, enabled, contract_version, min_fetch_interval_seconds, last_success_at, last_failure_at, consecutive_failures, COALESCE(paused_reason, '') FROM sources WHERE source_id=?`, sourceID)
	source, err := scanSource(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Source{}, ErrSourceNotFound
	}
	return source, err
}

type scanner interface{ Scan(...any) error }

func scanSource(row scanner) (Source, error) {
	var source Source
	var seconds int
	var success, failure sql.NullString
	err := row.Scan(&source.ID, &source.FetchURL, &source.Enabled, &source.ContractVersion, &seconds, &success, &failure, &source.ConsecutiveFailures, &source.PausedReason)
	if err != nil {
		return Source{}, err
	}
	source.MinFetchInterval = time.Duration(seconds) * time.Second
	var parseErr error
	if success.Valid {
		value, err := time.Parse(time.RFC3339Nano, success.String)
		parseErr = err
		source.LastSuccessAt = &value
	}
	if parseErr == nil && failure.Valid {
		value, err := time.Parse(time.RFC3339Nano, failure.String)
		parseErr = err
		source.LastFailureAt = &value
	}
	return source, parseErr
}

func (store *SQLiteStore) PruneExpired(ctx context.Context, now time.Time) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "DELETE FROM source_audit WHERE occurred_at < ?", now.Add(-auditRetention).UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM source_runs WHERE finished_at < ?", now.Add(-runRetention).UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM candidate_fingerprints WHERE last_seen_at < ?", now.Add(-runRetention).UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}
