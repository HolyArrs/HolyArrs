-- History keeps the plugin a change came through, if any.
ALTER TABLE events ADD COLUMN via_plugin TEXT;
