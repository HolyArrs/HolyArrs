// Lines the released app already showed in German read the same now: the list lines of a folder kept to tagged Sources have rules of their own and change nothing else.
import { lang } from './i18n.svelte.ts';
import { serverWords } from './server-words.ts';
import de from './messages/de/server-lines.json';
import en from './messages/en/server-lines.json';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';

describe('lines from before, in German', () => {
	const was = { value: lang.value, words: lang.words };
	beforeEach(() => {
		lang.value = 'de';
		lang.words = { ...(en as Record<string, string>), ...(de as Record<string, string>) };
	});
	afterEach(() => {
		lang.value = was.value;
		lang.words = was.words;
	});
	it('still put a known word into a gap called name', () => {
		expect(serverWords('in Germany')).toBe('in Deutschland');
		expect(serverWords('you left library folders out')).toBe('du hast Bibliotheksordner ausgelassen');
		expect(serverWords('the app has no exclusions list; skipped')).toBe('die App hat keine Ausschlüsse-Liste; übersprungen');
		expect(serverWords('Charting in Comedy and not in your Library')).toContain('Komödie');
	});
	it('still go to the line with the most words of its own', () => {
		expect(serverWords('Backup Plan: turned down by Maria')).toBe('Backup Plan: von Maria verworfen');
		expect(serverWords('Added Sugar asked for Dune (2021)')).toBe('Sugar hinzugefügt hat Dune (2021) angefragt');
		expect(serverWords('Download Station is paused')).toBe('Download: Station is paused');
	});
	it('keep a tag as it was typed in the new list lines, also when it is a word the app knows', () => {
		const line = serverWords("no Source takes this title's tags: Movies: its tags (movies, music) are not in the Pick rules' tags (music)");
		expect(line, line).toContain('Movies: ');
		expect(line, line).toContain('(movies, music)');
		expect(line, line).toContain('(music)');
		expect(serverWords('This folder only takes Sources tagged movies. None can be searched right now (Music is paused), so nothing was asked.')).toMatch(/mit den Tags movies\. .*Music ist pausiert/);
	});
});
