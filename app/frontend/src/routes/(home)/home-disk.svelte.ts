// Home's disk tile: the free space, its words and its tint.
import { freeSpace, n, shownState } from './home.ts';
import { bytes } from '#lib/core/format.ts';
import type { Tint } from '#lib/core/settings.ts';

import type { HomeContext } from './home-context.ts';

export function createHomeDisk(ctx: Pick<HomeContext, 'checks' | 'folders' | 'uncheckedFolders' | 'diskTotal' | 'settled' | 'worst' | 'fixes' | 'unchecked' | 'uncheckedDl' | 'dlCount' | 'speed' | 'stageWords'>) {
	const diskState = $derived.by(() => {
		const st = (ctx.checks?.checks ?? []).filter((c) => c.kind === 'disk_space').map((c) => shownState(c));
		return st.includes('error') ? 'error' : st.includes('warning') ? 'warning' : 'ok';
	});
	const free = $derived(freeSpace(ctx.folders, diskState !== 'ok'));
	const diskLine = $derived(
		!free
			? ctx.uncheckedFolders
				? 'Could not read the free room'
				: diskState === 'ok'
				? 'Across your Library folders'
				: 'A disk is running out of room'
			: free.free <= 0
				? free.many
					? 'A disk is full'
					: 'The disk is full'
				: `${diskState === 'ok' ? '' : 'Only '}${bytes(free.free)} free${free.many ? ' on the fullest disk' : ''}`
	);
	const diskTint = $derived<Tint>(diskState === 'error' ? 'red' : diskState === 'warning' ? 'amber' : 'teal');
	const diskUsed = $derived(!free ? 0 : free.free <= 0 ? 100 : Math.min(100, Math.max(2, (ctx.diskTotal / Math.max(1, ctx.diskTotal + free.free)) * 100)));
	const sysOk = $derived(ctx.settled && ctx.worst === 'ok');
	const sysNote = $derived(
		!ctx.settled
			? ''
			: (ctx.fixes.length ? `${n(ctx.fixes.length)} ${ctx.fixes.length === 1 ? 'thing needs' : 'things need'} a look` : ctx.unchecked ? 'Could not check everything' : 'All good') +
					(ctx.uncheckedDl ? '' : ctx.dlCount.down ? ` · ${n(ctx.dlCount.down)} downloading${ctx.speed ? ` at ${bytes(ctx.speed)}/s` : ''}` : ctx.dlCount.imp || ctx.dlCount.waiting || ctx.dlCount.paused ? ` · ${ctx.stageWords(ctx.dlCount)}` : '')
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
