<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon, { type IconName } from '#lib/ui/Icon.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import TopMenu from './TopMenu.svelte';
	import DlFind from './DlFind.svelte';
	import { tip } from './tip.ts';
	import { actionName } from './action-name.ts';
	import { KIND_TABS } from './size.ts';

	// Download toolbar: actions on the left, Library and Sources filters on the right. On phones one compact bar replaces it.
	type Act = { key: string; icon: IconName; label: string; hint: string; disabled: boolean; run?: () => void; href?: string; more?: { label: string; onselect: () => void }[] };
	let {
		kindOn = $bindable(''),
		srcOn = $bindable(''),
		trackOn = $bindable(''),
		trackAll = 0,
		trackHosts = [],
		trackProblems = 0,
		kindCount,
		srcCount,
		actions,
		watch = false,
		term = $bindable(''),
		shown,
		total,
		side = false,
		sideControls = 'sidebar',
		onside,
		searchKey = 'Ctrl K',
		allItems = []
	}: {
		kindOn?: string;
		srcOn?: string;
		trackOn?: string;
		trackAll?: number;
		trackHosts?: { key: string; label: string; count: number }[];
		trackProblems?: number;
		kindCount: (k: string) => number;
		srcCount: (k: string) => number;
		actions: Act[];
		watch?: boolean;
		term?: string;
		shown: number;
		total: number;
		side?: boolean;
		sideControls?: string;
		onside?: () => void;
		searchKey?: string;
		/** Phone menu for the bulk actions. */
		allItems?: { label: string; onselect: () => void }[];
	} = $props();
	let allMenu = $state<TopMenu>();
	let allBtn = $state<HTMLButtonElement>();

	const KIND_ICON: Record<string, IconName> = { movie: 'film', show: 'tv', artist: 'music' };
	const kinds = $derived(KIND_TABS.filter((k) => k.key).map((k) => ({ key: k.key as string, label: k.label, icon: KIND_ICON[k.key] })));
	const sources: { key: string; label: string; icon: IconName }[] = [
		{ key: 'usenet', label: 'Usenet', icon: 'usenet' },
		{ key: 'torrent', label: 'Torrent', icon: 'torrent' }
	];
	type F = { key: string; label: string; icon: IconName };

	const pickKind = (k: string) => (kindOn = kindOn === k ? '' : k);
	const pickSrc = (k: string) => (srcOn = srcOn === k ? '' : k);
	const showSide = $derived(side && !!onside);
	let trkBtn = $state<HTMLButtonElement>();
	let trkMenu = $state<TopMenu>();
	const trkItems = $derived([
		{ label: t('downloads.bar.allTrackers'), hint: String(trackAll), checked: !trackOn, tid: 'dl-tracker-opt', key: '', onselect: () => (trackOn = '') },
		...trackHosts.map((o) => ({ label: o.label, hint: String(o.count), checked: trackOn === o.key, tid: 'dl-tracker-opt', key: o.key, onselect: () => (trackOn = o.key) })),
		...(trackProblems ? [{ label: t('downloads.bar.trackerProblems'), hint: String(trackProblems), checked: trackOn === 'p:any', bad: true, tid: 'dl-tracker-opt', key: 'p:any', onselect: () => (trackOn = 'p:any') }] : [])
	]);

	// Phones use one compact bar. Start, Pause and Remove act on a picked row, so they live in its details.
	let fOpen = $state(false);
	// The Filters sheet closes once the screen grows past 640px.
	$effect(() => {
		const mq = matchMedia('(max-width: 640px)');
		const off = () => {
			if (!mq.matches) fOpen = false;
		};
		mq.addEventListener('change', off);
		return () => mq.removeEventListener('change', off);
	});
	const fCount = $derived((kindOn ? 1 : 0) + (srcOn ? 1 : 0) + (trackOn ? 1 : 0));
	const addAct = $derived(actions.find((a) => a.key === 'add'));
	const clearAll = () => {
		kindOn = '';
		srcOn = '';
		trackOn = '';
	};

	// Moves one shared plate to the picked segment.
	function slider(node: HTMLElement, _dep: string) {
		const place = () => {
			const on = node.querySelector<HTMLElement>('[aria-pressed="true"]');
			if (!on) {
				node.style.setProperty('--io', '0');
				return;
			}
			node.style.setProperty('--ix', `${on.offsetLeft}px`);
			node.style.setProperty('--iw', `${on.offsetWidth}px`);
			node.style.setProperty('--ih', `${on.offsetHeight}px`);
			node.style.setProperty('--iy', `${on.offsetTop}px`);
			node.style.setProperty('--io', '1');
		};
		place();
		const ro = new ResizeObserver(place);
		ro.observe(node);
		let frame = 0;
		return {
			update: () => {
				cancelAnimationFrame(frame);
				frame = requestAnimationFrame(place);
			},
			destroy: () => {
				cancelAnimationFrame(frame);
				ro.disconnect();
			}
		};
	}
</script>

{#snippet act(a: Act)}
	<span class="aw" data-testid="tb-{a.key}">
		<span class="tw2" use:tip={a.hint !== a.label ? a.hint : ''}>
			{#if a.href}
				<a class="it act stk" href={a.href} aria-label={actionName(a)}><span class="ic"><Icon name={a.icon} size={20} /></span><span class="lb">{a.label}</span></a>
			{:else}
				<button type="button" class="it act stk" disabled={a.disabled} aria-label={actionName(a)} onclick={() => a.run?.()} data-testid="tb-{a.key}-btn"><span class="ic"><Icon name={a.icon} size={20} /></span><span class="lb">{a.label}</span></button>
			{/if}
		</span>
		{#if a.more}<TopMenu label={t('downloads.bar.moreActions', { key: a.key })} items={a.more} />{/if}
	</span>
{/snippet}

{#snippet seg(label: string, list: F[], current: string, count: (k: string) => number, pick: (k: string) => void, tid: string, gtid: string)}
	<div class="seg" role="group" aria-label={label} data-testid={gtid} use:slider={current}>
		<i class="plate" aria-hidden="true"></i>
		{#each list as f (f.key)}
			{@const n = count(f.key)}
			{@const on = current === f.key}
			<button type="button" class="it fl stk" class:on class:zero={n === 0} aria-pressed={on} onclick={() => pick(f.key)} data-testid={tid} data-key={f.key} data-count={n}>
				<span class="ic"><Icon name={f.icon} size={20} /></span><span class="lb">{f.label}</span>
			</button>
			{#if f.key === 'torrent' && (trackHosts.length || trackProblems)}
				<span class="trw">
					<button type="button" class="trk" class:on={!!trackOn} bind:this={trkBtn} aria-haspopup="menu" aria-label={t('downloads.bar.filterTracker')} onclick={() => trkBtn && trkMenu?.show(trkBtn)} data-testid="dl-tracker-menu-btn"><Icon name="chevronDown" size={14} stroke={2.2} /></button>
					<TopMenu bare bind:this={trkMenu} label={t('downloads.bar.trackers')} items={trkItems} />
				</span>
			{/if}
		{/each}
	</div>
{/snippet}

{#snippet acts()}
	{#if !watch}<div class="grp acts" role="toolbar" aria-label={t('downloads.bar.actionsLabel')} data-testid="dl-toolbar">{#each actions as a (a.key)}{@render act(a)}{/each}</div>{/if}
{/snippet}
{#snippet library()}{@render seg(t('downloads.bar.library'), kinds, kindOn, kindCount, pickKind, 'bar-kind', 'dl-type-filter')}{/snippet}
{#snippet srcs()}{@render seg(t('downloads.bar.network'), sources, srcOn, srcCount, pickSrc, 'bar-source', 'dl-source-filter')}{/snippet}
{#snippet sideBtn()}
	<span class="tw2" use:tip={t('downloads.bar.showSidebar')}><button type="button" class="it side" aria-label={t('downloads.bar.showSidebar')} aria-expanded={false} aria-controls={sideControls} onclick={() => onside?.()} data-testid="dl-side-btn"><span class="ic"><Icon name="sidebar" size={20} /></span></button></span>
{/snippet}

<div class="dlbar" data-testid="dl-bar">
	<div class="row">
		{#if showSide || !watch}<div class="cap" class:sidecap={watch}>{#if showSide}{@render sideBtn()}{#if !watch}<span class="vr ph" aria-hidden="true"></span>{/if}{/if}{@render acts()}</div>{/if}
		<DlFind bind:term {shown} {total} {searchKey} />
		<div class="cap">{@render library()}</div><div class="cap">{@render srcs()}</div>
	</div>
	<div class="pbar" data-testid="dl-phonebar">
		<button type="button" class="pbn" class:has={fCount > 0} aria-haspopup="dialog" aria-label={fCount ? t('downloads.bar.filtersOn', { count: fCount }) : t('downloads.bar.filters')} onclick={() => (fOpen = true)} data-testid="dl-filters-btn">
			<Icon name="sliders" size={18} stroke={2} />
			<span>{t('downloads.bar.filters')}</span>{#if fCount}<b class="fc" data-testid="dl-filters-count">{fCount}</b>{/if}
		</button>
		{#if addAct && !watch}<button type="button" class="pbn add" onclick={() => addAct.run?.()} aria-label={actionName(addAct)} data-testid="dl-phone-add"><Icon name="plus" size={18} /><span>{t('downloads.bar.add')}</span></button>{/if}
		{#if allItems.length}<button type="button" class="pbn ico" bind:this={allBtn} aria-haspopup="menu" aria-label={t('downloads.bar.allDownloads')} onclick={() => allBtn && allMenu?.show(allBtn)} data-testid="dl-all-btn"><Icon name="more" size={18} /></button><TopMenu bare bind:this={allMenu} label={t('downloads.bar.allDownloads')} items={allItems} />{/if}
	</div>
</div>

{#snippet opt(on: boolean, label: string, n: number | undefined, pick: () => void, tid: string, key: string, bad = false)}
	<button type="button" class="fo" class:on class:bad aria-pressed={on} onclick={pick} data-testid={tid} data-key={key}><span class="fl2">{label}</span>{#if n !== undefined}<b>{n}</b>{/if}<span class="ck" aria-hidden="true">{#if on}<Icon name="check" size={16} />{/if}</span></button>
{/snippet}

<Sheet bind:open={fOpen} title={t('downloads.bar.filters')} size="sm" placement="bottom">
	<div class="fs" data-testid="dl-filters-sheet">
		<section aria-label={t('downloads.bar.library')}><h3>{t('downloads.bar.library')}</h3><div class="fg">
			{@render opt(!kindOn, t('downloads.bar.everything'), undefined, () => (kindOn = ''), 'dl-f-kind', '')}
			{#each kinds as k (k.key)}{@render opt(kindOn === k.key, k.label, kindCount(k.key), () => (kindOn = k.key), 'dl-f-kind', k.key)}{/each}
		</div></section>
		<section aria-label={t('downloads.bar.network')}><h3>{t('downloads.bar.network')}</h3><div class="fg">
			{@render opt(!srcOn, t('downloads.bar.anyNetwork'), undefined, () => (srcOn = ''), 'dl-f-src', '')}
			{#each sources as s (s.key)}{@render opt(srcOn === s.key, s.label, srcCount(s.key), () => (srcOn = s.key), 'dl-f-src', s.key)}{/each}
		</div></section>
		{#if trackHosts.length || trackProblems}
			<section aria-label={t('downloads.bar.trackers')}><h3>{t('downloads.bar.trackers')}</h3><div class="fg">
				{@render opt(!trackOn, t('downloads.bar.allTrackers'), trackAll, () => (trackOn = ''), 'dl-f-trk', '')}
				{#each trackHosts as o (o.key)}{@render opt(trackOn === o.key, o.label, o.count, () => (trackOn = o.key), 'dl-f-trk', o.key)}{/each}
				{#if trackProblems}{@render opt(trackOn === 'p:any', t('downloads.bar.trackerProblems'), trackProblems, () => (trackOn = 'p:any'), 'dl-f-trk', 'p:any', true)}{/if}
			</div></section>
		{/if}
	</div>
	{#snippet footer()}
		<button type="button" class="fbtn" disabled={!fCount} onclick={clearAll} data-testid="dl-filters-clear">{t('downloads.bar.clear')}</button>
		<button type="button" class="fbtn pri" onclick={() => (fOpen = false)} data-testid="dl-filters-done">{t('downloads.bar.show', { count: shown })}</button>
	{/snippet}
</Sheet>

<style>
	.trw { position: relative; display: flex; align-items: stretch; }
	.trk { display: grid; place-items: center; width: 32px; min-height: 32px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); font-size: 11px; cursor: pointer; }
	.trk:hover, .trk.on { color: var(--text); background: var(--surface-3); }
	.trk:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.dlbar { --ez: cubic-bezier(0.32, 0.72, 0, 1); flex: 1 1 0; min-width: 0; display: grid; gap: 4px; }
	.row { display: flex; align-items: center; gap: 8px; min-width: 0; padding: 4px 0; overflow-x: auto; overflow-y: hidden; overscroll-behavior-x: contain; scrollbar-width: none; }
	.row::-webkit-scrollbar { display: none; }
	.grp, .seg { display: flex; flex: none; align-items: center; gap: 2px; padding: 0; margin: 0; border: 0; background: none; }
	.aw, .tw2 { display: inline-flex; align-items: center; flex: none; }

	.cap { display: flex; flex: none; align-items: center; height: 52px; box-sizing: border-box; padding: 3px; border: 1px solid var(--line); border-radius: 14px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); backdrop-filter: blur(18px) saturate(1.4); -webkit-backdrop-filter: blur(18px) saturate(1.4); box-shadow: inset 0 1px 0 rgb(255 255 255 / 0.06), 0 6px 20px -10px rgb(0 0 0 / 0.35); }
	.cap > .grp, .cap > .seg { height: 44px; min-height: 0; align-items: stretch; }
	.dlbar .seg > .it, .dlbar .grp > .aw { margin: 0; }

	.it { position: relative; z-index: 1; display: inline-flex; align-items: center; flex: none; gap: 6px; padding: 0 10px; border: 0; border-radius: var(--r-btn); background: none; color: var(--text); font: var(--t-footnote); white-space: nowrap; text-decoration: none; cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform 180ms var(--ez); }
	.it .ic { position: relative; display: grid; place-items: center; color: inherit; transition: transform 180ms var(--ez); }
	.it .lb { display: inline-flex; align-items: center; gap: 4px; }
	.dlbar .it.stk { display: grid; grid-template-rows: 18px 12px; align-content: center; justify-items: center; gap: 3px; width: 68px; min-width: 0; height: 44px; padding: 0; border-radius: 10px; }
	.dlbar .it.stk .ic { height: 18px; }
	.dlbar .it.stk .lb { font: 500 11px/1 var(--font-ui); letter-spacing: 0; color: var(--text-muted); transition: color 160ms var(--ease-out); }
	.it:hover:not(:disabled) .lb { color: var(--text); }
	.dlbar .it:hover:not(:disabled) { background: color-mix(in srgb, var(--text) 7%, transparent); }
	.dlbar .it:active:not(:disabled) { transform: scale(0.97); }
	.it:disabled { cursor: not-allowed; }
	.dlbar .it:disabled .ic, .dlbar .it:disabled .lb { color: color-mix(in srgb, var(--text) 38%, transparent); }
	.it:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.it.zero:not(.on) .ic { opacity: 0.55; }
	.dlbar .it.on, .dlbar .it.on .lb { color: var(--text); font-weight: 500; }
	.dlbar .it.on .ic { color: var(--text); }

	.seg { position: relative; }
	.plate { position: absolute; z-index: 0; left: 0; top: 0; width: var(--iw, 0); height: var(--ih, 0); border-radius: 10px; background: var(--seg-thumb); box-shadow: 0 0 0 1px var(--line-strong), 0 1px 2px rgb(0 0 0 / 0.12), 0 2px 8px rgb(0 0 0 / 0.08); opacity: var(--io, 0); transform: translate(var(--ix, 0), var(--iy, 0)); transition: transform 320ms var(--ez), width 320ms var(--ez), opacity 160ms var(--ease-out); pointer-events: none; }
	.dlbar .seg .it.on:hover { background: none; }
	.dlbar .aw { position: relative; }
	.dlbar .aw :global(.caret) { position: absolute; top: 3px; right: 3px; z-index: 2; width: 16px; height: 16px; min-width: 0; margin: 0; padding: 0; border-radius: 5px; opacity: 0.55; }
	.dlbar .aw :global(.caret:hover) { opacity: 1; background: color-mix(in srgb, var(--text) 10%, transparent); }
	.dlbar .aw :global(.caret svg) { width: 11px; height: 11px; }

	.dlbar .it.side { justify-content: center; width: 44px; height: 44px; padding: 0; border-radius: 10px; }
	.vr { flex: none; align-self: center; width: 1px; height: 26px; margin: 0 4px; background: var(--line); }

	@media (min-width: 1100px) {
		.row { overflow: visible; }
	}
	/* Tablets and phones scroll the row sideways, with faded edges. */
	@media (max-width: 1099px) {
		.row > :global(.dlfind) { min-width: 200px; }
	}
	@media (max-width: 767px) {
		.row { -webkit-mask-image: linear-gradient(90deg, #000 calc(100% - 20px), transparent); mask-image: linear-gradient(90deg, #000 calc(100% - 20px), transparent); }
		.dlbar .it.stk { width: 58px; }
		.row > :global(.dlfind), .sidecap, .dlbar .it.side, .vr.ph { display: none; }
	}
	.pbar { display: none; }
	.pbn { display: inline-flex; align-items: center; gap: 6px; height: 44px; padding: 0 14px; border: 1px solid var(--line); border-radius: 999px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); color: var(--text); font: 500 15px/1 var(--font-ui); cursor: pointer; -webkit-tap-highlight-color: transparent; transition: background 160ms var(--ease-out), transform 160ms var(--ez); }
	.pbn:active { transform: scale(0.97); }
	.pbn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.pbn.ico { justify-content: center; width: 44px; padding: 0; }
	.pbn.has { border-color: var(--line-strong); background: var(--surface-2); }
	.fc { display: inline-grid; place-items: center; min-width: 20px; height: 20px; padding: 0 5px; box-sizing: border-box; border-radius: 999px; background: var(--text); color: var(--bg); font: 600 12px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.fs { display: grid; gap: 18px; }
	.fs section { display: grid; gap: 6px; }
	.fs h3 { margin: 0; color: var(--text-muted); font: 600 13px/1.2 var(--font-ui); }
	.fg { display: grid; border: 1px solid var(--line); border-radius: 12px; background: var(--surface-2); overflow: hidden; }
	.fo { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 0 14px; border: 0; border-top: 1px solid var(--line); background: none; color: var(--text); font: var(--t-callout); text-align: left; cursor: pointer; }
	.fo:first-child { border-top: 0; }
	.fo:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.fo .fl2 { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.fo b { color: var(--text-muted); font-weight: 500; font-variant-numeric: tabular-nums; }
	.fo .ck { display: grid; place-items: center; width: 18px; color: var(--accent-text, var(--text)); }
	.fo.on { font-weight: 600; }
	.fo.bad .fl2 { color: var(--fail-text); }
	.fbtn { min-height: 44px; padding: 0 18px; border: 1px solid var(--line); border-radius: 12px; background: none; color: var(--text); font: 500 15px/1 var(--font-ui); cursor: pointer; }
	.fbtn:disabled { opacity: 0.45; cursor: default; }
	.fbtn.pri { flex: 1; border-color: transparent; background: var(--text); color: var(--bg); }
	.fbtn:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	@media (min-width: 641px) and (max-width: 1099px) {
		.dlbar .it.stk { width: 56px; }
		.row > :global(.dlfind) { min-width: 160px; }
	}
	@media (max-width: 640px) {
		.dlbar { flex: 1 1 auto; }
		.row { display: none; }
		.trk { width: 44px; min-height: 44px; }
		.pbar { display: flex; justify-content: flex-end; align-items: center; gap: 8px; padding: 4px 0; }
	}
	@media (prefers-reduced-motion: reduce) {
		.pbn { transition: none; }
		.it, .it .ic, .plate, .dlbar .it.stk .lb { transition: none; }
		.dlbar .it:active:not(:disabled) { transform: none; }
	}
</style>
