// Where a download is: one stage per row and its status word. Pure, so it
// can be tested.
//
// The words mean the same thing for a torrent and a Usenet download:
// Downloading is bytes coming in (a stalled download is still trying);
// Importing is everything after the bytes arrive (Usenet verify, repair and
// unpack, waiting to import, and putting the files in the Library); Seeding
// is a finished torrent the Downloader still shares. ("Active" is not used:
// in torrent Downloaders it also counts finished torrents still seeding.)
import type { DownloadRow } from '../downloaders.ts';

export type Stage = 'queued' | 'paused' | 'downloading' | 'stalled' | 'importing' | 'done' | 'seeding' | 'failed' | 'decide';

type RowBits = Pick<DownloadRow, 'phase' | 'state' | 'progress'>;

/** stageOf places one row on its fine-grained stage. */
export function stageOf(r: RowBits): Stage {
	switch (r.phase) {
		case 'waiting':
			return 'queued';
		case 'paused':
			return 'paused';
		case 'downloading':
			// A Usenet job at full progress is being verified, repaired or
			// unpacked by the Downloader: the bytes are in, so it is importing.
			return r.progress >= 1 ? 'importing' : 'downloading';
		case 'stalled':
			return 'stalled';
		case 'waiting_to_import':
		case 'importing':
			return 'importing';
		case 'needs_a_decision':
			return 'decide';
		case 'failed':
		case 'vanished':
		case 'import_refused':
			return 'failed';
		case 'done':
			return r.state === 'seeding' ? 'seeding' : 'done';
	}
	// removed (cleaned up) and anything new: finished as far as the table goes.
	return 'done';
}

/** statusWord is the row's word in the status column; it is the same for torrent and
 *  Usenet. A cleaned-up download reads Removed. */
export function statusWord(r: RowBits): string {
	switch (stageOf(r)) {
		case 'queued':
			return 'Queued';
		case 'paused':
			return 'Paused';
		case 'downloading':
			return 'Downloading';
		case 'stalled':
			return 'Stalled';
		case 'importing':
			return 'Importing';
		case 'seeding':
			return 'Seeding';
		case 'decide':
			return 'Needs a decision';
		case 'failed':
			return 'Failed';
	}
	return r.phase === 'removed' ? 'Removed' : 'Done';
}
