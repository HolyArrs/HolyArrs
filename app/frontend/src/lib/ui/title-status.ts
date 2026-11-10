// Turns title facts into the status kind and label shown on pills.
import type { StatusKind } from './StatusPill.svelte';
import type { AskState, SlotView } from '../core/types.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export type Audience = 'member' | 'staff';

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

// Members see friendlier words than staff.
const staffWords = (): Record<StatusKind, string> => ({
	in_library: t('ui.titleStatus.inLibrary'),
	downloading: t('ui.titleStatus.downloading'),
	missing: t('ui.titleStatus.missing'),
	requested: t('ui.titleStatus.waiting'),
	approved: t('ui.titleStatus.approved'),
	turned_down: t('ui.titleStatus.declined'),
	failed: t('ui.titleStatus.failed'),
	coming_soon: t('ui.titleStatus.comingSoon')
});
const memberWords = (): Record<StatusKind, string> => ({
	...staffWords(),
	in_library: t('ui.titleStatus.readyToWatch'),
	missing: t('ui.titleStatus.getIt'),
	requested: t('ui.titleStatus.requested')
});

const askKind: Record<AskState, StatusKind> = { asked: 'requested', approved: 'approved', turned_down: 'turned_down', available: 'in_library' };

const haveStates = new Set(['in_library', 'better_wanted', 'no_hardlink']);
// Higher wins when a title has several asks.
const askRank: Record<AskState, number> = { available: 3, approved: 2, asked: 1, turned_down: 0 };

export function audienceOf(can: (perm: 'manage_library' | 'approve') => boolean): Audience {
	return can('manage_library') || can('approve') ? 'staff' : 'member';
}

export function statusLabel(kind: StatusKind, who: Audience, opts: { percent?: number; titleKind?: StatusFacts['titleKind'] } = {}): string {
	if (kind === 'downloading' && opts.percent !== undefined) return t('ui.titleStatus.downloadingPercent', { percent: Math.round(opts.percent) });
	if (kind === 'in_library' && who === 'member' && opts.titleKind === 'artist') return t('ui.titleStatus.readyToListen');
	return (who === 'member' ? memberWords() : staffWords())[kind];
}

// Members never see raw failure or missing when they have already asked.
export function audienceStatus(kind: StatusKind, who: Audience, ctx: Pick<StatusFacts, 'tracked' | 'titleKind' | 'percent' | 'ask'> = {}): TitleStatus {
	let k = kind;
	if (who === 'member') {
		if (k === 'failed') k = ctx.ask ? askKind[ctx.ask] : ctx.tracked ? 'requested' : 'missing';
		else if (k === 'missing' && ctx.tracked) k = 'requested';
	}
	return { kind: k, label: statusLabel(k, who, ctx) };
}

// Order matters: active download beats library, then failure, ask, coming soon.
export function titleStatus(f: StatusFacts, who: Audience): TitleStatus {
	if (!f.downloading && !f.inLibrary && !f.failed && !f.ask && f.neverAdd) return { kind: 'turned_down', label: t('ui.titleStatus.neverAdd') };
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
	return f.mine && (st.kind === 'requested' || st.kind === 'approved') && f.ask ? { ...st, label: t('ui.titleStatus.youAsked', { status: st.label }) } : st;
}

// Collapses slots and asks into facts, keeping the best ask.
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
