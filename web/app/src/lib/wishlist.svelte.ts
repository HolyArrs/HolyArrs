// The Wishlist badge: unread messages for the person, and asks waiting for
// people who approve. Asked again on page changes and every minute.

import { api } from './api.ts';
import type { AppMessage } from './types.ts';

export const inbox = $state<{ unread: number; waiting: number; messages: AppMessage[] }>({ unread: 0, waiting: 0, messages: [] });

/** refreshInbox reads the messages and counts; errors leave them as they are. */
export async function refreshInbox(): Promise<void> {
	try {
		const r = await api.get<{ messages: AppMessage[]; unread: number; waiting?: number }>('/messages');
		inbox.messages = r.messages;
		inbox.unread = r.unread;
		inbox.waiting = r.waiting ?? 0;
	} catch {
		// keep the last counts
	}
}

/** markRead marks every message read. */
export async function markRead(): Promise<void> {
	await api.post('/messages/read');
	inbox.unread = 0;
	inbox.messages = inbox.messages.map((m) => ({ ...m, read_at: m.read_at ?? new Date().toISOString() }));
}
