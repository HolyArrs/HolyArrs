
export type PartName = 'movies' | 'shows' | 'music' | 'subtitles' | 'wishlist';

export interface PartView {
	part: PartName;
	name: string;
	on: boolean;
	state: 'off' | 'starting' | 'running' | 'retrying' | 'failed';
	reason?: string;
}

export interface PartsAnswer {
	parts: PartView[];

	screens: Record<string, string>;
	safe_mode: boolean;

	safe_screens?: string[];

	stages?: string[];
}

export function screenOwner(screens: Record<string, string>, path: string): string | null {
	const row = screenRow(screens, path);
	return row === null ? null : screens[row];
}

function screenRow(screens: Record<string, string>, path: string): string | null {
	const got = path.replace(/^\/+|\/+$/g, '').split('/');
	let best: string | null = null;
	let bestWild = -1;
	for (const pattern of Object.keys(screens)) {
		const want = pattern.replace(/^\/+|\/+$/g, '').split('/');
		if (want.length !== got.length) continue;
		let wild = 0;
		for (let i = 0; i < want.length && wild >= 0; i++) {
			if (want[i].startsWith('[')) wild++;
			else if (want[i] !== got[i]) wild = -1;
		}
		if (wild >= 0 && (bestWild < 0 || wild < bestWild)) {
			best = pattern;
			bestWild = wild;
		}
	}
	return best;
}

export function offPartName(a: PartsAnswer | null, path: string): string | null {
	if (!a) return null;
	if (a.safe_mode && a.safe_screens?.includes(screenRow(a.screens, path) ?? '')) return null;
	const owner = screenOwner(a.screens, path);
	if (!owner || owner === 'core' || owner === 'title') return null;
	const on = (p: string) => a.parts.find((x) => x.part === p)?.on ?? true;
	if (owner === 'media') return on('movies') || on('shows') || on('music') ? null : 'Movies, TV shows and Music';
	if (on(owner)) return null;
	return a.parts.find((x) => x.part === owner)?.name ?? owner;
}

export const partOrder: PartName[] = ['movies', 'shows', 'music', 'subtitles', 'wishlist'];

export const partStops: Record<PartName, string> = {
	movies: 'Movie pages, movie searches and movie downloads stop. Movie files stay where they are.',
	shows: 'Show pages, episode searches and show downloads stop. Episode files stay where they are.',
	music: 'Artist and album pages, music searches and music downloads stop. Music files stay where they are.',
	subtitles: 'Subtitles are no longer looked for or fetched. Subtitle files already saved stay.',
	wishlist: 'Family and friends can no longer request titles. Requests already made are kept.'
};

export const partNeeds: Record<PartName, string> = {
	movies: '',
	shows: '',
	music: '',
	subtitles: 'Needs Movies or TV shows.',
	wishlist: 'Needs Movies, TV shows or Music.'
};

export function needsProblem(want: Record<PartName, boolean>): string {
	if (!want.movies && !want.shows && !want.music) return 'Pick at least one of Movies, TV shows and Music.';
	if (want.subtitles && !want.movies && !want.shows) return 'Subtitles need Movies or TV shows.';
	return '';
}

export function switchPlan(now: Record<PartName, boolean>, want: Record<PartName, boolean>): { part: PartName; on: boolean }[] {
	const ons = partOrder.filter((p) => want[p] && !now[p]).map((part) => ({ part, on: true }));
	const offs = [...partOrder]
		.reverse()
		.filter((p) => !want[p] && now[p])
		.map((part) => ({ part, on: false }));
	return [...ons, ...offs];
}
