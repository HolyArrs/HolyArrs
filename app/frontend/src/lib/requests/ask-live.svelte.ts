// Live download and file steps for the requests on screen.
import { liveByTitle, type TitleLive } from './live.ts';
import { onDestroy } from 'svelte';
import { api } from '#lib/core/api.ts';
import { session } from '#lib/core/session.svelte.ts';
import type { SearchView } from '#lib/core/types.ts';
import type { DownloadRow } from '#lib/core/types.ts';

export function createAskLive(ctx: any) {
	let dlSoon: ReturnType<typeof setTimeout> | undefined;
	let slotSoon: ReturnType<typeof setTimeout> | undefined;
	let stepReadAt = 0;
	function onDlPatch(data: unknown) {
		const p = data as { upsert?: unknown[]; remove?: unknown[] } | null;
		if (!p || (!p.upsert?.length && !p.remove?.length) || dlSoon) return;
		dlSoon = setTimeout(() => {
			dlSoon = undefined;
			void loadLive();
			// The person's own ask steps move with their downloads (one read in 10 s at most).
			if (!ctx.approver && Date.now() - stepReadAt > 10_000) {
				stepReadAt = Date.now();
				void ctx.load(true);
			}
		}, 1500);
	}
	onDestroy(() => (clearTimeout(dlSoon), clearTimeout(slotSoon)));

	// Where approved asks are in the pipeline (local lists, refreshed on the same live events as the sidebar).
	let titleLive = $state(new Map<number, TitleLive>());
	let liveBusy = false;
	let liveAgain = false;
	async function loadLive() {
		// Members read only the downloads of their own asks (the server keeps it to those), so they see why one failed; the searches list is for people who manage the Library.
		const staff = session.can('manage_library');
		// A live event during a read asks for one more read once it ends, so the last change is never lost.
		if (liveBusy) return void (liveAgain = true);
		liveBusy = true;
		try {
			const [d, s] = await Promise.all([
				// Every download still in play for an approved ask (no row cap), so the failed count is exact.
				api.get<{ downloads: DownloadRow[] }>('/downloads/view?asked=1'),
				staff ? api.get<{ running: SearchView[] }>('/searches') : Promise.resolve({ running: [] as SearchView[] })
			]);
			titleLive = liveByTitle(d.downloads, s.running ?? []);
		} catch {
			// Keep the last answer.
		} finally {
			liveBusy = false;
			if (liveAgain) {
				liveAgain = false;
				void loadLive();
			}
		}
	}

	return {
		loadLive,
		get slotSoon() { return slotSoon; },
		set slotSoon(v) { slotSoon = v; },
		onDlPatch,
		get titleLive() { return titleLive; },
		set titleLive(v) { titleLive = v; },
	};
}
