// Shared state for the breadcrumb shown in the top bar.
export type TopCrumb = { label: string; href: string };

export const topCrumbs = $state<{ parent: TopCrumb | null; here: string }>({ parent: null, here: '' });

export function setTopCrumbs(parent: TopCrumb | null, here = '') {
	topCrumbs.parent = parent;
	topCrumbs.here = here;
}
