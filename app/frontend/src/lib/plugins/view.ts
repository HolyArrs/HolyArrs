// Pure helpers for the Plugins page: row state, names, notices and limits in plain words.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { ActiveLimits, Plugin, PluginOverview } from './types.ts';

export type PluginState = 'not_installed' | 'running' | 'stopped' | 'off';

/** A newer signed version than the installed one is on the list. */
export const hasUpdate = (p: Plugin): boolean => p.installed && !!p.installed_version && p.installed_version !== p.version;

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

export interface LimitRow {
	label: string;
	value: string;
	on: boolean;
}

/** Every limit a running plugin is held to, with its value; caps this server can't apply say so. Numbers match the Engine's sandbox. */
export function limitRows(l: ActiveLimits): LimitRow[] {
	const off = t('plugins.limitOff');
	const row = (key: string, on: boolean): LimitRow => ({ label: t(`plugins.${key}`), value: on ? t(`plugins.${key}Value`) : off, on });
	return [
		row('limitMemory', l.Memory),
		row('limitCpu', l.CPU),
		row('limitPids', l.Pids),
		row('limitOpenFiles', true),
		row('limitFolders', true),
		{ label: t('plugins.limitNetwork'), value: t('plugins.limitNetworkValue'), on: false },
		row('limitStop', l.Cgroup || l.GroupKill)
	];
}

const known = new Set(['tailscale']);

/** The plugin's description: our own words for plugins we ship, else the signed manifest's. */
export function pluginAbout(p: Plugin): string {
	return known.has(p.id) ? t(`plugins.about.${p.id}`) : (p.description ?? '');
}

/** What the plugin will be able to do once installed, in plain words. */
export function abilities(p: Plugin): string[] {
	const out: string[] = [];
	const hosts = p.access?.hosts ?? [];
	if (hosts.length) out.push(known.has(p.id) ? t(`plugins.can.${p.id}.hosts`) : t('plugins.canHosts', { hosts: hosts.join(', ') }));
	if (p.reaches_app) out.push(known.has(p.id) ? t(`plugins.can.${p.id}.app`) : t('plugins.canApp'));
	out.push(t('plugins.canFolder'));
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

/** The card's note after a sign-out: empty when Tailscale removed the device too. */
export function signOutNotice(res: { removed_remotely?: boolean } | null | undefined): string {
	return res?.removed_remotely === false ? t('plugins.remoteRemoveInAdmin') : '';
}
