-- What MULTi means on a Source: the languages a release named MULTi carries
-- there (French and German trackers). Empty keeps the name's own languages.
ALTER TABLE sources ADD COLUMN multi_languages TEXT NOT NULL DEFAULT '[]' CHECK(json_valid(multi_languages));
