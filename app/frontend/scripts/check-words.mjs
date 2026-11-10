// Fails the build when the app's source or built files use an upstream
// coined word on the banned list.
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, extname } from 'node:path';

const banned = [
	'Monitored', 'Activity', 'Wanted', 'Cutoff', 'Quality Profile', 'Root Folder', 'Download Client',
	'Interactive Search', 'Manual Import', 'Grabbed', 'Connections', 'Discover', 'Series'
];
const roots = ['src', 'dist'];
const exts = new Set(['.svelte', '.ts', '.js', '.html', '.css']);
let bad = 0;

// The list is English words from other apps. A translated line may use the same letters as an ordinary word of its
// language (Spanish "Series"), so translated lines, quoted as they sit in the built files, are taken out before the check.
const messages = 'src/lib/core/i18n/messages';
const translated = [];
for (const lang of readdirSync(messages)) {
	if (lang === 'en') continue;
	for (const f of readdirSync(join(messages, lang)).filter((f) => f.endsWith('.json'))) {
		for (const v of Object.values(JSON.parse(readFileSync(join(messages, lang, f), 'utf8')))) {
			if (banned.some((w) => v.includes(w))) translated.push(JSON.stringify(v), `'${v}'`, '`' + v + '`');
		}
	}
}
translated.sort((a, b) => b.length - a.length);

function walk(dir) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		// vendor: third-party code (hls.js), not the app's words.
		if (statSync(p).isDirectory()) { if (name !== 'vendor') walk(p); }
		else if (exts.has(extname(p)) && !p.endsWith('.test.ts')) {
			let text = readFileSync(p, 'utf8');
			for (const v of translated) text = text.split(v).join('');
			for (const w of banned) {
				if (text.includes(w)) {
					console.error(`${p}: uses "${w}"`);
					bad++;
				}
			}
		}
	}
}

for (const r of roots) walk(r);
if (bad) {
	console.error(`${bad} banned word(s) found; see glossary.md`);
	process.exit(1);
}
console.log('word check: clean');
