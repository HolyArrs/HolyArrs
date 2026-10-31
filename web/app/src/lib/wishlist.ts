// Words and small rules for the Wishlist screens.

import type { Ask, AskLimit, AskState, LimitKind } from './types.ts';

/** askTone is the chip colour for an ask state. */
export function askTone(state: AskState): 'wait' | 'ok' | 'fail' | '' {
	return ({ asked: 'wait', approved: '', turned_down: 'fail', available: 'ok' } as const)[state] ?? '';
}

/** names joins people: "ana", "ana and bo", "ana, bo and cy". */
export function names(list: string[]): string {
	if (list.length <= 1) return list[0] ?? '';
	return `${list.slice(0, -1).join(', ')} and ${list[list.length - 1]}`;
}

/** askedBy says who asked: "Asked by ana and bo". */
export function askedBy(a: Pick<Ask, 'people' | 'asked_by'>): string {
	const list = a.people.length ? a.people.map((p) => p.name) : [a.asked_by.name];
	return `Asked by ${names(list)}`;
}

/** decided says who decided and how, or '' while it waits. */
export function decided(a: Pick<Ask, 'state' | 'decided_by' | 'auto_approved'>): string {
	if (!a.decided_by || a.state === 'asked') return '';
	if (a.state === 'turned_down') return `Declined by ${a.decided_by.name}`;
	if (a.auto_approved) return `Approved straight away (${a.decided_by.name} may auto-approve)`;
	return `Approved by ${a.decided_by.name}`;
}

/** waitWords says a wait in words, like the server: "2 days and 3 hours". */
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

/** kindAsks names asks of one kind: "1 movie ask", "3 show asks". */
export function kindAsks(kind: LimitKind, n: number): string {
	const word = { movies: 'movie', shows: 'show', music: 'music' }[kind];
	return `${n} ${word} ask${n === 1 ? '' : 's'}`;
}

/**
 * limitLine says how many asks of one kind are left this week, or when to
 * try again; '' when no limit applies.
 */
export function limitLine(l: AskLimit | null, kind: LimitKind, now = Date.now()): string {
	if (!l || l.no_limit) return '';
	const k = l[kind];
	if (!k || k.no_limit) return '';
	if (k.left > 0) return `You have ${kindAsks(kind, k.left)} left this week (your limit is ${k.per_week}).`;
	const wait = k.retry_at ? waitWords(Date.parse(k.retry_at) - now) : 'a few days';
	return `You have used your ${kindAsks(kind, k.per_week)} this week. Try again in ${wait}.`;
}

/** limitLines is limitLine for each of the kinds, leaving out kinds with no limit. */
export function limitLines(l: AskLimit | null, kinds: LimitKind[] = limitKinds, now = Date.now()): { kind: LimitKind; text: string }[] {
	return kinds.map((kind) => ({ kind, text: limitLine(l, kind, now) })).filter((x) => x.text);
}

export const momentWords: Record<string, [string, string]> = {
	ask_waiting: ['Someone asks', 'To the staff: a movie, show or artist waits for approval.'],
	ask_approved: ['Approved', 'To the people who asked.'],
	ask_turned_down: ['Declined', 'To the people who asked, with the note.'],
	ask_ready: ['Ready', 'To the people who asked, once the files are in the Library (ready to watch or listen to).'],
	new_episodes: ['New episodes', 'To the people who asked for or follow a show: new episodes and seasons are in the Library, one notice per show per hour. Each person can turn these off in Account.'],
	new_albums: ['New albums', 'To the people who follow an artist: a new album is in the Library, one notice per artist per hour.'],
	grab: ['Sent to download', 'To the staff: a Release went to a Downloader.'],
	import: ['In the Library', 'To the staff: files were placed (a better copy counts too).'],
	rename: ['Renamed', 'To the staff: files got new names.'],
	add: ['Added', 'To the staff: a movie or show added by hand, from a list or an approved ask.'],
	delete: ['Deleted', 'To the staff: a movie, a show, or a season or episodes of a show left the Library.'],
	health: ['Health', 'To the staff: a health check broke, or is fixed again.'],
	needs_decision: ['Needs a decision', 'To the staff: a download or import needs you.']
};

/** askMoments go to the people who asked (and the staff for a new ask). */
export const askMoments = ['ask_waiting', 'ask_approved', 'ask_turned_down', 'ask_ready', 'new_episodes', 'new_albums'] as const;
/** libraryMoments go to the staff. */
export const libraryMoments = ['grab', 'import', 'rename', 'add', 'delete', 'health', 'needs_decision'] as const;

export type WishlistView = 'waiting' | 'all' | 'mine';

/** viewCounts counts the asks in each Approvals view. */
export function viewCounts(asks: Pick<Ask, 'state' | 'mine'>[]): Record<WishlistView, number> {
	return { waiting: asks.filter((a) => a.state === 'asked').length, all: asks.length, mine: asks.filter((a) => a.mine).length };
}

/** firstView is the first view with items: Waiting, else Everyone's, else Mine. */
export function firstView(asks: Pick<Ask, 'state' | 'mine'>[]): WishlistView {
	const n = viewCounts(asks);
	return n.waiting ? 'waiting' : n.all ? 'all' : 'mine';
}

/** The Approvals tabs. Failed is an approved ask that never reached the Library. */
export type AskTab = 'waiting' | 'approved' | 'declined' | 'done' | 'failed';
export const askTabs: { tab: AskTab; label: string }[] = [
	{ tab: 'waiting', label: 'Pending' },
	{ tab: 'approved', label: 'Approved' },
	{ tab: 'declined', label: 'Declined' },
	{ tab: 'done', label: 'Available' },
	{ tab: 'failed', label: 'Failed' }
];

/** askTab says which tab an ask sits in. */
export function askTab(a: Pick<Ask, 'state' | 'title_id'>): AskTab {
	if (a.state === 'asked') return 'waiting';
	if (a.state === 'turned_down') return 'declined';
	if (a.state === 'available') return 'done';
	return a.title_id ? 'approved' : 'failed';
}

/** tabCounts counts the asks in each tab. */
export function tabCounts(asks: Pick<Ask, 'state' | 'title_id'>[]): Record<AskTab, number> {
	const n: Record<AskTab, number> = { waiting: 0, approved: 0, declined: 0, done: 0, failed: 0 };
	for (const a of asks) n[askTab(a)]++;
	return n;
}

/** firstTab is Waiting when anything waits, else the first tab with asks. */
export function firstTab(asks: Pick<Ask, 'state' | 'title_id'>[]): AskTab {
	const n = tabCounts(asks);
	return askTabs.find((t) => n[t.tab] > 0)?.tab ?? 'waiting';
}
