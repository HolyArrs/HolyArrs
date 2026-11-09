// Shared unread, waiting and report counts for the inbox badge.

import { api } from './api.ts';
import type { AppMessage } from './types.ts';

// Reactive so every badge updates from one place.
export const inbox = $state<{ unread: number; waiting: number; reports: number; messages: AppMessage[] }>({ unread: 0, waiting: 0, reports: 0, messages: [] });

export async function refreshInbox(): Promise<void> {
	try {
		const r = await api.get<{ messages: AppMessage[]; unread: number; waiting?: number; reports_open?: number }>('/messages');
		inbox.messages = r.messages;
		inbox.unread = r.unread;
		inbox.waiting = r.waiting ?? 0;
		inbox.reports = r.reports_open ?? 0;
	} catch {
		// A failed refresh keeps the last counts so the badge does not flash to zero.
	}
}
