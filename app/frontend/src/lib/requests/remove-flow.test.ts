import { beforeEach, describe, expect, it, vi } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

const calls: [string, string, unknown?][] = [];
const pushed: string[] = [];
let fail = false;
let removed = 0;
vi.mock('#lib/core/api.ts', () => ({
	api: {
		del: async (p: string) => { calls.push(['del', p]); if (fail) throw new Error('no'); },
		post: async (p: string, b: unknown) => { calls.push(['post', p, b]); if (fail) throw new Error('no'); return { removed }; }
	},
	message: (e: unknown) => (e instanceof Error ? e.message : 'x')
}));
vi.mock('#lib/core/toasts.svelte.ts', () => ({ toasts: { push: (text: string) => void pushed.push(text) } }));
vi.mock('#lib/core/i18n/i18n.svelte.ts', () => ({ t: (k: string, v?: Record<string, unknown>) => (v ? `${k} ${JSON.stringify(v)}` : k) }));

import { anyShared, createRemoveFlow, removeRequests, clearParts, clearScope } from './remove-flow.svelte.ts';

const ask = { id: 7 } as never;

describe('removing requests', () => {
	beforeEach(() => { calls.length = 0; pushed.length = 0; fail = false; removed = 0; });

	it('removes one with DELETE and many by filter', async () => {
		await removeRequests({ one: ask });
		await removeRequests({ all: 'failed', n: 3 });
		await removeRequests({ all: 'finished', n: 2 });
		expect(calls).toEqual([['del', '/asks/7'], ['post', '/asks/remove', { filter: 'failed' }], ['post', '/asks/remove', { filter: 'finished' }]]);
	});

	it('says it is a clear of the cards it showed, so the server keeps only those still failed or finished', async () => {
		await removeRequests({ all: 'failed', n: 3, ids: [4, 9], byDownload: [12] });
		await removeRequests({ all: 'finished', n: 2, ids: [1, 2] });
		await removeRequests({ one: { id: 5, state: 'asked' } as never });
		expect(calls).toEqual([
			['post', '/asks/remove', { ids: [4, 9], only: 'failed', failed_downloads: [12] }],
			['post', '/asks/remove', { ids: [1, 2], only: 'finished' }],
			['post', '/asks/remove', { ids: [5] }]
		]);
	});

	it('tells the count the server really removed', async () => {
		removed = 2;
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ all: 'failed', n: 3, ids: [4, 9, 12] });
		await f.confirm();
		expect(pushed).toEqual(['requests.board.removedN.other {"n":2}']);
	});

	const card = (id: number, o: Record<string, unknown> = {}) => ({ id, title_id: null, problem: '', ...o }) as never;

	it('with no filter on, 250 finished and 250 failed requests go by the filter alone, not by the cards loaded', async () => {
		const finished = Array.from({ length: 250 }, (_, i) => card(i + 1));
		const failed = Array.from({ length: 250 }, (_, i) => card(1000 + i, i < 2 ? { title_id: 5 } : {}));
		await removeRequests({ all: 'finished', n: 250, ...clearParts('finished', finished, false) });
		await removeRequests({ all: 'failed', n: 250, ...clearParts('failed', failed, false) });
		expect(calls).toEqual([
			['post', '/asks/remove', { filter: 'finished' }],
			['post', '/asks/remove', { filter: 'failed', ids: [], failed_downloads: [1000, 1001] }]
		]);
	});

	it('with a type or person filter on, only the shown ids are sent', () => {
		expect(clearParts('finished', [card(1), card(2)], true)).toEqual({ ids: [1, 2] });
		expect(clearParts('failed', [card(3), card(4, { title_id: 9 })], true)).toEqual({ ids: [3, 4], byDownload: [4] });
	});

	it('picks the failed downloads when the owner confirms, not when the sheet opened', async () => {
		let shown = [card(1, { title_id: 7 }), card(2, { title_id: 8 })];
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ all: 'failed', n: 2, current: () => shown, filtered: false });
		shown = [card(2, { title_id: 8 })]; // card 1's download recovered while the sheet was open
		await f.confirm();
		expect(calls).toEqual([['post', '/asks/remove', { filter: 'failed', ids: [], failed_downloads: [2] }]]);
	});

	it('does not clear everything when a filter is on and nothing shown is left', async () => {
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ all: 'finished', n: 1, current: () => [], filtered: true });
		await f.confirm();
		expect(calls).toEqual([]);
		expect(pushed).toEqual(['requests.board.removedNone']);
	});

	it('says plainly when nothing was removed', async () => {
		removed = 0;
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ all: 'failed', n: 3 });
		await f.confirm();
		expect(pushed).toEqual(['requests.board.removedNone']);
	});

	it('closes the sheet as soon as the server has answered, without waiting for the list to reload', async () => {
		removed = 5;
		const f = createRemoveFlow({ load: () => new Promise<void>(() => {}) });
		f.ask({ all: 'failed', n: 5 });
		await f.confirm();
		expect(f.open).toBe(false);
		expect(f.busy).toBe(false);
		expect(pushed).toEqual(['requests.board.removedN.other {"n":5}']);
	});

	it('says Removed 1 request when one is removed from its window', async () => {
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ one: { id: 7, state: 'approved' } as never });
		await f.confirm();
		expect(pushed).toEqual(['requests.board.removedN.one {"n":1}']);
	});

	it('with a filter on sends the shown ids and no filter, so a 1.1.4 server removes only those', async () => {
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ all: 'failed', n: 2, current: () => [card(1), card(2, { title_id: 9 })], filtered: true });
		await f.confirm();
		expect(calls).toEqual([['post', '/asks/remove', { ids: [1, 2], only: 'failed', failed_downloads: [2] }]]);
		expect(JSON.stringify(calls[0][2])).not.toContain('"filter"');
	});

	it('sends nothing to the remove route until the owner confirms', async () => {
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ all: 'finished', n: 250, current: () => [], filtered: false });
		f.ask({ all: 'failed', n: 250, current: () => [], filtered: false });
		expect(calls).toEqual([]);
		await f.confirm();
		expect(calls.length).toBe(1);
	});

	it('names the remove route in one place only, the confirm', () => {
		const here = fileURLToPath(new URL('../..', import.meta.url));
		const walk = (d: string): string[] => readdirSync(d).flatMap((n) => { const p = join(d, n); return statSync(p).isDirectory() ? (n === 'node_modules' ? [] : walk(p)) : /\.(svelte|ts)$/.test(n) && !n.endsWith('.test.ts') ? [p] : []; });
		const files = walk(here).filter((p) => readFileSync(p, 'utf8').includes('/asks/remove'));
		expect(files.map((p) => p.split(/[\\/]/).pop())).toEqual(['remove-flow.svelte.ts']);
		const src = readFileSync(files[0], 'utf8');
		expect([...src.matchAll(/removeRequests\(/g)].length).toBe(2); // its definition and the confirm
	});

	it('with a status picked on the Done tab, Clear finished counts and sends only the shown cards', async () => {
		const word = (a: { state: string }) => (a.state === 'turned_down' ? 'Declined' : 'Available');
		const done = [card(1, { state: 'turned_down' }), card(2, { state: 'available' }), card(3, { state: 'turned_down' }), card(4, { state: 'available' }), card(5, { state: 'available' })];
		// No status: every finished request, the server's total.
		expect(clearScope(done, word, '', 250)).toEqual({ asks: done, n: 250, narrowed: false });
		// Only Declined: the two declined cards, by id.
		const s = clearScope(done, word, 'Declined', 250);
		expect(s.n).toBe(2);
		expect(s.narrowed).toBe(true);
		const f = createRemoveFlow({ load: async () => {} });
		f.ask({ all: 'finished', n: s.n, current: () => s.asks, filtered: s.narrowed });
		await f.confirm();
		expect(calls).toEqual([['post', '/asks/remove', { ids: [1, 3], only: 'finished' }]]);
	});

	it('with a status picked on Processing that is not Failed, Clear failed has nothing to take', () => {
		const word = () => 'Failed';
		expect(clearScope([card(1), card(2)], word, 'Downloading', 40)).toEqual({ asks: [], n: 0, narrowed: true });
		expect(clearScope([card(1), card(2)], word, 'Failed', 40).n).toBe(2);
	});

	it('knows when a request was asked for by more than one person', () => {
		const p = (n: number) => ({ people: Array.from({ length: n }, (_, i) => ({ id: i })) }) as never;
		expect(anyShared([p(1), p(1)])).toBe(false);
		expect(anyShared([p(1), p(2)])).toBe(true);
		expect(anyShared([])).toBe(false);
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
