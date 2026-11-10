// Find one setting, not just a page. Built by hand: every entry names the page, the element id the page gives that field or card, and the other words a person might type for it.

import { isHeld } from '#lib/core/settings.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export interface SettingEntry {
	label: string;
	href: string;
	/** The id of the field or card on that page; the page scrolls to it and highlights it. */
	anchor: string;
	words?: string;
	advanced?: boolean;
	/** Set on the hand-made entries below: the base of their message keys, so their label and words can be shown in the person's language. */
	id?: string;
}

/** Search entries for features held back from this release, by anchor. */
const heldAnchors: Record<string, string> = { 'row-titles-card': 'row_title_stats', 'sites-card': 'subtitle_sites', 'trakt-card': 'trakt_lists', 'emby-card': 'emby_signin' };

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 40);

const e = (href: string, anchor: string, label: string, words = '', advanced = false): SettingEntry => ({
	href,
	anchor,
	label,
	words,
	advanced,
	id: `settings.index.${href.replace('/settings/', '')}.${anchor || 'page'}.${slug(label)}`
});

const said = (key: string, fallback: string) => {
	const line = t(key);
	return line === key ? fallback : line;
};

/** The entry's label in the person's language (the English label when the entry has no message). */
export const entryLabel = (entry: SettingEntry): string => (entry.id ? said(`${entry.id}.label`, entry.label) : entry.label);

/** The words a person might type for it, in their language. */
export const entryWords = (entry: SettingEntry): string => (entry.id ? said(`${entry.id}.words`, entry.words ?? '') : (entry.words ?? ''));

export const settingIndex: SettingEntry[] = [
	e('/settings/appearance', 'theme', 'Theme', 'appearance light dark automatic mode colours colors night'),
	e('/settings/appearance', 'language', 'Language', 'language translation locale english words'),
	e('/settings/appearance', 'autoplay-trailers', 'Play trailers by themselves', 'autoplay trailers video explore muted'),
	e('/settings/appearance', 'signin-art', 'Show Library art on the sign-in page', 'sign-in login page background posters art'),
	e('/settings/appearance', 'row-titles-card', 'Explore row titles', 'explore rows titles names layers taste'),
	e('/settings/appearance', 'ratings', 'Ratings', 'ratings scores imdb rotten tomatoes critics audience metacritic letterboxd trakt tmdb'),
	e('/settings/this-app', 'storage-card', 'Storage', 'paths config folder disk free space where things are stored', true),
	e('/settings/this-app', 'count-install', 'Count this install', 'install count country privacy metadata server', true),
	e('/settings/this-app', 'picture-storage', 'Picture storage', 'picture cache artwork posters covers gb', true),
	e('/settings/this-app', 'history-keep-card', 'History keep time', 'history events keep days forever clean up'),
	e('/settings/this-app', 'url-base', 'URL base', 'reverse proxy path subfolder base url nginx traefik', true),
	e('/settings/parts', 'parts-card', 'Turn parts on or off', 'parts movies shows music subtitles wishlist switch off disable enable'),
	e('/settings/parts', 'stages-card', 'Pause or restart a stage', 'stages background work stuck restart pause resume'),
	e('/settings/parts', 'links-card', 'Reconnect a linked app', 'reconnect downloader test source media server subtitle site'),
	e('/settings/schedules', '', 'Schedules', 'timer every run now how often refresh jobs tasks interval'),

	e('/settings/library-folders', '', 'Library folders', 'root folder path movies folder shows folder music folder where files live add folder'),
	e('/settings/library-folders', 'import-folders', 'Extra import folders', 'import files by hand manual import allowed folders downloads loose files'),
	e('/settings/title-defaults', 'td-movie-folder', 'Library folder for new movies', 'movies defaults root folder new movie add where'),
	e('/settings/title-defaults', 'td-movie-rules', 'Quality for new movies', 'movies defaults quality profile new movie add'),
	e('/settings/title-defaults', 'td-movie-keep', 'Keep new movies up to date', 'movies defaults monitor new movie add'),
	e('/settings/title-defaults', 'td-movie-search', 'Search when a movie is added', 'movies defaults search on add start search now'),
	e('/settings/title-defaults', 'td-gate', 'Release gate for new movies', 'minimum availability announced in cinemas released digital disc', true),
	e('/settings/title-defaults', 'td-gate-days', 'Extra days after the Release gate', 'delay days gate wait', true),
	e('/settings/title-defaults', 'td-show-folder', 'Library folder for new shows', 'shows tv defaults root folder new show add where'),
	e('/settings/title-defaults', 'td-show-rules', 'Quality for new shows', 'shows tv defaults quality profile new show add'),
	e('/settings/title-defaults', 'td-show-keep', 'Keep new shows up to date', 'shows tv defaults monitor new show add'),
	e('/settings/title-defaults', 'td-show-search', 'Search when a TV show is added', 'shows tv defaults search on add missing episodes'),
	e('/settings/title-defaults', 'td-seasons', 'Seasons a new TV show keeps up to date', 'shows seasons monitor all future latest none', true),
	e('/settings/title-defaults', 'td-new-seasons', 'New seasons of a TV show', 'shows new seasons monitor', true),
	e('/settings/title-defaults', 'td-order', 'Episode order for new shows', 'shows series type anime daily absolute aired numbering', true),
	e('/settings/title-defaults', 'td-source', 'Where show details come from', 'shows metadata tmdb tvmaze details source', true),
	e('/settings/naming', 'file-pattern', 'Movie file name pattern', 'naming rename format movie file'),
	e('/settings/naming', 'folder-pattern', 'Movie folder name pattern', 'naming folder format movie'),
	e('/settings/naming', 'colon', 'A colon in a name becomes', 'colon character replace', true),
	e('/settings/naming', 'illegal', 'Characters a disk cannot hold', 'illegal characters replace remove', true),
	e('/settings/naming', 'show-folder-pattern', 'Show folder pattern', 'naming tv show folder format'),
	e('/settings/naming', 'season-folders', 'A folder for each season', 'season folder'),
	e('/settings/naming', 'season-folder-pattern', 'Season folder pattern', 'season folder naming'),
	e('/settings/naming', 'specials-folder', 'Folder for specials', 'season 0 specials extras', true),
	e('/settings/naming', 'episode-pattern', 'Episode file name pattern', 'episode naming format rename'),
	e('/settings/naming', 'daily-pattern', 'Names for shows ordered by air day', 'daily air date talk show', true),
	e('/settings/naming', 'anime-pattern', 'Names for shows ordered by running number', 'anime absolute number', true),
	e('/settings/naming', 'multi-style', 'A file holding several episodes', 'multi episode style', true),
	e('/settings/naming', 'air-date-style', 'Air day format', 'date format daily', true),
	e('/settings/naming', 'absolute-digits', 'Digits in a running number', 'absolute digits anime padding', true),
	e('/settings/naming', 'artist-folder-pattern', 'Artist folder pattern', 'music naming artist folder'),
	e('/settings/naming', 'album-folder-pattern', 'Album folder pattern', 'music naming album folder'),
	e('/settings/naming', 'track-pattern', 'Track file name pattern', 'music naming track file song'),
	e('/settings/naming', 'multi-disc-pattern', 'Track name on a multi-disc album', 'disc cd multi disc', true),
	e('/settings/naming', 'disc-folders', 'A folder for each disc', 'disc folder cd', true),
	e('/settings/naming', 'disc-folder-pattern', 'Disc folder pattern', 'disc folder cd naming', true),
	e('/settings/naming', 'read-tags', "Read a track's own tags", 'tags id3 metadata music files', true),
	e('/settings/naming', 'rename-on-import', 'Rename files when they are imported', 'rename import'),
	e('/settings/naming', 'scan-limit', 'Folders read per library scan', 'import a library scan limit', true),
	e('/settings/naming', 'music-scan-limit', 'Artist folders read per music scan', 'music scan limit', true),
	e('/settings/naming', 'holding-days', 'Keep deleted files for', 'holding folder recycle bin trash deleted days'),
	e('/settings/naming', 'min-free', 'Always leave this much free space', 'free space disk minimum MB GB', true),
	e('/settings/naming', 'skip-free-check', 'Skip the free-space check', 'free space disk check', true),
	e('/settings/naming', 'holding-card', 'Empty the holding folder now', 'holding folder clean recycle bin trash'),
	e('/settings/music', 'm-albums', 'Albums a new artist keeps up to date', 'music new artist albums monitor'),
	e('/settings/music', 'm-new', 'New albums', 'music new albums release'),
	e('/settings/music', 'album-kinds', 'Album kinds', 'music album ep single kinds types'),
	e('/settings/music', 'extra-kinds', 'Extra kinds', 'music live compilation remix soundtrack', true),
	e('/settings/music', 'pressing-statuses', 'Pressings that count', 'music release status official bootleg', true),
	e('/settings/music', 'm-rule', 'Which Pressing an album follows', 'music pressing edition release', true),
	e('/settings/music', 'm-url', 'MusicBrainz address', 'musicbrainz metadata mirror server', true),
	e('/settings/music', 'm-hours', 'Keep MusicBrainz answers for', 'musicbrainz cache hours', true),
	e('/settings/subtitles', 'lang-card', 'Default subtitle languages', 'subtitles languages english spanish'),
	e('/settings/subtitles', 'pm-ep', 'Pass mark for episodes', 'subtitles score minimum episodes', true),
	e('/settings/subtitles', 'pm-mov', 'Pass mark for movies', 'subtitles score minimum movies', true),
	e('/settings/subtitles', 'embedded', 'Count subtitles inside the video', 'subtitles embedded inside', true),
	e('/settings/subtitles', 'no-picture', 'Leave out picture subtitles', 'subtitles pgs vobsub image', true),
	e('/settings/subtitles', 'hi-suffix', 'Hearing-impaired file names', 'subtitles hi cc sdh deaf', true),
	e('/settings/subtitles', 's-hours', 'Look for missing subtitles every', 'subtitles hours how often', true),
	e('/settings/subtitles', 's-batch', 'Subtitle files per run', 'subtitles batch', true),
	e('/settings/subtitles', 'better-subs', 'Look for better subtitles', 'subtitles upgrade better'),
	e('/settings/subtitles', 'sy-step', 'Sync fix shift', 'subtitles sync timing offset ms', true),
	e('/settings/subtitles', 'sites-card', 'Subtitle sites', 'subtitles opensubtitles provider site add'),
	e('/settings/tmdb', 'metadata-card', 'Metadata server', 'metadata tmdb posters names artwork server'),
	e('/settings/outside-lists', '', 'Outside lists', 'tmdb list trakt list import list auto add'),
	e('/settings/outside-lists', 'trakt-card', 'Trakt key', 'trakt key client id vip access token reviews comments'),
	e('/settings/outside-lists', 'never-card', 'Never-add list', 'never add exclusion block list'),

	e('/settings/sources', '', 'Sources', 'indexer torznab newznab rss tracker usenet add source'),
	e('/settings/search', 'feed-minutes', 'Check Source feeds every', 'rss feed minutes interval', true),
	e('/settings/search', 'missing-count', 'Titles to search each night', 'nightly search missing'),
	e('/settings/search', 'better-nightly', 'Also find better copies each night', 'upgrade better copy nightly'),
	e('/settings/search', 'missing-episodes', 'Search for missing episodes too', 'episodes missing nightly'),
	e('/settings/search', 'pack-upgrade', 'When a season pack is taken', 'season pack'),
	e('/settings/search', 'pack-prefer', 'Season pack or single episodes', 'season pack episodes'),
	e('/settings/search', 'multi-season', 'Take Releases with several seasons', 'multi season pack', true),
	e('/settings/search', 'missing-albums', 'Search for missing albums too', 'music albums missing nightly'),
	e('/settings/search', 'discographies', 'Take whole discographies', 'discography music', true),
	e('/settings/search', 'track-tolerance', 'Tracks an album Release may lack', 'music tracks missing', true),
	e('/settings/search', 'refuse-early', 'Turn down Releases posted before release', 'early fake release date'),
	e('/settings/search', 'early-grace', 'Hours a Release may be early', 'early hours grace', true),
	e('/settings/search', 'refuse-tiny', 'Turn down fakes by size and file type', 'fake size sample'),
	e('/settings/search', 'import-floors', 'Check file sizes at import', 'size import minimum', true),
	e('/settings/search', 'min-video', 'Smallest video file to place', 'size minimum MB video', true),
	e('/settings/search', 'min-track', 'Smallest track file to place', 'size minimum KB music', true),
	e('/settings/search', 'episode-minutes', 'Episode length when not known', 'runtime minutes', true),
	e('/settings/search', 'hold-unknown', 'Hold Releases of unknown size or date', 'hold unknown', true),
	e('/settings/search', 'search-now', 'Search now', 'search better copy feeds run'),
	e('/settings/downloaders', 'downloaders-card', 'Downloaders', 'download client qbittorrent sabnzbd transmission deluge nzbget torrent client add'),
	e('/settings/downloaders', 'st-stall', 'Stalled after', 'stalled stuck hours download', true),
	e('/settings/downloaders', 'st-act', 'Give up and try the next Release after', 'give up hours retry', true),
	e('/settings/downloaders', 'remove-failed', 'Remove failed downloads from the Downloader', 'failed remove cleanup'),
	e('/settings/downloaders', 'fail-programs', 'Fail downloads that hold programs or fakes', 'exe virus fake', true),
	e('/settings/downloaders', 'replace-removed', 'Replace downloads removed outside HolyArrs', 'removed outside', true),
	e('/settings/downloaders', 'remove-defaults', 'What removing a download does by default', 'remove delete files turned down search again'),
	e('/settings/downloaders', 'try-next-dl', 'Try the next Downloader', 'fallback downloader', true),
	e('/settings/downloaders', 'st-gap', 'Seconds between sends to one Downloader', 'delay seconds send', true),
	e('/settings/pick-rules', 'pick-rules-card', 'Quality', 'quality profile quality formats resolution 4k 1080p pick rules'),
	// The editor works on one set at a time, so these open the list to pick which set to change.
	e('/settings/pick-rules', 'pick-rules-card', 'New quality set', 'make add create copy new quality profile'),
	e('/settings/pick-rules', 'pick-rules-card', 'Quality steps and Good enough', 'pick rules steps resolution 720p 1080p 2160p 4k remux bluray web good enough cutoff upgrade until'),
	e('/settings/pick-rules', 'pick-rules-card', 'Traits and points', 'pick rules traits points score custom formats hevc x265 av1 dolby vision hdr atmos'),
	e('/settings/pick-rules', 'pick-rules-card', 'Must have and Never', 'pick rules must have never required ignored block 3d burned subtitles sample'),
	e('/settings/pick-rules', 'pick-rules-card', 'Release size per hour', 'pick rules size gb per hour smallest largest minimum maximum limits'),
	e('/settings/pick-rules', 'pick-rules-card', 'Waiting for a better Release', 'pick rules wait delay profile torrent usenet prefer protocol minutes'),
	e('/settings/pick-rules', 'pick-rules-card', 'Release languages', 'pick rules language english french german spanish dubbed original audio'),
	e('/settings/pick-rules', 'pick-rules-card', 'Fixed copies', 'pick rules proper repack fixed copy re-release'),

	e('/settings/requests', 'per-week-movies', 'Movie requests each week', 'wishlist limit request quota movies'),
	e('/settings/requests', 'per-week-shows', 'Show requests each week', 'wishlist limit request quota shows'),
	e('/settings/requests', 'per-week-music', 'Music requests each week', 'wishlist limit request quota music'),
	e('/settings/requests', 'ask-folder', 'Library folder for approved requests', 'wishlist approved folder'),
	e('/settings/requests', 'ask-rules', 'Quality for approved requests', 'wishlist approved pick rules quality', true),
	e('/settings/requests', 'search-approved', 'Search when approved', 'wishlist approved search'),
	e('/settings/users', 'inv-note', 'Invite someone', 'invite link add user family friend new user'),
	e('/settings/users', 'people-card', 'Users and roles', 'users people roles permissions admin member kid preset devices'),
	e('/settings/users', 'invites-card', 'Invite links', 'invites take back links'),

	e('/settings/sign-in', 'local-card', 'Skip sign-in on my local network', 'local network lan no password'),
	e('/settings/sign-in', 'twofactor-card', 'Admins must use two-factor sign-in', '2fa two-factor otp totp rule'),
	e('/settings/sign-in', 'min-pw', 'Shortest password allowed', 'password length minimum'),
	e('/settings/sign-in', 'plex-card', 'Sign in with Plex', 'plex sign-in login'),
	e('/settings/sign-in', 'jellyfin-card', 'Sign in with Jellyfin', 'jellyfin sign-in login'),
	e('/settings/sign-in', 'emby-card', 'Sign in with Emby', 'emby sign-in login'),
	e('/settings/sign-in', 'sso-card', 'Single sign-on', 'sso oidc openid connect authentik authelia keycloak trusted proxy forward auth unlink'),
	e('/settings/sign-in', 'hosts-card', 'Allowed host names', 'host names domain dns rebinding reverse proxy hostname', true),
	e('/settings/sign-in', 'proxy-card', 'Trusted proxies', 'reverse proxy forward auth', true),
	e('/settings/api', 'keys-card', 'API keys', 'api key token make revoke'),

	e('/settings/media-servers', '', 'Media servers', 'plex jellyfin emby library update notify'),
	e('/settings/alerts', '', 'Alerts', 'notifications discord telegram ntfy gotify pushover push email webhook notify'),
	e('/settings/alerts', 'pack-alerts', 'Season pack Alerts', 'season pack one alert per episode'),
	e('/settings/ai', 'ai-switch', 'AI apps', 'ai assistant claude chatgpt mcp llm agent bot connect turn on off'),
	e('/settings/ai', 'ai-keys', 'AI keys', 'ai key token claude mcp make revoke assistant'),
	e('/settings/ai', 'ai-connect', 'Connect Claude', 'claude desktop code connect setup address mcp'),
	e('/settings/api', 'api-start', 'API base address', 'api address url base openapi'),
	e('/settings/api', 'api-routes', 'Every API route', 'api routes endpoints openapi'),
	e('/settings/api', 'seerr-card', 'Requests from Seerr and Overseerr', 'seerr overseerr jellyseerr sonarr radarr requests connect'),
	e('/settings/move', '', 'Import from other apps', 'import move migrate radarr sonarr lidarr prowlarr bazarr seerr overseerr'),

	e('/settings/backups', 'backup-key', 'Master key', 'master key kit encryption secrets restore another server'),
	e('/settings/backups', 'backup-often', 'How often backups are made', 'backup schedule'),
	e('/settings/backups', 'backup-list', 'Restore from a backup', 'restore backup download delete'),
	e('/settings/logs', 'lg-level', 'Logs', 'log level errors warnings debug download log'),
	e('/settings/health', 'health-checks-card', 'Health check results', 'health checks problems check again'),
	e('/settings/health', 'hs-every', 'Check health every', 'health minutes interval', true),
	e('/settings/health', 'hs-guard', 'Alert after this many runs in a row', 'health flap', true),
	e('/settings/health', 'hs-low', 'Warn when free space is under', 'free space low disk MB', true),
	e('/settings/health', 'update-check', 'Check for a newer HolyArrs', 'update check daily every days'),
	e('/settings/health', 'hs-upd', 'Release feed', 'update feed url', true),
	e('/settings/danger', 'danger-card', 'Restart or shut down HolyArrs', 'restart reboot shutdown safe mode stop danger'),

	e('/settings/account', 'pw-current', 'Change my password', 'password my account change'),
	e('/settings/account', 'twofactor-me', 'My two-factor sign-in', '2fa two-factor otp authenticator backup codes'),
	e('/settings/account', 'alerts-me', 'Alerts for me', 'my email discord notifications'),
	e('/settings/account', 'sessions-me', 'My signed-in devices', 'sessions devices sign out everywhere')
];

const norm = (s: string) => s.toLowerCase().normalize('NFKD').replace(/[̀-ͯ]/g, '');

/** Finds settings whose label, words or group hold every typed word. Labels that start with the query come first, then labels that hold it, then matches on other words. Only pages in `allowed` are searched. */
export function searchSettings(query: string, allowed: (href: string) => boolean, groupOf: (href: string) => string = () => '', index: SettingEntry[] = settingIndex): SettingEntry[] {
	const q = norm(query.trim());
	const words = q.split(/\s+/).filter(Boolean);
	if (!words.length) return [];
	const scored: { entry: SettingEntry; score: number; at: number }[] = [];
	index.forEach((entry, at) => {
		if (!allowed(entry.href) || isHeld(heldAnchors[entry.anchor])) return;
		const label = norm(entryLabel(entry));
		const all = `${label} ${norm(entryWords(entry))} ${norm(groupOf(entry.href))}`;
		if (!words.every((w) => all.includes(w))) return;
		const score = label.startsWith(q) ? 0 : label.includes(q) ? 1 : words.every((w) => label.includes(w)) ? 2 : 3;
		scored.push({ entry, score, at });
	});
	return scored.sort((a, b) => a.score - b.score || a.at - b.at).map((s) => s.entry);
}
