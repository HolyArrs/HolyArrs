// Active downloads for the sidebar: how many are moving and their average
// progress. Same source as the Downloads page (/downloads/view).

import { api } from './core/api.ts';
import { nowPhases } from './downloaders.ts';
import { type DownloadRow } from './core/types.ts';

export const activeDownloads = $state<{ count: number; percent: number }>({ count: 0, percent: 0 });


/** summarise counts the rows on the Downloads page's "Now" list and averages their progress as a whole percent. */
export function summarise(rows: Pick<DownloadRow, 'phase' | 'progress'>[]): { count: number; percent: number } {
	const now = rows.filter((r) => nowPhases.has(r.phase));
	if (now.length === 0) return { count: 0, percent: 0 };
	const sum = now.reduce((a, r) => a + Math.min(Math.max(r.progress, 0), 1), 0);
	return { count: now.length, percent: Math.floor((sum / now.length) * 100) };
}

/** refreshActiveDownloads reads the list; errors leave the last answer. */
export async function refreshActiveDownloads(): Promise<void> {
	try {
		const r = await api.get<{ downloads: DownloadRow[] }>('/downloads/view?limit=100');
		const s = summarise(r.downloads);
		activeDownloads.count = s.count;
		activeDownloads.percent = s.percent;
	} catch {
		// keep the last answer
	}
}

/** resetActiveDownloads clears the answer (signed out, or not allowed). */
export function resetActiveDownloads(): void {
	activeDownloads.count = 0;
	activeDownloads.percent = 0;
}
