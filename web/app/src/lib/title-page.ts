// Pure helpers for the title page hero and the Similar shelf.
import type { StatusKind } from './ui/StatusPill.svelte';
import type { SlotView, TitleView } from './types.ts';

/** Where a title kind lives, for the hero breadcrumb. */
export function crumbFor(kind: string): { label: string; path: string } {
	if (kind === 'show') return { label: 'TV shows', path: '/library?kind=show' };
	if (kind === 'artist') return { label: 'Music', path: '/library?kind=artist' };
	return { label: 'Movies', path: '/library?kind=movie' };
}

type FileLike = { Quality?: Record<string, unknown> | null; MediaInfo?: Record<string, unknown> | null } | null | undefined;

const text = (v: unknown) => (typeof v === 'string' ? v.toLowerCase() : '');

/** Quality badges from the kept files: 4K, HDR, Dolby Vision, Atmos, CC. Only what the file data says. */
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

/** The hero status: downloading wins, then in library, then asked, then missing. */
export function heroStatus(slots: SlotView[], asked = false): { kind: StatusKind; percent?: number } {
	const dl = slots.find((s) => s.download?.active);
	if (dl?.download) return { kind: 'downloading', percent: Math.round((dl.download.progress ?? 0) * 100) };
	if (slots.some((s) => s.state === 'in_library' || s.state === 'better_wanted' || s.file)) return { kind: 'in_library' };
	if (slots.some((s) => s.state === 'failed')) return { kind: 'failed' };
	if (slots.some((s) => s.state === 'not_out_yet' || s.state === 'waiting')) return { kind: 'coming_soon' };
	if (asked) return { kind: 'requested' };
	return { kind: 'missing' };
}

type Similar = Pick<TitleView, 'id' | 'kind' | 'name' | 'year' | 'original_language'> & { genres?: string[]; people?: string[] };

/**
 * Titles in the library that are like this one: same kind first, scored by
 * shared genres (3 each) and shared people (2 each) when the data has them,
 * then same original language (1) and a year within 5 (1). Ties keep the
 * newest first. Titles with no score are left out.
 */
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

/** The view-transition-name shared by a poster and this title's hero art. */
export const artName = (id: number) => `title-art-${id}`;
