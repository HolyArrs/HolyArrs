import { message } from './api.ts';
import type { IconName } from './Icon.svelte';

export type GroupId = 'library' | 'downloads' | 'users' | 'connections' | 'system' | 'account';

/** Tile colours for the small icon squares in the Settings menu and home. */
export type Tint = 'gray' | 'blue' | 'indigo' | 'purple' | 'pink' | 'red' | 'orange' | 'amber' | 'green' | 'teal' | 'graphite';

export interface Section {
	href: string;
	label: string;
	icon: IconName;
	about: string;
	group: GroupId;
	tint: Tint;
	/** users: owner and admins; self: everyone; library: settings or manage_library (read only); otherwise the settings permission. */
	need?: 'users' | 'self' | 'library';
}

export interface Group {
	id: GroupId;
	label: string;
	icon: IconName;
	/** Three words on the Settings home card: what the group is for. */
	line: string;
	about: string;
}

type Can = (p: 'settings' | 'users' | 'manage_library') => boolean;

/** mayOpen says whether a person may open one section. */
export function mayOpen(s: Section, can: Can): boolean {
	if (s.need === 'self') return true;
	if (s.need === 'users') return can('users');
	if (s.need === 'library') return can('settings') || can('manage_library');
	return can('settings');
}

/** visible keeps the sections a person may open. */
export function visible(all: Section[], can: Can): Section[] {
	return all.filter((s) => mayOpen(s, can));
}

// Five groups, each a card on the Settings home (Luis, 2026-11-06, design
// settings-views v4): Library, Downloads, People, Linked apps, Server. My
// account lives in the account menu and stays here only for search and the
// page's own group. Pages keep their routes; only the grouping changed, so
// old links still open the same page.
export const groups: Group[] = [
	{ id: 'library', label: 'Library', icon: 'library', line: 'Where titles live', about: 'Folders, parts, defaults, subtitles, file names and moving in.' },
	{ id: 'downloads', label: 'Downloads', icon: 'download', line: 'How titles arrive', about: 'Downloaders, Sources, Pick rules, search and schedules.' },
	{ id: 'users', label: 'People', icon: 'people', line: 'Who comes in', about: 'Users, sign-in, request rules and appearance.' },
	{ id: 'connections', label: 'Linked apps', icon: 'tv', line: 'What it connects', about: 'Media servers, alerts, AI apps, the API and metadata.' },
	{ id: 'system', label: 'Server', icon: 'tools', line: 'Keeps it healthy', about: 'Health, backups, this app, logs and the danger zone.' },
	{ id: 'account', label: 'My account', icon: 'user', line: 'Just you', about: 'Password, two-factor, devices.' }
];

export const sections: Section[] = [
	{ href: '/settings/library-folders', group: 'library', tint: 'blue', label: 'Library folders', icon: 'folder', about: 'Where your titles live' },
	{ href: '/settings/title-defaults', group: 'library', tint: 'orange', label: 'Movies and TV defaults', icon: 'tv', about: 'What new movies and shows get', need: 'library' },
	{ href: '/settings/music', group: 'library', tint: 'pink', label: 'Music defaults', icon: 'music', about: 'What new artists get' },
	{ href: '/settings/subtitles', group: 'library', tint: 'amber', label: 'Subtitles', icon: 'subtitle', about: 'Languages and Subtitle sites' },
	{ href: '/settings/naming', group: 'library', tint: 'teal', label: 'Naming and files', icon: 'film', about: 'File names and free space' },
	{ href: '/settings/parts', group: 'library', tint: 'graphite', label: 'Parts', icon: 'grid', about: 'Turn parts on or off', need: 'users' },
	{ href: '/settings/outside-lists', group: 'library', tint: 'teal', label: 'Outside lists', icon: 'inbox', about: 'Lists that add titles for you' },
	{ href: '/settings/move', group: 'library', tint: 'teal', label: 'Move from other apps', icon: 'upload', about: 'Bring over Sonarr, Radarr and more', need: 'users' },

	{ href: '/settings/downloaders', group: 'downloads', tint: 'green', label: 'Downloaders', icon: 'downloader', about: 'Your torrent and Usenet apps' },
	{ href: '/settings/sources', group: 'downloads', tint: 'blue', label: 'Sources', icon: 'source', about: 'Where HolyArrs looks for Releases' },
	{ href: '/settings/pick-rules', group: 'downloads', tint: 'amber', label: 'Pick rules', icon: 'star', about: 'Which Release to take', need: 'library' },
	{ href: '/settings/search', group: 'downloads', tint: 'indigo', label: 'Search', icon: 'search', about: 'When and how HolyArrs searches' },
	{ href: '/settings/schedules', group: 'downloads', tint: 'orange', label: 'Schedules', icon: 'clock', about: 'Timed jobs and Run now' },

	{ href: '/settings/users', group: 'users', tint: 'blue', label: 'Users and roles', icon: 'people', about: 'Invite people, set roles', need: 'users' },
	{ href: '/settings/wishlist', group: 'users', tint: 'pink', label: 'Request rules', icon: 'wish', about: 'Limits and what requests get' },
	{ href: '/settings/sign-in', group: 'users', tint: 'graphite', label: 'Sign-in and security', icon: 'shield', about: 'Passwords, two-factor, local network' },
	{ href: '/settings/appearance', group: 'users', tint: 'indigo', label: 'Appearance', icon: 'appearance', about: 'Theme and title pages', need: 'self' },

	{ href: '/settings/media-servers', group: 'connections', tint: 'purple', label: 'Media servers', icon: 'tv', about: 'Plex, Jellyfin and Emby' },
	{ href: '/settings/alerts', group: 'connections', tint: 'red', label: 'Alerts', icon: 'bell', about: 'Discord, Telegram, email and more' },
	{ href: '/settings/ai', group: 'connections', tint: 'indigo', label: 'AI apps', icon: 'sparkle', about: 'Let AI assistants use HolyArrs' },
	{ href: '/settings/api', group: 'connections', tint: 'gray', label: 'API', icon: 'key', about: 'Keys and routes for other apps' },
	{ href: '/settings/tmdb', group: 'connections', tint: 'purple', label: 'Metadata', icon: 'sparkle', about: 'Where posters and names come from' },


	{ href: '/settings/health', group: 'system', tint: 'green', label: 'Health checks', icon: 'check', about: "What's working and what isn't" },
	{ href: '/settings/backups', group: 'system', tint: 'blue', label: 'Backups', icon: 'archive', about: 'Save and restore HolyArrs' },
	{ href: '/settings/logs', group: 'system', tint: 'gray', label: 'Logs', icon: 'copy', about: 'Read and download the app log', need: 'users' },
	{ href: '/settings/this-app', group: 'system', tint: 'gray', label: 'This app', icon: 'info', about: 'Version, updates and storage' },
	{ href: '/settings/danger', group: 'system', tint: 'red', label: 'Danger zone', icon: 'alert', about: 'Restart or shut down', need: 'users' },

	{ href: '/settings/account', group: 'account', tint: 'gray', label: 'My account', icon: 'user', about: 'Password, two-factor, devices', need: 'self' }
];

/** sectionFor finds the page a path belongs to (a source's health page belongs to Sources). */
export function sectionFor(all: Section[], path: string): Section | undefined {
	return all.find((s) => s.href === path || path.startsWith(s.href + '/'));
}

/** grouped returns each group with the visible pages inside it, leaving out empty groups. */
export function grouped(all: Section[]): { group: Group; items: Section[] }[] {
	return groups.map((group) => ({ group, items: all.filter((s) => s.group === group.id) })).filter((g) => g.items.length > 0);
}

/** groupOf finds a group by id. */
export function groupOf(id: GroupId | undefined): Group | undefined {
	return groups.find((g) => g.id === id);
}

/** filterSections keeps the pages whose name, words or group hold every word typed. */
export function filterSections(all: Section[], query: string): Section[] {
	const words = query.toLowerCase().split(/\s+/).filter(Boolean);
	if (!words.length) return all;
	return all.filter((s) => {
		const hay = `${s.label} ${s.about} ${groups.find((g) => g.id === s.group)?.label ?? ''}`.toLowerCase();
		return words.every((w) => hay.includes(w));
	});
}

/** plainMessage is message() for Settings: server text starts with a capital
 *  and ends with a stop, so an error never shows as a bare lowercase line. */
export function plainMessage(e: unknown): string {
	const t = message(e).trim();
	if (!t) return 'Something went wrong.';
	const s = t.charAt(0).toUpperCase() + t.slice(1);
	return /[.!?)]$/.test(s) ? s : s + '.';
}
