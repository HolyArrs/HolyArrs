// Settings page map, who may open each section, and small text helpers.
import { message } from './api.ts';
import type { IconName } from '#lib/ui/Icon.svelte';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type GroupId = 'library' | 'downloads' | 'users' | 'connections' | 'system' | 'account';

export type Tint = 'gray' | 'blue' | 'indigo' | 'purple' | 'pink' | 'red' | 'orange' | 'amber' | 'green' | 'teal' | 'graphite';

export interface Section {
	href: string;
	label: string;
	icon: IconName;
	about: string;
	group: GroupId;
	tint: Tint;
	need?: 'users' | 'self' | 'library';
	feature?: string;
}

export interface Group {
	id: GroupId;
	label: string;
	icon: IconName;
	line: string;
	about: string;
}

type Can = (p: 'settings' | 'users' | 'manage_library') => boolean;

// Set by the session, so this module can ask about held features without importing it.
let heldNow: (feature: string) => boolean = () => false;

// Order here is the order shown on the settings page.
export const groups: Group[] = [
	{ id: 'library', get label() { return t('core.settings.groupLibrary.label'); }, icon: 'library', get line() { return t('core.settings.groupLibrary.line'); }, get about() { return t('core.settings.groupLibrary.about'); } },
	{ id: 'downloads', get label() { return t('core.settings.groupDownloads.label'); }, icon: 'download', get line() { return t('core.settings.groupDownloads.line'); }, get about() { return t('core.settings.groupDownloads.about'); } },
	{ id: 'users', get label() { return t('core.settings.groupUsers.label'); }, icon: 'people', get line() { return t('core.settings.groupUsers.line'); }, get about() { return t('core.settings.groupUsers.about'); } },
	{ id: 'connections', get label() { return t('core.settings.groupLinkedApps.label'); }, icon: 'tv', get line() { return t('core.settings.groupLinkedApps.line'); }, get about() { return t('core.settings.groupLinkedApps.about'); } },
	{ id: 'system', get label() { return t('core.settings.groupSystem.label'); }, icon: 'tools', get line() { return t('core.settings.groupSystem.line'); }, get about() { return t('core.settings.groupSystem.about'); } },
	{ id: 'account', get label() { return t('core.settings.groupAccount.label'); }, icon: 'user', get line() { return t('core.settings.groupAccount.line'); }, get about() { return t('core.settings.groupAccount.about'); } }
];

export const sections: Section[] = [
	{ href: '/settings/library-folders', group: 'library', tint: 'blue', get label() { return t('core.settings.libraryFolders.label'); }, icon: 'folder', get about() { return t('core.settings.libraryFolders.about'); } },
	{ href: '/settings/title-defaults', group: 'library', tint: 'orange', get label() { return t('core.settings.titleDefaults.label'); }, icon: 'tv', get about() { return t('core.settings.titleDefaults.about'); }, need: 'library' },
	{ href: '/settings/music', group: 'library', tint: 'pink', get label() { return t('core.settings.music.label'); }, icon: 'music', get about() { return t('core.settings.music.about'); } },
	{ href: '/settings/subtitles', group: 'library', tint: 'amber', get label() { return t('core.settings.subtitles.label'); }, icon: 'subtitle', get about() { return t('core.settings.subtitles.about'); } },
	{ href: '/settings/naming', group: 'library', tint: 'teal', get label() { return t('core.settings.naming.label'); }, icon: 'film', get about() { return t('core.settings.naming.about'); } },
	{ href: '/settings/parts', group: 'library', tint: 'graphite', get label() { return t('core.settings.parts.label'); }, icon: 'grid', get about() { return t('core.settings.parts.about'); }, need: 'users' },
	{ href: '/settings/outside-lists', group: 'library', tint: 'teal', get label() { return t('core.settings.outsideLists.label'); }, icon: 'inbox', get about() { return t('core.settings.outsideLists.about'); } },
	{ href: '/settings/move', group: 'library', tint: 'teal', get label() { return t('core.settings.move.label'); }, icon: 'upload', get about() { return t('core.settings.move.about'); }, need: 'users' },

	{ href: '/settings/downloaders', group: 'downloads', tint: 'green', get label() { return t('core.settings.downloaders.label'); }, icon: 'downloader', get about() { return t('core.settings.downloaders.about'); } },
	{ href: '/settings/sources', group: 'downloads', tint: 'blue', get label() { return t('core.settings.sources.label'); }, icon: 'source', get about() { return t('core.settings.sources.about'); } },
	{ href: '/settings/pick-rules', group: 'downloads', tint: 'amber', get label() { return t('core.settings.pickRules.label'); }, icon: 'star', get about() { return t('core.settings.pickRules.about'); }, need: 'library' },
	{ href: '/settings/search', group: 'downloads', tint: 'indigo', get label() { return t('core.settings.search.label'); }, icon: 'search', get about() { return t('core.settings.search.about'); } },
	{ href: '/settings/schedules', group: 'downloads', tint: 'orange', get label() { return t('core.settings.schedules.label'); }, icon: 'clock', get about() { return t('core.settings.schedules.about'); } },

	{ href: '/settings/users', group: 'users', tint: 'blue', get label() { return t('core.settings.users.label'); }, icon: 'people', get about() { return t('core.settings.users.about'); }, need: 'users' },
	{ href: '/settings/requests', group: 'users', tint: 'pink', get label() { return t('core.settings.requests.label'); }, icon: 'wish', get about() { return t('core.settings.requests.about'); } },
	{ href: '/settings/sign-in', group: 'users', tint: 'graphite', get label() { return t('core.settings.signIn.label'); }, icon: 'shield', get about() { return t('core.settings.signIn.about'); } },
	{ href: '/settings/appearance', group: 'users', tint: 'indigo', get label() { return t('core.settings.appearance.label'); }, icon: 'appearance', get about() { return t('core.settings.appearance.about'); }, need: 'self' },

	{ href: '/settings/media-servers', group: 'connections', tint: 'purple', get label() { return t('core.settings.mediaServers.label'); }, icon: 'tv', get about() { return t('core.settings.mediaServers.about'); } },
	{ href: '/settings/alerts', group: 'connections', tint: 'red', get label() { return t('core.settings.alerts.label'); }, icon: 'bell', get about() { return t('core.settings.alerts.about'); } },
	{ href: '/settings/ai', group: 'connections', tint: 'indigo', get label() { return t('core.settings.ai.label'); }, icon: 'sparkle', get about() { return t('core.settings.ai.about'); }, feature: 'ai_apps' },
	{ href: '/settings/api', group: 'connections', tint: 'gray', get label() { return t('core.settings.api.label'); }, icon: 'key', get about() { return t('core.settings.api.about'); } },
	{ href: '/settings/tmdb', group: 'connections', tint: 'purple', get label() { return t('core.settings.tmdb.label'); }, icon: 'sparkle', get about() { return t('core.settings.tmdb.about'); } },

	{ href: '/settings/health', group: 'system', tint: 'green', get label() { return t('core.settings.health.label'); }, icon: 'check', get about() { return t('core.settings.health.about'); } },
	{ href: '/settings/backups', group: 'system', tint: 'blue', get label() { return t('core.settings.backups.label'); }, icon: 'archive', get about() { return t('core.settings.backups.about'); } },
	{ href: '/settings/logs', group: 'system', tint: 'gray', get label() { return t('core.settings.logs.label'); }, icon: 'copy', get about() { return t('core.settings.logs.about'); }, need: 'users' },
	{ href: '/settings/this-app', group: 'system', tint: 'gray', get label() { return t('core.settings.thisApp.label'); }, icon: 'info', get about() { return t('core.settings.thisApp.about'); } },
	{ href: '/settings/danger', group: 'system', tint: 'red', get label() { return t('core.settings.danger.label'); }, icon: 'alert', get about() { return t('core.settings.danger.about'); }, need: 'users' },

	{ href: '/settings/account', group: 'account', tint: 'gray', get label() { return t('core.settings.account.label'); }, icon: 'user', get about() { return t('core.settings.account.about'); }, need: 'self' }
];

// Each section may be hidden by a held feature; this is how the session says which.
export function setHeld(fn: (feature: string) => boolean): void {
	heldNow = fn;
}
export function isHeld(feature: string | undefined): boolean {
	return !!feature && heldNow(feature);
}

// Rule: held feature hides it, then the section's own need picks the permission.
export function mayOpen(s: Section, can: Can): boolean {
	if (isHeld(s.feature)) return false;
	if (s.need === 'self') return true;
	if (s.need === 'users') return can('users');
	if (s.need === 'library') return can('settings') || can('manage_library');
	return can('settings');
}

export function visible(all: Section[], can: Can): Section[] {
	return all.filter((s) => mayOpen(s, can));
}

export function sectionFor(all: Section[], path: string): Section | undefined {
	return all.find((s) => s.href === path || path.startsWith(s.href + '/'));
}

export function grouped(all: Section[]): { group: Group; items: Section[] }[] {
	return groups.map((group) => ({ group, items: all.filter((s) => s.group === group.id) })).filter((g) => g.items.length > 0);
}

export function groupOf(id: GroupId | undefined): Group | undefined {
	return groups.find((g) => g.id === id);
}

/** reachWords turns any "couldn't connect" answer into one plain line; other answers pass through. */
export function reachWords(text: string): string {
	return /connection (was )?refused|does not answer|refused to connect|no such host|timed? ?out|unreachable|dial tcp|no route|connection reset|could not (connect|reach)|couldn.t (connect|reach)|can.t connect|EOF/i.test(text)
		? t('core.settings.cantReach')
		: text;
}

// Turns any error into a short sentence with a capital and an end mark.
export function plainMessage(e: unknown): string {
	const text = message(e).trim();
	if (!text) return t('core.api.wentWrong');
	const s = text.charAt(0).toUpperCase() + text.slice(1);
	return /[.!?)]$/.test(s) ? s : s + '.';
}
