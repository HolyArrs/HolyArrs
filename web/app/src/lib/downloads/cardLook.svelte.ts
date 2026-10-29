// The opened card's layout (a, b or c), shared by the A/B/C bar and the rows.
// The URL (?card=) is only the saved copy; reading it back is not reliable
// after a replaceState, so both sides read this.
export type CardLook = 'a' | 'b' | 'c' | 'd';

export function lookOf(v: string | null | undefined): CardLook {
	return v === 'a' || v === 'b' || v === 'd' ? v : 'c';
}

export const cardLook = $state<{ value: CardLook }>({ value: 'c' });
