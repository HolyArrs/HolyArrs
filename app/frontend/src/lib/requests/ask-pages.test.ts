import { describe, expect, it } from 'vitest';
import type { Ask } from '../core/types.ts';
import { allWaiting, readList } from './ask-pages.ts';

const ask = (id: number, state = 'asked'): Ask => ({ id, state }) as unknown as Ask;
const counts = { pending: 0, processing: 0, done: 0, failed: 0 };
// A server holding n requests, answering ?limit= and ?offset= like the real one.
function server(n: number, state = 'asked') {
	const all = Array.from({ length: n }, (_, i) => ask(i + 1, state));
	const calls: string[] = [];
	const get = async (path: string) => {
		calls.push(path);
		const q = new URLSearchParams(path.split('?')[1] ?? '');
		const off = Number(q.get('offset') ?? 0);
		const lim = Number(q.get('limit') ?? 200);
		return { asks: all.slice(off, off + lim), counts, total: n };
	};
	return { get, calls };
}

describe('Reading the requests list', () => {
	it('reads one page when that covers what is shown', async () => {
		const s = server(251);
		const r = await readList(s.get, 0);
		expect(r.asks).toHaveLength(200);
		expect(r.total).toBe(251);
		expect(s.calls).toEqual(['/asks?limit=200']);
	});
	it('reads again as far as the page had scrolled, with nothing repeated', async () => {
		const s = server(651);
		const r = await readList(s.get, 600);
		expect(r.asks).toHaveLength(600);
		expect(new Set(r.asks.map((a) => a.id)).size).toBe(600);
		expect(s.calls).toEqual(['/asks?limit=200', '/asks?limit=400&offset=200']);
	});
});

describe('Approve all', () => {
	it('finds every waiting request, past the first page', async () => {
		const s = server(1100);
		const list = await allWaiting(s.get, () => true);
		expect(list).toHaveLength(1100);
		expect(s.calls).toEqual(['/asks?state=asked&limit=500', '/asks?state=asked&limit=500&offset=500', '/asks?state=asked&limit=500&offset=1000']);
	});
	it('keeps only the ones the filters show', async () => {
		const s = server(300);
		expect(await allWaiting(s.get, (a) => a.id % 2 === 0)).toHaveLength(150);
	});
});

describe('An older server', () => {
	it('that ignores offset ends Approve all after one page', async () => {
		const page = Array.from({ length: 500 }, (_, i) => ask(i + 1));
		let n = 0;
		const get = async () => (n++, { asks: page });
		expect(await allWaiting(get, () => true)).toHaveLength(500);
		expect(n).toBe(2);
	});
});
