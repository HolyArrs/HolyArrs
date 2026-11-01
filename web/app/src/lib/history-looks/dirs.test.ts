import { describe, expect, it } from 'vitest';
import { dirQuery, lookOf, readDir, storyOf, titleCards, whoMatches } from './dirs.ts';
import type { EventRecord } from '../types.ts';

let id = 0;
const ev = (Kind: string, extra: Partial<EventRecord> = {}): EventRecord => ({ ID: ++id, At: '2026-11-01T10:00:00Z', Kind, Summary: Kind, TitleID: null, ...extra });

describe('history preview looks', () => {
	it('reads the look, falling back to today', () => {
		expect(lookOf('table')).toBe('table');
		expect(lookOf('nope')).toBe('now');
		expect(lookOf(null)).toBe('now');
	});

	it('keeps ?look= and drops empty values when the address changes', () => {
		const q = new URLSearchParams('look=feed&title=4&offset=50');
		expect(dirQuery(q, { title: undefined, who: 'me' })).toBe('?look=feed&who=me');
		expect(readDir(new URLSearchParams('show=problems.imports&who=people')).who).toBe('people');
		expect(readDir(new URLSearchParams('show=problems.imports')).sub).toBe('imports');
	});

	it('tells who made an Event happen', () => {
		const mine = ev('ask_made', { Actor: { kind: 'user', id: 2 } });
		expect(whoMatches(mine, 'me', 2)).toBe(true);
		expect(whoMatches(mine, 'me', 3)).toBe(false);
		expect(whoMatches(mine, 'holyarrs')).toBe(false);
		expect(whoMatches(ev('search_done', { Actor: { kind: 'job' } }), 'holyarrs')).toBe(true);
	});

	it('lays out a story; a newer success clears an older failure', () => {
		const s = storyOf([ev('import_done'), ev('import_failed'), ev('download_failed'), ev('ask_made')]);
		const by = Object.fromEntries(s.map((x) => [x.key, x.state]));
		expect(by.imported).toBe('done');
		expect(by.downloaded).toBe('failed');
		expect(by.requested).toBe('done');
		expect(by.upgraded).toBe('waiting');
	});

	it('folds Events into one card per title, newest first', () => {
		const cards = titleCards([ev('import_done', { TitleID: 1, title_name: 'Dune' }), ev('download_sent', { TitleID: 1 }), ev('import_failed', { TitleID: 2, title_name: 'Alien' }), ev('health_broken')]);
		expect(cards.map((c) => [c.name, c.count, c.problem])).toEqual([['Dune', 2, false], ['Alien', 1, true]]);
	});
});
