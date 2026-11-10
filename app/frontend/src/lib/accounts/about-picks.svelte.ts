import { t } from '#lib/core/i18n/i18n.svelte.ts';
// Which lines a title page's About section shows, picked per browser in Settings > Appearance. The defaults stay short: what helps decide on a request and is not already in the hero (year, length, age rating, genres).

export type AboutItem = { id: string; group: 'Information' | 'Production' | 'Links' | 'Ratings' | 'Feel'; name: string; on: boolean };

export const ABOUT_ITEMS: AboutItem[] = [
	{ id: 'released', group: 'Information', get name() { return t('accounts.about.released'); }, on: false },
	{ id: 'length', group: 'Information', get name() { return t('accounts.about.length'); }, on: false },
	{ id: 'episodes', group: 'Information', get name() { return t('accounts.about.episodes'); }, on: true },
	{ id: 'type', group: 'Information', get name() { return t('accounts.about.type'); }, on: false },
	{ id: 'rated', group: 'Information', get name() { return t('accounts.about.rated'); }, on: false },
	{ id: 'status', group: 'Information', get name() { return t('accounts.about.status'); }, on: true },
	{ id: 'next', group: 'Information', get name() { return t('accounts.about.next'); }, on: true },
	{ id: 'theaters', group: 'Information', get name() { return t('accounts.about.theaters'); }, on: false },
	{ id: 'digital', group: 'Information', get name() { return t('accounts.about.digital'); }, on: true },
	{ id: 'disc', group: 'Information', get name() { return t('accounts.about.disc'); }, on: false },
	{ id: 'region', group: 'Information', get name() { return t('accounts.about.region'); }, on: false },
	{ id: 'language', group: 'Information', get name() { return t('accounts.about.language'); }, on: true },
	{ id: 'origTitle', group: 'Information', get name() { return t('accounts.about.origTitle'); }, on: true },
	{ id: 'studios', group: 'Production', get name() { return t('accounts.about.studios'); }, on: true },
	{ id: 'budget', group: 'Production', get name() { return t('accounts.about.budget'); }, on: true },
	{ id: 'boxOffice', group: 'Production', get name() { return t('accounts.about.boxOffice'); }, on: true },
	{ id: 'links', group: 'Links', get name() { return t('accounts.about.links'); }, on: true },
	{ id: 'scores', group: 'Ratings', get name() { return t('accounts.about.scores'); }, on: false },
	{ id: 'awards', group: 'Ratings', get name() { return t('accounts.about.awards'); }, on: false },
	{ id: 'genres', group: 'Feel', get name() { return t('accounts.about.genres'); }, on: false },
	{ id: 'themes', group: 'Feel', get name() { return t('accounts.about.themes'); }, on: false }
];

const DEFAULTS = ABOUT_ITEMS.filter((i) => i.on).map((i) => i.id);
const KEY = 'holyarrs.about-3';

export const aboutPicks = $state<{ list: string[] }>({ list: typeof window === 'undefined' ? DEFAULTS : stored() });
export const aboutIsDefault = () => aboutPicks.list.length === DEFAULTS.length && DEFAULTS.every((d) => aboutPicks.list.includes(d));

export function setAboutPick(id: string, on: boolean) {
	aboutPicks.list = ABOUT_ITEMS.map((i) => i.id).filter((x) => (x === id ? on : aboutPicks.list.includes(x)));
	try {
		localStorage.setItem(KEY, JSON.stringify(aboutPicks.list));
	} catch {
		/* Kept for this visit only. */
	}
}

export function resetAboutPicks() {
	aboutPicks.list = DEFAULTS;
	try {
		localStorage.removeItem(KEY);
	} catch {
		/* Nothing kept. */
	}
}

const BY_LABEL: Record<string, string> = {
	Premiered: 'released', Released: 'released', Seasons: 'length', 'Run time': 'length', Episodes: 'episodes', Type: 'type',
	Rated: 'rated', Status: 'status', 'Next episode': 'next', 'In theaters': 'theaters', Digital: 'digital',
	'On disc': 'disc', 'Region of origin': 'region', 'Regions of origin': 'region', 'Original language': 'language', 'Spoken languages': 'language',
	'Original title': 'origTitle', 'Directed by': 'makers', 'Created by': 'makers', Network: 'network', Studio: 'studios', Studios: 'studios',
	Budget: 'budget', 'Box office': 'boxOffice', Awards: 'awards', 'Best known for': 'awards', Genre: 'genres', Genres: 'genres', Themes: 'themes'
};

/** Whether a row (by its label) is picked; any other label is a score. */
export const aboutShown = (label: string, picks: string[]) => picks.includes(label.startsWith('link:') ? 'links' : (BY_LABEL[label] ?? 'scores'));

function stored(): string[] {
	try {
		const raw = localStorage.getItem(KEY);
		return raw ? (JSON.parse(raw) as unknown[]).filter((x): x is string => ABOUT_ITEMS.some((i) => i.id === x)) : DEFAULTS;
	} catch {
		return DEFAULTS;
	}
}
