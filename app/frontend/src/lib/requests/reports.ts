// Reported problems: anyone signed in reports a problem with a title in the Library; the owner and admins see every report on /requests/reports and mark it fixed (with an optional reply) or open it again. Members see their own.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { api } from '../core/api.ts';

export type ReportKind = 'picture' | 'sound' | 'subtitles' | 'wont_play' | 'other';
export type ReportState = 'open' | 'fixed';
export type ReportFilter = ReportState | 'all';

export interface ReportWho {
	id: number;
	name: string;
	display_name?: string;
}

export interface ReportView {
	id: number;
	created_at: string;
	title_id: number;
	title_name: string;
	title_kind: string;
	year?: number;
	poster?: string;
	album_id?: number;
	album_name?: string;
	season?: number;
	item_id?: number;
	episode?: number;
	track?: number;
	item_name?: string;
	version_id?: number;
	version?: string;
	file_id?: number;
	/** What part it is about in words: S02E03, Season 2, an album, Track 4. */
	where?: string;
	kind: ReportKind | string;
	kind_label: string;
	note?: string;
	state: ReportState | string;
	state_label: string;
	made_by?: ReportWho;
	closed_by?: ReportWho;
	closed_at?: string;
	reply?: string;
	mine: boolean;
}

export interface ReportInput {
	title_id: number;
	kind: ReportKind;
	note?: string;
	season?: number | null;
	item_id?: number | null;
	album_id?: number | null;
	version_id?: number | null;
}

/** The problem kinds, in the order the sheet shows them. */
export const reportKinds: { kind: ReportKind; label: string; hint: string }[] = [
	{ kind: 'picture', get label() { return t('requests.report.kind.picture'); }, get hint() { return t('requests.report.hint.picture'); } },
	{ kind: 'sound', get label() { return t('requests.report.kind.sound'); }, get hint() { return t('requests.report.hint.sound'); } },
	{ kind: 'subtitles', get label() { return t('requests.report.kind.subtitles'); }, get hint() { return t('requests.report.hint.subtitles'); } },
	{ kind: 'wont_play', get label() { return t('requests.report.kind.wontPlay'); }, get hint() { return t('requests.report.hint.wontPlay'); } },
	{ kind: 'other', get label() { return t('requests.report.kind.other'); }, get hint() { return t('requests.report.hint.other'); } }
];

export const reportFilters: { filter: ReportFilter; label: string }[] = [
	{ filter: 'open', get label() { return t('requests.report.filter.open'); } },
	{ filter: 'fixed', get label() { return t('requests.report.filter.fixed'); } },
	{ filter: 'all', get label() { return t('requests.report.filter.all'); } }
];

/** The longest note or reply the server takes. */
export const noteMax = 500;

/** Names a problem kind; unknown kinds keep their own word. */
export function kindLabel(kind: string): string {
	return reportKinds.find((k) => k.kind === kind)?.label ?? kind;
}

/** The name a person chose, else their sign-in name. */
export function whoName(w: ReportWho | undefined): string {
	return w ? w.display_name || w.name : '';
}

/** The title with its year: "Driftwood Moon (2024)". */
export function titleLine(r: Pick<ReportView, 'title_name' | 'year'>): string {
	return r.year ? `${r.title_name} (${r.year})` : r.title_name;
}

/** Which part a report is about: "S02E03 · The Mill", "Season 2", "Album name · Track 4", or "" for the whole title. */
export function partLine(r: Pick<ReportView, 'where' | 'item_name' | 'album_name' | 'track'>): string {
	const where = r.where?.trim() ?? '';
	const name = r.item_name?.trim() ?? '';
	if (!where && !name) return '';
	if (!name || where.includes(name)) return where;
	return where ? `${where} · ${name}` : name;
}

/** The picture for a problem kind (the report sheet's tiles, the cards' tag, the window). */
export function kindIcon(kind: string): 'image' | 'sound' | 'subtitle' | 'play' | 'info' {
	return kind === 'picture' ? 'image' : kind === 'sound' ? 'sound' : kind === 'subtitles' ? 'subtitle' : kind === 'wont_play' ? 'play' : 'info';
}

/** What a problem kind covers, or '' for a kind this app does not know. */
export function kindHint(kind: string): string {
	return reportKinds.find((k) => k.kind === kind)?.hint ?? '';
}

/** partLine for a card: a Version that came in brackets ("(Main)") reads "Main version". */
export function partWords(r: Pick<ReportView, 'where' | 'item_name' | 'album_name' | 'track'>): string {
	const part = partLine(r);
	const m = /^\((.+)\)$/.exec(part);
	return m ? t('requests.report.version', { name: m[1] }) : part;
}

/** The line under a report's title: what it is, its year and the part: "TV show · 2008 · Season 2". */
export function factsLine(r: Pick<ReportView, 'title_kind' | 'year' | 'where' | 'item_name' | 'album_name' | 'track'>): string {
	const what = t(r.title_kind === 'artist' ? 'requests.kind.artist' : r.title_kind === 'show' ? 'requests.kind.show' : 'requests.kind.movie');
	return [what, r.year, partWords(r)].filter(Boolean).join(' · ');
}

/** One-tap replies for Mark fixed, the kind's own first. */
export function quickReplies(kind: string): string[] {
	const own = kind === 'subtitles' ? [t('requests.report.reply.subtitles')] : kind === 'sound' ? [t('requests.report.reply.sound')] : kind === 'picture' ? [t('requests.report.reply.picture')] : [];
	return [...own, t('requests.report.reply.swapped'), t('requests.report.reply.tryAgain'), t('requests.report.reply.nothingWrong')];
}

/** What the server is sent: the note trimmed, parts that were not picked left out. */
export function reportBody(input: ReportInput): Record<string, unknown> {
	const out: Record<string, unknown> = { title_id: input.title_id, kind: input.kind };
	const note = (input.note ?? '').trim();
	if (note) out.note = note.slice(0, noteMax);
	if (typeof input.season === 'number' && input.season >= 0) out.season = input.season;
	for (const k of ['item_id', 'album_id', 'version_id'] as const) {
		const v = input[k];
		if (typeof v === 'number' && v > 0) out[k] = v;
	}
	return out;
}

/** Asks for one filter's reports; managers can narrow to their own. */
export function listPath(filter: ReportFilter, opts: { titleId?: number; mine?: boolean; limit?: number } = {}): string {
	const q = new URLSearchParams();
	if (filter !== 'all') q.set('state', filter);
	if (opts.titleId) q.set('title_id', String(opts.titleId));
	if (opts.mine) q.set('mine', '1');
	if (opts.limit) q.set('limit', String(opts.limit));
	const s = q.toString();
	return s ? `/reports?${s}` : '/reports'; // API path
}

/** Reads ?show= from the address; anything unknown is Open. */
export function filterOf(v: string | null | undefined): ReportFilter {
	return v === 'fixed' || v === 'all' ? v : 'open';
}

/** Counts the reports in each filter. */
export function stateCounts(reports: Pick<ReportView, 'state'>[]): Record<ReportFilter, number> {
	const open = reports.filter((r) => r.state === 'open').length;
	return { open, fixed: reports.length - open, all: reports.length };
}

/** Puts open reports first, then the newest. */
export function sortReports<T extends Pick<ReportView, 'state' | 'created_at' | 'id'>>(reports: T[]): T[] {
	return [...reports].sort((a, b) => (a.state === b.state ? 0 : a.state === 'open' ? -1 : 1) || b.created_at.localeCompare(a.created_at) || b.id - a.id);
}

/** The page a report's title opens on. */
export function reportHref(r: Pick<ReportView, 'title_id' | 'album_id' | 'title_kind' | 'season'>): string {
	if (r.album_id) return `/library/${r.title_id}/albums/${r.album_id}`;
	if (r.title_kind === 'show') return `/library/${r.title_id}?tab=library`;
	return `/library/${r.title_id}`;
}

/** Narrows "Find a better copy" to the reported part: a show's season or episode, an artist's album or track (a movie searches all of it). */
export function searchBody(r: Pick<ReportView, 'title_kind' | 'season' | 'item_id' | 'album_id' | 'version_id'>): Record<string, number> {
	const out: Record<string, number> = {};
	if (r.title_kind === 'show') {
		if (r.item_id) out.item_id = r.item_id;
		else if (typeof r.season === 'number') out.season = r.season;
	} else if (r.title_kind === 'artist') {
		if (r.item_id) out.item_id = r.item_id;
		else if (r.album_id) out.album_id = r.album_id;
	} else return out;
	if (r.version_id) out.version_id = r.version_id;
	return out;
}

/** An episode as the sheet's picker shows it: "E03 · The Mill". */
export function episodeWord(e: { episode: number; name?: string }): string {
	const n = `E${String(e.episode).padStart(2, '0')}`;
	return e.name ? `${n} · ${e.name}` : n;
}

/** Whether an episode or track has a file in the Library. */
export function hasFile(x: { slots?: { file?: unknown; file_id?: number; state?: string }[] }): boolean {
	return (x.slots ?? []).some((s) => !!s.file || !!s.file_id || s.state === 'in_library' || s.state === 'better_wanted');
}

export async function makeReport(input: ReportInput): Promise<ReportView> {
	return (await api.post<{ report: ReportView }>('/reports', reportBody(input))).report;
}

export async function listReports(filter: ReportFilter, opts: { titleId?: number; mine?: boolean; limit?: number } = {}): Promise<{ reports: ReportView[]; open?: number }> {
	const r = await api.get<{ reports: ReportView[] | null; open?: number }>(listPath(filter, opts));
	return { reports: r.reports ?? [], open: r.open };
}

export async function fixReport(id: number, reply = ''): Promise<ReportView> {
	const text = reply.trim().slice(0, noteMax);
	return (await api.post<{ report: ReportView }>(`/reports/${id}/fix`, text ? { reply: text } : {})).report;
}

export async function reopenReport(id: number): Promise<ReportView> {
	return (await api.post<{ report: ReportView }>(`/reports/${id}/reopen`)).report;
}
