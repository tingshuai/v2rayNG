CREATE TABLE IF NOT EXISTS source_inventories (
    source_id TEXT PRIMARY KEY REFERENCES sources(source_id) ON DELETE CASCADE,
    succeeded_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS source_candidate_presence (
    source_id TEXT NOT NULL REFERENCES sources(source_id) ON DELETE CASCADE,
    fingerprint TEXT NOT NULL REFERENCES candidate_fingerprints(fingerprint) ON DELETE CASCADE,
    last_present_at TEXT NOT NULL,
    PRIMARY KEY(source_id, fingerprint)
);

CREATE INDEX IF NOT EXISTS source_candidate_presence_candidate ON source_candidate_presence(fingerprint);
