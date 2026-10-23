// Words and small rules for the music screens: album choices and kinds,
// album and artist progress, track lengths and Pressing names.
import type { AlbumSummary, ArtistInfo, AskAlbum, PressingView } from '../types.ts';
import { to } from '../paths.ts';

/** albumChoices are the add-an-artist choices of which albums to keep up to date. */
export const albumChoices: [string, string, string][] = [
	['all', 'Every album', 'Every album the album kinds include.'],
	['future', 'Albums still to come', 'Only albums released from now on.'],
	['missing', 'Albums without files', 'Albums with no track in the Library yet.'],
	['latest', 'The latest album', 'Only the newest album that is out.'],
	['first', 'The first album', 'Only the oldest album.'],
	['none', 'None for now', 'Nothing yet; switch albums on from the artist page.']
];

/** newAlbumChoices say what happens to an album MusicBrainz lists later. */
export const newAlbumChoices: [string, string][] = [
	['all', 'Keep them up to date'],
	['new', 'Only albums released after the artist was added'],
	['none', 'Leave them off']
];

/** mainKinds are the main album kinds an artist can include. */
export const mainKinds: [string, string][] = [
	['album', 'Album'],
	['ep', 'EP'],
	['single', 'Single'],
	['broadcast', 'Broadcast'],
	['other', 'Other']
];

/** extraKinds are the extra album kinds; studio means "no extra kind". */
export const extraKinds: [string, string][] = [
	['studio', 'No extra kind (studio)'],
	['live', 'Live'],
	['compilation', 'Compilation'],
	['soundtrack', 'Soundtrack'],
	['remix', 'Remix'],
	['spokenword', 'Spoken word'],
	['interview', 'Interview'],
	['audiobook', 'Audiobook'],
	['audio drama', 'Audio drama'],
	['dj-mix', 'DJ mix'],
	['mixtape/street', 'Mixtape'],
	['demo', 'Demo'],
	['field recording', 'Field recording']
];

/** pressingStatuses are the Pressing statuses that may count. */
export const pressingStatuses: [string, string][] = [
	['official', 'Official'],
	['promotion', 'Promotion'],
	['bootleg', 'Bootleg'],
	['pseudo-release', 'Translation or transliteration']
];

/** pressingRules say which Pressing is chosen for an album. */
export const pressingRules: [string, string, string][] = [
	['common', 'The usual one', 'The track count most Pressings share, then a digital one, then the earliest.'],
	['most_tracks', 'The most tracks', 'The Pressing with the most tracks, such as a deluxe edition.'],
	['earliest', 'The earliest', 'The first Pressing to come out.']
];

/** word finds the label for a value in a list of [value, label, ...]. */
export function word(list: readonly (readonly string[])[], v: string | undefined): string {
	return list.find((x) => x[0] === v)?.[1] ?? v ?? '';
}

/** albumKind names an album's kinds: "Album", "Album · Live". */
export function albumKind(a: { type: string; secondary_types?: string[] }): string {
	const main = a.type ? word(mainKinds, a.type) : 'No kind';
	const extra = (a.secondary_types ?? []).map((x) => word(extraKinds, x));
	return [main, ...extra].join(' · ');
}

/** albumYear is the year an album came out, from a date or "YYYY[-MM[-DD]]". */
export function albumYear(date: string | undefined): string {
	return date ? date.slice(0, 4) : '';
}

const albumStates: Record<string, [string, string]> = {
	not_included: ['Not included', ''],
	no_tracks: ['No tracks yet', ''],
	complete: ['In library', 'ok'],
	not_out_yet: ['Not out yet', ''],
	not_kept: ['Not kept up to date', ''],
	partial: ['Some tracks in', 'wait'],
	missing: ['Missing', 'wait']
};

/** albumState is an album's state in words and its chip colour. */
export function albumState(state: string): { label: string; tone: string } {
	const [label, tone] = albumStates[state] ?? [state, ''];
	return { label, tone };
}

/** albumStateOf is an album's state, saying Not out yet for one without tracks that comes out later. */
export function albumStateOf(a: Pick<AlbumSummary, 'state' | 'release_date'>, now = Date.now()): { label: string; tone: string } {
	if (a.state === 'no_tracks' && a.release_date && Date.parse(a.release_date) > now) return albumState('not_out_yet');
	return albumState(a.state);
}

/** albumLine counts an album's tracks: "8 of 12 tracks in the Library". */
export function albumLine(a: Pick<AlbumSummary, 'tracks' | 'in_library' | 'missing' | 'included'>): string {
	if (!a.included) return 'Left out by the album kinds';
	if (!a.tracks) return 'Tracks come with the next refresh';
	const parts = [`${a.in_library} of ${a.tracks} tracks in the Library`];
	if (a.missing) parts.push(`${a.missing} missing`);
	return parts.join(' · ');
}

/** artistProgress sums an artist up: "3 of 12 tracks in the Library · 9 missing". */
export function artistProgress(a: ArtistInfo | undefined): string {
	if (!a) return '';
	if (!a.tracks) return `${a.included} of ${a.albums} albums included; tracks come with the next refresh.`;
	const parts = [`${a.in_library} of ${a.tracks} tracks in the Library`];
	if (a.missing) parts.push(`${a.missing} missing`);
	if (a.not_out_yet) parts.push(`${a.not_out_yet} not out yet`);
	return parts.join(' · ');
}

/** artistState is one chip for an artist in the Library list. */
export function artistState(a: ArtistInfo | undefined): { state: string; label: string } {
	if (!a) return { state: '', label: '' };
	if (a.missing) return { state: 'wanted', label: `${a.missing} missing` };
	if (a.tracks && a.in_library === a.tracks) return { state: 'in_library', label: 'In library' };
	if (a.not_out_yet) return { state: 'not_out_yet', label: 'Not out yet' };
	if (a.in_library > 0) return { state: 'in_library', label: `${a.in_library} ${a.in_library === 1 ? 'track' : 'tracks'} in library` };
	return { state: '', label: '' };
}

/** trackLength says a track's length as m:ss. */
export function trackLength(ms: number | undefined): string {
	if (!ms || ms < 0) return '';
	const s = Math.round(ms / 1000);
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** pressingLabel names a Pressing: "Harbour Songs (deluxe) · 2019 · GB · CD · 4 tracks". */
export function pressingLabel(p: Pick<PressingView, 'name' | 'disambiguation' | 'date' | 'country' | 'formats' | 'track_count'>): string {
	const formats = [...new Set(p.formats.filter(Boolean))];
	const bits = [p.disambiguation ? `${p.name} (${p.disambiguation})` : p.name];
	if (p.date) bits.push(p.date.slice(0, 4));
	if (p.country) bits.push(p.country);
	if (formats.length) bits.push(formats.join(' + '));
	bits.push(`${p.track_count} ${p.track_count === 1 ? 'track' : 'tracks'}`);
	return bits.join(' · ');
}

/** albumWords says which albums an ask is for, like the server. */
export function albumWords(albums: AskAlbum[] | undefined): string {
	const a = albums ?? [];
	if (a.length === 0) return 'every album';
	if (a.length === 1) return a[0].name;
	if (a.length <= 3) return `${a.slice(0, -1).map((x) => x.name).join(', ')} and ${a[a.length - 1].name}`;
	return `${a[0].name}, ${a[1].name} and ${a.length - 2} more albums`;
}

/** toggle adds or removes v from a list, keeping the order of all. */
export function toggle(list: string[], v: string, on: boolean, all: readonly (readonly string[])[]): string[] {
	const next = on ? [...new Set([...list, v])] : list.filter((x) => x !== v);
	const order = all.map((x) => x[0]);
	return next.sort((a, b) => order.indexOf(a) - order.indexOf(b));
}

// artistSearchToast says what a Search now on a whole artist queued and how
// many album searches it left for the next click (one click queues at most
// the artist search limit in Settings > Search).
export function artistSearchToast(queued: number, leftOut = 0): string {
	if (!queued) return 'No album is missing a track right now.';
	const s = `Searching for ${queued} ${queued === 1 ? 'album' : 'albums'}, a little apart. Home shows what they find.`;
	return leftOut > 0 ? `${s} ${leftOut} more ${leftOut === 1 ? 'is' : 'are'} left for the next Search now or the nightly search.` : s;
}

/** The discography rows of an artist page, in page order. */
export const DISCOGRAPHY = [
	['albums', 'Albums'],
	['singles', 'Singles and EPs'],
	['live', 'Live'],
	['compilations', 'Compilations'],
	['other', 'Other releases']
] as const;
export type DiscographyRow = (typeof DISCOGRAPHY)[number][0];

/** discographyRow places an album: live and compilations first by their extra kind, then by the main kind. */
export function discographyRow(a: Pick<AlbumSummary, 'type' | 'secondary_types'>): DiscographyRow {
	const extra = a.secondary_types ?? [];
	if (extra.includes('live')) return 'live';
	if (extra.includes('compilation')) return 'compilations';
	if (a.type === 'album') return extra.length ? 'other' : 'albums';
	if (a.type === 'single' || a.type === 'ep') return 'singles';
	return 'other';
}

/** discography groups albums into rows, newest first; rows with nothing are left out. */
export function discography<T extends Pick<AlbumSummary, 'type' | 'secondary_types' | 'release_date'>>(albums: T[]): { row: DiscographyRow; label: string; albums: T[] }[] {
	const newest = (x: T, y: T) => (y.release_date ?? '').localeCompare(x.release_date ?? '');
	return DISCOGRAPHY.map(([row, label]) => ({ row, label, albums: albums.filter((a) => discographyRow(a) === row).sort(newest) })).filter((g) => g.albums.length);
}

/** latestRelease is the newest album or EP that is out (never one still to come). */
export function latestRelease<T extends Pick<AlbumSummary, 'type' | 'secondary_types' | 'release_date'>>(albums: T[], now = Date.now()): T | null {
	const out = albums.filter((a) => a.release_date && Date.parse(a.release_date) <= now && (discographyRow(a) === 'albums' || discographyRow(a) === 'singles'));
	return out.sort((x, y) => (y.release_date ?? '').localeCompare(x.release_date ?? ''))[0] ?? null;
}

/** albumCover is the address of a Library album's cover (served from the artwork cache). */
export const albumCover = (id: number) => to(`/api/v1/albums/${id}/cover`);
