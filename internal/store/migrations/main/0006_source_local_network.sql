-- Sources may reach private, loopback, link-local and CGNAT addresses only
-- when the person allows it for that Source (off by default).
ALTER TABLE sources ADD COLUMN allow_local_network INTEGER NOT NULL DEFAULT 0;
