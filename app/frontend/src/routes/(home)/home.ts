// Home: small pure helpers so the page stays a layout.
import { duration } from '#lib/core/format.ts';
import { healthSentence, plainReason } from '#lib/core/plain.ts';
import type { DownloadRow } from '#lib/core/types.ts';
import { stageOf } from '#lib/downloads/index.ts';
import type { CalendarEntry, HealthCheck, TitleView } from '#lib/core/types.ts';
import type { Fresh } from '#lib/ui/fresh.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';

/** downloadLine is the meta line under a download card: percent and time left. */
export function downloadLine(d: Pick<DownloadRow, 'progress' | 'eta' | 'state' | 'label'>): string {
	if (d.state === 'queued') return t('home.waitingToStart');
	if (d.state === 'stalled') return t('home.stalled');
	const pct = Math.round(Math.max(0, Math.min(1, d.progress)) * 100);
	return d.eta ? t('home.left', { pct: pct, time: duration(d.eta) }) : `${pct}%`;
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

type StageBits = Pick<DownloadRow, 'phase' | 'state' | 'progress' | 'replaced' | 'client_hint'>;

/** homeFailures are the rows Downloads lists under Problems > Failed: a replaced failure is done, a refused import counts. */
export function homeFailures<T extends StageBits>(rows: T[]): T[] {
	return rows.filter((d) => stageOf(d) === 'failed');
}

const fetching = new Set(['checking', 'queued', 'downloading', 'stalled', 'importing']);

type CardRow = StageBits & Pick<DownloadRow, 'id' | 'active' | 'client_item' | 'outside' | 'title_id'>;

/**
 * homeCounts says whether Home counts a download as HolyArrs's: its own, its
 * own older ones that only come back as Downloader rows, and torrents linked
 * to a title in the Library.
 */
export function homeCounts(d: Pick<DownloadRow, 'client_item' | 'outside' | 'title_id'>): boolean {
	return !d.client_item || !d.outside || !!d.title_id;
}

const fetchingNow = (d: CardRow) => fetching.has(stageOf(d)) && (isActive(d) || stageOf(d) === 'importing');

/**
 * downloadingNow is Home's Downloading row: what HolyArrs is fetching now,
 * furthest along first, at most `cap`. A download being imported counts: the
 * server no longer calls it active (its state is finished), but it is not in the Library yet. Torrents HolyArrs did not send,
 * seeding, finished and failed rows stay on Downloads.
 */
export function downloadingNow<T extends CardRow>(rows: T[], cap = 20): T[] {
	return rows
		.filter((d) => homeCounts(d) && fetchingNow(d))
		.sort((a, b) => (b.progress || 0) - (a.progress || 0) || b.id - a.id)
		.slice(0, cap);
}

/** homeDownloadsURL reads every Downloader row (up to the server's 2000), so a busy Downloader or a second one is not cut off after 200. */
export const homeDownloadsURL = '/downloads/view?limit=100&outside_limit=2000';

/** notSentNow is what the Downloader is fetching that HolyArrs did not send and Home does not count. */
export function notSentNow<T extends CardRow>(rows: T[]): T[] {
	return rows.filter((d) => !homeCounts(d) && fetchingNow(d));
}

export interface Attention {
	key: string;
	tone: 'fail' | 'wait';
	/** The tone in a word, for screen readers: Failed, Problem, Your call, Warning. */
	word: string;
	/** The whole thing as one sentence. */
	text: string;
	/** What it is about: a title, or the part that has the problem. */
	title: string;
	/** Why, in a few words: "download failed". */
	reason: string;
	/** Items with the same cause merge into one row. */
	cause: string;
	/** What the Downloader said, in plain words. Shown to everyone who sees the row. */
	note?: string;
	/** Raw text for admins, shown inside a Details disclosure. */
	detail?: string;
	/** The part a health row is about (a Source's or Downloader's name), so a grouped row can name them. */
	subject?: string;
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
		case 'waiting_downloads':
			return '/downloads?show=waiting.paused';
		case 'config_folder':
			// No setting fixes it: Status shows the steps.
			return '/health';
		default:
			// Updates, trailers and anything new live on This app.
			return '/settings/this-app';
	}
}

// A health problem in a few words: what it is about (one, many) and why (one, many), by state.
// Trailers comes from the server but is not in the HealthCheck type yet, so it is named here too.
type HealthKind = HealthCheck['kind'] | 'trailers' | 'key_kit';
const healthWords = (): Record<HealthKind, { one: string; many: string; error: [string, string]; warning: [string, string] }> => ({
	library_folder: { one: t('home.libraryFolder'), many: t('home.libraryFolders'), error: [t('home.canTBeReached'), t('home.canTBeReached')], warning: [t('home.needsALook'), t('home.needALook')] },
	disk_space: { one: t('home.libraryFolder'), many: t('home.libraryFolders'), error: [t('home.outOfSpace'), t('home.outOfSpace')], warning: [t('home.runningLowOnSpace'), t('home.runningLowOnSpace')] },
	source: { one: t('home.source'), many: t('home.sources'), error: [t('home.notAnswering'), t('home.notAnswering')], warning: [t('home.hasAProblem'), t('home.haveAProblem')] },
	downloader: { one: t('home.downloader'), many: t('home.downloaders'), error: [t('home.notAnswering'), t('home.notAnswering')], warning: [t('home.hasAProblem'), t('home.haveAProblem')] },
	backups: { one: t('home.backups'), many: t('home.backups'), error: [t('home.notBeingMade'), t('home.notBeingMade')], warning: [t('home.gettingOld'), t('home.gettingOld')] },
	update: { one: t('home.holyarrsUpdate'), many: t('home.holyarrsUpdates'), error: [t('home.checkFailed'), t('home.checkFailed')], warning: [t('home.waiting'), t('home.waiting')] },
	config_disk: { one: t('home.configFolder'), many: t('home.configFolder'), error: [t('home.almostOutOfSpace'), t('home.almostOutOfSpace')], warning: [t('home.runningLowOnSpace'), t('home.runningLowOnSpace')] },
	config_folder: { one: t('home.configFolder'), many: t('home.configFolder'), error: [t('home.lostIfTheContainer'), t('home.lostIfTheContainer')], warning: [t('home.needsALook'), t('home.needsALook')] },
	waiting_downloads: { one: t('home.downloads'), many: t('home.downloads'), error: [t('home.waitingForTheirFiles'), t('home.waitingForTheirFiles')], warning: [t('home.waitingForTheirFiles'), t('home.waitingForTheirFiles')] },
	// The backups:key check: no key kit saved for the current master key.
	key_kit: { one: t('home.masterKey'), many: t('home.masterKey'), error: [t('home.noKeyKitSaved'), t('home.noKeyKitSaved')], warning: [t('home.noKeyKitSaved'), t('home.noKeyKitSaved')] },
	trailers: { one: t('home.trailers'), many: t('home.trailers'), error: [t('home.notLoading'), t('home.notLoading')], warning: [t('home.lookupsFailing'), t('home.lookupsFailing')] }
});

/**
 * shownState is the state Home shows for a check: the one the flap guard
 * confirmed (what Alerts were told) when the server sends it, else the raw one,
 * so a single missed run does not flash a problem.
 */
export function shownState(c: Pick<HealthCheck, 'state'> & { confirmed?: string }): HealthCheck['state'] {
	const s = c.confirmed || c.state;
	return s === 'error' || s === 'warning' ? s : 'ok';
}

// A Source or Downloader goes by its name ("SABnzbd not answering"); a folder's path comes last, so a long one is what gets cut ("out of space: /data/media").
const NAMED = new Set<string>(['source', 'downloader']);
const PLACED = new Set<string>(['library_folder', 'disk_space']);

function healthItem(c: HealthCheck, tone: 'fail' | 'wait', word: string, action: string, admin: boolean): Attention {
	const kind = c.key === 'backups:key' ? 'key_kit' : c.kind;
	const w = healthWords()[kind];
	const level = tone === 'fail' ? 'error' : 'warning';
	const href = admin ? healthFix(c.kind) : '/health';
	return {
		key: `h${c.key}`,
		tone,
		word,
		// The shown state, so the sentence matches the title and reason while the flap guard holds it.
		text: healthSentence({ ...c, state: level }),
		title: w?.one ?? 'HolyArrs',
		// A Source, Downloader or folder row says which one it is.
		reason: w ? (c.subject && NAMED.has(c.kind) ? `${c.subject} ${w[level][0]}` : c.subject && PLACED.has(c.kind) ? `${w[level][0]}: ${c.subject}` : w[level][0]) : level === 'error' ? t('home.notWorking') : t('home.needsALook'),
		cause: `health:${kind}:${level}`,
		detail: [c.subject, c.message].filter(Boolean).join(': ') || undefined,
		subject: NAMED.has(c.kind) ? c.subject || undefined : undefined,
		// Settings is only for people who may change it; everyone else sees the check on Status.
		href,
		action: href === '/health' ? t('home.look') : href.startsWith('/downloads') ? t('home.downloads') : action
	};
}

/**
 * attention puts failed downloads, decisions and health problems in one list, worst first.
 * `metadata` is the note while the metadata server is down; `admin` says the
 * viewer may open Settings (yes when left out).
 */
export function attention(input: {
	failed: (Pick<DownloadRow, 'id' | 'title' | 'title_id' | 'text'> & { name?: string; phase?: string; client_item?: boolean; downloader_id?: number; hash?: string })[];
	decisions: { slot_id: number; title: string; reason?: string; episode?: { label: string } | null }[];
	health: HealthCheck[];
	metadata?: string;
	admin?: boolean;
}): Attention[] {
	const out: Attention[] = [];
	const admin = input.admin ?? true;
	for (const c of input.health.filter((h) => shownState(h) === 'error')) out.push(healthItem(c, 'fail', t('home.problem'), t('home.fix'), admin));
	for (const raw of input.failed) {
		// A download with no matched title (an outside torrent, one that vanished) goes by its own name.
		const d = { ...raw, title: raw.title || raw.name || t('home.aDownload') };
		const refused = d.phase === 'import_refused';
		const r = plainReason(d.text || (refused ? t('home.theImportWasRefused') : t('home.theDownloadFailed')));
		// An outside torrent has no HolyArrs id (0): it is keyed by its Downloader and hash, and opens the Failed list.
		const own = !d.client_item && d.id > 0;
		out.push({ key: own ? `f${d.id}` : `fc${d.downloader_id ?? 0}:${d.hash || d.name || out.length}`, tone: 'fail', word: t('home.failed'), text: `${d.title}: ${r.text}`, title: d.title, reason: refused ? t('home.importRefused') : t('home.downloadFailed'), cause: 'download_failed', note: d.text ? r.text : undefined, detail: r.detail, href: own ? `/downloads?show=problems.failed&open=${d.id}` : '/downloads?show=problems.failed', action: t('home.downloads') });
	}
	// The metadata outage stays while it lasts and goes away on the next check that passes.
	if (input.metadata) out.push({ key: 'metadata', tone: 'wait', word: t('home.warning'), text: t('home.holyarrsMetadataCanT', { note: input.metadata }), title: t('home.holyarrsMetadata'), reason: t('home.canTBeReached'), cause: 'metadata', note: input.metadata, href: admin ? '/settings/tmdb' : '/health', action: t('home.look') });
	for (const d of input.decisions) {
		const r = plainReason(d.reason || t('home.holyarrsNeedsYouTo'));
		const title = `${d.title}${d.episode ? ` ${d.episode.label}` : ''}`;
		out.push({ key: `d${d.slot_id}`, tone: 'wait', word: t('home.yourCall'), text: `${title}: ${r.text}`, title, reason: t('home.needsADecision'), cause: 'decision', note: d.reason ? r.text : undefined, detail: r.detail, href: `/downloads?show=problems.decide&slot=${d.slot_id}`, action: t('home.decide') });
	}
	for (const c of input.health.filter((h) => shownState(h) === 'warning')) out.push(healthItem(c, 'wait', t('home.warning'), t('home.look'), admin));
	return out;
}

/** uncheckedLine says what Home could not check when a read failed. */
export function uncheckedLine(downloads: boolean, health: boolean): string {
	const what = downloads && health ? t('home.downloadsOrHealth') : downloads ? t('home.downloads2') : t('home.health');
	return t('home.homeCouldNotCheck', { what: what });
}

/** One row of Needs attention: a single item, or several that share a cause. */
export interface AttentionGroup {
	key: string;
	tone: Attention['tone'];
	word: string;
	/** The row's lead: a title, or "2 downloads failed". */
	title: string;
	/** After the lead: why, or (for several) which ones. */
	reason: string;
	href: string;
	action: string;
	items: Attention[];
}

/**
 * groupAttention merges items that share a cause ("2 downloads failed") into
 * one row that lists its titles; a single item stays a row of its own.
 * Rows keep the order of their first item, so the worst still comes first.
 */
export function groupAttention(items: Attention[]): AttentionGroup[] {
	const byCause = new Map<string, Attention[]>();
	for (const i of items) byCause.set(i.cause, [...(byCause.get(i.cause) ?? []), i]);
	const out: AttentionGroup[] = [];
	for (const [cause, list] of byCause) {
		const first = list[0];
		const n = list.length;
		// Several downloads together link to their tab on Downloads, not to the first one's sheet.
		const href = n > 1 ? first.href.replace(/&(?:open|slot)=-?\d+$/, '') : first.href;
		const base = { tone: first.tone, word: first.word, href, action: first.action, items: list };
		if (n === 1) {
			out.push({ ...base, key: first.key, title: first.title, reason: first.reason });
			continue;
		}
		const kind = cause.startsWith('health:') ? (cause.split(':')[1] as HealthKind) : undefined;
		const w = kind ? healthWords()[kind] : undefined;
		const level = cause.endsWith(':error') ? 'error' : 'warning';
		const names = list.map((i) => i.subject).filter(Boolean);
		if (w) out.push({ ...base, key: `g${cause}`, title: `${n} ${w.many}`, reason: names.length === n ? `${names.join(', ')} ${w[level][1]}` : w[level][1] });
		else if (cause === 'decision') out.push({ ...base, key: `g${cause}`, title: t('home.needADecision', { n: n }), reason: list.map((i) => i.title).join(', ') });
		else out.push({ ...base, key: `g${cause}`, title: t('home.downloadsFailed', { n: n }), reason: list.map((i) => i.title).join(', ') });
	}
	return out;
}

/**
 * moreLink is the link under Needs attention for the rows that do not fit:
 * how many rows, and the page that holds most of them (Health or Downloads).
 */
export function moreLink(hidden: Pick<AttentionGroup, 'items'>[], admin = true): { href: string; text: string } | null {
	if (!hidden.length) return null;
	const health = hidden.filter((g) => /^(health:|setup:|metadata$)/.test(g.items[0]?.cause ?? '')).length;
	if (health * 2 > hidden.length) return admin ? { href: '/settings/health', text: t('home.moreOnHealth', { n: hidden.length }) } : { href: '/health', text: t('home.moreOnStatus', { n: hidden.length }) };
	return { href: '/downloads', text: t('home.moreInDownloads', { n: hidden.length }) };
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
export function homeFresh(title: Pick<TitleView, 'id' | 'kind' | 'slots'> | undefined, dates: Pick<CalendarEntry, 'title_id' | 'date' | 'kind' | 'episode' | 'season'>[], now = Date.now()): Fresh | null {
	if (!title) return null;
	const at = arrivedAt(title);
	if (at !== undefined && now - at <= 7 * DAY) {
		const files = (title.slots ?? []).filter((s) => s.file).length;
		if (title.kind === 'show' && files > 1) return { kind: 'new_episode', text: t('home.newEpisode') };
		return { kind: 'recently_added', text: t('home.recentlyAdded') };
	}
	const next = dates.filter((d) => d.title_id === title.id && Date.parse(d.date) >= now - DAY).sort((a, b) => a.date.localeCompare(b.date))[0];
	if (next) {
		if (next.kind === 'episode' && next.episode === 1 && next.season) return { kind: 'season_coming', text: t('home.seasonComing', { n: next.season }) };
		return { kind: 'coming', text: next.kind === 'episode' ? t('home.newEpisodeSoon') : t('home.comingSoon') };
	}
	return null;
}

/** mixLine is a factual row subtitle: "2 movies and 3 shows". */
export function mixLine(cards: { kind: string }[]): string {
	const n = (k: string) => cards.filter((c) => c.kind === k).length;
	const word = (c: number, one: string, many: string) => (c ? t(c === 1 ? one : many, { n: c }) : '');
	const parts = [word(n('movie'), 'home.movie', 'home.movies'), word(n('show'), 'home.show', 'home.shows'), word(n('artist'), 'home.artist', 'home.artists')].filter(Boolean);
	return parts.length > 1 ? t('home.and', { list: parts.slice(0, -1).join(', '), last: parts.at(-1) ?? '' }) : (parts[0] ?? '');
}

/** arrivedTitle is Just arrived's smart title: today, this week, the last two weeks, or Recently added, true to the newest card. */
export function arrivedTitle(newest: number | undefined, now = Date.now()): string {
	if (newest === undefined) return t('home.justArrived');
	const age = now - newest;
	if (age < DAY) return t('home.newToday');
	if (age < 7 * DAY) return t('home.newThisWeek');
	if (age < 14 * DAY) return t('home.justArrived');
	return t('home.recentlyAdded2');
}

/** arrivedHead is Just arrived's title and subtitle from its newest card; the two-week line only while it holds. */
export function arrivedHead(cards: Pick<HomeCard, 'at'>[], now = Date.now()): { title: string; subtitle?: string } {
	const newest = Math.max(...cards.map((c) => (c.at ? Date.parse(c.at) : NaN)).filter((t) => !Number.isNaN(t)));
	if (!Number.isFinite(newest)) return { title: t('home.justArrived') };
	return { title: arrivedTitle(newest, now), subtitle: now - newest < 14 * DAY ? t('home.newInTheLast') : undefined };
}

/** soonTitle is Coming soon's subtitle: what is next and when. */
export function soonLine(first: { name: string; subtitle?: string } | undefined): string {
	if (!first) return t('home.theNext30Days');
	return first.subtitle ? t('home.nextUp', { name: first.name, subtitle: first.subtitle }) : t('home.nextUp2', { name: first.name });
}

// ---- GET /home: the server's row order and the rows it fills ----

/** HomeRow is one Home row, in the order the server sends. */
export type HomeRow = 'welcome' | 'attention' | 'arrived' | 'approval' | 'asks' | 'new_season' | 'following' | 'soon' | 'discover';

export interface HomeCard {
	key: string;
	kind: 'movie' | 'show' | 'artist';
	title_id: number;
	name: string;
	year?: number;
	/** What is new: "S5 · 3 new episodes", "Season 5 added", "New show", "3 new albums". */
	line?: string;
	href: string;
	backdrop?: string;
	poster?: string;
	logo?: string;
	season?: number;
	albums?: { id: number; name: string; cover: string }[];
	at?: string;
}

export interface AskProgress {
	progress: number;
	state: 'queued' | 'downloading' | 'stalled' | 'paused';
	eta?: number;
}

export interface HomeAnswer {
	rows: HomeRow[];
	owner: boolean;
	kid: boolean;
	arrived: HomeCard[];
	new_seasons: HomeCard[];
	following: HomeCard[];
	soon: HomeCard[];
	missing_count: number;
	ask_progress: Record<string, AskProgress>;
	/** What the Library holds per kind that is on (GET /home library). */
	library?: Partial<Record<LibraryKind, LibraryCount>>;
}

export type LibraryKind = 'movie' | 'show' | 'artist';

/** LibraryCount is one kind's share of the Library: titles, seasons or albums, items, and the main copy's state. */
export interface LibraryCount {
	titles: number;
	parts: number;
	items: number;
	on_disk: number;
	missing: number;
	downloading: number;
	size: number;
}

/** LibrarySegment is one part of a card's bar: its share of the items, and where it leads. */
export interface LibrarySegment {
	key: 'on_disk' | 'downloading' | 'missing';
	label: string;
	count: number;
	share: number;
	href?: string;
}

/** LibraryCardView is one Your Library card on Home. */
export interface LibraryCardView {
	kind: LibraryKind;
	label: string;
	icon: 'film' | 'tv' | 'music';
	href: string;
	big: number;
	/** The big number's word: "movies", "shows", "artists". */
	unit: string;
	/** Seasons and episodes, or albums and tracks; empty for movies. */
	detail: string;
	segments: LibrarySegment[];
	size: number;
}

/** n formats a count with thousands separators. */
export function n(v: number): string {
	return v.toLocaleString(appLocale('en'));
}

function plural(v: number, one: string, many: string): string {
	return t(v === 1 ? one : many, { n: n(v) });
}

/**
 * libraryCards turns the Home answer's counts into the Your Library cards,
 * in sidebar order (Movies, TV shows, Music), for the kinds that are on.
 * Each bar is on disk, downloading and missing as shares of all items;
 * the rest (not out yet, not kept up to date) stays the bar's track.
 * Downloading links to `downloadsHref` for people who may open Downloads.
 */
export function libraryCards(lib: HomeAnswer['library'], downloadsHref?: string): LibraryCardView[] {
	if (!lib) return [];
	const meta: [LibraryKind, string, LibraryCardView['icon'], string, string][] = [
		['movie', t('home.movies2'), 'film', t('home.movie2'), t('home.movies3')],
		['show', t('home.tvShows'), 'tv', t('home.show2'), t('home.shows2')],
		['artist', t('home.music'), 'music', t('home.artist2'), t('home.artists2')]
	];
	const out: LibraryCardView[] = [];
	for (const [kind, label, icon, one, many] of meta) {
		const c = lib[kind];
		if (!c) continue;
		const base = `/library?kind=${kind}`;
		const all = Math.max(c.items, c.on_disk + c.downloading + c.missing, 1);
		const seg = (key: LibrarySegment['key'], text: string, v: number, href?: string): LibrarySegment => ({ key, label: text, count: v, share: v / all, href });
		const detail = kind === 'show' ? `${plural(c.parts, 'home.season', 'home.seasons')} · ${plural(c.items, 'home.episode', 'home.episodes')}` : kind === 'artist' ? `${plural(c.parts, 'home.album', 'home.albums')} · ${plural(c.items, 'home.track', 'home.tracks')}` : '';
		out.push({
			kind,
			label,
			icon,
			href: base,
			big: c.titles,
			unit: c.titles === 1 ? one : many,
			detail,
			segments: [seg('on_disk', t('home.onDisk'), c.on_disk, `${base}&show=have`), seg('downloading', t('home.downloading'), c.downloading, downloadsHref), seg('missing', t('home.missing'), c.missing, `${base}&show=missing`)],
			size: c.size
		});
	}
	return out;
}

/** fallbackRows is the order used while GET /home is unreachable: only the rows the page can fill itself. */
export function fallbackRows(owner: boolean): HomeRow[] {
	return owner ? ['attention', 'approval', 'asks', 'soon', 'discover'] : ['asks', 'soon', 'discover'];
}

/** homeStrip is the card's one strip saying what is new: the show's line, or the artist and their new albums. */
export function homeStrip(c: Pick<HomeCard, 'kind' | 'name' | 'line'>): Fresh | null {
	// The server's plain "Show" and "Music" lines say nothing is new: no strip.
	if (!c.line || c.line === 'Show' || c.line === 'Music') return null;
	if (c.kind === 'show') return { kind: c.line.startsWith('Season') && !c.line.includes('·') ? 'new_season' : 'new_episode', text: serverWords(c.line) };
	if (c.kind === 'artist') return { kind: 'recently_added', text: `${c.name} · ${serverWords(c.line)}` };
	return null;
}

/** askProgressLine is the live line under an ask that is downloading. */
export function askProgressLine(p: AskProgress | undefined): string {
	if (!p) return '';
	if (p.state === 'queued') return t('home.waitingToStart');
	const pct = Math.round(Math.max(0, Math.min(1, p.progress)) * 100);
	if (p.state === 'stalled') return t('home.stalledAt', { pct: pct });
	if (p.state === 'paused') return t('home.pausedAt', { pct: pct });
	return p.eta ? t('home.downloadingLeft', { pct: pct, time: duration(p.eta) }) : t('home.downloading2', { pct: pct });
}

/**
 * downloadMoved says a live download event changes what Home shows: a new download or a new state.
 * Progress ticks alone do not, so Home is not re-read every few seconds while things download.
 */
export function downloadMoved(seen: Map<number, string>, x: unknown): boolean {
	const d = x as { ID?: number; id?: number; State?: string; state?: string } | null;
	const id = d?.ID ?? d?.id;
	const state = d?.State ?? d?.state;
	if (!id || !state) return true;
	if (seen.get(id) === state) return false;
	seen.set(id, state);
	return true;
}

// ---- Home's System grid ----

/**
 * partCount sums up Sources or Downloaders from GET /health. Only "disabled" is turned off; a part that is on
 * works when its state says so (enabled, ok). A failing Source is paused by the server and a failing Downloader is
 * unreachable or auth_failed, and each has a failing health check: `failing` is the names those checks are about
 * (or just how many), so a part whose state still looks fine is not counted as working. A removed Downloader (kept for its history
 * under "<name> (removed <id>)") is left out.
 */
export function partCount(parts: { name?: string; state: string }[] | null | undefined, failing: number | string[] = 0): { total: number; on: number; working: number; off: number; failing: number } {
	const list = (parts ?? []).filter((p) => !/ \(removed \d+\)$/.test(p.name ?? ''));
	const off = list.filter((p) => p.state === 'disabled').length;
	const on = list.length - off;
	const good = list.filter((p) => p.state === 'ok' || p.state === 'working' || p.state === 'enabled');
	let bad: number;
	if (typeof failing === 'number') bad = Math.max(0, failing - (on - good.length));
	else {
		// By name: a check for a part that is off (the flap guard holds it a little) or already counted as failing changes nothing;
		// a check that names no part we know still counts, so the tile never says more works than the rows do.
		const known = new Set(list.map((p) => p.name));
		bad = good.filter((p) => p.name && failing.includes(p.name)).length + failing.filter((name) => !known.has(name)).length;
	}
	const working = Math.max(0, good.length - bad);
	return { total: list.length, on, working, off, failing: on - working };
}

/**
 * freeSpace is the free room left for the Library. Folders on one disk are read a moment apart, so their numbers
 * differ (a download can land in between): readings within 5% (or 4 GB) of each other are the same disk. With several disks it is the
 * fullest one. A reading of 0 means "not known" unless `zeroIsFull` (the disk space check is failing): then it is a full disk.
 */
export function freeSpace(folders: { free_space: number }[], zeroIsFull = false): { free: number; many: boolean } | null {
	const sizes = folders.map((f) => f.free_space).filter((s) => s > 0 || (zeroIsFull && s === 0)).sort((x, y) => x - y);
	if (!sizes.length) return null;
	let disks = 1;
	for (let i = 1; i < sizes.length; i++) if (sizes[i] - sizes[i - 1] > Math.max(sizes[i] * 0.05, 4 * 1024 ** 3)) disks++;
	return { free: sizes[0], many: disks > 1 };
}
