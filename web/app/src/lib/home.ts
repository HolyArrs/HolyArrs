// Home: small pure helpers so the page stays a layout.
import { duration } from './format.ts';
import { healthSentence, looksLikeId, plainReason } from './plain.ts';
import type { DownloadRow } from './downloaders.ts';
import type { CalendarEntry, HealthCheck, TitleView } from './types.ts';
import type { Fresh } from './fresh.ts';

/**
 * greetName is the name to greet someone by, or '' when the name looks like
 * an id (preview-…, e2e-…, a long hex or number string) and not a person.
 */
export function greetName(name: string | null | undefined): string {
	return looksLikeId(name) ? '' : (name ?? '').trim();
}

/** downloadLine is the meta line under a download card: percent and time left. */
export function downloadLine(d: Pick<DownloadRow, 'progress' | 'eta' | 'state' | 'label'>): string {
	if (d.state === 'queued') return 'Waiting to start';
	if (d.state === 'stalled') return 'Stalled';
	const pct = Math.round(Math.max(0, Math.min(1, d.progress)) * 100);
	return d.eta ? `${pct}% · ${duration(d.eta)} left` : `${pct}%`;
}

/** downloadSubtitle names what a download covers: an episode range, an album or the version. */
export function downloadSubtitle(d: Pick<DownloadRow, 'covers' | 'album' | 'year' | 'version'>): string {
	if (d.covers) return d.covers;
	if (d.album) return d.album;
	return [d.year, d.version && d.version !== 'Main' ? d.version : ''].filter(Boolean).join(' · ');
}

/** isActive says whether a download is still in progress. */
export function isActive(d: Pick<DownloadRow, 'active' | 'state'>): boolean {
	return d.active || d.state === 'downloading' || d.state === 'queued' || d.state === 'stalled';
}

export interface Attention {
	key: string;
	tone: 'fail' | 'wait';
	word: string;
	text: string;
	/** Raw text for admins, shown inside a Details disclosure. */
	detail?: string;
	href: string;
	action: string;
}

/** healthFix is where a health problem is fixed. */
export function healthFix(kind: HealthCheck['kind']): string {
	switch (kind) {
		case 'library_folder':
		case 'disk_space':
			return '/settings/library-folders';
		case 'source':
			return '/settings/sources';
		case 'downloader':
			return '/settings/downloaders';
		case 'backups':
			return '/settings/backups';
		default:
			return '/settings/this-app';
	}
}

/** attention puts failed downloads, decisions and health problems in one list, worst first. */
export function attention(input: {
	failed: Pick<DownloadRow, 'id' | 'title' | 'title_id' | 'text'>[];
	decisions: { slot_id: number; title: string; reason?: string; episode?: { label: string } | null }[];
	health: HealthCheck[];
}): Attention[] {
	const out: Attention[] = [];
	for (const c of input.health.filter((h) => h.state === 'error'))
		out.push({ key: `h${c.key}`, tone: 'fail', word: 'Problem', text: healthSentence(c), detail: [c.subject, c.message].filter(Boolean).join(': ') || undefined, href: healthFix(c.kind), action: 'Fix' });
	for (const d of input.failed) {
		const r = plainReason(d.text || 'The download failed.');
		out.push({ key: `f${d.id}`, tone: 'fail', word: 'Failed', text: `${d.title}: ${r.text}`, detail: r.detail, href: '/downloads', action: 'Open Downloads' });
	}
	for (const d of input.decisions) {
		const r = plainReason(d.reason || 'HolyArrs needs you to choose.');
		out.push({ key: `d${d.slot_id}`, tone: 'wait', word: 'Your call', text: `${d.title}${d.episode ? ` ${d.episode.label}` : ''}: ${r.text}`, detail: r.detail, href: '/downloads/decide', action: 'Decide' });
	}
	for (const c of input.health.filter((h) => h.state === 'warning'))
		out.push({ key: `h${c.key}`, tone: 'wait', word: 'Warning', text: healthSentence(c), detail: [c.subject, c.message].filter(Boolean).join(': ') || undefined, href: healthFix(c.kind), action: 'Look' });
	return out;
}

// ---- Home rows: the same wide card system as Explore, from Library data ----

const DAY = 86400000;

/** arrivedAt is when the newest file of a title came in (ms), or undefined. */
export function arrivedAt(t: Pick<TitleView, 'slots'>): number | undefined {
	let best: number | undefined;
	for (const s of t.slots ?? []) {
		const at = s.file?.AddedAt ? Date.parse(s.file.AddedAt) : NaN;
		if (!Number.isNaN(at) && (best === undefined || at > best)) best = at;
	}
	return best;
}

/**
 * splitArrivals parts the newest titles into Just arrived (a file in the last
 * `days`, newest first) and Recently added (the rest, newest added first), so
 * no title is on both rows.
 */
export function splitArrivals<T extends Pick<TitleView, 'id' | 'slots'>>(titles: T[], now = Date.now(), days = 14): { arrived: T[]; earlier: T[] } {
	const cut = now - days * DAY;
	const arrived = titles.filter((t) => (arrivedAt(t) ?? 0) >= cut).sort((a, b) => (arrivedAt(b) ?? 0) - (arrivedAt(a) ?? 0) || b.id - a.id);
	const ids = new Set(arrived.map((t) => t.id));
	return { arrived, earlier: titles.filter((t) => !ids.has(t.id)).sort((a, b) => b.id - a.id) };
}

/** homeFresh is the one freshness strip for a Home card, from facts only: a new episode file this week, a new title, or a date coming up. */
export function homeFresh(t: Pick<TitleView, 'id' | 'kind' | 'slots'> | undefined, dates: Pick<CalendarEntry, 'title_id' | 'date' | 'kind' | 'episode' | 'season'>[], now = Date.now()): Fresh | null {
	if (!t) return null;
	const at = arrivedAt(t);
	if (at !== undefined && now - at <= 7 * DAY) {
		const files = (t.slots ?? []).filter((s) => s.file).length;
		if (t.kind === 'show' && files > 1) return { kind: 'new_episode', text: 'New Episode' };
		return { kind: 'recently_added', text: 'Recently Added' };
	}
	const next = dates.filter((d) => d.title_id === t.id && Date.parse(d.date) >= now - DAY).sort((a, b) => a.date.localeCompare(b.date))[0];
	if (next) {
		if (next.kind === 'episode' && next.episode === 1 && next.season) return { kind: 'season_coming', text: `Season ${next.season} Coming` };
		return { kind: 'coming', text: next.kind === 'episode' ? 'New Episode Soon' : 'Coming Soon' };
	}
	return null;
}

/** mixLine is a factual row subtitle: "2 movies and 3 shows". */
export function mixLine(cards: { kind: string }[]): string {
	const n = (k: string) => cards.filter((c) => c.kind === k).length;
	const word = (c: number, one: string, many: string) => (c ? `${c} ${c === 1 ? one : many}` : '');
	const parts = [word(n('movie'), 'movie', 'movies'), word(n('show'), 'show', 'shows'), word(n('artist'), 'artist', 'artists')].filter(Boolean);
	return parts.length > 1 ? `${parts.slice(0, -1).join(', ')} and ${parts.at(-1)}` : (parts[0] ?? '');
}

/** arrivedTitle is Just arrived's smart title: today, this week, or the last two weeks, true to the newest card. */
export function arrivedTitle(newest: number | undefined, now = Date.now()): string {
	if (newest === undefined) return 'Just arrived';
	const age = now - newest;
	if (age < DAY) return 'New today';
	if (age < 7 * DAY) return 'New this week';
	return 'Just arrived';
}

/** soonTitle is Coming soon's subtitle: what is next and when. */
export function soonLine(first: { name: string; subtitle?: string } | undefined): string {
	if (!first) return 'The next 30 days';
	return first.subtitle ? `Next up: ${first.name}, ${first.subtitle}` : `Next up: ${first.name}`;
}
