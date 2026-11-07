let base = '';

export function setBase(b: string) {
	base = b.replace(/\/+$/, '');
}

export function getBase(): string {
	return base;
}

export function to(p: string): string {
	return p.startsWith('/') && !p.startsWith('//') ? base + p : p;
}

export function appPath(pathname: string): string {
	if (!base) return pathname;
	if (pathname === base) return '/';
	return pathname.startsWith(base + '/') ? pathname.slice(base.length) : pathname;
}

export function detailPath(kind: 'movie' | 'show', tmdb: string): string {
	return `/title/${kind}/${encodeURIComponent(tmdb)}`;
}
