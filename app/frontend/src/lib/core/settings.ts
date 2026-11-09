// Settings page map, who may open each section, and small text helpers.
import { message } from './api.ts';
import type { IconName } from '#lib/ui/Icon.svelte';

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
	{ id: 'library', label: 'Library', icon: 'library', line: 'Where titles live', about: 'Folders, parts, defaults, subtitles, file names and moving in.' },
	{ id: 'downloads', label: 'Downloads', icon: 'download', line: 'How titles arrive', about: 'Downloaders, Sources, quality, search and schedules.' },
	{ id: 'users', label: 'People', icon: 'people', line: 'Who comes in', about: 'Users, sign-in, request rules and appearance.' },
	{ id: 'connections', label: 'Linked apps', icon: 'tv', line: 'What it connects', about: 'Media servers, alerts, the API and metadata.' },
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
	{ href: '/settings/pick-rules', group: 'downloads', tint: 'amber', label: 'Quality', icon: 'star', about: 'Which Release to take', need: 'library' },
	{ href: '/settings/search', group: 'downloads', tint: 'indigo', label: 'Search', icon: 'search', about: 'When and how HolyArrs searches' },
	{ href: '/settings/schedules', group: 'downloads', tint: 'orange', label: 'Schedules', icon: 'clock', about: 'Timed jobs and Run now' },

	{ href: '/settings/users', group: 'users', tint: 'blue', label: 'Users and roles', icon: 'people', about: 'Invite people, set roles', need: 'users' },
	{ href: '/settings/requests', group: 'users', tint: 'pink', label: 'Request rules', icon: 'wish', about: 'Limits and what requests get' },
	{ href: '/settings/sign-in', group: 'users', tint: 'graphite', label: 'Sign-in and security', icon: 'shield', about: 'Passwords, two-factor, local network' },
	{ href: '/settings/appearance', group: 'users', tint: 'indigo', label: 'Appearance', icon: 'appearance', about: 'Theme and title pages', need: 'self' },

	{ href: '/settings/media-servers', group: 'connections', tint: 'purple', label: 'Media servers', icon: 'tv', about: 'Plex, Jellyfin and Emby' },
	{ href: '/settings/alerts', group: 'connections', tint: 'red', label: 'Alerts', icon: 'bell', about: 'Discord, Telegram, email and more' },
	{ href: '/settings/ai', group: 'connections', tint: 'indigo', label: 'AI apps', icon: 'sparkle', about: 'Let AI assistants use HolyArrs', feature: 'ai_apps' },
	{ href: '/settings/api', group: 'connections', tint: 'gray', label: 'API', icon: 'key', about: 'Keys and routes for other apps' },
	{ href: '/settings/tmdb', group: 'connections', tint: 'purple', label: 'Metadata', icon: 'sparkle', about: 'Where posters and names come from' },

	{ href: '/settings/health', group: 'system', tint: 'green', label: 'Health checks', icon: 'check', about: "What's working and what isn't" },
	{ href: '/settings/backups', group: 'system', tint: 'blue', label: 'Backups', icon: 'archive', about: 'Save and restore HolyArrs' },
	{ href: '/settings/logs', group: 'system', tint: 'gray', label: 'Logs', icon: 'copy', about: 'Read and download the app log', need: 'users' },
	{ href: '/settings/this-app', group: 'system', tint: 'gray', label: 'This app', icon: 'info', about: 'Version, updates and storage' },
	{ href: '/settings/danger', group: 'system', tint: 'red', label: 'Danger zone', icon: 'alert', about: 'Restart or shut down', need: 'users' },

	{ href: '/settings/account', group: 'account', tint: 'gray', label: 'My account', icon: 'user', about: 'Password, two-factor, devices', need: 'self' }
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
		? "Can't reach that address. Check it and that the app is running."
		: text;
}

// Turns any error into a short sentence with a capital and an end mark.
export function plainMessage(e: unknown): string {
	const t = message(e).trim();
	if (!t) return 'Something went wrong.';
	const s = t.charAt(0).toUpperCase() + t.slice(1);
	return /[.!?)]$/.test(s) ? s : s + '.';
}
