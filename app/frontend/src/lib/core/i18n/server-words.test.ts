import { afterEach, describe, expect, it } from 'vitest';
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import en from './messages/en/server-lines.json';
import { lang } from './i18n.svelte.ts';
import { serverWords } from './server-words.ts';

const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
});

function speak(words: Record<string, string>) {
	lang.value = 'fr';
	lang.words = { ...(en as Record<string, string>), ...words };
}

describe('serverWords', () => {
	it('knows every card line and badge the server sends', async () => {
		const it_ = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/it/*.json', { eager: true })).map((f) => f.default));
		speak(it_);
		for (const [line, want] of [['New movie', 'Nuovo film'], ['New show', 'Nuova serie'], ['New episode Mon', 'Nuovo episodio · lun'], ['Coming Friday', 'Arriva venerdì'], ['New season', 'Nuova stagione']]) expect(serverWords(line)).toBe(want);
	});

	it('shows a known line in the picked language', () => {
		speak({ 'server.requests.waiting-for-approval': 'En attente de validation' });
		expect(serverWords('Waiting for approval')).toBe('En attente de validation');
	});

	it('fills a line with slots back in', () => {
		speak({ 'server.discover.s-new-episodes': 'S{season} · {fresh} nouveaux épisodes' });
		expect(serverWords('S5 · 3 new episodes')).toBe('S5 · 3 nouveaux épisodes');
	});

	it('passes slot values that are known lines through too', () => {
		speak({ 'server.requests.problem-fixed-by': '{label} : problème {kindlabel} réglé par {name}', 'server.requests.sound': 'Son' });
		expect(serverWords('Dune: Sound problem fixed by Ana')).toBe('Dune : problème Son réglé par Ana');
	});

	it('shows the newer server lines in the picked language', () => {
		const fr = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('./messages/fr/*.json', { eager: true })).map((f) => f.default));
		speak(fr);
		expect(serverWords('TMDB could not be read: timeout')).toBe("TMDB n'a pas pu être lu : timeout");
		expect(serverWords('Ana unlinked the trusted proxy sign-in of Ben')).toBe('Ana a délié la connexion trusted proxy de Ben');
		expect(serverWords('Download 1a2b3c4d: completed')).toBe('Téléchargement 1a2b3c4d : completed');
		expect(serverWords('uses {series_title}, which has no HolyArrs word')).toBe("utilise {series_title}, qui n'a pas de mot dans HolyArrs");
	});

	it('has every line the server sends (scripts/server-words.mjs --check)', () => {
		const web = fileURLToPath(new URL('../../../../', import.meta.url));
		const file = fileURLToPath(new URL('./messages/en/server-lines.json', import.meta.url));
		const before = readFileSync(file, 'utf8');
		let out = '';
		try {
			out = execFileSync(process.execPath, ['scripts/server-words.mjs', '--check'], { cwd: web, encoding: 'utf8' });
		} catch (e) {
			out = String((e as { stdout?: string }).stdout ?? e);
		}
		expect(readFileSync(file, 'utf8')).toBe(before);
		expect(out).toContain('server words: every line is in the file');
	}, 60_000);

	it('leaves unknown text and English alone', () => {
		speak({});
		expect(serverWords('Something only this server says 42')).toBe('Something only this server says 42');
		expect(serverWords('')).toBe('');
		lang.value = 'en';
		expect(serverWords('Waiting for approval')).toBe('Waiting for approval');
	});
});
