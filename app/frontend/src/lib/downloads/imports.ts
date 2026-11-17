// What an import did, folded into its download's row on the Downloads table. There is no separate Imports list: the Importing chip shows imports in progress, Done the placed ones, Problems the failed ones.
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { serverWords } from '#lib/core/i18n/server-words.ts';
import type { ImportRecord } from './types.ts';
import { stageOf } from './status-tabs.ts';
import type { DownloadRow } from '../core/types.ts';

export interface ImportFacts {
	/** "Added to the Library (no extra space used)", "2 of 3 files added to the Library". */
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

// One full line per way a file was placed; plain words, no "hardlink".
const methodWords: Record<string, string> = {
	get hardlink() { return t('downloads.import.hardlink'); },
	get move() { return t('downloads.import.move'); },
	get copy() { return t('downloads.import.copy'); },
	get reflink() { return t('downloads.import.reflink'); },
	get symlink() { return t('downloads.import.symlink'); }
};

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
	const ways = [...new Set(ok.map((r) => r.placed_by ?? ''))];
	const wanted = res.filter((r) => r.outcome !== 'skipped').length;
	const one = ways.length === 1 ? methodWords[ways[0]] : undefined;
	const how = im?.State === 'partly_done' && wanted > ok.length ? t('downloads.import.partly', { ok: ok.length, wanted }) : (one ?? t('downloads.import.placed'));
	const first = im?.Plan?.find((p) => p.source_file === ok[0].source_file) ?? im?.Plan?.find((p) => p.target_path);
	return { how, where: folder(first?.target_path) };
}

// An older version left the Downloader's "not found" error instead of the gone mark.
const notFound = /no such file or directory|cannot find the (file|path)/i;

/** filesGone: another app took every file before the import could read it. */
export function filesGone(im: ImportRecord | undefined): boolean {
	const res = (im?.Result ?? []).filter((r) => r.outcome !== 'skipped');
	return res.length > 0 && res.every((r) => r.outcome === 'gone' || (r.outcome === 'failed' && notFound.test(r.error ?? '')));
}

/** Why an import failed in a few words; null when it did not fail. */
export function failReason(im: ImportRecord | undefined): string | null {
	if (!im || im.State !== 'failed') return null;
	const err = im.Result?.find((r) => r.error)?.error?.trim();
	if (err) return err.length > 80 ? `${err.slice(0, 77)}…` : err;
	return t('downloads.import.noFile');
}

/** The short line after a row's status word: what an import in progress is doing, why it failed, or a seeding torrent's ratio. Empty when the word says it all. */
export function statusDetail(r: Pick<DownloadRow, 'phase' | 'state' | 'progress' | 'ratio' | 'refused_why'>, im: ImportRecord | undefined): string {
	const stage = stageOf(r);
	if ((stage === 'done' || stage === 'decide') && filesGone(im)) return t('downloads.detail.filesGone');
	if (stage === 'importing') {
		// Bytes are in; the Downloader verifies, repairs and unpacks (Usenet).
		if (r.phase === 'downloading') return t('downloads.detail.unpacking');
		if (r.phase === 'waiting_to_import') return t('downloads.detail.upNext');
		return t('downloads.detail.placing');
	}
	if (stage === 'failed' || stage === 'decide') {
		const why = failReason(im);
		if (why) return t('downloads.detail.importFailed', { why });
		// Failed also covers these two; the line says which.
		if (r.phase === 'vanished') return t('downloads.detail.removed');
		// A refused import says why in the importer's words, as History does.
		if (r.phase === 'import_refused') return r.refused_why ? serverWords(r.refused_why) : t('downloads.detail.noMatch');
	}
	if (stage === 'seeding' && typeof r.ratio === 'number') return t('downloads.detail.ratio', { ratio: r.ratio.toFixed(2) });
	return '';
}
