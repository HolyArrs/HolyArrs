// What the Downloads table receives from the page, and the artwork it draws.
import { manyWords, rowName, type DownloadRow } from '../downloaders.ts';
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

/** who names a download for buttons and screen readers: the title plus the episode. */
export function who(r: DownloadRow): string {
	const n = rowName(r);
	return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
}
