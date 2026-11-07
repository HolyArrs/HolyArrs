// Writes a brotli copy (name.br) next to each built script, style, page and
// SVG in app/frontend/dist, kept only when it is smaller. The Go server sends it to
// browsers that accept brotli; others get the file as it is, gzipped.
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs';
import { join, extname } from 'node:path';
import { brotliCompressSync, constants } from 'node:zlib';

const dist = new URL('../dist/', import.meta.url).pathname.replace(/^\/([A-Za-z]:)/, '$1');
const exts = new Set(['.js', '.css', '.html', '.svg', '.json', '.txt']);
let n = 0, before = 0, after = 0;
function walk(dir) {
	for (const name of readdirSync(dir)) {
		const p = join(dir, name);
		if (statSync(p).isDirectory()) { walk(p); continue; }
		if (!exts.has(extname(p))) continue;
		const b = readFileSync(p);
		if (b.length < 1024) continue;
		const z = brotliCompressSync(b, { params: { [constants.BROTLI_PARAM_QUALITY]: 11, [constants.BROTLI_PARAM_SIZE_HINT]: b.length } });
		if (z.length >= b.length * 0.9) continue;
		writeFileSync(p + '.br', z);
		n++; before += b.length; after += z.length;
	}
}
walk(dist);
console.log(`brotli: ${n} files, ${Math.round(before / 1024)} KB -> ${Math.round(after / 1024)} KB`);
