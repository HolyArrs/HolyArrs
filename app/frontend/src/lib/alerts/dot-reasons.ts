// What lights the red dot on the menu, one short line each. The dot and its list both come from here, so they never disagree; the Settings lines are the same ones the bell and Home's Needs attention read (the server's health checks).
import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { sections } from '../core/settings.ts';
import { pinned } from './attention-alerts.ts';
import type { Attention } from './settings-status.ts';

export interface DotReason {
	key: string;
	title: string;
	body?: string;
	tone: 'fail' | 'warn' | 'info';
	/** The page that fixes it; none opens the alerts. */
	href?: string;
}

export interface DotInput {
	/** Open Settings items (empty without the settings right). */
	settings: Attention[];
	decisions: number;
	waiting: number;
	unseen: number;
	/** Decisions, requests and Settings only light the dot for people who manage. */
	manager: boolean;
}

const rank = { fail: 0, warn: 1, info: 2 } as const;

export function dotReasons(input: DotInput): DotReason[] {
	const out: DotReason[] = [];
	if (input.manager) {
		const items = pinned(input.settings, sections).sort((a, b) => rank[a.tone] - rank[b.tone]);
		for (const p of items) out.push({ key: p.key, title: p.title, body: p.body, tone: p.tone, href: p.href });
		if (input.decisions > 0) out.push({ key: 'decisions', title: t('alerts.dot.decide', { n: input.decisions }), tone: 'warn', href: '/downloads/decide' });
		if (input.waiting > 0) out.push({ key: 'waiting', title: t('alerts.dot.waiting', { n: input.waiting }), tone: 'info', href: '/requests' });
	}
	if (input.unseen > 0) out.push({ key: 'alerts', title: t('alerts.dot.unseen', { n: input.unseen }), tone: 'info' });
	return out;
}
