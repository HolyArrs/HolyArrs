-- One profile picture per person: a 256 by 256 JPEG, made by the server from
-- the upload. The etag is the short hash of the bytes, for caching.
CREATE TABLE user_avatars (
    user_id    INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    jpeg       BLOB NOT NULL,
    etag       TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
