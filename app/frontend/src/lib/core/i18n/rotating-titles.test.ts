import { afterEach, describe, expect, it } from 'vitest';
import en from './messages/en/server-lines.json';
import { lang } from './i18n.svelte.ts';
import { serverWords } from './server-words.ts';

const start = { value: lang.value, words: lang.words };
afterEach(() => {
	lang.value = start.value;
	lang.words = start.words;
});

const files = {
	de: import.meta.glob<{ default: Record<string, string> }>('./messages/de/*.json', { eager: true }),
	fr: import.meta.glob<{ default: Record<string, string> }>('./messages/fr/*.json', { eager: true })
};
function speak(code: keyof typeof files) {
	lang.value = code;
	lang.words = { ...(en as Record<string, string>), ...Object.assign({}, ...Object.values(files[code]).map((f) => f.default)) };
}

describe('rotating row titles', () => {
	it('shows the rotating Home and Explore titles in the picked language, gaps filled', () => {
		speak('de');
		expect(serverWords('Proper horror')).toBe('Echter Horror');
		expect(serverWords('Back to the 1980s')).toBe('Zurück in die 1980er');
		expect(serverWords('Picked from Dune')).toBe('Ausgewählt nach Dune');
		const monday = new Intl.DateTimeFormat('de', { weekday: 'long' }).format(new Date(2023, 0, 2));
		expect(serverWords('A slow Monday')).toBe(`Ein ruhiger ${monday}`);
		speak('fr');
		expect(serverWords('Comedy picks for you')).not.toBe('Comedy picks for you');
		expect(serverWords('Friday night horror')).toBe('Horreur du vendredi soir');
	});

	it('shows the music row titles in the picked language, a country in its own name', () => {
		speak('fr');
		const germany = new Intl.DisplayNames('fr', { type: 'region' }).of('DE');
		expect(serverWords('Top in Germany')).toBe(`Top en ${germany}`);
		expect(serverWords('Top worldwide')).toBe('Top mondial');
		expect(serverWords('Charting in Japan')).toBe(`Dans les classements en ${new Intl.DisplayNames('fr', { type: 'region' }).of('JP')}`);
		expect(serverWords('K-pop right now')).toBe('K-pop en ce moment');
		const fr = new Intl.DisplayNames('fr', { type: 'region' });
		for (const [en, code] of [['the US', 'US'], ['the UK', 'GB'], ['the Netherlands', 'NL'], ['Turkey', 'TR']]) expect(serverWords(`Top in ${en}`)).toBe(`Top en ${fr.of(code)}`);
	});

	it('leaves a title English when a gap holds something it does not know', () => {
		speak('de');
		for (const line of ['Weekend xyzzy', 'Top in Atlantis', 'Picked from', 'The server is down: try again right now']) expect(serverWords(line)).toBe(line);
	});
});
