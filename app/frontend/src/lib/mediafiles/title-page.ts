// Title page helpers: breadcrumb, quality badges, similar titles and slot status.
import type { StatusKind } from '#lib/ui/StatusPill.svelte';
import type { SlotView, TitleView } from '../core/types.ts';
import { t } from '../core/i18n/i18n.svelte.ts';

type FileLike = { Quality?: Record<string, unknown> | null; MediaInfo?: Record<string, unknown> | null } | null | undefined;
type Similar = Pick<TitleView, 'id' | 'kind' | 'name' | 'year' | 'original_language'> & { genres?: string[]; people?: string[] };

const text = (v: unknown) => (typeof v === 'string' ? v.toLowerCase() : '');

export const artName = (id: number) => `title-art-${id}`;

export function crumbFor(kind: string): { label: string; path: string } {
	if (kind === 'show') return { label: t('mediafiles.kinds.show'), path: '/library?kind=show' };
	if (kind === 'artist') return { label: t('mediafiles.kinds.artist'), path: '/library?kind=artist' };
	return { label: t('mediafiles.kinds.movie'), path: '/library?kind=movie' };
}

// Badges come from file quality and media info; 4K hides HD, and Dolby Vision is not also counted as HDR.
export function qualityBadges(files: FileLike[]): string[] {
	const out = new Set<string>();
	for (const f of files) {
		if (!f) continue;
		const q = f.Quality ?? {};
		const m = f.MediaInfo ?? {};
		const res = Number(q.resolution ?? 0);
		if (res >= 2160) out.add('4K');
		else if (res >= 1080) out.add('HD');
		const hdr = `${text(q.hdr)} ${text(m.hdr)} ${text(m.video_dynamic_range)} ${text(m.hdr_format)}`;
		if (/dolby ?vision|\bdv\b|dovi/.test(hdr)) out.add('Dolby Vision');
		if (/hdr|hlg|pq/.test(hdr.replace(/dolby ?vision|dovi/g, ''))) out.add('HDR');
		const audio = `${text(q.audio)} ${text(m.audio_codec)} ${text(m.audio_format)}`;
		if (/atmos/.test(audio)) out.add('Atmos');
		const subs = m.subtitle_languages ?? m.subtitles;
		if (Array.isArray(subs) && subs.length) out.add('CC');
	}
	const order = ['4K', 'HD', 'Dolby Vision', 'HDR', 'Atmos', 'CC'];
	const list = order.filter((b) => out.has(b));
	return list.includes('4K') ? list.filter((b) => b !== 'HD') : list;
}

// Scores shared genres (3), shared people (2), same language (1) and a year within 5 (1).
export function similarTitles<T extends Similar>(current: Similar, all: T[], limit = 12): T[] {
	const g = new Set(current.genres ?? []);
	const p = new Set(current.people ?? []);
	const scored: { t: T; score: number }[] = [];
	for (const t of all) {
		if (t.id === current.id || t.kind !== current.kind) continue;
		let score = 0;
		for (const x of t.genres ?? []) if (g.has(x)) score += 3;
		for (const x of t.people ?? []) if (p.has(x)) score += 2;
		if (current.original_language && t.original_language === current.original_language) score += 1;
		if (current.year && t.year && Math.abs(current.year - t.year) <= 5) score += 1;
		if (score > 0) scored.push({ t, score });
	}
	scored.sort((a, b) => b.score - a.score || (b.t.year ?? 0) - (a.t.year ?? 0) || a.t.name.localeCompare(b.t.name));
	return scored.slice(0, limit).map((s) => s.t);
}

// Folds the many slot states into the few the status pill can show.
export function slotKind(state: string | undefined): StatusKind {
	switch (state) {
		case 'in_library':
		case 'better_wanted':
		case 'no_hardlink':
			return 'in_library';
		case 'downloading':
		case 'importing':
			return 'downloading';
		case 'failed':
			return 'failed';
		case 'not_out_yet':
			return 'coming_soon';
		default:
			return 'missing';
	}
}
