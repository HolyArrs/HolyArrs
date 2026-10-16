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
const quiet401 = ['/auth/login', '/auth/state', '/auth/me', '/auth/setup', '/auth/media-server', '/auth/plex', '/auth/invites', '/auth/two-factor', '/auth/password'];

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

	return {
		request,
		get: <T>(p: string, signal?: AbortSignal) => request<T>(p, { signal }),
		post: <T>(p: string, body?: unknown) => request<T>(p, { method: 'POST', body }),
		put: <T>(p: string, body?: unknown) => request<T>(p, { method: 'PUT', body }),
		del: <T = void>(p: string) => request<T>(p, { method: 'DELETE' }),
		setCsrf(v: string) {
			csrf = v;
		},
		csrf: () => csrf,
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
