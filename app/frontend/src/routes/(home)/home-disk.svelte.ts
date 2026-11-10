// Home's disk tile: the free space, its words and its tint.
import { freeSpace, n, shownState } from './home.ts';
import { bytes } from '#lib/core/format.ts';
import type { Tint } from '#lib/core/settings.ts';

import type { HomeContext } from './home-context.ts';
import { t } from '#lib/core/i18n/i18n.svelte.ts';

export function createHomeDisk(ctx: Pick<HomeContext, 'checks' | 'folders' | 'uncheckedFolders' | 'diskTotal' | 'settled' | 'worst' | 'fixes' | 'unchecked' | 'uncheckedDl' | 'dlCount' | 'speed' | 'stageWords'>) {
	const diskState = $derived.by(() => {
		const st = (ctx.checks?.checks ?? []).filter((c) => c.kind === 'disk_space').map((c) => shownState(c));
		return st.includes('error') ? 'error' : st.includes('warning') ? 'warning' : 'ok';
	});
	const free = $derived(freeSpace(ctx.folders, diskState !== 'ok'));
	const diskLine = $derived(
		!free
			? ctx.uncheckedFolders
				? t('home.couldNotReadThe')
				: diskState === 'ok'
				? t('home.acrossYourLibraryFolders')
				: t('home.aDiskIsRunning')
			: free.free <= 0
				? free.many
					? t('home.aDiskIsFull')
					: t('home.theDiskIsFull')
				: free.many
					? diskState === 'ok'
						? t('home.freeOnTheFullest', { size: bytes(free.free) })
						: t('home.onlyFreeOnThe', { size: bytes(free.free) })
					: diskState === 'ok'
						? t('home.free', { size: bytes(free.free) })
						: t('home.onlyFree', { size: bytes(free.free) })
	);
	const diskTint = $derived<Tint>(diskState === 'error' ? 'red' : diskState === 'warning' ? 'amber' : 'teal');
	const diskUsed = $derived(!free ? 0 : free.free <= 0 ? 100 : Math.min(100, Math.max(2, (ctx.diskTotal / Math.max(1, ctx.diskTotal + free.free)) * 100)));
	const sysOk = $derived(ctx.settled && ctx.worst === 'ok');
	const sysNote = $derived(
		!ctx.settled
			? ''
			: (ctx.fixes.length ? t(ctx.fixes.length === 1 ? 'home.thingNeedsALook' : 'home.thingsNeedALook', { n: n(ctx.fixes.length) }) : ctx.unchecked ? t('home.couldNotCheckEverything') : t('home.allGood')) +
					(ctx.uncheckedDl ? '' : ctx.dlCount.down ? ` · ${ctx.speed ? t('home.downloadingAtS', { n: n(ctx.dlCount.down), speed: bytes(ctx.speed) }) : t('home.downloading3', { n: n(ctx.dlCount.down) })}` : ctx.dlCount.imp || ctx.dlCount.waiting || ctx.dlCount.paused ? ` · ${ctx.stageWords(ctx.dlCount)}` : '')
	);

	/** A server-filled row's strip: what is new on the card, in the row's own kind. */

	return {
		get sysNote() { return sysNote; },
		get free() { return free; },
		get diskTint() { return diskTint; },
		get diskState() { return diskState; },
		get diskLine() { return diskLine; },
		get diskUsed() { return diskUsed; },
	};
}
