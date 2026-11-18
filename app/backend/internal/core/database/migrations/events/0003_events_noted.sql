-- History keeps searches and feed checks that say which Sources they left
-- out. This small index holds only those rows, so the default view does not
-- read every search to find them. Its WHERE must match the filter in events.go.
CREATE INDEX IF NOT EXISTS events_noted ON events(kind, at) WHERE coalesce(json_extract(data, '$.sources_note'), '') <> '';
