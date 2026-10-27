import type { IconName } from './Icon.svelte';

export type GroupId = 'general' | 'library' | 'copies' | 'people' | 'linked' | 'system' | 'account';

/** Tile colours for the small icon squares in the Settings menu and home. */
export type Tint = 'gray' | 'blue' | 'indigo' | 'purple' | 'pink' | 'red' | 'orange' | 'amber' | 'green' | 'teal' | 'graphite';

export interface Section {
	href: string;
	label: string;
	icon: IconName;
	about: string;
	group: GroupId;
	tint: Tint;
	/** users: owner and admins; self: everyone; otherwise the settings permission. */
	need?: 'users' | 'self';
}

export interface Group {
	id: GroupId;
	label: string;
	icon: IconName;
	about: string;
}

/** visible keeps the sections a person may open. */
export function visible(all: Section[], can: (p: 'settings' | 'users') => boolean): Section[] {
	return all.filter((s) => (s.need === 'self' ? true : s.need === 'users' ? can('users') : can('settings')));
}

// Six groups, then My account on its own. The order follows how the big
// media managers split things: the app itself, what the library is, what to
// get and from where, who may do what, linked apps, then a separate System
// area for health, timers, backups and logs. Pages keep their routes; only
// the grouping changed, so old links still open the same page.
export const groups: Group[] = [
	{ id: 'general', label: 'General', icon: 'settings', about: 'This HolyArrs, how it looks and which parts are on.' },
	{ id: 'library', label: 'Library', icon: 'library', about: 'Where titles live, how files are named, and what new titles get.' },
	{ id: 'copies', label: 'Getting copies', icon: 'download', about: 'Which Release to take, where to look, who downloads, and lists that add titles.' },
	{ id: 'people', label: 'People and sign-in', icon: 'people', about: 'Who may do what, what they may ask for, and how they sign in.' },
	{ id: 'linked', label: 'Linked apps', icon: 'bell', about: 'Media servers, Alerts, other apps that call HolyArrs, and moving over.' },
	{ id: 'system', label: 'System', icon: 'tools', about: 'Health checks, Schedules, backups, logs and the danger zone.' },
	{ id: 'account', label: 'My account', icon: 'user', about: 'Your password, two-factor sign-in and your devices.' }
];

export const sections: Section[] = [
	{ href: '/settings/this-app', group: 'general', tint: 'gray', label: 'This app', icon: 'info', about: 'Version, updates, trailers, picture storage and where things are stored.' },
	{ href: '/settings/appearance', group: 'general', tint: 'indigo', label: 'Appearance', icon: 'appearance', about: 'Light, dark or automatic, in this browser.', need: 'self' },
	{ href: '/settings/parts', group: 'general', tint: 'graphite', label: 'Parts and controls', icon: 'grid', about: 'Turn Movies, TV shows, Music, Subtitles and the Wishlist on or off; restart one part or stage.', need: 'users' },

	{ href: '/settings/library-folders', group: 'library', tint: 'blue', label: 'Library folders', icon: 'folder', about: 'Where finished movies, shows and music live, and their health.' },
	{ href: '/settings/naming', group: 'library', tint: 'teal', label: 'Naming and files', icon: 'film', about: 'How files and folders are named, the holding folder and free space.' },
	{ href: '/settings/title-defaults', group: 'library', tint: 'orange', label: 'Movies and TV defaults', icon: 'tv', about: 'What a new movie or show gets: folder, Pick rules, Keep up to date and Search when added.' },
	{ href: '/settings/music', group: 'library', tint: 'pink', label: 'Music defaults', icon: 'music', about: 'What a new artist gets, which Pressing an album follows, and MusicBrainz.' },
	{ href: '/settings/subtitles', group: 'library', tint: 'amber', label: 'Subtitles', icon: 'subtitle', about: 'Default languages, Pass marks, when to look, Sync fix and Subtitle sites.' },
	{ href: '/settings/tmdb', group: 'library', tint: 'purple', label: 'Metadata', icon: 'sparkle', about: 'Where names, dates and posters come from.' },

	{ href: '/settings/pick-rules', group: 'copies', tint: 'amber', label: 'Pick rules', icon: 'star', about: 'The rules that choose which Release to take, and where each is used.' },
	{ href: '/settings/sources', group: 'copies', tint: 'blue', label: 'Sources', icon: 'source', about: 'Where HolyArrs looks for Releases.' },
	{ href: '/settings/search', group: 'copies', tint: 'indigo', label: 'Search', icon: 'search', about: 'How often feeds are checked, the nightly search and the early and fake guards.' },
	{ href: '/settings/downloaders', group: 'copies', tint: 'green', label: 'Downloaders', icon: 'downloader', about: 'The apps that download for HolyArrs, and what happens when a download goes wrong.' },
	{ href: '/settings/outside-lists', group: 'copies', tint: 'teal', label: 'Outside lists', icon: 'inbox', about: 'TMDB and Trakt lists that add movies by themselves, and the Never-add list.' },

	{ href: '/settings/users', group: 'people', tint: 'blue', label: 'Users and roles', icon: 'people', about: 'Invite family and friends, roles, permissions and devices.', need: 'users' },
	{ href: '/settings/wishlist', group: 'people', tint: 'pink', label: 'Wishlist rules', icon: 'wish', about: 'Weekly limits, and the folder and Pick rules approved asks get.' },
	{ href: '/settings/sign-in', group: 'people', tint: 'graphite', label: 'Sign-in and security', icon: 'shield', about: 'Plex, Jellyfin and Emby sign-in, two-factor rules, the local network and API keys.' },

	{ href: '/settings/media-servers', group: 'linked', tint: 'purple', label: 'Media servers', icon: 'tv', about: 'Tell Plex, Jellyfin or Emby about imports, renames and deletes.' },
	{ href: '/settings/alerts', group: 'linked', tint: 'red', label: 'Alerts', icon: 'bell', about: 'Discord, email and webhook messages for asks, imports and health.' },
	{ href: '/settings/api', group: 'linked', tint: 'gray', label: 'API for other apps', icon: 'key', about: 'The base address, scopes, example calls and every route.' },
	{ href: '/settings/move', group: 'linked', tint: 'teal', label: 'Move from other apps', icon: 'upload', about: 'Bring a setup over from Radarr, Sonarr, Lidarr, Prowlarr, Bazarr, Seerr and the Downloaders.', need: 'users' },

	{ href: '/settings/health', group: 'system', tint: 'green', label: 'Health checks', icon: 'check', about: 'What HolyArrs checks, the results, and how often it checks.' },
	{ href: '/settings/schedules', group: 'system', tint: 'orange', label: 'Schedules', icon: 'clock', about: 'Everything that runs on a timer: last and next run, Run now, and how often.' },
	{ href: '/settings/backups', group: 'system', tint: 'blue', label: 'Backups and restore', icon: 'archive', about: 'How often HolyArrs saves itself, and restore.' },
	{ href: '/settings/logs', group: 'system', tint: 'gray', label: 'Logs', icon: 'copy', about: 'Read, filter and download the app log; secrets are always hidden.', need: 'users' },
	{ href: '/settings/danger', group: 'system', tint: 'red', label: 'Danger zone', icon: 'alert', about: 'Restart HolyArrs, restart in safe mode or shut it down.', need: 'users' },

	{ href: '/settings/account', group: 'account', tint: 'gray', label: 'My account', icon: 'user', about: 'Your password, two-factor sign-in and your devices.', need: 'self' }
];

/** sectionFor finds the page a path belongs to (a source's health page belongs to Sources). */
export function sectionFor(all: Section[], path: string): Section | undefined {
	return all.find((s) => s.href === path || path.startsWith(s.href + '/'));
}

/** grouped returns each group with the visible pages inside it, leaving out empty groups. */
export function grouped(all: Section[]): { group: Group; items: Section[] }[] {
	return groups.map((group) => ({ group, items: all.filter((s) => s.group === group.id) })).filter((g) => g.items.length > 0);
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
