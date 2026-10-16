-- Stage 3.3.1: music Pick rules presets (pick-rules.md section 8). Each must
-- match its domain function (LosslessFirst, Lossy320, AnyMusic); a test checks
-- it. Artists whose Version uses movie and show Pick rules (the default before
-- music search) move to "Lossless first": those rules have no music steps, so
-- they could never pick an album.

INSERT INTO pick_rules (created_at, created_by, updated_at, updated_by, name, kinds, revision, preset, body)
SELECT strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}',
       'Lossless first', '["music"]', 1, 1,
       '{"steps":[{"quality":"lossy_low","allowed":false},{"quality":"lossy_mid","allowed":false},{"quality":"lossy_high","allowed":true},{"quality":"lossless","allowed":true},{"quality":"lossless_hires","allowed":true}],"good_enough_at":"lossless","traits":[{"trait":"cd_source","points":10},{"trait":"log_cue","points":5},{"trait":"web_source","points":5},{"trait":"vinyl_source","points":-10}],"must_have":[],"never":["sample","executable","password_protected","single_image","lossy_transcode"],"min_points":-10,"size_limits":{"lossless":{"min":3145728,"max":15728640,"per_minute":true},"lossless_hires":{"min":6291456,"max":83886080,"per_minute":true},"lossy_high":{"min":1258291,"max":3670016,"per_minute":true},"lossy_low":{"min":314572,"max":1572864,"per_minute":true},"lossy_mid":{"min":1048576,"max":2621440,"per_minute":true}},"protocol":{"prefer":"either","wait_minutes_torrent":0,"wait_minutes_usenet":0,"skip_wait_at_good_enough":false},"language":{"wanted":[],"original_ok":true},"order":"quality_then_points","upgrade_min_points":10,"proper_policy":{"mode":"take_any"}}'
WHERE NOT EXISTS (SELECT 1 FROM pick_rules WHERE name = 'Lossless first');

INSERT INTO pick_rules (created_at, created_by, updated_at, updated_by, name, kinds, revision, preset, body)
SELECT strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}',
       'Lossy 320 or V0', '["music"]', 1, 1,
       '{"steps":[{"quality":"lossy_low","allowed":false},{"quality":"lossy_mid","allowed":true},{"quality":"lossy_high","allowed":true},{"quality":"lossless","allowed":false},{"quality":"lossless_hires","allowed":false}],"good_enough_at":"lossy_high","traits":[{"trait":"mp3","points":5},{"trait":"web_source","points":5},{"trait":"cd_source","points":5}],"must_have":[],"never":["sample","executable","password_protected","single_image","lossy_transcode"],"min_points":-10,"size_limits":{"lossless":{"min":3145728,"max":15728640,"per_minute":true},"lossless_hires":{"min":6291456,"max":83886080,"per_minute":true},"lossy_high":{"min":1258291,"max":3670016,"per_minute":true},"lossy_low":{"min":314572,"max":1572864,"per_minute":true},"lossy_mid":{"min":1048576,"max":2621440,"per_minute":true}},"protocol":{"prefer":"either","wait_minutes_torrent":0,"wait_minutes_usenet":0,"skip_wait_at_good_enough":false},"language":{"wanted":[],"original_ok":true},"order":"quality_then_points","upgrade_min_points":10,"proper_policy":{"mode":"take_any"}}'
WHERE NOT EXISTS (SELECT 1 FROM pick_rules WHERE name = 'Lossy 320 or V0');

INSERT INTO pick_rules (created_at, created_by, updated_at, updated_by, name, kinds, revision, preset, body)
SELECT strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}', strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), '{"kind":"system"}',
       'Any music', '["music"]', 1, 1,
       '{"steps":[{"quality":"lossy_low","allowed":true},{"quality":"lossy_mid","allowed":true},{"quality":"lossy_high","allowed":true},{"quality":"lossless","allowed":true},{"quality":"lossless_hires","allowed":true}],"good_enough_at":"lossless","traits":[{"trait":"cd_source","points":10},{"trait":"log_cue","points":5},{"trait":"web_source","points":5},{"trait":"vinyl_source","points":-10}],"must_have":[],"never":["sample","executable","password_protected","single_image","lossy_transcode"],"min_points":-10,"size_limits":{"lossless":{"min":3145728,"max":15728640,"per_minute":true},"lossless_hires":{"min":6291456,"max":83886080,"per_minute":true},"lossy_high":{"min":1258291,"max":3670016,"per_minute":true},"lossy_low":{"min":314572,"max":1572864,"per_minute":true},"lossy_mid":{"min":1048576,"max":2621440,"per_minute":true}},"protocol":{"prefer":"either","wait_minutes_torrent":0,"wait_minutes_usenet":0,"skip_wait_at_good_enough":false},"language":{"wanted":[],"original_ok":true},"order":"quality_then_points","upgrade_min_points":10,"proper_policy":{"mode":"take_any"}}'
WHERE NOT EXISTS (SELECT 1 FROM pick_rules WHERE name = 'Any music');

-- Only the music rules of that name: a person's own movie rules called
-- "Lossless first" are never given to an artist (the artist then keeps its
-- rules).
UPDATE versions SET pick_rules_id = (SELECT id FROM pick_rules WHERE name = 'Lossless first' AND kinds LIKE '%"music"%')
WHERE title_id IN (SELECT id FROM titles WHERE kind = 'artist')
  AND pick_rules_id NOT IN (SELECT id FROM pick_rules WHERE kinds LIKE '%"music"%')
  AND EXISTS (SELECT 1 FROM pick_rules WHERE name = 'Lossless first' AND kinds LIKE '%"music"%');
