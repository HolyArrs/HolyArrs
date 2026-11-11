// Home's downloads tile: the counts, the steps and what finished lately.
import type { IconName } from '#lib/ui/Icon.svelte';
import { stageOf } from '#lib/downloads/index.ts';
import { homeCounts, n, notSentNow } from './home.ts';
import { bytes, percent } from '#lib/core/format.ts';
import type { DownloadRow } from '#lib/core/types.ts';
import type { Ask } from '#lib/core/types.ts';

import type { HomeContext } from './home-context.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createHomeDownloads(ctx: Pick<HomeContext, 'waiting' | 'askPic' | 'fetchingAll' | 'manager' | 'downloads'>) {
	const waitingPics = $derived(
		ctx.waiting
			.slice(0, 20)
			.map((a) => ({ a, pic: ctx.askPic(a) }))
			.filter((x): x is { a: Ask; pic: string } => !!x.pic)
			.slice(0, 3)
	);
	// Downloads, counted once and used everywhere: waiting (queued), downloading (bytes coming in, stalled and checking included), importing, and paused apart (a paused download is not in the list or the row). The tile, the note beside the switch and the row all read these.
	const speed = $derived(ctx.fetchingAll.reduce((t, d) => t + (d.speed || 0), 0));
	const pausedCount = $derived(ctx.manager ? ctx.downloads.filter((d) => homeCounts(d) && stageOf(d) === 'paused').length : 0);
	// Torrents the Downloader is fetching that HolyArrs did not send: the tile names them instead of saying nothing is downloading.
	const notSent = $derived(ctx.manager ? notSentNow(ctx.downloads).length : 0);
	function countStages(rows: DownloadRow[], paused = 0) {
		const c = { waiting: 0, down: 0, imp: 0, paused };
		for (const d of rows) {
			const st = stageOf(d);
			if (st === 'queued') c.waiting++;
			else if (st === 'importing') c.imp++;
			else c.down++;
		}
		return c;
	}
	const dlCount = $derived(countStages(ctx.fetchingAll, pausedCount));
	const stageWords = (c: { waiting: number; down: number; imp: number; paused: number }) =>
		[c.down ? t('home.downloading3', { n: n(c.down) }) : '', c.imp ? t('home.importing', { n: n(c.imp) }) : '', c.waiting ? t('home.waiting2', { n: n(c.waiting) }) : '', c.paused ? t('home.paused', { n: n(c.paused) }) : ''].filter(Boolean).join(' · ');
	/** One running download in a few words: its percent and speed, or the step it is in. */
	function dlState(d: DownloadRow): string {
		const st = stageOf(d);
		if (st === 'queued') return t('home.waiting3');
		if (st === 'importing') return t('home.importing2');
		if (st === 'checking') return t('home.checking');
		const pct = Math.round(percent(d.progress));
		if (st === 'stalled') return t('home.stalledAt', { pct: pct });
		return d.speed ? t('home.s', { pct: pct, speed: bytes(d.speed) }) : t('home.text2', { pct: pct });
	}
	// The tile lists what is moving first: downloading, then stalled, checking, importing, and waiting last.
	const STEP_ORDER: Record<string, number> = { downloading: 0, stalled: 1, checking: 2, importing: 3, queued: 4 };
	const dlList = $derived([...ctx.fetchingAll].sort((x, y) => (STEP_ORDER[stageOf(x)] ?? 9) - (STEP_ORDER[stageOf(y)] ?? 9)).slice(0, 3));
	// When nothing runs, the tile shows what finished last.
	const recentDone = $derived(
		ctx.manager
			? ctx.downloads
					.filter((d) => !d.client_item && !!d.finished_at && d.phase === 'done')
					.sort((x, y) => Date.parse(y.finished_at ?? '') - Date.parse(x.finished_at ?? ''))
					.slice(0, 3)
			: []
	);
	// Keyed by the words the pill shows, so it follows the language.
	const pillIcon = (): Record<string, IconName> => ({ [t('home.newSeason')]: 'sparkle', [t('home.newToday')]: 'sparkle', [t('home.newThisWeek')]: 'sparkle', [t('home.justArrived')]: 'check', [t('home.recentlyAdded2')]: 'check', [t('home.comingSoon2')]: 'calendar', [t('home.following')]: 'bell' });

	return {
		get dlCount() { return dlCount; },
		get speed() { return speed; },
		stageWords,
		get pillIcon() { return pillIcon(); },
		get waitingPics() { return waitingPics; },
		countStages,
		get dlList() { return dlList; },
		dlState,
		get recentDone() { return recentDone; },
		get pausedCount() { return pausedCount; },
		get notSent() { return notSent; },
	};
}
