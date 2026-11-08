import { HOLD_MS, pressTimer, subLine } from './phone.ts';
import { describe, expect, it, vi } from 'vitest';

describe('subLine', () => {
	it('names one episode by code and name', () => {
		expect(subLine({ kind: 'show', title: 'Tulsa King', name: 'x', episodes: [{ season: 4, episode: 1, label: 'S04E01', name: 'W.W.D.D.' }] })).toBe('S04E01 · W.W.D.D.');
	});
	it('names a season pack by season', () => {
		const eps = [1, 2, 3].map((n) => ({ season: 3, episode: n, label: '' }));
		expect(subLine({ kind: 'show', title: 'T', name: 'x', episodes: eps })).toBe('Season 3');
	});
	it('falls back to covers for shows, album for music, year for movies', () => {
		expect(subLine({ kind: 'show', title: 'T', name: 'x', covers: 'S01E02' })).toBe('S01E02');
		expect(subLine({ kind: 'artist', title: 'A', name: 'x', album: 'Mud' })).toBe('Mud');
		expect(subLine({ kind: 'movie', title: 'Dune', name: 'x', year: 2026 })).toBe('2026');
		expect(subLine({ title: '', name: 'raw.release' })).toBe('');
	});
});

describe('pressTimer', () => {
	it('fires after the hold and swallows one click', () => {
		vi.useFakeTimers();
		const on = vi.fn();
		const p = pressTimer(on);
		p.down(10, 10);
		expect(p.holding).toBe(true);
		vi.advanceTimersByTime(HOLD_MS - 1);
		expect(on).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(on).toHaveBeenCalledOnce();
		expect(p.takeFired()).toBe(true);
		expect(p.takeFired()).toBe(false);
		vi.useRealTimers();
	});
	it('cancels on release or when the finger moves away', () => {
		vi.useFakeTimers();
		const on = vi.fn();
		const p = pressTimer(on);
		p.down(0, 0);
		p.cancel();
		p.down(0, 0);
		p.move(4, 4);
		expect(p.holding).toBe(true);
		p.move(20, 0);
		vi.advanceTimersByTime(HOLD_MS * 2);
		expect(on).not.toHaveBeenCalled();
		expect(p.takeFired()).toBe(false);
		vi.useRealTimers();
	});
});
