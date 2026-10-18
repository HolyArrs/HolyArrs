// One status vocabulary for a title's ask and Library state, used by every
// poster, title hero, Approvals row, search result and Explore card. Members
// get the plain words (spec section 7); staff get more precise ones.
import type { StatusKind } from './ui/StatusPill.svelte';
import type { AskState, SlotView } from './types.ts';

export type Audience = 'member' | 'staff';

/** audienceOf: people who manage the Library or approve asks get the staff words. */
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
	/** The state of the title's ask, if anyone asked. */
	ask?: AskState | null;
	neverAdd?: boolean;
	/** The title is in the Library list, with or without a file yet. */
	tracked?: boolean;
}

export interface TitleStatus {
	kind: StatusKind;
	label: string;
}

const staffWords: Record<StatusKind, string> = {
	in_library: 'In your Library',
	downloading: 'Downloading',
	missing: 'Missing',
	requested: 'Waiting for approval',
	approved: 'Approved',
	turned_down: 'Turned down',
	failed: 'Failed',
	coming_soon: 'Coming soon'
};
const memberWords: Record<StatusKind, string> = {
	...staffWords,
	in_library: 'Ready to watch',
	missing: 'Get it',
	requested: 'Requested'
};

/** statusLabel is the word for one status kind, for members or staff. */
export function statusLabel(kind: StatusKind, who: Audience, opts: { percent?: number; titleKind?: StatusFacts['titleKind'] } = {}): string {
	if (kind === 'downloading' && opts.percent !== undefined) return `Downloading ${Math.round(opts.percent)}%`;
	if (kind === 'in_library' && who === 'member' && opts.titleKind === 'artist') return 'Ready to listen';
	return (who === 'member' ? memberWords : staffWords)[kind];
}

const askKind: Record<AskState, StatusKind> = { asked: 'requested', approved: 'approved', turned_down: 'turned_down', available: 'in_library' };

/**
 * audienceStatus says one status kind in the words for members or staff.
 * Members never see a failed download (they see their ask, or that the
 * title is on its way), and a Library title still being looked for is
 * "Requested" for them, never "Get it".
 */
export function audienceStatus(kind: StatusKind, who: Audience, ctx: Pick<StatusFacts, 'tracked' | 'titleKind' | 'percent' | 'ask'> = {}): TitleStatus {
	let k = kind;
	if (who === 'member') {
		if (k === 'failed') k = ctx.ask ? askKind[ctx.ask] : ctx.tracked ? 'requested' : 'missing';
		else if (k === 'missing' && ctx.tracked) k = 'requested';
	}
	return { kind: k, label: statusLabel(k, who, ctx) };
}

/**
 * titleStatus picks the one status a title shows: downloading, then in the
 * Library, then a failed download, then its ask, then the Never-add list,
 * then coming soon, else missing; then says it for the audience.
 */
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
	return audienceStatus(kind, who, f);
}

const haveStates = new Set(['in_library', 'better_wanted', 'no_hardlink']);
const askRank: Record<AskState, number> = { available: 3, approved: 2, asked: 1, turned_down: 0 };

/** factsOf reads a Library title's slots and asks (so it is tracked). */
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
