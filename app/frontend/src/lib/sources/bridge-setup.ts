// First-run Sources step: the Prowlarr or Jackett address as Protocol | host | Port, its key page and what linking brings.

export const BRIDGE_PORTS: Record<'prowlarr' | 'jackett', string> = { prowlarr: '9696', jackett: '9117' };

// What linking brings over, shown beside the form (i18n keys, {name} is the app).
export const BRIDGE_BRINGS = ['sources.bridge.brings.sites', 'sources.bridge.brings.categories', 'sources.bridge.brings.search', 'sources.bridge.brings.sync'];

// The app's own page that shows its API key.
const KEY_PATHS: Record<string, string> = { prowlarr: '/settings/general', jackett: '/UI/Dashboard' };

/** The full address from the three boxes ('' with no host); a base path read from an older address stays on the end. */
export function joinAddress(ssl: boolean, host: string, port: string, basePath = ''): string {
	const h = host.trim();
	if (!h) return '';
	return `${ssl ? 'https' : 'http'}://${h}${port.trim() ? ':' + port.trim() : ''}${basePath}`;
}

/** Splits an address back into the boxes; a bare host:port reads as http. */
export function splitAddress(url: string, fallbackPort: string): { ssl: boolean; host: string; port: string; basePath: string } {
	try {
		const u = new URL(/^[a-z]+:\/\//i.test(url) ? url : `http://${url}`);
		const ssl = u.protocol === 'https:';
		return { ssl, host: u.hostname, port: u.port || (ssl ? '443' : '80'), basePath: u.pathname.replace(/\/+$/, '') };
	} catch {
		return { ssl: false, host: url.trim(), port: fallbackPort, basePath: '' };
	}
}

/** The app's key page once the host looks reachable from a browser, else ''. */
export function bridgeKeyPage(kind: string, ssl: boolean, host: string, port: string, basePath = ''): string {
	const h = host.trim().toLowerCase();
	const path = KEY_PATHS[kind];
	if (!h || !path || /[\s/@]/.test(h) || !(h === 'localhost' || h.includes('.') || h.includes(':'))) return '';
	return joinAddress(ssl, h, port, basePath) + path;
}
