/** Keeps a sign-in "next" link inside this app: a path on this site only, never another site ("//host", "/\host" and control characters, which browsers may read as another host), never the sign-in page itself. */
export function safeNext(n: string | null): string {
	// eslint-disable-next-line no-control-regex
	if (!n || !n.startsWith('/') || n.startsWith('//') || n.includes('\\') || /[\u0000-\u001f\u007f]/.test(n) || n.startsWith('/login')) return '/';
	return n;
}
