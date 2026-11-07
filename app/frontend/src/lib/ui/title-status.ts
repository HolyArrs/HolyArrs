import type { StatusKind } from './StatusPill.svelte';
import type { AskState, SlotView } from '../core/types.ts';

export type Audience = 'member' | 'staff';

export function audienceOf(can: (perm: 'manage_library' | 'approve') => boolean): Audience {
	return can('manage_library') || can('approve') ? 'staff' : 'member';
}

export interface StatusFacts {
	titleKind?: 'movie' | 'show' | 'artist' | 'episode';
	inLibrary?: boolean;
	downloading?: boolean;
	percent?: number;
	failed?: boolean;
	comingSoon?: boolean;
	ask?: AskState | null;
	neverAdd?: boolean;
	tracked?: boolean;
	mine?: boolean;
}

export interface TitleStatus {
	kind: StatusKind;
	label: string;
}

const staffWords: Record<StatusKind, string> = {
	in_library: 'In your Library',
	downloading: 'Downloading',
	missing: 'Missing',
	requested: 'Waiting',
	approved: 'Approved',
	turned_down: 'Declined',
	failed: 'Failed',
	coming_soon: 'Coming soon'
};
const memberWords: Record<StatusKind, string> = {
	...staffWords,
	in_library: 'Ready to watch',
	missing: 'Get it',
	requested: 'Requested'
};

export function statusLabel(kind: StatusKind, who: Audience, opts: { percent?: number; titleKind?: StatusFacts['titleKind'] } = {}): string {
	if (kind === 'downloading' && opts.percent !== undefined) return `Downloading ${Math.round(opts.percent)}%`;
	if (kind === 'in_library' && who === 'member' && opts.titleKind === 'artist') return 'Ready to listen';
	return (who === 'member' ? memberWords : staffWords)[kind];
}

const askKind: Record<AskState, StatusKind> = { asked: 'requested', approved: 'approved', turned_down: 'turned_down', available: 'in_library' };

export function audienceStatus(kind: StatusKind, who: Audience, ctx: Pick<StatusFacts, 'tracked' | 'titleKind' | 'percent' | 'ask'> = {}): TitleStatus {
	let k = kind;
	if (who === 'member') {
		if (k === 'failed') k = ctx.ask ? askKind[ctx.ask] : ctx.tracked ? 'requested' : 'missing';
		else if (k === 'missing' && ctx.tracked) k = 'requested';
	}
	return { kind: k, label: statusLabel(k, who, ctx) };
}

export function titleStatus(f: StatusFacts, who: Audience): TitleStatus {
	if (!f.downloading && !f.inLibrary && !f.failed && !f.ask && f.neverAdd) return { kind: 'turned_down', label: 'On the Never-add list' };
	const kind: StatusKind = f.downloading
		? 'downloading'
		: f.inLibrary
			? 'in_library'
			: f.failed
				? 'failed'
				: f.ask
					? askKind[f.ask]
					: f.comingSoon
						? 'coming_soon'
						: 'missing';
	const st = audienceStatus(kind, who, f);
	return f.mine && (st.kind === 'requested' || st.kind === 'approved') && f.ask ? { ...st, label: `You asked · ${st.label}` } : st;
}

const haveStates = new Set(['in_library', 'better_wanted', 'no_hardlink']);
const askRank: Record<AskState, number> = { available: 3, approved: 2, asked: 1, turned_down: 0 };

export function factsOf(t: { slots?: SlotView[] | null; asks?: { state: AskState }[] | null }): StatusFacts {
	const slots = t.slots ?? [];
	const dl = slots.find((s) => s.download?.active || s.state === 'downloading' || s.state === 'importing');
	const ask = (t.asks ?? []).reduce<AskState | null>((best, a) => (best === null || askRank[a.state] > askRank[best] ? a.state : best), null);
	return {
		tracked: true,
		inLibrary: slots.some((s) => haveStates.has(s.state) || !!s.file),
		downloading: !!dl,
		percent: dl?.download?.active ? Math.round((dl.download.progress ?? 0) * 100) : undefined,
		failed: slots.some((s) => s.state === 'failed'),
		comingSoon: slots.some((s) => s.state === 'not_out_yet' || s.state === 'waiting'),
		ask
	};
}
