// The artwork and the name a Downloads row uses.
import { manyWords, rowName } from './downloaders.ts';
import { type DownloadRow } from '../core/types.ts';
import type { TitleView } from '../core/types.ts';

/** The row's own artwork, else its Title's first poster. */
export function poster(r: DownloadRow, titles: Map<number, TitleView>): string | undefined {
	return r.artwork ?? titles.get(r.title_id ?? 0)?.artwork[0];
}

/** Names a download for buttons and screen readers: the title plus the episode. */
export function who(r: DownloadRow): string {
	const n = rowName(r);
	return manyWords(r.kind) && r.covers ? `${n} ${r.covers}` : n;
}
