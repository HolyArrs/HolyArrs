import { afterEach, describe, expect, it } from 'vitest';
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

	it('leaves unknown text and English alone', () => {
		speak({});
		expect(serverWords('Something only this server says 42')).toBe('Something only this server says 42');
		expect(serverWords('')).toBe('');
		lang.value = 'en';
		expect(serverWords('Waiting for approval')).toBe('Waiting for approval');
	});
});
