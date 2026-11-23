import { describe, expect, it } from 'vitest';
import { daysLeft, daysLeftWords, fileName } from './holding.ts';

describe('holding', () => {
	const now = Date.parse('2026-11-03T12:00:00Z');
	it('counts whole days left, rounding up', () => {
		expect(daysLeft('2026-11-05T00:00:00Z', now)).toBe(2);
		expect(daysLeft('2026-11-04T00:00:00Z', now)).toBe(1);
		expect(daysLeft('2026-11-01T00:00:00Z', now)).toBe(0);
		expect(daysLeft(undefined, now)).toBeNull();
		expect(daysLeft('soon', now)).toBeNull();
	});
	it('says it in plain words', () => {
		expect(daysLeftWords('2026-11-10T00:00:00Z', now)).toBe('7 days left');
		expect(daysLeftWords('2026-11-04T00:00:00Z', now)).toBe('1 day left');
		expect(daysLeftWords('2026-11-03T00:00:00Z', now)).toBe('Goes today');
		expect(daysLeftWords(undefined, now)).toBe('Kept until you empty it');
	});
	it('takes the file name off a path', () => {
		expect(fileName('Dune (2024)/Dune.mkv')).toBe('Dune.mkv');
		expect(fileName('Dune.mkv')).toBe('Dune.mkv');
	});
});

describe('#489 held copies', () => {
	const f = (id: number, replaced: boolean, expires_at?: string) => ({ id, path: `a/${id}.mkv`, held_path: '', folder: '', title_id: 1, title: 'T', kind: 'movie', title_deleted: false, size: 1, deleted_at: '', expires_at, replaced });
	it('filters by reason', async () => {
		const { heldOfReason } = await import('./holding.ts');
		const list = [f(1, true), f(2, false), f(3, true)];
		expect(heldOfReason(list, 'all').map((x) => x.id)).toEqual([1, 2, 3]);
		expect(heldOfReason(list, 'replaced').map((x) => x.id)).toEqual([1, 3]);
		expect(heldOfReason(list, 'deleted').map((x) => x.id)).toEqual([2]);
	});
	it('takes the soonest expiry, none when one is kept until emptied', async () => {
		const { soonestExpiry } = await import('./holding.ts');
		expect(soonestExpiry([f(1, true, '2026-11-20T00:00:00Z'), f(2, false, '2026-11-18T00:00:00Z')])).toBe('2026-11-18T00:00:00Z');
		expect(soonestExpiry([f(1, true, '2026-11-20T00:00:00Z'), f(2, false)])).toBeUndefined();
	});
});
