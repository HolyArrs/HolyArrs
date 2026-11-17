// A large import goes to the server in parts, one after another, so no
// request comes near the server's size limit for one request (64 KiB).
import { ApiError, message } from '#lib/core/api.ts';
import { scanCount, type AdoptResult, type JobView } from './organise.ts';

/** The most bytes of folders one part carries: half the server's limit, room for the rest of the body. */
export const partBytes = 32 * 1024;
/** The most folders one part carries, so progress moves often. */
export const partMost = 250;

const bytes = (x: unknown) => new TextEncoder().encode(JSON.stringify(x)).length + 1;

/** Splits the picks into parts under maxBytes of JSON and at most most folders each. */
export function adoptParts<T>(picks: T[], maxBytes = partBytes, most = partMost): T[][] {
	const out: T[][] = [];
	let part: T[] = [];
	let size = 0;
	for (const p of picks) {
		const b = bytes(p);
		if (part.length && (size + b > maxBytes || part.length >= most)) {
			out.push(part);
			part = [];
			size = 0;
		}
		part.push(p);
		size += b;
	}
	if (part.length) out.push(part);
	return out;
}

/** A part the server took, whose outcome could not be read: it may well be adopted. */
export class ResultUnknown extends Error {}

/** One folder's outcome on the page: left says it did not surely go through, so a scan should bring it in. */
export type PartResult = AdoptResult & { left?: boolean; notKnown?: boolean };

/** HolyArrs did not answer: no answer at all, or a proxy's own page in its place (a 502, 503 or 504, or its plain 404 while HolyArrs is down). */
export function noAnswer(e: unknown): boolean {
	return e instanceof ApiError && ([0, 502, 503, 504].includes(e.status) || (e.status === 404 && !jobForgotten(e)));
}

// The server is gone, or the person may no longer do this: sending more parts cannot work.
function stops(e: unknown): boolean {
	return e instanceof ResultUnknown || noAnswer(e) || (e instanceof ApiError && (e.status === 401 || e.status === 403));
}

/**
 * One part job's end: its results, and what went wrong when it did not finish ('' when it did).
 * A part that failed after its folders went in (its Follow step) keeps its results; one that
 * failed with no results throws. why turns the server's words into the page's.
 */
export function partEnd(end: Pick<JobView, 'State' | 'Result' | 'Error'>, why: (m?: string) => string): { res: AdoptResult[]; fail: string } {
	const res = end.Result?.results as AdoptResult[] | undefined;
	if (end.State === 'done') return { res: res ?? [], fail: '' };
	const fail = why(end.Error?.message);
	if (!res) throw new Error(fail);
	return { res, fail };
}

/**
 * Asks again after a short wait when an answer does not come (a restart,
 * a network that drops for a moment), a few times, before giving up with
 * ResultUnknown. A signed-out person, a page that is gone, or a part HolyArrs
 * itself says it forgot gives up at once; a proxy's plain 404 while HolyArrs
 * restarts is asked again.
 */
export async function keepAsking<T>(ask: () => Promise<T>, gone: () => boolean = () => false, waits = [3000, 6000, 9000, 12000]): Promise<T> {
	for (let i = 0; ; i++) {
		try {
			return await ask();
		} catch (e) {
			const out = e instanceof ApiError && (e.status === 401 || e.status === 403 || jobForgotten(e));
			if (out || gone() || i >= waits.length) throw new ResultUnknown(message(e));
			await new Promise((res) => setTimeout(res, waits[i]));
		}
	}
}

/**
 * Asks again after a short wait when HolyArrs does not answer (a network that
 * drops for a moment, a proxy's own page while HolyArrs restarts), a few times.
 * Any real answer, also a refusal, comes straight back, and a page that is
 * gone stops asking. A try that ran longer than fresh before it failed got
 * answers first, so the tries are counted from the start again: short
 * blips spread over a long watch never add up to giving up.
 */
export async function askThrough<T>(ask: () => Promise<T>, gone: () => boolean = () => false, waits = [1000, 2000, 4000, 8000, 15000], fresh = 30_000): Promise<T> {
	for (let i = 0; ; i++) {
		const began = Date.now();
		try {
			return await ask();
		} catch (e) {
			if (Date.now() - began > fresh) i = 0;
			if (!noAnswer(e) || gone() || i >= waits.length) throw e;
			await new Promise((res) => setTimeout(res, waits[i]));
		}
	}
}

/**
 * What a watched scan that ended in an error leaves on the page: the line to
 * show ('' for none) and whether its record is kept. HolyArrs puts a scan that
 * was running back in the queue when it starts again, so a scan it did not
 * answer for is kept for the next visit, with plain words, never a proxy's
 * page. A scan HolyArrs forgot, picked up on the way back, is simply gone.
 */
export function scanEnd(e: unknown, back: boolean, plain: string): { error: string; keep: boolean } {
	if (noAnswer(e)) return { error: plain, keep: true };
	return { error: back && jobForgotten(e) ? '' : message(e), keep: false };
}

/**
 * Sends the picks part by part. A part the server refused marks its folders
 * with the real reason and the next part still goes; a part whose outcome is
 * not known says so. When the server stops answering or the person is signed
 * out, the rest is not sent and says why. progress hears how many folders
 * are through and the size of the part being sent; gone stops the sending.
 */
export async function adoptInParts<T extends { folder: string }>(
	picks: T[],
	send: (part: T[]) => Promise<AdoptResult[]>,
	words: { unknown: string; notSent: string },
	progress: (done: number, total: number, part: number) => void = () => {},
	gone: () => boolean = () => false
): Promise<PartResult[]> {
	const out: PartResult[] = [];
	const parts = adoptParts(picks);
	let done = 0;
	progress(0, picks.length, parts[0]?.length ?? 0);
	for (let i = 0; i < parts.length && !gone(); i++) {
		const part = parts[i];
		try {
			out.push(...(await send(part)));
		} catch (e) {
			const notKnown = e instanceof ResultUnknown;
			// A proxy's own page in place of HolyArrs's answer is no reason a person can read: plain words instead.
			const proxy = noAnswer(e) && (e as ApiError).status !== 0;
			out.push(...part.map((p) => ({ folder: p.folder, error: notKnown ? words.unknown : proxy ? words.notSent : message(e), left: true, notKnown })));
			if (stops(e)) {
				const why = notKnown || proxy ? words.notSent : message(e);
				for (const rest of parts.slice(i + 1)) out.push(...rest.map((p) => ({ folder: p.folder, error: why, left: true })));
				progress(picks.length, picks.length, 0);
				break;
			}
		}
		done += part.length;
		progress(done, picks.length, parts[i + 1]?.length ?? 0);
	}
	return out;
}

/**
 * Where a scan should start to reach the first folder that did not go
 * through: the folder just before it in the scanned order, or from, where
 * the scan began, when it is the first. null when every folder went through.
 */
export function scanFromFirstLeft(order: string[], results: PartResult[], from: string): string | null {
	const left = new Set(results.filter((r) => r.left).map((r) => r.folder));
	if (!left.size) return null;
	const i = order.findIndex((f) => left.has(f));
	return i > 0 ? order[i - 1] : from;
}

/** How many folders are through: the parts done and the running part's own count. */
export function throughNow(sent: { done: number; total: number }, inPart: { done: number } | null): number {
	return Math.min(sent.total, sent.done + (inPart?.done ?? 0));
}

/** An import whose page stopped sending: its part jobs, and why it stopped (the tab closed, the person left, the sign-in ended). at: when a page still sending last wrote it, 0 once it stopped. */
export type Cut = { lf: number; total: number; jobs: number[]; why: 'closed' | 'left' | 'signedOut'; at?: number };

/**
 * Where the browser has no locks: how long a sending page's note counts as
 * fresh. A tab hidden for a while has its timers woken about once a minute,
 * so this leaves room for a late wake-up or two.
 */
export const CUT_FRESH_MS = 150_000;

/** The key of one tab's note under a person's base key. */
export function cutKeyOf(base: string, tab: string): string {
	return `${base}.${tab}`;
}

type Store = Pick<Storage, 'length' | 'key' | 'getItem'>;

/** The first record under a person's base key (by any tab, or by an older page under the base itself) that ok accepts, with its key. */
export function findKept<T>(store: Store, base: string, ok: (v: T, key: string) => boolean): { key: string; v: T } | null {
	for (let i = 0; i < store.length; i++) {
		const key = store.key(i);
		if (!key || (key !== base && !key.startsWith(`${base}.`))) continue;
		try {
			const v = JSON.parse(store.getItem(key) ?? 'null');
			if (v && typeof v === 'object' && ok(v, key)) return { key, v };
		} catch {
			// Not a record: skip it.
		}
	}
	return null;
}

const isCut = (v: Partial<Cut>) => typeof v.total === 'number' && Array.isArray(v.jobs);

/** A note whose tab still sends: its lock is held where the browser has locks (held), else it was written lately. */
function sending(v: Cut, key: string, now: number, held: Set<string> | null): boolean {
	if (!(Number(v.at) > 0)) return false;
	return held ? held.has(key) : now - Number(v.at) < CUT_FRESH_MS;
}

/** The first note left under a person's base key, with its key. A note whose tab still sends is an import still running there: passed over; so is one of a Library folder not among ids, when given. */
export function findCut(store: Store, base: string, now = Date.now(), held: Set<string> | null = null, ids?: number[]): { key: string; cut: Cut } | null {
	const f = findKept<Cut>(store, base, (v, key) => isCut(v) && !sending(v, key, now, held) && (!ids || ids.includes(v.lf)));
	return f && { key: f.key, cut: f.v };
}

/** The notes no tab still sends whose Library folder is not among ids: they can never show again. */
export function cutsGone(store: Store, base: string, ids: number[], now = Date.now(), held: Set<string> | null = null): string[] {
	const out: string[] = [];
	for (let i = 0; i < store.length; i++) {
		const key = store.key(i);
		if (!key || (key !== base && !key.startsWith(`${base}.`))) continue;
		try {
			const v = JSON.parse(store.getItem(key) ?? 'null');
			if (v && isCut(v) && !ids.includes(v.lf) && !sending(v, key, now, held)) out.push(key);
		} catch {
			// Not a note: left alone.
		}
	}
	return out;
}

/** A part the server no longer knows: a 404 in HolyArrs's own words, never a proxy's plain 404 page while HolyArrs is down. */
export function jobForgotten(e: unknown): boolean {
	return e instanceof ApiError && e.status === 404 && e.body?.error === 'no such job';
}

/** The notes of one Library folder's imports that no tab still sends: once a later import of it finished they say nothing true. */
export function staleCuts(store: Store, base: string, lf: number, now = Date.now(), held: Set<string> | null = null): string[] {
	const out: string[] = [];
	for (let i = 0; i < store.length; i++) {
		const key = store.key(i);
		if (!key || (key !== base && !key.startsWith(`${base}.`))) continue;
		try {
			const v = JSON.parse(store.getItem(key) ?? 'null');
			if (v && isCut(v) && v.lf === lf && !sending(v, key, now, held)) out.push(key);
		} catch {
			// Not a note: left alone.
		}
	}
	return out;
}

type Locks = { request: (name: string, cb: () => Promise<void>) => Promise<unknown> };

/** Holds the lock name until the returned function is called, so other tabs can see this one is still sending. Without locks it does nothing. */
export function holdWhile(locks: Locks | undefined, name: string): () => void {
	let free = () => {};
	const kept = new Promise<void>((res) => (free = res));
	try {
		void locks?.request(name, () => kept).catch(() => {});
	} catch {
		// No locks here: the note's time stands in.
	}
	return free;
}

type LockQuery = { query: () => Promise<{ held?: { name?: string }[]; pending?: { name?: string }[] }> };

/** The names of the locks held or waited for in this browser now (a tab asks for its lock before it writes its note), or null where there are no locks. */
export async function heldLocks(locks: LockQuery | undefined): Promise<Set<string> | null> {
	if (!locks) return null;
	try {
		const q = await locks.query();
		return new Set([...(q.held ?? []), ...(q.pending ?? [])].map((l) => l.name ?? ''));
	} catch {
		return null;
	}
}

/**
 * The records under a person's base key as they are now, then the locks: read in that order,
 * a note that was read belongs to a tab whose lock was already asked for when the locks are asked.
 */
export async function notesThenLocks(store: Store, base: string, locks: LockQuery | undefined): Promise<{ store: Store; held: Set<string> | null }> {
	const m = new Map<string, string | null>();
	for (let i = 0; i < store.length; i++) {
		const key = store.key(i);
		if (key && (key === base || key.startsWith(`${base}.`))) m.set(key, store.getItem(key));
	}
	const keys = [...m.keys()];
	return { store: { length: keys.length, key: (i) => keys[i] ?? null, getItem: (k) => m.get(k) ?? null }, held: await heldLocks(locks) };
}

/**
 * Loads the Library folders first, then reads the notes and the locks, with the time right after:
 * a slow folder load never makes a note another tab keeps fresh look old. null when the folders
 * did not load (HolyArrs, or a proxy in front of it, did not answer) or there is no storage.
 */
export async function foldersThenNotes(
	load: () => Promise<boolean>,
	store: () => Store,
	base: string,
	locks: LockQuery | undefined,
	clock: () => number = Date.now
): Promise<{ store: Store; held: Set<string> | null; now: number } | null> {
	if (!(await load())) return null;
	try {
		const read = await notesThenLocks(store(), base, locks);
		return { ...read, now: clock() };
	} catch {
		return null;
	}
}

/** Scan records under a person's base key whose Library folder is no longer there: no tab can follow them again. */
export function scansGone(store: Store, base: string, ids: number[]): string[] {
	const out: string[] = [];
	for (let i = 0; i < store.length; i++) {
		const key = store.key(i);
		if (!key || (key !== base && !key.startsWith(`${base}.`))) continue;
		try {
			const v = JSON.parse(store.getItem(key) ?? 'null');
			if (v && typeof v.lf === 'number' && !ids.includes(v.lf)) out.push(key);
		} catch {
			// Not a record: left alone.
		}
	}
	return out;
}

/** How many folders the server took, from the part jobs themselves: a finished part's results, a running part's own count. */
export function throughJobs(jobs: (JobView | null | undefined)[]): number {
	let n = 0;
	for (const j of jobs) {
		if (!j) continue;
		const res = j.Result?.results;
		n += Array.isArray(res) ? res.length : (scanCount(j.ProgressNote)?.done ?? 0);
	}
	return n;
}

/** The note for an import cut short, or null when every folder went through. */
export function cutNote(c: Cut, through: number): { key: string; done: number; total: number } | null {
	if (through >= c.total) return null;
	const key = c.why === 'left' ? 'library.import.interruptedLeft' : c.why === 'signedOut' ? 'library.import.interruptedSignedOut' : 'library.import.interrupted';
	return { key, done: through, total: c.total };
}

/**
 * The note for an import cut short, read from its part jobs (null: a part the server forgot, undefined: a part not answered).
 * null also when the server no longer knows a part, so how far it got cannot be told; undefined when a part got no answer this time, so the note stays as it is.
 */
export function cutFromJobs(c: Cut, jobs: (JobView | null | undefined)[]): { key: string; done: number; total: number } | null | undefined {
	if (jobs.some((j) => j === undefined)) return undefined;
	return jobs.some((j) => !j) ? null : cutNote(c, throughJobs(jobs));
}

/** How long a wait for an answer goes before the page says HolyArrs is not answering. */
export const quietAfter = 5000;

/** True when the last answer came more than quietAfter ago. */
export function quiet(heard: number, now: number): boolean {
	return heard > 0 && now - heard > quietAfter;
}
