// Content of the History details panel, shared by the desktop pane and the phone sheet: the step mark, title, meta line, art, a download's steps, one line of new facts, the facts, and where to go.
import { t as say, lang, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
import { artOf, byWhom, clock, dayKey, titleHref, type Verb } from './history-kinds.ts';
import { cap, episodeBits, missingLine, folderGone, folderName, plainWhy, resolutionOf, rowText, type LineContext } from './history-lines.ts';
import { canFind, filterVerb, rowLinks, type GoLink } from './history-search.ts';
import type { HistoryRow, StepKey } from './history.ts';
import { bytes } from '../core/format.ts';
import type { EventRecord, TitleView } from '../core/types.ts';

/** One fact row: label, value and how the value is set. */
export interface Fact {
	label: 'When' | 'By' | 'Quality' | 'Size' | 'Source' | 'Downloader' | 'Release' | 'File' | 'From' | 'Folder' | 'Why';
	value: string;
	/** The muted relative part of When ("36 min ago"), same day only. */
	ago?: string;
	/** The datetime for a When value. */
	at?: string;
	/** Release, File, From and Folder are set in mono. */
	mono?: boolean;
	/** A path: the full path, for the hover title and the copy button. */
	full?: string;
}

export interface PanelStep {
	key: StepKey;
	label: string;
	state: 'done' | 'waiting' | 'failed' | 'needs';
}

export interface PanelView {
	verb: Verb;
	title: string;
	meta: string;
	art: { src?: string; name: string; square: boolean } | null;
	steps: PanelStep[];
	line: string;
	/** A reported note may use three lines. */
	long: boolean;
	facts: Fact[];
	/** Find another (rows about a release). */
	find: boolean;
	/** Where to go, in order: Import plan, the title, Downloads, Requests or Problems. */
	actions: GoLink[];
}

export interface PanelContext extends Omit<LineContext, 'titles'> {
	titles: Map<number, Pick<TitleView, 'kind' | 'artwork'>>;
}

/** The facts each kind of row shows; When always comes first. */
type Label = Fact['label'];

const str = (v: unknown) => (typeof v === 'string' ? v.trim() : '');
const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const join = (...parts: (string | false | null | undefined)[]) => parts.filter(Boolean).join(' · ');
const count = (n: number) => n.toLocaleString(appLocale('en-US'));
const plural = (n: number, one: string) => say(`downloads.hl.pl.${one}.${n === 1 ? 'one' : 'other'}`, { count: count(n) });
const problemWords: Record<string, string> = {
	get picture() { return say('downloads.hl.problem.picture'); },
	get sound() { return say('downloads.hl.problem.sound'); },
	get subtitles() { return say('downloads.hl.problem.subtitles'); },
	get wont_play() { return say('downloads.hl.problem.wontPlay'); },
	get other() { return say('downloads.hl.problem.other'); }
};

/** The words for a fact's label; the labels themselves stay as ids. */
export const FACT_LABEL: Record<Fact['label'], string> = {
	get When() { return say('downloads.hp.when'); },
	get By() { return say('downloads.hp.by'); },
	get Quality() { return say('downloads.col.quality'); },
	get Size() { return say('downloads.pf.size'); },
	get Source() { return say('downloads.col.source'); },
	get Downloader() { return say('downloads.col.proto'); },
	get Release() { return say('downloads.pf.release'); },
	get File() { return say('downloads.hp.file'); },
	get From() { return say('downloads.hp.from'); },
	get Folder() { return say('downloads.pf.folder'); },
	get Why() { return say('downloads.hp.why'); }
};

/** A fact's time: "Today, 10:00 PM" with "36 min ago", "Yesterday, 4:50 PM", "Oct 21, 3:05 PM". */
export function whenText(at: string, now = new Date()): { text: string; ago: string } {
	const t = new Date(at);
	if (Number.isNaN(t.getTime())) return { text: '', ago: '' };
	const k = dayKey(at);
	const y = new Date(now);
	y.setDate(y.getDate() - 1);
	if (k === dayKey(now.toISOString())) {
		const s = Math.max(0, (now.getTime() - t.getTime()) / 1000);
		const ago = s < 60 ? say('downloads.quiet.justNow') : s < 3600 ? say('downloads.hp.minAgo', { n: Math.floor(s / 60) }) : say('downloads.hp.hAgo', { n: Math.floor(s / 3600) });
		return { text: say('downloads.hp.todayAt', { time: clock(at) }), ago };
	}
	if (k === dayKey(y.toISOString())) return { text: say('downloads.hp.yesterdayAt', { time: clock(at) }), ago: '' };
	const day = t.toLocaleDateString(appLocale('en-US'), { month: 'short', day: 'numeric', ...(t.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) });
	return { text: say('downloads.hp.dayAt', { day, time: clock(at) }), ago: '' };
}

const segs = (p: string) => p.split(/[\\/]+/).filter(Boolean);
const absolute = (p: string) => /^([a-z]:[\\/]|[\\/])/i.test(p);
const sep = (p: string) => (p.includes('\\') ? '\\' : '/');
const esc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

/** Shows a path from the Library folder down ("movies\Heat (1995)\Heat (1995).mkv"): from the folder before the title's own folder when present, else a file with its folder, or a folder alone. */
export function shortPath(p: string, title = ''): string {
	const s = p.trim();
	if (!s || !absolute(s)) return s;
	const parts = segs(s);
	const j = sep(s);
	if (title) {
		const own = new RegExp(`^${esc(title)}(\\s*\\(\\d{4}\\))?$`, 'i');
		const i = parts.findIndex((x, n) => n < parts.length - 1 && own.test(x));
		if (i > 0) return parts.slice(i - 1).join(j);
	}
	const file = /\.[a-z0-9]{2,4}$/i.test(parts[parts.length - 1] ?? '');
	return parts.slice(file ? -2 : -1).join(j);
}

/** Splits a shown path into pieces that each end at a separator so the view breaks only there. A path longer than two lines keeps its end and starts with an ellipsis. */
export function pathBits(p: string, max = 76): string[] {
	const j = sep(p);
	const bits = p.split(/(?<=[\\/])/);
	let len = 0;
	let i = bits.length;
	while (i > 0 && len + bits[i - 1].length <= max) len += bits[--i].length;
	if (i === bits.length) i--;
	// The ellipsis stays on the line of the piece after it, never alone.
	return i > 0 ? [`…${j}${bits[i]}`, ...bits.slice(i + 1)] : bits;
}

/** Keeps a path piece on one line: only separators may break it. */
export const glue = (bit: string) => bit.replace(/ /g, ' ').replace(/-/g, '‑');

const pathsOf = (d: Record<string, unknown>) => (Array.isArray(d.paths) ? (d.paths as unknown[]).filter((p): p is Record<string, unknown> => !!p && typeof p === 'object') : []);

const SYSTEM_LINE: Record<string, string> = {
	get signed_in() { return say('downloads.hp.signedIn'); },
	get signed_out() { return say('downloads.hp.signedOut'); },
	get sign_in_failed() { return say('downloads.hp.signInFailed'); },
	get api_key_created() { return say('downloads.hp.keyMade'); },
	get api_key_deleted() { return say('downloads.hp.keyDeleted'); }
};
const DOWNLOAD: Label[] = ['Quality', 'Size', 'Source', 'Downloader', 'Release'];

/** Names a health or alert row like title rows: what it is about, then what happened ("Discord" · "401 Unauthorized"). */
const splitKinds = /^(health_broken|health_recovered|alert_failed|free_space_low|library_folder_unreachable)$/;

export function sysSplit(row: HistoryRow, ctx: PanelContext): { name: string; line: string } | null {
	const e = row.head;
	if (e.title_name || row.steps.length || !splitKinds.test(e.Kind)) return null;
	const name = subject(e);
	if (!name) return null;
	return { name, line: lineOf(row, ctx, '', '').line };
}

/** Everything the details panel shows for a row. */
export function panelView(row: HistoryRow, ctx: PanelContext): PanelView {
	const e = row.head;
	const now = ctx.now ?? new Date();
	const w = rowText(row, ctx);
	const verb = filterVerb(row, w.tag);
	const kind = e.TitleID ? ctx.titles.get(e.TitleID)?.kind : undefined;
	const d = e.Data ?? {};

	// The title's name, else what a system Event is about, else its own line.
	const own = e.title_name || (w.name !== 'HolyArrs' ? w.name : '') || subject(e);
	const title = own || w.line;

	// Episode bits, a season, an album, or Artist and Album apart.
	const eps = episodeBits(e.episode);
	const season = /^Season (\d+):/.exec(e.Summary)?.[1];
	const album = str(d.album) && !/^\d+$/.test(str(d.album)) ? str(d.album) : '';
	const sort = kind === 'artist' || str(d.artist) ? say('downloads.hl.kind.artist') : kind === 'album' ? say('downloads.hp.album') : '';
	// The default Version ("Main") adds nothing, like "Movie" or "Show".
	const version = e.version && !/^main$/i.test(e.version.trim()) ? e.version : '';
	const meta = own ? join(eps.length ? eps.join(' · ') : season ? say('downloads.hl.season', { n: season }) : album || sort, version) : '';

	const src = e.TitleID && e.title_name ? artOf(e, ctx.titles) : undefined;
	const art = src && e.title_name ? { src, name: e.title_name, square: kind === 'artist' || kind === 'album' } : null;

	// A download's three steps; a decision waiting on an import shows where it stopped.
	const waits = row.events.some((x) => x.Kind === 'needs_decision');
	let steps: PanelStep[] = row.steps.map((s) => (waits && s.state === 'failed' ? { key: s.key, label: say('downloads.hl.tag.needs'), state: 'needs' } : { key: s.key, label: s.label, state: s.state }));
	if (!steps.length && e.Kind === 'needs_decision' && /plac|import/i.test(str(d.reason)))
		steps = [{ key: 'sent', label: say('downloads.hk.sent'), state: 'done' }, { key: 'downloaded', label: say('downloads.hk.downloaded'), state: 'done' }, { key: 'imported', label: say('downloads.hl.tag.needs'), state: 'needs' }];
	const failedAt = row.steps.find((s) => s.state === 'failed')?.key ?? '';

	const { line, long } = lineOf(row, ctx, failedAt, own ? w.line : '');

	// Each fact only when the Events kept it, in one fixed order.
	const wants = new Set(factSet(row, failedAt));
	const facts: Fact[] = [];
	const when = whenText(e.At, now);
	facts.push({ label: 'When', value: when.text, ago: when.ago, at: e.At });
	const add = (label: Label, value: string, extra: Partial<Fact> = {}) => {
		if (value && wants.has(label)) facts.push({ label, value, ...extra });
	};
	const by = person(e, ctx);
	if (!(own && /^sign/.test(e.Kind))) add('By', by);
	const release = first(row, (x) => str(x.release));
	const fromFile = first(row, (x) => str(pathsOf(x)[0]?.from));
	add('Quality', first(row, (x) => str(x.quality)) || resolutionOf(release) || resolutionOf(fromFile));
	const size = first(row, (x) => String(num(x.size_bytes) || num(x.size) || num(x.bytes) || ''));
	add('Size', size ? bytes(Number(size)) : '');
	add('Source', first(row, (x) => str(x.source_name) || str(x.indexer) || str(x.source) || str(x.provider)));
	add('Downloader', first(row, (x) => str(x.downloader)));
	add('Release', release, { mono: true });
	const path = (label: Label, full: string) => add(label, shortPath(full, e.title_name ?? ''), { mono: true, full });
	const isRename = /^(renamed|rename_planned)$/.test(e.Kind);
	path('File', first(row, (x, ev) => str(pathsOf(x).find((p) => str(p.to))?.to) || (isRename || /^file_(missing|back)$/.test(ev.Kind) ? str(x.to) || str(x.path) : '') || str(x.file)));
	path('From', isRename ? str(d.from) : fromFile || first(row, (x) => str(x.from)));
	path('Folder', str(d.folder) || str(d.path) || str(d.save_path));
	const why = str(d.reason) || str(d.error) || str(d.message);
	// Say the reason once: no Why when the line or title already says it.
	const wl = plainWhy(why).toLowerCase(), ll = line.toLowerCase();
	if (why && !ll.includes(wl) && !(ll && wl.includes(ll)) && !title.toLowerCase().includes(wl)) add('Why', cap(plainWhy(why)));

	return { verb, title, meta, art, steps, line, long, facts, find: canFind(row), actions: rowLinks(row, titleHref(e.TitleID, ctx.titles)) };
}

function first(row: HistoryRow, read: (d: Record<string, unknown>, e: EventRecord) => string): string {
	for (const e of row.events) {
		const v = read(e.Data ?? {}, e);
		if (v) return v;
	}
	return '';
}

/** A person or an AI app; '' for HolyArrs, jobs and automatic actions. */
function person(e: EventRecord, ctx: PanelContext): string {
	if (e.Actor?.kind === 'ai') return byWhom(e);
	if (e.Actor?.kind !== 'user') return '';
	const d = e.Data ?? {};
	if (d.auto === true) return '';
	const id = e.Actor.id;
	const named = (v: unknown) => (typeof v === 'string' ? v : Array.isArray(v) && typeof v[0] === 'string' ? v[0] : '');
	const k = e.Kind;
	const own = k === 'ask_made' ? named(d.asked_by) : k === 'ask_approved' ? named(d.approved_by) : k === 'ask_turned_down' ? named(d.turned_down_by) : k === 'report_made' ? named(d.made_by) : k === 'report_fixed' ? named(d.fixed_by) : k === 'report_reopened' ? named(d.reopened_by) : '';
	return own || e.actor_name || (id != null ? ctx.people?.get(id) : '') || '';
}

/** The subject of an Event with no title: the folder, person or downloader it is about. */
function subject(e: EventRecord): string {
	const d = e.Data ?? {};
	const k = e.Kind;
	if (/^sign/.test(k)) return str(d.name) || e.actor_name || '';
	if (k.startsWith('library_folder_')) return folderName(e) || (shortPath(str(d.path)) ? say('downloads.hp.folderOf', { name: shortPath(str(d.path)) }) : '');
	if (k === 'health_broken') {
		// What the check is about, with the word before it when the message has one ("Downloader NZBGet").
		const msg = str(d.message) || e.Summary, sub = str(d.subject);
		const i = sub ? msg.indexOf(sub) : -1;
		if (i >= 0 && i <= 24) return msg.slice(0, i + sub.length).trim();
		// A subject followed by a note ("Trailer reader, last good lookup ...") keeps its name only.
		return sub.split(', ')[0] || /^([^:]{1,60}):/.exec(msg)?.[1] || '';
	}
	if (k === 'health_recovered') return str(d.subject).split(', ')[0];
	if (k === 'alert_failed') return str(d.alert);
	if (k === 'free_space_low') return say('downloads.hl.lowSpace');
	if (k === 'holding_cleaned') return say('downloads.hp.setAside');
	if (k === 'settings_changed') return str(d.name);
	if (k.startsWith('download_') && !e.title_name) return str(d.release) || str(d.downloader);
	return '';
}

function factSet(row: HistoryRow, failedAt: StepKey | ''): Label[] {
	const k = row.head.Kind;
	const has = (re: RegExp) => row.events.some((e) => re.test(e.Kind));
	if (failedAt === 'imported' || /^import_(failed|rejected)$/.test(k)) return ['Release', 'From'];
	if (failedAt === 'downloaded') return ['Downloader', 'Release'];
	if (has(/^file_replaced$/)) return ['Size', 'File'];
	if (has(/^import_(done|partly_done)$/)) return ['By', 'Quality', 'Size', 'File'];
	if (row.steps.length || /^(release_chosen|release_chosen_by_hand|download_(sent|queued|downloading|finished|seeding|stalled|paused|failed|vanished))$/.test(k)) return DOWNLOAD;
	if (k === 'needs_decision' || k === 'stopped_looking') return ['Release'];
	if (/^(renamed|rename_planned)$/.test(k)) return ['File', 'From'];
	if (/^(title_removed|episodes_removed|file_held|holding_cleaned)$/.test(k)) return ['By'];
	if (/^download_(removed|not_removed)$/.test(k)) return ['By', 'Downloader'];
	if (/^(ai_)?(ask|report)_/.test(k)) return ['By'];
	if (k.startsWith('subtitle_')) return ['Source'];
	if (k === 'adopted' || k.startsWith('library_folder_')) return ['Folder'];
	if (k === 'file_missing' || k === 'file_back') return ['File'];
	if (k === 'gate_opened' || /^sign/.test(k)) return [];
	return ['By', 'Downloader', 'Release', 'File', 'From', 'Folder', 'Why'];
}

/** One line of new facts, empty when the mark, title, meta and facts say it all. */
function lineOf(row: HistoryRow, ctx: PanelContext, failedAt: StepKey | '', fallback: string): { line: string; long: boolean } {
	const e = row.head;
	const d = e.Data ?? {};
	const k = e.Kind;
	const reason = (x: unknown) => cap(plainWhy(str(x)));
	if (failedAt) {
		const bad = row.events.find((x) => /failed|vanished|rejected/.test(x.Kind));
		return { line: reason(bad?.Data?.reason) || (failedAt === 'imported' ? say('downloads.hl.nothingPlaced') : ''), long: false };
	}
	if (row.steps.length) return { line: '', long: false };
	if (k in SYSTEM_LINE) return { line: SYSTEM_LINE[k], long: false };
	switch (k) {
		case 'gate_opened': return { line: e.episode ? say('downloads.hl.tag.aired') : say('downloads.hp.released'), long: false };
		case 'title_added': {
			const why = str(d.why_added);
			return { line: why === 'import' ? say('downloads.hp.fromImport') : why === 'ask' ? say('downloads.hp.fromRequest') : why === 'list' ? say('downloads.hp.fromList') : '', long: false };
		}
		case 'library_folder_unreachable': return { line: folderGone(e), long: false };
		case 'health_broken': {
			// The message without its subject: "Does not answer: ..." under the title "Downloader NZBGet".
			const msg = plainWhy(str(d.message) || e.Summary);
			const sub = subject(e);
			if (!sub) return { line: '', long: false };
			const rest = sub && msg.startsWith(sub) ? msg.slice(sub.length).replace(/^[\s:,-]+/, '') : msg;
			// The part in brackets says what happened.
			const inner = /\(([^)]+)\)$/.exec(rest)?.[1];
			return { line: cap(inner || rest || msg), long: false };
		}
		case 'health_recovered': return { line: say('downloads.hl.workingAgain'), long: false };
		case 'alert_failed':
		case 'free_space_low': {
			const sub = subject(e);
			const w = rowText(row, ctx).line;
			return { line: sub && w.startsWith(sub + ' · ') ? w.slice(sub.length + 3) : w, long: false };
		}
		case 'file_held': return { line: say('downloads.hp.fileAside'), long: false };
		case 'holding_cleaned': return { line: '', long: false };
		case 'import_failed':
		case 'import_rejected': return { line: reason(d.reason) || say('downloads.hl.nothingPlaced'), long: false };
		case 'needs_decision':
		case 'stopped_looking': return { line: reason(d.reason), long: false };
		case 'file_replaced': {
			const was = str(d.old_quality), now = str(d.new_quality);
			return { line: was && now ? `${was} → ${now}` : '', long: false };
		}
		case 'title_removed': return { line: d.delete_files === true ? say('downloads.hp.filesDeleted') : say('downloads.hp.filesKept'), long: false };
		case 'download_removed': return { line: join(say('downloads.hl.removedDl'), d.delete_files === true && say('downloads.hp.filesDeletedLow')), long: false };
		case 'ask_made':
		case 'ai_ask_made': return { line: cap(str(d.albums) || str(d.seasons)), long: false };
		case 'ask_approved':
		case 'ai_ask_approved': return { line: join(d.auto === true && say('downloads.hl.automatic'), str(d.note)), long: false };
		case 'ask_turned_down':
		case 'ai_ask_turned_down': return { line: cap(str(d.note)), long: false };
		case 'report_made': {
			const note = str(d.note);
			return { line: join(problemWords[str(d.kind)] ?? cap(str(d.kind)), note && `“${note}”`), long: true };
		}
		// The mark says Reported, so the outcome comes first.
		case 'report_fixed': return { line: join(say('downloads.hl.tag.fixed'), problemWords[str(d.kind)] ?? cap(str(d.kind)), str(d.reply) && `“${str(d.reply)}”`), long: true };
		case 'report_reopened': return { line: join(say('downloads.hp.openedAgain'), problemWords[str(d.kind)] ?? cap(str(d.kind)), str(d.note) && `“${str(d.note)}”`), long: true };
		case 'adopted': {
			const files = num(d.files), albums = num(d.albums);
			return { line: albums ? join(plural(files, 'track'), plural(albums, 'album')) : '', long: false };
		}
		case 'file_missing': return { line: missingLine(e), long: false };
		case 'file_back': return { line: say('downloads.hk.fileBack'), long: false };
		case 'renamed':
		case 'release_chosen':
		case 'release_chosen_by_hand':
		case 'download_sent':
		case 'download_finished':
		case 'import_done': return { line: '', long: false };
	}
	if (k.startsWith('subtitle_')) {
		const langs = Array.isArray(d.languages) ? (d.languages as unknown[]).map(str).filter(Boolean).join(', ') : str(d.language);
		return { line: langs || fallback, long: false };
	}
	return { line: fallback, long: false };
}
