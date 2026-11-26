import { describe, expect, it } from 'vitest';
import { fromPluginBody, servesAs } from './from-plugin.ts';

const p = (installed: boolean, actions: string[] | null) => ({ installed, access: { hosts: [], folders: [], actions } }) as never;

describe('from a plugin', () => {
	it.each([
		[p(true, ['sources.downloader']), 'downloader', true],
		[p(true, ['sources.releases']), 'downloader', false],
		[p(true, ['sources.releases']), 'source', true],
		[p(false, ['sources.downloader']), 'downloader', false],
		[p(true, null), 'source', false]
	] as const)('%#', (plugin, kind, want) => {
		expect(servesAs(plugin, kind)).toBe(want);
	});
	it('sends only the plugin address', () => {
		expect(fromPluginBody({ id: 'dl', name: 'My client' }, 'downloader')).toEqual({ name: 'My client', kind: 'plugin-torrent', url: 'plugin://dl' });
		expect(fromPluginBody({ id: 'ix', name: 'Idx' }, 'source')).toEqual({ name: 'Idx', format: 'torznab', url: 'plugin://ix' });
	});
	it('sends the download type the plugin brings', () => {
		const slsk = { id: 'slskd', name: 'Soulseek', download_types: ['soulseek'] };
		expect(fromPluginBody(slsk, 'downloader')).toEqual({ name: 'Soulseek', kind: 'plugin', protocol: 'soulseek', url: 'plugin://slskd' });
		expect(fromPluginBody(slsk, 'source')).toEqual({ name: 'Soulseek', format: 'torznab', protocol: 'soulseek', url: 'plugin://slskd' });
	});
});
