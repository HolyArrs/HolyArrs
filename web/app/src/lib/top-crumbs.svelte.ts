// Where-am-I links a page puts in the top bar, after the back and forward
// buttons (desktop only). The last item is the page itself (no link).
export type TopCrumb = { label: string; href?: string };

export const topCrumbs = $state<{ items: TopCrumb[] }>({ items: [] });

export function setTopCrumbs(items: TopCrumb[]) {
	topCrumbs.items = items;
}
