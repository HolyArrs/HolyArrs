import { t } from '#lib/core/i18n/i18n.svelte.ts';
import type { Attention } from './settings-status.ts';

export interface PinnedAlert {
	key: string;
	title: string;
	body?: string;
	tone: 'fail' | 'warn' | 'info';
	href: string;
	label: string;
}

// Server text looks like "Page: Title: next step". The page prefix is dropped because the button names it.
export function pinned(items: Attention[], pages: { href: string; label: string }[]): PinnedAlert[] {
	return items.map((i) => {
		const label = pages.find((p) => p.href === i.href)?.label ?? t('alerts.fallback.settings');
		let text = i.text.trim();
		if (text.toLowerCase().startsWith(`${label.toLowerCase()}: `)) text = text.slice(label.length + 2);
		const cut = text.indexOf(': ');
		const title = upper((cut > 0 ? text.slice(0, cut) : text).replace(/\.$/, ''));
		const body = cut > 0 ? upper(text.slice(cut + 2).replace(/\.$/, '')) : undefined;
		const tone = i.severity === 'error' ? 'fail' : i.severity === 'warning' ? 'warn' : 'info';
		return { key: `${i.href}|${i.text}`, title, body, tone, href: i.href, label };
	});
}

const upper = (s: string) => (s ? s[0].toUpperCase() + s.slice(1) : s);
