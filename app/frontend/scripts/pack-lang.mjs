// Packs each non-English language into one small file: only its lines, in the
// order of the sorted English keys (the keys themselves ship once, with English).
// Writes src/lib/core/i18n/packs/<lang>.json plus fit.json; Vite gives each pack a
// hashed /_app/immutable/ address, so browsers cache it for good and the server
// gzips it on the way out. A line a language lacks falls back to English.
import { readdirSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { createHash } from 'node:crypto';

const root = 'src/lib/core/i18n/messages';
const out = 'src/lib/core/i18n/packs';

const load = (l) =>
	Object.assign({}, ...readdirSync(join(root, l)).filter((f) => f.endsWith('.json')).sort().map((f) => JSON.parse(readFileSync(join(root, l, f), 'utf8'))));
const keys = Object.keys(load('en')).sort();
// A pack only fits the key list it was built from.
const fit = createHash('sha256').update(JSON.stringify(keys)).digest('hex').slice(0, 12);
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });
let n = 0;
for (const l of readdirSync(root).filter((d) => d !== 'en').sort()) {
	const w = load(l);
	let missing = 0;
	const values = keys.map((k) => {
		if (typeof w[k] === 'string') return w[k];
		missing++;
		return null;
	});
	if (missing) console.warn(`pack-lang: ${l} has ${missing} lines missing; they show in English`);
	writeFileSync(join(out, `${l}.json`), JSON.stringify({ fit, values }));
	n++;
}
writeFileSync(join(out, 'fit.json'), JSON.stringify({ fit }));
console.log(`pack-lang: ${n} languages packed`);
