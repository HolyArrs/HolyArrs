import { api } from './api.ts';
import type { AppMessage } from './types.ts';

export const inbox = $state<{ unread: number; waiting: number; reports: number; messages: AppMessage[] }>({ unread: 0, waiting: 0, reports: 0, messages: [] });

/** refreshInbox reads the messages and counts; errors leave them as they are. */
export async function refreshInbox(): Promise<void> {
	try {
		const r = await api.get<{ messages: AppMessage[]; unread: number; waiting?: number; reports_open?: number }>('/messages');
		inbox.messages = r.messages;
		inbox.unread = r.unread;
		inbox.waiting = r.waiting ?? 0;
		inbox.reports = r.reports_open ?? 0;
	} catch {
		// Keep the last counts.
	}
}
