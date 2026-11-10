// Checks one language's message files against English: every key present, same {placeholders} and <markers>.
import { readdirSync, readFileSync, existsSync } from 'node:fs';
const lang = process.argv[2];
const dir = 'src/lib/core/i18n/messages';
const marks = (s) => [...s.matchAll(/\{\w+\}|<\/?\w+>/g)].map((m) => m[0]).sort().join(' ');
let missing = 0, wrong = 0, extra = 0;
for (const f of readdirSync(`${dir}/en`).filter((f) => f.endsWith('.json'))) {
	const en = JSON.parse(readFileSync(`${dir}/en/${f}`, 'utf8'));
	const p = `${dir}/${lang}/${f}`;
	const tr = existsSync(p) ? JSON.parse(readFileSync(p, 'utf8')) : {};
	for (const [k, v] of Object.entries(en)) {
		if (!(k in tr)) { missing++; continue; }
		if (marks(v) !== marks(tr[k])) { wrong++; console.log(`${f} ${k}: ${marks(v)} vs ${marks(tr[k])}`); }
	}
	for (const k of Object.keys(tr)) if (!(k in en)) { extra++; console.log(`${f} extra ${k}`); }
}
console.log(`${lang}: missing ${missing}, marker mismatch ${wrong}, extra ${extra}`);
process.exit(missing || wrong || extra ? 1 : 0);
