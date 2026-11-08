// Short words for alerts that come from outside the page: request messages and reasons.
import { looksLikeId } from '#lib/core/plain.ts';
import type { AppMessage } from '#lib/core/types.ts';

/** Turns a request message into a title and a second line. */
export function messageWords(m: Pick<AppMessage, 'kind' | 'text'>): { title: string; body?: string; tone: 'ok' | 'fail' | 'info' } {
	const head = { approved: 'Request approved', turned_down: 'Request turned down', ready: 'Ready to watch', fixed: 'Problem fixed' }[m.kind] ?? 'Request updated';
	return { title: head, body: cut(plain(m.text)), tone: m.kind === 'turned_down' ? 'fail' : 'ok' };
}

// Ids in a message read as a person, as on the Requests page.
const plain = (t: string) => t.replace(/[\w.-]+/g, (w) => (/\d/.test(w) && /\p{L}/u.test(w) && looksLikeId(w) ? 'someone in your household' : w));

// Second lines stay short: the first sentence, at most about 60 characters.
export function cut(s: string): string {
	const first = s.split(/(?<=[.!?])\s/)[0].replace(/\.$/, '');
	return first.length > 64 ? `${first.slice(0, 61).trimEnd()}…` : first;
}
