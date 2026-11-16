// Pure helpers for the Plugins page: row state, names, notices and limits in plain words.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { ActiveLimits, Plugin, PluginOverview } from './types.ts';

export type PluginState = 'not_installed' | 'running' | 'stopped' | 'off';

const names: Record<string, string> = { tailscale: 'Tailscale', mcp: 'MCP Server', 'mcp-server': 'MCP Server' };

export function pluginName(id: string): string {
	return names[id] ?? id.replace(/[-_]+/g, ' ').replace(/^\w/, (c) => c.toUpperCase());
}

export function pluginState(p: Plugin): PluginState {
	if (!p.installed) return 'not_installed';
	if (!p.enabled) return 'off';
	return p.running ? 'running' : 'stopped';
}

export function stateLabel(p: Plugin): string {
	const s = pluginState(p);
	if (s === 'not_installed') return t('plugins.stateNotInstalled');
	if (s === 'running') return t('plugins.stateRunning');
	if (s === 'stopped') return t('plugins.stateStopped');
	return p.disabled_reason ? t('plugins.stateOffWhy', { reason: p.disabled_reason }) : t('plugins.stateOff');
}

/** The version to show: the installed one when it is installed. */
export function shownVersion(p: Plugin): string {
	return p.installed && p.installed_version ? p.installed_version : p.version;
}

/** Page notices, most blocking first. */
export function notices(ov: Pick<PluginOverview, 'key_set' | 'can_run' | 'blocklist_stale'>): string[] {
	const out: string[] = [];
	if (!ov.key_set) out.push(t('plugins.noKey'));
	if (!ov.can_run) out.push(t('plugins.linuxOnly'));
	if (ov.blocklist_stale) out.push(t('plugins.blocklistStale'));
	return out;
}

/** Each limit the Engine puts on a running plugin, as a short line. */
export function limitLines(l: ActiveLimits): string[] {
	const out: string[] = [];
	if (l.Cgroup) out.push(t('plugins.limitGroup'));
	if (l.Pids) out.push(t('plugins.limitPids'));
	if (l.Memory) out.push(t('plugins.limitMemory'));
	if (l.CPU) out.push(t('plugins.limitCpu'));
	if (l.GroupKill) out.push(t('plugins.limitKill'));
	return out;
}
