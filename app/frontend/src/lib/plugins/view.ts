// Pure helpers for the Plugins page: row state, names, notices and what each plugin can reach, in plain words.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Plugin, PluginOverview } from './types.ts';

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
export function notices(ov: Pick<PluginOverview, 'key_set' | 'can_run' | 'blocklist_stale' | 'list_bad'>): string[] {
	const out: string[] = [];
	if (!ov.key_set) out.push(t('plugins.noKey'));
	if (!ov.can_run) out.push(t('plugins.linuxOnly'));
	if (ov.list_bad) out.push(t('plugins.listBad'));
	if (ov.blocklist_stale) out.push(t('plugins.blocklistStale'));
	return out;
}

const known = new Set(['tailscale']);

/** The plugin's description: our own words for plugins we ship, else the signed manifest's. */
export function pluginAbout(p: Plugin): string {
	return known.has(p.id) ? t(`plugins.about.${p.id}`) : (p.description ?? '');
}

/** What the signed manifest lets the plugin reach outside its own folder, in plain words. */
export function reaches(p: Plugin): string[] {
	const out: string[] = [];
	const hosts = p.access?.hosts ?? [];
	if (hosts.length) out.push(known.has(p.id) ? t(`plugins.can.${p.id}.hosts`) : t('plugins.canHosts', { hosts: hosts.join(', ') }));
	if (p.reaches_app) out.push(known.has(p.id) ? t(`plugins.can.${p.id}.app`) : t('plugins.canApp'));
	return out;
}

/** What the plugin will be able to do once installed, in plain words. */
export function abilities(p: Plugin): string[] {
	return [...reaches(p), t('plugins.canFolder')];
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

export type PluginSort = 'name' | 'new';
export type PluginView = 'grid' | 'list';

/** Plugins whose name or description holds every word typed. */
export function findPlugins(list: Plugin[], q: string): Plugin[] {
	const words = q.toLowerCase().split(/\s+/).filter(Boolean);
	if (!words.length) return list;
	return list.filter((p) => {
		const hay = `${pluginName(p.id)} ${p.name ?? ''} ${pluginAbout(p)}`.toLowerCase();
		return words.every((w) => hay.includes(w));
	});
}

/** Name A to Z, or newest first. The signed list carries no date, so newest is the list's own order: later entries were added later. */
export function sortPlugins(list: Plugin[], by: PluginSort): Plugin[] {
	if (by === 'new') return [...list].reverse();
	return [...list].sort((a, b) => pluginName(a.id).localeCompare(pluginName(b.id)));
}
