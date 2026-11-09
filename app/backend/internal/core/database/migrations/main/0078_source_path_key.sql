-- An rss address may keep its passkey in the path (rss.example/<passkey>):
-- that part goes to the vault and the stored address holds a marker.
ALTER TABLE sources ADD COLUMN path_secret_ref INTEGER;
