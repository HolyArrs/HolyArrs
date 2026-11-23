// The owner's Plex servers as GET /auth/plex/servers lists them, one row per address.
export type PlexServerRow = { id: string; name: string; address: string; port: number; local: boolean; url: string; secure_url?: string; relay?: boolean };

// The addresses of one row, best first: a home server's secure address, then
// its plain one; a remote server only ever its secure address. A relay row is
// listed by the server only when nothing else reaches it.
export function plexAddresses(c: PlexServerRow): string[] {
	if (!c.local) return [c.secure_url || c.url].filter(Boolean);
	return [...new Set([c.secure_url, c.url].filter((u): u is string => !!u))];
}

// Which row is picked first: home before remote, relay only when it is all there is.
export function firstPlexRow(list: PlexServerRow[]): number {
	const rank = (c: PlexServerRow) => (c.local ? 0 : c.relay ? 2 : 1);
	let best = -1;
	list.forEach((c, i) => {
		if (best < 0 || rank(c) < rank(list[best])) best = i;
	});
	return best;
}
