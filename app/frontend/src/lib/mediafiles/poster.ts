import type { LibState, LibStateKey } from './library-state.ts';

export const attentionKeys: readonly LibStateKey[] = [
	'failed', 'needs_decision', 'stalled', 'missing_file', 'missing', 'better_wanted',
	'paused', 'downloading', 'queued', 'waiting_approval', 'not_out_yet'
];

export function needsAttention(s: LibState | null | undefined): boolean {
	return !!s && attentionKeys.includes(s.key);
}

export type BarTone = 'ok' | 'busy' | 'gap' | 'fail' | 'idle';

const clamp01 = (n: number) => (Number.isFinite(n) ? Math.min(Math.max(n, 0), 1) : 0);

export function barOf(s: LibState | null | undefined, have: number, total: number): { tone: BarTone; fill: number } {
	const share = total > 0 ? clamp01(have / total) : 0;
	if (!s) return { tone: 'ok', fill: total > 0 ? share : 1 };
	switch (s.key) {
		case 'failed': case 'needs_decision': case 'stalled': case 'missing_file':
			return { tone: 'fail', fill: s.percent !== undefined ? clamp01(s.percent / 100) : 1 };
		case 'downloading': case 'queued': case 'paused':
			return { tone: 'busy', fill: s.percent !== undefined ? clamp01(s.percent / 100) : share };
		case 'missing': case 'better_wanted':
			return { tone: 'gap', fill: total > 0 ? share : 1 };
		case 'not_out_yet': case 'waiting_approval': case 'approved': case 'declined': case 'not_kept':
			return { tone: 'idle', fill: total > 0 ? share : 1 };
		default:
			return { tone: 'ok', fill: 1 };
	}
}

export function tiltFor(x: number, y: number, w: number, h: number, max = 4): { rx: number; ry: number } {
	if (!(w > 0) || !(h > 0)) return { rx: 0, ry: 0 };
	const nx = Math.min(Math.max(x / w, 0), 1) * 2 - 1;
	const ny = Math.min(Math.max(y / h, 0), 1) * 2 - 1;
	const r = (n: number) => Math.round(n * max * 100) / 100 || 0;
	return { rx: r(-ny), ry: r(nx) };
}

export type PosterDetail = 'title' | 'status' | 'full';
export const detailLabels: Record<PosterDetail, string> = { title: 'Title only', status: 'Title and problems', full: 'Everything' };

type Store = Pick<Storage, 'getItem' | 'setItem'> | undefined;
const detailKey = (member: number | undefined) => `holyarrs.poster-detail.${member ?? 'guest'}`;

export function loadPosterDetail(member: number | undefined, store: Store = globalThis.localStorage): PosterDetail {
	try {
		const v = store?.getItem(detailKey(member));
		return v === 'title' || v === 'full' ? v : 'status';
	} catch {
		return 'status';
	}
}
export function savePosterDetail(member: number | undefined, d: PosterDetail, store: Store = globalThis.localStorage) {
	try {
		store?.setItem(detailKey(member), d);
	} catch {
		// Storage can be blocked (private window); nothing to do.
	}
}

export function flipFrom(a: { left: number; top: number; width: number }, b: { left: number; top: number; width: number }): string | null {
	const dx = a.left - b.left;
	const dy = a.top - b.top;
	const s = b.width > 0 ? a.width / b.width : 1;
	if (Math.abs(dx) < 0.5 && Math.abs(dy) < 0.5 && Math.abs(s - 1) < 0.005) return null;
	return `translate(${Math.round(dx)}px, ${Math.round(dy)}px) scale(${Math.round(s * 1000) / 1000})`;
}
