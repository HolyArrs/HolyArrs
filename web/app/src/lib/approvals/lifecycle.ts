// An ask's life cycle as one slim line: how far it has come from asked to in
// the Library. Each stage owns a weight; while an ask is being got, real
// download progress (0..1) fills the span between getting and getting_end.
import type { Ask } from '../types.ts';
import type { AskInfo } from './inbox.ts';

export const STAGE_WEIGHT = { asked: 8, approved: 30, getting: 45, getting_end: 92, ready: 100 } as const;

export type Stage = 'asked' | 'upcoming' | 'getting' | 'ready' | 'declined' | 'failed';
export interface Life {
	stage: Stage;
	/** 0..100, how much of the line is filled. */
	pct: number;
	/** wait (amber), soon (muted), go (accent), ok (green), off (dim), bad (red). */
	tone: 'wait' | 'soon' | 'go' | 'ok' | 'off' | 'bad';
	label: string;
	/** Ready with more of the title not asked for: "5 more available". */
	more?: string;
}

/** What else is known about an approved ask; each part is left out when unknown. */
export interface LifeFacts {
	/** The download's 0..1. */
	progress?: number;
	/** Not released yet: the short date it comes out ('' when the date is unknown). */
	upcoming?: string;
	/**
	 * How many of the seasons or albums the ask covers (as approved, never the whole
	 * catalogue) are in the Library. `unit` adds the word: "Processing · 1 season left".
	 */
	partial?: { have: number; total: number; unit?: 'seasons' | 'albums' };
	/** Seasons or albums the title has beyond what was asked (shown once Ready). */
	more?: number;
	/** Why it failed, when known. */
	reason?: string;
}

/** lifecycle places an ask on its line; `facts` adds what is known beyond its state. */
export function lifecycle(a: Pick<Ask, 'state' | 'title_id'>, facts: LifeFacts = {}): Life {
	if (a.state === 'asked') return { stage: 'asked', pct: STAGE_WEIGHT.asked, tone: 'wait', label: 'Pending' };
	if (a.state === 'turned_down') return { stage: 'declined', pct: 100, tone: 'off', label: 'Declined' };
	if (a.state === 'available') return ready(facts);
	if (!a.title_id) return { stage: 'failed', pct: STAGE_WEIGHT.approved, tone: 'bad', label: facts.reason ? `Failed · ${facts.reason}` : 'Failed' };
	const lo = STAGE_WEIGHT.getting;
	const hi = STAGE_WEIGHT.getting_end;
	const part = facts.partial;
	// Everything asked for is in: Ready, however much more the title has.
	if (part && part.total > 0 && part.have >= part.total) return ready(facts);
	if (part && part.total > 0 && part.have > 0) {
		const left = part.total - part.have;
		const unit = part.unit ? ` ${left === 1 ? part.unit.slice(0, -1) : part.unit}` : '';
		return { stage: 'getting', pct: lo + (hi - lo) * (part.have / part.total), tone: 'go', label: `Processing · ${left}${unit} left` };
	}
	if (facts.upcoming !== undefined) return { stage: 'upcoming', pct: STAGE_WEIGHT.approved, tone: 'soon', label: facts.upcoming ? `Upcoming · out ${facts.upcoming}` : 'Upcoming' };
	const progress = facts.progress;
	const p = progress === undefined || !Number.isFinite(progress) ? 0 : Math.min(Math.max(progress, 0), 1);
	return { stage: 'getting', pct: lo + (hi - lo) * p, tone: 'go', label: p > 0 ? `Processing · ${Math.round(p * 100)}%` : 'Processing' };
}

const word = (n: number, unit: 'seasons' | 'albums') => `${n} ${n === 1 ? unit.slice(0, -1) : unit}`;

/** Ready names what came in ("Ready · 2 albums") and what's left to request. */
function ready(f: LifeFacts): Life {
	const part = f.partial;
	const life: Life = { stage: 'ready', pct: 100, tone: 'ok', label: part?.unit && part.total > 0 ? `Ready · ${word(part.total, part.unit)}` : 'Ready' };
	if (f.more && f.more > 0) life.more = `${f.more} more available`;
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
	const today = now.toISOString().slice(0, 10);
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

