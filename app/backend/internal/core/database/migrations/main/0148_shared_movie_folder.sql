-- Several movies may be adopted in place from one collection folder.
ALTER TABLE versions ADD COLUMN shared_folder INTEGER NOT NULL DEFAULT 0;
