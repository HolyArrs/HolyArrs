import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { describe, expect, it } from 'vitest';

// The world-list source's brand never shows in the app: no UI string, label or code path in src names it.
const brand = new RegExp(['spot', 'ify'].join(''), 'i');

function files(dir: string): string[] {
	return readdirSync(dir).flatMap((n) => {
		const p = join(dir, n);
		if (statSync(p).isDirectory()) return n === 'node_modules' || n.startsWith('.') ? [] : files(p);
		return /\.(svelte|ts|js|json|html|css)$/.test(n) && !n.endsWith('.test.ts') ? [p] : [];
	});
}

describe('list source brand', () => {
	it('appears in no UI file', () => {
		const hits = files(join(__dirname, '..', '..')).filter((f) => brand.test(readFileSync(f, 'utf8')));
		expect(hits).toEqual([]);
	});
});
