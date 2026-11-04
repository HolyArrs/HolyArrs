-- Bridges (#96): a Jackett or Prowlarr the person connects in Settings >
-- Sources. HolyArrs keeps the Sources it made from one in step with it;
-- each such Source names its bridge in its extra ("bridge", "bridge_site").
CREATE TABLE bridges (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at TEXT NOT NULL,
  created_by TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(created_by)),
  updated_at TEXT NOT NULL,
  updated_by TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(updated_by)),
  kind TEXT NOT NULL CHECK(kind IN ('jackett', 'prowlarr')),
  url TEXT NOT NULL,
  secret_ref INTEGER,
  -- How HolyArrs is reached from the bridge (Prowlarr calls back into HolyArrs).
  callback_url TEXT NOT NULL DEFAULT '',
  last_check TEXT,
  last_error TEXT NOT NULL DEFAULT '',
  site_count INTEGER NOT NULL DEFAULT 0,
  extra TEXT NOT NULL DEFAULT '{}' CHECK(json_valid(extra))
);
CREATE UNIQUE INDEX bridges_url ON bridges(kind, lower(url));

INSERT INTO schedules (created_at, created_by, updated_at, updated_by, kind, every_seconds, enabled) VALUES
    (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', 'check_bridges', 21600, 1)
ON CONFLICT (kind) DO NOTHING;
