CREATE TABLE IF NOT EXISTS candidate_daily_assignments (
    fingerprint TEXT NOT NULL,
    day_utc TEXT NOT NULL,
    assignment_count INTEGER NOT NULL DEFAULT 0 CHECK (assignment_count >= 0),
    PRIMARY KEY(fingerprint, day_utc)
);

CREATE INDEX IF NOT EXISTS candidate_daily_assignments_expiry ON candidate_daily_assignments(day_utc);
