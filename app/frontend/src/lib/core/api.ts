// Fetch client with request sharing, early boot asks, csrf and sign-out handling.
import { to } from './paths.ts';
import { registerPictures } from './pictures.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';

export class ApiError extends Error {
	status: number;
	retryAfter: number | null;
	body: Record<string, unknown>;
	constructor(message: string, status: number, body: Record<string, unknown> = {}, retryAfter: number | null = null) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.body = body;
		this.retryAfter = retryAfter;
	}
}

export interface RequestOptions {
	method?: 'GET' | 'POST' | 'PUT' | 'DELETE';
	body?: unknown;
	raw?: BodyInit;
	signal?: AbortSignal;
}

type FetchFn = (input: string, init?: RequestInit) => Promise<Response>;

type Follower<T> = { res?: (v: T) => void; rej?: (e: unknown) => void; signal?: AbortSignal; stop?: () => void };

export interface EarlyAsks {
	at: number;
	asks: Record<string, Promise<Response>>;
}

export type Client = ReturnType<typeof createClient>;

// A 401 on these routes is an answer, not an expired session, so it must not sign the user out.
const quiet401 = [
	'/auth/login',
	'/auth/state',
	'/auth/me',
	'/auth/setup',
	'/auth/media-server',
	'/auth/plex',
	'/auth/invites',
	'/auth/two-factor',
	'/auth/password',
	'/auth/oidc',
	'/auth/forward',
	'/auth/reauth'
];

// Shortest and longest time a finished GET is shared with later callers.
export const KEEP_MOMENT = 1500;
export const KEEP_MAX = 60_000;

// Boot asks older than this are stale and are ignored.
export const EARLY_MAX = 30_000;

// The page starts these fetches before the app loads, so the first view has no wait.
const bootAsks = () => (globalThis as { __holyarrsEarly?: EarlyAsks }).__holyarrsEarly;

// Cache-Control tells how long a reply may be reused; no-store means never.
export function keepFor(cacheControl: string | null): number {
	const cc = (cacheControl ?? '').toLowerCase();
	if (/no-store|no-cache/.test(cc)) return 0;
	const m = /max-age=(\d+)/.exec(cc);
	if (!m) return KEEP_MOMENT;
	return Math.min(KEEP_MAX, Math.max(KEEP_MOMENT, Number(m[1]) * 1000));
}

// Partial or pending replies are still being built, so they must not be kept.
export function isFilling(v: unknown): boolean {
	if (!v || typeof v !== 'object') return false;
	const o = v as { partial?: unknown; pending?: unknown };
	return o.partial === true || o.pending === true;
}

export function isIncomplete(v: unknown): boolean {
	return !!v && typeof v === 'object' && (v as { incomplete?: unknown }).incomplete === true;
}

// One shared client per page; same-path GETs in flight or fresh are answered from one request.
export function createClient(fetchFn: FetchFn = (i, o) => fetch(i, o), early: () => EarlyAsks | undefined = bootAsks) {
	let csrf = '';
	let signedOut: (() => void) | null = null;

	function takeEarly(url: string): Promise<Response> | undefined {
		const e = early();
		if (!e) return undefined;
		const p: Promise<Response> | undefined = e.asks[url];
		delete e.asks[url];
		return Date.now() - e.at <= EARLY_MAX ? p : undefined;
	}
	function dropEarly() {
		const e = early();
		if (e) e.asks = {};
	}

	async function request<T>(path: string, opts: RequestOptions = {}, seen?: (res: Response) => void): Promise<T> {
		const method = opts.method ?? 'GET';
		const headers: Record<string, string> = { Accept: 'application/json' };
		let body: BodyInit | undefined;
		if (opts.raw !== undefined) {
			body = opts.raw;
			if (!(typeof FormData !== 'undefined' && opts.raw instanceof FormData)) headers['Content-Type'] = 'application/octet-stream';
		} else if (opts.body !== undefined) {
			body = JSON.stringify(opts.body);
			headers['Content-Type'] = 'application/json';
		}
		if (method !== 'GET' && csrf) headers['X-CSRF-Token'] = csrf;
		const url = to('/api/v1' + path);
		const ahead = method === 'GET' && body === undefined ? takeEarly(url) : undefined;
		const ask = () => fetchFn(url, { method, headers, body, signal: opts.signal, credentials: 'same-origin' });
		let res: Response;
		try {
			res = ahead ? await ahead.catch(ask) : await ask();
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') throw e;
			throw new ApiError(t('core.api.notAnswering'), 0);
		}
		seen?.(res);
		if (res.status === 204) return undefined as T;
		let data: Record<string, unknown> = {};
		const text = await res.text();
		if (text) {
			try {
				data = JSON.parse(text);
			} catch {
				data = { error: text.trim() };
			}
		}
		if (!res.ok) {
			if (res.status === 401) dropEarly();
			if (res.status === 401 && !quiet401.some((p) => path.startsWith(p))) {
				shared.clear();
				signedOut?.();
			}
			if (res.status === 403 && data.code === 'reauth' && typeof window !== 'undefined') window.dispatchEvent(new CustomEvent('holyarrs:reauth', { detail: { method, path } }));
			const ra = Number(res.headers.get('Retry-After'));
			// message is shown in the picked language; body.error keeps the server's English for matching.
			const msg = typeof data.error === 'string' && data.error ? serverWords(data.error) : t('core.api.requestFailed', { status: res.status });
			throw new ApiError(msg, res.status, data, Number.isFinite(ra) && ra > 0 ? ra : null);
		}
		if (data && typeof data === 'object' && 'pictures' in data) registerPictures(data.pictures);
		return data as T;
	}

	const shared = new Map<string, { p: Promise<unknown>; done: number; keep: number }>();

	function start(path: string, entry: { keep: number }): Promise<unknown> {
		return Promise.resolve().then(() => request<unknown>(path, {}, (res) => (entry.keep = keepFor(res.headers.get('Cache-Control')))));
	}

	function get<T>(path: string, signal?: AbortSignal): Promise<T> {
		const now = Date.now();
		let hit = shared.get(path);
		if (!hit || (hit.done && (!hit.keep || now - hit.done > hit.keep))) {
			const entry = { p: Promise.resolve<unknown>(undefined), done: 0, keep: KEEP_MOMENT };
			entry.p = start(path, entry);
			entry.p.then(
				(v) => {
					entry.done = Date.now();
					if (isIncomplete(v) || isFilling(v) || (!!v && typeof v === 'object' && (v as { building?: unknown }).building === true)) entry.keep = 0;
				},
				() => shared.get(path) === entry && shared.delete(path)
			);
			shared.set(path, entry);
			hit = entry;
		}
		if (signal?.aborted) return Promise.reject(signal.reason ?? new DOMException('Aborted', 'AbortError'));
		return follow<T>(hit.p, signal);
	}

	function change<T>(path: string, opts: RequestOptions): Promise<T> {
		shared.clear();
		dropEarly();
		return request<T>(path, opts).finally(() => shared.clear());
	}

	return {
		request: <T>(path: string, opts: RequestOptions = {}) => ((opts.method ?? 'GET') === 'GET' && opts.raw === undefined && opts.body === undefined ? get<T>(path, opts.signal) : change<T>(path, opts)),
		get,
		post: <T>(p: string, body?: unknown) => change<T>(p, { method: 'POST', body }),
		put: <T>(p: string, body?: unknown) => change<T>(p, { method: 'PUT', body }),
		del: <T = void>(p: string) => change<T>(p, { method: 'DELETE' }),
		setCsrf(v: string) {
			if (v !== csrf) shared.clear();
			if (csrf && v !== csrf) dropEarly();
			csrf = v;
		},
		csrf: () => csrf,
		forget: () => {
			shared.clear();
			dropEarly();
		},
		warm<T>(path: string, keep: number): Promise<T> {
			const answer = get<T>(path);
			const hit = shared.get(path);
			hit?.p.then(
				() => {
					if (hit.keep > 0 && hit.keep < keep) hit.keep = Math.min(keep, KEEP_MAX);
				},
				() => {}
			);
			return answer;
		},
		has(path: string): boolean {
			const hit = shared.get(path);
			return !!hit && hit.done > 0 && hit.keep > 0 && Date.now() - hit.done <= hit.keep;
		},
		onSignedOut(fn: (() => void) | null) {
			signedOut = fn;
		}
	};
}

export const api = createClient();

export function message(e: unknown): string {
	if (e instanceof ApiError) return e.message;
	if (e instanceof Error) return e.message;
	return t('core.api.wentWrong');
}

// Each caller gets its own copy so one cannot change what another reads, and its own abort.
function follow<T>(shared: Promise<unknown>, signal?: AbortSignal): Promise<T> {
	const f: Follower<T> = {};
	const out = new Promise<T>((res, rej) => {
		f.res = res;
		f.rej = rej;
	});
	shared.then(
		(v) => settle(f, true, v === undefined ? v : structuredClone(v)),
		(e) => settle(f, false, e)
	);
	if (signal) {
		f.signal = signal;
		f.stop = () => settle(f, false, f.signal?.reason ?? new DOMException('Aborted', 'AbortError'));
		signal.addEventListener('abort', f.stop, { once: true });
	}
	return out;
}

function settle<T>(f: Follower<T>, ok: boolean, v: unknown) {
	const { res, rej, signal, stop } = f;
	f.res = f.rej = f.signal = f.stop = undefined;
	if (signal && stop) signal.removeEventListener('abort', stop);
	if (ok) res?.(v as T);
	else rej?.(v);
}

