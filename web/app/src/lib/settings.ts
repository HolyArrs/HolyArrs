import type { IconName } from './Icon.svelte';

export interface Section {
	href: string;
	label: string;
	icon: IconName;
	about: string;
	/** users: owner and admins; self: everyone; otherwise the settings permission. */
	need?: 'users' | 'self';
}

/** visible keeps the sections a person may open. */
export function visible(all: Section[], can: (p: 'settings' | 'users') => boolean): Section[] {
	return all.filter((s) => (s.need === 'self' ? true : s.need === 'users' ? can('users') : can('settings')));
}

export const sections: Section[] = [
	{ href: '/settings/library-folders', label: 'Library folders', icon: 'folder', about: 'Where finished movies live, and their health.' },
	{ href: '/settings/naming', label: 'Naming and files', icon: 'film', about: 'File and folder name patterns, the holding folder and the free-space floor.' },
	{ href: '/settings/sources', label: 'Sources', icon: 'source', about: 'Where HolyArrs looks for Releases.' },
	{ href: '/settings/search', label: 'Search', icon: 'search', about: 'How often feeds are checked, the daily search for missing movies, and Downloader fallback.' },
	{ href: '/settings/subtitles', label: 'Subtitles', icon: 'subtitle', about: 'Default languages, Pass marks, when to look, Sync fix and Subtitle sites.' },
	{ href: '/settings/downloaders', label: 'Downloaders', icon: 'downloader', about: 'The apps that download for HolyArrs.' },
	{ href: '/settings/outside-lists', label: 'Outside lists', icon: 'source', about: 'TMDB and Trakt lists that add movies by themselves, and the Never-add list.' },
	{ href: '/settings/music', label: 'Music', icon: 'music', about: 'What a new artist gets, which Pressing an album follows, and MusicBrainz.' },
	{ href: '/settings/wishlist', label: 'Wishlist', icon: 'wish', about: 'Default weekly limits per kind, and the folder and Pick rules for approved movie, show and music asks.' },
	{ href: '/settings/alerts', label: 'Alerts', icon: 'bell', about: 'Discord, email and webhook messages: asks, imports, renames, deletes, health and decisions.' },
	{ href: '/settings/media-servers', label: 'Media servers', icon: 'film', about: 'Tell Plex, Jellyfin or Emby about imports, renames and deletes.' },
	{ href: '/settings/tmdb', label: 'TMDB key', icon: 'film', about: 'Movie names, dates and posters.' },
	{ href: '/settings/backups', label: 'Backups', icon: 'archive', about: 'How often HolyArrs saves itself, and restore.' },
	{ href: '/settings/sign-in', label: 'Sign-in', icon: 'shield', about: 'Plex, Jellyfin and Emby sign-in, two-factor rules, local network and API keys.' },
	{ href: '/settings/move', label: 'Move from other apps', icon: 'upload', about: 'Bring a setup over from Radarr, Sonarr, Lidarr, Prowlarr, Bazarr, Seerr and the Downloaders.', need: 'users' },
	{ href: '/settings/parts', label: 'Parts and controls', icon: 'settings', about: 'Turn Movies, Shows, Music, Subtitles and the Wishlist on or off; restart a part, a stage, a linked app or HolyArrs.', need: 'users' },
	{ href: '/settings/users', label: 'Users', icon: 'people', about: 'Invite family and friends, roles and permissions.', need: 'users' },
	{ href: '/settings/account', label: 'My account', icon: 'user', about: 'Your password and two-factor sign-in.', need: 'self' },
	{ href: '/settings/this-app', label: 'This app', icon: 'info', about: 'Version, health checks, updates and where things are stored.' },
	{ href: '/settings/schedules', label: 'Schedules', icon: 'calendar', about: 'Everything that runs on a timer: last and next run, Run now, and how often.' },
	{ href: '/settings/api', label: 'API for other apps', icon: 'key', about: 'Make a key, the scopes, the base address, example calls and every route.' },
	{ href: '/settings/logs', label: 'Logs', icon: 'archive', about: 'Read, filter and download the app log; secrets are always hidden.', need: 'users' }
];
