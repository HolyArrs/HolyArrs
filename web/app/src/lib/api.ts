// The one way the app talks to HolyArrs: /api/v1, JSON, the CSRF header on
// every change, and one place that notices a lost sign-in.
import { to } from './paths.ts';

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

// Calls that answer 401 as part of their normal job; a 401 there is not a
// lost session.
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

export function createClient(fetchFn: FetchFn = (i, o) => fetch(i, o)) {
	let csrf = '';
	let signedOut: (() => void) | null = null;

	async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
		const method = opts.method ?? 'GET';
		const headers: Record<string, string> = { Accept: 'application/json' };
		let body: BodyInit | undefined;
		if (opts.raw !== undefined) {
			body = opts.raw;
			headers['Content-Type'] = 'application/octet-stream';
		} else if (opts.body !== undefined) {
			body = JSON.stringify(opts.body);
			headers['Content-Type'] = 'application/json';
		}
		if (method !== 'GET' && csrf) headers['X-CSRF-Token'] = csrf;
		let res: Response;
		try {
			res = await fetchFn(to('/api/v1' + path), { method, headers, body, signal: opts.signal, credentials: 'same-origin' });
		} catch (e) {
			if (e instanceof DOMException && e.name === 'AbortError') throw e;
			throw new ApiError('HolyArrs is not answering. Check that it is running.', 0);
		}
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
			if (res.status === 401 && !quiet401.some((p) => path.startsWith(p))) signedOut?.();
			const ra = Number(res.headers.get('Retry-After'));
			const msg = typeof data.error === 'string' && data.error ? data.error : `Request failed (${res.status})`;
			throw new ApiError(msg, res.status, data, Number.isFinite(ra) && ra > 0 ? ra : null);
		}
		return data as T;
	}

	// Identical GETs share one answer while it is in flight and for a moment
	// after, so parts of one page asking the same thing cost one request.
	// Any change (POST, PUT, DELETE) forgets them. Each caller gets its own copy.
	const shared = new Map<string, { p: Promise<unknown>; done: number }>();
	const keep = 1500;

	function get<T>(path: string, signal?: AbortSignal): Promise<T> {
		const now = Date.now();
		let hit = shared.get(path);
		if (!hit || (hit.done && now - hit.done > keep)) {
			const entry = { p: request<unknown>(path), done: 0 };
			entry.p.then(
				() => (entry.done = Date.now()),
				() => shared.get(path) === entry && shared.delete(path)
			);
			shared.set(path, entry);
			hit = entry;
		}
		const copy = hit.p.then((v) => (v === undefined ? v : structuredClone(v)) as T);
		if (!signal) return copy;
		if (signal.aborted) return Promise.reject(signal.reason ?? new DOMException('Aborted', 'AbortError'));
		return new Promise<T>((resolve, reject) => {
			const stop = () => reject(signal.reason ?? new DOMException('Aborted', 'AbortError'));
			signal.addEventListener('abort', stop, { once: true });
			copy.then(resolve, reject).finally(() => signal.removeEventListener('abort', stop));
		});
	}

	function change<T>(path: string, opts: RequestOptions): Promise<T> {
		shared.clear();
		return request<T>(path, opts).finally(() => shared.clear());
	}

	return {
		request: <T>(path: string, opts: RequestOptions = {}) => ((opts.method ?? 'GET') === 'GET' && opts.raw === undefined && opts.body === undefined ? get<T>(path, opts.signal) : change<T>(path, opts)),
		get,
		post: <T>(p: string, body?: unknown) => change<T>(p, { method: 'POST', body }),
		put: <T>(p: string, body?: unknown) => change<T>(p, { method: 'PUT', body }),
		del: <T = void>(p: string) => change<T>(p, { method: 'DELETE' }),
		setCsrf(v: string) {
			csrf = v;
		},
		csrf: () => csrf,
		/** forget drops shared reads, for news of a change made somewhere else (the live stream). */
		forget: () => shared.clear(),
		onSignedOut(fn: (() => void) | null) {
			signedOut = fn;
		}
	};
}

export type Client = ReturnType<typeof createClient>;

export const api = createClient();

/** message turns any error into a sentence for the screen. */
export function message(e: unknown): string {
	if (e instanceof ApiError) return e.message;
	if (e instanceof Error) return e.message;
	return 'Something went wrong.';
}
