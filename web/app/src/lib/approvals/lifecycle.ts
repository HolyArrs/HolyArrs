// An ask's life cycle as one slim line: how far it has come from asked to in
// the Library. Each stage owns a weight; while an ask is being got, real
// download progress (0..1) fills the span between getting and getting_end.
import type { Ask } from '../types.ts';
import type { AskInfo } from './inbox.ts';

export const STAGE_WEIGHT = { asked: 8, approved: 30, getting: 45, getting_end: 92, ready: 100 } as const;

export type Stage = 'asked' | 'upcoming' | 'searching' | 'downloading' | 'importing' | 'getting' | 'ready' | 'declined' | 'failed';
export interface Life {
	stage: Stage;
	/** 0..100, how much of the line is filled. */
	pct: number;
	/** wait (amber), soon (muted), go (accent), ok (green), off (dim), bad (red). */
	tone: 'wait' | 'soon' | 'go' | 'ok' | 'off' | 'bad';
	label: string;
	/** Ready with more of the title not asked for: "2 more seasons available". */
	more?: string;
	/** Why it failed, when known (said in the window, not in the label). */
	reason?: string;
}

/** What else is known about an approved ask; each part is left out when unknown. */
export interface LifeFacts {
	/** The download's 0..1. */
	progress?: number;
	/** What the pipeline is doing for it right now (from the live searches and downloads). */
	phase?: 'searching' | 'downloading' | 'importing' | 'stuck' | 'failed';
	/** Not released yet: the short date it comes out ('' when the date is unknown). */
	upcoming?: string;
	/**
	 * How many of the seasons or albums the ask covers (as approved, never the whole
	 * catalogue) are in the Library. `unit` adds the word: "Processing · 1 season left".
	 */
	partial?: { have: number; total: number; unit?: 'seasons' | 'albums' };
	/** Seasons or albums the title has beyond what was asked (shown once Ready). */
	more?: number;
	/** Episodes or songs left unticked inside the asked seasons or albums. */
	skipped?: number;
	/** Why it failed, when known. */
	reason?: string;
}

/** lifecycle places an ask on its line; `facts` adds what is known beyond its state. */
export function lifecycle(a: Pick<Ask, 'state' | 'title_id'>, facts: LifeFacts = {}): Life {
	if (a.state === 'asked') return { stage: 'asked', pct: STAGE_WEIGHT.asked, tone: 'wait', label: 'Pending' };
	if (a.state === 'turned_down') return { stage: 'declined', pct: 100, tone: 'off', label: 'Declined' };
	if (a.state === 'available') return ready(facts);
	if (!a.title_id) return failedLife(facts);
	const lo = STAGE_WEIGHT.getting;
	const hi = STAGE_WEIGHT.getting_end;
	const part = facts.partial;
	if (facts.phase === 'failed') return failedLife(facts);
	if (facts.phase === 'searching') return { stage: 'searching', pct: lo + (hi - lo) * 0.1, tone: 'go', label: 'Searching' };
	// Downloaded, but the import waits on a person: say so instead of a plain Processing.
	if (facts.phase === 'stuck') return { stage: 'getting', pct: hi - 3, tone: 'wait', label: 'Needs a decision' };
	if (facts.phase === 'importing') return { stage: 'importing', pct: hi - 3, tone: 'go', label: 'Importing' };
	if (facts.phase === 'downloading') {
		const d = facts.progress === undefined || !Number.isFinite(facts.progress) ? 0 : Math.min(Math.max(facts.progress, 0), 1);
		return { stage: 'downloading', pct: lo + (hi - lo) * (0.15 + 0.75 * d), tone: 'go', label: `Downloading · ${Math.round(d * 100)}%` };
	}
	// Everything asked for is in: Ready, however much more the title has.
	if (part && part.total > 0 && part.have >= part.total) return ready(facts);
	if (part && part.total > 0 && part.have > 0) {
		const left = part.total - part.have;
		const unit = part.unit ? ` ${left === 1 ? part.unit.slice(0, -1) : part.unit}` : '';
		return { stage: 'getting', pct: lo + (hi - lo) * (part.have / part.total), tone: 'go', label: `Processing · ${left}${unit} left` };
	}
	if (facts.upcoming !== undefined) return { stage: 'upcoming', pct: STAGE_WEIGHT.approved, tone: 'soon', label: facts.upcoming ? `Upcoming · ${facts.upcoming}` : 'Upcoming' };
	// No live phase: the line still fills, but the words stay plain.
	const progress = facts.progress;
	const p = progress === undefined || !Number.isFinite(progress) ? 0 : Math.min(Math.max(progress, 0), 1);
	return { stage: 'getting', pct: lo + (hi - lo) * p, tone: 'go', label: 'Processing' };
}

function failedLife(f: LifeFacts): Life {
	const life: Life = { stage: 'failed', pct: STAGE_WEIGHT.approved, tone: 'bad', label: 'Failed' };
	if (f.reason) life.reason = f.reason;
	return life;
}

const word = (n: number, unit: string) => `${n} ${n === 1 ? unit.slice(0, -1) : unit}`;

/** Ready names what came in ("Ready · 2 albums") and what's left to request. */
function ready(f: LifeFacts): Life {
	const part = f.partial;
	const life: Life = { stage: 'ready', pct: 100, tone: 'ok', label: part?.unit && part.total > 0 ? `Ready · ${word(part.total, part.unit)}` : 'Ready' };
	// Whole seasons or albums not asked for come first; otherwise unticked episodes or songs.
	// The unit stays when known: "2 more seasons available", "3 more episodes available".
	const one = (n: number, unit: string) => (n === 1 ? unit.slice(0, -1) : unit);
	if (f.more && f.more > 0) life.more = part?.unit ? `${f.more} more ${one(f.more, part.unit)} available` : `${f.more} more available`;
	else if (f.skipped && f.skipped > 0 && part?.unit) life.more = `${f.skipped} more ${one(f.skipped, part.unit === 'seasons' ? 'episodes' : 'songs')} available`;
	return life;
}

const day = (iso: string) => iso.slice(0, 10);

/**
 * upcoming says when an approved ask's title comes out, as an ISO day, when it
 * is not out yet: a movie with no release on or before today; a show whose
 * every asked season first airs after today (the whole show: its first air
 * day). '' when it is out, or when the dates aren't known (never guessed).
 * `firstAir` gives a season's first air day: null while unknown.
 */
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

