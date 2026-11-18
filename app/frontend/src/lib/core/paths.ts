// Helpers for building and reading app paths when served under a sub-path.

// Sub-path prefix the app is mounted under, empty at the root.
let base = '';

export function setBase(b: string) {
	base = b.replace(/\/+$/, '');
}

export function getBase(): string {
	return base;
}

// Prefixes root-relative paths only; protocol-relative and full URLs pass through.
export function to(p: string): string {
	return p.startsWith('/') && !p.startsWith('//') ? base + p : p;
}

// Prefixes the app's own /api/ addresses (pictures the server names without the base), once: an address that already carries the base stays as it is.
export function toOwn(p: string): string {
	if (!base || !p.startsWith('/api/') || p.startsWith(base + '/api/')) return p;
	return base + p;
}

// Strips the prefix so routing sees the app-internal path.
export function appPath(pathname: string): string {
	if (!base) return pathname;
	if (pathname === base) return '/';
	return pathname.startsWith(base + '/') ? pathname.slice(base.length) : pathname;
}

export function detailPath(kind: 'movie' | 'show', tmdb: string): string {
	return `/title/${kind}/${encodeURIComponent(tmdb)}`;
}

// A path with a fresh= stamp so the server skips its cache.
export const freshPath = (path: string, now = Date.now()): string => `${path}${path.includes('?') ? '&' : '?'}fresh=${now}`;
