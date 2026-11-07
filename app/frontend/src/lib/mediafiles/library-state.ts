// Works out the one state a Library title shows: its label, tone, icon and sort rank.

import type { TitleView } from '../core/types.ts';
import type { StatusKind } from '#lib/ui/StatusPill.svelte';
import type { IconName } from '#lib/ui/Icon.svelte';
import { quality } from '#lib/ui/title-cards.ts';
import { date } from '../core/format.ts';

export type LibStateKey =
	| 'failed' | 'needs_decision' | 'stalled' | 'missing_file' | 'missing' | 'better_wanted' | 'paused' | 'downloading' | 'queued'
	| 'approved' | 'waiting_approval' | 'not_out_yet' | 'not_kept' | 'specials_only' | 'declined' | 'in_library' | 'up_to_date' | 'ended';

export type LibTone = 'ok' | 'info' | 'wait' | 'fail' | 'idle';

export interface LibState {
	key: LibStateKey;
	label: string;
	tone: LibTone;
	icon: IconName;
	pill: StatusKind;
	percent?: number;
	rank: number;
}

// Most urgent first; rank is the position here.
const order: LibStateKey[] = [
	'failed', 'needs_decision', 'stalled', 'missing_file', 'missing', 'better_wanted', 'paused', 'downloading', 'queued',
	'approved', 'waiting_approval', 'not_out_yet', 'not_kept', 'specials_only', 'declined', 'in_library', 'up_to_date', 'ended'
];

// Tone, icon and pill for each state.
const look: Record<LibStateKey, [LibTone, IconName, StatusKind]> = {
	failed: ['fail', 'alert', 'failed'],
	needs_decision: ['fail', 'alert', 'failed'],
	stalled: ['fail', 'clock', 'downloading'],
	missing_file: ['fail', 'alert', 'missing'],
	missing: ['wait', 'partOwned', 'missing'],
	better_wanted: ['wait', 'sparkle', 'in_library'],
	paused: ['idle', 'pause', 'downloading'],
	downloading: ['info', 'download', 'downloading'],
	queued: ['info', 'clock', 'downloading'],
	approved: ['ok', 'waiting', 'approved'],
	waiting_approval: ['idle', 'waiting', 'requested'],
	not_out_yet: ['idle', 'calendarDay', 'coming_soon'],
	not_kept: ['idle', 'x', 'turned_down'],
	specials_only: ['ok', 'owned', 'in_library'],
	declined: ['idle', 'declined', 'turned_down'],
	in_library: ['ok', 'owned', 'in_library'],
	up_to_date: ['ok', 'owned', 'in_library'],
	ended: ['ok', 'owned', 'in_library']
};

// Shows and artists count their children; movies read their own slots.
export function libraryState(t: TitleView, now = Date.now()): LibState | null {
	if (t.kind === 'show' || t.kind === 'artist') return groupState(t, now);
	return movieState(t, now);
}

// Anything ranked at or above Better copy needs attention.
export function isProblem(s: LibState | null): boolean {
	return !!s && s.rank <= order.indexOf('better_wanted');
}

function make(key: LibStateKey, label: string, percent?: number): LibState {
	const [tone, icon, pill] = look[key];
	return { key, label, tone, icon, pill, rank: order.indexOf(key), ...(percent !== undefined ? { percent } : {}) };
}

// Clamps progress to 0 to 100 percent.
const pct = (p: number | undefined) => Math.round(Math.min(Math.max(p ?? 0, 0), 1) * 100);

// Picks the strongest request on this title: approved, then asked, then turned down.
function askOf(t: TitleView): 'asked' | 'approved' | 'turned_down' | null {
	const states = (t.asks ?? []).filter((a) => a.title_id === undefined || a.title_id === t.id).map((a) => a.state);
	if (states.includes('approved')) return 'approved';
	if (states.includes('asked')) return 'asked';
	if (states.includes('turned_down')) return 'turned_down';
	return null;
}

// Maps a request answer to a state when nothing is owned yet.
function askState(ask: ReturnType<typeof askOf>): LibState | null {
	if (ask === 'approved') return make('approved', 'Searching');
	if (ask === 'asked') return make('waiting_approval', 'Waiting');
	if (ask === 'turned_down') return make('declined', 'Declined');
	return null;
}

// Checks in order of urgency, so a failure beats a download beats owned.
function movieState(t: TitleView, now: number): LibState {
	const slots = t.slots ?? [];
	const st = (s: string) => slots.some((x) => x.state === s);
	if (st('failed')) return make('failed', 'Failed');
	if (st('needs_a_decision')) return make('needs_decision', 'To review');
	const dl = slots.find((s) => s.download?.active || ['chosen', 'downloading', 'stalled', 'importing'].includes(s.state));
	if (dl) {
		const d = dl.download;
		const p = pct(d?.progress);
		if (dl.state === 'stalled' || d?.state === 'stalled') return make('stalled', `Stalled · ${p}%`, p);
		if (d?.state === 'paused') return make('paused', `Paused · ${p}%`, p);
		if (d?.state === 'queued' || d?.state === 'sent' || dl.state === 'chosen') return make('queued', 'Queued');
		if (dl.state === 'importing') return make('downloading', 'Importing', 100);
		return make('downloading', `Downloading · ${p}%`, p);
	}
	if (slots.some((s) => s.file?.State === 'missing')) return make('missing_file', 'Missing file');
	const q = quality(t);
	if (st('better_wanted')) return make('better_wanted', q ? `Better copy · ${q}` : 'Better copy');
	const has = slots.some((s) => s.state === 'in_library' || s.state === 'no_hardlink' || !!s.file);
	if (!t.keep_up_to_date) return make('not_kept', has && q ? `Not kept · ${q}` : 'Not kept');
	if (has) return make('in_library', q ? `In library · ${q}` : 'In library');
	const ask = askState(askOf(t));
	if (ask) return ask;
	if (st('not_out_yet') || st('waiting')) {
		const d = t.digital_date ?? t.physical_date ?? t.cinema_date;
		return make('not_out_yet', d && Date.parse(d) > now ? `Upcoming · ${date(d)}` : 'Upcoming');
	}
	return make('missing', 'Missing');
}

// Same idea as movieState, using the show or artist counts.
function groupState(t: TitleView, now: number): LibState | null {
	const show = t.kind === 'show';
	const s = show ? t.show : t.artist;
	if (!s) return askState(askOf(t));
	const dl = s.downloading ?? 0;
	const failed = s.failed ?? 0;
	if (failed > 0) return make('failed', `Failed · ${failed}`);
	if (dl > 0) return make('downloading', `Downloading · ${dl}`);
	if (!s.in_library && !s.missing && !s.not_out_yet && !s.better_wanted && !askOf(t)) return t.keep_up_to_date ? null : make('not_kept', 'Not kept');
	if (s.in_library === 0 && s.missing === 0) {
		const ask = askState(askOf(t));
		if (ask) return ask;
	}
	if (!t.keep_up_to_date && s.missing === 0) return make('not_kept', 'Not kept');
	if (show && t.show) {
		const kept = t.show.seasons.filter((x) => x.keep_up_to_date && !x.dropped);
		if (kept.length > 0 && kept.every((x) => x.number === 0)) return make('specials_only', `Specials only · ${t.show.in_library}`);
	}
	if (s.missing > 0) return make('missing', `Missing · ${s.missing}`);
	if (s.better_wanted > 0) return make('better_wanted', `Better copy · ${s.better_wanted}`);
	if (s.in_library === 0) {
		const next = show ? t.show?.next_air_date : t.artist?.next_release;
		if (s.not_out_yet > 0 || next) return make('not_out_yet', next && Date.parse(next) > now ? `Upcoming · ${date(next)}` : 'Upcoming');
		const ask = askState(askOf(t));
		if (ask) return ask;
		return make('missing', 'Missing');
	}
	if (show && t.show?.status === 'ended') return make('ended', 'Complete · ended');
	return make('up_to_date', 'Complete');
}
