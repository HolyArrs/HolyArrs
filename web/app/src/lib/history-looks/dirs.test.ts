import { describe, expect, it } from 'vitest';
import { dateLabel, dirQuery, lookOf, readDir, readSearch, searchChips, stepKinds, storyOf, textMatches, titleCards, toggleStep, typeMatches, whoMatches } from './dirs.ts';
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

	it('maps steps to kinds and leaves routine work out', () => {
		expect(stepKinds(['asked', 'chosen'])).toEqual(['ask_made', 'release_chosen', 'release_chosen_by_hand']);
		expect(stepKinds(['failed'])).toContain('download_failed');
		const all = stepKinds([]);
		expect(all).toContain('import_done');
		expect(all).not.toContain('search_done');
		expect(all).not.toContain('library_scanned');
		expect(all.length).toBeLessThanOrEqual(50);
	});

	it('reads the search look and builds a chip per filter', () => {
		const s = readSearch(new URLSearchParams('look=pill&q=dune&step=failed,asked,bogus&type=movies&who=me&from=2026-10-26'));
		expect(s.steps).toEqual(['failed', 'asked']);
		expect(toggleStep(s.steps, 'failed')).toBe('asked');
		const chips = searchChips(s, new Date('2026-11-02T12:00:00Z'));
		expect(chips.map((c) => c.label)).toEqual(['Asked', 'Failed', 'Movies', 'You', 'Last 7 days']);
		expect(chips.find((c) => c.key === 'step.failed')?.change).toEqual({ step: 'asked' });
		expect(searchChips(readSearch(new URLSearchParams('q=x')))).toEqual([]);
		expect(dateLabel('2026-10-01', '2026-10-05')).toBe('2026-10-01 to 2026-10-05');
	});

	it('matches search text and type', () => {
		const e = ev('download_sent', { title_name: 'Dune', Data: { source_name: 'NZBgeek' }, Actor: { kind: 'user', id: 2 } });
		expect(textMatches(e, 'dun')).toBe(true);
		expect(textMatches(e, 'geek')).toBe(true);
		expect(textMatches(e, 'luis', new Map([[2, 'Luis']]))).toBe(true);
		expect(textMatches(e, 'alien')).toBe(false);
		expect(typeMatches('artist', 'music')).toBe(true);
		expect(typeMatches('show', 'movies')).toBe(false);
	});
});
