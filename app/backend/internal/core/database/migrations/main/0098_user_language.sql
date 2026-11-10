-- Each person's language for the app's words. Empty means follow the browser.
ALTER TABLE user_profiles ADD COLUMN language TEXT NOT NULL DEFAULT '';
