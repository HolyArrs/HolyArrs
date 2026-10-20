-- Asked by: each person may let the household see their name on their
-- asks (off by default). The owner, admins and managers always see it.
ALTER TABLE user_ask_settings ADD COLUMN show_my_name INTEGER NOT NULL DEFAULT 0;
