// New & Popular → Music: the answer, the hero's note and the Explore more links.
import type { MusicCardData, MusicRowData } from './music-trending.ts';
import { to } from './paths.ts';

export type NewMusicCard = MusicCardData & { badge?: string; following?: boolean };

export interface NewMusicAnswer {
	kid: boolean;
	building: boolean;
	hero: NewMusicCard[];
	rows: (MusicRowData & { items: NewMusicCard[] })[];
	/** The new releases' top genres, for Explore more. */
	genres: string[];
}

export const newMusicPath = '/explore/new/music';

function daysAgo(date: string, now: Date): number | null {
	const t = Date.parse(`${date}T00:00:00Z`);
	if (Number.isNaN(t)) return null;
	return Math.round((Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) - t) / 86400000);
}

/** heroNote is the one-line note over a hero card, made only from its data. */
export function heroNote(c: NewMusicCard, now = new Date()): string {
	const d = c.date ? daysAgo(c.date, now) : null;
	const what = c.badge === 'New EP' ? 'EP' : 'album';
	const when = d === null ? '' : d < 0 ? `out in ${-d} day${d === -1 ? '' : 's'}` : d === 0 ? 'out today' : d === 1 ? 'out yesterday' : d <= 7 ? 'out this week' : `out ${d} days ago`;
	if (c.title_id) return `From an artist in your Library${when ? `, ${when}` : ''}`;
	if (c.rank && c.rank <= 50) return `#${c.rank} on this week's album chart`;
	return when ? `New ${what}, ${when}` : `New ${what}`;
}

/** eyebrow is the small label above a hero card. */
export function eyebrow(c: NewMusicCard, now = new Date()): string {
	const d = c.date ? daysAgo(c.date, now) : null;
	if (d !== null && d < 0 && d >= -6) return `Out ${new Date(`${c.date}T00:00:00Z`).toLocaleDateString(undefined, { weekday: 'long', timeZone: 'UTC' })}`;
	return c.badge ?? 'New album';
}

/** trackNote: the artist and when, for a Fresh tracks row line. */
export function rowNote(row: Pick<MusicRowData, 'type'>, c: NewMusicCard, now = new Date()): string {
	if (row.type === 'np_music_coming' && c.date) {
		const t = new Date(`${c.date}T00:00:00Z`);
		return [c.artist, `Out ${t.toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' })}`].filter(Boolean).join(' · ');
	}
	const d = c.date ? daysAgo(c.date, now) : null;
	const when = d === null ? '' : d <= 0 ? 'Today' : d === 1 ? 'Yesterday' : `${d} days ago`;
	return [c.artist, row.type === 'np_music_climbing' ? '' : when].filter(Boolean).join(' · ');
}

export const MUSIC_DECADES = ['2020s', '2010s', '2000s', '90s', '80s', '70s'];
export const MUSIC_MOODS = ['chill', 'party', 'workout', 'focus', 'sad', 'romantic'];

/** exploreLinks are the Explore more chips: genres, then decades and moods, into Search → Music. */
export function exploreLinks(genres: string[]): { label: string; href: string; group: 'genre' | 'decade' | 'mood' | 'chart' }[] {
	const link = (g: string) => to(`/search?kind=music&mgenre=${encodeURIComponent(g)}`);
	return [
		...genres.slice(0, 8).map((g) => ({ label: g, href: link(g), group: 'genre' as const })),
		...MUSIC_DECADES.map((g) => ({ label: g, href: link(g), group: 'decade' as const })),
		...MUSIC_MOODS.map((g) => ({ label: g, href: link(g), group: 'mood' as const })),
		{ label: 'Top albums and artists', href: to('/explore?type=music'), group: 'chart' as const }
	];
}
