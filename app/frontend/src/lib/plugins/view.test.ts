import { describe, expect, it } from 'vitest';
import { abilities, categories, categoryLabel, developers, findPlugins, inTab, notices, platformLabel, safeLink, shownName, sortPlugins, reaches, pluginName, pluginState, shownVersion, noLockNotice, showsNoLockCard, showsNoLockTag, stateLabel } from './view.ts';
import { eventLabel, needLabel, newAbilities } from './view.ts';
import { isNew, freshUpdate, trendingIds, shortDate, pluginIcon, devPicture } from './view.ts';
import type { Plugin } from './types.ts';

const base: Plugin = { id: 'tailscale', version: '1.2.0', installed: false, running: false, enabled: false, access: null };

describe('plugin rows', () => {
	it('names known plugins and tidies unknown ids', () => {
		expect(pluginName('tailscale')).toBe('Tailscale');
		expect(pluginName('mcp-server')).toBe('MCP Server');
		expect(pluginName('my_tool')).toBe('My tool');
	});
	it('reads the four states', () => {
		expect(pluginState(base)).toBe('not_installed');
		expect(pluginState({ ...base, installed: true, enabled: true, running: true })).toBe('running');
		expect(pluginState({ ...base, installed: true, enabled: true })).toBe('stopped');
		expect(pluginState({ ...base, installed: true, enabled: false, running: true })).toBe('off');
	});
	it('says why a plugin is turned off', () => {
		expect(stateLabel({ ...base, installed: true, disabled_reason: 'on the blocklist' })).toBe('Turned off: on the blocklist');
		expect(stateLabel({ ...base, installed: true })).toBe('Turned off');
		expect(stateLabel(base)).toBe('Not installed');
	});
	it('shows the installed version once installed', () => {
		expect(shownVersion({ ...base, installed: true, installed_version: '1.1.0' })).toBe('1.1.0');
		expect(shownVersion(base)).toBe('1.2.0');
	});
});

describe('page notices and access', () => {
	it('lists every notice that applies, key first', () => {
		expect(notices({ key_set: true, can_run: true, blocklist_stale: false })).toEqual([]);
		expect(notices({ key_set: false, can_run: false, blocklist_stale: true })).toEqual([
			"Plugins can't be installed yet. This HolyArrs build doesn't have the plugin signing key.",
			'Plugins run only on Linux.',
			"Couldn't check for blocked plugins for over a week."
		]);
	});
	it('stays calm when the list is just missing, warns when its signature fails', () => {
		expect(notices({ key_set: true, can_run: true, blocklist_stale: false, list_bad: false })).toEqual([]);
		expect(notices({ key_set: true, can_run: true, blocklist_stale: false, list_bad: true })).toEqual([
			"The plugin list didn't pass its signature check, so it isn't shown."
		]);
	});
	it('says why the file lock is missing and what to change', () => {
		expect(noLockNotice('old_kernel')).toContain('too old for the plugin file lock');
		expect(noLockNotice(undefined)).toContain('too old for the plugin file lock');
		expect(noLockNotice('blocked')).toContain("container's security profile (seccomp)");
		expect(noLockNotice('turned_off')).toContain('lsm=');
		expect(notices({ key_set: true, can_run: true, blocklist_stale: false })).toEqual([]);
	});
	it('shows one file lock card, and the tag only once allowed', () => {
		const ov = { can_run: true, no_file_lock: true, allow_no_file_lock: false };
		expect(showsNoLockCard(ov)).toBe(true);
		expect(showsNoLockCard({ ...ov, can_run: false })).toBe(false);
		expect(showsNoLockCard({ ...ov, no_file_lock: false })).toBe(false);
		expect(showsNoLockTag(ov)).toBe(false);
		expect(showsNoLockTag({ ...ov, allow_no_file_lock: true })).toBe(true);
		expect(showsNoLockTag({ ...ov, allow_no_file_lock: true, no_file_lock: false })).toBe(false);
	});
	it('lists what a plugin can reach from its manifest, nothing when it declares nothing', () => {
		const ts = { ...base, reaches_app: true, access: { hosts: ['controlplane.tailscale.com'], folders: null, actions: null } };
		expect(reaches(ts)).toEqual(["Connect to Tailscale's servers to sign in and find your devices", 'Reach nothing else: other websites and devices are blocked', 'Let devices on your tailnet open HolyArrs']);
		expect(reaches({ ...base, id: 'x', access: { hosts: ['a.org', 'b.org'], folders: null, actions: null } })).toEqual(['Connect to a.org, b.org', 'Reach nothing else: other websites and devices are blocked']);
		expect(reaches({ ...base, id: 'x' })).toEqual([]);
		expect(abilities({ ...base, id: 'x' })).toEqual(['Keep files in its own folder, and nowhere else on this server']);
	});
});

describe('plugin search and sort', () => {
	const list: Plugin[] = [
		{ ...base, id: 'zeta', description: 'Sends alerts' },
		{ ...base, id: 'alpha', description: 'Backs up the database' }
	];
	it('finds by name or description, every word', () => {
		expect(findPlugins(list, '').length).toBe(2);
		expect(findPlugins(list, 'ALPHA').map((p) => p.id)).toEqual(['alpha']);
		expect(findPlugins(list, 'backs database').map((p) => p.id)).toEqual(['alpha']);
		expect(findPlugins(list, 'nothing')).toEqual([]);
	});
	it('sorts by name, or newest first by list order', () => {
		expect(sortPlugins(list, 'name').map((p) => p.id)).toEqual(['alpha', 'zeta']);
		expect(sortPlugins(list, 'new').map((p) => p.id)).toEqual(['alpha', 'zeta']);
		expect(list.map((p) => p.id)).toEqual(['zeta', 'alpha']);
	});
});

describe('directory tiles and panel', () => {
	const a: Plugin = { ...base, id: 'watch-sync', name: 'Watch Sync', category: 'Sync', updated: '2026-11-01', access: { hosts: ['api.example.net'], folders: [{ name: 'imports', mode: 'read' }, { name: 'exports', mode: 'read_write' }], actions: ['library.read', 'odd.thing'] } };
	const b: Plugin = { ...base, id: 'theme-pack', name: 'Theme Pack', category: 'Look', updated: '2026-11-15', installed: true, access: { hosts: ['*'], folders: null, actions: null } };
	const c: Plugin = { ...base, id: 'no-cat', access: null };
	it('uses the manifest name first', () => {
		expect(shownName(a)).toBe('Watch Sync');
		expect(shownName(c)).toBe('No cat');
	});
	it('sorts newest by the updated date, then list order', () => {
		expect(sortPlugins([a, b, c], 'new').map((p) => p.id)).toEqual(['theme-pack', 'watch-sync', 'no-cat']);
	});
	it('groups by category with Other last, and filters tabs', () => {
		expect(categories([c, a, b])).toEqual(['Look', 'Sync', 'Other']);
		expect(inTab([a, b, c], 'installed').map((p) => p.id)).toEqual(['theme-pack']);
		expect(inTab([a, b, c], 'cat:Other').map((p) => p.id)).toEqual(['no-cat']);
		expect(categoryLabel('Sync')).toBe('Sync');
		expect(categoryLabel('Weird')).toBe('Weird');
	});
	it('says actions, folders and any host in plain words', () => {
		expect(reaches(a)).toEqual(['Connect to api.example.net', 'Reach nothing else: other websites and devices are blocked', 'See your library', 'Use HolyArrs for: odd.thing', 'Use the folders you choose: imports (read only), exports (read and write)']);
		expect(reaches(b)).toEqual(['Connect to any web address']);
	});
	it('shows only GitHub logins as developers and only https links', () => {
		expect(developers({ developers: ['CoderLuii', 'bad name'], author: 'x' })).toEqual(['CoderLuii']);
		expect(developers({ author: 'holyarrs-community' })).toEqual(['holyarrs-community']);
		expect(developers({ author: 'Some Person' })).toEqual([]);
		expect(safeLink('https://github.com/HolyArrs/HolyArrs-Plugins')).toBe('https://github.com/HolyArrs/HolyArrs-Plugins');
		expect(safeLink('javascript:alert(1)')).toBe('');
		expect(platformLabel('linux-arm64')).toBe('ARM 64-bit');
	});
});

describe('public numbers', () => {
	const now = Date.parse('2026-11-20T12:00:00Z');
	const mk = (id: string, week?: number): Plugin => ({ id, version: '1', installed: false, running: false, enabled: false, access: null, stats: week == null ? undefined : { week } });
	it('marks new, fresh and trending plugins', () => {
		expect(isNew({ stats: { listed: '2026-11-10T05:18:53Z' } }, now)).toBe(true);
		expect(isNew({ stats: { listed: '2026-11-01T00:00:00Z' } }, now)).toBe(false);
		expect(isNew({}, now)).toBe(false);
		expect(freshUpdate({ updated: '2026-11-15' }, now)).toBe(true);
		expect(freshUpdate({ updated: '2026-11-10' }, now)).toBe(false);
		const list = [mk('a', 5), mk('b', 50), mk('c'), mk('d', 0), mk('e', 9), mk('f', 7)];
		expect([...trendingIds(list)]).toEqual(['b', 'e', 'f']);
		expect(sortPlugins(list, 'trending').map((p) => p.id).slice(0, 2)).toEqual(['b', 'e']);
	});
});

describe('shortDate', () => {
	it('always keeps the year', () => {
		expect(shortDate('2026-11-18', 'en')).toBe('Nov 18, 2026');
		expect(shortDate('2025-03-02', 'en')).toBe('Mar 2, 2025');
		expect(shortDate('bad', 'en')).toBe('');
	});
});

describe('plugin pictures come from this server only', () => {
	it('uses the robot copy of the logo, sized for where it shows', () => {
		const p = { id: 'disk-guard', icon_url: 'images/disk-guard/icon-256.webp' };
		expect(pluginIcon(p, 64)).toBe('/api/v1/plugin-images/images/disk-guard/icon-128.webp');
		expect(pluginIcon(p, 96)).toBe('/api/v1/plugin-images/images/disk-guard/icon-256.webp');
	});
	it('never uses an outside link, another plugin\'s logo or a non-picture data link', () => {
		expect(pluginIcon({ id: 'a', icon_url: 'https://github.com/a.png' })).toBe('');
		expect(pluginIcon({ id: 'a', icon_url: 'images/b/icon-256.webp' })).toBe('');
		expect(pluginIcon({ id: 'a', icon: 'https://example.com/logo.png' })).toBe('');
		expect(pluginIcon({ id: 'a', icon: 'data:text/html;base64,PGI+' })).toBe('');
		expect(pluginIcon({ id: 'a', icon: 'data:image/png;base64,AAAA' })).toBe('data:image/png;base64,AAAA');
	});
	it('gives developer pictures from the robot copies, never github.com', () => {
		expect(devPicture('CoderLuii')).toBe('/api/v1/plugin-images/images/avatars/CoderLuii-64.webp');
		expect(devPicture('../x')).toBe('');
	});
});

describe('folders before start', () => {
	it('holds Start until every required folder has a path', async () => {
		const { foldersMissing } = await import('./view.ts');
		const p: Plugin = { ...base, installed: true, access: { folders: [{ name: 'media', mode: 'read', required: true }, { name: 'cache', mode: 'read_write' }] } } as Plugin;
		expect(foldersMissing(p)).toBe(true);
		expect(foldersMissing({ ...p, grants: { media: { path: '/data/media', mode: 'read' } } } as Plugin)).toBe(false);
		expect(foldersMissing(base)).toBe(false);
	});
});

describe('plugin events', () => {
	const base = { id: 'covers', installed: true, access: { hosts: [], folders: [], actions: [], events: ['library.add', 'library.artwork'] } } as unknown as Plugin;
	it('lists the events it is told, in plain words', () => {
		const line = reaches(base).find((l) => l.includes('a title is added'));
		expect(line).toContain('cover art changes');
	});
	it('names a held event in the approval list', () => {
		expect(needLabel('event:library.delete', base)).toContain('a title is deleted');
		expect(eventLabel('library.someday')).toBe('library.someday');
	});
	it('says nothing about events for a plugin that asks for none', () => {
		expect(reaches({ ...base, access: { hosts: [], folders: [], actions: [] } } as unknown as Plugin).some((l) => l.includes('right away'))).toBe(false);
	});
});

describe('an update that asks for more events', () => {
	it('names only the new events', () => {
		const p = { id: 'covers', installed: true, installed_version: '1.0.0', version: '1.0.1',
			installed_access: { hosts: [], folders: [], actions: [], events: ['library.add'] },
			access: { hosts: [], folders: [], actions: [], events: ['library.add', 'library.delete'] } } as unknown as Plugin;
		const n = newAbilities(p);
		expect(n).toHaveLength(1);
		expect(n[0]).toContain('a title is deleted');
		expect(n[0]).not.toContain('a title is added');
	});
	it('says nothing new when the events stay the same', () => {
		const same = { hosts: [], folders: [], actions: [], events: ['library.add'] };
		expect(newAbilities({ id: 'covers', installed: true, installed_access: same, access: same } as unknown as Plugin)).toEqual([]);
	});
});
