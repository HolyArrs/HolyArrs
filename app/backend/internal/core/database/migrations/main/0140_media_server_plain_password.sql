-- A Subsonic-style music server signs in with a salted token by default.
-- plain_password = 1 is the owner's choice to send the password itself
-- (servers that check passwords against LDAP); off for every server.
ALTER TABLE media_servers ADD COLUMN plain_password INTEGER NOT NULL DEFAULT 0;
