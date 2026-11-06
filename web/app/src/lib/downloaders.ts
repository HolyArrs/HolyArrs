// Downloader helpers, kept apart from the pages so they can be tested.
import type { Tone } from './format.ts';

export interface DownloaderHealth {
	state: 'working' | 'paused' | 'resting' | 'not_answering' | 'sign_in_refused';
	label: string;
	text: string;
}

export interface PathMap {
	theirs: string;
	ours: string;
}

export interface DownloaderView {
	id: number;
	name: string;
	kind: 'qbittorrent' | 'sabnzbd' | 'transmission' | 'nzbget' | 'deluge';
	protocol: 'torrent' | 'usenet';
	url: string;
	username: string;
	secret: string;
	category: string;
	priority: number;
	path_map: PathMap[];
	allow_local_network: boolean;
	/** "HolyArrs can change this Downloader"; off, HolyArrs only reads it. */
	writable: boolean;
	/** HOLYARRS_READONLY_DOWNLOADERS keeps it read only whatever the switch says. */
	writable_locked: boolean;
	/** Titles with one of these go only to Downloaders that have it (#108). */
	tags?: string[];
	health: DownloaderHealth;
	checked_at?: string;
	active_downloads: number;
}

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
	/** Add a matched outside item's movie or show to the Library (no search). Off by default. */
	downloads_auto_add?: boolean;
}

/** DownloaderAttempt is the public recovery record for one uncertain send.
 * It intentionally has no Downloader address, request marker or error text. */
export interface DownloaderAttempt {
	id: number;
	downloader_id: number;
	release_id: number;
	slot_id: number;
	state: 'pending' | 'resolved' | 'abandoned';
	created_at: string;
	updated_at: string;
}

/** pendingAttempts returns only sends that still need the owner's decision. */
export function pendingAttempts(attempts: DownloaderAttempt[] | undefined): DownloaderAttempt[] {
	return (attempts ?? []).filter((attempt) => attempt.state === 'pending');
}

/** DownloadView is a Download in plain words (the movie page). */
export interface DownloadView {
	id: number;
	state: string;
	label: string;
	text: string;
	progress: number;
	speed: number;
	/** Upload speed, seeds and peers (torrents; older servers omit them). */
	up_speed?: number;
	seeds?: number;
	peers?: number;
	seeds_total?: number;
	peers_total?: number;
	eta?: number;
	active: boolean;
}

export { testVersion } from './test-tag.ts';

export const kindWords: Record<DownloaderView['kind'], string> = {
	qbittorrent: 'qBittorrent',
	sabnzbd: 'SABnzbd',
	transmission: 'Transmission',
	nzbget: 'NZBGet',
	deluge: 'Deluge'
};

/** healthTone colours a Downloader's health chip. */
export function healthTone(state: DownloaderHealth['state']): Tone {
	if (state === 'working') return 'ok';
	if (state === 'paused') return 'idle';
	if (state === 'resting') return 'wait';
	return 'fail';
}

/** downloadTone colours a Download's state chip. */
export function downloadTone(state: string): Tone {
	if (state === 'finished' || state === 'seeding') return 'ok';
	if (state === 'failed' || state === 'stalled' || state === 'vanished') return 'fail';
	if (state === 'cleaned') return 'idle';
	return 'wait';
}

/** percent says progress as a whole percent. */
export function percent(p: number): string {
	return `${Math.floor(Math.min(Math.max(p, 0), 1) * 100)}%`;
}

/** timeLeft says seconds left in words. */
export function timeLeft(s?: number): string {
	if (!s || s <= 0) return '';
	if (s < 60) return 'under a minute left';
	if (s < 3600) return `${Math.round(s / 60)} min left`;
	const h = Math.floor(s / 3600);
	const m = Math.round((s % 3600) / 60);
	return m ? `${h} h ${m} min left` : `${h} h left`;
}

export interface DownloaderForm {
	name: string;
	kind: DownloaderView['kind'];
	url: string;
	username: string;
	secret: string;
	category: string;
	priority: number;
	path_map: PathMap[];
	allow_local_network: boolean;
	writable: boolean;
	tags: string[];
}

/** emptyForm starts a new Downloader. Downloaders usually run at home, so
 * the local-network switch starts on (decision log 4.0.2). */
export function emptyForm(kind: DownloaderView['kind'] = 'qbittorrent'): DownloaderForm {
	return {
		name: kindWords[kind],
		kind,
		url: kind === 'sabnzbd' ? 'http://' : 'http://',
		username: '',
		secret: '',
		category: kind === 'sabnzbd' ? 'movies' : 'holyarrs',
		priority: 1,
		path_map: [],
		allow_local_network: true,
		writable: true,
		tags: []
	};
}

/** formFrom fills the form from a saved Downloader; the secret stays masked. */
export function formFrom(d: DownloaderView): DownloaderForm {
	return {
		name: d.name,
		kind: d.kind,
		url: d.url,
		username: d.username,
		secret: d.secret,
		category: d.category,
		priority: d.priority,
		path_map: d.path_map.map((p) => ({ ...p })),
		allow_local_network: d.allow_local_network,
		writable: d.writable,
		tags: [...(d.tags ?? [])]
	};
}

/** formBody turns the form into the API body. Empty path map rows are dropped. */
export function formBody(f: DownloaderForm): Record<string, unknown> {
	return {
		name: f.name.trim(),
		kind: f.kind,
		url: f.url.trim(),
		username: f.kind === 'qbittorrent' || f.kind === 'transmission' || f.kind === 'nzbget' ? f.username.trim() : '',
		secret: f.secret,
		category: f.category.trim(),
		priority: Number(f.priority) || 1,
		path_map: f.path_map.map((p) => ({ theirs: p.theirs.trim(), ours: p.ours.trim() })).filter((p) => p.theirs || p.ours),
		allow_local_network: f.allow_local_network,
		writable: f.writable,
		tags: f.tags
	};
}

/** DownloadRow is one row of the Downloads screen. */
export interface DownloadRow extends DownloadView {
	phase: string;
	name: string;
	title_id?: number;
	title: string;
	year?: number;
	version: string;
	downloader: string;
	/** The folder or file the Downloader holds it in, as it sees it. */
	path?: string;
	/** The Downloader's adapter (qbittorrent, sabnzbd...) and its protocol. */
	downloader_kind?: string;
	protocol?: string;
	size: number;
	started_at?: string;
	finished_at?: string;
	updated_at: string;
	slot_state?: string;
	can_act: boolean;
	/** HolyArrs may not change this row's Downloader: only its own records (Remove, Mark failed). */
	client_locked?: boolean;
	/** A failed copy HolyArrs turned down and moved on from (searching again or another copy on its way). */
	replaced?: boolean;
	kind?: 'movie' | 'show' | 'artist';
	episodes?: EpisodeLine[];
	covers?: string;
	album?: string;
	/** Optional artwork supplied with an activity row; Library metadata remains the fallback. */
	artwork?: string;
	/** A seeding torrent's share ratio, when the Downloader reports one. */
	ratio?: number;
	/** A client torrent's own seeding rules (-2 the Downloader's rule, -1 none). */
	/** A client torrent's finer state: checking, moving, errored, missing (files gone) or idle_seed (finished, nobody asking now). */
	/** A client torrent's place in the Downloader's queue (absent: not queued). */
	queue_pos?: number;
	/** The host of the tracker a client torrent works with now. */
	tracker?: string;
	/** Who asked for the row's title ("Added in <client>" when nobody did; absent with no title). */
	requested_by?: string;
	/** The Source the release came from (downloads HolyArrs sent). */
	source_name?: string;
	source_id?: number;
	/** When the Usenet post was made (RFC3339). */
	posted_at?: string;
	/** Trackers not all well: error, warning, or announce (none answering). */
	tracker_problem?: 'error' | 'warning' | 'announce';
	/** A downloading torrent's full copies in the swarm (under 1: nobody holds every piece). */
	availability?: number;
	/** A client torrent's own speed caps in bytes per second (absent: none). */
	limit_down?: number;
	limit_up?: number;
	client_hint?: 'checking' | 'moving' | 'errored' | 'missing' | 'idle_seed' | 'verifying' | 'repairing' | 'unpacking' | 'finishing' | 'script';
	limits?: { ratio: number; seeding_minutes: number; inactive_minutes: number; action: '' | 'stop' | 'remove' | 'remove_files' | 'super_seed' };
	/** Read straight from the client's own list: no HolyArrs download id; addressed by downloader_id and hash. */
	client_item?: boolean;
	/** HolyArrs did not send it (a torrent added in the client by hand or by another app). */
	outside?: boolean;
	/** An outside torrent matched to a Library title: title, kind and covers come from that match. */
	linked?: boolean;
	downloader_id?: number;
	hash?: string;
	/** The client's own category for it. */
	category?: string;
	/** The row's key in push updates: "<downloader id>:<external id>". */
	live_key?: string;
}

/** rowPath is the API path of a row's client controls: a HolyArrs download by its id, a client item by Downloader and hash. */
export function rowPath(r: Pick<DownloadRow, 'id' | 'client_item' | 'downloader_id' | 'hash'>): string {
	return r.client_item && r.hash ? `/downloaders/${r.downloader_id}/items/${r.hash}` : `/downloads/${r.id}`;
}

/** kindOf is a row's library type; a client item HolyArrs does not know has none. */
export function kindOf(r: Pick<DownloadRow, 'kind' | 'client_item'>): 'movie' | 'show' | 'artist' | '' {
	return r.kind || (r.client_item ? '' : 'movie');
}

/** EpisodeLine is one episode (or track) a download covers, with where its
 *  Slot is; a track carries its disc in season and its number in episode. */
export interface EpisodeLine {
	slot_id: number;
	season: number;
	episode: number;
	label: string;
	name?: string;
	state: string;
	track?: boolean;
}

/** manyWords says whether a download fills many Slots of one Title (a show's
 *  episodes or an album's tracks), and the plain words for them. */
export function manyWords(kind: string | undefined): { one: string; many: string; pack: string } | null {
	if (kind === 'show') return { one: 'episode', many: 'episodes', pack: 'season pack' };
	if (kind === 'artist') return { one: 'track', many: 'tracks', pack: 'album' };
	return null;
}

/** episodeStateWords says where one episode of a download is. */
export function episodeStateWords(state: string): { text: string; tone: Tone } {
	switch (state) {
		case 'in_library':
		case 'better_wanted':
			return { text: 'In the Library', tone: 'ok' };
		case 'needs_a_decision':
			return { text: 'Review', tone: 'fail' };
		case 'importing':
			return { text: 'Importing', tone: 'wait' };
		case 'downloading':
		case 'chosen':
			return { text: 'Downloading', tone: 'wait' };
		case 'stalled':
			return { text: 'Stalled', tone: 'fail' };
		case 'failed':
		case 'rejected':
		case 'wanted':
		case 'searching':
		case 'held':
			return { text: 'Looking for another copy', tone: 'wait' };
	}
	return { text: state.replaceAll('_', ' '), tone: 'idle' };
}

/** rowName names a Downloads row: the movie with its year, the show, or
 *  Artist – Album (Year). */
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

export interface RemoveChoice {
	delete_files: boolean;
	turn_down: boolean;
	search_again: boolean;
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

/** Decision is one movie on Needs a decision. */
export interface Decision {
	slot_id: number;
	title_id: number;
	title: string;
	year?: number;
	version: string;
	versions: number;
	why: 'ran_out' | 'conflict' | 'not_replaced' | 'not_sent';
	reason: string;
	release_id?: number;
	release_name?: string;
	has_copy: boolean;
	since: string;
	kind?: 'movie' | 'show' | 'artist';
	episode?: EpisodeLine;
	album?: string;
	download_id?: number;
	pack_slot_ids?: number[];
	pack?: EpisodeLine[];
	/** Optional artwork supplied with a decision; Library metadata remains the fallback. */
	artwork?: string;
}

/** decisionName names what waits: the movie (and year) or the show and episode. */
export function decisionName(d: Pick<Decision, 'title' | 'year' | 'episode' | 'album'>): string {
	if (d.episode && d.album) return `${d.title} – ${d.album} ${d.episode.label}${d.episode.name ? ` ${d.episode.name}` : ''}`;
	if (d.episode) return `${d.title} ${d.episode.label}${d.episode.name ? ` ${d.episode.name}` : ''}`;
	return `${d.title}${d.year ? ` (${d.year})` : ''}`;
}

/** DecisionGroup is one card on Needs a decision: a movie, an episode, or
 *  the stuck episodes of one season pack together. */
export interface DecisionGroup {
	key: string;
	lead: Decision;
	items: Decision[];
	pack: boolean;
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
			return 'The import could not finish on its own.';
		case 'not_replaced':
			return 'Its download was removed outside HolyArrs and is not replaced by itself (setting).';
		case 'not_sent':
			return 'No Downloader could take it: check the tags on the title and your Downloaders.';
	}
	return 'HolyArrs ran out of Releases that pass the Pick rules.';
}

/** removeFilesWords says what "Also delete files" deletes: the folder of
 * each row when known, otherwise its files in the Downloader. */
export function removeFilesWords(rows: Pick<DownloadRow, 'path' | 'downloader'>[]): string {
	if (!rows.length) return '';
	if (rows.length === 1) {
		const r = rows[0];
		const client = r.downloader || 'the Downloader';
		return r.path ? `Deletes ${r.path} in ${client}.` : `Deletes its files in ${client}.`;
	}
	const paths = rows.map((r) => r.path).filter((p): p is string => !!p);
	const clients = [...new Set(rows.map((r) => r.downloader || 'the Downloader'))].join(', ');
	if (!paths.length) return `Deletes their files in ${clients}.`;
	const shown = paths.slice(0, 3).join(', ');
	const rest = rows.length - Math.min(paths.length, 3);
	return `Deletes ${shown}${rest ? ` and the files of ${rest} more` : ''} in ${clients}.`;
}

/** reachWords turns any "couldn't connect" answer into one plain line; other answers pass through. */
export function reachWords(text: string): string {
	return /refused|no such host|timed? ?out|unreachable|dial tcp|no route|connection reset|could not (connect|reach)|couldn.t (connect|reach)|can.t connect|EOF/i.test(text)
		? "Can't reach that address. Check it and that the app is running."
		: text;
}
