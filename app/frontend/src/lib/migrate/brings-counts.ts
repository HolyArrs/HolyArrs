import { t } from '#lib/core/i18n/i18n.svelte.ts';

// The real amounts beside "What comes over" in setup: each line's count keys
// from POST /migrate/preview. A line with no keys shows no number.
const KEYS: Record<string, string[]> = {
	'migrate.brings.movies': ['titles'],
	'migrate.brings.shows': ['titles'],
	'migrate.brings.artists': ['titles'],
	'migrate.brings.folders': ['folders'],
	'migrate.brings.quality': ['quality_profiles', 'custom_formats'],
	'migrate.brings.rules': ['release_profiles', 'delay_profiles'],
	'migrate.brings.sources': ['indexers', 'download_clients'],
	'migrate.brings.lists': ['import_lists', 'exclusions'],
	'migrate.brings.alerts': ['notifications'],
	'migrate.brings.indexers': ['indexers'],
	'migrate.brings.appLinks': ['app_links'],
	'migrate.brings.downloadApps': ['download_clients'],
	'migrate.brings.people': ['users'],
	'migrate.brings.requests': ['requests'],
	'migrate.brings.subLanguages': ['subtitle_languages'],
	'migrate.brings.subProfiles': ['subtitle_profiles'],
	'migrate.brings.subSites': ['subtitle_sites'],
	'migrate.brings.dlFolders': ['categories'],
	'migrate.brings.usenetServers': ['usenet_servers']
};

// One and many wording per count key ({n} in each).
const WORDS: Record<string, [string, string]> = {
	folders: ['migrate.libraryFolder', 'migrate.libraryFolders2'],
	quality_profiles: ['migrate.setOfPickRules', 'migrate.setsOfPickRules'],
	custom_formats: ['migrate.trait', 'migrate.traits'],
	release_profiles: ['migrate.releaseRule', 'migrate.releaseRules'],
	delay_profiles: ['migrate.waitRule', 'migrate.waitRules'],
	indexers: ['migrate.source', 'migrate.sources2'],
	download_clients: ['migrate.downloader', 'migrate.downloaders2'],
	import_lists: ['migrate.outsideList', 'migrate.outsideLists2'],
	exclusions: ['migrate.exclusion', 'migrate.exclusions'],
	notifications: ['migrate.alert', 'migrate.alerts2'],
	app_links: ['migrate.linkedApp', 'migrate.linkedApps'],
	users: ['migrate.person', 'migrate.people2'],
	requests: ['migrate.request', 'migrate.requests2'],
	subtitle_languages: ['migrate.subtitleLanguage', 'migrate.subtitleLanguages'],
	subtitle_profiles: ['migrate.subtitleLanguageSet', 'migrate.subtitleLanguageSets'],
	subtitle_sites: ['migrate.subtitleSite', 'migrate.subtitleSites'],
	categories: ['migrate.category', 'migrate.categories'],
	usenet_servers: ['migrate.usenetServer', 'migrate.usenetServers']
};
const TITLES: Record<string, [string, string]> = {
	radarr: ['migrate.movie', 'migrate.movies'],
	sonarr: ['migrate.show', 'migrate.shows'],
	lidarr: ['migrate.artist', 'migrate.artists']
};

/** One counted thing: the whole phrase ("252 movies") and its parts around the number. */
export type CountItem = { key: string; n: number; text: string; num: string; before: string; after: string; zero: boolean };

const MARK = '\u0001';

/** The counted things for one line, or [] when the line has none or nothing was counted. */
export function bringsItems(line: string, kind: string, counts: Record<string, number> | null, locale?: string): CountItem[] {
	const keys = KEYS[line];
	if (!counts || !keys) return [];
	const f = new Intl.NumberFormat(locale);
	return keys
		.filter((k) => k in counts)
		.map((k) => {
			const n = counts[k];
			const w = (k === 'titles' ? TITLES[kind] : WORDS[k]) ?? ['migrate.title', 'migrate.titles2'];
			const num = f.format(n);
			const [before, after = ''] = t(n === 1 ? w[0] : w[1], { n: MARK }).split(MARK);
			return { key: k, n, num, before: before.trim(), after: after.trim(), text: `${before}${num}${after}`, zero: n === 0 };
		});
}

/** Whether the line gets a number at all (and so a spinner while counting). */
export const hasCount = (line: string) => line in KEYS;
