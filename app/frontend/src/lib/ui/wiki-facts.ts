// Cleans and caches Wikipedia facts for titles and people.
import { to } from '../core/paths.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type WikiAward = { name: string; family: string; won: boolean; year: number; for_work: string };
export type WikiBio = { title: string; extract: string; url: string; license: string; license_url: string; attribution: string };
export type WikiFacts = { badges: string[]; awards: WikiAward[]; born?: string; died?: string; birth_place?: string; death_place?: string; bio: WikiBio | null; image?: string };

const EMPTY: WikiFacts = { badges: [], awards: [], bio: null };

export const CC_BY_SA = 'https://creativecommons.org/licenses/by-sa/4.0/';

const cache = new Map<string, Promise<WikiFacts>>();

// Only https Wikipedia links are trusted for display.
export function wikipediaURL(v: unknown): string {
	try {
		const u = new URL(str(v));
		return u.protocol === 'https:' && (u.hostname === 'wikipedia.org' || u.hostname.endsWith('.wikipedia.org')) ? u.href : '';
	} catch {
		return '';
	}
}

// Unknown or unsafe license links fall back to CC BY-SA.
export const licenseURL = (v: unknown) => (str(v).startsWith('https://creativecommons.org/') && !/[\s"<>]/.test(str(v)) ? str(v) : CC_BY_SA);

// Drops anything malformed so the UI can trust the shape.
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
	return { badges, awards, born: str(r.born) || undefined, died: str(r.died) || undefined, birth_place: str(r.birth_place) || undefined, death_place: str(r.death_place) || undefined, bio, image: personImage(str(r.image)) };
}

// Prefers major award families; max caps the lines before an "and N more".
export function awardsTip(awards: WikiAward[], max = 6): string {
	const line = (a: WikiAward) => {
		const name = a.year ? `${a.name} (${a.year})` : a.name;
		return a.won ? t('ui.wikiFacts.won', { name }) : t('ui.wikiFacts.nominated', { name });
	};
	const major = awards.filter((a) => a.family);
	const list = (major.length ? major : awards).slice(0, max).map(line);
	const more = (major.length ? major : awards).length - list.length;
	return more > 0 ? [...list, t('ui.wikiFacts.andMore', { count: more })].join('\n') : list.join('\n');
}

// Caches per request so repeated opens do not refetch; ids must be positive numbers.
export function loadWikiFacts(get: <T>(path: string) => Promise<T>, kind: 'movie' | 'show' | 'person', id: string | null | undefined, name = ''): Promise<WikiFacts> {
	if (!id || !/^[1-9][0-9]*$/.test(id)) return Promise.resolve(EMPTY);
	const q = kind === 'person' && name ? `?name=${encodeURIComponent(name.slice(0, 100))}` : '';
	const key = `${kind}:${id}${q}`;
	let p = cache.get(key);
	if (!p) {
		p = get<unknown>(`/wiki/${kind}/${id}${q}`).then(cleanWikiFacts, () => EMPTY);
		cache.set(key, p);
	}
	return p;
}

const str = (v: unknown) => (typeof v === 'string' ? v : '');

// Photos are trusted only from our own artwork route or known image hosts.
const ownPhoto = /^\/api\/v1\/artwork\/img\/person\/\d+\/\d+\.(?:webp|jpg)(?:\?v=[A-Za-z0-9_-]+)?$/;
function personImage(v: string): string | undefined {
	if (ownPhoto.test(v)) return to(v);
	return /^https:\/\/(upload\.wikimedia\.org|static\.tvmaze\.com)\//.test(v) ? v : undefined;
}
