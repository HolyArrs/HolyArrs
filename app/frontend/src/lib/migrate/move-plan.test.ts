import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '#lib/core/api.ts';
import { OTHER } from './move.ts';
import { createMovePlan } from './move-plan.svelte.ts';

function ctxOf(over: Record<string, unknown> = {}) {
	const ctx: any = {
		extra: [],
		skip: [],
		picked: ['a'],
		ruleErrors: 0,
		picks: {},
		others: {},
		people: {},
		seen: new Map(),
		planSeq: 0,
		trySeq: 0,
		changes: 0,
		destroyed: false,
		plan: null,
		choices: () => ({ picks: ctx.picks, skip: ctx.skip, extra: ctx.extra }),
		changed: vi.fn(),
		quiet: () => false,
		...over
	};
	return ctx;
}

describe('createMovePlan', () => {
	let req: ReturnType<typeof vi.spyOn>;
	beforeEach(() => {
		req = vi.spyOn(api, 'request');
	});
	afterEach(() => {
		vi.restoreAllMocks();
		vi.useRealTimers();
	});

	it('plans with the ctx choices and keeps the plan and suggestions on ctx', async () => {
		const plan = { items: [] };
		req.mockResolvedValue({ plan, suggested_path_rules: [] });
		const ctx = ctxOf({ skip: ['/old/b'], extra: [{ from: '/old', to: '/new' }] });
		await createMovePlan(ctx).makePlan();
		expect(req).toHaveBeenCalledTimes(1);
		const [path, opts] = req.mock.calls[0] as [string, any];
		expect(path).toBe('/migrate/plan');
		expect(opts.method).toBe('POST');
		expect(opts.body.skip).toEqual(['/old/b']);
		expect(opts.body.extra).toEqual([{ from: '/old', to: '/new' }]);
		expect(ctx.plan).toBe(plan);
		expect(ctx.suggested).toEqual({});
		expect(ctx.planning).toBe(false);
		expect(ctx.planError).toBe('');
	});

	it('keeps the folders that are not here and the folders HolyArrs can see', async () => {
		req.mockResolvedValue({ plan: {}, suggested_path_rules: [], missing_paths: { '/mnt/78/Movies': '/home/mark/Movies' }, seen_folders: ['/data/movies'] });
		const ctx = ctxOf();
		await createMovePlan(ctx).makePlan();
		expect(ctx.missingPaths).toEqual({ '/mnt/78/Movies': '/home/mark/Movies' });
		expect(ctx.seenFolders).toEqual(['/data/movies']);
		req.mockResolvedValue({ plan: {}, suggested_path_rules: [] });
		await createMovePlan(ctx).makePlan();
		expect(ctx.missingPaths).toEqual({});
		expect(ctx.seenFolders).toEqual([]);
	});

	it('does not plan with nothing picked or with rule errors', async () => {
		req.mockResolvedValue({ plan: {} });
		await createMovePlan(ctxOf({ picked: [] })).makePlan();
		await createMovePlan(ctxOf({ ruleErrors: 1 })).makePlan();
		expect(req).not.toHaveBeenCalled();
	});

	it('fills a folder nobody set from a suggestion, then plans again with it', async () => {
		req
			.mockResolvedValueOnce({ plan: { n: 1 }, suggested_path_rules: [{ from: '/old/x', to: '/new/x' }, { from: '/old/y', to: '/new/y' }] })
			.mockResolvedValueOnce({ plan: { n: 2 }, suggested_path_rules: [] });
		const ctx = ctxOf({ picks: { '/old/y': '/mine' }, extra: [] });
		await createMovePlan(ctx).makePlan();
		await vi.waitFor(() => expect(req).toHaveBeenCalledTimes(2));
		expect(ctx.picks).toEqual({ '/old/y': '/mine', '/old/x': '/new/x' });
		expect(ctx.changed).toHaveBeenCalled();
	});

	it('leaves a folder alone when an extra rule already covers it', async () => {
		req.mockResolvedValue({ plan: {}, suggested_path_rules: [{ from: '/old', to: '/new' }] });
		const ctx = ctxOf({ extra: [{ from: ' /old ', to: '/mine' }] });
		await createMovePlan(ctx).makePlan();
		expect(ctx.picks).toEqual({});
		expect(req).toHaveBeenCalledTimes(1);
	});

	it('drops the answer of an older plan', async () => {
		let first!: (v: unknown) => void;
		req.mockImplementationOnce(() => new Promise((r) => (first = r)) as any).mockResolvedValueOnce({ plan: { n: 'new' } });
		const ctx = ctxOf();
		const mp = createMovePlan(ctx);
		const slow = mp.makePlan();
		await mp.makePlan();
		first({ plan: { n: 'old' } });
		await slow;
		expect(ctx.plan).toEqual({ n: 'new' });
	});

	it('shows a plan error, and marks a 404 as gone', async () => {
		req.mockRejectedValueOnce(new Error('boom'));
		const ctx = ctxOf();
		await createMovePlan(ctx).makePlan();
		expect(ctx.planError).not.toBe('');
		expect(ctx.planning).toBe(false);
		req.mockRejectedValueOnce(new ApiError('gone', 404));
		const c2 = ctxOf();
		await createMovePlan(c2).makePlan();
		expect(c2.gone).toBe(true);
		expect(c2.planError ?? '').toBe('');
	});

	it('onextra and onfolder write to ctx and tell the page', () => {
		vi.useFakeTimers();
		req.mockResolvedValue({ plan: {} });
		const ctx = ctxOf();
		const mp = createMovePlan(ctx);
		mp.onextra([{ from: '/old', to: '/new' }]);
		expect(ctx.extra).toEqual([{ from: '/old', to: '/new' }]);
		expect(ctx.changed).toHaveBeenCalledTimes(1);
		expect(req).not.toHaveBeenCalled();
		mp.onfolder('/old', OTHER);
		expect(ctx.picks['/old']).toBe(OTHER);
		vi.advanceTimersByTime(1000);
		expect(req).not.toHaveBeenCalled();
		mp.onfolder('/old', '/new');
		vi.advanceTimersByTime(399);
		expect(req).not.toHaveBeenCalled();
		vi.advanceTimersByTime(1);
		expect(req).toHaveBeenCalledTimes(1);
	});

	it('ongroup adds and removes names from the skip list and plans again', () => {
		vi.useFakeTimers();
		req.mockResolvedValue({ plan: {} });
		const ctx = ctxOf({ skip: ['/old/b'] });
		const mp = createMovePlan(ctx);
		mp.ongroup('/old/a', false);
		expect(ctx.skip).toEqual(['/old/b', '/old/a']);
		mp.ongroup('/old/b', true);
		expect(ctx.skip).toEqual(['/old/a']);
		vi.advanceTimersByTime(400);
		expect(req).toHaveBeenCalledTimes(1);
		expect((req.mock.calls[0][1] as any).body.skip).toEqual(['/old/a']);
	});

	it('onperson sets and clears a person choice', () => {
		vi.useFakeTimers();
		const ctx = ctxOf();
		const mp = createMovePlan(ctx);
		mp.onperson('p1', 'merge' as any);
		expect(ctx.people).toEqual({ p1: 'merge' });
		mp.onperson('p1', null);
		expect(ctx.people).toEqual({});
	});

	it('the test run plans first when there is no plan, then keeps what it saw', async () => {
		req
			.mockResolvedValueOnce({ plan: { n: 1 }, suggested_path_rules: [] })
			.mockResolvedValueOnce({ report: { lines: [{ key: 'k1', choose: true }, { key: 'k2' }] }, plan_hash: 'h' });
		const ctx = ctxOf();
		await createMovePlan(ctx).runTry();
		expect((req.mock.calls[0] as any)[0]).toBe('/migrate/plan');
		expect((req.mock.calls[1] as any)[0]).toBe('/migrate/dry-run');
		expect(ctx.planHash).toBe('h');
		expect([...ctx.seen.keys()]).toEqual(['k1']);
		expect(ctx.trying).toBe(false);
	});

	it('a failed test run says so', async () => {
		req.mockRejectedValueOnce(new Error('nope'));
		const ctx = ctxOf({ plan: { n: 1 } });
		await createMovePlan(ctx).runTry();
		expect(ctx.tryFailed).toBe(true);
		expect(ctx.actionError).not.toBe('');
	});
});
