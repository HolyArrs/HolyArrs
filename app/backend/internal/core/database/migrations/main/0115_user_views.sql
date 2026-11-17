-- Each person's saved view picks (the Library view per page, the Calendar's
-- view and follows), one row per person and name, so a pick follows the
-- person to every device. value is short JSON. Create-only.
CREATE TABLE user_views (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    value   TEXT NOT NULL CHECK (json_valid(value)),
    PRIMARY KEY (user_id, name)
);
