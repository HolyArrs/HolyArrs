<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Poster from '#lib/ui/Poster.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import Menu from '#lib/ui/Menu.svelte';
	import type { MenuItem } from '#lib/ui/Menu.svelte';
	import DecisionPanel from './DecisionPanel.svelte';
	import DlTabsView from './DlTabsView.svelte';
	import { TAB_WORD, tabsOf } from './pane-facts.ts';
	import { healthLine } from './panel-words.ts';
	import { size as bytes, protocolOf } from './size.ts';
	import { statusWord } from './status-tabs.ts';
	import { pausable, retryable } from './usenet.ts';
	import { poster, who } from './row-art.ts';
	import { problem } from './problem.ts';
	import { barTone } from './sections.ts';
	import type { PaneDetail } from './pane-detail.svelte.ts';
	import type { PaneTab } from './pane-facts.ts';
	import type { ImportRecord } from './types.ts';
	import { tick } from 'svelte';
	import { to } from '../core/paths.ts';
	import { percent } from '../core/format.ts';
	import { sheetIn, sheetOut } from '../core/transitions.ts';
	import type { Decision, DownloadRow, TitleView } from '../core/types.ts';

	const isPaused = (r: DownloadRow | undefined) => r?.state === 'paused';
	type ClientAction = 'pause' | 'resume' | 'top' | 'recheck' | 'reannounce' | 'retry';
	type Props = {
		r: DownloadRow;
		paneOpen: boolean;
		openId: number;
		tab: PaneTab;
		ontab: (id: number, tab: PaneTab) => void;
		pane: PaneDetail;
		importOf: Map<number, ImportRecord>;
		titles: Map<number, TitleView>;
		titlesReady: boolean;
		watch: boolean;
		busy: number;
		decisionsOf: (r: DownloadRow) => Decision[];
		ondone: () => void;
		onact?: (r: DownloadRow, action: ClientAction) => void;
		onretry?: (rows: DownloadRow[]) => void;
		onremove: (r: DownloadRow, withFiles?: boolean) => void;
		mayEdit: (r: DownloadRow) => boolean;
		paneMenu: (r: DownloadRow) => MenuItem[];
		maxPane: () => number;
	};
	let { r, paneOpen = $bindable(), openId = $bindable(), tab, ontab, pane, importOf, titles, titlesReady, watch, busy, decisionsOf, ondone, onact, onretry, onremove, mayEdit, paneMenu, maxPane }: Props = $props();

	// The pane height is kept per browser.
	const PANE_KEY = 'holyarrs.downloads.pane';
	let paneH = $state(260);
	// The pane fits its content, capped so the list keeps rows. Dragging the divider sets a height; a double-click goes back to fitting.
	let paneFit = $state(true);
	let grabY: number | null = null;
	try {
		const v = typeof localStorage === 'undefined' ? null : Number(localStorage.getItem(PANE_KEY));
		if (v && Number.isFinite(v)) paneH = Math.max(120, Math.min(900, Math.round(v)));
		else if (typeof innerHeight !== 'undefined') paneH = Math.max(150, Math.round(innerHeight * 0.3));
	} catch {
		/* Private window: the default height applies. */
	}
	function savePane() {
		try {
			localStorage.setItem(PANE_KEY, String(Math.round(paneH)));
		} catch {
			/* Private window: the height lasts for this visit. */
		}
	}
	function dragDown(e: PointerEvent) {
		if (e.button !== 0 || !paneOpen) return;
		e.preventDefault();
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		const y0 = e.clientY;
		const pane = el.parentElement?.querySelector<HTMLElement>('.dpane');
		const h0 = paneFit && pane ? pane.offsetHeight : paneH;
		paneFit = false;
		paneH = h0;
		const mv = (m: PointerEvent) => (paneH = Math.max(120, Math.min(maxPane(), Math.round(h0 + (y0 - m.clientY)))));
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			savePane();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function dividerKey(e: KeyboardEvent) {
		if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return;
		e.preventDefault();
		e.stopPropagation();
		const pane = (e.currentTarget as HTMLElement).parentElement?.querySelector<HTMLElement>('.dpane');
		const h0 = paneFit && pane ? pane.offsetHeight : paneH;
		paneFit = false;
		paneH = Math.max(120, Math.min(maxPane(), h0 + (e.key === 'ArrowUp' ? 24 : -24)));
		savePane();
	}
	// Moves the tab underline with transform only.
	function ink(node: HTMLElement, _key: string) {
		const set = () => {
			const a = node.querySelector<HTMLElement>('[aria-selected="true"]');
			if (!a) return;
			node.style.setProperty('--ix', `${a.offsetLeft}px`);
			node.style.setProperty('--iw', String(a.offsetWidth));
		};
		set();
		const ro = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(set);
		ro?.observe(node);
		return { update: () => requestAnimationFrame(set), destroy: () => ro?.disconnect() };
	}
	// Same keys as the status tabs: one Tab stop, arrows, Home and End.
	function paneTabKey(e: KeyboardEvent, r: DownloadRow) {
		const tabs = [...(e.currentTarget as HTMLElement).parentElement!.querySelectorAll<HTMLElement>('[role="tab"]')];
		const i = tabs.indexOf(e.currentTarget as HTMLElement);
		const j = e.key === 'ArrowRight' ? i + 1 : e.key === 'ArrowLeft' ? i - 1 : e.key === 'Home' ? 0 : e.key === 'End' ? tabs.length - 1 : -2;
		if (j === -2 || !tabs[j]) return;
		e.preventDefault();
		tabs[j].click();
		tick().then(() => tabs[j].focus());
	}
</script>

{#snippet btns(r: DownloadRow)}
	{@const more = watch ? [] : paneMenu(r)}
	<div class="xbt" data-testid="dl-actions">
		{#if !watch}
			{#if retryable(r) && mayEdit(r)}<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} onclick={() => (onretry ? onretry([r]) : onact?.(r, 'retry'))} data-testid="dl-panel-retry-job">{t('downloads.act.again')}</button>
			{:else if pausable(r) && !r.client_locked}<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} onclick={() => onact?.(r, isPaused(r) ? 'resume' : 'pause')} data-testid="dl-panel-pause">{isPaused(r) ? t('downloads.act.start') : t('downloads.act.pause')}</button>{/if}
			<button type="button" class="xbn" disabled={!r.can_act || busy === r.id} title={r.can_act ? t('downloads.act.remove') : t('downloads.pane.cannotRemove')} onclick={() => onremove(r)} data-testid="dl-panel-remove">{t('downloads.act.remove')}</button>
		{/if}
		{#if r.title_id && (r.linked || !r.outside)}<a class="xbn" href={to(`/library/${r.title_id}`)} data-testid="dl-open-title">{t('downloads.act.library')}</a>{/if}
		{#if more.length}<span class="more" data-testid="dl-more"><Menu label={t('downloads.pane.moreActions')} items={more} width={220} /></span>{/if}
	</div>
{/snippet}

{#snippet tabStrip(r: DownloadRow)}
	<div class="xtabs" role="tablist" aria-label={t('downloads.pane.details')} use:ink={`${r.id}:${tab}:${paneOpen}`}>
		{#each tabsOf(r) as tb (tb)}
			<button type="button" role="tab" class="xtab" class:on={tab === tb} aria-selected={tab === tb} aria-controls="dlv-{r.id}" id="dlt-{r.id}-{tb}" tabindex={tab === tb ? 0 : -1} onclick={() => { ontab(r.id, tb); paneOpen = true; }} onkeydown={(e) => paneTabKey(e, r)} data-testid="dl-tab-{tb.toLowerCase().replaceAll(' ', '-')}">{TAB_WORD[tb]}</button>
		{/each}
		<i class="ink" class:gone={!paneOpen} aria-hidden="true"></i>
	</div>
{/snippet}

{#snippet panel(r: DownloadRow)}
	{@const ds = decisionsOf(r)}
	{@const sizeWord = r.size ? bytes(r.size) : t('downloads.col.notReported')}
	<section class="dpane" class:shut={!paneOpen} style={paneOpen ? (paneFit ? `max-height: ${maxPane()}px; --ph: ${maxPane()}px` : `height: ${paneH}px; --ph: ${paneH}px`) : ''} id="dlx-{r.id}" data-testid="dl-pane" aria-label={t('downloads.pane.detailsOf', { name: who(r) })}>
		<div class="phd">
			<button type="button" class="grab" aria-label={t('downloads.pane.close')} onpointerdown={(e) => (grabY = e.clientY)} onpointerup={(e) => { if (grabY != null && e.clientY - grabY > 40) openId = 0; grabY = null; }} onclick={() => (openId = 0)} data-testid="dl-pane-grab"><i></i></button>
			<p class="xt pt"><b>{who(r)}</b><span class="hs"><i class="hdot {barTone(r)}" class:bad={problem(r)?.tone === 'fail' || r.phase === 'failed'} aria-hidden="true"></i><span data-testid="dl-health">{protocolOf(r) === 'torrent' ? healthLine({ ...r, done_at: pane.det.id === r.id ? pane.det.detail?.torrent?.done_at ?? r.finished_at : r.finished_at, seeding_time: pane.det.id === r.id ? pane.det.detail?.torrent?.seeding_time : undefined, ratio: pane.det.id === r.id ? pane.det.detail?.torrent?.ratio ?? r.ratio : r.ratio }) : t('downloads.pane.line', { status: statusWord(r), pct: Math.floor(percent(r.progress)), size: sizeWord })}</span></span></p>
			{@render tabStrip(r)}
			{#if paneOpen}
			<div class="pact" data-testid="dl-pane-actions">
				{#if r.replaced}<a class="tdl" href={to('/library/turned-down')} data-testid="dl-turned-down-link">{t('downloads.pane.turnedDown')}</a>{/if}
				{@render btns(r)}
			</div>
			{/if}
			<button type="button" class="pcl" aria-expanded={paneOpen} aria-label={paneOpen ? t('downloads.pane.collapse') : t('downloads.pane.expand')} onclick={() => (paneOpen = !paneOpen)} data-testid="dl-pane-toggle"><Icon name="chevronDown" size={14} stroke={2.2} /></button>
		</div>
		{#if paneOpen}
		<div class="pb" data-testid="dl-panel" data-card="d">
			<span class="big" class:square={r.kind === 'artist'} data-testid="dl-big-poster"><Poster src={poster(r, titles)} name={titlesReady ? r.title || r.name.replace(/[._]+/g, ' ') : ''} /></span>
			<div class="xb">
				{#key tab}<div class="tview" role="tabpanel" id="dlv-{r.id}" aria-labelledby="dlt-{r.id}-{tab}"><DlTabsView {r} tab={tab} {pane} {importOf} {watch} {mayEdit} /></div>{/key}
				{#if ds.length && !watch}<div class="dp"><DecisionPanel items={ds} compact {ondone} /></div>{/if}
			</div>
		</div>
		{/if}
	</section>
{/snippet}

	<div class="sheet" in:sheetIn|global out:sheetOut|global data-testid="dl-sheet">
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div class="dv" role="separator" aria-orientation="horizontal" aria-label={t('downloads.pane.resize')} aria-valuemin={0} aria-valuemax={900} aria-valuenow={paneOpen ? (paneFit ? 0 : paneH) : 0} tabindex="0" class:fixed={!paneOpen} title={t('downloads.pane.dividerTitle')} onpointerdown={dragDown} ondblclick={() => (paneFit = true)} onkeydown={dividerKey} data-testid="dl-divider"><i aria-hidden="true"></i></div>
	{@render panel(r)}
	</div>

<style>
	.types { display: inline-flex; gap: 2px; padding: 2px; margin-bottom: 12px; border-radius: var(--r-btn); background: var(--surface-2); }
	:global(.post).square { height: var(--pi, 26px); --poster-ratio: 1; }
	.xb { min-width: 0; }
	.big { width: 72px; height: 108px; overflow: hidden; border-radius: 6px; box-shadow: 0 4px 14px rgb(0 0 0 / 0.18); --poster-ratio: 2 / 3; }
	.big.square { height: 72px; --poster-ratio: 1; }
	.big :global(.poster) { border-radius: 6px; }
	.dp { margin-top: 10px; }
	.xt { display: grid; gap: 2px; margin: 0; min-width: 0; font: var(--t-footnote); }
	.xt b { font-weight: 600; overflow-wrap: anywhere; }
	.xt span { color: var(--text-muted); font-variant-numeric: tabular-nums; }
	.xtabs { display: flex; flex-wrap: wrap; gap: 2px; padding: 2px; width: max-content; max-width: 100%; border-radius: var(--r-btn); background: var(--surface-2); }
	.xtab { min-height: 28px; padding: 0 12px; border: 0; border-radius: calc(var(--r-btn) - 2px); background: none; color: var(--text-muted); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xtab.on { background: var(--surface-1); color: var(--text); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12); }
	.xtab:focus-visible, .xbn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dv { flex: none; position: relative; height: 9px; border-top: 1px solid var(--line); cursor: row-resize; touch-action: none; }
	.dv.fixed { cursor: default; }
	.dv i { position: absolute; left: 50%; top: 3px; width: 38px; height: 3px; margin-left: -19px; border-radius: 2px; background: var(--line-strong, var(--text-muted)); opacity: 0.6; }
	.dv:hover i, .dv:focus-visible i { opacity: 1; }
	.dv:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.sheet { flex: none; display: flex; flex-direction: column; min-height: 0; }
	.dpane { flex: none; display: flex; flex-direction: column; min-height: 0; background: none; }
	.dpane.shut { height: auto; }
	.phd { flex: none; }
	.grab { display: none; }
	.phd { display: flex; align-items: center; gap: 2px 14px; flex-wrap: wrap; padding: 3px var(--dl-gutter); border-bottom: 1px solid var(--line); min-width: 0; }
	.dpane.shut .phd { border-bottom: 0; }
	.pt { flex: 0 1 380px; min-width: 0; margin: 0; }
	.pt b { font: 600 16px / 1.3 var(--font-ui); }
	.pt span { font: var(--t-callout); }
	.pt b, .pt span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.phd .xtabs { margin: 0; background: none; padding: 0; }
	.pcl { margin-left: auto; display: grid; place-items: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.pcl :global(svg) { transition: transform var(--dur-fast) var(--ease-out); }
	.dpane.shut .pcl :global(svg) { transform: rotate(180deg); }
	.pcl:hover { background: var(--surface-3); color: var(--text); }
	.pcl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pb { display: grid; grid-template-columns: 112px minmax(0, 1fr); align-items: start; gap: 12px; flex: 1 1 auto; min-height: 0; padding: 8px var(--dl-gutter); overflow: auto; }
	.pb .big { width: 112px; height: 168px; border-radius: var(--r-card, 10px); }
	.pb .big :global(.poster) { border-radius: var(--r-card, 10px); }
	.pb .big.square { height: 112px; }
	.pb .xtab { min-height: 28px; padding: 0 10px; font: var(--t-callout); font-weight: 500; }
	.pb .xbt { margin-top: 6px; }
	.pb .xbn { min-height: 26px; padding: 0 10px; font: var(--t-caption); font-weight: 500; }
	.dv:active i { opacity: 1; }
	.tab { display: inline-flex; align-items: center; flex: none; gap: 6px; height: 28px; padding: 0 12px; border: 0; border-radius: 999px; background: none; color: color-mix(in srgb, var(--text) 76%, var(--bg)); font: var(--t-footnote); white-space: nowrap; cursor: pointer; transition: color 200ms var(--ease-out); }
	.tab:hover { color: var(--text); }
	.tab:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.more { min-width: 36px; padding: 0 10px; }
	.xbt { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
	.xbn { min-height: 32px; padding: 0 14px; border: 1px solid var(--surface-3); border-radius: var(--r-btn); background: var(--surface-2); color: var(--text); font: var(--t-footnote); font-weight: 500; cursor: pointer; }
	.xbn:hover:not(:disabled) { background: var(--surface-3); }
	.xbn:disabled { opacity: 0.5; cursor: not-allowed; }
	.big, .big :global(.poster) { border-radius: var(--r-poster); }
	.big { box-shadow: var(--shadow-card); }
	.pcl { border-radius: var(--r-sm); }
	.dv { height: 12px; border-top: 1px solid var(--line); }
	.dv i { top: 4px; width: 36px; margin-left: -18px; border-radius: var(--r-pill); transition: transform var(--dur-fast) var(--ease-out), opacity var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.dv:not(.fixed):hover i, .dv:focus-visible i { transform: scaleX(1.5); opacity: 1; }
	.dv:not(.fixed):active i { transform: scaleX(1.8) scaleY(1.3); background: var(--text-muted); }
	.phd { position: relative; min-height: 48px; padding: 0 var(--dl-gutter); gap: 0 16px; }
	.phd .xtabs { position: relative; align-self: stretch; gap: 4px; flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; }
	.phd .xtabs::-webkit-scrollbar { display: none; }
	.phd .xtab { min-height: 48px; height: 100%; padding: 0 8px; border-radius: 0; background: none; box-shadow: none; font: var(--t-callout); font-weight: 500; white-space: nowrap; transition: color var(--dur-fast) var(--ease-out); }
	.phd .xtab:hover { color: var(--text); }
	.phd .xtab.on { background: none; box-shadow: none; color: var(--text); }
	.phd .xtab:focus-visible { box-shadow: inset var(--focus-ring); border-radius: var(--r-sm); }
	.ink { position: absolute; left: 0; bottom: 0; width: 1px; height: 2px; background: var(--accent); transform-origin: left; transform: translateX(var(--ix, 0)) scaleX(var(--iw, 0)); transition: transform var(--dur) var(--ease-out), opacity var(--dur-fast) var(--ease-out); pointer-events: none; }
	.ink.gone { opacity: 0; }
	.pb { padding: 12px var(--dl-gutter); gap: 16px; animation: fadein var(--dur) var(--ease-out); }
	.tview { animation: tabin var(--dur) var(--ease-out); }
	.pact { position: relative; flex: none; margin-left: auto; display: flex; align-items: center; gap: 6px; min-width: 0; }
	.phd .pcl { margin-left: 0; }
	.pact .xbt { margin: 0; gap: 6px; flex-wrap: nowrap; }
	.pact .xbn { min-height: 32px; padding: 0 10px; border-radius: var(--r-btn); font: var(--t-subhead); font-weight: 500; transition: background-color var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), border-color var(--dur-fast) var(--ease-out), transform var(--dur-instant) var(--ease-out); }
	.pact .xbn:active:not(:disabled) { transform: scale(0.97); }
	.pact .more { display: inline-flex; }
	.tdl { color: var(--text-muted); font: var(--t-caption); text-decoration: underline; text-decoration-color: var(--line-strong); text-underline-offset: 3px; border-radius: var(--r-sm); }
	.tdl:hover { color: var(--text); }
	.tdl:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	a.xbn { display: inline-flex; align-items: center; text-decoration: none; }
	.pt span { font-variant-numeric: tabular-nums; }
	.pb { grid-template-columns: max-content minmax(0, 1fr); gap: 20px; }
	.pb .big { width: 160px; height: 240px; border-radius: var(--r-card); box-shadow: 0 2px 6px rgb(0 0 0 / 0.08), 0 12px 32px rgb(0 0 0 / 0.16); }
	.pb .big :global(.poster) { border-radius: var(--r-card); }
	.pb .big.square { height: 160px; }
	.pb::-webkit-scrollbar { width: 12px; height: 12px; background: transparent; }
	.pb::-webkit-scrollbar-track { margin: 4px; border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 6%, transparent) padding-box; }
	.pb::-webkit-scrollbar-corner { background: transparent; }
	.pb::-webkit-scrollbar-button { display: none; width: 0; height: 0; }
	.pb::-webkit-scrollbar-thumb { border: 3px solid transparent; border-radius: 12px; background: color-mix(in srgb, var(--text) 26%, transparent) padding-box; }
	.pb::-webkit-scrollbar-thumb:hover { background: color-mix(in srgb, var(--text) 40%, transparent) padding-box; }
	.hs { display: flex; align-items: center; gap: 6px; min-width: 0; color: var(--text-muted); }
	.hdot { flex: none; width: 8px; height: 8px; border-radius: 50%; background: var(--ok); }
	.hdot:global(.paused) { background: var(--text-muted); }
	.hdot:global(.held) { background: var(--wait); }
	.hdot:global(.queued) { background: var(--info); }
	.hdot.bad { background: var(--fail); }
	.pt { gap: 3px; padding: 8px 0; }
	.pt b { font: 600 17px / 1.25 var(--font-ui); letter-spacing: -0.01em; }
	.phd .xtabs { align-self: center; gap: 2px; padding: 2px; border: 1px solid var(--line); border-radius: 999px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); }
	.phd .xtab { min-height: 28px; height: auto; padding: 0 12px; border-radius: 999px; color: var(--text-muted); transition: color var(--dur-fast) var(--ease-out), background-color var(--dur-fast) var(--ease-out); }
	.phd .xtab:hover { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.phd .xtab.on, .phd .xtab.on:hover { background: var(--seg-thumb); color: var(--text); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); }
	.phd .xtab:focus-visible { border-radius: 999px; box-shadow: var(--focus-ring); }
	.phd .ink { display: none; }
	.pb .big { width: 120px; height: 180px; border: 1px solid var(--line); box-shadow: none; }
	.pb .big.square { height: 120px; }
	@media (prefers-reduced-motion: reduce) {
		.tab { transition: none; }
		.pb, .tview { animation: none; }
		.ink, .dv i, .pact .xbn { transition: none; }
	}
	@media (max-width: 640px) {
		.big { width: 56px; height: 84px; }
		.big.square { height: 56px; }
		.dv { display: none; }
		.dpane { background: var(--surface-1); }
		.dpane { position: fixed; left: 0; right: 0; bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom)); z-index: 40; height: min(64dvh, 520px) !important; border-radius: 16px 16px 0 0; box-shadow: 0 -12px 32px rgb(0 0 0 / 0.32); }
		.dpane.shut { height: auto !important; }
		.pb { grid-template-columns: 56px minmax(0, 1fr); gap: 12px; }
		.pb .big { width: 56px; height: 84px; border-radius: var(--r-poster); }
		.pb .big.square { height: 56px; }
		.pt { flex-basis: 100%; }
		.phd .pt { order: 0; }
		.phd .xtabs { order: 1; }
		.phd .pcl { order: 2; margin-left: auto; }
		.pact { order: 3; flex: 1 0 100%; margin-left: 0; flex-wrap: wrap; padding-bottom: 4px; }
		.pact .xbt { flex-wrap: wrap; }
		.dpane { height: min(60dvh, calc(100dvh - var(--tabbar, 56px) - env(safe-area-inset-bottom) - 340px), 520px) !important; }
		.grab { display: grid; place-items: center; order: -1; flex: 1 0 100%; height: 44px; margin: -13px 0; padding: 0; border: 0; background: none; cursor: grab; touch-action: none; }
		.grab i { width: 40px; height: 4px; border-radius: 2px; background: var(--line-strong, var(--text-muted)); }
		.dpane.shut .grab { display: none; }
		.dpane { border-radius: var(--r-panel) var(--r-panel) 0 0; box-shadow: var(--shadow-modal); }
		.pb { grid-template-columns: minmax(0, 1fr); }
		.pb .big { display: none; }
		.pact { flex-wrap: wrap; }
		.pb { grid-template-columns: minmax(0, 1fr); }
		.pb .big { display: none; }
		.pb .big { display: none; }
		.phd .xtabs { max-width: 100%; }
		.phd .xtab { padding: 0 12px; }
		.tab { height: 32px; }
		.pcl { width: 32px; height: 32px; }
		.xbn { min-height: 32px; }
		.phd .xtab { min-height: 32px; }
	}
	@media (min-width: 1800px) {
		.pb { grid-template-columns: max-content minmax(0, 1fr); gap: 24px; }
		.pb .big { width: 200px; height: 300px; }
		.pb .big.square { height: 200px; }
		.pb .big { width: 140px; height: 210px; }
		.pb .big.square { height: 140px; }
	}
	@media (max-width: 767px) {
		.pb { grid-template-columns: 56px minmax(0, 1fr); gap: 12px; }
		.pb .big { width: 56px; height: 84px; border-radius: var(--r-poster); }
		.pb .big.square { height: 56px; }
		.pb .big { width: 56px; height: 84px; }
		.pb .big.square { height: 56px; }
	}
	@supports not selector(::-webkit-scrollbar) {
		.pb { scrollbar-width: thin; scrollbar-color: color-mix(in srgb, var(--text) 24%, transparent) transparent; }
		.pb:hover { scrollbar-color: color-mix(in srgb, var(--text) 38%, transparent) transparent; }
	}
	@media (min-width: 641px) {
		.dv { background: var(--surface-2); }
		/* The pane shares the table's surface; the divider is the only seam. */
	.dpane { margin: 0; border-radius: 0; background: var(--surface-2); overflow: hidden; }
		.dpane .phd { padding: 8px var(--dl-gutter); border: 0; border-bottom: 1px solid var(--line); border-radius: 0; background: transparent; box-shadow: none; }
		.pb { padding: 12px var(--dl-gutter) 16px; background: transparent; }
		/* The title takes the room it needs, up to 42% of the header. */
	.dpane .pt { flex: 0 1 auto; min-width: 0; max-width: 42%; }
		.dpane .phd .xtabs { margin-inline: auto; }
	}
	@media (min-width: 641px) and (max-width: 1240px) {
		.dpane .pt { max-width: none; flex: 1 1 200px; }
		.dpane .phd .xtabs { order: 2; flex: 0 1 auto; margin: 0 auto 6px 0; }
		.dpane .pact, .dpane .pcl { order: 1; flex: none; }
	}
	@media (max-width: 767px) and (pointer: coarse) {
		.tab { position: relative; }
		.tab::after { content: ''; position: absolute; inset: -10px 0; }
	}
</style>
