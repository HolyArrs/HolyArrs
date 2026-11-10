// The movie page's search, the failed download mark and the refresh.
import type { TitleView } from '#lib/core/types.ts';
import { api, message } from '#lib/core/api.ts';
import { toasts } from '#lib/core/toasts.svelte.ts';
import { scrollToStart } from '#lib/core/motion.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createTitleSearch(ctx: { id: number; title: TitleView | null; slotPick: number; searching: boolean; running: number[]; relOffset: number; refreshing: boolean; load: (which?: 'all' | 'title' | 'releases') => Promise<void> }) {
	let failing = $state(0);
	async function markFailed(downloadID: number) {
		if (!confirm(t('mediafiles.title.markConfirm'))) return;
		failing = downloadID;
		try {
			await api.post(`/downloads/${downloadID}/mark-failed`);
			toasts.push(t('mediafiles.title.markedFailed'), 'ok', { body: t('mediafiles.title.searchingNext') });
			await ctx.load('title');
		} catch (e) {
			toasts.push(t('mediafiles.title.markFailed'), 'fail', { body: message(e) });
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
			toasts.push(t('mediafiles.title.searching'), 'info', pick ? undefined : { body: t('mediafiles.title.pickFromList') });
		} catch (e) {
			toasts.push(t('mediafiles.title.startFailed'), 'fail', { body: message(e) });
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
			toasts.push(t('mediafiles.title.refreshing', { name: ctx.title?.name ?? t('mediafiles.title.titleLower') }), 'info', { body: t('mediafiles.title.refreshingBody') });
		} catch (e) {
			toasts.push(t('mediafiles.title.refreshFailed'), 'fail', { body: message(e) });
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
