CREATE TABLE IF NOT EXISTS feedback_aggregates (
    fingerprint TEXT NOT NULL,
    day_utc TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('success', 'failure')),
    latency_bucket TEXT NOT NULL CHECK (latency_bucket IN ('none', 'lt_100', '100_300', '300_1000', 'gte_1000')),
    report_count INTEGER NOT NULL DEFAULT 0 CHECK (report_count >= 0),
    PRIMARY KEY(fingerprint, day_utc, outcome, latency_bucket)
);

CREATE INDEX IF NOT EXISTS feedback_aggregates_expiry ON feedback_aggregates(day_utc);
