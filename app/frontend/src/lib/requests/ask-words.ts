// Words and small rules for Requests screens.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { AskLimit, LimitKind } from './types.ts';
import type { Ask } from '../core/types.ts';

/** The Requests tabs. Failed is an approved ask that never reached the Library. */
export type AskTab = 'waiting' | 'approved' | 'declined' | 'done' | 'failed';

/** The approvers' tabs, simpler than the members' five: Pending waits for an answer, Processing is approved and not in the Library yet (failed ones too), Done is in the Library or declined. */
export type ApproverTab = 'pending' | 'processing' | 'done';

/** Joins people: "ana", "ana and bo", "ana, bo and cy". */
export function names(list: string[]): string {
	if (list.length <= 1) return list[0] ?? '';
	return t('requests.words.join', { head: list.slice(0, -1).join(', '), last: list[list.length - 1] });
}

/** Who asked: "Asked by ana and bo". */
export function askedBy(a: Pick<Ask, 'people' | 'asked_by'>): string {
	const list = a.people.length ? a.people.map((p) => p.name) : [a.asked_by.name];
	return t('requests.words.askedBy', { names: names(list) });
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
	if (unnamed) said.push(said.length ? t(unnamed > 1 ? 'requests.words.othersMany' : 'requests.words.othersOne', { n: unnamed }) : unnamed > 1 ? t('requests.words.otherPeople', { n: unnamed }) : t('requests.words.otherPerson'));
	return t('requests.words.declinedBefore', { names: names(said) });
}

/** A wait in words, like the server: "2 days and 3 hours". */
export function waitWords(ms: number): string {
	if (ms <= 60000) return t('requests.wait.minute');
	// Rounded up to the unit shown, never said shorter than it is (as the server does).
	const m = ms > 86400000 ? Math.ceil(ms / 3600000) * 60 : Math.ceil(ms / 60000);
	const days = Math.floor(m / 1440);
	const hours = Math.floor((m % 1440) / 60);
	const mins = m % 60;
	const n = (v: number, w: 'day' | 'hour' | 'minute') => t(`requests.wait.${w}.${v === 1 ? 'one' : 'other'}`, { n: v });
	if (days && hours) return t('requests.wait.and', { a: n(days, 'day'), b: n(hours, 'hour') });
	if (days) return n(days, 'day');
	if (hours && mins) return t('requests.wait.and', { a: n(hours, 'hour'), b: n(mins, 'minute') });
	if (hours) return n(hours, 'hour');
	return n(mins, 'minute');
}

export const limitKinds: LimitKind[] = ['movies', 'shows', 'music'];

/** Names asks of one kind: "1 movie ask", "3 show asks". */
export function kindAsks(kind: LimitKind, n: number): string {
	return t(`requests.askCount.${kind}.${n === 1 ? 'one' : 'other'}`, { n });
}

/** How many asks of one kind are left this week, or when to try again; '' when no limit applies. */
export function limitLine(l: AskLimit | null, kind: LimitKind, now = Date.now()): string {
	if (!l || l.no_limit) return '';
	const k = l[kind];
	if (!k || k.no_limit) return '';
	if (k.left > 0) return t('requests.limit.left', { asks: kindAsks(kind, k.left), limit: k.per_week });
	const wait = k.retry_at ? waitWords(Date.parse(k.retry_at) - now) : t('requests.wait.fewDays');
	return t('requests.limit.used', { asks: kindAsks(kind, k.per_week), wait });
}

/** limitLine for each of the kinds, leaving out kinds with no limit. */
export function limitLines(l: AskLimit | null, kinds: LimitKind[] = limitKinds, now = Date.now()): { kind: LimitKind; text: string }[] {
	return kinds.map((kind) => ({ kind, text: limitLine(l, kind, now) })).filter((x) => x.text);
}

export const momentWords: Record<string, [string, string]> = {
	get ask_waiting() { return [t('requests.moment.ask_waiting.title'), t('requests.moment.ask_waiting.body')] as [string, string]; },
	get ask_approved() { return [t('requests.moment.ask_approved.title'), t('requests.moment.ask_approved.body')] as [string, string]; },
	get ask_turned_down() { return [t('requests.moment.ask_turned_down.title'), t('requests.moment.ask_turned_down.body')] as [string, string]; },
	get ask_ready() { return [t('requests.moment.ask_ready.title'), t('requests.moment.ask_ready.body')] as [string, string]; },
	get new_episodes() { return [t('requests.moment.new_episodes.title'), t('requests.moment.new_episodes.body')] as [string, string]; },
	get new_albums() { return [t('requests.moment.new_albums.title'), t('requests.moment.new_albums.body')] as [string, string]; },
	get report_made() { return [t('requests.moment.report_made.title'), t('requests.moment.report_made.body')] as [string, string]; },
	get report_fixed() { return [t('requests.moment.report_fixed.title'), t('requests.moment.report_fixed.body')] as [string, string]; },
	get grab() { return [t('requests.moment.grab.title'), t('requests.moment.grab.body')] as [string, string]; },
	get import() { return [t('requests.moment.import.title'), t('requests.moment.import.body')] as [string, string]; },
	get rename() { return [t('requests.moment.rename.title'), t('requests.moment.rename.body')] as [string, string]; },
	get add() { return [t('requests.moment.add.title'), t('requests.moment.add.body')] as [string, string]; },
	get delete() { return [t('requests.moment.delete.title'), t('requests.moment.delete.body')] as [string, string]; },
	get health() { return [t('requests.moment.health.title'), t('requests.moment.health.body')] as [string, string]; },
	get needs_decision() { return [t('requests.moment.needs_decision.title'), t('requests.moment.needs_decision.body')] as [string, string]; },
};

/** Go to the people who asked (and the staff for a new ask). */
export const askMoments = ['ask_waiting', 'ask_approved', 'ask_turned_down', 'ask_ready', 'new_episodes', 'new_albums', 'report_made', 'report_fixed'] as const;

/** Go to the staff. */
export const libraryMoments = ['grab', 'import', 'rename', 'add', 'delete', 'health', 'needs_decision'] as const;

export const askTabs: { tab: AskTab; label: string }[] = [
	{ tab: 'waiting', get label() { return t('requests.tab.ask.waiting'); } },
	{ tab: 'approved', get label() { return t('requests.tab.ask.approved'); } },
	{ tab: 'declined', get label() { return t('requests.tab.ask.declined'); } },
	{ tab: 'done', get label() { return t('requests.tab.ask.done'); } },
	{ tab: 'failed', get label() { return t('requests.tab.ask.failed'); } }
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
	{ tab: 'pending', get label() { return t('requests.tab.approver.pending'); } },
	{ tab: 'processing', get label() { return t('requests.tab.approver.processing'); } },
	{ tab: 'done', get label() { return t('requests.tab.approver.done'); } }
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
