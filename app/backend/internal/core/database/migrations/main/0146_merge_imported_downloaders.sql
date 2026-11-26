-- An earlier import added a second Downloader named "<name> (imported)"
-- for a client already here (same kind, same address). Fold each such copy
-- into the one that was here first: its Downloads, history and links move
-- over, the kept one only takes the copy's user name, key and folder maps
-- where its own are empty, and the copy is marked removed. A pair is left
-- alone when anything would clash (the same item tracked on both) or when
-- the copy uses another category and already has Downloads.

-- downloader_merges keeps a record of each pair folded here (add-only:
-- nothing is dropped or deleted).
CREATE TABLE downloader_merges (
    dup  INTEGER NOT NULL PRIMARY KEY,
    keep INTEGER NOT NULL
);

INSERT INTO downloader_merges (dup, keep)
WITH pairs AS (
    SELECT d.id AS dup, k.id AS keep
    FROM downloaders d
    JOIN downloaders k ON k.adapter = d.adapter
        AND d.name = k.name || ' (imported)'
        AND lower(rtrim(trim(d.url), '/')) = lower(rtrim(trim(k.url), '/'))
        AND k.id < d.id
    WHERE d.removed = 0 AND k.removed = 0
)
SELECT p.dup, p.keep FROM pairs p
WHERE p.dup NOT IN (SELECT dup FROM pairs GROUP BY dup HAVING count(*) > 1)
    AND p.keep NOT IN (SELECT keep FROM pairs GROUP BY keep HAVING count(*) > 1)
    AND p.keep NOT IN (SELECT dup FROM pairs)
    AND NOT EXISTS (SELECT 1 FROM downloads a JOIN downloads b ON a.external_id = b.external_id WHERE a.downloader_id = p.dup AND b.downloader_id = p.keep)
    AND NOT EXISTS (SELECT 1 FROM client_links a JOIN client_links b ON a.hash = b.hash WHERE a.downloader_id = p.dup AND b.downloader_id = p.keep)
    AND NOT EXISTS (SELECT 1 FROM torrent_seen a JOIN torrent_seen b ON a.hash = b.hash WHERE a.downloader_id = p.dup AND b.downloader_id = p.keep)
    AND NOT EXISTS (SELECT 1 FROM dl_items a JOIN dl_items b ON a.key = b.key WHERE a.downloader_id = p.dup AND b.downloader_id = p.keep)
    AND NOT EXISTS (SELECT 1 FROM dl_match_queue a JOIN dl_match_queue b ON a.key = b.key WHERE a.downloader_id = p.dup AND b.downloader_id = p.keep)
    AND NOT EXISTS (SELECT 1 FROM downloaders d JOIN downloaders k ON k.id = p.keep
        WHERE d.id = p.dup AND d.category <> k.category
        AND EXISTS (SELECT 1 FROM downloads x WHERE x.downloader_id = d.id));

UPDATE downloaders SET
    username = CASE WHEN username = '' THEN (SELECT d.username FROM downloaders d JOIN downloader_merges m ON m.dup = d.id WHERE m.keep = downloaders.id) ELSE username END,
    secret_ref = COALESCE(secret_ref, (SELECT d.secret_ref FROM downloaders d JOIN downloader_merges m ON m.dup = d.id WHERE m.keep = downloaders.id)),
    path_map = CASE WHEN path_map IN ('', '[]') THEN (SELECT d.path_map FROM downloaders d JOIN downloader_merges m ON m.dup = d.id WHERE m.keep = downloaders.id) ELSE path_map END
WHERE id IN (SELECT keep FROM downloader_merges);

UPDATE downloads SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = downloads.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);
UPDATE downloader_attempts SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = downloader_attempts.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);
UPDATE client_links SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = client_links.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);
UPDATE torrent_log SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = torrent_log.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);
UPDATE torrent_seen SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = torrent_seen.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);
UPDATE dl_items SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = dl_items.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);
UPDATE dl_match_queue SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = dl_match_queue.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);
UPDATE sources SET downloader_id = (SELECT keep FROM downloader_merges WHERE dup = sources.downloader_id) WHERE downloader_id IN (SELECT dup FROM downloader_merges);

-- The copy keeps its row (removed, paused) and gives up its key, now the kept one's when that had none.
UPDATE downloaders SET removed = 1, state = 'disabled', secret_ref = NULL, name = name || ' (removed ' || id || ')' WHERE id IN (SELECT dup FROM downloader_merges);
