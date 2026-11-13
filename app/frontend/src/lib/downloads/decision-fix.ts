// Which fix a download waiting for a decision leads with, and how its reason reads.
import type { Decision } from '../core/types.ts';

export type Fix = 'edition' | 'version' | 'hand' | 'search' | 'folder' | 'import' | 'match';

type D = Pick<Decision, 'kind' | 'why' | 'download_id' | 'reason'> & Partial<Pick<Decision, 'import_file' | 'import_folder'>>;

// The importer refused the file in place of the copy that is already there.
const copyInPlace = (r: string) => /copy in place/i.test(r);
// The importer could not settle the edition (copyRefusal), which Set edition can fix.
const editionRefusal = (r: string) => /\bedition\b/i.test(r) && !copyInPlace(r);

// The files could not be read because no save folder or path pair is set up (Settings > Downloaders).
const noSaveFolder = (r: string) => /HOLYARRS_SAVE_PATH/.test(r);

// No file in the download matched what it was grabbed for (importer/unread_reason.go), old wording too: which one.
export function noMatchKind(r: string): 'movie' | 'episode' | 'track' | '' {
	const m = /^(?:no file in the download matched (this movie|an episode|a track)|nothing in the download could be matched to (an episode|a track)|nothing could be placed[;.]|nothing could be placed$)/i.exec(r.trim());
	if (!m) return '';
	const w = (m[1] ?? m[2] ?? '').toLowerCase();
	return w === 'an episode' ? 'episode' : w === 'a track' ? 'track' : 'movie';
}

export function decisionFix(d: D): Fix {
	const r = d.reason ?? '';
	if (noSaveFolder(r)) return 'folder';
	if (d.kind === 'movie' && d.download_id && editionRefusal(r)) return 'edition';
	if (d.kind === 'movie' && copyInPlace(r)) return 'version';
	// The movie's download holds one video: the person can say it is this movie.
	if (d.kind === 'movie' && d.import_file) return 'import';
	// Nothing matched: Import files by hand opens on the download's folder to match its files.
	if (d.import_folder && noMatchKind(r)) return 'match';
	if (d.why === 'conflict' || d.why === 'taken') return 'hand';
	return 'search';
}

// edition: the edition line; copy: the copy in place stays; taken: the files went elsewhere; nomatch: no file matched; plain: the reason itself.
export function reasonKind(d: D): 'edition' | 'copy' | 'taken' | 'nomatch' | 'plain' {
	const fix = decisionFix(d);
	if (fix === 'edition') return 'edition';
	if (fix === 'version') return 'copy';
	if (d.why === 'taken') return 'taken';
	if (noMatchKind(d.reason ?? '')) return 'nomatch';
	return 'plain';
}
