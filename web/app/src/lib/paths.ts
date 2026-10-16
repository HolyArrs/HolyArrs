// The URL base HolyArrs runs under behind a reverse proxy (such as
// /holyarrs; "" for none). The layout sets it from the router once; every
// link, API call and the live stream goes through to().
let base = '';

/** setBase records the base ("" or "/x", no trailing slash). */
export function setBase(b: string) {
	base = b.replace(/\/+$/, '');
}

/** getBase is the base in use. */
export function getBase(): string {
	return base;
}

/** to puts the base in front of an app path ("/library"); other links (https:, //, #) are left alone. */
export function to(p: string): string {
	return p.startsWith('/') && !p.startsWith('//') ? base + p : p;
}

/** appPath takes the base off an address path, giving the app's own path. */
export function appPath(pathname: string): string {
	if (!base) return pathname;
	if (pathname === base) return '/';
	return pathname.startsWith(base + '/') ? pathname.slice(base.length) : pathname;
}
