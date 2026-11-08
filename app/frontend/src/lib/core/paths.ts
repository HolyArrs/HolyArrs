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

// Strips the prefix so routing sees the app-internal path.
export function appPath(pathname: string): string {
	if (!base) return pathname;
	if (pathname === base) return '/';
	return pathname.startsWith(base + '/') ? pathname.slice(base.length) : pathname;
}

export function detailPath(kind: 'movie' | 'show', tmdb: string): string {
	return `/title/${kind}/${encodeURIComponent(tmdb)}`;
}
