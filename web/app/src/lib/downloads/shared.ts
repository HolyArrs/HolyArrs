// What every Downloads layout receives from the page, and the artwork it draws.
import { manyWords, rowName, type DownloadRow } from '../downloaders.ts';
import { sized } from '../tmdb-image.ts';
import type { ImportRecord, TitleView } from '../types.ts';

export interface LayoutProps {
	rows: DownloadRow[];
	imports: ImportRecord[];
	titles: Map<number, TitleView>;
	/** The id of the download an action is running on (0: none). */
	busy: number;
	ondetails: (r: DownloadRow) => void;
	onfail: (r: DownloadRow) => void;
	onremove: (r: DownloadRow) => void;
}

/** poster is the row's own artwork, else its Title's first poster. */
export function poster(r: DownloadRow, titles: Map<number, TitleView>): string | undefined {
	return r.artwork ?? titles.get(r.title_id ?? 0)?.artwork[0];
}

/** backdrop is the Title's wide picture at the size the slot needs (empty: none known). */
export function backdrop(r: DownloadRow, titles: Map<number, TitleView>, as: 'backdrop' | 'large' = 'backdrop'): string {
	const b = titles.get(r.title_id ?? 0)?.backdrop;
	return b ? sized(b, as) : '';
}

/** logo is the Title's transparent logo art, when known. */
export function logo(r: DownloadRow, titles: Map<number, TitleView>): string {
	const l = titles.get(r.title_id ?? 0)?.logo;
	return l ? sized(l, 'poster') : '';
}

/** who names a download for buttons and screen readers: the title plus the episode. */
export function who(r: DownloadRow): string {
	const n = rowName(r);
	return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
}
