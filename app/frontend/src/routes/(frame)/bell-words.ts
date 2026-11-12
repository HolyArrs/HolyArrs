// Short words for alerts that come from outside the page: request messages and reasons.
import { looksLikeId } from '#lib/core/plain.ts';
import type { AppMessage, HealthCheck } from '#lib/core/types.ts';
import type { PinnedAlert } from '#lib/alerts/index.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

/** Turns a request message into a title and a second line. */
export function messageWords(m: Pick<AppMessage, 'kind' | 'text'>): { title: string; body?: string; tone: 'ok' | 'fail' | 'info' } {
	const head = { approved: t('frame.bell.approved'), turned_down: t('frame.bell.turnedDown'), ready: t('frame.bell.ready'), fixed: t('frame.bell.fixed') }[m.kind] ?? t('frame.bell.updated');
	return { title: head, body: cut(plain(m.text)), tone: m.kind === 'turned_down' ? 'fail' : 'ok' };
}

// Ids in a message read as a person, as on the Requests page.
const plain = (text: string) => text.replace(/[\w.-]+/g, (w) => (/\d/.test(w) && /\p{L}/u.test(w) && looksLikeId(w) ? t('frame.bell.household') : w));

// Second lines stay short: the first sentence, at most about 60 characters.
export function cut(s: string): string {
	const first = s.split(/(?<=[.!?])\s/)[0].replace(/\.$/, '');
	return first.length > 64 ? `${first.slice(0, 61).trimEnd()}…` : first;
}

/** Imported downloads whose clear from the Downloader failed: one pinned row with Clear now, until every clear worked. */
export function clearPin(checks: Pick<HealthCheck, 'kind' | 'state' | 'message'>[], go: () => Promise<void>): PinnedAlert | null {
	const c = checks.find((x) => x.kind === 'clear_waiting');
	if (!c || c.state === 'ok') return null;
	// The server line starts with the count; a line without one reads as one.
	const n = parseInt(c.message, 10) || 1;
	return {
		key: 'clear_waiting',
		title: n === 1 ? t('frame.feeds.clearOne') : t('frame.feeds.clearMany', { n }),
		body: t('frame.feeds.clearBody'),
		tone: 'warn',
		href: '/downloads',
		label: t('frame.feeds.downloads'),
		run: { label: t('frame.feeds.clearNow'), go }
	};
}
