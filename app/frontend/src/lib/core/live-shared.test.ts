import { describe, expect, it, vi } from 'vitest';
import { createLive, type ChannelLike, type LocksLike, type SourceFactory } from './live.svelte.ts';

type Src = { url: string; onopen: null | ((e: Event) => void); onerror: null | ((e: Event) => void); onmessage: null | ((e: MessageEvent) => void); close: () => void; closed: boolean };

function browser() {
	const made: Src[] = [];
	const factory = ((url: string) => {
		const s: Src = { url, onopen: null, onerror: null, onmessage: null, closed: false, close: () => (s.closed = true) };
		made.push(s);
		return s;
	}) as unknown as SourceFactory;

	let held = false;
	const queue: { fn: () => Promise<void>; signal?: AbortSignal; resolve: (v: unknown) => void; reject: (e: unknown) => void }[] = [];
	const next = () => {
		if (held) return;
		const w = queue.shift();
		if (!w) return;
		held = true;
		w.fn().then(() => {
			held = false;
			w.resolve(undefined);
			next();
		});
	};
	const locks: LocksLike = {
		request(_name, opts, fn) {
			return new Promise((resolve, reject) => {
				const w = { fn, signal: opts.signal, resolve, reject };
				opts.signal?.addEventListener('abort', () => {
					const i = queue.indexOf(w);
					if (i >= 0) {
						queue.splice(i, 1);
						reject(new DOMException('aborted', 'AbortError'));
					}
				});
				queue.push(w);
				next();
			});
		}
	};

	const chans = new Set<ChannelLike>();
	const channel = (): ChannelLike => {
		const c: ChannelLike = {
			onmessage: null,
			postMessage(msg) {
				const copy = structuredClone(msg);
				for (const o of chans) if (o !== c) queueMicrotask(() => o.onmessage?.(new MessageEvent('message', { data: copy })));
			},
			close() {
				chans.delete(c);
			}
		};
		chans.add(c);
		return c;
	};
	const posts: string[] = [];
	const post = (v: string) => {
		posts.push(v);
		return Promise.resolve();
	};
	const tab = () => createLive(factory, { locks, channel, post });
	const open = () => made.filter((s) => !s.closed);
	return { made, open, tab, posts };
}

const flush = async () => {
	for (let i = 0; i < 10; i++) await Promise.resolve();
};
const ev = (topic: string, id: string, data: unknown = {}) => new MessageEvent('message', { data: JSON.stringify({ topic, data }), lastEventId: id });

describe('one live stream shared by every tab', () => {
	it('three tabs open exactly one stream; the others get every message over the channel', async () => {
		const b = browser();
		const tabs = [b.tab(), b.tab(), b.tab()];
		const got = tabs.map(() => vi.fn());
		tabs.forEach((t, i) => t.on('dl', got[i]));
		tabs.forEach((t) => t.start());
		await flush();
		expect(b.made).toHaveLength(1);
		expect(tabs.map((t) => t.status.leader)).toEqual([true, false, false]);
		b.made[0].onopen?.(new Event('open'));
		await flush();
		expect(tabs.map((t) => t.status.state)).toEqual(['live', 'live', 'live']);
		b.made[0].onmessage?.(ev('dl', 'B-1', { n: 1 }));
		await flush();
		got.forEach((fn) => expect(fn).toHaveBeenCalledWith({ n: 1 }, 'dl'));
		tabs.forEach((t) => t.stop());
	});

	it('closing the leader hands over: the next tab resumes with ?last= and nothing is missed', async () => {
		const b = browser();
		const [a, c] = [b.tab(), b.tab()];
		const got: unknown[] = [];
		c.on('dl', (d) => got.push(d));
		a.start();
		c.start();
		await flush();
		b.made[0].onopen?.(new Event('open'));
		b.made[0].onmessage?.(ev('dl', 'B-7', 7));
		await flush();
		a.stop();
		await flush();
		expect(b.made[0].closed).toBe(true);
		expect(c.status.leader).toBe(true);
		expect(b.made[1].url).toBe('/live?last=B-7');
		b.made[1].onopen?.(new Event('open'));
		b.made[1].onmessage?.(ev('dl', 'B-8', 8));
		await flush();
		expect(got).toEqual([7, 8]);
		expect(b.open()).toHaveLength(1);
		c.stop();
	});

	it('interest from a follower is posted once, by the leader', async () => {
		const b = browser();
		const [a, c] = [b.tab(), b.tab()];
		a.start();
		c.start();
		await flush();
		c.interest('downloads');
		await flush();
		a.interest('downloads');
		expect(b.posts).toEqual(['downloads']);
		a.stop();
		c.stop();
	});

	it('a follower sees a new boot id as a resync', async () => {
		const b = browser();
		const [a, c] = [b.tab(), b.tab()];
		const re = vi.fn();
		c.on('job', re);
		a.start();
		c.start();
		await flush();
		b.made[0].onmessage?.(ev('job', 'A-1'));
		b.made[0].onmessage?.(ev('job', 'Z-1'));
		await flush();
		expect(re).toHaveBeenCalledWith(null, 'resync');
		a.stop();
		c.stop();
	});

	it('without locks or channel each tab keeps its own stream', () => {
		const made: string[] = [];
		const f = ((u: string) => (made.push(u), { onopen: null, onerror: null, onmessage: null, close() {} })) as unknown as SourceFactory;
		const t1 = createLive(f);
		const t2 = createLive(f);
		t1.start();
		t2.start();
		expect(made).toHaveLength(2);
		t1.stop();
		t2.stop();
	});
});
