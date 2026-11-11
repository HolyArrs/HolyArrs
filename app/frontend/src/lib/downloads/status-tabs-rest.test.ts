import { TABS, counts, moreOf, restSum, restTotal, subCounts, withRest, type Rest } from './status-tabs.ts';
import { describe, expect, it } from 'vitest';

type R = { phase: string; state: string; progress: number };
const seeding = (n: number): R[] => Array.from({ length: n }, () => ({ phase: 'done', state: 'seeding', progress: 1 }));
const count = (c: { key: string; count: number }[], key: string) => c.find((x) => x.key === key)?.count;

describe('tab counts with torrents the page has not loaded', () => {
	it('adds the rest the server reports to the loaded rows, so All says every torrent', () => {
		const rest: Rest = { '1': { protocol: 'torrent', stages: { seeding: 682, downloading: 1 } } };
		const c = withRest(counts(seeding(200), TABS), TABS, restSum(rest));
		expect(count(c, 'active')).toBe(883);
		expect(count(c, 'done')).toBe(882);
		expect(count(c, 'progress')).toBe(1);
		expect(count(c, 'waiting')).toBe(0);
	});

	it('keeps counting loaded rows live: one that moves stage moves tabs, with no count left behind', () => {
		const rest: Rest = { '1': { protocol: 'torrent', stages: { seeding: 682 } } };
		const loaded: R[] = [...seeding(199), { phase: 'downloading', state: 'downloading', progress: 0.3 }];
		const before = withRest(counts(loaded, TABS), TABS, restSum(rest));
		loaded[199] = { phase: 'done', state: 'seeding', progress: 1 };
		const after = withRest(counts(loaded, TABS), TABS, restSum(rest));
		expect(count(before, 'progress')).toBe(1);
		expect(count(after, 'progress')).toBe(0);
		expect(count(after, 'done')).toBe((count(before, 'done') ?? 0) + 1);
		expect(count(after, 'active')).toBe(count(before, 'active'));
	});

	it('sums every Downloader and gives sub-tabs their share', () => {
		const extra = restSum({ '1': { protocol: 'torrent', stages: { downloading: 2, stalled: 3 } }, '2': { protocol: 'usenet', stages: { downloading: 1, queued: 4 } } });
		expect(extra).toEqual({ downloading: 3, stalled: 3, queued: 4 });
		expect(restTotal(extra)).toBe(10);
		const progress = TABS.find((t) => t.key === 'progress');
		const subs = withRest(subCounts([], progress), progress?.subs ?? [], extra);
		expect(count(subs, 'downloading')).toBe(6);
		expect(count(subs, 'checking')).toBe(0);
	});

	it('changes nothing when the page holds every torrent', () => {
		const loaded = seeding(5);
		expect(withRest(counts(loaded, TABS), TABS, restSum({}))).toEqual(counts(loaded, TABS));
		expect(restTotal(restSum({}))).toBe(0);
	});

	it('follows the source filter: Torrent offers more while torrents are left, Usenet never does when none are', () => {
		const rest: Rest = { '1': { protocol: 'torrent', stages: { seeding: 682, downloading: 1 } } };
		const torrent = moreOf({ rest, src: 'torrent', partial: false, loaded: 200, canMore: true });
		expect(torrent.more).toBe(true);
		expect(torrent.total).toBe(883);
		const usenet = moreOf({ rest, src: 'usenet', partial: false, loaded: 0, canMore: true });
		expect(usenet.more).toBe(false);
		expect(usenet.left).toBe(0);
		expect(usenet.extra).toEqual({});
		// Back to Torrent (and to All): the offer is there again, with the same numbers.
		expect(moreOf({ rest, src: 'torrent', partial: false, loaded: 200, canMore: true })).toEqual(torrent);
		expect(moreOf({ rest, src: '', partial: false, loaded: 200, canMore: true }).more).toBe(true);
	});

	it('keeps the note but stops adding to the counts under another filter, and says capped at the limit', () => {
		const rest: Rest = { '1': { protocol: 'torrent', stages: { seeding: 10 } } };
		const searched = moreOf({ rest, src: '', partial: true, loaded: 2000, canMore: true });
		expect(searched.extra).toEqual({});
		expect(searched.more).toBe(true);
		const atCap = moreOf({ rest, src: '', partial: false, loaded: 2000, canMore: false });
		expect(atCap.more).toBe(false);
		expect(atCap.capped).toBe(true);
	});
});
