-- Search finds a title by its original name too ("Le Fabuleux Destin
-- d'Amélie Poulain" for Amélie): the original name joins the aliases column
-- of the full-text index.
DROP TRIGGER title_search_insert;
DROP TRIGGER title_search_update;

CREATE TRIGGER title_search_insert AFTER INSERT ON titles BEGIN
    INSERT INTO title_search(rowid, name, sort_name, aliases, year)
    VALUES (new.id, new.name, new.sort_name,
        trim(coalesce(new.original_name, '') || ' ' || coalesce((SELECT group_concat(value, ' ') FROM json_each(new.aliases)), '')), new.year);
END;

CREATE TRIGGER title_search_update AFTER UPDATE OF id, name, sort_name, aliases, year, original_name ON titles
WHEN old.id IS NOT new.id OR old.name IS NOT new.name OR old.sort_name IS NOT new.sort_name
    OR old.aliases IS NOT new.aliases OR old.year IS NOT new.year OR old.original_name IS NOT new.original_name
BEGIN
    UPDATE title_search SET rowid = new.id, name = new.name, sort_name = new.sort_name,
        aliases = trim(coalesce(new.original_name, '') || ' ' || coalesce((SELECT group_concat(value, ' ') FROM json_each(new.aliases)), '')), year = new.year
    WHERE rowid = old.id;
END;

INSERT OR REPLACE INTO title_search(rowid, name, sort_name, aliases, year)
SELECT id, name, sort_name,
    trim(coalesce(original_name, '') || ' ' || coalesce((SELECT group_concat(value, ' ') FROM json_each(titles.aliases)), '')), year
FROM titles;
