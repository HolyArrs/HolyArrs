import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '#lib/core/api.ts';
import { CUT_FRESH_MS, ResultUnknown, adoptInParts, adoptParts, cutFromJobs, cutKeyOf, askThrough, cutNote, cutsGone, findCut, findKept, foldersThenNotes, heldLocks, holdWhile, notesThenLocks, scansGone, jobForgotten, keepAsking, partEnd, partMost, quiet, quietAfter, scanEnd, scanFromFirstLeft, staleCuts, throughJobs, throughNow } from './adopt-parts.ts';
import { followJob, type JobView } from './organise.ts';

const limit = 1 << 16; // the server's size limit for one request
const body = (folders: unknown[]) => new TextEncoder().encode(JSON.stringify({ folders, follow: true })).length;
const movies = (n: number, name = (i: number) => `Some Movie Title ${i} (2001)`) => Array.from({ length: n }, (_, i) => ({ folder: name(i), tmdb_id: String(100000 + i) }));

describe('adoptParts', () => {
	it('keeps every part of a big selection under the size limit', () => {
		const picks = movies(4879);
		expect(body(picks)).toBeGreaterThan(limit); // one request for all of them is refused
		const parts = adoptParts(picks);
		expect(parts.flat()).toEqual(picks);
		for (const p of parts) expect(body(p)).toBeLessThan(limit / 2 + 64);
		expect(Math.max(...parts.map((p) => p.length))).toBe(partMost);
	});

	it('makes smaller parts for long folder names with odd characters', () => {
		const long = (i: number) => `${'Ünïcödé "quoted" \\ name '.repeat(9)}${i}`.slice(0, 250);
		const picks = movies(1200, long);
		const parts = adoptParts(picks);
		expect(parts.flat()).toEqual(picks);
		for (const p of parts) expect(body(p)).toBeLessThan(limit / 2 + 64);
		expect(parts[0].length).toBeLessThan(partMost);
	});

	it('keeps the files of Import files by hand under the limit and the server count of 500', () => {
		const items = Array.from({ length: 2000 }, (_, i) => ({ path: `/media/downloads/Some.Show.S01E${i}.1080p.WEB-DL.x264-GROUP/Some.Show.S01E${i}.1080p.WEB-DL.x264-GROUP.mkv`, slot_id: 100000 + i, title_id: 4500, version_id: 6700, season: 1, episode: i, mode: 'move', replace: false }));
		expect(new TextEncoder().encode(JSON.stringify({ items: items.slice(0, 300) })).length).toBeGreaterThan(limit); // one request is refused
		const parts = adoptParts(items);
		expect(parts.flat()).toEqual(items);
		for (const p of parts) {
			expect(new TextEncoder().encode(JSON.stringify({ items: p })).length).toBeLessThan(limit / 2 + 64);
			expect(p.length).toBeLessThanOrEqual(500);
		}
	});

	it('gives no parts for no picks, and one for one', () => {
		expect(adoptParts([])).toEqual([]);
		expect(adoptParts(movies(1))).toHaveLength(1);
	});
});

const words = { unknown: 'sent, result not known', notSent: 'not sent: the server stopped answering' };
const ok = async (part: { folder: string }[]) => part.map((p, i) => ({ folder: p.folder, title_id: i + 1 }));

describe('adoptInParts', () => {
	it('sends the parts in turn, tells progress and goes on past a refused part with its real reason', async () => {
		const picks = movies(600);
		const sent: number[] = [];
		const seen: [number, number, number][] = [];
		let n = 0;
		const out = await adoptInParts(
			picks,
			async (part) => {
				sent.push(part.length);
				if (n++ === 1) throw new ApiError('That folder name is not allowed.', 400);
				return ok(part);
			},
			words,
			(done, total, part) => seen.push([done, total, part])
		);
		expect(sent).toEqual([250, 250, 100]);
		expect(seen).toEqual([[0, 600, 250], [250, 600, 250], [500, 600, 100], [600, 600, 0]]);
		expect(out.filter((r) => r.title_id)).toHaveLength(350);
		const bad = out.filter((r) => r.error);
		expect(bad).toHaveLength(250);
		expect(bad.every((r) => r.error === 'That folder name is not allowed.' && r.left && !r.notKnown)).toBe(true);
	});

	it('says a part the server took but could not report on is not known, not failed, and sends no more', async () => {
		const picks = movies(600);
		let n = 0;
		const out = await adoptInParts(picks, async (part) => {
			if (n++ === 1) throw new ResultUnknown('HolyArrs is not answering.');
			return ok(part);
		}, words);
		expect(n).toBe(2);
		expect(out).toHaveLength(600);
		expect(out.slice(250, 500).every((r) => r.notKnown && r.left && r.error === words.unknown)).toBe(true);
		expect(out.slice(500).every((r) => !r.notKnown && r.left && r.error === words.notSent)).toBe(true);
	});

	it('stops at once when the person is signed out or no answer came at all, with the real reason', async () => {
		for (const status of [401, 0]) {
			let n = 0;
			const out = await adoptInParts(movies(600), async () => {
				n++;
				throw new ApiError(`reason ${status}`, status);
			}, words);
			expect(n).toBe(1);
			expect(out).toHaveLength(600);
			expect(out.every((r) => r.error === `reason ${status}` && r.left)).toBe(true);
		}
	});

	it("stops at once on a proxy's own page while HolyArrs is down, and says in plain words that the rest was not sent, never the proxy's text", async () => {
		const pages = [
			new ApiError('404 page not found', 404, { error: '404 page not found' }),
			new ApiError('<html><head><title>502 Bad Gateway</title></head></html>', 502, { error: '<html><head><title>502 Bad Gateway</title></head></html>' }),
			new ApiError('Service Unavailable', 503, { error: 'Service Unavailable' }),
			new ApiError('Gateway Timeout', 504, { error: 'Gateway Timeout' })
		];
		for (const page of pages) {
			let n = 0;
			const out = await adoptInParts(movies(600), async () => {
				n++;
				throw page;
			}, words);
			expect(n, String(page.status)).toBe(1);
			expect(out).toHaveLength(600);
			expect(out.every((r) => r.error === words.notSent && r.left && !r.notKnown), String(page.status)).toBe(true);
		}
	});

	it('still sends the rest after a part HolyArrs itself refused with a 404 of its own', async () => {
		let n = 0;
		const out = await adoptInParts(movies(600), async (part) => {
			if (n++ === 0) throw new ApiError('no such job', 404, { error: 'no such job' });
			return ok(part);
		}, words);
		expect(n).toBe(3);
		expect(out.filter((r) => r.title_id)).toHaveLength(350);
	});

	it('sends nothing more once the page is gone', async () => {
		let n = 0;
		let gone = false;
		await adoptInParts(movies(600), async (part) => {
			n++;
			gone = true;
			return ok(part);
		}, words, () => {}, () => gone);
		expect(n).toBe(1);
	});
});

describe('partEnd, the end of one part job', () => {
	const why = (m?: string) => m || 'unfinished';
	const res = [{ folder: 'A (2001)', title_id: 7 }];
	it('hands back the results of a finished part, with nothing gone wrong', () => {
		expect(partEnd({ State: 'done', Result: { results: res } }, why)).toEqual({ res, fail: '' });
		expect(partEnd({ State: 'done' }, why)).toEqual({ res: [], fail: '' });
	});
	it('keeps the results of a part whose Follow step failed after the folders went in, and still says what went wrong', () => {
		expect(partEnd({ State: 'failed', Result: { results: res }, Error: { code: 'x', message: 'database is locked' } }, why)).toEqual({ res, fail: 'database is locked' });
		expect(partEnd({ State: 'failed', Result: { results: res }, Error: null }, why)).toEqual({ res, fail: 'unfinished' });
	});
	it('throws what went wrong when a part failed with no results', () => {
		expect(() => partEnd({ State: 'failed', Error: { code: 'x', message: 'no movie Library folder' } }, why)).toThrow('no movie Library folder');
		expect(() => partEnd({ State: 'cancelled' }, why)).toThrow('unfinished');
	});
});

describe('keepAsking', () => {
	it('asks again through a short outage', async () => {
		let n = 0;
		const v = await keepAsking(async () => {
			if (n++ < 3) throw new ApiError('not answering', 0);
			return 'done';
		}, () => false, [0, 0, 0, 0]);
		expect(v).toBe('done');
		expect(n).toBe(4);
	});
	it('gives up as not known after a few tries, and at once when signed out', async () => {
		let n = 0;
		await expect(keepAsking(async () => { n++; throw new ApiError('not answering', 0); }, () => false, [0, 0])).rejects.toBeInstanceOf(ResultUnknown);
		expect(n).toBe(3);
		n = 0;
		await expect(keepAsking(async () => { n++; throw new ApiError('signed out', 401); }, () => false, [0, 0])).rejects.toBeInstanceOf(ResultUnknown);
		expect(n).toBe(1);
	});
	it("asks again through a proxy's plain 404 while HolyArrs restarts, and gives up at once only when HolyArrs forgot the job", async () => {
		let n = 0;
		const v = await keepAsking(async () => {
			if (n++ < 2) throw new ApiError('Request failed (404)', 404, { error: '404 page not found' });
			return 'done';
		}, () => false, [0, 0, 0, 0]);
		expect(v).toBe('done');
		expect(n).toBe(3);
		n = 0;
		await expect(keepAsking(async () => { n++; throw new ApiError('no such job', 404, { error: 'no such job' }); }, () => false, [0, 0])).rejects.toBeInstanceOf(ResultUnknown);
		expect(n).toBe(1);
	});
});

describe('scanFromFirstLeft', () => {
	const order = ['A', 'B', 'C', 'D', 'E'];
	it('starts the scan just before the first folder that did not go through', () => {
		expect(scanFromFirstLeft(order, [{ folder: 'A', title_id: 1 }, { folder: 'D', left: true }, { folder: 'C', left: true }], '')).toBe('B');
	});
	it('starts where the scan began when the first folder did not go through, and is null when all did', () => {
		expect(scanFromFirstLeft(order, [{ folder: 'A', left: true }], 'Z0')).toBe('Z0');
		expect(scanFromFirstLeft(order, [{ folder: 'A', title_id: 1 }], '')).toBeNull();
	});
});

describe('throughNow', () => {
	it('moves inside a part', () => {
		expect(throughNow({ done: 500, total: 4500 }, null)).toBe(500);
		expect(throughNow({ done: 500, total: 4500 }, { done: 120 })).toBe(620);
		expect(throughNow({ done: 4400, total: 4500 }, { done: 250 })).toBe(4500);
	});
});

describe('an import cut short', () => {
	const done = (n: number) => ({ ID: 1, State: 'done' as const, Progress: 1, Result: { results: Array.from({ length: n }, () => ({})) } });
	it('counts what the server took from the part jobs, not what the page last wrote', () => {
		const running = { ID: 3, State: 'running' as const, Progress: 0.4, ProgressNote: '100/250' };
		expect(throughJobs([done(250), done(250), running, null])).toBe(600);
	});
	it('says nothing when every folder went through', () => {
		expect(cutNote({ lf: 1, total: 228, jobs: [7], why: 'left' }, 228)).toBeNull();
	});
	it('names the real cause in plain words', () => {
		expect(cutNote({ lf: 1, total: 907, jobs: [], why: 'signedOut' }, 500)).toEqual({ key: 'library.import.interruptedSignedOut', done: 500, total: 907 });
		expect(cutNote({ lf: 1, total: 907, jobs: [], why: 'left' }, 250)?.key).toBe('library.import.interruptedLeft');
		expect(cutNote({ lf: 1, total: 907, jobs: [], why: 'closed' }, 250)?.key).toBe('library.import.interrupted');
	});
});

describe('quiet', () => {
	it('says HolyArrs is not answering only after a few seconds without an answer', () => {
		expect(quiet(10_000, 10_000 + quietAfter - 1)).toBe(false);
		expect(quiet(10_000, 10_000 + quietAfter + 1)).toBe(true);
		expect(quiet(0, 99_999)).toBe(false);
	});
});

describe('the record of a running import, one per tab', () => {
	const store = (m: Record<string, string>) => ({ get length() { return Object.keys(m).length; }, key: (i: number) => Object.keys(m)[i] ?? null, getItem: (k: string) => m[k] ?? null });
	const rec = JSON.stringify({ lf: 3, total: 500, jobs: [7], why: 'closed' });
	it('has its own key in each tab', () => {
		expect(cutKeyOf('holyarrs.import.running.4', 'aaa')).not.toBe(cutKeyOf('holyarrs.import.running.4', 'bbb'));
	});
	it("finds a record another tab or an older page left, never another person's", () => {
		expect(findCut(store({ 'holyarrs.import.running.4.aaa': rec }), 'holyarrs.import.running.4')).toEqual({ key: 'holyarrs.import.running.4.aaa', cut: JSON.parse(rec) });
		expect(findCut(store({ 'holyarrs.import.running.4': rec }), 'holyarrs.import.running.4')?.key).toBe('holyarrs.import.running.4');
		expect(findCut(store({ 'holyarrs.import.running.41.aaa': rec, other: 'x' }), 'holyarrs.import.running.4')).toBeNull();
		expect(findCut(store({ 'holyarrs.import.running.4.aaa': 'not json' }), 'holyarrs.import.running.4')).toBeNull();
	});
	it('passes over a note another tab is still keeping fresh: that import is still running', () => {
		const now = 1_000_000;
		const live = JSON.stringify({ ...JSON.parse(rec), at: now - 3_000 });
		const dead = JSON.stringify({ ...JSON.parse(rec), at: now - 200_000 });
		const stopped = JSON.stringify({ ...JSON.parse(rec), at: 0 });
		expect(findCut(store({ 'holyarrs.import.running.4.bbb': live }), 'holyarrs.import.running.4', now)).toBeNull();
		expect(findCut(store({ 'holyarrs.import.running.4.bbb': live, 'holyarrs.import.running.4.ccc': dead }), 'holyarrs.import.running.4', now)?.key).toBe('holyarrs.import.running.4.ccc');
		expect(findCut(store({ 'holyarrs.import.running.4.ddd': stopped }), 'holyarrs.import.running.4', now)?.key).toBe('holyarrs.import.running.4.ddd');
	});
	it('trusts the lock of the sending tab where the browser has locks, however rarely a hidden tab writes', () => {
		const now = 10_000_000;
		const old = JSON.stringify({ ...JSON.parse(rec), at: now - 10 * 60_000 });
		const key = 'holyarrs.import.running.4.bbb';
		expect(findCut(store({ [key]: old }), 'holyarrs.import.running.4', now, new Set([key]))).toBeNull();
		// No lock held for it: that tab is gone, said at once.
		expect(findCut(store({ [key]: JSON.stringify({ ...JSON.parse(rec), at: now - 1_000 }) }), 'holyarrs.import.running.4', now, new Set())?.key).toBe(key);
	});
	it('without locks, a hidden tab that writes only once a minute still counts as running', () => {
		expect(CUT_FRESH_MS).toBeGreaterThanOrEqual(120_000);
		// A browser wakes a tab hidden for over 5 minutes about once a minute, sometimes later.
		const key = 'holyarrs.import.running.4.bbb';
		let at = 0;
		for (let now = 0; now <= 30 * 60_000; now += 1_000) {
			if (now % 65_000 === 0) at = now || 1;
			expect(findCut(store({ [key]: JSON.stringify({ ...JSON.parse(rec), at }) }), 'holyarrs.import.running.4', now, null)).toBeNull();
		}
	});
});

describe('the lock held while a tab sends', () => {
	it('is held until it is let go, and the page goes on where there are no locks', async () => {
		const names: string[] = [];
		let done: Promise<unknown> = Promise.resolve();
		const locks = { request: (name: string, cb: () => Promise<void>) => { names.push(name); done = cb(); return done; } };
		let freed = false;
		const free = holdWhile(locks, 'holyarrs.import.running.4.aaa');
		void done.then(() => (freed = true));
		await Promise.resolve();
		expect(names).toEqual(['holyarrs.import.running.4.aaa']);
		expect(freed).toBe(false);
		free();
		await done;
		expect(freed).toBe(true);
		expect(() => holdWhile(undefined, 'x')()).not.toThrow();
	});
	it('can be seen from another tab, and says nothing where there are no locks', async () => {
		expect(await heldLocks({ query: async () => ({ held: [{ name: 'a' }, { name: 'b' }], pending: [] }) })).toEqual(new Set(['a', 'b']));
		expect(await heldLocks(undefined)).toBeNull();
		expect(await heldLocks({ query: async () => { throw new Error('no'); } })).toBeNull();
	});
	it('counts a lock still waited for as held, since its tab asks for it before writing the note', async () => {
		expect(await heldLocks({ query: async () => ({ held: [{ name: 'a' }], pending: [{ name: 'b' }] }) })).toEqual(new Set(['a', 'b']));
		expect(await heldLocks({ query: async () => ({ pending: [{ name: 'b' }] }) })).toEqual(new Set(['b']));
	});
	it('reads the notes before asking for the locks, so a tab that starts in between is never called stopped', async () => {
		const base = 'holyarrs.import.running.4';
		const key = `${base}.bbb`;
		const m: Record<string, string> = {};
		const store = { get length() { return Object.keys(m).length; }, key: (i: number) => Object.keys(m)[i] ?? null, getItem: (k: string) => m[k] ?? null };
		// The other tab asks for its lock and writes its note right after this tab's question is answered.
		const held = new Set<string>();
		const locks = {
			query: async () => {
				const r = { held: [...held].map((name) => ({ name })), pending: [] };
				held.add(key);
				m[key] = JSON.stringify({ lf: 3, total: 534, jobs: [], why: 'closed', at: 5 });
				return r;
			}
		};
		const got = await notesThenLocks(store, base, locks);
		expect(findCut(got.store, base, 10, got.held)).toBeNull();
		// Asked again later, the lock is seen and the note is still left alone.
		const again = await notesThenLocks(store, base, locks);
		expect(findCut(again.store, base, 10, again.held)).toBeNull();
		// Its tab gone (lock let go, note marked stopped): now it is a note to show.
		held.clear();
		m[key] = JSON.stringify({ lf: 3, total: 534, jobs: [], why: 'closed', at: 0 });
		expect(findCut((await notesThenLocks(store, base, locks)).store, base, 10, new Set())?.key).toBe(key);
	});
	it('reads the notes after a slow folder load, so a tab still sending is never called stopped where there are no locks', async () => {
		const base = 'holyarrs.import.running.4';
		const key = `${base}.bbb`;
		let clock = 1_000_000;
		const m: Record<string, string> = { [key]: JSON.stringify({ lf: 3, total: 534, jobs: [], why: 'closed', at: clock - 1_000 }) };
		const store = { get length() { return Object.keys(m).length; }, key: (i: number) => Object.keys(m)[i] ?? null, getItem: (k: string) => m[k] ?? null };
		// The folders take longer than a note stays fresh; the other tab writes its note again meanwhile.
		const slowLoad = async () => {
			clock += CUT_FRESH_MS + 60_000;
			m[key] = JSON.stringify({ lf: 3, total: 534, jobs: [], why: 'closed', at: clock - 1_000 });
			return true;
		};
		const got = await foldersThenNotes(slowLoad, () => store, base, undefined, () => clock);
		expect(got && findCut(got.store, base, got.now, got.held)).toBeNull();
		// Folders that did not load: no note is read at all.
		let asked = false;
		expect(await foldersThenNotes(async () => false, () => ((asked = true), store), base, undefined, () => clock)).toBeNull();
		expect(asked).toBe(false);
		// No storage: nothing read, nothing thrown.
		expect(await foldersThenNotes(async () => true, () => { throw new Error('no storage'); }, base, undefined, () => clock)).toBeNull();
	});
});

describe('old notes of an import cut short', () => {
	const store = (m: Record<string, string>) => ({ get length() { return Object.keys(m).length; }, key: (i: number) => Object.keys(m)[i] ?? null, getItem: (k: string) => m[k] ?? null });
	const note = (lf: number, at = 0) => JSON.stringify({ lf, total: 500, jobs: [7], why: 'closed', at });
	it('are cleared once a later import of the same folder finished, never one still running', () => {
		const now = 10_000_000;
		const m = { 'holyarrs.import.running.4.aaa': note(3), 'holyarrs.import.running.4.bbb': note(3, now - 1_000), 'holyarrs.import.running.4.ccc': note(5), 'holyarrs.import.running.41.ddd': note(3) };
		expect(staleCuts(store(m), 'holyarrs.import.running.4', 3, now, new Set(['holyarrs.import.running.4.bbb']))).toEqual(['holyarrs.import.running.4.aaa']);
		expect(staleCuts(store(m), 'holyarrs.import.running.4', 3, now, null)).toEqual(['holyarrs.import.running.4.aaa']);
	});
	it('never say 0 of N when the server no longer knows a part', () => {
		const c = { lf: 1, total: 900, jobs: [7, 8], why: 'closed' as const };
		const done = { ID: 7, State: 'done' as const, Progress: 1, Result: { results: Array.from({ length: 250 }, () => ({})) } };
		expect(cutFromJobs(c, [done, null])).toBeNull();
		expect(cutFromJobs(c, [null, null])).toBeNull();
		expect(cutFromJobs(c, [done, done])).toEqual({ key: 'library.import.interrupted', done: 500, total: 900 });
		// Nothing was taken before it stopped: that is known, and said.
		expect(cutFromJobs({ ...c, jobs: [] }, [])).toEqual({ key: 'library.import.interrupted', done: 0, total: 900 });
	});
	it('are kept, and say nothing this time, when the server did not answer for a part', () => {
		const c = { lf: 1, total: 900, jobs: [7, 8], why: 'closed' as const };
		const done = { ID: 7, State: 'done' as const, Progress: 1, Result: { results: Array.from({ length: 250 }, () => ({})) } };
		expect(cutFromJobs(c, [done, undefined])).toBeUndefined();
		expect(cutFromJobs(c, [undefined, undefined])).toBeUndefined();
		// Not answered beats forgotten: nothing is dropped on a moment without an answer.
		expect(cutFromJobs(c, [null, undefined])).toBeUndefined();
	});
});

describe('stopped notes of a Library folder that is gone', () => {
	const store = (m: Record<string, string>) => ({ get length() { return Object.keys(m).length; }, key: (i: number) => Object.keys(m)[i] ?? null, getItem: (k: string) => m[k] ?? null });
	const note = (lf: number, at = 0) => JSON.stringify({ lf, total: 500, jobs: [7], why: 'closed', at });
	it("are cleared, never one still running, and never hide another folder's note", () => {
		const now = 10_000_000;
		const base = 'holyarrs.import.running.4';
		const m = { [`${base}.aaa`]: note(9), [`${base}.bbb`]: note(9, now - 1_000), [`${base}.ccc`]: note(3), 'holyarrs.import.running.41.ddd': note(9) };
		const held = new Set([`${base}.bbb`]);
		expect(cutsGone(store(m), base, [3, 4], now, held)).toEqual([`${base}.aaa`]);
		expect(cutsGone(store(m), base, [3, 4], now, null)).toEqual([`${base}.aaa`]);
		expect(cutsGone(store(m), base, [3, 9], now, held)).toEqual([]);
		expect(findCut(store(m), base, now, held, [3, 4])?.key).toBe(`${base}.ccc`);
		expect(findCut(store(m), base, now, held, [4])).toBeNull();
		// Without the folders, every stopped note is found as before.
		expect(findCut(store(m), base, now, held)?.key).toBe(`${base}.aaa`);
	});
});

describe('a part of a stopped import the server forgot', () => {
	it("is only a 404 in HolyArrs's own words, never a proxy's plain 404 page", () => {
		expect(jobForgotten(new ApiError('no such job', 404, { error: 'no such job' }))).toBe(true);
		expect(jobForgotten(new ApiError('Request failed (404)', 404, { error: '404 page not found' }))).toBe(false);
		expect(jobForgotten(new ApiError('Request failed (404)', 404))).toBe(false);
		expect(jobForgotten(new ApiError('no such job', 502, { error: 'no such job' }))).toBe(false);
		expect(jobForgotten(new Error('no such job'))).toBe(false);
	});
});

describe('a scan the page remembers, one per tab', () => {
	const store = (m: Record<string, string>) => ({ get length() { return Object.keys(m).length; }, key: (i: number) => Object.keys(m)[i] ?? null, getItem: (k: string) => m[k] ?? null });
	it("is found under the person's base, whichever tab wrote it", () => {
		const ok = (v: { job?: unknown }) => typeof v.job === 'number';
		expect(findKept(store({ 'holyarrs.import.scan.4.aaa': '{"lf":3,"job":9,"from":""}' }), 'holyarrs.import.scan.4', ok)).toEqual({ key: 'holyarrs.import.scan.4.aaa', v: { lf: 3, job: 9, from: '' } });
		expect(findKept(store({ 'holyarrs.import.scan.4': '{"lf":3,"job":9}' }), 'holyarrs.import.scan.4', ok)?.key).toBe('holyarrs.import.scan.4');
		expect(findKept(store({ 'holyarrs.import.scan.41.a': '{"lf":3,"job":9}', 'holyarrs.import.scan.4.b': 'x' }), 'holyarrs.import.scan.4', ok)).toBeNull();
	});
	it("of a Library folder that is gone is cleared, and never hides another tab's scan", () => {
		const m = { 'holyarrs.import.scan.4.aaa': '{"lf":9,"job":5}', 'holyarrs.import.scan.4.bbb': '{"lf":3,"job":14}', 'holyarrs.import.scan.4.ccc': 'x', 'holyarrs.import.scan.41.ddd': '{"lf":9,"job":6}' };
		expect(scansGone(store(m), 'holyarrs.import.scan.4', [3, 4])).toEqual(['holyarrs.import.scan.4.aaa']);
		expect(scansGone(store(m), 'holyarrs.import.scan.4', [3, 9])).toEqual([]);
		const ids = [3, 4];
		const ok = (v: { lf?: unknown; job?: unknown }) => typeof v.job === 'number' && ids.includes(v.lf as number);
		expect(findKept(store(m), 'holyarrs.import.scan.4', ok)?.key).toBe('holyarrs.import.scan.4.bbb');
	});
});

describe('askThrough, for a scan that is being watched', () => {
	it('asks again when no answer comes at all, so a network that drops for a moment does not lose the scan', async () => {
		let n = 0;
		const v = await askThrough(async () => {
			if (n++ < 2) throw new ApiError('not answering', 0);
			return 'plan';
		}, () => false, [0, 0, 0]);
		expect(v).toBe('plan');
		expect(n).toBe(3);
	});
	it('hands a real answer straight back, also a refusal, and stops when the page is gone', async () => {
		let n = 0;
		const gone404 = new ApiError('no such job', 404, { error: 'no such job' });
		await expect(askThrough(async () => { n++; throw gone404; }, () => false, [0, 0])).rejects.toBe(gone404);
		expect(n).toBe(1);
		n = 0;
		const refused = new ApiError('That folder name is not allowed.', 400, { error: 'That folder name is not allowed.' });
		await expect(askThrough(async () => { n++; throw refused; }, () => false, [0, 0])).rejects.toBe(refused);
		expect(n).toBe(1);
		n = 0;
		await expect(askThrough(async () => { n++; throw new ApiError('not answering', 0); }, () => true, [0, 0])).rejects.toBeInstanceOf(ApiError);
		expect(n).toBe(1);
		n = 0;
		await expect(askThrough(async () => { n++; throw new ApiError('not answering', 0); }, () => false, [0, 0])).rejects.toBeInstanceOf(ApiError);
		expect(n).toBe(3);
	});
	it("asks again through a proxy's own page while HolyArrs restarts: its plain 404, a 502, 503 or 504", async () => {
		for (const status of [404, 502, 503, 504]) {
			let n = 0;
			const v = await askThrough(async () => {
				if (n++ < 2) throw new ApiError('proxy page', status, { error: status === 404 ? '404 page not found' : 'Bad Gateway' });
				return 'plan';
			}, () => false, [0, 0, 0]);
			expect(v, String(status)).toBe('plan');
			expect(n).toBe(3);
		}
	});
});

describe('askThrough over a long scan', () => {
	const blip = () => new ApiError('Bad Gateway', 502, { error: 'Bad Gateway' });
	afterEach(() => vi.useRealTimers());
	it('keeps watching through many short blips spread over a long watch, each after answers came, and ends with the plan', async () => {
		vi.useFakeTimers();
		let n = 0;
		// Each try follows the scan for a minute (answers coming) before one moment without an answer.
		const p = askThrough(async () => {
			if (n++ < 7) {
				await new Promise((res) => setTimeout(res, 60_000));
				throw blip();
			}
			return 'plan';
		});
		const end = expect(p).resolves.toBe('plan');
		await vi.runAllTimersAsync();
		await end;
		expect(n).toBe(8);
	});
	it('still gives up after the same few tries when the blips come one after another', async () => {
		vi.useFakeTimers();
		let n = 0;
		const p = askThrough(async () => {
			n++;
			throw blip();
		});
		const end = expect(p).rejects.toBeInstanceOf(ApiError);
		await vi.runAllTimersAsync();
		await end;
		expect(n).toBe(6);
	});
});

describe('a watched scan while HolyArrs restarts behind a proxy', () => {
	const proxy404 = () => new ApiError('404 page not found', 404, { error: '404 page not found' });
	it('keeps watching through the restart and ends with the plan, as the page does', async () => {
		// HolyArrs puts a scan that was running back in the queue when it starts again, under the same job.
		const answers: (JobView | ApiError)[] = [
			{ ID: 9, State: 'running', Progress: 0.3, ProgressNote: '120/600' },
			proxy404(),
			new ApiError('Bad Gateway', 502, { error: 'Bad Gateway' }),
			new ApiError('HolyArrs is starting', 503, { error: 'HolyArrs is starting' }),
			{ ID: 9, State: 'running', Progress: 0.1, ProgressNote: 'restarted after a stop' },
			{ ID: 9, State: 'done', Progress: 1, Result: { plan: { folders: [] } } }
		];
		let asked = 0;
		const get = async () => {
			const a = answers[Math.min(asked++, answers.length - 1)];
			if (a instanceof ApiError) throw a;
			return a;
		};
		const end = await askThrough(() => followJob(get, 9, () => {}, { every: 0 }), () => false, [0, 0, 0, 0, 0]);
		expect(end.State).toBe('done');
		expect(asked).toBe(6);
	});
	it('leaves the scan remembered and a plain line when HolyArrs stays away, never the proxy text', () => {
		const plain = 'HolyArrs is not answering. Check that it is running.';
		for (const e of [proxy404(), new ApiError('<html>502</html>', 502, { error: '<html>502</html>' }), new ApiError('x', 503), new ApiError('x', 504), new ApiError(plain, 0)]) {
			expect(scanEnd(e, false, plain), String(e.status)).toEqual({ error: plain, keep: true });
			expect(scanEnd(e, true, plain), String(e.status)).toEqual({ error: plain, keep: true });
		}
	});
	it('forgets a scan HolyArrs says it no longer has, quietly when it was picked up on the way back', () => {
		const forgot = new ApiError('no such job', 404, { error: 'no such job' });
		expect(scanEnd(forgot, true, 'plain')).toEqual({ error: '', keep: false });
		expect(scanEnd(forgot, false, 'plain')).toEqual({ error: 'no such job', keep: false });
		expect(scanEnd(new Error('The scan did not finish.'), true, 'plain')).toEqual({ error: 'The scan did not finish.', keep: false });
		expect(scanEnd(new ApiError('signed out', 401), false, 'plain')).toEqual({ error: 'signed out', keep: false });
	});
});
