// Which lines a title page's About section shows, picked per browser in Settings > Appearance. The defaults stay short: what helps decide on a request and is not already in the hero (year, length, age rating, genres).

export type AboutItem = { id: string; group: 'Information' | 'Production' | 'Links' | 'Ratings' | 'Feel'; name: string; on: boolean };

export const ABOUT_ITEMS: AboutItem[] = [
	{ id: 'released', group: 'Information', name: 'Released or premiered', on: false },
	{ id: 'length', group: 'Information', name: 'Run time or seasons', on: false },
	{ id: 'episodes', group: 'Information', name: 'Episodes, length and total hours', on: true },
	{ id: 'type', group: 'Information', name: 'Type of show', on: false },
	{ id: 'rated', group: 'Information', name: 'Age rating', on: false },
	{ id: 'status', group: 'Information', name: 'Status (and when an ended show stopped)', on: true },
	{ id: 'next', group: 'Information', name: 'Next episode', on: true },
	{ id: 'theaters', group: 'Information', name: 'In theaters', on: false },
	{ id: 'digital', group: 'Information', name: 'Digital release', on: true },
	{ id: 'disc', group: 'Information', name: 'On disc', on: false },
	{ id: 'region', group: 'Information', name: 'Region of origin', on: false },
	{ id: 'language', group: 'Information', name: 'Language', on: true },
	{ id: 'origTitle', group: 'Information', name: 'Original title', on: true },
	{ id: 'studios', group: 'Production', name: 'Studios (shows; a movie’s studio is beside the story)', on: true },
	{ id: 'budget', group: 'Production', name: 'Budget', on: true },
	{ id: 'boxOffice', group: 'Production', name: 'Box office', on: true },
	{ id: 'links', group: 'Links', name: 'Links to IMDb, TMDB, Trakt and Letterboxd', on: true },
	{ id: 'scores', group: 'Ratings', name: 'Every score', on: false },
	{ id: 'awards', group: 'Ratings', name: 'Awards', on: false },
	{ id: 'genres', group: 'Feel', name: 'Genres', on: false },
	{ id: 'themes', group: 'Feel', name: 'Themes', on: false }
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
