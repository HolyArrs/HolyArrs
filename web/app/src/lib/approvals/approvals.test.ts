import { describe, expect, it } from 'vitest';
import { approvalViews, askHref, boardColumn, detailKey, kindWord, personGroups, runtimeLine, stepIndex } from './approvals.ts';

const who = (id: number, name: string) => ({ id, name });

describe('approvals looks', () => {
	it('has Now plus five numbered layouts', () => {
		expect(approvalViews.map((v) => v.label)).toEqual(['Now', '1 Cards', '2 By person', '3 Focus', '4 Inbox', '5 Board']);
	});

	it('links each ask to its Library page, title page or artist page', () => {
		expect(askHref({ kind: 'movie', tmdb_id: '496243', title_id: 12 })).toBe('/library/12');
		expect(askHref({ kind: 'movie', tmdb_id: '496243' })).toBe('/title/movie/496243');
		expect(askHref({ kind: 'show', tmdb_id: '119051' })).toBe('/title/show/119051');
		expect(askHref({ kind: 'artist', tmdb_id: '', ref: 'mbid:63aa26c3' })).toBe('/music/artist/63aa26c3');
		expect(askHref({ kind: 'artist', tmdb_id: '' })).toBe('');
		// A pending approval's placeholder id is not a Library page.
		expect(askHref({ kind: 'movie', tmdb_id: '1', title_id: -1 })).toBe('/title/movie/1');
	});

	it('groups asks by the first person who asked, biggest first', () => {
		const maya = who(3, 'Maya');
		const leo = who(4, 'Leo (Kid)');
		const asks = [
			{ id: 1, asked_by: leo, people: [{ ...leo, asked_at: '' }] },
			{ id: 2, asked_by: maya, people: [{ ...maya, asked_at: '' }] },
			{ id: 3, asked_by: maya, people: [] }
		];
		const g = personGroups(asks);
		expect(g.map((x) => [x.name, x.asks.length])).toEqual([
			['Maya', 2],
			['Leo (Kid)', 1]
		]);
	});

	it('puts each ask in a board column', () => {
		const dl = new Set([7]);
		expect(boardColumn({ state: 'asked' }, dl)).toBe('waiting');
		expect(boardColumn({ state: 'approved', title_id: 7 }, dl)).toBe('downloading');
		expect(boardColumn({ state: 'approved', title_id: 8 }, dl)).toBe('looking');
		expect(boardColumn({ state: 'approved' }, dl)).toBe('looking');
		expect(boardColumn({ state: 'available', title_id: 7 }, dl)).toBe('done');
		expect(boardColumn({ state: 'turned_down' }, dl)).toBe('turned_down');
	});

	it('wraps the focus position', () => {
		expect(stepIndex(0, -1, 8)).toBe(7);
		expect(stepIndex(7, 1, 8)).toBe(0);
		expect(stepIndex(3, 1, 0)).toBe(0);
	});

	it('says small words', () => {
		expect(kindWord('show')).toBe('Show');
		expect(kindWord('artist')).toBe('Music');
		expect(kindWord('movie')).toBe('Movie');
		expect(runtimeLine(112)).toBe('1 h 52 min');
		expect(runtimeLine(120)).toBe('2 h');
		expect(runtimeLine(48)).toBe('48 min');
		expect(runtimeLine(undefined)).toBe('');
		expect(detailKey({ kind: 'artist', tmdb_id: '' })).toBe('');
		expect(detailKey({ kind: 'show', tmdb_id: '5' })).toBe('show:5');
	});
});
