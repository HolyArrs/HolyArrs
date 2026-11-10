// Words and small rules for the music screens: album choices and kinds, album and artist progress, track lengths and Pressing names.
import type { PressingView } from './types.ts';
import type { AlbumSummary, ArtistInfo, AskAlbum } from '../core/types.ts';
import { to } from '../core/paths.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type DiscographyRow = 'albums' | 'singles' | 'live' | 'compilations' | 'other';

/** A [value, label, hint?] row. The words are read when used, so they follow the language. */
function row(value: string, ...keys: string[]): string[] {
	const out = [value, ...keys.map(() => '')];
	keys.forEach((k, i) => Object.defineProperty(out, i + 1, { get: () => t(k), enumerable: true }));
	return out;
}

/** The add-an-artist choices of which albums to keep up to date. */
export const albumChoices = [
	row('all', 'music.everyAlbum', 'music.everyAlbumTheAlbum'),
	row('future', 'music.albumsStillToCome', 'music.onlyAlbumsReleasedFrom'),
	row('missing', 'music.albumsWithoutFiles', 'music.albumsWithNoTrack'),
	row('latest', 'music.theLatestAlbum', 'music.onlyTheNewestAlbum'),
	row('first', 'music.theFirstAlbum', 'music.onlyTheOldestAlbum'),
	row('none', 'music.noneForNow', 'music.nothingYetSwitchAlbums')
] as [string, string, string][];

/** What happens to an album MusicBrainz lists later. */
export const newAlbumChoices = [
	row('all', 'music.keepThemUpTo'),
	row('new', 'music.onlyAlbumsReleasedAfter'),
	row('none', 'music.leaveThemOff')
] as [string, string][];

/** The main album kinds an artist can include. */
export const mainKinds = [
	row('album', 'music.album'),
	row('ep', 'music.ep'),
	row('single', 'music.single'),
	row('broadcast', 'music.broadcast'),
	row('other', 'music.other')
] as [string, string][];

/** The extra album kinds; studio means "no extra kind". */
export const extraKinds = [
	row('studio', 'music.noExtraKindStudio'),
	row('live', 'music.live'),
	row('compilation', 'music.compilation'),
	row('soundtrack', 'music.soundtrack'),
	row('remix', 'music.remix'),
	row('spokenword', 'music.spokenWord'),
	row('interview', 'music.interview'),
	row('audiobook', 'music.audiobook'),
	row('audio drama', 'music.audioDrama'),
	row('dj-mix', 'music.djMix'),
	row('mixtape/street', 'music.mixtape'),
	row('demo', 'music.demo'),
	row('field recording', 'music.fieldRecording')
] as [string, string][];

/** The Pressing statuses that may count. */
export const pressingStatuses = [
	row('official', 'music.official'),
	row('promotion', 'music.promotion'),
	row('bootleg', 'music.bootleg'),
	row('pseudo-release', 'music.translationOrTransliteration')
] as [string, string][];

/** Which Pressing is chosen for an album. */
export const pressingRules = [
	row('common', 'music.theUsualOne', 'music.theTrackCountMost'),
	row('most_tracks', 'music.theMostTracks', 'music.thePressingWithThe'),
	row('earliest', 'music.theEarliest', 'music.theFirstPressingTo')
] as [string, string, string][];

/** Finds the label for a value in a list of [value, label, ...]. */
export function word(list: readonly (readonly string[])[], v: string | undefined): string {
	return list.find((x) => x[0] === v)?.[1] ?? v ?? '';
}

/** An album's kinds: "Album", "Album · Live". */
export function albumKind(a: { type: string; secondary_types?: string[] }): string {
	const main = a.type ? word(mainKinds, a.type) : t('music.noKind');
	const extra = (a.secondary_types ?? []).map((x) => word(extraKinds, x));
	return [main, ...extra].join(' · ');
}

/** The year an album came out, from a date or "YYYY[-MM[-DD]]". */
export function albumYear(date: string | undefined): string {
	return date ? date.slice(0, 4) : '';
}

const albumStates = (): Record<string, [string, string]> => ({
	not_included: [t('music.notIncluded'), ''],
	no_tracks: [t('music.noTracksYet'), ''],
	complete: [t('music.inLibrary'), 'ok'],
	not_out_yet: [t('music.notOutYet'), ''],
	not_kept: [t('music.notKept'), ''],
	partial: [t('music.someTracksIn'), 'wait'],
	missing: [t('music.missing'), 'wait']
});

/** An album's state in words and its chip colour. */
export function albumState(state: string): { label: string; tone: string } {
	const [label, tone] = albumStates()[state] ?? [state, ''];
	return { label, tone };
}

/** An album's state, saying Not out yet for one without tracks that comes out later. */
export function albumStateOf(a: Pick<AlbumSummary, 'state' | 'release_date'>, now = Date.now()): { label: string; tone: string } {
	if (a.state === 'no_tracks' && a.release_date && Date.parse(a.release_date) > now) return albumState('not_out_yet');
	return albumState(a.state);
}

/** Sums an artist up: "3 of 12 tracks in the Library · 9 missing". */
export function artistProgress(a: ArtistInfo | undefined): string {
	if (!a) return '';
	if (!a.tracks) return t('music.ofAlbumsIncludedTracks', { included: a.included, albums: a.albums });
	const parts = [t('music.ofTracksInThe', { n: a.in_library, total: a.tracks })];
	if (a.missing) parts.push(t('music.missing2', { n: a.missing }));
	if (a.not_out_yet) parts.push(t('music.notOutYet2', { n: a.not_out_yet }));
	return parts.join(' · ');
}

/** A track's length as m:ss. */
export function trackLength(ms: number | undefined): string {
	if (!ms || ms < 0) return '';
	const s = Math.round(ms / 1000);
	return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
}

/** A Pressing's name: "Harbour Songs (deluxe) · 2019 · GB · CD · 4 tracks". */
export function pressingLabel(p: Pick<PressingView, 'name' | 'disambiguation' | 'date' | 'country' | 'formats' | 'track_count'>): string {
	const formats = [...new Set(p.formats.filter(Boolean))];
	const bits = [p.disambiguation ? `${p.name} (${p.disambiguation})` : p.name];
	if (p.date) bits.push(p.date.slice(0, 4));
	if (p.country) bits.push(p.country);
	if (formats.length) bits.push(formats.join(' + '));
	bits.push(p.track_count === 1 ? t('music.track', { n: p.track_count }) : t('music.tracks', { n: p.track_count }));
	return bits.join(' · ');
}

/** Which albums an ask is for, like the server. */
export function albumWords(albums: AskAlbum[] | undefined): string {
	const a = albums ?? [];
	if (a.length === 0) return t('music.everyAlbum2');
	if (a.length === 1) return a[0].name;
	if (a.length <= 3) return t('music.and', { list: a.slice(0, -1).map((x) => x.name).join(', '), last: a[a.length - 1].name });
	return t('music.andMoreAlbums', { first: a[0].name, second: a[1].name, n: a.length - 2 });
}

/** Adds or removes v from a list, keeping the order of all. */
export function toggle(list: string[], v: string, on: boolean, all: readonly (readonly string[])[]): string[] {
	const next = on ? [...new Set([...list, v])] : list.filter((x) => x !== v);
	const order = all.map((x) => x[0]);
	return next.sort((a, b) => order.indexOf(a) - order.indexOf(b));
}

/** The discography rows of an artist page, in page order. */
export const DISCOGRAPHY = [
	row('albums', 'music.albums'),
	row('singles', 'music.singlesAndEps'),
	row('live', 'music.live'),
	row('compilations', 'music.compilations'),
	row('other', 'music.otherReleases')
] as unknown as readonly (readonly [DiscographyRow, string])[];

/** Places an album: live and compilations first by their extra kind, then by the main kind. */
export function discographyRow(a: Pick<AlbumSummary, 'type' | 'secondary_types'>): DiscographyRow {
	const extra = a.secondary_types ?? [];
	if (extra.includes('live')) return 'live';
	if (extra.includes('compilation')) return 'compilations';
	if (a.type === 'album') return extra.length ? 'other' : 'albums';
	if (a.type === 'single' || a.type === 'ep') return 'singles';
	return 'other';
}

/** Groups albums into rows, newest first; rows with nothing are left out. */
export function discography<T extends Pick<AlbumSummary, 'type' | 'secondary_types' | 'release_date'>>(albums: T[]): { row: DiscographyRow; label: string; albums: T[] }[] {
	const newest = (x: T, y: T) => (y.release_date ?? '').localeCompare(x.release_date ?? '');
	return DISCOGRAPHY.map(([row, label]) => ({ row, label, albums: albums.filter((a) => discographyRow(a) === row).sort(newest) })).filter((g) => g.albums.length);
}

/** The address of a Library album's cover, served from the artwork cache. */
export const albumCover = (id: number) => to(`/api/v1/albums/${id}/cover`);
