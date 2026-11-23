import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '#lib/core/api.ts';
import { checkNote, runCheck } from './move-plan.svelte.ts';

describe('runCheck (#486)', () => {
	afterEach(() => vi.restoreAllMocks());

	it('follows a background check to its result and reports progress', async () => {
		const req = vi.spyOn(api, 'request');
		req.mockResolvedValueOnce({ check_id: 'c1', state: 'running', step: 'titles', done: 0, total: 3 } as any);
		req.mockResolvedValueOnce({ check_id: 'c1', state: 'running', step: 'titles', done: 2, total: 3, now: 'Cunk on Earth' } as any);
		req.mockResolvedValueOnce({ check_id: 'c1', state: 'done', report: { lines: [] }, plan_hash: 'h' } as any);
		const seen: number[] = [];
		const r = await runCheck<{ plan_hash: string }>('/migrate/dry-run', {}, new AbortController().signal, (v) => seen.push(v.done ?? 0));
		expect(r.plan_hash).toBe('h');
		expect(seen).toEqual([0, 2]);
		expect((req.mock.calls[1] as any)[0]).toBe('/migrate/checks/c1');
	});

	it('starts again when a restart forgot the check', async () => {
		const req = vi.spyOn(api, 'request');
		req.mockResolvedValueOnce({ check_id: 'old', state: 'running' } as any);
		req.mockRejectedValueOnce(new ApiError('gone', 410));
		req.mockResolvedValueOnce({ state: 'done', plan_hash: 'again' } as any);
		const r = await runCheck<{ plan_hash: string }>('/migrate/plan', {}, new AbortController().signal, () => {});
		expect(r.plan_hash).toBe('again');
		expect((req.mock.calls[2] as any)[0]).toBe('/migrate/plan');
	});

	it('says how far it is in a few words', () => {
		expect(checkNote({ state: 'running', step: 'folders' })).not.toBe('');
		const s = checkNote({ state: 'running', step: 'titles', done: 2, total: 9, now: 'Cunk on Earth', counts: { not_found: 1 } });
		expect(s).toContain('Cunk on Earth');
		expect(s).toContain('2');
		expect(checkNote({ state: 'done' })).toBe('');
	});
});
