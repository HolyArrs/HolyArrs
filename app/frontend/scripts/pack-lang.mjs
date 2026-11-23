// Packs each non-English language into one small file: only its lines, in the
// order of the sorted English keys (the keys themselves ship once, with English).
// Writes lang-out/<lang>.json ({lang, app, fit, values}), outside src so no page
// build takes it in: the app ships English only and installs the others as
// signed language packs (deploy/pack_languages.py packs these for the plugin
// list). fit.json goes to src/lib/core/i18n/packs/, so the app knows which packs
// fit it. A line a language lacks falls back to English; a line whose {slots}
// or <markers> differ from English fails the build (as check-lang.mjs).
//   node scripts/pack-lang.mjs [--out <dir>] [--app <version>]
import { existsSync, readdirSync, readFileSync, writeFileSync, mkdirSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { createHash } from 'node:crypto';

const root = 'src/lib/core/i18n/messages';
const arg = (name) => {
	const i = process.argv.indexOf(name);
	return i > 0 ? process.argv[i + 1] : undefined;
};
const out = arg('--out') ?? 'lang-out';
const fitDir = 'src/lib/core/i18n/packs';
// The app version the packs are for: the repo's VERSION file unless given.
const app = arg('--app') ?? (existsSync('../../VERSION') ? readFileSync('../../VERSION', 'utf8').trim() : '0.0.0-dev');

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
rmSync(fitDir, { recursive: true, force: true });
mkdirSync(fitDir, { recursive: true });
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
	writeFileSync(join(out, `${l}.json`), JSON.stringify({ lang: l, app, fit, values }));
	n++;
}
if (wrong.length) {
	console.error(`pack-lang: ${wrong.length} lines have other slots or markers than English:`);
	for (const x of wrong) console.error(`  ${x}`);
	process.exit(1);
}
writeFileSync(join(fitDir, 'fit.json'), JSON.stringify({ fit }));
console.log(`pack-lang: ${n} languages packed for ${app} in ${out}`);
