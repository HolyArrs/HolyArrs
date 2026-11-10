// Search's result cards: a movie, show, artist or person as a card, and the owned mark on it.
import type { SearchScope } from './search-address.ts';
import type { CardOwned } from '#lib/ui/CardBadges.svelte';
import type { PersonSummary } from '#lib/ui/people.ts';
import type { ExploreCard } from '#lib/ui/title-cards.ts';
import { appPicture, artistHref } from '#lib/music/index.ts';
import { detailPath } from '#lib/metadata/index.ts';
import type { LookupArtist } from '#lib/music/index.ts';
import { to } from '#lib/core/paths.ts';
import type { LookupMovie, TitleView } from '#lib/core/types.ts';
import type { LookupShow } from '#lib/core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type Card = { key: string; type: Exclude<SearchScope, 'all'>; titleId?: number; name: string; year?: number; image?: string | null; href: string; owned: CardOwned | null; preview?: { kind: 'movie' | 'show'; tmdb: string; year?: number; titleId?: number }; artist?: LookupArtist; subtitle?: string; /** A movie or show as an Explore card, for its Add or Ask. */ ec?: ExploreCard };

export type LibraryAnswer = { titles: TitleView[]; albums: { id: number; title_id: number; name: string; artist: string; year?: number; image?: string }[]; people: { name: string; titles: { id: number; name: string; kind: string }[] }[] };

export const inLibrary: CardOwned = { state: 'available', get label() { return t('discover.searchCards.inYourLibrary'); } };
function askMark(ask: { state: string; mine: boolean } | undefined | null): CardOwned | null {
	if (!ask || ask.state === 'turned_down') return null;
	return { state: 'requested', label: ask.state === 'approved' ? t('discover.searchCards.approvedWaiting') : t('discover.searchCards.requested') };
}
/** A search result as an Explore card, so its + adds or asks the same way. */
const asCard = (key: string, kind: 'movie' | 'show', name: string, year: number | undefined, tmdb: string, titleId: number | undefined, ask: LookupMovie['ask']): ExploreCard =>
	({ key, kind, name, year, titleId, ref: `tmdb:${tmdb}`, status: titleId ? 'in_library' : undefined, badges: [], mine: !!ask?.mine, ask: ask && ask.state !== 'turned_down' ? ask : undefined });
export const movieCard = (m: LookupMovie): Card | null =>
	m.ids.tmdb
		? { key: `m${m.ids.tmdb}`, type: 'movie', titleId: m.title_id || undefined, name: m.name, year: m.year || undefined, subtitle: m.year ? String(m.year) : undefined, image: m.poster, href: to(m.title_id ? `/library/${m.title_id}` : detailPath('movie', m.ids.tmdb)), owned: m.title_id ? inLibrary : askMark(m.ask), preview: { kind: 'movie', tmdb: m.ids.tmdb, year: m.year || undefined, titleId: m.title_id || undefined }, ec: asCard(`m${m.ids.tmdb}`, 'movie', m.name, m.year || undefined, m.ids.tmdb, m.title_id || undefined, m.ask) }
		: null;
export const showCard = (s: LookupShow, i: number): Card => {
	const tmdb = s.ids.tmdb;
	return { key: `s${s.source}${s.ids[s.source] ?? s.name}${i}`, type: 'show', titleId: s.title_id || undefined, name: s.name, year: s.year, subtitle: s.year ? String(s.year) : undefined, image: s.poster, href: to(s.title_id ? `/library/${s.title_id}` : tmdb ? detailPath('show', tmdb) : `/search?q=${encodeURIComponent(s.name)}&kind=show`), owned: s.title_id ? inLibrary : askMark(s.ask), preview: tmdb ? { kind: 'show', tmdb, year: s.year, titleId: s.title_id || undefined } : undefined, ec: tmdb ? asCard(`s${s.source}${s.ids[s.source] ?? s.name}${i}`, 'show', s.name, s.year, tmdb, s.title_id || undefined, s.ask) : undefined };
};
export const artistCard = (a: LookupArtist): Card => ({ key: `a${a.mbid}`, type: 'music', titleId: a.title_id || undefined, name: a.name, subtitle: a.disambiguation || [a.type, a.country].filter(Boolean).join(' · ') || undefined, image: appPicture(a.image), href: artistHref(a.mbid), owned: a.title_id ? inLibrary : askMark(a.ask), artist: a });
export const personCard = (p: PersonSummary): Card => ({ key: `p${p.id}`, type: 'people', name: p.name, subtitle: p.department, image: p.profile, href: to(`/people/${encodeURIComponent(p.id)}`), owned: null });
