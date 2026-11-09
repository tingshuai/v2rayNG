CREATE TABLE IF NOT EXISTS schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sources (
    source_id TEXT PRIMARY KEY,
    fetch_url TEXT NOT NULL,
    enabled INTEGER NOT NULL CHECK (enabled IN (0, 1)),
    contract_version INTEGER NOT NULL,
    min_fetch_interval_seconds INTEGER NOT NULL CHECK (min_fetch_interval_seconds > 0),
    last_success_at TEXT,
    last_failure_at TEXT,
    consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    paused_reason TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS source_audit (
    audit_id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id TEXT NOT NULL,
    action TEXT NOT NULL,
    reason TEXT NOT NULL,
    occurred_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS source_audit_source_time ON source_audit(source_id, occurred_at);

CREATE TABLE IF NOT EXISTS source_runs (
    run_id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id TEXT NOT NULL,
    started_at TEXT NOT NULL,
    finished_at TEXT NOT NULL,
    outcome TEXT NOT NULL,
    candidate_count INTEGER NOT NULL DEFAULT 0 CHECK (candidate_count >= 0),
    error_code TEXT
);

CREATE INDEX IF NOT EXISTS source_runs_source_time ON source_runs(source_id, finished_at);

CREATE TABLE IF NOT EXISTS candidate_fingerprints (
    fingerprint TEXT PRIMARY KEY,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    success_count INTEGER NOT NULL DEFAULT 0 CHECK (success_count >= 0),
    failure_count INTEGER NOT NULL DEFAULT 0 CHECK (failure_count >= 0)
);

CREATE TABLE IF NOT EXISTS published_snapshots (
    snapshot_id INTEGER PRIMARY KEY AUTOINCREMENT,
    content_sha256 TEXT NOT NULL,
    active_path TEXT NOT NULL,
    backup_path TEXT,
    published_at TEXT NOT NULL
);
