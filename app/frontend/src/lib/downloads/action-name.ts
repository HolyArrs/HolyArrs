// The accessible name starts with the visible label (WCAG 2.5.3); a longer hint follows it.
// A hint that already holds the label ("Start (select a download)", "Speed limit: 42 MB/s") is the whole name.
export const actionName = (a: { label: string; hint: string }): string =>
	!a.hint || a.hint === a.label ? a.label : a.hint.includes(a.label) ? a.hint : `${a.label}: ${a.hint}`;
