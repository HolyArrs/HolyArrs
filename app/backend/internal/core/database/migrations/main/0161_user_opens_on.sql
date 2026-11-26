-- The page the app opens on for each person: home (as before) or explore.
ALTER TABLE user_profiles ADD COLUMN opens_on TEXT NOT NULL DEFAULT 'home' CHECK (opens_on IN ('home', 'explore'));
