import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError } from '#lib/core/api.ts';
import { session } from '#lib/core/session.svelte.ts';
import { createMovePassword } from './move-password.svelte.ts';
import { createMoveRun } from './move-run.svelte.ts';

const start = { hasPassword: session.s.hasPassword, localRecent: session.s.localRecent };
afterEach(() => {
	session.s.hasPassword = start.hasPassword;
	session.s.localRecent = start.localRecent;
	vi.restoreAllMocks();
});

// The page as the import sees it: the owner's choices on step 2, a fresh test run.
function pageOf() {
	const picks = { '/old/movies': '/data/media/movies', '/old/tv': '' };
	const ctx: any = {
		step: 'choose',
		dry: { lines: [] },
		stale: false,
		planHash: 'h1',
		moves: [],
		picked: [3, 5],
		choices: () => ({ snapshots: [3, 5], path_rules: [{ from: '/old/movies', to: picks['/old/movies'] }], leave_folders: [] }),
		quiet: (e: unknown) => e instanceof ApiError && e.body.code === 'reauth',
		follower: { follow: vi.fn() },
		setUrl: vi.fn(),
		focusStep: vi.fn()
	};
	ctx.runFlow = createMoveRun(ctx);
	return { ctx, picks, pw: createMovePassword(ctx) };
}

describe('the password check while importing', () => {
	it('goes on with the same choices once the password is confirmed', async () => {
		session.s.hasPassword = true;
		session.s.localRecent = true;
		const post = vi.spyOn(api, 'post')
			.mockRejectedValueOnce(new ApiError('confirm your local password again', 403, { code: 'reauth' }))
			.mockResolvedValueOnce({ move: { id: 9, state: 'queued', snapshots: [3, 5] } });
		const { ctx, picks, pw } = pageOf();
		await ctx.runFlow.start();
		// The server asked for the password: the page waits on step 2, choices as they were.
		pw.waitForCheck();
		pw.settle();
		expect(post).toHaveBeenCalledTimes(1);
		expect(ctx.step).toBe('choose');
		expect(picks['/old/movies']).toBe('/data/media/movies');
		// The password is confirmed: the import goes on by itself, with the same body.
		session.s.localRecent = true;
		pw.settle();
		await vi.waitFor(() => expect(post).toHaveBeenCalledTimes(2));
		expect(post.mock.calls[1]).toEqual(post.mock.calls[0]);
		expect(ctx.step).toBe('move');
		// Once only.
		pw.settle();
		expect(post).toHaveBeenCalledTimes(2);
	});

	it('does not import on its own once the owner has left step 2', () => {
		session.s.hasPassword = true;
		const post = vi.spyOn(api, 'post');
		const { ctx, pw } = pageOf();
		pw.waitForCheck();
		ctx.step = 'read';
		session.s.localRecent = true;
		pw.settle();
		expect(post).not.toHaveBeenCalled();
	});
});
