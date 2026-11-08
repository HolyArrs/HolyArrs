// What an import did, folded into its download's row on the Downloads table. There is no separate Imports list: the Importing chip shows imports in progress, Done the placed ones, Problems the failed ones.
import type { ImportRecord } from './types.ts';
import { stageOf } from './status-tabs.ts';
import type { DownloadRow } from '../core/types.ts';

export interface ImportFacts {
	/** "Placed by hardlink", "2 of 3 files placed by hardlink". */
	how: string;
	/** The Library folder it went to ("Se7en (1995)"), when known. */
	where: string;
}

/** Keeps the newest import of every download. */
export function byDownload(imports: ImportRecord[]): Map<number, ImportRecord> {
	const out = new Map<number, ImportRecord>();
	for (const im of imports) {
		const id = im.DownloadID;
		if (!id) continue;
		const had = out.get(id);
		if (!had || im.ID > had.ID) out.set(id, im);
	}
	return out;
}

const methodWords: Record<string, string> = { hardlink: 'hardlink', move: 'moving', copy: 'copying', reflink: 'reflink', symlink: 'link' };

/** The last folder of a path, never the whole path. */
export function folder(path: string | undefined): string {
	const parts = (path ?? '').split(/[\\/]+/).filter(Boolean);
	return parts.length > 1 ? parts[parts.length - 2] : '';
}

/** How and where an import put the files; null when nothing was placed. */
export function placed(im: ImportRecord | undefined): ImportFacts | null {
	const res = im?.Result ?? [];
	const ok = res.filter((r) => r.outcome === 'placed');
	if (!ok.length) return null;
	const ways = [...new Set(ok.map((r) => methodWords[r.placed_by ?? ''] ?? r.placed_by ?? ''))].filter(Boolean);
	const by = ways.length ? ` by ${ways.join(' and ')}` : '';
	const wanted = res.filter((r) => r.outcome !== 'skipped').length;
	const how = im?.State === 'partly_done' && wanted > ok.length ? `${ok.length} of ${wanted} files placed${by}` : `Placed${by}`;
	const first = im?.Plan?.find((p) => p.source_file === ok[0].source_file) ?? im?.Plan?.find((p) => p.target_path);
	return { how, where: folder(first?.target_path) };
}

/** Why an import failed in a few words; null when it did not fail. */
export function failReason(im: ImportRecord | undefined): string | null {
	if (!im || im.State !== 'failed') return null;
	const err = im.Result?.find((r) => r.error)?.error?.trim();
	if (err) return err.length > 80 ? `${err.slice(0, 77)}…` : err;
	return 'no file could be placed';
}

/** The short line after a row's status word: what an import in progress is doing, why it failed, or a seeding torrent's ratio. Empty when the word says it all. */
export function statusDetail(r: Pick<DownloadRow, 'phase' | 'state' | 'progress' | 'ratio'>, im: ImportRecord | undefined): string {
	const stage = stageOf(r);
	if (stage === 'importing') {
		// Bytes are in; the Downloader verifies, repairs and unpacks (Usenet).
		if (r.phase === 'downloading') return 'unpacking';
		if (r.phase === 'waiting_to_import') return 'up next';
		return 'placing files';
	}
	if (stage === 'failed' || stage === 'decide') {
		const why = failReason(im);
		if (why) return `import failed: ${why}`;
		// Failed also covers these two; the line says which.
		if (r.phase === 'vanished') return 'removed outside HolyArrs';
		if (r.phase === 'import_refused') return 'the files did not match';
	}
	if (stage === 'seeding' && typeof r.ratio === 'number') return `ratio ${r.ratio.toFixed(2)}`;
	return '';
}
