-- A person's own display name, shown instead of the sign-in name where
-- people see each other. Empty means "use the sign-in name".
CREATE TABLE user_profiles (
    user_id      INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    display_name TEXT NOT NULL DEFAULT ''
);
