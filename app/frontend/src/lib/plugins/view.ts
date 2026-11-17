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

/** The Remote access state in plain words. A plugin the owner stopped reads as disconnected, whatever its last status says; a signed-out one as not connected. */
export function remoteStateLabel(state: string, p?: Pick<Plugin, 'running' | 'enabled' | 'disabled_reason'>): string {
	if (p && !p.running) {
		if (p.enabled) return t('plugins.remoteStarting');
		return p.disabled_reason === 'signed out by the owner' ? t('plugins.remoteNotConnected') : t('plugins.remotePaused');
	}
	switch (state) {
		case 'waiting_for_sign_in':
			return t('plugins.remoteSignIn');
		case 'waiting_for_admin_approval':
			return t('plugins.remoteAdmin');
		case 'connected':
			return t('plugins.remoteConnected');
		case 'login_expiring':
			return t('plugins.remoteExpiring');
		case 'login_expired':
			return t('plugins.remoteExpired');
		case 'device_deleted':
			return t('plugins.remoteDeleted');
		default:
			return t('plugins.remoteNotConnected');
	}
}

/** Direct or relayed in plain words; empty when Tailscale has not said. */
export function connectionLabel(c: string | undefined): string {
	if (c === 'direct') return t('plugins.remoteDirect');
	if (c === 'relayed') return t('plugins.remoteRelayed');
	return '';
}
