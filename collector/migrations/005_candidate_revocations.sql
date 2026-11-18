CREATE TABLE IF NOT EXISTS candidate_revocations (
    fingerprint TEXT PRIMARY KEY,
    reason TEXT NOT NULL,
    revoked_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS candidate_revocation_audit (
    audit_id INTEGER PRIMARY KEY AUTOINCREMENT,
    fingerprint TEXT NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('revoked', 'restored')),
    reason TEXT NOT NULL,
    occurred_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS candidate_revocation_audit_time ON candidate_revocation_audit(occurred_at);
