-- The address people's browsers open a Jellyfin or Emby server at, for the
-- Open in button on Library title pages. The server address HolyArrs uses
-- (url) is often one only Docker reaches. Empty: no button.
ALTER TABLE media_servers ADD COLUMN web_url TEXT NOT NULL DEFAULT '';
