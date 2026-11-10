// An ask's life cycle as one slim line: how far it has come from asked to in the Library. Each stage owns a weight; while an ask is being got, real download progress (0 to 1) fills the span between getting and getting_end.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { AskInfo } from './inbox.ts';
import type { Ask } from '../core/types.ts';

export type Stage = 'asked' | 'upcoming' | 'searching' | 'downloading' | 'importing' | 'getting' | 'ready' | 'declined' | 'failed';

export interface Life {
	stage: Stage;
	/** 0 to 100: how much of the line is filled. */
	pct: number;
	/** wait (amber), soon (muted), go (accent), ok (green), off (dim), bad (red). */
	tone: 'wait' | 'soon' | 'go' | 'ok' | 'off' | 'bad';
	label: string;
	/** The status word in plain English lower case, a stable key for icons (the label is in the person's language). */
	status: string;
	/** Ready with more of the title not asked for: "2 more seasons available". */
	more?: string;
	/** Why it failed, when known (said in the window, not in the label). */
	reason?: string;
	/** Waiting for a release: when it was last looked for. */
	searchedAt?: string;
	/** Not looked for yet: when the search for missing looks next. */
	nextSearch?: string;
}

/** What else is known about an approved ask; each part is left out when unknown. */
export interface LifeFacts {
	/** The download row behind it, for a link to Downloads. */
	downloadId?: number;
	/** The download's progress, 0 to 1. */
	progress?: number;
	/** What the pipeline is doing for it right now (from the live searches and downloads). */
	phase?: 'searching' | 'downloading' | 'importing' | 'stuck' | 'failed' | 'waiting';
	/** Not released yet: the short date it comes out ('' when the date is unknown). */
	upcoming?: string;
	/** How many of the seasons or albums the ask covers (as approved, never the whole catalogue) are in the Library. `unit` adds the word: "Processing · 1 season left". */
	partial?: { have: number; total: number; unit?: 'seasons' | 'albums' | 'episodes' | 'songs' };
	/** Seasons or albums the title has beyond what was asked (shown once Ready). */
	more?: number;
	/** Episodes or songs left unticked inside the asked seasons or albums. */
	skipped?: number;
	/** Why it failed (or, while waiting, why nothing came yet), when known. */
	reason?: string;
	/** Waiting: when it was last looked for. */
	searchedAt?: string;
	/** Not looked for yet: when the search for missing looks next. */
	nextSearch?: string;
}

export const STAGE_WEIGHT = { asked: 8, approved: 30, getting: 45, getting_end: 92, ready: 100 } as const;

/** Places an ask on its line; `facts` adds what is known beyond its state. */
export function lifecycle(a: Pick<Ask, 'state' | 'title_id'>, facts: LifeFacts = {}): Life {
	if (a.state === 'asked') return { stage: 'asked', pct: STAGE_WEIGHT.asked, tone: 'wait', status: 'waiting', label: t('requests.life.waiting') };
	if (a.state === 'turned_down') return { stage: 'declined', pct: 100, tone: 'off', status: 'declined', label: t('requests.life.declined') };
	if (a.state === 'available') return ready(facts);
	if (!a.title_id) return failedLife(facts);
	const lo = STAGE_WEIGHT.getting;
	const hi = STAGE_WEIGHT.getting_end;
	const part = facts.partial;
	if (facts.phase === 'failed') return failedLife(facts);
	if (facts.phase === 'searching') return { stage: 'searching', pct: lo + (hi - lo) * 0.1, tone: 'go', status: 'searching', label: t('requests.life.searching') };
	// Downloaded, but the import waits on a person: say so instead of a plain Processing.
	if (facts.phase === 'stuck') return { stage: 'getting', pct: hi - 3, tone: 'wait', status: 'needs a decision', label: t('requests.life.needsDecision') };
	if (facts.phase === 'importing') return { stage: 'importing', pct: hi - 3, tone: 'go', status: 'importing', label: t('requests.life.importing') };
	if (facts.phase === 'downloading') {
		// The server's step knows a download runs, not how far, so no made-up 0%.
		if (facts.progress === undefined || !Number.isFinite(facts.progress)) return { stage: 'downloading', pct: lo + (hi - lo) * 0.15, tone: 'go', status: 'downloading', label: t('requests.life.downloading') };
		const d = Math.min(Math.max(facts.progress, 0), 1);
		return { stage: 'downloading', pct: lo + (hi - lo) * (0.15 + 0.75 * d), tone: 'go', status: 'downloading', label: t('requests.life.downloadingPct', { pct: Math.round(d * 100) }) };
	}
	// Nothing in flight and nothing found yet: say so, with why and when it last looked.
	if (facts.phase === 'waiting') {
		const life: Life = { stage: 'getting', pct: lo, tone: 'wait', status: 'looking for a release', label: t('requests.life.looking') };
		if (facts.reason) life.reason = facts.reason;
		if (facts.searchedAt) life.searchedAt = facts.searchedAt;
		if (facts.nextSearch) life.nextSearch = facts.nextSearch;
		return life;
	}
	// Everything asked for is in: Ready, however much more the title has.
	if (part && part.total > 0 && part.have >= part.total) return ready(facts);
	if (part && part.total > 0 && part.have > 0) {
		const left = part.total - part.have;
		const label = part.unit ? t(`requests.life.left.${part.unit}.${left === 1 ? 'one' : 'other'}`, { n: left }) : t('requests.life.leftPlain', { n: left });
		return { stage: 'getting', pct: lo + (hi - lo) * (part.have / part.total), tone: 'go', status: 'processing', label };
	}
	if (facts.upcoming !== undefined) return { stage: 'upcoming', pct: STAGE_WEIGHT.approved, tone: 'soon', status: 'upcoming', label: facts.upcoming ? t('requests.life.upcomingOn', { date: facts.upcoming }) : t('requests.life.upcoming') };
	// No live phase: the line still fills, but the words stay plain.
	const progress = facts.progress;
	const p = progress === undefined || !Number.isFinite(progress) ? 0 : Math.min(Math.max(progress, 0), 1);
	return { stage: 'getting', pct: lo + (hi - lo) * p, tone: 'go', status: 'processing', label: t('requests.life.processing') };
}

const word = (n: number, unit: 'seasons' | 'albums' | 'episodes' | 'songs') => t(`requests.parts.${unit}.${n === 1 ? 'one' : 'other'}`, { n });

const day = (iso: string) => iso.slice(0, 10);

/** When an approved ask's title comes out, as an ISO day, when it is not out yet: a movie with no release on or before today; a show whose every asked season first airs after today. */
export function upcoming(
	a: Pick<Ask, 'kind' | 'seasons' | 'future_only'>,
	info: Pick<AskInfo, 'dates' | 'status'> | null | undefined,
	firstAir: (season: number) => string | null,
	now = new Date()
): string {
	if (!info || a.future_only) return '';
	// The local day, not UTC: late evening must not count as tomorrow.
	const today = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-${String(now.getDate()).padStart(2, '0')}`;
	const later = (ds: (string | null | undefined)[]) => {
		const d = ds.map((x) => (x ? day(x) : ''));
		return d.length && d.every((x) => x && x > today) ? d.sort()[0] : '';
	};
	if (a.kind === 'show') {
		const s = a.seasons ?? [];
		return s.length ? later(s.map(firstAir)) : later([info.dates.first_air]);
	}
	if (a.kind === 'artist' || info.status === 'Released') return '';
	const ds = [info.dates.theaters, info.dates.digital, info.dates.physical].filter(Boolean);
	return ds.length ? later(ds) : '';
}

function failedLife(f: LifeFacts): Life {
	const life: Life = { stage: 'failed', pct: STAGE_WEIGHT.approved, tone: 'bad', status: 'failed', label: t('requests.life.failed') };
	if (f.reason) life.reason = f.reason;
	return life;
}

/** Ready names what came in ("Ready · 2 albums") and what is left to request. */
function ready(f: LifeFacts): Life {
	const part = f.partial;
	const life: Life = { stage: 'ready', pct: 100, tone: 'ok', status: 'ready', label: part?.unit && part.total > 0 ? t('requests.life.readyWith', { what: word(part.total, part.unit) }) : t('requests.life.ready') };
	// Whole seasons or albums not asked for come first; otherwise unticked episodes or songs. The unit stays when known: "2 more seasons available", "3 more episodes available".
	// "More" counts whole seasons or albums, also when only episodes or songs were asked.
	const whole = part?.unit === 'episodes' ? 'seasons' : part?.unit === 'songs' ? 'albums' : part?.unit;
	const inside = whole === 'seasons' ? 'episodes' : 'songs';
	if (f.more && f.more > 0) life.more = whole ? t(`requests.life.more.${whole}.${f.more === 1 ? 'one' : 'other'}`, { n: f.more }) : t('requests.life.morePlain', { n: f.more });
	else if (f.skipped && f.skipped > 0 && whole) life.more = t(`requests.life.more.${inside}.${f.skipped === 1 ? 'one' : 'other'}`, { n: f.skipped });
	return life;
}
