// History preview looks (?look=now|feed|table|story): the address they share,
// the "who did it" filter, the table's cells and a title's story steps. The
// looks sit side by side so they can be compared; each reads the same Events.
import { readFilter, type HistoryFilter, type HistoryRow } from '../history.ts';
import { showOf, showParam } from '../history-looks.ts';
import { resolutionOf } from '../history-lines.ts';
import { bytes } from '../format.ts';
import type { EventRecord } from '../types.ts';

export type Look = 'now' | 'feed' | 'table' | 'story';
export const looks: { key: Look; label: string }[] = [
	{ key: 'now', label: 'Now' },
	{ key: 'feed', label: 'Feed' },
	{ key: 'table', label: 'Table' },
	{ key: 'story', label: 'Story' }
];

/** lookOf reads ?look= (today's look when missing or unknown). */
export function lookOf(v: string | null): Look {
	return looks.some((l) => l.key === v) ? (v as Look) : 'now';
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
