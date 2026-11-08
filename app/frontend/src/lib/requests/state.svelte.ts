import { api } from '../core/api.ts';
import { inbox } from '../core/inbox.svelte.ts';

/** Marks every message read. */
export async function markRead(): Promise<void> {
	await api.post('/messages/read');
	inbox.unread = 0;
	inbox.messages = inbox.messages.map((m) => ({ ...m, read_at: m.read_at ?? new Date().toISOString() }));
}
