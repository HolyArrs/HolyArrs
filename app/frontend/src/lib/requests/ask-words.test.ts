import { alsoDeclined, approverCounts, approverSort, approverTab, askMoments, askTab, askTabs, askedBy, firstTab, libraryMoments, momentWords, tabCounts, limitLine, limitLines, names, waitWords } from './ask-words.ts';
import type { AskKindLimit, AskLimit } from './types.ts';
import { describe, expect, it } from 'vitest';
import type { AskState } from '../core/types.ts';

describe('requests words', () => {
	it('joins names', () => {
		expect(names([])).toBe('');
		expect(names(['ana'])).toBe('ana');
		expect(names(['ana', 'bo'])).toBe('ana and bo');
		expect(names(['ana', 'bo', 'cy'])).toBe('ana, bo and cy');
	});

	it('says who asked', () => {
		const who = (id: number, name: string) => ({ id, name, asked_at: '' });
		expect(askedBy({ asked_by: { id: 2, name: 'ana' }, people: [who(2, 'ana'), who(3, 'bo')] })).toBe('Asked by ana and bo');
	});

	it('says waits like the server', () => {
		const min = 60_000;
		expect(waitWords(20_000)).toBe('a minute');
		expect(waitWords(40 * min)).toBe('40 minutes');
		expect(waitWords(90 * min)).toBe('1 hour and 30 minutes');
		expect(waitWords(26 * 60 * min)).toBe('1 day and 2 hours');
		expect(waitWords(48 * 60 * min)).toBe('2 days');
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
		expect(limitLine(l, 'movies')).toBe('You have 1 movie request left this week (your limit is 10).');
		expect(limitLine(lim(k(10, 8), k(1, 0), k(5, 0)), 'shows')).toBe('You have 1 show request left this week (your limit is 1).');
		expect(limitLine(lim(k(10, 8), k(1, 0), k(5, 0)), 'music')).toBe('You have 5 music requests left this week (your limit is 5).');
		expect(limitLine(l, 'shows', now)).toBe('You have used your 3 show requests this week. Try again in 2 days and 3 hours.');
		expect(limitLine(l, 'music')).toBe('');
		expect(limitLines(l, undefined, now).map((x) => x.kind)).toEqual(['movies', 'shows']);
		expect(limitLines(l, ['music'], now)).toEqual([]);
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

describe('alsoDeclined (#136)', () => {
	const w = (id: number, name: string, display_name?: string) => ({ id, name, display_name });
	it('is empty when nobody declined before', () => {
		expect(alsoDeclined({})).toBe('');
		expect(alsoDeclined({ also_declined: [] })).toBe('');
	});
	it('names everyone, display name first', () => {
		expect(alsoDeclined({ also_declined: [w(2, 'bo', 'Bo B')] })).toBe('Bo B declined it before.');
		expect(alsoDeclined({ also_declined: [w(2, 'bo', 'Bo B'), w(3, 'cy', 'Cy'), w(4, 'di', ' Di ')] })).toBe('Bo B, Cy and Di declined it before.');
	});
	it('uses my own fresh display name for me', () => {
		expect(alsoDeclined({ also_declined: [w(1, 'luis', 'Old')] }, { id: 1, displayName: 'Luis R' })).toBe('Luis R declined it before.');
	});
	it('never says a sign-in name: people without a display name are counted', () => {
		expect(alsoDeclined({ also_declined: [w(2, 'bo', 'Bo B'), w(3, 'cy')] })).toBe('Bo B and 1 other declined it before.');
		expect(alsoDeclined({ also_declined: [w(3, 'cy')] })).toBe('1 other person declined it before.');
		expect(alsoDeclined({ also_declined: [w(3, 'cy'), w(4, 'di', '  ')] })).toBe('2 other people declined it before.');
	});
});

describe('Alert moments', () => {
	it('offers every moment that has words, so a new Alert speaks for it', () => {
		const offered: string[] = [...askMoments, ...libraryMoments];
		expect(Object.keys(momentWords).filter((m) => !offered.includes(m))).toEqual([]);
	});
	it('puts the download moments between Sent to download and In the Library, as the server lists them', () => {
		expect(libraryMoments.slice(0, 4)).toEqual(['grab', 'downloaded', 'download_failed', 'import']);
	});
});
