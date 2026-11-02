// History preview looks (?look=now|feed|table|story): the address they share,
// the "who did it" filter, the table's cells and a title's story steps. The
// looks sit side by side so they can be compared; each reads the same Events.
import { readFilter, type HistoryFilter, type HistoryRow } from '../history.ts';
import { lookTabs, showOf, showParam, tabKinds } from '../history-looks.ts';
import { resolutionOf } from '../history-lines.ts';
import { bytes } from '../format.ts';
import type { EventRecord } from '../types.ts';

export type Look = 'now' | 'feed' | 'table' | 'story' | 'pill' | 'plain';
export const looks: { key: Look; label: string }[] = [
	{ key: 'now', label: 'Now' },
	{ key: 'feed', label: 'Feed' },
	{ key: 'table', label: 'Table' },
	{ key: 'story', label: 'Story' },
	{ key: 'pill', label: 'Search+Failed' },
	{ key: 'plain', label: 'Search' }
];

/** lookOf reads ?look= (the chosen search look when missing or unknown). */
export function lookOf(v: string | null): Look {
	return looks.some((l) => l.key === v) ? (v as Look) : 'plain';
}

/** Who made an Event happen, as the Filter offers it. */
export type Who = '' | 'me' | 'people' | 'holyarrs';
export const whoLabels: Record<Who, string> = { '': 'Anyone', me: 'You', people: 'Members', holyarrs: 'HolyArrs on its own' };

export function whoOf(v: string | null): Who {
	return v === 'me' || v === 'people' || v === 'holyarrs' ? v : '';
}

/** whoMatches keeps an Event made by the picked kind of actor. */
export function whoMatches(e: Pick<EventRecord, 'Actor'>, who: Who, me?: number): boolean {
	const user = e.Actor?.kind === 'user';
	if (who === 'me') return user && e.Actor?.id != null && e.Actor.id === me;
	if (who === 'people') return user;
	if (who === 'holyarrs') return !user;
	return true;
}

/** Everything a preview look reads from the address. */
export interface DirState {
	show: string;
	sub: string;
	who: Who;
	filter: HistoryFilter;
}

type Params = { get(name: string): string | null; toString(): string };

export function readDir(params: Params): DirState {
	const { show, sub } = showOf(params.get('show'));
	return { show, sub, who: whoOf(params.get('who')), filter: { ...readFilter(params, 200), groups: [], offset: 0 } };
}

/** dirQuery is the address for a change, keeping ?look= and dropping empty values. */
export function dirQuery(params: Params, change: Record<string, string | number | undefined>): string {
	const q = new URLSearchParams(params.toString());
	for (const [k, v] of Object.entries(change)) {
		if (v === undefined || v === '' || v === 0) q.delete(k);
		else q.set(k, String(v));
	}
	q.delete('offset');
	const s = q.toString();
	return s ? '?' + s : '';
}

/** showChange is the ?show= value for a tab and sub-tab. */
export const showChange = (show: string, sub = '') => ({ show: showParam(show, sub) });

const str = (v: unknown) => (typeof v === 'string' ? v : '');
const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const first = (row: HistoryRow, read: (d: Record<string, unknown>) => string) => {
	for (const e of row.events) {
		const v = read(e.Data ?? {});
		if (v) return v;
	}
	return '';
};

/** The table's cells for one row ('' when the Events did not keep it). */
export function releaseOf(row: HistoryRow): string {
	return first(row, (d) => str(d.release));
}
export function qualityOf(row: HistoryRow): string {
	return first(row, (d) => str(d.quality)) || resolutionOf(releaseOf(row));
}
export function sourceOf(row: HistoryRow): string {
	return first(row, (d) => str(d.source_name) || str(d.indexer) || str(d.source));
}
export function sizeOf(row: HistoryRow): number {
	for (const e of row.events) {
		const d = e.Data ?? {};
		const n = num(d.size_bytes) || num(d.size) || num(d.bytes);
		if (n > 0) return n;
	}
	return 0;
}
export const sizeText = (row: HistoryRow) => (sizeOf(row) ? bytes(sizeOf(row)) : '');
/** partOf is the episode or album a row is about. */
export function partOf(row: HistoryRow): string {
	return row.head.episode || first(row, (d) => str(d.album)) || '';
}

/** The steps of a title's story, in the order they happen. */
export type StoryKey = 'requested' | 'approved' | 'searched' | 'chosen' | 'downloaded' | 'imported' | 'subtitles' | 'upgraded';
export interface StoryStep {
	key: StoryKey;
	label: string;
	state: 'done' | 'failed' | 'waiting';
	/** The newest Event of the step, if any. */
	last?: EventRecord;
	count: number;
}

const storyKinds: { key: StoryKey; label: string; done: RegExp; failed?: RegExp }[] = [
	{ key: 'requested', label: 'Requested', done: /^ask_made$/ },
	{ key: 'approved', label: 'Approved', done: /^ask_approved$/, failed: /^ask_turned_down$/ },
	{ key: 'searched', label: 'Searched', done: /^(search_done|feed_checked)$/, failed: /^(search_failed|stopped_looking)$/ },
	{ key: 'chosen', label: 'Chosen', done: /^release_chosen(_by_hand)?$/, failed: /^needs_decision$/ },
	{ key: 'downloaded', label: 'Downloaded', done: /^download_(finished|seeding)$/, failed: /^download_(failed|vanished)$/ },
	{ key: 'imported', label: 'Imported', done: /^(import_done|import_partly_done|adopted)$/, failed: /^(import_failed|import_rejected|file_hold_failed)$/ },
	{ key: 'subtitles', label: 'Subtitles', done: /^subtitle_(saved|upgraded|synced)$/, failed: /^subtitle_/ },
	{ key: 'upgraded', label: 'Upgraded', done: /^file_replaced$/ }
];

/** storyOf lays a title's Events (newest first) out as its story; a later success clears an older failure. */
export function storyOf(events: EventRecord[]): StoryStep[] {
	return storyKinds.map((s) => {
		const hits = events.filter((e) => s.done.test(e.Kind) || s.failed?.test(e.Kind));
		const last = hits[0];
		const state = !last ? 'waiting' : s.done.test(last.Kind) ? 'done' : 'failed';
		return { key: s.key, label: s.label, state, last, count: hits.length };
	});
}

/** One card of the Story look: a title and its newest Event. */
export interface TitleCard {
	id: number;
	name: string;
	last: EventRecord;
	count: number;
	problem: boolean;
}

const bad = /failed|rejected|vanished|needs_decision/;

/** titleCards folds Events (newest first) into one card per title, newest first. */
export function titleCards(events: EventRecord[]): TitleCard[] {
	const out = new Map<number, TitleCard>();
	for (const e of events) {
		if (!e.TitleID) continue;
		const c = out.get(e.TitleID);
		if (c) c.count++;
		else out.set(e.TitleID, { id: e.TitleID, name: e.title_name ?? 'Untitled', last: e, count: 1, problem: bad.test(e.Kind) });
	}
	return [...out.values()];
}

/** canRetry: the row failed somewhere. canBlock: the row is about a release that could be swapped. */
export const canRetry = (row: HistoryRow) => row.steps.some((s) => s.state === 'failed') || row.events.some((e) => bad.test(e.Kind));
export const canBlock = (row: HistoryRow) => row.steps.length > 0 || !!releaseOf(row);

/* The search looks (?look=pill|plain): a search bar, a Filter with steps,
   type, who and dates, and a chip per filter that is on. Routine work
   (searches, refreshes, scans) is left out of them. */

/** A step of a download's life, as the Filter offers it, and the Event kinds it means. */
export const searchSteps: { key: string; label: string; kinds: string[] }[] = [
	{ key: 'asked', label: 'Asked', kinds: ['ask_made'] },
	{ key: 'approved', label: 'Approved', kinds: ['ask_approved'] },
	{ key: 'chosen', label: 'Chosen', kinds: ['release_chosen', 'release_chosen_by_hand'] },
	{ key: 'sent', label: 'Sent', kinds: ['download_sent', 'download_queued'] },
	{ key: 'downloaded', label: 'Downloaded', kinds: ['download_finished', 'download_seeding'] },
	{ key: 'imported', label: 'Imported', kinds: ['import_done', 'import_partly_done', 'adopted'] },
	{ key: 'renamed', label: 'Renamed', kinds: ['renamed', 'rename_planned'] },
	{ key: 'subtitles', label: 'Subtitles', kinds: ['subtitle_*'] },
	{ key: 'media', label: 'In media server', kinds: ['media_server_told'] },
	{ key: 'upgraded', label: 'Upgraded', kinds: ['file_replaced'] },
	{ key: 'deleted', label: 'Deleted', kinds: ['title_removed', 'episodes_removed', 'file_held', 'holding_cleaned'] },
	{ key: 'failed', label: 'Failed', kinds: tabKinds('problems') ?? [] }
];

/** Every kind but routine work: what the search looks show with no step picked. */
export const nonRoutineKinds = lookTabs.filter((t) => t.key !== 'routine').flatMap((t) => t.subs.flatMap((s) => s.kinds));

/** stepKinds is what the picked steps ask the server for (every kind but routine when none is picked). */
export function stepKinds(steps: string[]): string[] {
	const picked = searchSteps.filter((s) => steps.includes(s.key));
	return picked.length ? [...new Set(picked.flatMap((s) => s.kinds))] : [...nonRoutineKinds];
}

export type MediaType = '' | 'movies' | 'shows' | 'music';
export const typeLabels: Record<Exclude<MediaType, ''>, string> = { movies: 'Movies', shows: 'Shows', music: 'Music' };

/** typeMatches keeps an Event about a title of the picked type (a title's kind: movie, show, artist or album). */
export function typeMatches(kind: string | undefined, type: MediaType): boolean {
	if (!type) return true;
	if (type === 'movies') return kind === 'movie';
	if (type === 'shows') return kind === 'show';
	return kind === 'artist' || kind === 'album' || kind === 'music';
}

/** textMatches keeps an Event whose title, summary, episode, source, release or person holds the text. */
export function textMatches(e: EventRecord, q: string, people?: Map<number, string>): boolean {
	const t = q.trim().toLowerCase();
	if (!t) return true;
	const d = e.Data ?? {};
	const who = e.Actor?.kind === 'user' && e.Actor.id != null ? people?.get(e.Actor.id) : '';
	return [e.title_name, e.Summary, e.episode, who, str(d.source_name), str(d.indexer), str(d.source), str(d.release)].some((v) => v?.toLowerCase().includes(t));
}

/** Everything a search look reads from the address. */
export interface SearchState {
	q: string;
	steps: string[];
	type: MediaType;
	who: Who;
	from?: string;
	to?: string;
}

export function readSearch(params: Params): SearchState {
	const f = readFilter(params);
	const steps = (params.get('step') ?? '').split(',').filter((k) => searchSteps.some((s) => s.key === k));
	const t = params.get('type');
	const type: MediaType = t === 'movies' || t === 'shows' || t === 'music' ? t : '';
	return { q: params.get('q') ?? '', steps, type, who: whoOf(params.get('who')), from: f.from, to: f.to };
}

/** toggleStep is the ?step= value with one step switched on or off. */
export function toggleStep(steps: string[], key: string): string {
	return (steps.includes(key) ? steps.filter((s) => s !== key) : [...steps, key]).join(',');
}

const day = (d: Date) => d.toISOString().slice(0, 10);

/** dateLabel names a date range ("Last 7 days" when it runs from n days ago to now). */
export function dateLabel(from?: string, to?: string, now = new Date()): string {
	if (from && !to) {
		for (const n of [1, 7, 30, 90]) {
			if (from === day(new Date(now.getTime() - n * 86400000))) return n === 1 ? 'Last 24 hours' : `Last ${n} days`;
		}
		return `From ${from}`;
	}
	if (from && to) return `${from} to ${to}`;
	return to ? `Until ${to}` : '';
}

/** One removable chip under the bar: its label and the address change that clears it. */
export interface Chip {
	key: string;
	label: string;
	change: Record<string, string | undefined>;
}

/** searchChips is a chip per filter that is on (the search text has its own box). */
export function searchChips(s: SearchState, now = new Date()): Chip[] {
	const out: Chip[] = [];
	for (const st of searchSteps) if (s.steps.includes(st.key)) out.push({ key: 'step.' + st.key, label: st.label, change: { step: toggleStep(s.steps, st.key) } });
	if (s.type) out.push({ key: 'type', label: typeLabels[s.type], change: { type: undefined } });
	if (s.who) out.push({ key: 'who', label: whoLabels[s.who], change: { who: undefined } });
	if (s.from || s.to) out.push({ key: 'date', label: dateLabel(s.from, s.to, now), change: { from: undefined, to: undefined } });
	return out;
}

/** The address change that clears every chip. */
export const clearChips = { step: undefined, type: undefined, who: undefined, from: undefined, to: undefined };

/* The Search looks' row: the step in plain words, then at most one extra piece. */

/** What a row's step is: its word, its icon and whether it went wrong. */
export interface RowStep {
	label: string;
	icon: 'wish' | 'check' | 'search' | 'upload' | 'download' | 'folder' | 'copy' | 'subtitle' | 'tv' | 'sparkle' | 'trash' | 'x' | 'alert' | 'info';
	failed: boolean;
}
const problemKinds = new Set(tabKinds('problems') ?? []);
/** isProblem says whether a kind is one of the Problems tab's kinds. */
export function isProblem(kind: string): boolean {
	return problemKinds.has(kind) || [...problemKinds].some((k) => k.endsWith('*') && kind.startsWith(k.slice(0, -1)));
}
const kindStep: [RegExp, Omit<RowStep, 'failed'>][] = [
	[/^ask_made$/, { label: 'Asked', icon: 'wish' }],
	[/^ask_approved$/, { label: 'Approved', icon: 'check' }],
	[/^release_chosen(_by_hand)?$/, { label: 'Chosen', icon: 'search' }],
	[/^download_(sent|queued)$/, { label: 'Sent', icon: 'upload' }],
	[/^download_(downloading|stalled|paused)$/, { label: 'Downloading', icon: 'download' }],
	[/^download_(finished|seeding)$/, { label: 'Downloaded', icon: 'download' }],
	[/^(import_done|import_partly_done|adopted)$/, { label: 'Imported', icon: 'folder' }],
	[/^(renamed|rename_planned)$/, { label: 'Renamed', icon: 'copy' }],
	[/^subtitle_/, { label: 'Subtitles', icon: 'subtitle' }],
	[/^media_server_told$/, { label: 'In media server', icon: 'tv' }],
	[/^file_replaced$/, { label: 'Upgraded', icon: 'sparkle' }],
	[/^(title_removed|episodes_removed|file_held|holding_cleaned|download_removed)$/, { label: 'Deleted', icon: 'trash' }]
];
/** rowStep names what a row's newest step is; a folded download reads its furthest step. */
export function rowStep(row: HistoryRow, fallback = ''): RowStep {
	const kinds = row.events.map((e) => e.Kind);
	const bad = kinds.find(isProblem);
	if (bad || row.steps.some((s) => s.state === 'failed')) return { label: bad === 'needs_decision' ? 'Needs you' : 'Failed', icon: bad === 'needs_decision' ? 'alert' : 'x', failed: true };
	const pick = row.steps.length ? (kinds.includes('file_replaced') ? 'file_replaced' : kinds.find((k) => /^import_/.test(k)) ?? kinds.find((k) => /^download_(finished|seeding)$/.test(k)) ?? row.head.Kind) : row.head.Kind;
	const hit = kindStep.find(([re]) => re.test(pick));
	return hit ? { ...hit[1], failed: false } : { label: fallback || 'Changed', icon: 'info', failed: false };
}
/**
 * rowExtra is the one extra piece a row shows after its step ('' when the
 * Events did not keep it): who for asks, quality when chosen or sent, size
 * when imported, quality when upgraded, language for subtitles, the reason
 * for a failure.
 */
export function rowExtra(row: HistoryRow, step: RowStep, who: (e: EventRecord) => string = () => ''): string {
	if (step.failed) {
		for (const e of row.events) {
			const d = e.Data ?? {};
			const why = str(d.reason) || str(d.error) || str(d.message);
			if (why) return why.trim().replace(/[.;]\s*$/, '');
		}
		return '';
	}
	switch (step.label) {
		case 'Asked':
		case 'Approved': return who(row.head);
		case 'Chosen':
		case 'Sent':
		case 'Upgraded': return qualityOf(row);
		case 'Imported': return sizeText(row);
		case 'Subtitles': return first(row, (d) => str(d.language)).toUpperCase();
	}
	return '';
}
/** agoShort is a short relative time: "now", "2m", "3h", "Yesterday", then the date. */
export function agoShort(at: string, now = new Date()): string {
	const t = new Date(at);
	const s = Math.max(0, (now.getTime() - t.getTime()) / 1000);
	if (s < 60) return 'now';
	if (s < 3600) return `${Math.floor(s / 60)}m`;
	const y = new Date(now);
	y.setDate(y.getDate() - 1);
	if (t.toDateString() === now.toDateString()) return `${Math.floor(s / 3600)}h`;
	if (t.toDateString() === y.toDateString()) return 'Yesterday';
	return t.toLocaleDateString('en-US', { month: 'short', day: 'numeric', ...(t.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) });
}
