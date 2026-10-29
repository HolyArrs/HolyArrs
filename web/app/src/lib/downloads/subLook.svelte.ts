// How the sub-statuses look (?sub=a..g), shared by the switcher bar and the Downloads list.
// The URL is only the saved copy; both sides read this.
export type SubLook = 'a' | 'b' | 'c' | 'd' | 'e' | 'f' | 'g';

export function subOf(v: string | null | undefined): SubLook {
	return v === 'b' || v === 'c' || v === 'd' || v === 'e' || v === 'f' || v === 'g' ? v : 'a';
}

export const subLook = $state<{ value: SubLook }>({ value: 'a' });
