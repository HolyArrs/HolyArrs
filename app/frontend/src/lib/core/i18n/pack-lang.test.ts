import { afterEach, describe, expect, it } from 'vitest';
import { spawnSync } from 'node:child_process';
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(new URL('../../../../scripts/pack-lang.mjs', import.meta.url));
const dirs: string[] = [];
afterEach(() => {
	for (const d of dirs.splice(0)) rmSync(d, { recursive: true, force: true });
});

// Packs a tiny copy of the message folders (English plus one language) and returns how it went.
function pack(fr: Record<string, string>) {
	const dir = mkdtempSync(join(tmpdir(), 'pack-lang-'));
	dirs.push(dir);
	const messages = join(dir, 'src/lib/core/i18n/messages');
	const write = (l: string, words: Record<string, string>) => {
		mkdirSync(join(messages, l), { recursive: true });
		writeFileSync(join(messages, l, 'a.json'), JSON.stringify(words));
	};
	write('en', { 'a.hello': 'Hello {name}', 'a.bold': 'Say <b>yes</b>', 'a.left': 'Left out' });
	write('fr', fr);
	const r = spawnSync(process.execPath, [script], { cwd: dir, encoding: 'utf8' });
	return { status: r.status, text: `${r.stdout}${r.stderr}` };
}

describe('pack-lang', () => {
	it('packs a language whose lines keep the English slots and markers', () => {
		const r = pack({ 'a.hello': 'Bonjour {name}', 'a.bold': 'Dites <b>oui</b>' });
		expect(r.status).toBe(0);
		expect(r.text).toContain('1 languages packed');
	});

	it('fails the build when a line loses or renames a slot', () => {
		const r = pack({ 'a.hello': 'Bonjour {nom}', 'a.bold': 'Dites <b>oui</b>' });
		expect(r.status).toBe(1);
		expect(r.text).toContain('fr a.hello');
	});

	it('fails the build when a line drops a marker', () => {
		const r = pack({ 'a.hello': 'Bonjour {name}', 'a.bold': 'Dites oui' });
		expect(r.status).toBe(1);
		expect(r.text).toContain('fr a.bold');
	});
});
