// New & Popular > Music: the answer and the notes over its hero and rows.
import type { MusicCardData, MusicRowData } from './music-trending.ts';

/** explicit: explicit lyrics (the "E" mark). track: a Fresh tracks song, whose card asks for its album. */
export type NewMusicCard = MusicCardData & { badge?: string; following?: boolean; explicit?: boolean; track?: string };

export interface NewMusicAnswer {
	kid: boolean;
	building: boolean;
	hero: NewMusicCard[];
	rows: (MusicRowData & { items: NewMusicCard[] })[];
	/** The new releases' top genres. */
	genres: string[];
}

/** The art badges of a New music album card: the explicit "E" only. */
export function cardBadges(c: Pick<NewMusicCard, 'explicit'>): string[] {
	return c.explicit ? ['E'] : [];
}

export const newMusicPath = '/explore/new/music';

/** A card says "New" only within 60 days of its first release. */
export const NEW_DAYS = 60;

/** Out (or due within a week) and at most 60 days old. */
export function isNewRelease(date: string | undefined, now = new Date()): boolean {
	const d = date ? daysAgo(date, now) : null;
	return d !== null && d >= -7 && d <= NEW_DAYS;
}

/** The one-line note over a hero card, made only from its data. */
export function heroNote(c: NewMusicCard, now = new Date()): string {
	const d = c.date ? daysAgo(c.date, now) : null;
	const what = c.badge === 'New EP' ? 'EP' : 'album';
	const old = d !== null && d > NEW_DAYS && c.date ? olderWords(c.date, d) : '';
	const when = d === null || old ? '' : d < 0 ? `out in ${-d} day${d === -1 ? '' : 's'}` : d === 0 ? 'out today' : d === 1 ? 'out yesterday' : d <= 7 ? 'out this week' : `out ${d} days ago`;
	if (c.title_id) return `From an artist in your Library${when ? `, ${when}` : old ? ` · ${old}` : ''}`;
	if (c.rank && c.rank <= 50) return `#${c.rank} on this week's album chart${old ? ` · ${old}` : ''}`;
	if (old) return old;
	return when ? `New ${what}, ${when}` : '';
}

/** The small label above a hero card: "New album/EP" only within 60 days of release; otherwise the card's real reason (its chart place, Climbing, Popular now). */
export function eyebrow(c: NewMusicCard, now = new Date()): string {
	const d = c.date ? daysAgo(c.date, now) : null;
	if (d !== null && d < 0 && d >= -6) return `Out ${new Date(`${c.date}T00:00:00Z`).toLocaleDateString(undefined, { weekday: 'long', timeZone: 'UTC' })}`;
	if (isNewRelease(c.date, now)) return c.badge?.startsWith('New') ? c.badge : 'New album';
	if (c.kicker && !/^new\b/i.test(c.kicker)) return c.kicker;
	return 'Popular now';
}

/** The artist and when, for a Fresh tracks row line. */
export function rowNote(row: Pick<MusicRowData, 'type'>, c: NewMusicCard, now = new Date()): string {
	if (row.type === 'np_music_coming' && c.date) {
		const t = new Date(`${c.date}T00:00:00Z`);
		return [c.artist, `Out ${t.toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' })}`].filter(Boolean).join(' · ');
	}
	const d = c.date ? daysAgo(c.date, now) : null;
	const when = d === null || !c.date ? '' : d <= 0 ? 'Today' : d === 1 ? 'Yesterday' : d <= NEW_DAYS ? `${d} days ago` : d <= 365 ? shortDate(c.date) : c.date.slice(0, 4);
	return [c.artist, row.type === 'np_music_climbing' ? (c.note ?? '') : when].filter(Boolean).join(' · ');
}

function daysAgo(date: string, now: Date): number | null {
	const t = Date.parse(`${date}T00:00:00Z`);
	if (Number.isNaN(t)) return null;
	return Math.round((Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()) - t) / 86400000);
}

function shortDate(date: string): string {
	return new Date(`${date}T00:00:00Z`).toLocaleDateString(undefined, { month: 'short', day: 'numeric', timeZone: 'UTC' });
}

/** "Released Mar 4" within a year, else "2013 album". */
function olderWords(date: string, d: number): string {
	return d > 365 ? `${date.slice(0, 4)} album` : `Released ${shortDate(date)}`;
}
