import type { IconName } from './Icon.svelte';

export type GroupId = 'general' | 'library' | 'copies' | 'people' | 'security' | 'linked' | 'maintenance' | 'account';

export interface Section {
	href: string;
	label: string;
	icon: IconName;
	about: string;
	group: GroupId;
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

// Seven groups, then My account on its own. Pages keep their routes; only the grouping changed.
export const groups: Group[] = [
	{ id: 'general', label: 'General', icon: 'settings', about: 'This HolyArrs, how it looks, its parts and its timers.' },
	{ id: 'library', label: 'Library', icon: 'library', about: 'Where titles live, how files are named, and what new titles get.' },
	{ id: 'copies', label: 'Getting copies', icon: 'download', about: 'Where HolyArrs looks, how it searches and who downloads.' },
	{ id: 'people', label: 'Wishlist and people', icon: 'people', about: 'What people may ask for, and who may do what.' },
	{ id: 'security', label: 'Sign-in and security', icon: 'shield', about: 'How people sign in, two-factor rules and API keys.' },
	{ id: 'linked', label: 'Linked apps', icon: 'bell', about: 'Media servers, Alerts, other apps that call HolyArrs, and moving over.' },
	{ id: 'maintenance', label: 'Maintenance', icon: 'archive', about: 'Backups, logs, health checks and the danger zone.' },
	{ id: 'account', label: 'My account', icon: 'user', about: 'Your password, two-factor sign-in and your devices.' }
];

export const sections: Section[] = [
	{ href: '/settings/this-app', group: 'general', label: 'This app', icon: 'info', about: 'Version, updates, a health summary and where things are stored.' },
	{ href: '/settings/appearance', group: 'general', label: 'Appearance', icon: 'appearance', about: 'Light, dark or automatic, in this browser.', need: 'self' },
	{ href: '/settings/parts', group: 'general', label: 'Parts and controls', icon: 'tools', about: 'Turn Movies, Shows, Music, Subtitles and the Wishlist on or off; restart one part or stage.', need: 'users' },
	{ href: '/settings/schedules', group: 'general', label: 'Schedules', icon: 'calendar', about: 'Everything that runs on a timer: last and next run, Run now, and how often.' },

	{ href: '/settings/library-folders', group: 'library', label: 'Library folders', icon: 'folder', about: 'Where finished movies, shows and music live, and their health.' },
	{ href: '/settings/naming', group: 'library', label: 'Naming and files', icon: 'film', about: 'How files and folders are named, the holding folder and free space.' },
	{ href: '/settings/music', group: 'library', label: 'Music defaults', icon: 'music', about: 'What a new artist gets, which Pressing an album follows, and MusicBrainz.' },
	{ href: '/settings/subtitles', group: 'library', label: 'Subtitles', icon: 'subtitle', about: 'Default languages, Pass marks, when to look, Sync fix and Subtitle sites.' },
	{ href: '/settings/tmdb', group: 'library', label: 'Metadata', icon: 'film', about: 'The TMDB key for names, dates and posters.' },
	{ href: '/settings/outside-lists', group: 'library', label: 'Outside lists', icon: 'source', about: 'TMDB and Trakt lists that add movies by themselves, and the Never-add list.' },

	{ href: '/settings/sources', group: 'copies', label: 'Sources', icon: 'source', about: 'Where HolyArrs looks for Releases.' },
	{ href: '/settings/search', group: 'copies', label: 'Search', icon: 'search', about: 'How often feeds are checked, the nightly search and the early and fake guards.' },
	{ href: '/settings/downloaders', group: 'copies', label: 'Downloaders', icon: 'downloader', about: 'The apps that download for HolyArrs, and what happens when a download goes wrong.' },
	{ href: '/settings/pick-rules', group: 'copies', label: 'Pick rules', icon: 'star', about: 'The rules that choose which Release to take, and where each is used.' },

	{ href: '/settings/wishlist', group: 'people', label: 'Wishlist rules', icon: 'wish', about: 'Weekly limits, and the folder and Pick rules approved asks get.' },
	{ href: '/settings/users', group: 'people', label: 'Users and roles', icon: 'people', about: 'Invite family and friends, roles, permissions and devices.', need: 'users' },

	{ href: '/settings/sign-in', group: 'security', label: 'Sign-in and security', icon: 'shield', about: 'Plex, Jellyfin and Emby sign-in, two-factor rules, the local network and API keys.' },

	{ href: '/settings/media-servers', group: 'linked', label: 'Media servers', icon: 'tv', about: 'Tell Plex, Jellyfin or Emby about imports, renames and deletes.' },
	{ href: '/settings/alerts', group: 'linked', label: 'Alerts', icon: 'bell', about: 'Discord, email and webhook messages for asks, imports and health.' },
	{ href: '/settings/api', group: 'linked', label: 'API for other apps', icon: 'key', about: 'The base address, scopes, example calls and every route.' },
	{ href: '/settings/move', group: 'linked', label: 'Move from other apps', icon: 'upload', about: 'Bring a setup over from Radarr, Sonarr, Lidarr, Prowlarr, Bazarr, Seerr and the Downloaders.', need: 'users' },

	{ href: '/settings/backups', group: 'maintenance', label: 'Backups and restore', icon: 'archive', about: 'How often HolyArrs saves itself, and restore.' },
	{ href: '/settings/logs', group: 'maintenance', label: 'Logs', icon: 'inbox', about: 'Read, filter and download the app log; secrets are always hidden.', need: 'users' },
	{ href: '/settings/health', group: 'maintenance', label: 'Health checks', icon: 'check', about: 'What HolyArrs checks, the results, and how often it checks.' },
	{ href: '/settings/danger', group: 'maintenance', label: 'Danger zone', icon: 'alert', about: 'Restart HolyArrs, restart in safe mode or shut it down.', need: 'users' },

	{ href: '/settings/account', group: 'account', label: 'My account', icon: 'user', about: 'Your password, two-factor sign-in and your devices.', need: 'self' }
];

/** sectionFor finds the page a path belongs to (a source's health page belongs to Sources). */
export function sectionFor(all: Section[], path: string): Section | undefined {
	return all.find((s) => s.href === path || path.startsWith(s.href + '/'));
}

/** grouped returns each group with the visible pages inside it, leaving out empty groups. */
export function grouped(all: Section[]): { group: Group; items: Section[] }[] {
	return groups.map((group) => ({ group, items: all.filter((s) => s.group === group.id) })).filter((g) => g.items.length > 0);
}
