-- Individual adoption checks one path rather than scanning every library file.
CREATE INDEX files_active_path ON files(lower(relative_path))
WHERE state IN ('present','missing');

-- Shared collection imports need only convert older exclusive parents once.
CREATE INDEX versions_exclusive_folder ON versions(library_folder_id, lower(folder_name))
WHERE shared_folder = 0;
