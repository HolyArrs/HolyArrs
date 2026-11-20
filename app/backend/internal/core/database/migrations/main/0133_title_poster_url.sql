-- The address the poster was fetched from, beside the stored copy in
-- artwork. The old-app API sends it as the poster's remoteUrl, which phone
-- apps load directly. Filled on the next poster fetch; until then empty.
ALTER TABLE titles ADD COLUMN poster_url TEXT NOT NULL DEFAULT '';
