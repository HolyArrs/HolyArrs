// Extra artwork (fanart.tv) and outside ratings (IMDb, Rotten Tomatoes,
// Metacritic…) from GET /api/v1/extras/{kind}/{id}. The server already
// drops scores with too few votes; anything missing comes back empty, so
// the page keeps TMDB's art and hides the strip.

export type OutsideScore = { source: string; value: number; max: number; votes: number };
export type Extras = { logo?: string; thumb?: string; backgrounds: string[]; scores: OutsideScore[] };

const ORDER = ['imdb', 'rt_critics', 'rt_audience', 'metacritic', 'letterboxd', 'trakt'];
const NAMES: Record<string, string> = { imdb: 'IMDb', rt_critics: 'Rotten Tomatoes critics', rt_audience: 'Rotten Tomatoes audience', metacritic: 'Metacritic', letterboxd: 'Letterboxd', trakt: 'Trakt' };
const EMPTY: Extras = { backgrounds: [], scores: [] };

const fanart = (v: unknown) => (typeof v === 'string' && v.startsWith('https://assets.fanart.tv/') ? v : undefined);

/** cleanExtras keeps fanart.tv pictures and known, sane scores, in strip order. */
export function cleanExtras(raw: unknown): Extras {
	if (!raw || typeof raw !== 'object') return EMPTY;
	const r = raw as Record<string, unknown>;
	const scores = (Array.isArray(r.scores) ? r.scores : [])
		.filter((s): s is OutsideScore => !!s && ORDER.includes(s.source) && Number.isFinite(s.value) && Number.isFinite(s.max) && s.value > 0 && s.max > 0 && s.value <= s.max)
		.sort((a, b) => ORDER.indexOf(a.source) - ORDER.indexOf(b.source));
	const backgrounds = (Array.isArray(r.backgrounds) ? r.backgrounds : []).map(fanart).filter((u): u is string => !!u);
	return { logo: fanart(r.logo), thumb: fanart(r.thumb), backgrounds, scores };
}

/** scoreText is how a score reads in the strip: "8.7", "83%", "73", "4.2". */
export function scoreText(s: OutsideScore): string {
	if (s.max === 100) return s.source === 'metacritic' ? String(Math.round(s.value)) : `${Math.round(s.value)}%`;
	return (Math.round(s.value * 10) / 10).toFixed(1);
}

/** scoreName is the source's full name, for the label and tooltip. */
export const scoreName = (source: string) => NAMES[source] ?? source;

const cache = new Map<string, Promise<Extras>>();

/** loadExtras asks once per title (kind movie, show or artist); failures answer empty. */
export function loadExtras(get: <T>(path: string) => Promise<T>, kind: 'movie' | 'show' | 'artist', id: string | null | undefined): Promise<Extras> {
	if (!id) return Promise.resolve(EMPTY);
	const key = `${kind}:${id}`;
	let p = cache.get(key);
	if (!p) {
		p = get<unknown>(`/extras/${kind}/${encodeURIComponent(id)}`).then(cleanExtras, () => EMPTY);
		cache.set(key, p);
	}
	return p;
}
