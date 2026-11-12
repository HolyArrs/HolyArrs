// The words for a History row. Line 1 says what it is (the title, the episode or the kind of title), a status tag says what kind of thing happened, and line 2 says what happened in plain words, never repeating the title.
import { t as say, lang, appLocale } from '#lib/core/i18n/i18n.svelte.ts';
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
	aired: { get word() { return say('downloads.hl.tag.aired'); }, icon: 'calendarDay', tone: 'info' },
	out: { get word() { return say('downloads.hl.tag.out'); }, icon: 'calendarDay', tone: 'info' },
	added: { get word() { return say('downloads.hl.tag.added'); }, icon: 'plus', tone: 'ok' },
	removed: { get word() { return say('downloads.hl.tag.removed'); }, icon: 'trash', tone: 'idle' },
	checked: { get word() { return say('downloads.hl.tag.checked'); }, icon: 'refresh', tone: 'idle' },
	searched: { get word() { return say('downloads.hl.tag.searched'); }, icon: 'search', tone: 'idle' },
	came: { get word() { return say('downloads.hl.tag.came'); }, icon: 'check', tone: 'ok' },
	getting: { get word() { return say('downloads.hl.tag.getting'); }, icon: 'download', tone: 'info' },
	stalled: { get word() { return say('downloads.hl.tag.stalled'); }, icon: 'waiting', tone: 'wait' },
	paused: { get word() { return say('downloads.hl.tag.paused'); }, icon: 'pause', tone: 'idle' },
	failed: { get word() { return say('downloads.hl.tag.failed'); }, icon: 'x', tone: 'fail' },
	needs: { get word() { return say('downloads.hl.tag.needs'); }, icon: 'alert', tone: 'wait' },
	warning: { get word() { return say('downloads.hl.tag.warning'); }, icon: 'alert', tone: 'wait' },
	fixed: { get word() { return say('downloads.hl.tag.fixed'); }, icon: 'check', tone: 'ok' },
	changed: { get word() { return say('downloads.hl.tag.changed'); }, icon: 'settings', tone: 'idle' },
	asked: { get word() { return say('downloads.hl.tag.asked'); }, icon: 'wish', tone: 'info' },
	approved: { get word() { return say('downloads.hl.tag.approved'); }, icon: 'check', tone: 'ok' },
	declined: { get word() { return say('downloads.hl.tag.declined'); }, icon: 'declined', tone: 'idle' },
	reported: { get word() { return say('downloads.hl.tag.reported'); }, icon: 'alert', tone: 'wait' },
	reopened: { get word() { return say('downloads.hl.tag.reopened'); }, icon: 'alert', tone: 'wait' }
} satisfies Record<string, Tag>;

const PL: Record<string, string> = { 'Library folder': 'libraryFolder', 'new season': 'newSeason', 'new episode': 'newEpisode', 'new album': 'newAlbum', 'new track': 'newTrack', episode: 'episode', album: 'album', track: 'track', file: 'file' };
const plural = (n: number, one: string) => say(`downloads.hl.pl.${PL[one]}.${n === 1 ? 'one' : 'other'}`, { count: n });
const str = (v: unknown) => (typeof v === 'string' ? v.trim() : '');
const num = (v: unknown) => (typeof v === 'number' && Number.isFinite(v) ? v : 0);
const join = (...parts: (string | false | null | undefined)[]) => parts.filter(Boolean).join(' · ');

/** Capitalises the first letter but leaves names like "qBittorrent" and "iPhone" alone. */
export const cap = (s: string) => (s && !/^[a-z][A-Z]/.test(s) ? s[0].toUpperCase() + s.slice(1) : s);

const problemWords: Record<string, string> = {
	get picture() { return say('downloads.hl.problem.picture'); },
	get sound() { return say('downloads.hl.problem.sound'); },
	get subtitles() { return say('downloads.hl.problem.subtitles'); },
	get wont_play() { return say('downloads.hl.problem.wontPlay'); },
	get other() { return say('downloads.hl.problem.other'); }
};
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

const kindWord: Record<string, string> = {
	get movie() { return say('downloads.hl.kind.movie'); },
	get show() { return say('downloads.hl.kind.show'); },
	get artist() { return say('downloads.hl.kind.artist'); }
};

/** When, in a sentence: "today", "yesterday", "on Oct 25". */
export function dayWord(at: string, now = new Date()): string {
	const k = dayKey(at);
	if (k === dayKey(now.toISOString())) return say('downloads.hl.today');
	const y = new Date(now);
	y.setDate(y.getDate() - 1);
	if (k === dayKey(y.toISOString())) return say('downloads.hl.yesterday');
	const d = new Date(at);
	return say('downloads.hl.onDate', { date: d.toLocaleDateString(appLocale('en-US'), { month: 'short', day: 'numeric', ...(d.getFullYear() !== now.getFullYear() ? { year: 'numeric' } : {}) }) });
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
	if (dl) s = dl[1].toLowerCase() === 'error' ? say('downloads.hl.dlError') : say('downloads.hl.dlSays', { says: dl[1] });
	s = s.replace(/[;,]?\s*look at the import plan$/i, '');
	s = s.replace(/\bno Release passed the Pick rules\b/i, say('downloads.hl.nothingGood')).replace(/\bRelease(s?)\b/g, 'download$1').replace(/\bPick rules\b/g, say('downloads.hl.qualityRules'));
	return s;
}

const count = (n: number) => n.toLocaleString(appLocale('en-US'));

/** How much of a Library folder went out of reach: "All 1,347 files gone". */
export function folderGone(e: EventRecord): string {
	const d = e.Data ?? {};
	const gone = num(d.gone), known = num(d.known);
	const lost = gone && known ? (gone >= known ? say(known === 1 ? 'downloads.hl.allGone.one' : 'downloads.hl.allGone.other', { known: count(known) }) : say(known === 1 ? 'downloads.hl.someGone.one' : 'downloads.hl.someGone.other', { gone: count(gone), known: count(known) })) : cap(plainWhy(str(d.reason)));
	return lost;
}

/** Names a missing file: "03 - One for the Road · Missing". */
export function missingLine(e: Pick<EventRecord, 'Data'>): string {
	const d = e.Data ?? {};
	const name = (str(d.path) || str(d.to) || str(d.file)).split(/[\\/]/).pop()?.replace(/\.[a-z0-9]{2,4}$/i, '') ?? '';
	return name ? say('downloads.hl.missingNamed', { name }) : say('downloads.hl.fileMissing');
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
	const what = !eps.length ? (season ? say('downloads.hl.season', { n: season }) : kind ? kindWord[kind] : '') : '';
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
			return eps.length || e.episode ? t(TAG.aired, say('downloads.hl.aired', { when: dayWord(e.At, now) })) : t(TAG.out, say('downloads.hl.outToWatch', { when: dayWord(e.At, now) }));
		case 'title_added': {
			const by = who(e, ctx);
			const how = str(d.why_added) === 'import' ? say('downloads.hl.addedImport') : str(d.why_added) === 'ask' ? (by ? say('downloads.hl.addedAskBy', { by }) : say('downloads.hl.addedAsk')) : by ? say('downloads.hl.addedBy', { by }) : say('downloads.hl.addedByApp');
			return t(TAG.added, how);
		}
		case 'list_synced': return t(TAG.added, say('downloads.hl.addedList'));
		case 'title_removed': return t(TAG.removed, join(say('downloads.hl.removedLib'), d.delete_files === true ? say('downloads.hl.filesDeleted') : say('downloads.hl.filesKept')));
		case 'episodes_removed': return t(TAG.removed, withoutTitle(e.Summary, name) || say('downloads.hl.episodesRemoved'));
		case 'folder_scanned': {
			const v = Array.isArray(d.versions) ? (d.versions as Record<string, unknown>[]) : [];
			const none = v.length > 0 && v.every((x) => x.exists === false);
			return t(TAG.checked, join(say('downloads.hl.checkedFolder'), none ? say('downloads.hl.noFolderYet') : say('downloads.hl.nothingChanged')));
		}
		case 'library_rescanned': {
			const changes = [num(d.missing) && say('downloads.hl.nMissing', { n: num(d.missing) }), num(d.back) && say('downloads.hl.nBack', { n: num(d.back) }), num(d.found) && say('downloads.hl.nFound', { n: num(d.found) }), num(d.unreachable) && say('downloads.hl.nOut', { n: num(d.unreachable) })].filter(Boolean) as string[];
			return t(TAG.checked, join(say('downloads.hl.checkedN', { what: plural(num(d.folders) || 1, 'Library folder') }), changes.length ? changes.join(', ') : say('downloads.hl.nothingChanged')));
		}
		case 'library_scanned': return t(TAG.checked, join(say('downloads.hl.checkedFolders'), withoutTitle(e.Summary, name)));
		case 'library_folder_health':
			return str(d.health) && str(d.health) !== 'ok' ? t(TAG.failed, join(say('downloads.hl.folderOut'), plainWhy(str(d.reason)))) : t(TAG.checked, say('downloads.hl.folderFine'));
		case 'title_refreshed': {
			const changes = [num(d.seasons_added) && plural(num(d.seasons_added), 'new season'), num(d.episodes_added) && plural(num(d.episodes_added), 'new episode'), num(d.albums_added) && plural(num(d.albums_added), 'new album'), num(d.tracks_added) && plural(num(d.tracks_added), 'new track')].filter(Boolean) as string[];
			return t(TAG.checked, join(say('downloads.hl.checkedDetails'), changes.length ? changes.join(', ') : say('downloads.hl.nothingNew')));
		}
		case 'episodes_changed':
		case 'albums_changed': {
			const changes = [num(d.seasons_added) && plural(num(d.seasons_added), 'new season'), num(d.episodes_added) && plural(num(d.episodes_added), 'new episode'), num(d.albums_added) && plural(num(d.albums_added), 'new album'), num(d.tracks_added) && plural(num(d.tracks_added), 'new track'), num(d.episodes_removed) && say('downloads.hl.gone', { what: plural(num(d.episodes_removed), 'episode') }), num(d.albums_removed) && say('downloads.hl.gone', { what: plural(num(d.albums_removed), 'album') })].filter(Boolean) as string[];
			return t(TAG.changed, changes.length ? join(...changes) : say('downloads.hl.checkedList'));
		}
		case 'album_changed': {
			const album = /^.*?:\s*(.+?):\s*Keep up to date (on|off)$/i.exec(e.Summary)?.[1];
			return t(TAG.changed, join(album, keepLine(d) || withoutTitle(e.Summary, name)));
		}
		case 'season_changed': return t(TAG.changed, join(say('downloads.hl.season', { n: num(d.season) }), keepLine(d) || say('downloads.hl.settingsChanged')));
		case 'title_edited': {
			const c = (d.change ?? {}) as Record<string, unknown>;
			return t(TAG.changed, join(say('downloads.hl.settingsChangedCap'), keepLine(c)));
		}
		case 'details_source_changed': return t(TAG.changed, say('downloads.hl.detailsSource'));
		case 'version_changed': return t(TAG.changed, say('downloads.hl.versionsChanged'));
		case 'renamed': return t(TAG.changed, say('downloads.hl.renamed'));
		case 'rename_planned': return t(TAG.changed, say('downloads.hl.renameReady'));
		case 'search_done': {
			const m = /\((\d+) found, (\d+) refused\)/.exec(e.Summary);
			const found = m ? Number(m[1]) : 0;
			if (/^(Season \d+: )?no Release passed/i.test(e.Summary) || !m) return t(TAG.searched, join(say('downloads.hl.lookedFor'), found ? say('downloads.hl.foundNone', { n: found }) : say('downloads.hl.nothingFound')));
			return t(TAG.searched, join(say('downloads.hl.lookedFor'), say('downloads.hl.nFound', { n: found })));
		}
		case 'search_failed': return t(TAG.failed, join(say('downloads.hl.searchFailed'), plainWhy(str(d.reason) || str(d.error))));
		case 'feed_checked': return t(TAG.checked, say('downloads.hl.feeds'));
		case 'release_chosen':
		case 'release_chosen_by_hand': {
			const r = resolutionOf(str(d.release) || e.Summary);
			return t(TAG.getting, join(k === 'release_chosen_by_hand' ? say('downloads.hl.pickedHand') : say('downloads.hl.picked'), r));
		}
		case 'release_turned_down': return t(TAG.declined, join(say('downloads.hl.turnedDown'), plainWhy(str(d.reason))));
		case 'turned_down_removed': return t(TAG.changed, say('downloads.hl.unblocked'));
		case 'release_held': return t(TAG.stalled, join(say('downloads.hl.heldBack'), plainWhy(str(d.reason))));
		case 'held_removed': return t(TAG.changed, say('downloads.hl.heldGone'));
		case 'download_sent': return t(TAG.getting, res);
		case 'download_queued': return t(TAG.getting, join(say('downloads.hl.waiting'), res));
		case 'download_downloading': return t(TAG.getting, join(num(d.progress) ? say('downloads.hl.downloadingPct', { pct: Math.round(num(d.progress) * 100) }) : say('downloads.hl.downloading'), res));
		case 'download_stalled': return t(TAG.stalled, join(say('downloads.hl.stalledLine'), plainWhy(str(d.reason))));
		case 'download_paused': return t(TAG.paused, join(num(d.progress) ? say('downloads.hl.pausedAt', { pct: Math.round(num(d.progress) * 100) }) : say('downloads.hl.paused'), res));
		case 'download_finished': return t(TAG.getting, join(res, sizeOf(d)));
		case 'download_seeding': return t(TAG.came, join(say('downloads.hl.sharing'), res));
		case 'download_removed': return t(TAG.changed, say('downloads.hl.removedDl'));
		case 'download_changed': return t(TAG.changed, e.Summary || say('downloads.hl.changedDl'));
		case 'download_failed': return t(TAG.failed, plainWhy(str(d.reason)) || say('downloads.hl.dlFailed'));
		case 'download_vanished': return t(TAG.failed, say('downloads.hl.vanished'));
		case 'download_not_removed': return t(TAG.failed, say('downloads.hl.notRemoved'));
		case 'download_not_moved': return t(TAG.failed, say('downloads.hl.notMoved'));
		case 'import_done': return t(TAG.came, join(resolutionOf(releaseOf([e])), sizeOf(d)));
		case 'import_partly_done': return t({ ...TAG.came, tone: 'wait' }, join(say('downloads.hl.partly'), say('downloads.hl.needLook')));
		case 'import_failed':
		case 'import_rejected': return t(TAG.failed, plainWhy(str(d.reason)) || say('downloads.hl.nothingPlaced'));
		case 'file_replaced': return t(TAG.came, join(say('downloads.hl.better'), say('downloads.hl.oldAside')));
		case 'adopted': {
			const files = num(d.files), albums = num(d.albums);
			return t(TAG.came, join(say('downloads.hl.foundFolder'), albums ? say('downloads.hl.inAlbums', { tracks: plural(files, 'track'), albums: plural(albums, 'album') }) : files ? plural(files, 'file') : ''));
		}
		case 'file_missing': return t(TAG.failed, missingLine(e));
		case 'file_back': return t(TAG.fixed, say('downloads.hl.fileBack'));
		case 'file_held': return t(TAG.changed, say('downloads.hl.fileHeld'));
		case 'file_hold_failed': return t(TAG.failed, say('downloads.hl.holdFailed'));
		case 'holding_cleaned': return t(TAG.changed, say('downloads.hl.holdingCleaned'));
		case 'free_space_low': return t(TAG.warning, join(say('downloads.hl.lowSpace'), plainWhy(str(d.reason) || str(d.message)).replace(/\s*\([^)]*\)$/, '')));
		case 'needs_decision': return t(TAG.needs, plainWhy(str(d.reason)));
		case 'stopped_looking': return t(TAG.changed, join(say('downloads.hl.stoppedLooking'), plainWhy(str(d.reason))));
		case 'health_broken': return t(str(d.state) === 'warning' ? TAG.warning : TAG.failed, plainWhy(str(d.message) || withoutTitle(e.Summary, name)));
		case 'health_recovered': return t(TAG.fixed, join(say('downloads.hl.workingAgain'), str(d.subject)));
		// The tag says the step; the line keeps only who and what ("Maya · Every album").
		case 'ask_made': return named(TAG.asked, who(e, ctx) || str(d.asked_by) || say('downloads.hl.aMember'), cap(str(d.albums) || str(d.seasons)));
		case 'ask_approved': return d.auto === true ? t(TAG.approved, join(say('downloads.hl.automatic'), str(d.note))) : named(TAG.approved, str(d.approved_by) || who(e, ctx), str(d.note));
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
			return user && said.startsWith(user) ? { name, bits, tag: TAG.changed, line: said } : t(TAG.changed, plainWhy(withoutTitle(said, name)) || say('downloads.hl.somethingChanged'));
		}
		case 'library_folder_unreachable': return t(TAG.failed, join(folderName(e), folderGone(e)) || plainWhy(withoutTitle(e.Summary, name)));
		case 'secrets_imported': return t(TAG.changed, join(say('downloads.hl.settingsImported'), say('downloads.hl.nSet', { n: num(d.set) })));
		case 'media_server_told': return t(TAG.checked, say('downloads.hl.mediaTold'));
		case 'media_server_failed': return t(TAG.failed, join(say('downloads.hl.mediaFailed'), plainWhy(str(d.reason) || str(d.error))));
		case 'alert_failed': {
			const to = str(d.alert);
			const why = plainWhy(str(d.reason) || str(d.error));
			return t(TAG.failed, join(to || say('downloads.hl.alertNotSent'), to && why.startsWith(to) ? cap(why.slice(to.length).replace(/^\s*(answered|says|said)?\s*/i, '')) : why));
		}
		case 'ai_action_failed': return t(TAG.failed, cap(plainWhy(str(d.error) || str(d.reason))) || withoutTitle(e.Summary, name));
	}
	if (k.startsWith('subtitle_')) {
		const bad = /failed|none|not_found/.test(k);
		return t(bad ? { word: say('downloads.hl.tag.subtitles'), icon: 'subtitle', tone: 'wait' } : TAG.changed, withoutTitle(e.Summary, name) || say('downloads.hl.subtitlesChanged'));
	}
	if (k.startsWith('library_folder_')) return t(/unreach|missing|fail/.test(k) ? TAG.failed : TAG.checked, plainWhy(withoutTitle(e.Summary, name)));
	if (k.startsWith('ask_')) return t(TAG.asked, withoutTitle(e.Summary, name));
	return t(TAG.changed, plainWhy(withoutTitle(e.Summary, name)) || say('downloads.hl.somethingChanged'));
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
		const why = plainWhy(str(bad?.Data?.reason)) || (failed.key === 'imported' ? say('downloads.hl.nothingPlaced') : say('downloads.hl.dlFailed'));
		return { ...head, tag: TAG.failed, line: scrub(why, head.name) };
	}
	if (row.events.some((e) => e.Kind === 'file_replaced')) return { ...head, tag: TAG.came, line: join(say('downloads.hl.better'), res, size, say('downloads.hl.oldAside')) };
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
	return d.keep_up_to_date === true ? say('downloads.hl.kept') : d.keep_up_to_date === false ? say('downloads.hl.notKept') : '';
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
