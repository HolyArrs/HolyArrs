-- What an app such as Profilarr sends to a side's format and profile routes
-- of the arr API, kept as sent (body) so it lists back unchanged.
--
-- compat_formats: one format per side and name. trait_id is the trait it
-- became, NULL when it tests something HolyArrs cannot (skipped says what).
-- AUTOINCREMENT so a removed format's id is never handed out again.
CREATE TABLE compat_formats (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    side       TEXT NOT NULL CHECK (side IN ('sonarr', 'radarr')),
    name       TEXT NOT NULL,
    body       TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(body)),
    trait_id   INTEGER REFERENCES own_traits(id) ON DELETE SET NULL,
    skipped    TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at TEXT NOT NULL, updated_by TEXT NOT NULL,
    UNIQUE (side, name)
);

-- compat_profiles: the Pick rules a profile sent to a side became. The
-- row goes with its Pick rules.
CREATE TABLE compat_profiles (
    pick_rules_id INTEGER PRIMARY KEY REFERENCES pick_rules(id) ON DELETE CASCADE,
    side          TEXT NOT NULL CHECK (side IN ('sonarr', 'radarr')),
    name          TEXT NOT NULL,
    body          TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(body)),
    notes         TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(notes)),
    created_at    TEXT NOT NULL, created_by TEXT NOT NULL,
    updated_at    TEXT NOT NULL, updated_by TEXT NOT NULL,
    UNIQUE (side, name)
);
