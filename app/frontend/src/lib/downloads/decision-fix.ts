// Which fix a download waiting for a decision leads with, and how its reason reads.
import type { Decision } from '../core/types.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { cap } from './history-lines.ts';

export type Fix = 'edition' | 'version' | 'hand' | 'search' | 'folder' | 'import' | 'match' | 'again';

type D = Pick<Decision, 'kind' | 'why' | 'download_id' | 'reason'> & Partial<Pick<Decision, 'import_file' | 'import_folder'>>;

// A placing that failed: what follows the lead may hold the system's words and file paths, which are never read for what to fix.
const placingFailed = (r: string) => /^\s*the file (for .+? )?could not be placed\b/i.test(r);
// The importer refused the file in place of the copy that is already there.
const copyInPlace = (r: string) => !placingFailed(r) && /copy in place/i.test(r);
// The importer could not settle the edition (the lines of copyRefusal), which Set edition can fix.
const editionRefusal = (r: string) => /^\s*(edition (is|does)\b|file and release name disagree about the edition)/i.test(r);

// The files could not be read because no save folder or path pair is set up, or the one set up leads nowhere (Settings > Downloaders).
const noSaveFolder = (r: string) => !placingFailed(r) && /HOLYARRS_SAVE_PATH|where the Downloader says it is/.test(r);

// HolyArrs may not read the downloaded file or write to the Library folder: a permission the person gives outside HolyArrs.
const noAccess = (r: string) => /^\s*HolyArrs may not (read the downloaded file|write to the folder this file goes to)/i.test(r);

// The same download can be imported again once the person fixed the folder, the mount or the permission the reason names (importer.CanImportAgain).
// The import gave up on a fault that may have passed; the same download can be imported again.
const gaveUp = (r: string) => /^\s*the file could not be placed: something went wrong; see the log\.?\s*$/i.test(r);
// Reading the download folder took longer than its time limit; trying again may read it.
const slowRead = (r: string) => /^\s*reading the download folder took too long; try again\.?\s*$/i.test(r);

export function canImportAgain(d: D): boolean {
	const r = d.reason ?? '';
	return !!d.download_id && (noSaveFolder(r) || noAccess(r) || gaveUp(r) || slowRead(r));
}

// Every file of one download waits on the same reason Import again answers: one press imports the whole download again.
export function packImportAgain(items: D[]): boolean {
	const [a] = items;
	if (!a || !canImportAgain(a)) return false;
	const r = (a.reason ?? '').trim();
	return items.every((d) => d.download_id === a.download_id && (d.reason ?? '').trim() === r);
}

// No file in the download matched what it was grabbed for (importer/unread_reason.go), old wording too: which one.
export function noMatchKind(r: string): 'movie' | 'episode' | 'track' | '' {
	const m = /^(?:no file in the download matched (this movie|an episode|a track)|nothing in the download could be matched to (an episode|a track)|nothing could be placed[;.]|nothing could be placed$)/i.exec(r.trim());
	if (!m) return '';
	const w = (m[1] ?? m[2] ?? '').toLowerCase();
	return w === 'an episode' ? 'episode' : w === 'a track' ? 'track' : 'movie';
}

// The importer refused a file the Downloader placed outside the download folder (importer refusedMedia).
export const refusedOutside = (r: string) => /^HolyArrs refused .+: the Downloader reported a file outside the download folder\.?$/.test(r.trim());

// The refused file line in the picked language; the API ends it with a full stop the stored line has not.
export function refusedLine(r: string): string {
	const dot = r.endsWith('.');
	const line = serverWords(dot ? r.slice(0, -1) : r);
	return dot && !line.endsWith('.') ? line + '.' : line;
}

export function decisionFix(d: D): Fix {
	const r = d.reason ?? '';
	if (noSaveFolder(r)) return 'folder';
	if (d.kind === 'movie' && d.download_id && editionRefusal(r)) return 'edition';
	if (d.kind === 'movie' && copyInPlace(r)) return 'version';
	// The movie's download holds one video: the person can say it is this movie.
	if (d.kind === 'movie' && d.import_file) return 'import';
	// A permission was missing: once given, the same download is imported again.
	if (canImportAgain(d)) return 'again';
	// Nothing matched: Import files by hand opens on the download's folder to match its files.
	if (d.import_folder && (noMatchKind(r) || refusedOutside(r))) return 'match';
	if (d.why === 'conflict' || d.why === 'taken') return 'hand';
	return 'search';
}

// edition: the edition line; copy: the copy in place stays; taken: the files went elsewhere; nomatch: no file matched; refused: a file outside the folder was refused; plain: the reason itself.
export function reasonKind(d: D): 'edition' | 'copy' | 'taken' | 'nomatch' | 'refused' | 'plain' {
	const fix = decisionFix(d);
	if (fix === 'edition') return 'edition';
	if (fix === 'version') return 'copy';
	if (d.why === 'taken') return 'taken';
	if (noMatchKind(d.reason ?? '')) return 'nomatch';
	if (refusedOutside(d.reason ?? '')) return 'refused';
	return 'plain';
}

// A reason the server wrote, in the picked language. The server ends it with a full stop and the stored lines have none; a reason with no stored line stays as it came.
// A translated reason ends with the full stop the language's own sentences end with (taken from a stored line that ends a sentence), or none where the language uses none.
export function reasonWords(text: string): string {
	const bare = text.replace(/[.;]\s*$/, '');
	const said = serverWords(bare);
	if (said === bare) return text;
	const stop = /\.\s*$/.test(text) && !/[.。।!?！？:：]$/.test(said) ? (t('server.metadata.the-metadata-server-is-slow-right').match(/[.。।]$/)?.[0] ?? '') : '';
	return cap(said) + stop;
}
