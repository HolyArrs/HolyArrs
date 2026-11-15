-- The address people's browsers open a Jellyfin or Emby server at, for the
-- "Open in Jellyfin" button. HolyArrs' own address for the server (url) is
-- often one only Docker reaches, like http://jellyfin:8096. Empty: no button.
ALTER TABLE media_servers ADD COLUMN web_url TEXT NOT NULL DEFAULT '';
