-- 3.4.3: signed-in devices. A session keeps one public id (family) across
-- its hourly id rotation, so the screens can name it and end it; the old
-- row a rotation leaves for requests in flight is marked replaced and is
-- not listed. The id is random, never the cookie or its hash.
ALTER TABLE sessions ADD COLUMN family TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN replaced INTEGER NOT NULL DEFAULT 0;
UPDATE sessions SET family = lower(hex(randomblob(16))) WHERE family = '';
CREATE INDEX sessions_family ON sessions(family);
