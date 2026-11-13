// Replaces each built script, style, language file and SVG in app/frontend/dist
// with a brotli copy (name.br) when that is smaller, so the program that embeds
// dist stays small. The Go server sends the copy as it is to browsers that
// accept brotli and unpacks it for the others. index.html stays plain: the
// server rewrites it on every page load.
import { readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { join, extname } from 'node:path';
import { brotliCompressSync, constants } from 'node:zlib';

const dist = new URL('../dist/', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
const exts = new Set(['.js', '.css', '.html', '.svg', '.json', '.txt']);
let n = 0, before = 0, after = 0;
function walk(dir) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) { walk(p); continue; }
		if (!exts.has(extname(p)) || name === 'index.html') continue;
		const b = readFileSync(p);
		if (b.length < 1024) continue;
		// The image build packs the language files itself, with bzip2 (deploy/pack_languages.py).
		if (process.env.HOLYARRS_LANGUAGES_PLAIN && extname(p) === '.json' && b.subarray(0, 8).toString() === '{"fit":"') continue;
		const z = brotliCompressSync(b, { params: { [constants.BROTLI_PARAM_QUALITY]: 11, [constants.BROTLI_PARAM_SIZE_HINT]: b.length } });
		if (z.length >= b.length * 0.9) continue;
		writeFileSync(p + '.br', z);
		rmSync(p);
		n++; before += b.length; after += z.length;
	}
}
walk(dist);
console.log(`brotli: ${n} files, ${Math.round(before / 1024)} KB -> ${Math.round(after / 1024)} KB`);
