// The left rail's look (?rail=a|b|c), shared by the switcher bar and the rail.
// The URL is only the saved copy; both sides read this.
export type RailLook = 'a' | 'b' | 'c';

export function railOf(v: string | null | undefined): RailLook {
	return v === 'b' || v === 'c' ? v : 'a';
}

export const railLook = $state<{ value: RailLook }>({ value: 'a' });
