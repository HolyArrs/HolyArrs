/** Keeps a sign-in "next" link inside this app: a path on this site only, never another site ("//host", "/\host" and control characters, which browsers may read as another host), never the sign-in page itself. */
export function safeNext(n: string | null): string {
	// eslint-disable-next-line no-control-regex
	if (!n || !n.startsWith('/') || n.startsWith('//') || n.includes('\\') || /[\u0000-\u001f\u007f]/.test(n) || n.startsWith('/login')) return '/';
	return n;
}

/** Where a finished sign-in goes: the page's "next", or where single sign-on was asked for as proof. */
export function signInNext(params: { get(key: string): string | null }): string {
	let n = params.get('next') ?? '/';
	// Back from single sign-on asked for as proof: return where it was asked.
	if (params.get('sso') === 'finish') {
		try {
			n = sessionStorage.getItem('holyarrs-sso-next') ?? n;
			sessionStorage.removeItem('holyarrs-sso-next');
		} catch {
			// No storage: Home.
		}
	}
	// Only paths inside this app; never another site.
	return safeNext(n);
}

/** What other pages hand the sign-in page through this tab's storage (never the address): an invite join's two-factor ticket, and an import that stopped because the sign-in ended. Each is read once. */
export function takeHandoff(): { ticket: string; importPaused: boolean } {
	const out = { ticket: '', importPaused: false };
	try {
		const saved = sessionStorage.getItem('holyarrs-2fa-ticket');
		if (saved) {
			sessionStorage.removeItem('holyarrs-2fa-ticket');
			out.ticket = saved;
		}
		if (sessionStorage.getItem('holyarrs-import-paused')) {
			sessionStorage.removeItem('holyarrs-import-paused');
			out.importPaused = true;
		}
	} catch {
		// No storage: they sign in again.
	}
	return out;
}
