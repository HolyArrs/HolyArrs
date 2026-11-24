import { describe, expect, it } from 'vitest';
import { moveIndex, newAbilities, type Plugin, type PluginOverview } from './index.ts';
import { applyScenario, scenarioNames } from './scenarios.svelte.ts';

const p = (over: Partial<Plugin>): Plugin => ({ id: 'x', version: '1.1.0', installed: true, installed_version: '1.0.0', running: true, enabled: true, access: { hosts: ['a.example', 'b.example'], folders: [], actions: [] }, ...over });

describe('newAbilities', () => {
	it('lists only what the update adds', () => {
		expect(newAbilities(p({ installed_access: { hosts: ['a.example', 'b.example'], folders: [], actions: [] } }))).toEqual([]);
		expect(newAbilities(p({ installed_access: { hosts: [], folders: [], actions: [] } })).join(' ')).toContain('a.example');
	});
	it('asks nothing for a plugin not installed or without the installed access', () => {
		expect(newAbilities(p({ installed: false, installed_access: { hosts: [], folders: [], actions: [] } }))).toEqual([]);
		expect(newAbilities(p({}))).toEqual([]);
	});
});

describe('moveIndex', () => {
	// Three per row: 0 1 2 / 3 4
	const grid = [0, 1, 2, 3, 4].map((i) => ({ left: (i % 3) * 100, top: Math.floor(i / 3) * 100 }));
	it('moves along, down and up a grid and stops at the ends', () => {
		expect(moveIndex(grid, 0, 'ArrowRight')).toBe(1);
		expect(moveIndex(grid, 0, 'ArrowLeft')).toBe(0);
		expect(moveIndex(grid, 1, 'ArrowDown')).toBe(4);
		expect(moveIndex(grid, 2, 'ArrowDown')).toBe(4);
		expect(moveIndex(grid, 4, 'ArrowUp')).toBe(1);
		expect(moveIndex(grid, 3, 'End')).toBe(4);
		expect(moveIndex(grid, -1, 'ArrowRight')).toBe(-1);
	});
});

describe('applyScenario', () => {
	const ov: PluginOverview = { key_set: true, can_run: true, blocklist_checked_at: null, blocklist_stale: false, plugins: Array.from({ length: 12 }, (_, i) => p({ id: `p${i}`, installed: false, installed_version: undefined, running: false, enabled: false })) } as PluginOverview;
	it('covers every named scenario without touching the live list', () => {
		for (const [s] of scenarioNames) {
			if (!s) continue;
			const r = applyScenario(ov, s);
			expect(r.ov).not.toBe(ov);
		}
		expect(ov.plugins.every((x) => !x.installed)).toBe(true);
	});
	it('dresses the states the dialog shows', () => {
		const find = (s: Parameters<typeof applyScenario>[1]) => {
			const r = applyScenario(ov, s);
			return { r, x: r.ov.plugins.find((y) => y.id === r.open)! };
		};
		expect(find('installing').x.install_step).toBe('checking');
		expect(find('install-failed').x.install_error).toBeTruthy();
		expect(find('needs-app').x.needs_app).toBe('9.9.0');
		expect(newAbilities(find('update-access').x).length).toBeGreaterThan(0);
		expect(find('no-lock').r.ov.no_file_lock).toBe(true);
		expect(find('empty-match').r.search).toBeTruthy();
	});
});
