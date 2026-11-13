// Downloader helpers, kept apart from the pages so they can be tested.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { DownloadRow, Decision } from '../core/types.ts';
import type { Tone } from '../core/format.ts';

export interface DownloaderHealth {
	state: 'working' | 'paused' | 'resting' | 'not_answering' | 'sign_in_refused';
	label: string;
	text: string;
}

export interface PathMap {
	theirs: string;
	ours: string;
}

// samePathPair says both sides of a path pair name the same folder (a
// trailing slash does not count; on Windows-style paths neither does case).
// Such a pair renames nothing, but it is still kept: its HolyArrs side tells
// HolyArrs where the Downloader's files are.
export function samePathPair(p: PathMap): boolean {
	const norm = (x: string) => {
		x = x.trim();
		if (x.includes('\\') || /^[a-zA-Z]:/.test(x)) x = x.replaceAll('\\', '/').toLowerCase();
		return x.replace(/\/+/g, '/').replace(/(.)\/$/, '$1');
	};
	const a = norm(p.theirs);
	return a !== '' && a === norm(p.ours);
}

export interface DownloaderView {
	id: number;
	name: string;
	kind: 'qbittorrent' | 'sabnzbd' | 'transmission' | 'nzbget' | 'deluge' | 'rtorrent' | 'aria2';
	protocol: 'torrent' | 'usenet';
	url: string;
	username: string;
	secret: string;
	category: string;
	/** Category for one kind of title; empty uses category. */
	category_movies?: string;
	category_shows?: string;
	category_music?: string;
	priority: number;
	path_map: PathMap[];
	allow_local_network: boolean;
	/** True when HolyArrs can change this Downloader; false means it only reads. */
	writable: boolean;
	/** HOLYARRS_READONLY_DOWNLOADERS keeps it read only whatever the switch says. */
	writable_locked: boolean;
	/** Titles with one of these go only to Downloaders that have it. */
	tags?: string[];
	/** Remove after seeding (torrents only); ratio and minutes goals only on qBittorrent and rTorrent. */
	after_seeding?: SeedingChoice;
	/** qBittorrent only: a torrent moves to this category once imported; empty keeps it. */
	category_after_import?: string;
	/** Shows and changes only items in this Downloader's own categories. */
	own_only?: boolean;
	health: DownloaderHealth;
	checked_at?: string;
	active_downloads: number;
}

export interface SeedingChoice {
	remove: boolean;
	ratio: number | null;
	minutes: number | null;
	delete_files: boolean;
}

/** Downloaders that report ratio and seeding time reliably enough for a goal. */
export const seedGoalKinds: DownloaderView['kind'][] = ['qbittorrent', 'rtorrent'];

export interface DownloaderSettings {
	stalled_after_hours: number;
	stalled_act_hours: number;
	remove_failed: boolean;
	fail_dangerous: boolean;
	replace_vanished: boolean;
	grab_spacing_seconds: number;
	try_next_downloader: boolean;
	remove_delete_files: boolean;
	remove_turn_down: boolean;
	remove_search_again: boolean;
	/** Add the movie or show of a matched outside item to the Library, without a search. Off by default. */
	downloads_auto_add?: boolean;
	/** Show Usenet jobs in other apps' categories on Downloads, read-only; never matched. On by default. */
	downloads_sab_outside?: boolean;
	/** Dead torrent rule, 0 for off: stuck finding files for N minutes (5 to 1440). */
	dead_metadata_minutes?: number;
	/** Crawl rule, 0 for off: slower than N KB/s for H hours. */
	crawl_kbps?: number;
	crawl_hours?: number;
}

/** The public recovery record for one uncertain send. It has no Downloader address, request marker or error text on purpose. */
export interface DownloaderAttempt {
	id: number;
	downloader_id: number;
	release_id: number;
	slot_id: number;
	state: 'pending' | 'resolved' | 'abandoned';
	created_at: string;
	updated_at: string;
}

export interface DownloaderForm {
	name: string;
	kind: DownloaderView['kind'];
	url: string;
	username: string;
	secret: string;
	category: string;
	category_movies: string;
	category_shows: string;
	category_music: string;
	priority: number;
	path_map: PathMap[];
	allow_local_network: boolean;
	writable: boolean;
	tags: string[];
	seed_remove: boolean;
	/** Empty means no ratio goal. */
	seed_ratio: number | '' | null;
	/** Seeding time goal in hours; empty means none. */
	seed_hours: number | '' | null;
	seed_delete_files: boolean;
	category_after_import: string;
	own_only: boolean;
}

export interface RemoveChoice {
	delete_files: boolean;
	turn_down: boolean;
	search_again: boolean;
}

/** DecisionGroup is one card on Needs a decision: a movie, an episode, or
 *  the stuck episodes of one season pack together. */
export interface DecisionGroup {
	key: string;
	lead: Decision;
	items: Decision[];
	pack: boolean;
}

/** Only sends that still need the owner's decision. */
export function pendingAttempts(attempts: DownloaderAttempt[] | undefined): DownloaderAttempt[] {
	return (attempts ?? []).filter((attempt) => attempt.state === 'pending');
}

export { testVersion } from '../core/test-tag.ts';

export const kindWords: Record<DownloaderView['kind'], string> = {
	qbittorrent: 'qBittorrent',
	sabnzbd: 'SABnzbd',
	transmission: 'Transmission',
	nzbget: 'NZBGet',
	deluge: 'Deluge',
	rtorrent: 'rTorrent',
	aria2: 'aria2'
};

/** Colours a Downloader's health chip. */
export function healthTone(state: DownloaderHealth['state']): Tone {
	if (state === 'working') return 'ok';
	if (state === 'paused') return 'idle';
	if (state === 'resting') return 'wait';
	return 'fail';
}

/** Colours a Download's state chip. */
export function downloadTone(state: string): Tone {
	if (state === 'finished' || state === 'seeding') return 'ok';
	if (state === 'failed' || state === 'stalled' || state === 'vanished') return 'fail';
	if (state === 'cleaned') return 'idle';
	return 'wait';
}

/** Progress as a whole percent. */
export function percent(p: number): string {
	return `${Math.floor(Math.min(Math.max(p, 0), 1) * 100)}%`;
}

/** Seconds left, in words. */
export function timeLeft(s?: number): string {
	if (!s || s <= 0) return '';
	if (s < 60) return t('downloads.left.under');
	if (s < 3600) return t('downloads.left.min', { m: Math.round(s / 60) });
	const h = Math.floor(s / 3600);
	const m = Math.round((s % 3600) / 60);
	return m ? t('downloads.left.hm', { h, m }) : t('downloads.left.h', { h });
}

/** Starts a new Downloader. Downloaders usually run at home, so the local-network switch starts on. */
export function emptyForm(kind: DownloaderView['kind'] = 'qbittorrent'): DownloaderForm {
	return {
		name: kindWords[kind],
		kind,
		url: kind === 'sabnzbd' ? 'http://' : 'http://',
		username: '',
		secret: '',
		category: kind === 'sabnzbd' ? 'movies' : 'holyarrs',
		category_movies: '',
		category_shows: '',
		category_music: '',
		priority: 1,
		path_map: [],
		allow_local_network: true,
		writable: true,
		tags: [],
		seed_remove: false,
		seed_ratio: '',
		seed_hours: '',
		seed_delete_files: false,
		category_after_import: '',
		own_only: false
	};
}

/** Fills the form from a saved Downloader; the secret stays masked. */
export function formFrom(d: DownloaderView): DownloaderForm {
	return {
		name: d.name,
		kind: d.kind,
		url: d.url,
		username: d.username,
		secret: d.secret,
		category: d.category,
		category_movies: d.category_movies ?? '',
		category_shows: d.category_shows ?? '',
		category_music: d.category_music ?? '',
		priority: d.priority,
		path_map: d.path_map.map((p) => ({ ...p })),
		allow_local_network: d.allow_local_network,
		writable: d.writable,
		tags: [...(d.tags ?? [])],
		seed_remove: !!d.after_seeding?.remove,
		seed_ratio: d.after_seeding?.ratio ?? '',
		seed_hours: d.after_seeding?.minutes ? Math.round((d.after_seeding.minutes / 60) * 100) / 100 : '',
		seed_delete_files: !!d.after_seeding?.delete_files,
		category_after_import: d.category_after_import ?? '',
		own_only: !!d.own_only
	};
}

/** Turns the form into the API body; empty path map rows are dropped. */
export function formBody(f: DownloaderForm): Record<string, unknown> {
	return {
		name: f.name.trim(),
		kind: f.kind,
		url: f.url.trim(),
		username: f.kind === 'qbittorrent' || f.kind === 'transmission' || f.kind === 'nzbget' || f.kind === 'rtorrent' ? f.username.trim() : '',
		secret: f.secret,
		category: f.category.trim(),
		category_movies: f.category_movies.trim(),
		category_shows: f.category_shows.trim(),
		category_music: f.category_music.trim(),
		priority: Number(f.priority) || 1,
		path_map: f.path_map.map((p) => ({ theirs: p.theirs.trim(), ours: p.ours.trim() })).filter((p) => p.theirs || p.ours),
		allow_local_network: f.allow_local_network,
		writable: f.writable,
		tags: f.tags,
		...(f.kind === 'qbittorrent' ? { category_after_import: f.category_after_import.trim() } : {}),
		...(f.kind === 'sabnzbd' || f.kind === 'nzbget' ? {} : { own_only: f.own_only }),
		...(seedingFields(f.kind) === 'none' ? {} : { after_seeding: seedingBody(f) })
	};
}

function goal(v: number | '' | null): number | null {
	const n = Number(v);
	return v === '' || v === null || !Number.isFinite(n) || n <= 0 ? null : n;
}

/** Which After seeding fields a Downloader gets: none for Usenet, goals only where ratio and time are reported. */
export function seedingFields(kind: DownloaderView['kind']): 'none' | 'remove' | 'goals' {
	if (kind === 'sabnzbd' || kind === 'nzbget') return 'none';
	return seedGoalKinds.includes(kind) ? 'goals' : 'remove';
}

/** The dead torrent rule counts as on while either check has a number. */
export function deadRuleOn(s: Pick<DownloaderSettings, 'dead_metadata_minutes' | 'crawl_kbps'>): boolean {
	return !!Number(s.dead_metadata_minutes) || !!Number(s.crawl_kbps);
}

/** The after_seeding body: goals only where the Downloader can report them. */
export function seedingBody(f: DownloaderForm): SeedingChoice {
	const goals = seedGoalKinds.includes(f.kind);
	const hours = goals ? goal(f.seed_hours) : null;
	return {
		remove: f.seed_remove,
		ratio: goals ? goal(f.seed_ratio) : null,
		minutes: hours === null ? null : Math.max(1, Math.round(hours * 60)),
		delete_files: f.seed_remove && f.seed_delete_files
	};
}

/** API path of a row's client controls: a HolyArrs download by id, a client item by Downloader and hash. */
export function rowPath(r: Pick<DownloadRow, 'id' | 'client_item' | 'downloader_id' | 'hash'>): string {
	return r.client_item && r.hash ? `/downloaders/${r.downloader_id}/items/${r.hash}` : `/downloads/${r.id}`;
}

/** A row's library type; a client item HolyArrs does not know has none. */
export function kindOf(r: Pick<DownloadRow, 'kind' | 'client_item'>): 'movie' | 'show' | 'artist' | '' {
	return r.kind || (r.client_item ? '' : 'movie');
}

/** Whether a download fills many Slots of one Title (a show's episodes or an album's tracks), and plain words for them. */
export function manyWords(kind: string | undefined): { one: string; many: string; pack: string } | null {
	if (kind === 'show') return { get one() { return t('downloads.many.episode'); }, get many() { return t('downloads.many.episodes'); }, get pack() { return t('downloads.many.seasonPack'); } };
	if (kind === 'artist') return { get one() { return t('downloads.many.track'); }, get many() { return t('downloads.many.tracks'); }, get pack() { return t('downloads.many.album'); } };
	return null;
}

/** rowName names a Downloads row: the movie with its year, the show, or
 *  artist and album joined by an en dash, then the year in brackets. */
export function rowName(r: Pick<DownloadRow, 'title' | 'year' | 'name' | 'kind' | 'album'>): string {
	if (!r.title) return r.name;
	if (r.kind === 'show') return r.title;
	if (r.kind === 'artist') return r.album ? `${r.title} – ${r.album}${r.year ? ` (${r.year})` : ''}` : r.title;
	return `${r.title}${r.year ? ` (${r.year})` : ''}`;
}

/** phaseTone colours a Downloads screen row by its place. */
export function phaseTone(phase: string): Tone {
	switch (phase) {
		case 'done':
			return 'ok';
		case 'failed':
		case 'stalled':
		case 'vanished':
		case 'needs_a_decision':
		case 'import_refused':
			return 'fail';
		case 'removed':
		case 'paused':
			return 'idle';
	}
	return 'wait';
}

/** nowPhases are the phases on the Downloads page's "Now" list; the sidebar badge counts the same set. */
export const nowPhases: ReadonlySet<string> = new Set(['waiting', 'downloading', 'stalled', 'paused', 'waiting_to_import', 'importing']);

/** split puts rows still in motion first, the finished ones after. */
export function split(rows: DownloadRow[]): { now: DownloadRow[]; past: DownloadRow[] } {
	return { now: rows.filter((r) => nowPhases.has(r.phase)), past: rows.filter((r) => !nowPhases.has(r.phase)) };
}

/** removeStart is where the Remove dialog starts, from the settings. */
export function removeStart(s: Pick<DownloaderSettings, 'remove_delete_files' | 'remove_turn_down' | 'remove_search_again'> | null): RemoveChoice {
	if (!s) return { delete_files: true, turn_down: false, search_again: true };
	return { delete_files: s.remove_delete_files, turn_down: s.remove_turn_down, search_again: s.remove_search_again };
}

/** removeBody is what is sent; search again counts only with Turned down. */
export function removeBody(c: RemoveChoice): RemoveChoice {
	return { delete_files: c.delete_files, turn_down: c.turn_down, search_again: c.turn_down && c.search_again };
}

/** decisionName names what waits: the movie (and year) or the show and episode. */
export function decisionName(d: Pick<Decision, 'title' | 'year' | 'episode' | 'album'>): string {
	if (d.episode && d.album) return `${d.title} – ${d.album} ${d.episode.label}${d.episode.name ? ` ${d.episode.name}` : ''}`;
	if (d.episode) return `${d.title} ${d.episode.label}${d.episode.name ? ` ${d.episode.name}` : ''}`;
	return `${d.title}${d.year ? ` (${d.year})` : ''}`;
}

/** groupDecisions puts the stuck episodes of one pack on one card. */
export function groupDecisions(items: Decision[]): DecisionGroup[] {
	const out: DecisionGroup[] = [];
	const byPack = new Map<number, DecisionGroup>();
	for (const d of items) {
		const packed = manyWords(d.kind) != null && d.download_id != null && (d.pack_slot_ids?.length ?? 0) > 1;
		if (!packed) {
			out.push({ key: `slot-${d.slot_id}`, lead: d, items: [d], pack: false });
			continue;
		}
		const g = byPack.get(d.download_id!);
		if (g) {
			g.items.push(d);
			continue;
		}
		const ng = { key: `pack-${d.download_id}`, lead: d, items: [d], pack: true };
		byPack.set(d.download_id!, ng);
		out.push(ng);
	}
	for (const g of out) g.items.sort((a, b) => (a.episode?.season ?? 0) - (b.episode?.season ?? 0) || (a.episode?.episode ?? 0) - (b.episode?.episode ?? 0));
	return out;
}

/** whyWords says in one line why a movie or an episode waits for a person. */
export function whyWords(d: Pick<Decision, 'why' | 'has_copy'>): string {
	switch (d.why) {
		case 'conflict':
			return t('downloads.why.conflict');
		case 'not_replaced':
			return t('downloads.why.notReplaced');
		case 'not_sent':
			return t('downloads.why.notSent');
		case 'taken':
			return t('downloads.why.taken');
	}
	return t('downloads.why.out');
}

/** removeFilesWords says what "Also delete files" deletes: the folder of
 * each row when known, otherwise its files in the Downloader. */
export function removeFilesWords(rows: Pick<DownloadRow, 'path' | 'downloader'>[]): string {
	if (!rows.length) return '';
	if (rows.length === 1) {
		const r = rows[0];
		const client = r.downloader || t('downloads.rm.theDownloader');
		return r.path ? t('downloads.rm.pathIn', { path: r.path, client }) : t('downloads.rm.oneFiles', { client });
	}
	const paths = rows.map((r) => r.path).filter((p): p is string => !!p);
	const clients = [...new Set(rows.map((r) => r.downloader || t('downloads.rm.theDownloader')))].join(', ');
	if (!paths.length) return t('downloads.rm.theirFiles', { clients });
	const shown = paths.slice(0, 3).join(', ');
	const rest = rows.length - Math.min(paths.length, 3);
	return rest ? t('downloads.rm.shownMore', { shown, rest, clients }) : t('downloads.rm.shown', { shown, clients });
}
