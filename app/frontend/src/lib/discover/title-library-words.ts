import { type SheetEpisode } from '#lib/shows/index.ts';
import { bytes } from '#lib/core/format.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

// "Yesterday", "Tomorrow", "Sat, Oct 31" this year, else the date with its year.
export function dayWords(iso: string): string {
	const d = new Date(iso);
	// A bare day ("2026-11-15") is the same day everywhere; a full time is the viewer's own day.
	const bare = /^\d{4}-\d{2}-\d{2}(T00:00:00(\.0+)?Z)?$/.test(iso);
	const y = bare ? d.getUTCFullYear() : d.getFullYear();
	const day = Date.UTC(y, bare ? d.getUTCMonth() : d.getMonth(), bare ? d.getUTCDate() : d.getDate());
	const now = new Date();
	const diff = Math.round((day - Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())) / 864e5);
	if (diff === 0) return t('discover.titleLibraryWords.today');
	if (diff === -1) return t('discover.titleLibraryWords.yesterday');
	if (diff === 1) return t('discover.titleLibraryWords.tomorrow');
	const same = y === now.getFullYear();
	const zone = bare ? { timeZone: 'UTC' } : {};
	return d.toLocaleDateString(appLocale(), same ? { weekday: Math.abs(diff) < 7 ? 'short' : undefined, month: 'short', day: 'numeric', ...zone } : { month: 'short', day: 'numeric', year: 'numeric', ...zone });
}

const epSources: Record<string, string> = { web_dl: 'WEB', webrip: 'WEB', bluray: 'BluRay', remux: 'Remux', hdtv: 'TV', dvd: 'DVD' };

// The quality of the file you have, as the status: "1080p WEB".
export function epQuality(e: SheetEpisode): string {
	const q = (e.slots[0]?.file?.Quality ?? null) as { resolution?: number; source?: string } | null;
	return q ? [q.resolution ? `${q.resolution}p` : '', epSources[q.source ?? ''] ?? ''].filter(Boolean).join(' ') : '';
}

export function epFile(e: SheetEpisode): string {
	const f = e.slots[0]?.file;
	return f?.Size ? bytes(f.Size as number) : '';
}

export const epTone = (st: string) => (st === 'have' || st === 'imported' || st === 'in_library' || st === 'kept' ? 'have' : st === 'downloading' || st === 'importing' || st === 'searching' ? 'downloading' : st === 'failed' ? 'failed' : 'missing');
