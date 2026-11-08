-- Alerts may reach a Discord-like server or a mail server on this machine or
-- the local network only when the person allows it for that Alert (off by
-- default). Without it a Discord link must be https on discord.com.
ALTER TABLE alerts ADD COLUMN allow_local_network INTEGER NOT NULL DEFAULT 0;
