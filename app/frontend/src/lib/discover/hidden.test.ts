import { describe, expect, it, vi } from 'vitest';
import { addHide, hiddenPill, hideFor, hideLine, hideName, loadHides, showAgain, type Hide } from './hidden.ts';

const hide = (h: Partial<Hide>): Hide => ({ id: 1, kind: 'movie', user_id: 0, created_at: '2026-11-11T00:00:00Z', ...h });
const people = new Map([[2, 'Sam']]);

describe('hidden from browsing', () => {
	it('names a title hide by its name and a genre hide by its genre', () => {
		expect(hideName(hide({ tmdb_id: '19995', name: 'Avatar' }))).toBe('Avatar');
		expect(hideName(hide({ genre: 'Horror' }))).toBe('Horror');
		expect(hideName(hide({ tmdb_id: '19995' }))).toBe('TMDB 19995');
	});

	it('says who a hide is for and what it covers', () => {
		expect(hideFor(hide({}), people)).toBe('Everyone');
		expect(hideFor(hide({ user_id: 2 }), people)).toBe('Sam');
		expect(hideFor(hide({ user_id: 9 }), people)).toBe('Removed person');
		expect(hideLine(hide({ tmdb_id: '1', name: 'A' }), people)).toBe('Movie · Everyone');
		expect(hideLine(hide({ kind: 'show', genre: 'Crime', user_id: 2 }), people)).toBe('Show genre · Sam');
	});

	it('pill names the one person, or counts them', () => {
		expect(hiddenPill([], people)).toBe('Hidden from browsing');
		expect(hiddenPill([hide({}), hide({ id: 2, genre: 'Horror' })], people)).toBe('Hidden from browsing · Everyone');
		// A hide for everyone already covers each person.
		expect(hiddenPill([hide({}), hide({ id: 2, user_id: 2 })], people)).toBe('Hidden from browsing · Everyone');
		expect(hiddenPill([hide({ user_id: 2 }), hide({ id: 2, user_id: 3 })], people)).toBe('Hidden from browsing · 2 people');
		// Before the names arrive, two people are still two, never one "Removed person".
		expect(hiddenPill([hide({ user_id: 2 }), hide({ id: 2, user_id: 3 })], new Map())).toBe('Hidden from browsing · 2 people');
		expect(hiddenPill([hide({ user_id: 2 }), hide({ id: 2, user_id: 2, genre: 'Horror' })], people)).toBe('Hidden from browsing · Sam');
	});

	it('asks browsing again after a hide and after Show again', async () => {
		const c = { get: vi.fn(async () => ({ hidden: [hide({})] })), post: vi.fn(async () => hide({ id: 7 })), del: vi.fn(async () => undefined), freshBrowsing: vi.fn() };
		expect(await loadHides(c as never, 2)).toHaveLength(1);
		expect(c.get).toHaveBeenCalledWith('/hidden?user_id=2');
		expect((await addHide(c as never, { kind: 'movie', genre: 'Horror' })).id).toBe(7);
		await showAgain(c as never, 7);
		expect(c.del).toHaveBeenCalledWith('/hidden/7');
		expect(c.freshBrowsing).toHaveBeenCalledTimes(2);
	});
});
