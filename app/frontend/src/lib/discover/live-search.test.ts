import { createSuggest, live, step, subline, suggestHref, SUGGEST_WAIT, type SuggestAnswer } from './live-search.ts';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const ans = (q: string): SuggestAnswer => ({ q, completions: [q], items: [] });

describe('createSuggest', () => {
	beforeEach(() => {
		vi.useFakeTimers();
		live.on = null;
	});
	afterEach(() => vi.useRealTimers());

	it('waits for 2 characters and the pause after the last key', async () => {
		const fetch = vi.fn((q: string) => Promise.resolve(ans(q)));
		const got: (SuggestAnswer | null)[] = [];
		const s = createSuggest(fetch, (a) => got.push(a));
		s.type('r');
		s.type('ra');
		s.type('rad');
		await vi.advanceTimersByTimeAsync(SUGGEST_WAIT - 1);
		expect(fetch).not.toHaveBeenCalled();
		await vi.advanceTimersByTimeAsync(1);
		expect(fetch).toHaveBeenCalledTimes(1);
		expect(fetch.mock.calls[0][0]).toBe('rad');
		expect(got.at(-1)?.q).toBe('rad');
	});

	it('a slow older answer never overwrites a newer list', async () => {
		const pending: Record<string, (a: SuggestAnswer) => void> = {};
		const signals: Record<string, AbortSignal> = {};
		const fetch = (q: string, signal: AbortSignal) => {
			signals[q] = signal;
			return new Promise<SuggestAnswer>((res) => (pending[q] = res));
		};
		const got: (SuggestAnswer | null)[] = [];
		const s = createSuggest(fetch, (a) => got.push(a));
		s.type('ra');
		await vi.advanceTimersByTimeAsync(SUGGEST_WAIT);
		s.type('radi');
		await vi.advanceTimersByTimeAsync(SUGGEST_WAIT);
		expect(signals.ra.aborted).toBe(true);
		pending.radi(ans('radi'));
		await Promise.resolve();
		pending.ra(ans('ra'));
		await Promise.resolve();
		await Promise.resolve();
		expect(got.filter(Boolean).map((a) => a!.q)).toEqual(['radi']);
	});

	it('a 404 turns live search off', async () => {
		const fetch = vi.fn(() => Promise.reject(Object.assign(new Error('off'), { status: 404 })));
		const s = createSuggest(fetch, () => {});
		s.type('radi');
		await vi.advanceTimersByTimeAsync(SUGGEST_WAIT);
		expect(live.on).toBe(false);
		s.type('radio');
		await vi.advanceTimersByTimeAsync(SUGGEST_WAIT);
		expect(fetch).toHaveBeenCalledTimes(1);
	});
});

describe('keys and rows', () => {
	it('step moves through the list and back to the field', () => {
		expect(step(-1, 3, 1)).toBe(0);
		expect(step(2, 3, 1)).toBe(0);
		expect(step(0, 3, -1)).toBe(-1);
		expect(step(-1, 0, 1)).toBe(-1);
	});
	it('rows open the right page', () => {
		expect(suggestHref({ kind: 'movie', name: 'A', title_id: 4 })).toMatch(/\/library\/4$/);
		expect(suggestHref({ kind: 'movie', name: 'A', tmdb: '12' })).toMatch(/\/title\/movie\/12$/);
		expect(suggestHref({ kind: 'artist', name: 'A', mbid: 'abc' })).toMatch(/\/music\/artist\/abc$/);
		expect(suggestHref({ kind: 'song', name: 'Creep', sub: 'Radiohead' })).toMatch(/\/search\?q=Creep%20Radiohead&kind=music$/);
		expect(suggestHref({ kind: 'person', name: 'P', person_id: '7' })).toMatch(/\/people\/7$/);
	});
	it('sublines start with the type', () => {
		expect(subline({ kind: 'album', name: 'X', sub: 'Radiohead' })).toBe('Album · Radiohead');
		expect(subline({ kind: 'show', name: 'X', sub: 'Show · 2019' })).toBe('Show · 2019');
		expect(subline({ kind: 'artist', name: 'X' })).toBe('Artist');
	});
});
