import { applyPatch, fromAnswer, isDl, mergeClient, wentStale, type DlHealth, type DlPatch } from './dl-store.ts';
import { describe, expect, it } from 'vitest';
import type { DownloadRow } from '../core/types.ts';

const row = (over: Partial<DownloadRow>): DownloadRow =>
	({ id: 1, phase: 'downloading', state: 'downloading', label: '', text: '', progress: 0, speed: 0, active: true, name: 'x', title: '', version: '', downloader: 'Q', size: 1, updated_at: '', can_act: true, ...over }) as DownloadRow;

const own = row({ id: 7, live_key: '3:aa', progress: 0.1, eta: 50 });
const outside = row({ id: -1_000_000_000, live_key: '3:bb', client_item: true, outside: true, hash: 'bb', downloader_id: 3 });
const other = row({ id: 8, live_key: '4:cc' });
const sync = { boot: 'B', revs: { '3': 10, '4': 2 } };
const patch = (over: Partial<DlPatch>): DlPatch => ({ kind: 'patch', boot: 'B', client: 3, fromRev: 10, toRev: 11, ...over });

describe('dl push store', () => {
	it('reads revs from a view answer; no revs means the server does not push', () => {
		expect(fromAnswer({ downloads: [] } as never)).toBeNull();
		expect(fromAnswer({ boot: 'B', revs: { '3': 1 } })).toEqual({ boot: 'B', revs: { '3': 1 } });
	});

	it('applies a patch on top of its rev: changed fields only, null clears, rev moves for that client only', () => {
		const out = applyPatch([own, outside, other], sync, patch({ upsert: [{ key: '3:aa', progress: 0.5, eta: null }], totals: { speed: 1, up_speed: 9 } }));
		expect(out.kind).toBe('applied');
		if (out.kind !== 'applied') return;
		expect(out.rows[0]).toMatchObject({ id: 7, progress: 0.5, name: 'x' });
		expect('eta' in out.rows[0]).toBe(false);
		expect(out.rows[1]).toBe(outside);
		expect(out.sync.revs).toEqual({ '3': 11, '4': 2 });
		expect(out.upTotal).toBe(9);
	});

	it('drops a patch the snapshot already holds', () => {
		expect(applyPatch([own], sync, patch({ fromRev: 9, toRev: 10 })).kind).toBe('dropped');
		expect(applyPatch([own], sync, patch({ fromRev: 5, toRev: 8 })).kind).toBe('dropped');
	});

	it('a gap for one client asks for a read; other clients are unaffected', () => {
		expect(applyPatch([own], sync, patch({ fromRev: 11, toRev: 12 })).kind).toBe('reread');
		// A window that starts before the page's rev but ends after it cannot be laid on top either.
		expect(applyPatch([own], sync, patch({ fromRev: 9, toRev: 12 })).kind).toBe('reread');
		expect(applyPatch([own, other], sync, patch({ client: 4, fromRev: 2, toRev: 3, upsert: [{ key: '4:cc', speed: 5 }] })).kind).toBe('applied');
	});

	it('a client never seen counts from rev 0', () => {
		expect(applyPatch([], sync, patch({ client: 9, fromRev: 0, toRev: 1 })).kind).toBe('applied');
		expect(applyPatch([], sync, patch({ client: 9, fromRev: 1, toRev: 2 })).kind).toBe('reread');
	});

	it('a new server boot means a read (resync)', () => {
		expect(applyPatch([own], sync, patch({ boot: 'C' })).kind).toBe('reread');
	});

	it('adds a new client torrent from a whole row and removes gone ones; own rows are never removed by a patch', () => {
		const ids: string[] = [];
		const out = applyPatch([own, outside], sync, patch({
			upsert: [{ key: '3:dd', client_item: true, outside: true, name: 'New', hash: 'dd', downloader_id: 3 }, { key: '3:ee', progress: 1 }],
			remove: ['3:bb', '3:aa']
		}), (r) => (ids.push(r.live_key!), { ...r, id: -5 }));
		if (out.kind !== 'applied') throw new Error(out.kind);
		expect(out.rows.map((r) => r.live_key)).toEqual(['3:aa', '3:dd']);
		expect(out.rows[1]).toMatchObject({ id: -5, name: 'New' });
		expect(ids).toEqual(['3:dd']);
	});

	it('recognises dl messages', () => {
		expect(isDl({ kind: 'patch', client: 1 })).toBe(true);
		expect(isDl({ kind: 'health', client: 1 })).toBe(true);
		expect(isDl({ kind: 'x', client: 1 })).toBe(false);
		expect(isDl(null)).toBe(false);
	});

	it('a gap names its client; that client page replaces only its rows and rev', () => {
		const ownA = row({ id: 7, live_key: '3:aa', downloader_id: 3, progress: 0.1 });
		const ownB = row({ id: 8, live_key: '4:cc', downloader_id: 4, progress: 0.2 });
		const gap = applyPatch([ownA, ownB, outside], sync, patch({ fromRev: 12, toRev: 13 }));
		expect(gap).toEqual({ kind: 'reread', client: 3 });
		const fresh = [row({ id: 7, live_key: '3:aa', downloader_id: 3, progress: 0.9 }), row({ id: -5, live_key: '3:dd', client_item: true, downloader_id: 3, hash: 'dd' })];
		const out = mergeClient([ownA, ownB, outside], sync, 3, fresh, { '3': 13 });
		expect(out.sync.revs).toEqual({ '3': 13, '4': 2 });
		expect(out.rows.map((r) => r.live_key)).toEqual(['4:cc', '3:aa', '3:dd']);
		expect(out.rows[1].progress).toBe(0.9);
		expect(out.rows[0]).toBe(ownB);
	});
});

describe('wentStale', () => {
	const h = (stale?: boolean): DlHealth => ({ kind: 'health', client: 1, health: stale ? 'offline' : 'ok', stale });
	it('is true only when a client turns stale', () => {
		expect(wentStale(undefined, h(true))).toBe(true);
		expect(wentStale({ stale: false }, h(true))).toBe(true);
		expect(wentStale({ stale: true }, h(true))).toBe(false);
		expect(wentStale({ stale: true }, h(false))).toBe(false);
	});
});
