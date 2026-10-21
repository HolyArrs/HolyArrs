// Awards (Wikidata, CC0) and the Wikipedia bio (CC BY-SA 4.0) for a title
// or person, from GET /api/v1/wiki/{kind}/{id}. Anything missing comes
// back empty, so the page hides the badge and keeps TMDB's biography.

export type WikiAward = { name: string; family: string; won: boolean; year: number; for_work: string };
export type WikiBio = { title: string; extract: string; url: string; license: string; license_url: string; attribution: string };
export type WikiFacts = { badges: string[]; awards: WikiAward[]; born?: string; died?: string; birth_place?: string; death_place?: string; bio: WikiBio | null };

const EMPTY: WikiFacts = { badges: [], awards: [], bio: null };

const str = (v: unknown) => (typeof v === 'string' ? v : '');

export const CC_BY_SA = 'https://creativecommons.org/licenses/by-sa/4.0/';

/** wikipediaURL keeps an https link on a *.wikipedia.org host, else ''. */
export function wikipediaURL(v: unknown): string {
	try {
		const u = new URL(str(v));
		return u.protocol === 'https:' && (u.hostname === 'wikipedia.org' || u.hostname.endsWith('.wikipedia.org')) ? u.href : '';
	} catch {
		return '';
	}
}

/** licenseURL keeps a Creative Commons https link, else the CC BY-SA 4.0 one. */
export const licenseURL = (v: unknown) => (str(v).startsWith('https://creativecommons.org/') && !/[\s"<>]/.test(str(v)) ? str(v) : CC_BY_SA);

/** cleanWikiFacts keeps sane badges and awards and a Wikipedia bio only with its link and license. */
export function cleanWikiFacts(raw: unknown): WikiFacts {
	if (!raw || typeof raw !== 'object') return EMPTY;
	const r = raw as Record<string, unknown>;
	const badges = (Array.isArray(r.badges) ? r.badges : []).filter((b): b is string => typeof b === 'string' && b.length > 0 && b.length < 40);
	const awards = (Array.isArray(r.awards) ? r.awards : [])
		.filter((a): a is WikiAward => !!a && typeof a === 'object' && typeof (a as WikiAward).name === 'string' && (a as WikiAward).name !== '')
		.map((a) => ({ name: a.name, family: str(a.family), won: a.won === true, year: Number.isFinite(a.year) ? a.year : 0, for_work: str(a.for_work) }));
	const b = r.bio as Record<string, unknown> | null | undefined;
	const bio = b && typeof b === 'object' && str(b.extract) && wikipediaURL(b.url) && str(b.license)
		? { title: str(b.title), extract: str(b.extract), url: wikipediaURL(b.url), license: str(b.license), license_url: licenseURL(b.license_url), attribution: str(b.attribution) }
		: null;
	return { badges, awards, born: str(r.born) || undefined, died: str(r.died) || undefined, birth_place: str(r.birth_place) || undefined, death_place: str(r.death_place) || undefined, bio };
}

/** awardsTip lists the awards for the badge's hover: wins first, a few lines. */
export function awardsTip(awards: WikiAward[], max = 6): string {
	const line = (a: WikiAward) => `${a.won ? 'Won' : 'Nominated'}: ${a.name}${a.year ? ` (${a.year})` : ''}`;
	const major = awards.filter((a) => a.family);
	const list = (major.length ? major : awards).slice(0, max).map(line);
	const more = (major.length ? major : awards).length - list.length;
	return more > 0 ? [...list, `and ${more} more`].join('\n') : list.join('\n');
}

const cache = new Map<string, Promise<WikiFacts>>();

/** loadWikiFacts asks once per title or person (TMDB id); failures answer empty. */
export function loadWikiFacts(get: <T>(path: string) => Promise<T>, kind: 'movie' | 'show' | 'person', id: string | null | undefined): Promise<WikiFacts> {
	if (!id || !/^[1-9][0-9]*$/.test(id)) return Promise.resolve(EMPTY);
	const key = `${kind}:${id}`;
	let p = cache.get(key);
	if (!p) {
		p = get<unknown>(`/wiki/${kind}/${id}`).then(cleanWikiFacts, () => EMPTY);
		cache.set(key, p);
	}
	return p;
}
