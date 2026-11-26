// Downloads step: the downloader address as Protocol | host | Port, and SABnzbd's key page.

/** The full address from the three boxes ('' with no host); a base path read from an older address stays on the end. */
export function joinDlAddress(ssl: boolean, host: string, port: string, basePath = ''): string {
	const h = host.trim();
	if (!h) return '';
	return `${ssl ? 'https' : 'http'}://${h}${port.trim() ? ':' + port.trim() : ''}${basePath}`;
}

/** Splits an address back into the boxes; a bare host:port reads as http, and no host keeps the app's default port. */
export function splitDlAddress(url: string, fallbackPort: string): { ssl: boolean; host: string; port: string; basePath: string } {
	const v = url.trim();
	if (!v || /^[a-z]+:\/\/$/i.test(v)) return { ssl: /^https/i.test(v), host: '', port: fallbackPort, basePath: '' };
	try {
		const u = new URL(/^[a-z]+:\/\//i.test(v) ? v : `http://${v}`);
		const ssl = u.protocol === 'https:';
		return { ssl, host: u.hostname, port: u.port || (ssl ? '443' : '80'), basePath: u.pathname.replace(/\/+$/, '') };
	} catch {
		return { ssl: false, host: v, port: fallbackPort, basePath: '' };
	}
}

/** SABnzbd's General settings page (it shows the API key) once the host looks reachable from a browser, else ''. */
export function sabKeyPage(ssl: boolean, host: string, port: string, basePath = ''): string {
	const h = host.trim().toLowerCase();
	if (!h || /[\s/@]/.test(h) || !(h === 'localhost' || h.includes('.') || h.includes(':'))) return '';
	return joinDlAddress(ssl, h, port, basePath) + '/config/general/';
}
