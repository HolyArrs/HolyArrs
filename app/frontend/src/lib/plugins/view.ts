// Pure helpers for the Plugins page: row state, names, notices and what each plugin can reach, in plain words.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { toOwn } from '#lib/core/paths.ts';
import { SvelteSet } from 'svelte/reactivity';
import type { Plugin, PluginOverview, PluginResources, ScopeGroup } from './types.ts';

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

/** Why a plugin is off, in plain words: the backend's lock errors name kernel features, so they get the app's own wording. */
export function offReason(reason: string): string {
	if (reason.includes('(seccomp user-notify)')) return t('plugins.noNetCardTitle');
	if (reason.includes('(Landlock)')) return t('plugins.noLockTag');
	return reason;
}

export function stateLabel(p: Plugin): string {
	const s = pluginState(p);
	if (s === 'not_installed') return t('plugins.stateNotInstalled');
	if (s === 'running') return t('plugins.stateRunning');
	if (s === 'stopped') return t('plugins.stateStopped');
	return p.disabled_reason ? t('plugins.stateOffWhy', { reason: offReason(p.disabled_reason) }) : t('plugins.stateOff');
}

/** The version to show: the installed one when it is installed. */
export function shownVersion(p: Plugin): string {
	return p.installed && p.installed_version ? p.installed_version : p.version;
}

/** Page notices, most blocking first. The missing file lock has its own card. */
export function notices(ov: Pick<PluginOverview, 'key_set' | 'can_run' | 'blocklist_stale' | 'list_bad'>): string[] {
	const out: string[] = [];
	if (!ov.key_set) out.push(t('plugins.noKey'));
	if (!ov.can_run) out.push(t('plugins.linuxOnly'));
	if (ov.list_bad) out.push(t('plugins.listBad'));
	if (ov.blocklist_stale) out.push(t('plugins.blocklistStale'));
	return out;
}

/** Why the file lock is missing, with the fix for a container or boot setting, else the old kernel. */
export function noLockNotice(reason: PluginOverview['file_lock_reason']): string {
	if (reason === 'blocked') return t('plugins.noLockBlocked');
	if (reason === 'turned_off') return t('plugins.noLockTurnedOff');
	return t('plugins.noLockOldKernel');
}

/** Why the network lock is missing, with what to check. */
export function netLockNotice(reason: PluginOverview['net_lock_reason']): string {
	if (reason === 'blocked') return t('plugins.netLockBlocked');
	if (reason === 'turned_off') return t('plugins.netLockTurnedOff');
	return t('plugins.netLockOldKernel');
}

/** The network lock card shows when plugins can run here but the system can't keep them to their websites. */
export const showsNoNetLockCard = (ov: Pick<PluginOverview, 'can_run' | 'no_net_lock'>): boolean => !!(ov.can_run && ov.no_net_lock);

/** A folder request in plain words, with its mode. */
export function folderLabel(f: { name: string; mode: string }): string {
	return t(f.mode === 'read_write' ? 'plugins.folderReadWrite' : 'plugins.folderRead', { name: f.name });
}

/** One item of needs_approval in plain words. */
export function needLabel(need: string, p: Plugin): string {
	const [kind, ...rest] = need.split(':');
	const name = rest.join(':');
	if (kind === 'host') return t('plugins.canHosts', { hosts: name });
	if (kind === 'scope') {
		const g = p.scope_groups?.find((x) => x.scopes.includes(name));
		return g ? `${scopeLine(name, g.id)} (${name})` : t('plugins.needScope', { scope: name });
	}
	if (kind === 'resources') return t('plugins.needResources');
	const f = p.access?.folders?.find((x) => x.name === name);
	return f ? folderLabel(f) : name;
}

/** The file lock card shows when plugins can run here but the system can't lock their files. */
export const showsNoLockCard = (ov: Pick<PluginOverview, 'can_run' | 'no_file_lock'>): boolean => !!(ov.can_run && ov.no_file_lock);

/** Plugins carry the "No file lock on this system" tag once the owner let them run without it. */
export const showsNoLockTag = (ov: Pick<PluginOverview, 'can_run' | 'no_file_lock' | 'allow_no_file_lock'>): boolean => showsNoLockCard(ov) && !!ov.allow_no_file_lock;

const known = new Set(['tailscale']);

/** The plugin's description: our own words for plugins we ship, else the signed manifest's. */
export function pluginAbout(p: Plugin): string {
	return known.has(p.id) ? t(`plugins.about.${p.id}`) : (p.description ?? '');
}

/** Actions with our own plain words; any other shows as the manifest wrote it. */
const knownActions = new Set(['library.read', 'watched.write', 'activity.read', 'requests.create', 'requests.read', 'requests.approve', 'collections.write', 'downloads.read', 'downloads.pause', 'ui.theme']);

/** What the signed manifest lets the plugin reach outside its own folder, in plain words. */
export function reaches(p: Plugin): string[] {
	const out: string[] = [];
	const hosts = p.access?.hosts ?? [];
	if (hosts.length) {
		if (known.has(p.id)) out.push(t(`plugins.can.${p.id}.hosts`));
		else if (hosts.includes('*')) out.push(t('plugins.canAnyHost'));
		else out.push(t('plugins.canHosts', { hosts: shortList(hosts) }));
		if (!hosts.includes('*')) out.push(t('plugins.netLocked'));
	}
	if (p.reaches_app) out.push(known.has(p.id) ? t(`plugins.can.${p.id}.app`) : t('plugins.canApp'));
	if (p.scope_groups) out.push(...p.scope_groups.map(groupLine));
	else for (const a of p.access?.actions ?? []) out.push(knownActions.has(a) ? t(`plugins.act.${a.replace('.', '_')}`) : t('plugins.canAction', { action: a }));
	const folders = (p.access?.folders ?? []).map((f) => (typeof f === 'string' ? { name: f, mode: 'read' } : f));
	if (folders.length) out.push(t('plugins.canFolders', { folders: shortList(folders.map(folderLabel)) }));
	return out;
}

/** A long list cut to its first five, with how many more: plugins may list up to 100 hosts or folders. */
export function shortList(items: string[], keep = 5): string {
	return items.length > keep ? `${items.slice(0, keep).join(', ')} +${items.length - keep}` : items.join(', ');
}

/** One install group in plain words: "Can see ..." or "Can see and change ...". */
export function groupLine(g: ScopeGroup): string {
	return t(`plugins.group.${g.id}.${g.change ? 'change' : 'read'}`);
}

/** One granted scope in plain words: its group's line for reading or changing. */
export function scopeLine(scope: string, groupId: string): string {
	return t(`plugins.group.${groupId}.${scope.endsWith('.read') ? 'read' : 'change'}`);
}

/** Every scope of a plugin with its plain line, so none hides behind a group. */
export function scopeLines(groups: ScopeGroup[] | undefined): { scope: string; line: string }[] {
	return (groups ?? []).flatMap((g) => g.scopes.map((s) => ({ scope: s, line: scopeLine(s, g.id) })));
}

/** The red install warnings of a plugin's scopes, each once. */
export function scopeWarnings(groups: ScopeGroup[] | undefined): string[] {
	return [...new Set((groups ?? []).flatMap((g) => g.warnings ?? []))].map((w) => t(`plugins.warn.${w}`));
}

/** The caps a plugin asks for in plain words, defaults filled in; empty when it asks for none. */
export function resourcesLine(r: PluginResources | undefined): string {
	if (!r) return '';
	return t('plugins.resourcesAsked', { memory: r.memory_mib ?? 512, cpus: r.cpus ?? 1, processes: r.processes ?? 256, files: r.open_files ?? 1024 });
}

/** What the plugin will be able to do once installed, in plain words. */
export function abilities(p: Plugin): string[] {
	return [...reaches(p), t('plugins.canFolder')];
}

/** What an update can do that the installed version can't, in plain words; empty when it asks for nothing new. */
export function newAbilities(p: Plugin): string[] {
	if (!p.installed || !p.installed_access) return [];
	const had = new Set(reaches({ ...p, access: p.installed_access }));
	return reaches(p).filter((a) => !had.has(a));
}

/** Arrow-key target in a list or grid of items: the next, previous, or the one above or below (same column). */
export function moveIndex(rects: { left: number; top: number }[], from: number, key: string): number {
	const n = rects.length;
	if (n === 0 || from < 0) return -1;
	if (key === 'ArrowRight') return Math.min(n - 1, from + 1);
	if (key === 'ArrowLeft') return Math.max(0, from - 1);
	if (key === 'Home') return 0;
	if (key === 'End') return n - 1;
	const cols = Math.max(1, rects.filter((r) => Math.abs(r.top - rects[0].top) < 2).length);
	if (key === 'ArrowDown') return Math.min(n - 1, from + cols);
	if (key === 'ArrowUp') return Math.max(0, from - cols);
	return from;
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

export type PluginSort = 'name' | 'new' | 'trending';
export type PluginView = 'grid' | 'list';

/** Plugins whose name or description holds every word typed. */
export function findPlugins(list: Plugin[], q: string): Plugin[] {
	const words = q.toLowerCase().split(/\s+/).filter(Boolean);
	if (!words.length) return list;
	return list.filter((p) => {
		const hay = `${shownName(p)} ${pluginAbout(p)} ${p.category ? categoryLabel(categoryOf(p)) : ''} ${p.category ?? ''} ${p.author ?? ''} ${(p.developers ?? []).join(' ')}`.toLowerCase();
		return words.every((w) => hay.includes(w));
	});
}

/** Name A to Z, or newest first: by the manifest's updated date, else the list's own order (later entries were added later). */
export function sortPlugins(list: Plugin[], by: PluginSort): Plugin[] {
	if (by === 'trending') return [...list].sort((a, b) => (b.stats?.week ?? -1) - (a.stats?.week ?? -1) || shownName(a).localeCompare(shownName(b)));
	if (by === 'new') {
		const order = new Map(list.map((p, i) => [p.id, i]));
		return [...list].sort((a, b) => (b.updated ?? '').localeCompare(a.updated ?? '') || order.get(b.id)! - order.get(a.id)!);
	}
	return [...list].sort((a, b) => shownName(a).localeCompare(shownName(b)));
}

/** The plugin's name: the signed manifest's, else our own or a tidied id. */
export function shownName(p: Pick<Plugin, 'id' | 'name'>): string {
	return p.name?.trim() || pluginName(p.id);
}

/** The tab a plugin sits under: its manifest category, or Other. */
export const categoryOf = (p: Plugin): string => p.category?.trim() || 'Other';

const knownCategories = new Set(['sync', 'chat', 'ai', 'lists', 'home', 'library', 'downloads', 'subtitles', 'backup', 'automation', 'requests', 'music', 'look', 'network', 'other']);

/** A category in plain words: ours for the known ones, else as the manifest wrote it. */
export function categoryLabel(c: string): string {
	const k = c.toLowerCase();
	return knownCategories.has(k) ? t(`plugins.cat.${k}`) : c;
}

/** The categories present, Other last. */
export function categories(list: Plugin[]): string[] {
	const set = [...new Set(list.map(categoryOf))];
	return set.sort((a, b) => (a === 'Other' ? 1 : b === 'Other' ? -1 : categoryLabel(a).localeCompare(categoryLabel(b))));
}

export type PluginTab = 'all' | 'installed' | `cat:${string}`;

/** The plugins under one tab. */
export function inTab(list: Plugin[], tab: PluginTab): Plugin[] {
	if (tab === 'all') return list;
	if (tab === 'installed') return list.filter((p) => p.installed);
	return list.filter((p) => categoryOf(p) === tab.slice(4));
}

/** The developers to show: the manifest's GitHub logins, else the author when it reads as one. */
export function developers(p: Pick<Plugin, 'developers' | 'author'>): string[] {
	if (p.developers?.length) return p.developers.filter(isLogin);
	return p.author && isLogin(p.author) ? [p.author] : [];
}
const isLogin = (s: string) => /^[A-Za-z0-9][A-Za-z0-9-]{0,38}$/.test(s);

/** A CPU type in plain words. */
export function platformLabel(p: string): string {
	if (p === 'linux-amd64') return t('plugins.cpuAmd64');
	if (p === 'linux-arm64') return t('plugins.cpuArm64');
	if (p === 'linux-arm-v7') return t('plugins.cpuArmV7');
	return p;
}

/** Only https links are shown as links. */
export const safeLink = (u: string | undefined): string => (u && /^https:\/\/[^\s"'<>]+$/.test(u) ? u : '');

const DAY = 86_400_000;
const dayMs = (d?: string) => (d && /^\d{4}-\d{2}-\d{2}/.test(d) ? Date.parse(d.slice(0, 10) + 'T00:00:00Z') : NaN);

/** First listed in the last 14 days. */
export function isNew(p: Pick<Plugin, 'stats'>, now = Date.now()): boolean {
	const at = dayMs(p.stats?.listed);
	return at <= now && now - at < 14 * DAY;
}

/** Released in the last 7 days (the Updated chip turns green). */
export function freshUpdate(p: Pick<Plugin, 'updated'>, now = Date.now()): boolean {
	const at = dayMs(p.updated);
	return at <= now && now - at < 7 * DAY;
}

/** The few plugins with the most downloads this week (any at all). */
export function trendingIds(list: Plugin[], top = 3): Set<string> {
	return new Set(list.filter((p) => (p.stats?.week ?? 0) > 0).sort((a, b) => b.stats!.week! - a.stats!.week!).slice(0, top).map((p) => p.id));
}

/** A count in the app's language, short: 1.2K. */
export function shortCount(n: number, locale?: string): string {
	return new Intl.NumberFormat(locale, { notation: 'compact', maximumFractionDigits: 1 }).format(n);
}

/** A YYYY-MM-DD day for a chip, always with its year: "Nov 18, 2026" (localised). */
export function shortDate(day: string, locale?: string): string {
	const d = new Date(day + 'T12:00:00Z');
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleDateString(locale, { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' });
}

/** The numeric short form with the year (11/19/2026 in en-US), for a card footer short on room. */
export function numDate(day: string, locale?: string): string {
	const d = new Date(day + 'T12:00:00Z');
	if (Number.isNaN(d.getTime())) return '';
	return d.toLocaleDateString(locale, { month: 'numeric', day: 'numeric', year: 'numeric', timeZone: 'UTC' });
}

// Logos and developer pictures come through this server from the plugins robot's copies on
// plugins.holyarrs.com, so the browser never asks github.com or an author's site for them.
/** Picture addresses that failed to load; they show a letter instead. */
export const brokenPics = new SvelteSet<string>();
const iconUrlRe = /^images\/([a-z0-9][a-z0-9-]{0,63})\/icon-256\.webp$/;

/** The plugin's logo: the robot's copy (128 px up to 64 px shown, else 256 px), an older inline icon, or '' for the letter. */
export function pluginIcon(p: Pick<Plugin, 'id' | 'icon' | 'icon_url'>, size = 64): string {
	const m = p.icon_url ? iconUrlRe.exec(p.icon_url) : null;
	if (m && m[1] === p.id) return toOwn(`/api/v1/plugin-images/images/${p.id}/icon-${size <= 64 ? 128 : 256}.webp`);
	return p.icon?.startsWith('data:image/') ? p.icon : '';
}

/** A developer's picture, as the robot copied it from GitHub; '' for a name that is not a GitHub login. */
/** The project itself (HolyArrs): its logo shows bare, with no circle behind it. */
export const isProject = (login: string): boolean => login.trim().toLowerCase() === 'holyarrs';

/** Where a developer's name links: HolyArrs to its app repo, anyone else to their GitHub profile. */
export function devLink(login: string): string {
	return login.trim().toLowerCase() === 'holyarrs' ? 'https://github.com/HolyArrs/HolyArrs' : `https://github.com/${login}`;
}

export function devPicture(login: string): string {
	// The project itself always shows its own logo.
	if (login.trim().toLowerCase() === 'holyarrs') return toOwn('/brand/mark-color.svg');
	return isLogin(login) ? toOwn(`/api/v1/plugin-images/images/avatars/${login}-64.webp`) : '';
}

/** A required folder has no path yet: the server refuses to start it, so Start waits too. */
export function foldersMissing(p: Plugin): boolean {
	return (p.access?.folders ?? []).some((f) => typeof f !== 'string' && !!f.required && !p.grants?.[f.name]?.path?.trim());
}

/** Whether a plugin forwards HolyArrs from outside (it asks for network.forward). */
export function forwards(p: { access?: { actions?: string[] | null } | null }): boolean {
	return !!p.access?.actions?.includes('network.forward');
}
