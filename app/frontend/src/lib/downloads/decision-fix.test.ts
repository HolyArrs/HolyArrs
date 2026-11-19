import { canImportAgain, mayImportAgain, decisionFix, noMatchKind, packImportAgain, reasonKind, reasonWords, refusedLine } from './decision-fix.ts';
import { lang } from '#lib/core/i18n/i18n.svelte.ts';
import en from '../core/i18n/messages/en/server-lines.json';
import { plainReason } from '../core/plain.ts';
import { describe, expect, it } from 'vitest';

const dec = (o: Record<string, unknown>) => ({ kind: 'movie', why: 'conflict', download_id: 7, reason: '', ...o }) as Parameters<typeof decisionFix>[0];

describe('decision fix', () => {
	it('leads a refused outside file with Import by hand on the folder and names it', () => {
		// The exact text /api/v1/needs-decision sends: capital first letter and a full stop.
		const reason = 'HolyArrs refused Show.S01E01.mkv: the Downloader reported a file outside the download folder.';
		expect(decisionFix(dec({ kind: 'show', why: 'unmatched', reason, import_folder: '/dl/Show' }))).toBe('match');
		expect(reasonKind(dec({ kind: 'show', why: 'unmatched', reason }))).toBe('refused');
		expect(reasonKind(dec({ kind: 'show', why: 'unmatched', reason: reason.slice(0, -1) }))).toBe('refused');
	});
	it('says the refused file line in the picked language with the API full stop', () => {
		const start = { value: lang.value, words: lang.words };
		const reason = 'HolyArrs refused Show.S01E01.mkv: the Downloader reported a file outside the download folder.';
		try {
			lang.value = 'en';
			expect(refusedLine(reason)).toBe(reason);
			const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/de/server-lines.json', { eager: true })).map((f) => f.default));
			lang.value = 'de';
			lang.words = { ...(en as Record<string, string>), ...de };
			expect(refusedLine(reason)).toBe('HolyArrs hat Show.S01E01.mkv abgelehnt: Der Downloader meldete eine Datei außerhalb des Download-Ordners.');
		} finally {
			lang.value = start.value;
			lang.words = start.words;
		}
	});
	it('still reads the other reason kinds with the API full stop', () => {
		expect(reasonKind(dec({ kind: 'show', why: 'unmatched', reason: 'No file in the download matched an episode; match the files by hand.' }))).toBe('nomatch');
		expect(decisionFix(dec({ reason: 'The copy in place is protected; choose another Version for an upgrade.' }))).toBe('version');
		expect(decisionFix(dec({ reason: 'Edition is unknown; confirm the edition or choose an unconstrained Version.' }))).toBe('edition');
	});
	it('sends a missing save folder to Settings > Downloaders', () => {
		const reason = "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and SABnzbd has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again";
		expect(decisionFix(dec({ reason }))).toBe('folder');
		expect(decisionFix(dec({ kind: 'show', why: 'unreadable', reason }))).toBe('folder');
		expect(reasonKind(dec({ reason }))).toBe('plain');
	});
	it('offers Set edition only for an edition refusal', () => {
		for (const reason of ['edition is unknown; confirm the edition or choose an unconstrained Version', 'edition is ambiguous; confirm it explicitly', 'edition does not match the selected Version', 'file and release name disagree about the edition; confirm it explicitly']) {
			expect(decisionFix(dec({ reason }))).toBe('edition');
			expect(reasonKind(dec({ reason }))).toBe('edition');
		}
	});
	it('never offers Set edition for another conflict', () => {
		expect(decisionFix(dec({ reason: 'No file in the download matched this movie; match the files by hand.' }))).toBe('hand');
		expect(reasonKind(dec({ reason: 'No file in the download matched this movie; match the files by hand.' }))).toBe('nomatch');
		expect(reasonKind(dec({ reason: 'nothing could be placed: not enough free space in the Library folder' }))).toBe('plain');
	});
	it('points a copy in place reason to another Version', () => {
		for (const reason of ['the copy in place is protected; choose another Version for an upgrade', 'the new edition differs from the copy in place; choose another Version', "file is 1080p web, which this Version's Pick rules don't allow, so it can't replace the copy in place; choose another Version"]) {
			expect(decisionFix(dec({ reason }))).toBe('version');
			expect(reasonKind(dec({ reason }))).toBe('copy');
		}
	});
	it('leads with Import this file when the movie download holds one video', () => {
		const reason = 'No file in the download matched this movie; match the files by hand.';
		expect(decisionFix(dec({ reason, import_file: 'grp-ean.mkv' }))).toBe('import');
		expect(reasonKind(dec({ reason, import_file: 'grp-ean.mkv' }))).toBe('nomatch');
		expect(decisionFix(dec({ reason: 'edition is unknown', import_file: 'x.mkv' }))).toBe('edition');
		expect(decisionFix(dec({ kind: 'show', reason, import_file: 'x.mkv' }))).toBe('hand');
	});
	it('leads with Import files by hand when nothing matched and the folder is known (#312)', () => {
		const reason = 'No file in the download matched an episode; match the files by hand.';
		expect(decisionFix(dec({ kind: 'show', reason, import_folder: '/data/x' }))).toBe('match');
		expect(decisionFix(dec({ kind: 'show', reason }))).toBe('hand');
		expect(decisionFix(dec({ reason: 'nothing could be placed; look at the import plan', import_folder: '/d' }))).toBe('match');
		expect(decisionFix(dec({ reason: 'No file in the download matched this movie; match the files by hand.', import_folder: '/d', import_file: 'a.mkv' }))).toBe('import');
		expect(reasonKind(dec({ kind: 'show', reason, import_folder: '/d' }))).toBe('nomatch');
	});
	it('names what did not match, old reasons too', () => {
		expect(noMatchKind('No file in the download matched this movie; match the files by hand.')).toBe('movie');
		expect(noMatchKind('no file in the download matched an episode; match the files by hand')).toBe('episode');
		expect(noMatchKind('No file in the download matched a track; match the files by hand.')).toBe('track');
		expect(noMatchKind('Nothing could be placed; look at the import plan.')).toBe('movie');
		expect(noMatchKind('nothing in the download could be matched to an episode; look at the import plan')).toBe('episode');
		expect(noMatchKind('nothing in the download could be matched to a track; look at the import plan')).toBe('track');
		expect(noMatchKind('nothing could be placed: not enough free space in the Library folder')).toBe('');
	});
	it('keeps shows and missing downloads off the movie fixes', () => {
		expect(decisionFix(dec({ kind: 'show', reason: 'edition is unknown' }))).toBe('hand');
		expect(decisionFix(dec({ download_id: undefined, reason: 'edition is unknown' }))).toBe('hand');
		expect(decisionFix(dec({ kind: 'show', reason: 'the copy in place is protected; choose another Version for an upgrade' }))).toBe('hand');
		expect(decisionFix(dec({ why: 'ran_out', reason: 'no other Release passed the Pick rules' }))).toBe('search');
		expect(reasonKind(dec({ why: 'taken', reason: 'x' }))).toBe('taken');
	});
	it('offers Import again for a reason the person fixes outside HolyArrs', () => {
		const pair = "HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again";
		const read = 'HolyArrs may not read the downloaded file; give the user HolyArrs runs as (PUID and PGID) read access to the file and the folder it is in, then import again';
		const write = 'HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again';
		const save = "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and SABnzbd has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again";
		const slow = 'reading the download folder took too long; try again';
		for (const kind of ['movie', 'show', 'artist']) {
			expect(decisionFix(dec({ kind, reason: slow }))).toBe('again');
			expect(canImportAgain(dec({ kind, reason: slow }))).toBe(true);
			// The folder is set up first, then the same download is imported again.
			for (const reason of [pair, save]) {
				expect(decisionFix(dec({ kind, reason }))).toBe('folder');
				expect(canImportAgain(dec({ kind, reason }))).toBe(true);
			}
			for (const reason of [read, write]) {
				expect(decisionFix(dec({ kind, reason }))).toBe('again');
				expect(canImportAgain(dec({ kind, reason }))).toBe(true);
			}
		}
		// An import that gave up on a passing fault can be asked for again; other placing faults cannot.
		for (const kind of ['show', 'artist']) {
			expect(decisionFix(dec({ kind, reason: 'the file could not be placed: something went wrong; see the log' }))).toBe('again');
			expect(canImportAgain(dec({ kind, reason: 'The file could not be placed: something went wrong; see the log.' }))).toBe(true);
		}
		expect(canImportAgain(dec({ reason: 'the file could not be placed: open /dl/something went wrong; see the log' }))).toBe(false);
		// A movie whose one video is known keeps Import this file in front; Import again stays on offer.
		expect(decisionFix(dec({ reason: read, import_file: 'a.mkv' }))).toBe('import');
		expect(canImportAgain(dec({ reason: read, import_file: 'a.mkv' }))).toBe(true);
		// No download to import again, or another reason: not on offer.
		expect(canImportAgain(dec({ reason: read, download_id: undefined }))).toBe(false);
		expect(decisionFix(dec({ kind: 'show', reason: read, download_id: undefined }))).toBe('hand');
		for (const reason of ['no file in the download matched this movie; match the files by hand', 'the file could not be placed: open /dl/HolyArrs may not read the downloaded file.mkv: input/output error', 'no other Release passed the Pick rules'])
			expect(canImportAgain(dec({ reason }))).toBe(false);
	});
	it('does not take a word in a path for an edition, copy or folder reason', () => {
		const placed = (tail: string) => dec({ import_file: 'a.mkv', import_folder: '/d', reason: 'the file could not be placed: ' + tail });
		const path = placed('hardlink failed (link /dl/Blade.Runner.1982.Special.Edition.1080p.mkv /movies/Blade Runner (1982)/Blade Runner (1982).mkv: operation not permitted), copy failed (open /dl/Blade.Runner.1982.Special.Edition.1080p.mkv: input/output error)');
		expect(decisionFix(path)).toBe('import');
		expect(reasonKind(path)).toBe('plain');
		expect(decisionFix(placed('mkdir /movies/My copy in place/x: read-only file system'))).toBe('import');
		expect(decisionFix(placed('open /dl/HOLYARRS_SAVE_PATH/x.mkv: input/output error'))).toBe('import');
		for (const r of ['edition is ambiguous; confirm it explicitly', 'edition is unknown; confirm the edition or choose an unconstrained Version', 'Edition does not match the selected Version.', 'file and release name disagree about the edition; confirm it explicitly'])
			expect(decisionFix(dec({ reason: r }))).toBe('edition');
		expect(decisionFix(dec({ reason: 'the new edition differs from the copy in place; choose another Version' }))).toBe('version');
	});
	it('says why a file was not placed as a plain line, in the picked language', () => {
		const taken = 'A file with the same name that HolyArrs did not place is already in the folder; move it out of the folder, then import again.';
		const reason = (r: string) => dec({ import_file: 'a.mkv', import_folder: '/d', reason: r });
		expect(reasonKind(reason(taken))).toBe('plain');
		expect(decisionFix(reason(taken))).toBe('import');
		expect(reasonKind(reason('HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again'))).toBe('plain');
		expect(reasonKind(dec({ why: 'conflict', import_file: 'a.mkv', import_folder: '/d', reason: "HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again" }))).toBe('plain');
		expect(decisionFix(dec({ why: 'conflict', import_file: 'a.mkv', import_folder: '/d', reason: "HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again" }))).toBe('folder');
		expect(reasonWords(taken)).toBe(taken);
		const start = { value: lang.value, words: lang.words };
		lang.value = 'de';
		lang.words = { 'server.importer.a-file-with-the-same-name': 'a file with the same name that HolyArrs did not place is already in the folder; move it out of the folder, then import again' };
		expect(reasonWords(taken)).toBe(taken);
		lang.words = { ...lang.words, 'server.importer.a-file-with-the-same-name': 'Im Ordner liegt schon eine Datei' };
		expect(reasonWords(taken)).toBe('Im Ordner liegt schon eine Datei.');
		lang.value = start.value;
		lang.words = start.words;
	});
	it('keeps the cut-down lead of a system reason in the picked language', () => {
		const de = Object.assign({}, ...Object.values(import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/de/server-lines.json', { eager: true })).map((f) => f.default));
		const start = { value: lang.value, words: lang.words };
		lang.value = 'de';
		lang.words = de;
		const lead = plainReason('The file could not be placed: mkdir /x: not a directory.').text;
		expect(lead).toBe('The file could not be placed.');
		expect(reasonWords(lead)).toBe('Die Datei konnte nicht abgelegt werden.');
		lang.value = start.value;
		lang.words = start.words;
	});
	const packs = import.meta.glob<{ default: Record<string, string> }>('../core/i18n/messages/*/server-lines.json', { eager: true });
	const pack = (code: string) => ({ ...packs[`../core/i18n/messages/${code}/server-lines.json`].default });
	it('shows the unread-files line and the no-match lines wholly in German, ending as the English one does', () => {
		const start = { value: lang.value, words: lang.words };
		lang.value = 'de';
		lang.words = pack('de');
		const unread = reasonWords("HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and SABnzbd has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again.");
		expect(unread).toBe('HolyArrs kann die Dateien dieses Downloads nicht lesen: HOLYARRS_SAVE_PATH ist nicht gesetzt und für SABnzbd gibt es kein Pfadpaar. Setze HOLYARRS_SAVE_PATH, wenn beide Apps denselben Download-Ordner sehen, oder füge unter Einstellungen > Downloader ein Pfadpaar hinzu, und importiere erneut.');
		expect(unread).not.toMatch(/\b(can't|read|path pair|Settings|import again|the Downloader)\b/);
		expect(reasonWords("HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and the Downloader has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again.")).toMatch(/für den Downloader gibt es kein Pfadpaar.+erneut\.$/);
		expect(reasonWords('No file in the download matched this movie; match the files by hand.')).toBe('Keine Datei im Download passt zu diesem Film; ordne die Dateien von Hand zu.');
		expect(reasonWords('No file in the download matched an episode; match the files by hand.')).toBe('Keine Datei im Download passt zu einer Episode; ordne die Dateien von Hand zu.');
		expect(reasonWords('No file in the download matched a track; match the files by hand.')).toBe('Keine Datei im Download passt zu einem Titel; ordne die Dateien von Hand zu.');
		expect(reasonWords("HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again.")).toMatch(/importiere erneut\.$/);
		lang.value = start.value;
		lang.words = start.words;
	});
	it('adds no end mark after a line that ends in a colon', () => {
		const start = { value: lang.value, words: lang.words };
		for (const code of ['ja', 'tr', 'ko'] as const) {
			lang.value = code;
			lang.words = pack(code);
			expect(reasonWords('Downloader for.'), code).not.toMatch(/[:：][.。।]$/);
		}
		lang.value = 'ja';
		lang.words = { 'server.health.downloader-for': 'ダウンローダー:' };
		expect(reasonWords('Downloader for.')).toBe('ダウンローダー:');
		lang.value = start.value;
		lang.words = start.words;
	});
	it('ends a translated reason with each language\'s own full stop', () => {
		const start = { value: lang.value, words: lang.words };
		const line = 'No file in the download matched this movie; match the files by hand.';
		for (const [code, end] of [['ja', /。$/], ['zh-cn', /。$/], ['hi', /।$/], ['th', /[^.。।]$/], ['fr', /\.$/]] as const) {
			lang.value = code;
			lang.words = pack(code);
			expect(reasonWords(line), code).toMatch(end);
			expect(reasonWords(line.slice(0, -1)), code).not.toMatch(/[.。।]$/);
		}
		lang.value = start.value;
		lang.words = start.words;
	});
});

describe('album or season card', () => {
	const write = 'HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again';
	const tracks = (reason: (i: number) => string, id = (_: number) => 7) => Array.from({ length: 8 }, (_, i) => dec({ kind: 'artist', why: 'unreadable', download_id: id(i), reason: reason(i) }));
	it('leads with Import again when all 8 tracks wait on the same line Import again answers', () => {
		expect(packImportAgain(tracks(() => write))).toBe(true);
		for (const r of ['reading the download folder took too long; try again', 'the file could not be placed: something went wrong; see the log', "HolyArrs can't read this download's files: HOLYARRS_SAVE_PATH is not set and SABnzbd has no path pair. Set HOLYARRS_SAVE_PATH if both apps see the same download folder, or add a path pair in Settings > Downloaders, then import again"])
			expect(packImportAgain(tracks(() => r))).toBe(true);
	});
	it("keeps today's lead when the reasons differ or none allows Import again", () => {
		expect(packImportAgain(tracks((i) => (i === 3 ? 'no file in the download matched a track' : write)))).toBe(false);
		expect(packImportAgain(tracks(() => 'no file in the download matched a track'))).toBe(false);
		expect(packImportAgain(tracks(() => write, (i) => (i === 0 ? 7 : 8)))).toBe(false);
		expect(packImportAgain([])).toBe(false);
	});
});

describe('Import again when no file matched', () => {
	it('is offered in the menu and Import files by hand still leads', () => {
		const d = { kind: 'show', why: 'ran_out', download_id: 7, import_folder: '/dl/x', reason: 'no file in the download matched an episode; match the files by hand' } as Parameters<typeof mayImportAgain>[0];
		expect(mayImportAgain(d)).toBe(true);
		expect(canImportAgain(d)).toBe(false);
		expect(decisionFix(d)).toBe('match');
		expect(mayImportAgain({ ...d, download_id: undefined })).toBe(false);
		expect(mayImportAgain({ ...d, reason: 'nothing could be placed' })).toBe(false);
	});
});
