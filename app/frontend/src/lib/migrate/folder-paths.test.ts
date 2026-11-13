import { afterEach, describe, expect, it } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { render } from 'svelte/server';
import { lang } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';
import FolderOutcome from './FolderOutcome.svelte';
import type { Line } from './move.ts';

const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
});

const arWords = () => Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/ar/*.json', { eager: true })).map((f) => f.default));

const LRI = '⁦';
const PDI = '⁩';

describe('paths in the import from other apps', () => {
	it('keep their slashes in place inside an Arabic server line', () => {
		lang.value = 'ar';
		lang.words = arWords();
		const line = serverWords('left out: /data/media/tv is set to "Don\'t bring them"; pick a folder for it and import again');
		expect(line).toContain(`${LRI}/data/media/tv${PDI}`);
		expect(line).not.toContain('left out');
	});

	it('stay plain in a left-to-right language', () => {
		lang.value = 'en';
		const line = 'left out: /data/media/tv is set to "Don\'t bring them"; pick a folder for it and import again';
		expect(serverWords(line)).toBe(line);
	});

	it('are kept left to right on the Done screen in Arabic', () => {
		lang.value = 'ar';
		lang.words = arWords();
		const lines: Line[] = [
			{ group: 'titles', key: 'a', name: 'A', result: 'skipped', attention: 'unchecked', folder: '/data/media/tv' },
			{ group: 'titles', key: 'b', name: 'B', result: 'created', attention: 'unreachable', folder: '/data/media/mediamanager/movies' }
		];
		const { body } = render(FolderOutcome, { props: { lines, after: true } });
		expect(body).toContain(`${LRI}/data/media/tv${PDI}`);
		expect(body).toContain(`${LRI}/data/media/mediamanager/movies${PDI}`);
	});

	it('names the real control when a folder was left out', () => {
		const lines: Line[] = [{ group: 'titles', key: 'a', name: 'A', result: 'skipped', attention: 'unchecked', folder: '/data/media/tv' }];
		const { body } = render(FolderOutcome, { props: { lines, after: true } });
		expect(body).toContain('/data/media/tv is set to "Don\'t bring them"; pick a folder for it and import again');
		expect(body).not.toContain('left unchecked');
	});

	// Svelte drops the space at the start of an {#if} block, so a sentence
	// joined that way runs into the one before it ("volume.It can see").
	it('never joins two sentences with a space at the start of a block', () => {
		const dir = fileURLToPath(new URL('.', import.meta.url));
		for (const f of readdirSync(dir).filter((n) => n.endsWith('.svelte'))) {
			expect(readFileSync(dir + f, 'utf8'), f).not.toMatch(/\{(#if|:else)[^}]*\} \{t\(/);
		}
	});
});
