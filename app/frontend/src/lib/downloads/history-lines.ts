// The words for a History row. Line 1 says what it is (the title, the episode or the kind of title), a status tag says what kind of thing happened, and line 2 says what happened in plain words, never repeating the title.
import type { IconName } from '#lib/ui/Icon.svelte';
import { dayKey } from './history-kinds.ts';
import type { HistoryRow } from './history.ts';
import { bytes } from '../core/format.ts';
import type { EventRecord, TitleView } from '../core/types.ts';

/** A dot and a word. */
export interface Tag {
	word: string;
	icon: IconName;
	tone: 'ok' | 'fail' | 'wait' | 'info' | 'idle';
}

/** What a row's words need besides the Event. */
export interface LineContext {
	titles?: Map<number, Pick<TitleView, 'kind'>>;
	/** Member names by id, for "Added to the Library by Maya". */
	people?: Map<number, string>;
	/** The signed-in member, named "you". */
	me?: number;
	now?: Date;
}

export interface Bit {
	text: string;
	kind: 'episode' | 'what' | 'version';
}

export interface RowText {
	name: string;
	/** What it is, after the name: "S39 E192", "Pastry Week", "Movie", "4K". */
	bits: Bit[];
	tag: Tag;
	/** What happened. */
	line: string;
}

const TAG = {
	aired: { word: 'Aired', icon: 'calendarDay', tone: 'info' },
	out: { word: 'Out now', icon: 'calendarDay', tone: 'info' },
	added: { word: 'Added', icon: 'plus', tone: 'ok' },
	removed: { word: 'Removed', icon: 'trash', tone: 'idle' },
	checked: { word: 'Checked', icon: 'refresh', tone: 'idle' },
	searched: { word: 'Searched', icon: 'search', tone: 'idle' },
	came: { word: 'Came in', icon: 'check', tone: 'ok' },
	getting: { word: 'Downloading', icon: 'download', tone: 'info' },
	stalled: { word: 'Stalled', icon: 'waiting', tone: 'wait' },
	paused: { word: 'Paused', icon: 'pause', tone: 'idle' },
	failed: { word: 'Failed', icon: 'x', tone: 'fail' },
	needs: { word: 'Needs you', icon: 'alert', tone: 'wait' },
	warning: { word: 'Warning', icon: 'alert', tone: 'wait' },
	fixed: { word: 'Fixed', icon: 'check', tone: 'ok' },
	changed: { word: 'Changed', icon: 'settings', tone: 'idle' },
	asked: { word: 'Asked', icon: 'wish', tone: 'info' },
	approved: { word: 'Approved', icon: 'check', tone: 'ok' },
	declined: { word: 'Turned down', icon: 'declined', tone: 'idle' },
	reported: { word: 'Reported', icon: 'alert', tone: 'wait' },
	reopened: { word: 'Reopened', icon: 'alert', tone: 'wait' }
} satisfies Record<string, Tag>;

const plural = (n: number, one: string, many = one + 's') => `${n} ${n === 1 ? one : many}`;
const str = (v: unknown) => (typeof v === 'string' ? v.trim() : '');
const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const join = (...parts: (string | false | null | undefined)[]) => parts.filter(Boolean).join(' · ');

/** Capitalises the first letter but leaves names like "qBittorrent" and "iPhone" alone. */
export const cap = (s: string) => (s && !/^[a-z][A-Z]/.test(s) ? s[0].toUpperCase() + s.slice(1) : s);

const problemWords: Record<string, string> = { picture: 'Picture', sound: 'Sound', subtitles: 'Subtitles', wont_play: "Won't play", other: 'Other' };
const problem = (v: unknown) => problemWords[str(v)] ?? str(v);

/** A made-up episode name: "Episode 8827", "TBA", or a bare date. */
export function fakeEpisodeName(name: string): boolean {
	const s = name.trim();
	return !s || /^episode\s*#?\d+$/i.test(s) || /^(tba|tbd|untitled)$/i.test(s) || /^\d{1,4}[/.-]\d{1,2}[/.-]\d{1,4}$/.test(s) || /^\d{4}-\d{2}-\d{2}$/.test(s);
}

/** Splits "S39E192 Episode 8827" into ["S39 E192"], keeping a real name. */
export function episodeBits(ep: string | undefined): string[] {
	const s = (ep ?? '').trim();
	if (!s) return [];
	const m = /^S(\d+)E(\d+)(?:[-–]E?(\d+))?\s*(.*)$/i.exec(s);
	if (!m) return [s];
	const two = (v: string) => String(Number(v)).padStart(2, '0');
	const code = `S${two(m[1])} E${two(m[2])}${m[3] ? `–${two(m[3])}` : ''}`;
	const name = m[4].trim();
	return name && !fakeEpisodeName(name) ? [code, name] : [code];
}

const kindWord: Record<string, string> = { movie: 'Movie', show: 'Show', artist: 'Artist' };

/** When, in a sentence: "today", "yesterday", "on Oct 25". */
export function dayWord(at: string, now = new Date()): string {
	const k = dayKey(at);
	if (k === dayKey(now.toISOString())) return 'today';
	const y = new Date(now);
	y.setDate(y.getDate() - 1);
	if (k === dayKey(y.toISOString())) return 'yesterday';
	const d = new Date(at);
	return 'on ' + d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', ...(d.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) });
}

/** The picture size a release name carries ("1080p", "4K"). */
export function resolutionOf(release: string): string {
	const m = /\b(2160p|4k|uhd|1080p|1080i|720p|576p|480p)\b/i.exec(release);
	if (!m) return '';
	const r = m[1].toLowerCase();
	return r === '2160p' || r === '4k' || r === 'uhd' ? '4K' : r;
}

export function plainWhy(reason: string): string {
	let s = reason.trim().replace(/[.;]\s*$/, '');
	const dl = /^the downloader says (.+)$/i.exec(s);
	if (dl) s = dl[1].toLowerCase() === 'error' ? 'downloader error' : `the downloader says ${dl[1]}`;
	s = s.replace(/[;,]?\s*look at the import plan$/i, '');
	s = s.replace(/\bno Release passed the Pick rules\b/i, 'nothing good enough was found').replace(/\bRelease(s?)\b/g, 'download$1').replace(/\bPick rules\b/g, 'your quality rules');
	return s;
}

const count = (n: number) => n.toLocaleString('en-US');

/** How much of a Library folder went out of reach: "All 1,347 files gone". */
export function folderGone(e: EventRecord): string {
	const d = e.Data ?? {};
	const gone = num(d.gone), known = num(d.known);
	const lost = gone && known ? (gone >= known ? `All ${count(known)} ${known === 1 ? 'file' : 'files'} gone` : `${count(gone)} of ${count(known)} ${known === 1 ? 'file' : 'files'} gone`) : cap(plainWhy(str(d.reason)));
	return lost;
}

/** Names a missing file: "03 - One for the Road · Missing". */
export function missingLine(e: Pick<EventRecord, 'Data'>): string {
	const d = e.Data ?? {};
	const name = (str(d.path) || str(d.to) || str(d.file)).split(/[\\/]/).pop()?.replace(/\.[a-z0-9]{2,4}$/i, '') ?? '';
	return name ? `${name} · Missing` : 'File missing';
}

/** The Library folder an Event is about ("Music folder"), read from its summary. */
export function folderName(e: EventRecord): string {
	const n = /^Library folder (.+?):/.exec(e.Summary)?.[1];
	return n ? `${n} folder` : '';
}

/** The words of one Event. */
export function eventText(e: EventRecord, ctx: LineContext = {}): RowText {
	const name = e.title_name ?? askedName(e) ?? 'HolyArrs';
	const d = e.Data ?? {};
	const now = ctx.now ?? new Date();
	const kind = e.TitleID ? ctx.titles?.get(e.TitleID)?.kind : undefined;
	const eps = episodeBits(e.episode);
	const season = /^Season (\d+):/.exec(e.Summary)?.[1];
	const what = !eps.length ? (season ? `Season ${season}` : kind ? kindWord[kind] : '') : '';
	const bits: Bit[] = [...eps.map((text) => ({ text, kind: 'episode' as const })), ...(what ? [{ text: what, kind: 'what' as const }] : []), ...(e.version ? [{ text: e.version, kind: 'version' as const }] : [])];
	const t = (tag: Tag, line: string): RowText => ({ name, bits, tag, line: scrub(line, name) });
	// A line that starts with a member's name keeps the name's own case.
	const named = (tag: Tag, person: string, ...rest: string[]): RowText => {
		const tail = scrub(join(...rest), name);
		return person ? { name, bits, tag, line: join(person, tail) } : { name, bits, tag, line: tail };
	};
	const res = resolutionOf(str(d.release));
	const k = e.Kind;

	switch (k) {
		case 'gate_opened':
			return eps.length || e.episode ? t(TAG.aired, `Aired ${dayWord(e.At, now)}`) : t(TAG.out, `Out to watch ${dayWord(e.At, now)}`);
		case 'title_added': {
			const by = who(e, ctx);
			const how = str(d.why_added) === 'import' ? 'Added from a Library import' : str(d.why_added) === 'ask' ? `Added after an ask${by ? ` by ${by}` : ''}` : by ? `Added to the Library by ${by}` : 'Added to the Library by HolyArrs';
			return t(TAG.added, how);
		}
		case 'list_synced': return t(TAG.added, 'Added from a list');
		case 'title_removed': return t(TAG.removed, join('Removed from the Library', d.delete_files === true ? 'its files were deleted' : 'its files were kept'));
		case 'episodes_removed': return t(TAG.removed, withoutTitle(e.Summary, name) || 'Episodes removed');
		case 'folder_scanned': {
			const v = Array.isArray(d.versions) ? (d.versions as Record<string, unknown>[]) : [];
			const none = v.length > 0 && v.every((x) => x.exists === false);
			return t(TAG.checked, join('Checked its folder', none ? 'no folder yet' : 'nothing changed'));
		}
		case 'library_rescanned': {
			const changes = [num(d.missing) && `${num(d.missing)} missing`, num(d.back) && `${num(d.back)} back`, num(d.found) && `${num(d.found)} found`, num(d.unreachable) && `${num(d.unreachable)} out of reach`].filter(Boolean) as string[];
			return t(TAG.checked, join(`Checked ${plural(num(d.folders) || 1, 'Library folder')}`, changes.length ? changes.join(', ') : 'nothing changed'));
		}
		case 'library_scanned': return t(TAG.checked, join('Checked the Library folders', withoutTitle(e.Summary, name)));
		case 'library_folder_health':
			return str(d.health) && str(d.health) !== 'ok' ? t(TAG.failed, join('Library folder out of reach', plainWhy(str(d.reason)))) : t(TAG.checked, 'Checked a Library folder · it is fine');
		case 'title_refreshed': {
			const changes = [num(d.seasons_added) && plural(num(d.seasons_added), 'new season'), num(d.episodes_added) && plural(num(d.episodes_added), 'new episode'), num(d.albums_added) && plural(num(d.albums_added), 'new album'), num(d.tracks_added) && plural(num(d.tracks_added), 'new track')].filter(Boolean) as string[];
			return t(TAG.checked, join('Checked its details', changes.length ? changes.join(', ') : 'nothing new'));
		}
		case 'episodes_changed':
		case 'albums_changed': {
			const changes = [num(d.seasons_added) && plural(num(d.seasons_added), 'new season'), num(d.episodes_added) && plural(num(d.episodes_added), 'new episode'), num(d.albums_added) && plural(num(d.albums_added), 'new album'), num(d.tracks_added) && plural(num(d.tracks_added), 'new track'), num(d.episodes_removed) && `${plural(num(d.episodes_removed), 'episode')} gone`, num(d.albums_removed) && `${plural(num(d.albums_removed), 'album')} gone`].filter(Boolean) as string[];
			return t(TAG.changed, changes.length ? join(...changes) : 'Checked its list · nothing new');
		}
		case 'album_changed': {
			const album = /^.*?:\s*(.+?):\s*Keep up to date (on|off)$/i.exec(e.Summary)?.[1];
			return t(TAG.changed, join(album, keepLine(d) || withoutTitle(e.Summary, name)));
		}
		case 'season_changed': return t(TAG.changed, join(`Season ${num(d.season)}`, keepLine(d) || 'settings changed'));
		case 'title_edited': {
			const c = (d.change ?? {}) as Record<string, unknown>;
			return t(TAG.changed, join('Settings changed', keepLine(c)));
		}
		case 'details_source_changed': return t(TAG.changed, 'Its details now come from another source');
		case 'version_changed': return t(TAG.changed, 'Versions changed');
		case 'renamed': return t(TAG.changed, 'Renamed its files to match');
		case 'rename_planned': return t(TAG.changed, 'Rename ready');
		case 'search_done': {
			const m = /\((\d+) found, (\d+) refused\)/.exec(e.Summary);
			const found = m ? Number(m[1]) : 0;
			if (/^(Season \d+: )?no Release passed/i.test(e.Summary) || !m) return t(TAG.searched, join('Looked for it', found ? `${found} found, none good enough` : 'nothing found yet'));
			return t(TAG.searched, join('Looked for it', `${found} found`));
		}
		case 'search_failed': return t(TAG.failed, join('Search failed', plainWhy(str(d.reason) || str(d.error))));
		case 'feed_checked': return t(TAG.checked, 'Checked the feeds for new downloads');
		case 'release_chosen':
		case 'release_chosen_by_hand': {
			const r = resolutionOf(str(d.release) || e.Summary);
			return t(TAG.getting, join(k === 'release_chosen_by_hand' ? 'Picked by hand' : 'Picked a download', r));
		}
		case 'release_turned_down': return t(TAG.declined, join('A download was turned down', plainWhy(str(d.reason))));
		case 'turned_down_removed': return t(TAG.changed, 'Unblocked a download');
		case 'release_held': return t(TAG.stalled, join('A download is held back', plainWhy(str(d.reason))));
		case 'held_removed': return t(TAG.changed, 'A held download was let go');
		case 'download_sent': return t(TAG.getting, res);
		case 'download_queued': return t(TAG.getting, join('Waiting to download', res));
		case 'download_downloading': return t(TAG.getting, join(`Downloading${num(d.progress) ? ` · ${Math.round(num(d.progress) * 100)}%` : ''}`, res));
		case 'download_stalled': return t(TAG.stalled, join('Download stalled', plainWhy(str(d.reason))));
		case 'download_paused': return t(TAG.paused, join(`Download paused${num(d.progress) ? ` at ${Math.round(num(d.progress) * 100)}%` : ''}`, res));
		case 'download_finished': return t(TAG.getting, join(res, sizeOf(d)));
		case 'download_seeding': return t(TAG.came, join('Sharing it back', res));
		case 'download_removed': return t(TAG.changed, 'Removed from downloader');
		case 'download_changed': return t(TAG.changed, e.Summary || 'Changed in the downloader');
		case 'download_failed': return t(TAG.failed, plainWhy(str(d.reason)) || 'Download failed');
		case 'download_vanished': return t(TAG.failed, 'Download disappeared from the downloader');
		case 'download_not_removed': return t(TAG.failed, 'Could not remove it from the downloader');
		case 'import_done': return t(TAG.came, join(resolutionOf(releaseOf([e])), sizeOf(d)));
		case 'import_partly_done': return t({ ...TAG.came, tone: 'wait' }, join('Partly in the Library', 'some files need a look'));
		case 'import_failed':
		case 'import_rejected': return t(TAG.failed, plainWhy(str(d.reason)) || 'Nothing could be placed');
		case 'file_replaced': return t(TAG.came, join('A better copy came in', 'the old one is set aside'));
		case 'adopted': {
			const files = num(d.files), albums = num(d.albums);
			return t(TAG.came, join('Found in its folder', albums ? `${plural(files, 'track')} in ${plural(albums, 'album')}` : files ? plural(files, 'file') : ''));
		}
		case 'file_missing': return t(TAG.failed, missingLine(e));
		case 'file_back': return t(TAG.fixed, 'A missing file is back on disk');
		case 'file_held': return t(TAG.changed, 'A file was set aside');
		case 'file_hold_failed': return t(TAG.failed, 'Could not set a file aside');
		case 'holding_cleaned': return t(TAG.changed, 'Cleared old set-aside files');
		case 'free_space_low': return t(TAG.warning, join('Low space', plainWhy(str(d.reason) || str(d.message)).replace(/\s*\([^)]*\)$/, '')));
		case 'needs_decision': return t(TAG.needs, plainWhy(str(d.reason)));
		case 'stopped_looking': return t(TAG.changed, join('Stopped looking for it', plainWhy(str(d.reason))));
		case 'health_broken': return t(str(d.state) === 'warning' ? TAG.warning : TAG.failed, plainWhy(str(d.message) || withoutTitle(e.Summary, name)));
		case 'health_recovered': return t(TAG.fixed, join('Working again', str(d.subject)));
		// The tag says the step; the line keeps only who and what ("Maya · Every album").
		case 'ask_made': return named(TAG.asked, who(e, ctx) || str(d.asked_by) || 'a member', cap(str(d.albums) || str(d.seasons)));
		case 'ask_approved': return d.auto === true ? t(TAG.approved, join('Automatic', str(d.note))) : named(TAG.approved, str(d.approved_by) || who(e, ctx), str(d.note));
		case 'ask_turned_down': return named(TAG.declined, str(d.turned_down_by) || who(e, ctx), str(d.note));
		case 'report_made': return t(TAG.reported, join(problem(d.kind), str(d.note)));
		case 'report_fixed': return t(TAG.fixed, join(problem(d.kind), str(d.reply)));
		case 'report_reopened': return t(TAG.reopened, join(problem(d.kind), str(d.note)));
		// Sign-in names show as they are, never capitalised.
		case 'signed_in':
		case 'signed_out':
		case 'sign_in_failed': {
			const user = str(d.name) || e.actor_name || '';
			const said = e.Summary.trim();
			return user && said.startsWith(user) ? { name, bits, tag: TAG.changed, line: said } : t(TAG.changed, plainWhy(withoutTitle(said, name)) || 'Something changed');
		}
		case 'library_folder_unreachable': return t(TAG.failed, join(folderName(e), folderGone(e)) || plainWhy(withoutTitle(e.Summary, name)));
		case 'secrets_imported': return t(TAG.changed, join('Settings imported', `${num(d.set)} set`));
		case 'media_server_told': return t(TAG.checked, 'Told the media server about it');
		case 'media_server_failed': return t(TAG.failed, join('Could not reach the media server', plainWhy(str(d.reason) || str(d.error))));
		case 'alert_failed': {
			const to = str(d.alert);
			const why = plainWhy(str(d.reason) || str(d.error));
			return t(TAG.failed, join(to || 'Alert not sent', to && why.startsWith(to) ? cap(why.slice(to.length).replace(/^\s*(answered|says|said)?\s*/i, '')) : why));
		}
		case 'ai_action_failed': return t(TAG.failed, cap(plainWhy(str(d.error) || str(d.reason))) || withoutTitle(e.Summary, name));
	}
	if (k.startsWith('subtitle_')) {
		const bad = /failed|none|not_found/.test(k);
		return t(bad ? { word: 'Subtitles', icon: 'subtitle', tone: 'wait' } : TAG.changed, withoutTitle(e.Summary, name) || 'Subtitles changed');
	}
	if (k.startsWith('library_folder_')) return t(/unreach|missing|fail/.test(k) ? TAG.failed : TAG.checked, plainWhy(withoutTitle(e.Summary, name)));
	if (k.startsWith('ask_')) return t(TAG.asked, withoutTitle(e.Summary, name));
	return t(TAG.changed, plainWhy(withoutTitle(e.Summary, name)) || 'Something changed');
}

/** The words of a History row; a folded download says how far it got. */
export function rowText(row: HistoryRow, ctx: LineContext = {}): RowText {
	const head = eventText(row.head, ctx);
	if (!row.steps.length) return head;
	const release = releaseOf(row.events);
	const res = resolutionOf(release);
	const size = row.events.map((e) => sizeOf(e.Data ?? {})).find(Boolean) ?? '';
	const failed = row.steps.find((s) => s.state === 'failed');
	if (failed) {
		const bad = row.events.find((e) => /failed|vanished|rejected/.test(e.Kind));
		const why = plainWhy(str(bad?.Data?.reason)) || (failed.key === 'imported' ? 'Nothing could be placed' : 'Download failed');
		return { ...head, tag: TAG.failed, line: scrub(why, head.name) };
	}
	if (row.events.some((e) => e.Kind === 'file_replaced')) return { ...head, tag: TAG.came, line: join('A better copy came in', res, size, 'the old one is set aside') };
	if (row.steps.find((s) => s.key === 'imported')?.state === 'done') return { ...head, tag: TAG.came, line: join(res, size) };
	return head;
}

function sizeOf(d: Record<string, unknown>): string {
	const n = num(d.size_bytes) || num(d.size) || num(d.bytes);
	return n > 0 ? bytes(n) : '';
}

function releaseOf(events: EventRecord[]): string {
	for (const e of events) {
		const r = str(e.Data?.release);
		if (r) return r;
	}
	for (const e of events) {
		const p = e.Data?.paths;
		if (Array.isArray(p) && p[0] && typeof p[0] === 'object') return str((p[0] as { from?: unknown }).from);
	}
	return '';
}

function who(e: EventRecord, ctx: LineContext): string {
	const d = e.Data ?? {};
	const named = str(d.asked_by) || (Array.isArray(d.asked_by) ? str(d.asked_by[0]) : '');
	if (e.Actor?.kind === 'user') {
		const id = e.Actor.id;
		return (id != null && ctx.people?.get(id)) || (id != null && id === ctx.me ? 'you' : '') || e.actor_name || named || 'a member';
	}
	return '';
}

/** Takes the title off the front of a summary ("Dune: approved by Maya" becomes "approved by Maya"). */
function withoutTitle(summary: string, name: string): string {
	let s = summary.trim();
	if (name) {
		const esc = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
		s = s.replace(new RegExp(`^${esc}(\\s*\\(\\d{4}\\))?(\\s+S\\d+E\\d+)?\\s*:?\\s*`, 'i'), '');
		s = s.replace(new RegExp(`\\s*\\b${esc}\\b(\\s*\\(\\d{4}\\))?`, 'gi'), ' it').replace(/\s+/g, ' ').trim();
	}
	return cap(s);
}

/** Line 2 never repeats the title; a leftover mention becomes "it". */
function scrub(line: string, name: string): string {
	if (!name || name === 'HolyArrs') return cap(line);
	const esc = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	return cap(line.replace(new RegExp(`(^|[^\\w])${esc}(\\s*\\(\\d{4}\\))?(?=$|[^\\w])`, 'gi'), '$1it').replace(/\s+/g, ' ').trim());
}

function keepLine(d: Record<string, unknown>): string {
	return d.keep_up_to_date === true ? 'Kept up to date' : d.keep_up_to_date === false ? 'No longer kept up to date' : '';
}

/** The title an ask is about, read from its summary when the title is not in the Library. */
function askedName(e: EventRecord): string | undefined {
	// An AI app's ask: "AI (Chat app, key home) asked for Dune for Maya".
	if (e.Kind.startsWith('ai_ask_')) {
		const t = /^AI \(.*?\) (?:asked for|approved|turned down) (.+) for [^\s].*$/.exec(e.Summary)?.[1]?.trim();
		return t && t !== 'a title' ? t.replace(/\s*\(\d{4}\)$/, '') : undefined;
	}
	if (!e.Kind.startsWith('ask_')) return undefined;
	const made = /asked for (.+?)(?:\s*\(\d{4}\))?(?:,\s.*)?$/.exec(e.Summary)?.[1];
	if (e.Kind === 'ask_made') return made?.trim() || undefined;
	return /^(.+?)(?:\s*\(\d{4}\))?:\s(?:approved|turned down)\b/i.exec(e.Summary)?.[1]?.trim() || undefined;
}
