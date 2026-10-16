-- Decode aliases before indexing: JSON escapes are not part of a name.
CREATE VIRTUAL TABLE title_search USING fts5(
    name, sort_name, aliases, year,
    tokenize = 'unicode61 remove_diacritics 2'
);

CREATE TRIGGER title_search_insert AFTER INSERT ON titles BEGIN
    INSERT INTO title_search(rowid, name, sort_name, aliases, year)
    VALUES (new.id, new.name, new.sort_name,
        coalesce((SELECT group_concat(value, ' ') FROM json_each(new.aliases)), ''), new.year);
END;

CREATE TRIGGER title_search_delete AFTER DELETE ON titles BEGIN
    DELETE FROM title_search WHERE rowid = old.id;
END;

CREATE TRIGGER title_search_update AFTER UPDATE OF id, name, sort_name, aliases, year ON titles
WHEN old.id IS NOT new.id OR old.name IS NOT new.name OR old.sort_name IS NOT new.sort_name
    OR old.aliases IS NOT new.aliases OR old.year IS NOT new.year
BEGIN
    DELETE FROM title_search WHERE rowid = old.id;
    INSERT INTO title_search(rowid, name, sort_name, aliases, year)
    VALUES (new.id, new.name, new.sort_name,
        coalesce((SELECT group_concat(value, ' ') FROM json_each(new.aliases)), ''), new.year);
END;

INSERT INTO title_search(rowid, name, sort_name, aliases, year)
SELECT id, name, sort_name,
    coalesce((SELECT group_concat(value, ' ') FROM json_each(titles.aliases)), ''), year
FROM titles;
