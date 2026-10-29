// How the five status tabs look (?tabs=1..5), shared by the switcher bar and the Downloads list.
// The URL is only the saved copy; both sides read this.
export type TabsLook = '1' | '2' | '3' | '4' | '5';

export function tabsOf(v: string | null | undefined): TabsLook {
	return v === '2' || v === '3' || v === '4' || v === '5' ? v : '1';
}

export const tabsLook = $state<{ value: TabsLook }>({ value: '1' });
