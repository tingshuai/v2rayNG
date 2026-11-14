ALTER TABLE candidate_fingerprints ADD COLUMN consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0);
ALTER TABLE candidate_fingerprints ADD COLUMN last_probe_at TEXT;
ALTER TABLE candidate_fingerprints ADD COLUMN last_success_at TEXT;
ALTER TABLE candidate_fingerprints ADD COLUMN isolated_until TEXT;
ALTER TABLE candidate_fingerprints ADD COLUMN assignment_count INTEGER NOT NULL DEFAULT 0 CHECK (assignment_count >= 0);

CREATE INDEX IF NOT EXISTS candidate_fingerprints_probe_due ON candidate_fingerprints(isolated_until, last_probe_at);
