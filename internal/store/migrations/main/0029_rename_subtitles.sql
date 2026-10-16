-- Stage 4.16: subtitle files beside a video follow it when it is renamed.
-- A rename step is now either the video itself or one subtitle beside it;
-- both are written before the disk changes, so a cut-off rename finishes.
ALTER TABLE rename_steps ADD COLUMN part TEXT NOT NULL DEFAULT 'video' CHECK (part IN ('video', 'subtitle'));
