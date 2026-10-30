// An ask's life cycle as one slim line: how far it has come from asked to in
// the Library. Each stage owns a weight; while an ask is being got, real
// download progress (0..1) fills the span between getting and getting_end.
import type { Ask } from '../types.ts';

export const STAGE_WEIGHT = { asked: 8, approved: 30, getting: 45, getting_end: 92, ready: 100 } as const;

export type Stage = 'asked' | 'getting' | 'ready' | 'declined' | 'failed';
export interface Life {
	stage: Stage;
	/** 0..100, how much of the line is filled. */
	pct: number;
	/** wait (amber), go (accent), ok (green), off (dim), bad (red). */
	tone: 'wait' | 'go' | 'ok' | 'off' | 'bad';
	label: string;
}

/** lifecycle places an ask on its line; `progress` is the download's 0..1 when known. */
export function lifecycle(a: Pick<Ask, 'state' | 'title_id'>, progress?: number): Life {
	if (a.state === 'asked') return { stage: 'asked', pct: STAGE_WEIGHT.asked, tone: 'wait', label: 'Waiting for you' };
	if (a.state === 'turned_down') return { stage: 'declined', pct: 100, tone: 'off', label: 'Turned down' };
	if (a.state === 'available') return { stage: 'ready', pct: 100, tone: 'ok', label: 'In the Library' };
	if (!a.title_id) return { stage: 'failed', pct: STAGE_WEIGHT.approved, tone: 'bad', label: 'Stuck, needs a look' };
	const lo = STAGE_WEIGHT.getting;
	const hi = STAGE_WEIGHT.getting_end;
	const p = progress === undefined || !Number.isFinite(progress) ? 0 : Math.min(Math.max(progress, 0), 1);
	return { stage: 'getting', pct: lo + (hi - lo) * p, tone: 'go', label: p > 0 ? `Downloading · ${Math.round(p * 100)}%` : 'Approved · looking for it' };
}
