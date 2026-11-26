-- The ready-made 4K and Best quality sets mark down SDR 4K copies, as
-- mediafiles.FourK() and Best() do (#904); a test checks they match. Only a
-- set whose points list still matches the shipped one changes, so edits stay.
UPDATE pick_rules
SET body = json_insert(body, '$.traits[#]', json('{"trait":"sdr_4k","points":-100}'))
WHERE preset = 1 AND name IN ('4K', 'Best')
  AND json_extract(body, '$.traits') = json('[{"trait":"dv","points":20},{"trait":"hdr10plus","points":15},{"trait":"hdr10","points":10},{"trait":"atmos","points":10},{"trait":"hevc","points":5},{"trait":"upscaled","points":-100}]');
