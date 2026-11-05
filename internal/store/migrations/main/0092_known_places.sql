-- Places a name finished a sign-in from (an address, or an IPv6 /64), so
-- strangers holding a name's wait cannot keep its owner out of home after a
-- restart. user_key and ip_key are the sign-in limit keys ("name:luis",
-- "ip:203.0.113.50"). Rows older than 30 days are skipped when read at
-- start and dropped when that name next signs in.
CREATE TABLE known_places (
    user_key TEXT NOT NULL,
    ip_key   TEXT NOT NULL,
    last_at  TEXT NOT NULL,
    PRIMARY KEY (user_key, ip_key)
);
