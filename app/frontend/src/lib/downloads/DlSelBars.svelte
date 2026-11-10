<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Selection bars for ticked downloads: a toolbar above the list on wide screens, a floating sheet on phones.
	import Icon from '#lib/ui/Icon.svelte';
	import type { BulkAction, SteerMode } from './manage.ts';
	import { retryable } from './usenet.ts';
	import type { DownloadRow } from '../core/types.ts';

	type Targets = { rows: DownloadRow[] };
	let {
		pickedRows,
		bulkRows,
		steer,
		startT,
		pauseT,
		batchBusy = false,
		steerable,
		mayEdit,
		runOn,
		onbulk,
		onsteer,
		onretry,
		onremovemany,
		onall,
		onclear
	}: {
		pickedRows: DownloadRow[];
		/** Ticked torrents a torrent Downloader can steer by hash. */
		bulkRows: DownloadRow[];
		steer: { mode: SteerMode; label: string; short: string }[];
		startT: Targets;
		pauseT: Targets;
		batchBusy?: boolean;
		steerable: (r: DownloadRow) => boolean;
		mayEdit: (r: DownloadRow) => boolean;
		runOn: (t: Targets, one: 'resume' | 'pause', many: BulkAction) => void;
		onbulk?: (rows: DownloadRow[], action: BulkAction) => void;
		onsteer?: (rows: DownloadRow[], mode: SteerMode) => void;
		onretry?: (rows: DownloadRow[]) => void;
		onremovemany?: (rows: DownloadRow[], withFiles?: boolean) => void;
		onall: () => void;
		onclear: () => void;
	} = $props();

	const PHONE_BULK = $derived<{ action: BulkAction; label: string; icon: 'play' | 'pause' | 'refresh' }[]>([{ action: 'start', label: t('downloads.act.start'), icon: 'play' }, { action: 'pause', label: t('downloads.act.pause'), icon: 'pause' }, { action: 'recheck', label: t('downloads.act.checkAgain'), icon: 'refresh' }]);
	const BULK = $derived<{ action: BulkAction; label: string }[]>([
		{ action: 'recheck', label: t('downloads.act.checkAgain') }, { action: 'reannounce', label: t('downloads.act.askTrackers') }, { action: 'top', label: t('downloads.act.top') },
		{ action: 'up', label: t('downloads.act.up') }, { action: 'down', label: t('downloads.act.down') }, { action: 'bottom', label: t('downloads.act.bottom') }, { action: 'force', label: t('downloads.act.forceStart') }
	]);
</script>

	<div class="selbar" role="toolbar" aria-label={t('downloads.sel.label')} data-testid="dl-selbar">
		<span class="sn" aria-live="polite"><Words key="downloads.sel.selectedCount" vars={{ n: pickedRows.length }} /></span>
		{#if bulkRows.length}{#each BULK as b (b.action)}<button type="button" class="xbn" disabled={batchBusy} onclick={() => onbulk?.(bulkRows, b.action)} data-testid="dl-bulk-{b.action}">{b.label}</button>{/each}{/if}
		{#if pickedRows.some(steerable)}{#each steer as m (m.mode)}<button type="button" class="xbn" onclick={() => onsteer?.(pickedRows.filter(steerable), m.mode)} data-testid="dl-bulk-steer-{m.mode}">{m.short}</button>{/each}{/if}
		{#if pickedRows.some((r) => retryable(r) && mayEdit(r))}<button type="button" class="xbn" disabled={batchBusy} onclick={() => onretry?.(pickedRows.filter((r) => retryable(r) && mayEdit(r)))} data-testid="dl-bulk-retry">{t('downloads.act.again')}</button>{/if}
		<button type="button" class="xbn" onclick={onall} data-testid="dl-bulk-all">{t('downloads.sel.all')}</button>
		<button type="button" class="xbn" onclick={onclear} data-testid="dl-bulk-clear">{t('downloads.sel.clear')}</button>
	</div>
		<div class="psel" role="toolbar" aria-label={t('downloads.sel.label')} data-testid="dl-psel">
			<div class="pst"><span class="sn" aria-live="polite"><Words key="downloads.sel.selectedCount" vars={{ n: pickedRows.length }} /></span><button type="button" class="plk" onclick={onall} data-testid="dl-psel-all">{t('downloads.sel.all')}</button><button type="button" class="plk" onclick={onclear} data-testid="dl-psel-cancel">{t('downloads.sel.cancel')}</button></div>
			<div class="psa">
				{#each PHONE_BULK.filter((b) => b.action !== 'recheck' || bulkRows.length) as b (b.action)}<button type="button" class="pa" disabled={batchBusy || (b.action === 'start' ? !startT.rows.length : b.action === 'pause' ? !pauseT.rows.length : false)} onclick={() => (b.action === 'start' ? runOn(startT, 'resume', 'start') : b.action === 'pause' ? runOn(pauseT, 'pause', 'pause') : onbulk?.(bulkRows, b.action))} data-testid="dl-psel-{b.action}"><Icon name={b.icon} size={20} /><span>{b.label}</span></button>{/each}
				<button type="button" class="pa bad" onclick={() => onremovemany?.(pickedRows)} data-testid="dl-psel-remove"><Icon name="trash" size={20} /><span>{t('downloads.act.remove')}</span></button>
			</div>
		</div>

<style>
	.selbar { position: sticky; top: 0; left: 0; z-index: 3; display: flex; flex-wrap: wrap; align-items: center; gap: 6px; padding: 8px var(--gutter, 12px); background: var(--surface-1); border-bottom: 1px solid var(--line); }
	.selbar .sn { font: var(--t-footnote); color: var(--text-muted); margin-right: 4px; }
	.selbar .sn :global(b) { color: var(--text); }
	.psel { display: none; }
	@keyframes pselIn { from { opacity: 0; transform: translateY(12px); } to { opacity: 1; transform: none; } }
	@media (prefers-reduced-motion: reduce) {
		.psel { animation: none; }
	}
	.xbn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.xbn { min-height: 32px; padding: 0 14px; border: 1px solid var(--surface-3); border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xbn:hover:not(:disabled) { background: var(--surface-3); }
	.xbn:disabled { opacity: 0.5; cursor: not-allowed; }
	@media (max-width: 640px) {
		.selbar { display: none; }
		.psel { position: fixed; left: 8px; right: 8px; bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom) + 8px); z-index: 45; display: grid; gap: 4px; padding: 6px 8px 8px; border: 1px solid var(--line-strong); border-radius: 18px; background: color-mix(in srgb, var(--surface-1) 97%, transparent); backdrop-filter: blur(20px) saturate(1.4); -webkit-backdrop-filter: blur(20px) saturate(1.4); box-shadow: var(--shadow-modal); animation: pselIn 220ms var(--ease-out); }
		.pst { display: flex; align-items: center; gap: 4px; }
		.pst .sn { flex: 1; padding-left: 8px; color: var(--text-muted); font: 400 13px/1 var(--font-ui); }
		.pst .sn :global(b) { color: var(--text); font-weight: 600; }
		.plk { min-height: 44px; padding: 0 12px; border: 0; border-radius: 10px; background: none; color: var(--accent-text, var(--text)); font: 600 14px/1 var(--font-ui); cursor: pointer; }
		.psa { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 4px; }
		.pa { display: grid; justify-items: center; align-content: center; gap: 4px; min-height: 52px; padding: 4px 2px; border: 0; border-radius: 12px; background: var(--surface-2); color: var(--text); font: 500 11px/1.1 var(--font-ui); cursor: pointer; -webkit-tap-highlight-color: transparent; }
		.pa span { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
		.pa:active, .plk:active { transform: scale(0.97); }
		.pa.bad { color: var(--fail-text); }
		.pa:disabled { opacity: 0.45; }
		.pa:focus-visible, .plk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
		.xbn { min-height: 32px; }
	}
</style>
