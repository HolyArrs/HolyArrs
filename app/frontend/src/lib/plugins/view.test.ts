import { describe, expect, it } from 'vitest';
import { limitLines, notices, pluginName, pluginState, shownVersion, stateLabel } from './view.ts';
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

describe('page notices and limits', () => {
	it('lists every notice that applies, key first', () => {
		expect(notices({ key_set: true, can_run: true, blocklist_stale: false })).toEqual([]);
		expect(notices({ key_set: false, can_run: false, blocklist_stale: true })).toEqual([
			"Plugins can't be installed yet. This HolyArrs build doesn't have the plugin signing key.",
			'Plugins run only on Linux.',
			"Couldn't check for blocked plugins for over a week."
		]);
	});
	it('words only the limits that are on', () => {
		expect(limitLines({ Cgroup: false, Pids: false, Memory: false, CPU: false, GroupKill: true })).toEqual(['Every process it starts stops with it']);
		expect(limitLines({ Cgroup: true, Pids: true, Memory: true, CPU: true, GroupKill: true })).toHaveLength(5);
	});
});
