// Lab-only (dev server): the Design preview bar's Scenarios pill dresses the real plugin list
// as one state at a time, so every case can be looked at without breaking a real install.
// Nothing here reaches the server; it never runs in a release build.
import type { Plugin, PluginOverview } from './types.ts';

export const scenarioNames = [
	['', 'Live data'],
	['not-installed', 'Not installed'],
	['installing', 'Installing (Checking step)'],
	['running', 'Running'],
	['stopped', 'Stopped'],
	['install-failed', 'Install failed (Retry)'],
	['start-failed', 'Failed to start'],
	['update', 'Update available'],
	['update-access', 'Update asks for more access'],
	['blocked', 'Pulled by the list'],
	['no-lock', 'No file lock on this system'],
	['no-net-lock', 'No network lock on this system'],
	['needs-approval', 'Update asks for a folder and a host'],
	['needs-app', 'Needs a newer HolyArrs'],
	['empty-match', 'No plugins match'],
	['first-visit', 'First visit (nothing installed)']
] as const;
export type Scenario = (typeof scenarioNames)[number][0];

/** ov dressed as the scenario, and the plugin whose dialog shows it ('' for page-level ones). */
export function applyScenario(ov: PluginOverview, s: Scenario): { ov: PluginOverview; open: string; search?: string } {
	const o: PluginOverview = structuredClone($state.snapshot(ov) as PluginOverview);
	const list = o.plugins.filter((p) => p.id !== 'tailscale');
	const pick = (i: number): Plugin | undefined => list[i % Math.max(1, list.length)];
	const base = (p: Plugin | undefined, patch: Partial<Plugin>) => {
		if (!p) return '';
		Object.assign(p, { installed: false, installed_version: undefined, running: false, enabled: false, disabled_reason: undefined, install_step: undefined, install_error: undefined, installed_access: undefined, needs_app: undefined }, patch);
		return p.id;
	};
	const old = (v: string) => v.replace(/(\d+)$/, (n) => String(Math.max(0, Number(n) - 1)));
	switch (s) {
		case 'not-installed':
			return { ov: o, open: base(pick(0), {}) };
		case 'installing':
			return { ov: o, open: base(pick(1), { install_step: 'checking' }) };
		case 'running':
			return { ov: o, open: base(pick(2), { installed: true, installed_version: pick(2)?.version, running: true, enabled: true }) };
		case 'stopped':
			return { ov: o, open: base(pick(3), { installed: true, installed_version: pick(3)?.version, disabled_reason: 'stopped by the owner' }) };
		case 'install-failed':
			return { ov: o, open: base(pick(4), { install_error: 'could not fetch from the plugin list server: connection refused' }) };
		case 'start-failed':
			return { ov: o, open: base(pick(5), { installed: true, installed_version: pick(5)?.version, disabled_reason: 'plugin program changed since install' }) };
		case 'update':
			return { ov: o, open: base(pick(6), { installed: true, installed_version: old(pick(6)?.version ?? '1.0.1'), running: true, enabled: true, installed_access: pick(6)?.access ?? undefined }) };
		case 'update-access': {
			const p = pick(7);
			return { ov: o, open: base(p, { installed: true, installed_version: old(p?.version ?? '1.0.1'), running: true, enabled: true, installed_access: { hosts: [], folders: [], actions: (p?.access?.actions ?? []).slice(0, 1) } }) };
		}
		case 'blocked':
			return { ov: o, open: base(pick(8), { installed: true, installed_version: pick(8)?.version, disabled_reason: 'on the blocklist: sends library data to a host it did not declare' }) };
		case 'no-lock':
			Object.assign(o, { no_file_lock: true, file_lock_reason: 'blocked', allow_no_file_lock: false, no_file_lock_asked: false });
			return { ov: o, open: '' };
		case 'no-net-lock':
			Object.assign(o, { no_net_lock: true, net_lock_reason: 'old_kernel', allow_no_net_lock: false, no_net_lock_asked: false });
			return { ov: o, open: '' };
		case 'needs-approval': {
			const p = pick(10);
			if (p) p.access = { hosts: [...(p.access?.hosts ?? []), 'api.example.com'], actions: p.access?.actions ?? [], folders: [{ name: 'music', mode: 'read', why: 'to read song tags', required: true }, { name: 'downloads', mode: 'read_write' }] };
			return { ov: o, open: base(p, { installed: true, installed_version: p?.version, disabled_reason: 'this plugin asks for more access: approve it on the Plugins page', needs_approval: ['host:api.example.com', 'folder:music', 'folder:downloads'], blocked_count: 3 }) };
		}
		case 'needs-app':
			return { ov: o, open: base(pick(9), { needs_app: '9.9.0' }) };
		case 'empty-match':
			return { ov: o, open: '', search: 'nothing matches this' };
		case 'first-visit':
			for (const p of o.plugins) base(p, {});
			return { ov: o, open: '' };
	}
	return { ov: o, open: '' };
}
