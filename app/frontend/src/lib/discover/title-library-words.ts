import { type SheetEpisode } from '#lib/shows/index.ts';
import { bytes } from '#lib/core/format.ts';
import { t, appLocale } from '#lib/core/i18n/i18n.svelte.ts';

// "Yesterday", "Tomorrow", "Sat, Oct 31" this year, else the date with its year.
function dayDiff(iso: string, local: boolean): number {
	const d = new Date(iso);
	// A bare day (no time) is the same day everywhere; a full time is the viewer's own day.
	const bare = !local && /^\d{4}-\d{2}-\d{2}(T00:00:00(\.0+)?Z)?$/.test(iso);
	const day = Date.UTC(bare ? d.getUTCFullYear() : d.getFullYear(), bare ? d.getUTCMonth() : d.getMonth(), bare ? d.getUTCDate() : d.getDate());
	const now = new Date();
	return Math.round((day - Date.UTC(now.getFullYear(), now.getMonth(), now.getDate())) / 864e5);
}

export function dayWords(iso: string, local = false): string {
	const d = new Date(iso);
	const bare = !local && /^\d{4}-\d{2}-\d{2}(T00:00:00(\.0+)?Z)?$/.test(iso);
	const y = bare ? d.getUTCFullYear() : d.getFullYear();
	const diff = dayDiff(iso, local);
	const now = new Date();
	if (diff === 0) return t('discover.titleLibraryWords.today');
	if (diff === -1) return t('discover.titleLibraryWords.yesterday');
	if (diff === 1) return t('discover.titleLibraryWords.tomorrow');
	const same = y === now.getFullYear();
	const zone = bare ? { timeZone: 'UTC' } : {};
	return d.toLocaleDateString(appLocale(), same ? { weekday: Math.abs(diff) < 7 ? 'short' : undefined, month: 'short', day: 'numeric', ...zone } : { month: 'short', day: 'numeric', year: 'numeric', ...zone });
}

// The day an episode row shows. While the air moment is still ahead, a show day already behind the viewer's today
// would read "Yesterday" next to "Not out yet", so the day of that same moment in the viewer's zone is shown.
export function airWords(day?: string, at?: string, now = Date.now()): string {
	const label = day || at;
	if (!label) return '';
	return at && Date.parse(at) > now && dayDiff(label, false) < 0 ? dayWords(at, true) : dayWords(label);
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

export const epTone = (st: string, hasFile = false) => (st === 'have' || st === 'imported' || st === 'in_library' || st === 'kept' || (hasFile && st !== 'downloading' && st !== 'importing' && st !== 'searching') ? 'have' : st === 'downloading' || st === 'importing' || st === 'searching' ? 'downloading' : st === 'failed' ? 'failed' : 'missing');

/** A season switched on does nothing while the show itself is off: the season list says to turn the show on. */
export const showOffHint = (showKept: boolean, seasons: { keep_up_to_date: boolean }[]) => !showKept && seasons.some((s) => s.keep_up_to_date);

/** "Get all missing" only while the show is on and a kept season still misses aired episodes; anything else queues nothing. */
export const getAllMissingShown = (manage: boolean, showKept: boolean, seasons: { keep_up_to_date: boolean; in_library: number; episodes: number; not_out_yet?: number }[]) =>
	manage && showKept && seasons.some((s) => s.keep_up_to_date && s.in_library < s.episodes - (s.not_out_yet ?? 0));

/** The words when a search queued nothing: an off show needs turning on, an on show had nothing to look for. */
export const nothingQueuedKey = (showKept: boolean) => (showKept ? 'shows.page.nothingToSearch' : 'discover.titleLibrary.turnShowOnHint');

/** A search answer with an empty job list started nothing; an answer without a list is taken as started. */
export const searchQueued = (res: unknown) => {
	const ids = (res as { job_ids?: unknown } | null | undefined)?.job_ids;
	return ids === undefined || (Array.isArray(ids) && ids.length > 0);
};

// Up to six language codes from a file's media info (a list or a slash-separated string).
export const fileLangs = (v: unknown) => (Array.isArray(v) ? v : typeof v === 'string' && v ? v.split(/[\/,]/) : []).map((s) => String(s).trim().toUpperCase()).filter(Boolean).slice(0, 6);
// The day a file was added: month and day this year, else with its year.
export const addedOn = (iso?: string) => { if (!iso) return ''; const d = new Date(iso); return d.toLocaleDateString(appLocale(), { month: 'short', day: 'numeric', ...(d.getFullYear() !== new Date().getFullYear() ? { year: 'numeric' } : {}) }); };
// The last two parts of a folder path, to tell same-named library folders apart.
export const pathEnd = (p: string) => p.split(/[\\/]+/).filter(Boolean).slice(-2).join('/');
