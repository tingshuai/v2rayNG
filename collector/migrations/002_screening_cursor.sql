CREATE TABLE IF NOT EXISTS source_screening_cursors (
    source_id TEXT PRIMARY KEY REFERENCES sources(source_id) ON DELETE CASCADE,
    next_offset INTEGER NOT NULL DEFAULT 0 CHECK (next_offset >= 0),
    updated_at TEXT NOT NULL
);
