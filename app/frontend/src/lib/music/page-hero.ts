import type { MusicCardData, MusicRowData } from './music-trending.ts';

// The Browse page hero's words: what the genre, mood, season, decade or activity sounds like, then the page's leading artists.
const blurbs: Record<string, string> = {
	'genre:pop': 'Big hooks and bright choruses',
	'genre:hip-hop': 'Hard beats, sharp verses and the songs everyone quotes',
	'genre:rnb': 'Smooth vocals and slow grooves',
	'genre:rock': 'Loud guitars, live drums and anthems',
	'genre:alternative': 'Off-centre guitars and big feelings',
	'genre:indie': 'Small labels, bedroom studios and songs that stick',
	'genre:latin': 'Reggaeton, salsa, corridos and more',
	'genre:country': 'Story songs, steel guitar and back roads',
	'genre:dance': 'Club beats, synths and drops',
	'genre:kpop': 'Polished pop, sharp choreography and huge fandoms',
	'genre:metal': 'Heavy riffs and fast drums',
	'genre:afro': 'Afrobeats, amapiano and the sounds of the continent',
	'genre:jazz': 'Swing, improvisation and late-night horns',
	'genre:classical': 'Orchestras, piano and the great composers',
	'genre:soul-funk': 'Deep grooves, horns and big voices',
	'genre:folk': 'Acoustic guitars and honest songs',
	'genre:reggae': 'Offbeat rhythms and island sunshine',
	'genre:blues': 'Bent notes and hard-lived stories',
	'genre:punk': 'Three chords, fast and loud',
	'genre:christian': 'Worship, gospel and songs of faith',
	'genre:soundtracks': 'Music from films, shows and games',
	'genre:kids': 'Songs for the whole family',
	'mood:chill': 'Easy, unhurried music',
	'mood:party': 'Songs that fill a dance floor',
	'mood:love': 'Songs about falling for someone',
	'mood:romance': 'Slow, close and late at night',
	'mood:sad': 'Songs for heavy days',
	'mood:happy': 'Bright songs that lift the room',
	'mood:focus': 'Music that stays out of your way',
	'mood:sleep': 'Soft and slow, for winding down',
	'mood:workout': 'Fast tempos to keep you moving',
	'mood:energy': 'Loud, fast and full of drive',
	'mood:feel-good': 'Warm songs that just feel right',
	'mood:throwback': 'Long-loved songs from years ago'
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
	const lead = blurbs[key] ?? (kind === 'decade' ? `The albums that defined the ${name}` : kind === 'season' ? `Music for ${name}` : kind === 'activity' ? `Music for ${name.toLowerCase()}` : `The best of ${name}`);
	return artists.length ? `${lead}: ${artists.join(', ')} and more` : lead;
}
