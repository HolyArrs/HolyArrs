// The library check for titles with files that stream on the owner's services.

import { t } from '#lib/core/i18n/i18n.svelte.ts';
import { bytes } from '#lib/core/format.ts';

/** A title with files on one of the owner's services, from GET /settings/asks/streaming-cleanup. */
export interface OnServicesTitle {
	id: number;
	kind: 'movie' | 'show';
	name: string;
	year?: number;
	tmdb_id: string;
	bytes: number;
	files: number;
	service: string;
}

export interface CleanupCheck {
	running: boolean;
	checked: number;
	total: number;
	failed: number;
	region: string;
	found: OnServicesTitle[];
	started_at?: string;
	finished_at?: string;
}

/** The one line under the check button: never run, running, or what it found. */
export function cleanupLine(c: CleanupCheck | null): string {
	if (!c || (!c.running && !c.finished_at)) return '';
	if (c.running) return t('settings.requests.cleanupRunning', { n: c.checked, total: c.total });
	const size = c.found.reduce((n, f) => n + f.bytes, 0);
	let line = c.found.length ? t('settings.requests.cleanupFound', { n: c.found.length, size: bytes(size) }) : t('settings.requests.cleanupNothing', { total: c.total });
	if (c.failed) line += ' ' + t('settings.requests.cleanupFailed', { n: c.failed });
	return line;
}

