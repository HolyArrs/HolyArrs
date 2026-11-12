// Which fix a download waiting for a decision leads with, and how its reason reads.
import type { Decision } from '../core/types.ts';

export type Fix = 'edition' | 'version' | 'hand' | 'search' | 'folder';

type D = Pick<Decision, 'kind' | 'why' | 'download_id' | 'reason'>;

// The importer refused the file in place of the copy that is already there.
const copyInPlace = (r: string) => /copy in place/i.test(r);
// The importer could not settle the edition (copyRefusal), which Set edition can fix.
const editionRefusal = (r: string) => /\bedition\b/i.test(r) && !copyInPlace(r);

// The files could not be read because no save folder or path pair is set up (Settings > Downloaders).
const noSaveFolder = (r: string) => /HOLYARRS_SAVE_PATH/.test(r);

export function decisionFix(d: D): Fix {
	const r = d.reason ?? '';
	if (noSaveFolder(r)) return 'folder';
	if (d.kind === 'movie' && d.download_id && editionRefusal(r)) return 'edition';
	if (d.kind === 'movie' && copyInPlace(r)) return 'version';
	if (d.why === 'conflict' || d.why === 'taken') return 'hand';
	return 'search';
}

// edition: the edition line; copy: the copy in place stays; taken: the files went elsewhere; plain: the reason itself.
export function reasonKind(d: D): 'edition' | 'copy' | 'taken' | 'plain' {
	const fix = decisionFix(d);
	if (fix === 'edition') return 'edition';
	if (fix === 'version') return 'copy';
	if (d.why === 'taken') return 'taken';
	return 'plain';
}
