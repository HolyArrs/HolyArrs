import { describe, expect, it } from 'vitest';
import { approverCounts, approverSort, approverTab, askTab, askTabs, askTone, askedBy, decided, firstTab, firstView, tabCounts, limitLine, limitLines, names, viewCounts, waitWords } from './wishlist.ts';
import type { AskKindLimit, AskLimit, AskState } from './types.ts';

describe('wishlist words', () => {
	it('joins names', () => {
		expect(names([])).toBe('');
		expect(names(['ana'])).toBe('ana');
		expect(names(['ana', 'bo'])).toBe('ana and bo');
		expect(names(['ana', 'bo', 'cy'])).toBe('ana, bo and cy');
	});

	it('says who asked and who decided', () => {
		const who = (id: number, name: string) => ({ id, name, asked_at: '' });
		expect(askedBy({ asked_by: { id: 2, name: 'ana' }, people: [who(2, 'ana'), who(3, 'bo')] })).toBe('Asked by ana and bo');
		expect(decided({ state: 'asked', auto_approved: false })).toBe('');
		expect(decided({ state: 'approved', decided_by: { id: 1, name: 'luis' }, auto_approved: false })).toBe('Approved by luis');
		expect(decided({ state: 'available', decided_by: { id: 2, name: 'ana' }, auto_approved: true })).toBe('Auto-approved');
		expect(decided({ state: 'turned_down', decided_by: { id: 1, name: 'luis' }, auto_approved: false })).toBe('Declined by luis');
		expect(decided({ state: 'turned_down', decided_by: { id: 1, name: 'luis' }, auto_approved: false }, { id: 1, displayName: 'Luis R' })).toBe('Declined by Luis R');
		expect(decided({ state: 'turned_down', decided_by: { id: 1, name: 'luis' }, auto_approved: false }, { id: 2, displayName: 'Ana' })).toBe('Declined by luis');
	});

	it('colours states', () => {
		expect(askTone('asked')).toBe('wait');
		expect(askTone('available')).toBe('ok');
		expect(askTone('turned_down')).toBe('fail');
	});

	it('says waits like the server', () => {
		const min = 60_000;
		expect(waitWords(20_000)).toBe('a minute');
		expect(waitWords(40 * min)).toBe('40 minutes');
		expect(waitWords(90 * min)).toBe('1 hour and 30 minutes');
		expect(waitWords(26 * 60 * min)).toBe('1 day and 2 hours');
		expect(waitWords(48 * 60 * min)).toBe('2 days');
		// Rounded up, never shorter than it is.
		expect(waitWords(7 * 1440 * min - 1)).toBe('7 days');
		expect(waitWords(90 * min + 1000)).toBe('1 hour and 31 minutes');
	});

	it('says what is left of the weekly limit', () => {
		const k = (per_week: number, used: number, retry_at?: string): AskKindLimit => ({ per_week, used, left: per_week ? Math.max(0, per_week - used) : 0, no_limit: per_week === 0, retry_at });
		const lim = (movies: AskKindLimit, shows: AskKindLimit, music: AskKindLimit): AskLimit => ({ no_limit: false, movies, shows, music });
		const now = Date.parse('2026-10-13T12:00:00Z');
		const l = lim(k(10, 9), k(3, 3, '2026-10-15T15:00:00Z'), k(0, 4));
		expect(limitLine(null, 'movies')).toBe('');
		expect(limitLine({ ...l, no_limit: true }, 'movies')).toBe('');
		expect(limitLine(l, 'movies')).toBe('You have 1 movie ask left this week (your limit is 10).');
		expect(limitLine(lim(k(10, 8), k(1, 0), k(5, 0)), 'shows')).toBe('You have 1 show ask left this week (your limit is 1).');
		expect(limitLine(lim(k(10, 8), k(1, 0), k(5, 0)), 'music')).toBe('You have 5 music asks left this week (your limit is 5).');
		expect(limitLine(l, 'shows', now)).toBe('You have used your 3 show asks this week. Try again in 2 days and 3 hours.');
		// 0 is no limit for that kind: no line.
		expect(limitLine(l, 'music')).toBe('');
		expect(limitLines(l, undefined, now).map((x) => x.kind)).toEqual(['movies', 'shows']);
		expect(limitLines(l, ['music'], now)).toEqual([]);
	});
});

describe('wishlist views', () => {
	const ask = (state: string, mine: boolean) => ({ state, mine }) as Pick<import('./types.ts').Ask, 'state' | 'mine'>;

	it('counts each view', () => {
		expect(viewCounts([ask('asked', true), ask('approved', false), ask('asked', false)])).toEqual({ waiting: 2, all: 3, mine: 1 });
	});

	it('opens on Waiting when something waits', () => {
		expect(firstView([ask('approved', true), ask('asked', false)])).toBe('waiting');
	});

	it("opens on Everyone's when nothing waits but there is history", () => {
		expect(firstView([ask('approved', false), ask('available', true)])).toBe('all');
	});

	it('opens on Mine only when the list is empty', () => {
		expect(firstView([])).toBe('mine');
	});
});

describe('ask tabs', () => {
	const a = (state: AskState, title_id?: number) => ({ state, title_id });
	it('puts each ask in one tab', () => {
		expect(askTab(a('asked'))).toBe('waiting');
		expect(askTab(a('approved', 4))).toBe('approved');
		expect(askTab(a('approved'))).toBe('failed');
		expect(askTab(a('turned_down'))).toBe('declined');
		expect(askTab(a('available', 4))).toBe('done');
	});
	it('calls the requester tab for titles in the Library Ready', () => {
		expect(askTabs.find((t) => t.tab === 'done')?.label).toBe('Ready');
	});
	it('counts and picks the first tab with asks', () => {
		const list = [a('approved', 1), a('available', 2), a('turned_down')];
		expect(tabCounts(list)).toEqual({ waiting: 0, approved: 1, declined: 1, done: 1, failed: 0 });
		expect(firstTab(list)).toBe('approved');
		expect(firstTab([...list, a('asked')])).toBe('waiting');
		expect(firstTab([])).toBe('waiting');
	});
});

describe('approver tabs', () => {
	const a = (id: number, state: AskState, title_id?: number, decided_at?: string) => ({ id, state, title_id, asked_at: `2026-10-0${id}T00:00:00Z`, decided_at });
	it('folds the five states into Pending, Processing and Done', () => {
		expect(approverTab(a(1, 'asked'))).toBe('pending');
		expect(approverTab(a(1, 'approved', 4))).toBe('processing');
		expect(approverTab(a(1, 'approved'))).toBe('processing');
		expect(approverTab(a(1, 'available', 4))).toBe('done');
		expect(approverTab(a(1, 'turned_down'))).toBe('done');
	});
	it('counts each tab and the failed ones', () => {
		expect(approverCounts([a(1, 'asked'), a(2, 'approved'), a(3, 'approved', 1), a(4, 'turned_down')])).toEqual({ pending: 1, processing: 2, done: 1, failed: 1 });
	});
	it('puts failed first in Processing, then the newest approved', () => {
		const list = [a(1, 'approved', 1, '2026-10-05T00:00:00Z'), a(2, 'approved', 1, '2026-10-09T00:00:00Z'), a(3, 'approved', undefined, '2026-10-01T00:00:00Z')];
		expect(approverSort('processing', list).map((x) => x.id)).toEqual([3, 2, 1]);
	});
	it('puts the newest decision first in Done, the newest ask first in Pending', () => {
		const done = [a(1, 'available', 1, '2026-10-02T00:00:00Z'), a(2, 'turned_down', undefined, '2026-10-08T00:00:00Z')];
		expect(approverSort('done', done).map((x) => x.id)).toEqual([2, 1]);
		expect(approverSort('pending', [a(1, 'asked'), a(3, 'asked'), a(2, 'asked')]).map((x) => x.id)).toEqual([3, 2, 1]);
	});
});
