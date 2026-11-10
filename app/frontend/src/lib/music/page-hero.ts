import type { MusicCardData, MusicRowData } from './music-trending.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

// The Browse page hero's words: what the genre, mood, season, decade or activity sounds like, then the page's leading artists.
const blurbs: Record<string, string> = {
	'genre:pop': 'music.bigHooksAndBright',
	'genre:hip-hop': 'music.hardBeatsSharpVerses',
	'genre:rnb': 'music.smoothVocalsAndSlow',
	'genre:rock': 'music.loudGuitarsLiveDrums',
	'genre:alternative': 'music.offCentreGuitarsAnd',
	'genre:indie': 'music.smallLabelsBedroomStudios',
	'genre:latin': 'music.reggaetonSalsaCorridosAnd',
	'genre:country': 'music.storySongsSteelGuitar',
	'genre:dance': 'music.clubBeatsSynthsAnd',
	'genre:kpop': 'music.polishedPopSharpChoreography',
	'genre:metal': 'music.heavyRiffsAndFast',
	'genre:afro': 'music.afrobeatsAmapianoAndThe',
	'genre:jazz': 'music.swingImprovisationAndLate',
	'genre:classical': 'music.orchestrasPianoAndThe',
	'genre:soul-funk': 'music.deepGroovesHornsAnd',
	'genre:folk': 'music.acousticGuitarsAndHonest',
	'genre:reggae': 'music.offbeatRhythmsAndIsland',
	'genre:blues': 'music.bentNotesAndHard',
	'genre:punk': 'music.threeChordsFastAnd',
	'genre:christian': 'music.worshipGospelAndSongs',
	'genre:soundtracks': 'music.musicFromFilmsShows',
	'genre:kids': 'music.songsForTheWhole',
	'mood:chill': 'music.easyUnhurriedMusic',
	'mood:party': 'music.songsThatFillA',
	'mood:love': 'music.songsAboutFallingFor',
	'mood:romance': 'music.slowCloseAndLate',
	'mood:sad': 'music.songsForHeavyDays',
	'mood:happy': 'music.brightSongsThatLift',
	'mood:focus': 'music.musicThatStaysOut',
	'mood:sleep': 'music.softAndSlowFor',
	'mood:workout': 'music.fastTemposToKeep',
	'mood:energy': 'music.loudFastAndFull',
	'mood:feel-good': 'music.warmSongsThatJust',
	'mood:throwback': 'music.longLovedSongsFrom'
};

/** The page's leading artists, once each, in row order. */
export function heroArtists(hero: MusicCardData[], rows: MusicRowData[], n = 3): string[] {
	const out: string[] = [];
	for (const c of [...hero, ...rows.flatMap((r) => r.items)]) {
		const a = c.kind === 'artist' ? c.name : c.artist;
		if (a && !out.includes(a)) out.push(a);
		if (out.length === n) break;
	}
	return out;
}

/** The one-line description: the key's words, then its artists. */
export function heroLine(key: string, name: string, artists: string[]): string {
	const kind = key.split(':')[0];
	const lead = blurbs[key] ? t(blurbs[key]) : kind === 'decade' ? t('music.theAlbumsThatDefined', { name }) : kind === 'season' ? t('music.musicFor', { name }) : kind === 'activity' ? t('music.musicFor', { name: name.toLowerCase() }) : t('music.theBestOf', { name });
	return artists.length ? t('music.andMore', { lead, list: artists.join(', ') }) : lead;
}
