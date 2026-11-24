
// Types and wording for files held after a delete, before they are removed for good.
import { t } from '../core/i18n/i18n.svelte.ts';

export interface HeldFile {
	id: number;
	path: string;
	held_path: string;
	folder: string;
	title_id: number;
	title: string;
	kind: string;
	title_deleted: boolean;
	size: number;
	deleted_at: string;
	expires_at?: string;
	replaced?: boolean;
	replaced_by?: string;
}

export interface RestoreResult {
	id: number;
	path?: string;
	result: string;
	/** Another file sits where it was: ask, then restore again with how. */
	taken?: boolean;
	/** The file there is a tracked copy that can trade places with it. */
	can_swap?: boolean;
}

/** What to do when the place is taken: swap sends the copy there to Recently deleted, both restores it beside it. */
export type RestoreHow = '' | 'swap' | 'both';

/** Which held files a reason filter keeps. */
export function heldOfReason(files: HeldFile[], reason: 'all' | 'deleted' | 'replaced'): HeldFile[] {
	return reason === 'all' ? files : files.filter((f) => !!f.replaced === (reason === 'replaced'));
}

/** The soonest expiry among held files (undefined: one is kept until emptied, or none). */
export function soonestExpiry(files: HeldFile[]): string | undefined {
	let at: string | undefined;
	for (const f of files) {
		if (!f.expires_at) return undefined;
		if (!at || Date.parse(f.expires_at) < Date.parse(at)) at = f.expires_at;
	}
	return at;
}

// Null means the held file has no expiry.
export function daysLeft(expires: string | undefined, now = Date.now()): number | null {
	if (!expires) return null;
	const at = Date.parse(expires);
	if (Number.isNaN(at)) return null;
	return Math.max(0, Math.ceil((at - now) / 86_400_000));
}

export function daysLeftWords(expires: string | undefined, now = Date.now()): string {
	const d = daysLeft(expires, now);
	if (d === null) return t('mediafiles.holding.untilEmpty');
	if (d === 0) return t('mediafiles.holding.today');
	return d === 1 ? t('mediafiles.holding.dayLeft') : t('mediafiles.holding.daysLeft', { n: d });
}

// Held paths use forward slashes.
export function fileName(path: string): string {
	return path.slice(path.lastIndexOf('/') + 1);
}
