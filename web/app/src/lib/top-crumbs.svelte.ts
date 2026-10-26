// The where-am-I line in the top bar, after the back and forward buttons
// (desktop only), the same on every detail page: the page you came from
// (stepping back there with its scroll and state), then this page. A page
// with no history (opened from a link) shows its parent instead, so there is
// always a way out.
export type TopCrumb = { label: string; href: string };

export const topCrumbs = $state<{ parent: TopCrumb | null; here: string }>({ parent: null, here: '' });

export function setTopCrumbs(parent: TopCrumb | null, here = '') {
	topCrumbs.parent = parent;
	topCrumbs.here = here;
}
