-- Sources: an optional proxy per Source (stage 3.0.2) and a record of every
-- call for the health and stats page (stage 3.0.3).
ALTER TABLE sources ADD COLUMN proxy_url TEXT NOT NULL DEFAULT '';
ALTER TABLE sources ADD COLUMN proxy_secret_ref INTEGER;
ALTER TABLE sources ADD COLUMN proxy_allow_local_network INTEGER NOT NULL DEFAULT 0;

CREATE TABLE source_calls (
    id         INTEGER PRIMARY KEY,
    source_id  INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    at         TEXT NOT NULL,
    op         TEXT NOT NULL CHECK (op IN ('caps', 'search', 'feed', 'test')),
    ok         INTEGER NOT NULL,
    ms         INTEGER NOT NULL,
    error_kind TEXT NOT NULL DEFAULT ''
);
CREATE INDEX source_calls_by_source ON source_calls(source_id, at);
