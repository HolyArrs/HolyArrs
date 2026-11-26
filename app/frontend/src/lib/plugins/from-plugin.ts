// Adding a plugin as a download client or a search source: which plugins can, and what is sent.
import type { Plugin } from './types.ts';

export type FromPluginKind = 'downloader' | 'source';

const scopeOf: Record<FromPluginKind, string> = { downloader: 'sources.downloader', source: 'sources.releases' };

/** An installed plugin whose approved access includes the hook for kind. */
export function servesAs(p: Pick<Plugin, 'installed' | 'access'>, kind: FromPluginKind): boolean {
	return !!p.installed && !!p.access?.actions?.includes(scopeOf[kind]);
}

/** The body that adds p: its own address plugin://<id>, and the first download type it brings, if any. */
export function fromPluginBody(p: Pick<Plugin, 'id' | 'name' | 'download_types'>, kind: FromPluginKind): Record<string, unknown> {
	const url = `plugin://${p.id}`;
	const own = p.download_types?.[0];
	if (kind === 'source') return own ? { name: p.name, format: 'torznab', protocol: own, url } : { name: p.name, format: 'torznab', url };
	return own ? { name: p.name, kind: 'plugin', protocol: own, url } : { name: p.name, kind: 'plugin-torrent', url };
}
