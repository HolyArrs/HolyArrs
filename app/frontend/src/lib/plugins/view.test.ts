import { describe, expect, it } from 'vitest';
import { abilities, findPlugins, notices, sortPlugins, reaches, pluginName, pluginState, shownVersion, stateLabel } from './view.ts';
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
	it('lists what a plugin can reach from its manifest, nothing when it declares nothing', () => {
		const ts = { ...base, reaches_app: true, access: { hosts: ['controlplane.tailscale.com'], folders: null, actions: null } };
		expect(reaches(ts)).toEqual(["Connect to Tailscale's servers to sign in and find your devices", 'Let devices on your tailnet open HolyArrs']);
		expect(reaches({ ...base, id: 'x', access: { hosts: ['a.org', 'b.org'], folders: null, actions: null } })).toEqual(['Connect to a.org, b.org']);
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
