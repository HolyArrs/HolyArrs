// Words and small rules for Requests screens.

import type { AskLimit, LimitKind } from './types.ts';
import type { Ask } from '../core/types.ts';

/** The Requests tabs. Failed is an approved ask that never reached the Library. */
export type AskTab = 'waiting' | 'approved' | 'declined' | 'done' | 'failed';

/** The approvers' tabs, simpler than the members' five: Pending waits for an answer, Processing is approved and not in the Library yet (failed ones too), Done is in the Library or declined. */
export type ApproverTab = 'pending' | 'processing' | 'done';

/** Joins people: "ana", "ana and bo", "ana, bo and cy". */
export function names(list: string[]): string {
	if (list.length <= 1) return list[0] ?? '';
	return `${list.slice(0, -1).join(', ')} and ${list[list.length - 1]}`;
}

/** Who asked: "Asked by ana and bo". */
export function askedBy(a: Pick<Ask, 'people' | 'asked_by'>): string {
	const list = a.people.length ? a.people.map((p) => p.name) : [a.asked_by.name];
	return `Asked by ${names(list)}`;
}

/** Names the others who declined the same title on earlier requests (approvers only get the list): "Bo and Cy declined it before."; '' when nobody did. Display names only, mine freshest; a sign-in name is never shown. */
export function alsoDeclined(a: Pick<Ask, 'also_declined'>, me?: { id: number; displayName: string }): string {
	const list = a.also_declined ?? [];
	if (!list.length) return '';
	const said: string[] = [];
	let unnamed = 0;
	for (const w of list) {
		const n = (me && me.id === w.id ? me.displayName.trim() : '') || w.display_name?.trim() || '';
		if (n) said.push(n);
		else unnamed++;
	}
	if (unnamed) said.push(said.length ? `${unnamed} other${unnamed > 1 ? 's' : ''}` : unnamed > 1 ? `${unnamed} other people` : '1 other person');
	return `${names(said)} declined it before.`;
}

/** A wait in words, like the server: "2 days and 3 hours". */
export function waitWords(ms: number): string {
	if (ms <= 60000) return 'a minute';
	// Rounded up to the unit shown, never said shorter than it is (as the server does).
	const m = ms > 86400000 ? Math.ceil(ms / 3600000) * 60 : Math.ceil(ms / 60000);
	const days = Math.floor(m / 1440);
	const hours = Math.floor((m % 1440) / 60);
	const mins = m % 60;
	const n = (v: number, w: string) => `${v} ${w}${v === 1 ? '' : 's'}`;
	if (days && hours) return `${n(days, 'day')} and ${n(hours, 'hour')}`;
	if (days) return n(days, 'day');
	if (hours && mins) return `${n(hours, 'hour')} and ${n(mins, 'minute')}`;
	if (hours) return n(hours, 'hour');
	return n(mins, 'minute');
}

export const limitKinds: LimitKind[] = ['movies', 'shows', 'music'];

/** Names asks of one kind: "1 movie ask", "3 show asks". */
export function kindAsks(kind: LimitKind, n: number): string {
	const word = { movies: 'movie', shows: 'show', music: 'music' }[kind];
	return `${n} ${word} request${n === 1 ? '' : 's'}`;
}

/** How many asks of one kind are left this week, or when to try again; '' when no limit applies. */
export function limitLine(l: AskLimit | null, kind: LimitKind, now = Date.now()): string {
	if (!l || l.no_limit) return '';
	const k = l[kind];
	if (!k || k.no_limit) return '';
	if (k.left > 0) return `You have ${kindAsks(kind, k.left)} left this week (your limit is ${k.per_week}).`;
	const wait = k.retry_at ? waitWords(Date.parse(k.retry_at) - now) : 'a few days';
	return `You have used your ${kindAsks(kind, k.per_week)} this week. Try again in ${wait}.`;
}

/** limitLine for each of the kinds, leaving out kinds with no limit. */
export function limitLines(l: AskLimit | null, kinds: LimitKind[] = limitKinds, now = Date.now()): { kind: LimitKind; text: string }[] {
	return kinds.map((kind) => ({ kind, text: limitLine(l, kind, now) })).filter((x) => x.text);
}

export const momentWords: Record<string, [string, string]> = {
	ask_waiting: ['New request', 'To the staff: a movie, show or artist waits for approval.'],
	ask_approved: ['Approved', 'To the people who requested it.'],
	ask_turned_down: ['Declined', 'To the people who requested it, with the note.'],
	ask_ready: ['Ready', 'To the people who requested it, once the files are in the Library (ready to watch or listen to).'],
	new_episodes: ['New episodes', 'To the people who requested or follow a show: new episodes and seasons are in the Library, one notice per show per hour. Each person can turn these off in Account.'],
	new_albums: ['New albums', 'To the people who follow an artist: a new album is in the Library, one notice per artist per hour.'],
	report_made: ['Problem reported', 'To the staff: someone reported a problem with a title in the Library.'],
	report_fixed: ['Problem fixed', 'To the person who reported it: the problem was marked fixed, with your reply.'],
	grab: ['Sent to download', 'To the staff: a Release went to a Downloader.'],
	import: ['In the Library', 'To the staff: files were placed (a better copy counts too).'],
	rename: ['Renamed', 'To the staff: files got new names.'],
	add: ['Added', 'To the staff: a movie or show added by hand, from a list or an approved request.'],
	delete: ['Deleted', 'To the staff: a movie, a show, or a season or episodes of a show left the Library.'],
	health: ['Health', 'To the staff: a health check broke, or is fixed again.'],
	needs_decision: ['Needs a decision', 'To the staff: a download or import needs you.']
};

/** Go to the people who asked (and the staff for a new ask). */
export const askMoments = ['ask_waiting', 'ask_approved', 'ask_turned_down', 'ask_ready', 'new_episodes', 'new_albums', 'report_made', 'report_fixed'] as const;

/** Go to the staff. */
export const libraryMoments = ['grab', 'import', 'rename', 'add', 'delete', 'health', 'needs_decision'] as const;

export const askTabs: { tab: AskTab; label: string }[] = [
	{ tab: 'waiting', label: 'Waiting' },
	{ tab: 'approved', label: 'Approved' },
	{ tab: 'declined', label: 'Declined' },
	{ tab: 'done', label: 'Ready' },
	{ tab: 'failed', label: 'Failed' }
];

/** Which tab an ask sits in. */
export function askTab(a: Pick<Ask, 'state' | 'title_id'>): AskTab {
	if (a.state === 'asked') return 'waiting';
	if (a.state === 'turned_down') return 'declined';
	if (a.state === 'available') return 'done';
	return a.title_id ? 'approved' : 'failed';
}

/** Counts the asks in each tab. */
export function tabCounts(asks: Pick<Ask, 'state' | 'title_id'>[]): Record<AskTab, number> {
	const n: Record<AskTab, number> = { waiting: 0, approved: 0, declined: 0, done: 0, failed: 0 };
	for (const a of asks) n[askTab(a)]++;
	return n;
}

/** Waiting when anything waits, else the first tab with asks. */
export function firstTab(asks: Pick<Ask, 'state' | 'title_id'>[]): AskTab {
	const n = tabCounts(asks);
	return askTabs.find((t) => n[t.tab] > 0)?.tab ?? 'waiting';
}

export const approverTabs: { tab: ApproverTab; label: string }[] = [
	{ tab: 'pending', label: 'Waiting' },
	{ tab: 'processing', label: 'Processing' },
	{ tab: 'done', label: 'Done' }
];

/** Which approver tab an ask sits in. */
export function approverTab(a: Pick<Ask, 'state'>): ApproverTab {
	if (a.state === 'asked') return 'pending';
	if (a.state === 'approved') return 'processing';
	return 'done';
}

/** Counts the asks in each approver tab, and the failed ones among Processing. */
export function approverCounts(asks: Pick<Ask, 'state' | 'title_id'>[]): Record<ApproverTab, number> & { failed: number } {
	const n = { pending: 0, processing: 0, done: 0, failed: 0 };
	for (const a of asks) {
		n[approverTab(a)]++;
		if (askTab(a) === 'failed') n.failed++;
	}
	return n;
}

const when = (iso?: string) => (iso ? Date.parse(iso) || 0 : 0);

/** Orders one approver tab: Pending newest ask first; Processing failed first, then newest approved; Done newest decision first. */
export function approverSort<T extends Pick<Ask, 'state' | 'title_id' | 'asked_at' | 'decided_at' | 'available_at'>>(tab: ApproverTab, list: T[]): T[] {
	const out = [...list];
	if (tab === 'pending') return out.sort((x, y) => when(y.asked_at) - when(x.asked_at));
	const decidedAt = (a: T) => when(a.decided_at) || when(a.available_at) || when(a.asked_at);
	if (tab === 'processing') return out.sort((x, y) => Number(askTab(y) === 'failed') - Number(askTab(x) === 'failed') || decidedAt(y) - decidedAt(x));
	return out.sort((x, y) => decidedAt(y) - decidedAt(x));
}
