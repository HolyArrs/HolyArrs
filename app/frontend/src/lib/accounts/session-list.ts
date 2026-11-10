// Signed-in devices: the session list the account and Users pages show.
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export interface SessionRow {
	id: string;
	device: string;
	created_at: string;
	last_used_at: string;
	expires_at: string;
	current: boolean;
}

/** Puts this device first, then the rest by last use (newest first). */
export function order(rows: SessionRow[]): SessionRow[] {
	return [...rows].sort((a, b) => {
		if (a.current !== b.current) return a.current ? -1 : 1;
		return b.last_used_at.localeCompare(a.last_used_at);
	});
}

/** Counts the sessions that are not this device. */
export function others(rows: SessionRow[]): number {
	return rows.filter((r) => !r.current).length;
}

/** How many sessions a sign-out ended. */
export function endedWords(n: number): string {
	if (n === 0) return t('accounts.sessions.none');
	return n === 1 ? t('accounts.sessions.one') : t('accounts.sessions.many', { n });
}
