// Settings health warnings (GET /settings/status) as pinned rows in Recent alerts:
// the same title + second line pattern as every alert, plus a button to the page.
import type { Attention } from './settings-status.ts';

export interface PinnedAlert {
	key: string;
	title: string;
	body?: string;
	tone: 'fail' | 'warn' | 'info';
	href: string;
	/** The page's name for the button, such as "Backups". */
	label: string;
}

const upper = (s: string) => (s ? s[0].toUpperCase() + s.slice(1) : s);

/**
 * pinned turns status items into rows. The server writes "Page: Title: next step";
 * the page prefix is dropped (the button already names it), the rest splits into a
 * title and a second line.
 */
export function pinned(items: Attention[], pages: { href: string; label: string }[]): PinnedAlert[] {
	return items.map((i) => {
		const label = pages.find((p) => p.href === i.href)?.label ?? 'Settings';
		let text = i.text.trim();
		if (text.toLowerCase().startsWith(`${label.toLowerCase()}: `)) text = text.slice(label.length + 2);
		const cut = text.indexOf(': ');
		const title = upper((cut > 0 ? text.slice(0, cut) : text).replace(/\.$/, ''));
		const body = cut > 0 ? upper(text.slice(cut + 2).replace(/\.$/, '')) : undefined;
		const tone = i.severity === 'error' ? 'fail' : i.severity === 'warning' ? 'warn' : 'info';
		return { key: `${i.href}|${i.text}`, title, body, tone, href: i.href, label };
	});
}
