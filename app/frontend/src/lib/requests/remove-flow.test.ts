import { beforeEach, describe, expect, it, vi } from 'vitest';

const calls: [string, string, unknown?][] = [];
let fail = false;
vi.mock('#lib/core/api.ts', () => ({
	api: {
		del: async (p: string) => { calls.push(['del', p]); if (fail) throw new Error('no'); },
		post: async (p: string, b: unknown) => { calls.push(['post', p, b]); if (fail) throw new Error('no'); }
	},
	message: (e: unknown) => (e instanceof Error ? e.message : 'x')
}));

import { createRemoveFlow, removeRequests } from './remove-flow.svelte.ts';

const ask = { id: 7 } as never;

describe('removing requests', () => {
	beforeEach(() => { calls.length = 0; fail = false; });

	it('removes one with DELETE and many by filter', async () => {
		await removeRequests({ one: ask });
		await removeRequests({ all: 'failed', n: 3 });
		await removeRequests({ all: 'finished', n: 2 });
		expect(calls).toEqual([['del', '/asks/7'], ['post', '/asks/remove', { filter: 'failed' }], ['post', '/asks/remove', { filter: 'finished' }]]);
	});

	it('closes and reloads after the confirm; a failure keeps the sheet open with the error', async () => {
		const load = vi.fn(async () => {});
		const f = createRemoveFlow({ load });
		f.ask({ all: 'failed', n: 2 });
		expect(f.open).toBe(true);
		await f.confirm();
		expect(load).toHaveBeenCalledTimes(1);
		expect(f.open).toBe(false);
		fail = true;
		f.ask({ one: ask });
		await f.confirm();
		expect(f.open).toBe(true);
		expect(f.error).toBe('no');
		expect(f.busy).toBe(false);
	});
});
