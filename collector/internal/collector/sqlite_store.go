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
	auditRetention = 30 * 24 * time.Hour
	runRetention   = 30 * 24 * time.Hour
	probeCooldown  = 30 * time.Minute
	healthyRecheck = 6 * time.Hour
	isolationTime  = 30 * time.Minute
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
	if outcome != RunSucceeded && outcome != RunFailed && outcome != RunScreenedOut && outcome != RunDeferred {
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
	} else if outcome == RunFailed {
		failures++
		if failures >= FailurePauseLimit {
			enabled = false
			pauseReason = "consecutive_fetch_failures"
		}
		_, err = tx.ExecContext(ctx, "UPDATE sources SET last_failure_at=?, consecutive_failures=?, enabled=?, paused_reason=?, updated_at=? WHERE source_id=?", nowText, failures, enabled, pauseReason, nowText, sourceID)
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE sources SET updated_at=? WHERE source_id=?", nowText, sourceID)
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

func (store *SQLiteStore) RecordPublication(ctx context.Context, fingerprints []string, snapshot PublishedSnapshot) error {
	if snapshot.ContentSHA256 == "" || snapshot.ActivePath == "" || snapshot.PublishedAt.IsZero() {
		return errors.New("published snapshot is incomplete")
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	publishedAt := snapshot.PublishedAt.UTC().Format(time.RFC3339Nano)
	for _, value := range fingerprints {
		if _, err := tx.ExecContext(ctx, `INSERT INTO candidate_fingerprints(fingerprint, first_seen_at, last_seen_at, success_count, failure_count) VALUES(?, ?, ?, 0, 0) ON CONFLICT(fingerprint) DO UPDATE SET last_seen_at=excluded.last_seen_at`, value, publishedAt, publishedAt); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO published_snapshots(content_sha256, active_path, backup_path, published_at) VALUES(?, ?, ?, ?)`, snapshot.ContentSHA256, snapshot.ActivePath, nullableString(snapshot.BackupPath), publishedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
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

func (store *SQLiteStore) ScreeningCursor(ctx context.Context, sourceID string) (int, error) {
	var offset int
	err := store.db.QueryRowContext(ctx, "SELECT next_offset FROM source_screening_cursors WHERE source_id = ?", sourceID).Scan(&offset)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return offset, err
}

func (store *SQLiteStore) AdvanceScreeningCursor(ctx context.Context, sourceID string, offset int) error {
	if offset < 0 {
		return errors.New("screening cursor cannot be negative")
	}
	_, err := store.db.ExecContext(ctx, `INSERT INTO source_screening_cursors(source_id, next_offset, updated_at)
		VALUES(?, ?, ?) ON CONFLICT(source_id) DO UPDATE SET next_offset=excluded.next_offset, updated_at=excluded.updated_at`,
		sourceID, offset, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (store *SQLiteStore) CandidateProbeDue(ctx context.Context, candidateFingerprint string, now time.Time) (bool, error) {
	var lastProbe, isolatedUntil sql.NullString
	var consecutiveFailures int
	err := store.db.QueryRowContext(ctx, `SELECT last_probe_at, isolated_until, consecutive_failures
		FROM candidate_fingerprints WHERE fingerprint = ?`, candidateFingerprint).Scan(&lastProbe, &isolatedUntil, &consecutiveFailures)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if isolatedUntil.Valid {
		until, err := time.Parse(time.RFC3339Nano, isolatedUntil.String)
		if err != nil {
			return false, err
		}
		if now.Before(until) {
			return false, nil
		}
	}
	if !lastProbe.Valid {
		return true, nil
	}
	last, err := time.Parse(time.RFC3339Nano, lastProbe.String)
	if err != nil {
		return false, err
	}
	cooldown := healthyRecheck
	if consecutiveFailures > 0 {
		cooldown = probeCooldown
	}
	return !now.Before(last.Add(cooldown)), nil
}

func (store *SQLiteStore) RecordCandidateProbe(ctx context.Context, candidateFingerprint string, succeeded bool, now time.Time) error {
	nowText := now.UTC().Format(time.RFC3339Nano)
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO candidate_fingerprints(fingerprint, first_seen_at, last_seen_at, success_count, failure_count, consecutive_failures, last_probe_at, last_success_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(fingerprint) DO UPDATE SET
		last_seen_at=excluded.last_seen_at,
		last_probe_at=excluded.last_probe_at,
		last_success_at=CASE WHEN ? THEN excluded.last_success_at ELSE candidate_fingerprints.last_success_at END,
		success_count=candidate_fingerprints.success_count + ?,
		failure_count=candidate_fingerprints.failure_count + ?,
		consecutive_failures=CASE WHEN ? THEN 0 ELSE candidate_fingerprints.consecutive_failures + 1 END,
		isolated_until=CASE WHEN ? THEN NULL WHEN candidate_fingerprints.consecutive_failures + 1 >= 3 THEN ? ELSE candidate_fingerprints.isolated_until END`,
		candidateFingerprint, nowText, nowText, boolInt(succeeded), boolInt(!succeeded), boolInt(!succeeded), nowText, nullableStringIf(succeeded, nowText),
		succeeded, boolInt(succeeded), boolInt(!succeeded), succeeded, succeeded, now.Add(isolationTime).UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return tx.Commit()
}

func (store *SQLiteStore) CandidateQuality(ctx context.Context, fingerprints []string) (map[string]CandidateQuality, error) {
	qualities := make(map[string]CandidateQuality, len(fingerprints))
	if len(fingerprints) == 0 {
		return qualities, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(fingerprints)), ",")
	args := make([]any, 0, len(fingerprints))
	for _, value := range fingerprints {
		args = append(args, value)
	}
	rows, err := store.db.QueryContext(ctx, `SELECT fingerprint, success_count, failure_count, consecutive_failures, assignment_count, last_success_at
		FROM candidate_fingerprints WHERE fingerprint IN (`+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var fingerprint string
		var quality CandidateQuality
		var lastSuccess sql.NullString
		if err := rows.Scan(&fingerprint, &quality.SuccessCount, &quality.FailureCount, &quality.ConsecutiveFails, &quality.AssignmentCount, &lastSuccess); err != nil {
			return nil, err
		}
		if lastSuccess.Valid {
			parsed, err := time.Parse(time.RFC3339Nano, lastSuccess.String)
			if err != nil {
				return nil, err
			}
			quality.LastSuccessAt = &parsed
		}
		qualities[fingerprint] = quality
	}
	return qualities, rows.Err()
}

func (store *SQLiteStore) RecordCandidateAssignments(ctx context.Context, fingerprints []string) error {
	if len(fingerprints) == 0 {
		return nil
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, candidateFingerprint := range fingerprints {
		if _, err := tx.ExecContext(ctx, `INSERT INTO candidate_fingerprints(fingerprint, first_seen_at, last_seen_at, success_count, failure_count, assignment_count)
			VALUES(?, ?, ?, 0, 0, 1)
			ON CONFLICT(fingerprint) DO UPDATE SET assignment_count=candidate_fingerprints.assignment_count + 1`, candidateFingerprint, now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableStringIf(ok bool, value string) any {
	if !ok {
		return nil
	}
	return value
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
