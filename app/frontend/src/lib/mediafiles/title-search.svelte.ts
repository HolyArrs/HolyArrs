// The movie page's search, the failed download mark and the refresh.
import type { TitleView } from '#lib/core/types.ts';
import { api, message } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { scrollToStart } from '#lib/core/motion.ts';

export function createTitleSearch(ctx: { id: number; title: TitleView | null; slotPick: number; searching: boolean; running: number[]; relOffset: number; refreshing: boolean; load: (which?: 'all' | 'title' | 'releases') => Promise<void> }) {
	let failing = $state(0);
	async function markFailed(downloadID: number) {
		if (!confirm('Mark this download failed? Its Release is turned down, it is removed from the Downloader and HolyArrs looks for the next Release.')) return;
		failing = downloadID;
		try {
			await api.post(`/downloads/${downloadID}/mark-failed`);
			toasts.push('Marked failed', 'ok', { body: 'Searching for the next Release' });
			await ctx.load('title');
		} catch (e) {
			toasts.push(`Couldn't mark failed`, 'fail', { body: message(e) });
		} finally {
			failing = 0;
		}
	}

	async function searchNow(pick = true) {
		ctx.searching = true;
		try {
			const body: Record<string, unknown> = {};
			if (!pick) body.choose = false;
			if (ctx.slotPick) body.slot_id = ctx.slotPick;
			const r = await api.post<{ job_ids: number[] }>(`/titles/${ctx.id}/search`, body);
			ctx.running = [...new Set([...ctx.running, ...r.job_ids])];
			ctx.relOffset = 0;
			toasts.push('Searching for a Release', 'info', pick ? undefined : { body: 'Pick one from the list' });
		} catch (e) {
			toasts.push(`Couldn't start search`, 'fail', { body: message(e) });
		} finally {
			setTimeout(() => (ctx.searching = false), 1500);
		}
	}

	// More > Choose a release: bring the Releases section into view and start the same pick-it-yourself search as its own button.
	function chooseRelease() {
		const h = document.getElementById('h-rel');
		scrollToStart(h?.closest('section'));
		h?.focus({ preventScroll: true });
		void searchNow(false);
	}

	async function refresh() {
		ctx.refreshing = true;
		try {
			await api.post(`/titles/${ctx.id}/refresh`);
			toasts.push(`Refreshing ${ctx.title?.name ?? 'title'}`, 'info', { body: 'Names, dates and posters' });
		} catch (e) {
			toasts.push(`Couldn't refresh`, 'fail', { body: message(e) });
		} finally {
			setTimeout(() => (ctx.refreshing = false), 1500);
		}
	}

	return {
		searchNow,
		chooseRelease,
		refresh,
		markFailed,
		get failing() { return failing; },
		set failing(v) { failing = v; },
	};
}
