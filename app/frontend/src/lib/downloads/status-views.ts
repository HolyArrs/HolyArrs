// Words for the live status at the right of the Downloads status tab row.
import { stageOf } from './status-tabs.ts';

type RowBits = Parameters<typeof stageOf>[0];

/** Counts the rows the Review status (Problems > Review) holds. */
export function reviewCount(rows: RowBits[]): number {
	return rows.filter((r) => stageOf(r) === 'decide').length;
}

/** The status line's call to action, with its space kept: "2 to review". */
export function reviewWord(n: number): string {
	return `${n} to review`;
}

/** When everything still on its way finishes: "25m left". `eta` is the short duration ("25m"); an empty eta means nothing is moving. */
export function timeWords(eta: string): string {
	return eta ? `${eta} left` : 'Nothing moving';
}
