-- Skipping sign-in on the local network now covers only the networks the
-- owner lists (sign_in_local_networks). Installs that already had it on
-- get the ranges it trusted before (private, loopback and link-local), so
-- nothing changes for them until the owner narrows the list.

INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by)
SELECT grp, 'sign_in_local_networks',
       '["10.0.0.0/8","172.16.0.0/12","192.168.0.0/16","127.0.0.0/8","169.254.0.0/16","fc00::/7","::1/128","fe80::/10"]',
       created_at, created_by, updated_at, updated_by
FROM settings WHERE grp = 'this_app' AND key = 'skip_login_local_network' AND value = 'true'
ON CONFLICT (grp, key) DO NOTHING;
