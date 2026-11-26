// Adding a plugin as a download client or a search source: which plugins can, and what is sent.
import type { Plugin } from './types.ts';

export type FromPluginKind = 'downloader' | 'source';

const scopeOf: Record<FromPluginKind, string> = { downloader: 'sources.downloader', source: 'sources.releases' };

/** An installed plugin whose approved access includes the hook for kind. */
export function servesAs(p: Pick<Plugin, 'installed' | 'access'>, kind: FromPluginKind): boolean {
	return !!p.installed && !!p.access?.actions?.includes(scopeOf[kind]);
}

/** The body that adds p: its own address plugin://<id>, nothing else to fill in. */
export function fromPluginBody(p: Pick<Plugin, 'id' | 'name'>, kind: FromPluginKind): Record<string, unknown> {
	const url = `plugin://${p.id}`;
	return kind === 'downloader' ? { name: p.name, adapter: 'plugin-torrent', url } : { name: p.name, format: 'torznab', url };
}
