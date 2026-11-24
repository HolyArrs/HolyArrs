// Explore row stats for the owner and admins: how often each row had a card opened or a request started, counted on this server only and summed over everyone.

/** One row type's sums, as GET /explore/row-stats answers. */
export interface RowStat {
	row_type: string;
	/** The row's fixed server title; missing when the title names a title, person, genre or decade. */
	title?: string;
	shown: number;
	opens: number;
	asks: number;
}

/** Rows the server cannot give one fixed title, so the page names them itself. */
export const NAMED_ROWS = ['because_have', 'because_asked', 'person', 'genre', 'popular_genre', 'popular_decade', 'decade', 'mood', 'hero'] as const;

/** Table lines in the server's order (busiest first). words shows a server line in the picked language; own names a row in NAMED_ROWS. */
export function rowStatLines(rows: RowStat[], words: (s: string) => string, own: (type: string) => string) {
	return rows.map((r) => ({
		key: r.row_type,
		row: (NAMED_ROWS as readonly string[]).includes(r.row_type) ? own(r.row_type) : r.title ? words(r.title) : r.row_type,
		shown: r.shown,
		opens: r.opens,
		asks: r.asks
	}));
}
