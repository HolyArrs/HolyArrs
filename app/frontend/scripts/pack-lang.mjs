// Packs each non-English language into one small file: only its lines, in the
// order of the sorted English keys (the keys themselves ship once, with English).
// Writes src/lib/core/i18n/packs/<lang>.json plus fit.json; Vite gives each pack a
// hashed /_app/immutable/ address, so browsers cache it for good and the server
// gzips it on the way out. A line a language lacks falls back to English; a line
// whose {slots} or <markers> differ from English fails the build (as check-lang.mjs).
import { readdirSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { createHash } from 'node:crypto';

const root = 'src/lib/core/i18n/messages';
const out = 'src/lib/core/i18n/packs';

const load = (l) =>
	Object.assign({}, ...readdirSync(join(root, l)).filter((f) => f.endsWith('.json')).sort().map((f) => JSON.parse(readFileSync(join(root, l, f), 'utf8'))));
const en = load('en');
const keys = Object.keys(en).sort();
const marks = (s) => [...s.matchAll(/\{\w+\}|<\/?\w+>/g)].map((m) => m[0]).sort().join(' ');
const wrong = [];
// A pack only fits the key list it was built from.
const fit = createHash('sha256').update(JSON.stringify(keys)).digest('hex').slice(0, 12);
rmSync(out, { recursive: true, force: true });
mkdirSync(out, { recursive: true });
let n = 0;
for (const l of readdirSync(root).filter((d) => d !== 'en').sort()) {
	const w = load(l);
	let missing = 0;
	const values = keys.map((k) => {
		if (typeof w[k] === 'string') {
			if (marks(w[k]) !== marks(en[k])) wrong.push(`${l} ${k}: ${marks(en[k])} vs ${marks(w[k])}`);
			return w[k];
		}
		missing++;
		return null;
	});
	if (missing) console.warn(`pack-lang: ${l} has ${missing} lines missing; they show in English`);
	writeFileSync(join(out, `${l}.json`), JSON.stringify({ fit, values }));
	n++;
}
if (wrong.length) {
	console.error(`pack-lang: ${wrong.length} lines have other slots or markers than English:`);
	for (const x of wrong) console.error(`  ${x}`);
	process.exit(1);
}
writeFileSync(join(out, 'fit.json'), JSON.stringify({ fit }));
console.log(`pack-lang: ${n} languages packed`);
