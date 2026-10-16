-- 3.5.13 review: Sources and Downloaders that `holyarrs import-secrets`
-- made. They are made paused; a re-run pauses again one of these that is
-- somehow active. The owner resuming it (state leaves 'disabled') or
-- deleting it turns the mark off (live = 0), so a re-run never undoes the
-- owner's choice. Rows are only added or turned off (M1 rule).
CREATE TABLE import_created (
    tbl     TEXT NOT NULL CHECK (tbl IN ('sources', 'downloaders')),
    item_id INTEGER NOT NULL,
    live    INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (tbl, item_id)
);

CREATE TRIGGER import_created_source_resumed AFTER UPDATE OF state ON sources
WHEN OLD.state = 'disabled' AND NEW.state <> 'disabled'
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'sources' AND item_id = NEW.id;
END;

CREATE TRIGGER import_created_source_gone AFTER DELETE ON sources
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'sources' AND item_id = OLD.id;
END;

CREATE TRIGGER import_created_downloader_resumed AFTER UPDATE OF state ON downloaders
WHEN OLD.state = 'disabled' AND NEW.state <> 'disabled'
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = NEW.id;
END;

CREATE TRIGGER import_created_downloader_gone AFTER DELETE ON downloaders
BEGIN
    UPDATE import_created SET live = 0 WHERE tbl = 'downloaders' AND item_id = OLD.id;
END;
