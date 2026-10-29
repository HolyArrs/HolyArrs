<script lang="ts">
	import Icon from '../Icon.svelte';
	import { tip } from './tip.ts';
	import { KIND_TABS } from './size.ts';

	// The left rail: Library and Sources. Its own component, so it can move (to the top bar, say) later.
	// Rows are three columns (a 16px icon, a left label, a right count); the header chevron is at the right end and shows on hover.
	let {
		kindOn = $bindable(''),
		srcOn = $bindable(''),
		kindCount,
		srcCount,
		px = $bindable(196)
	}: {
		kindOn?: string;
		srcOn?: string;
		kindCount: (k: string) => number;
		srcCount: (k: string) => number;
		/** The column width the rail takes (the full rail, or the icon strip). */
		px?: number;
	} = $props();

	const KIND_ICON: Record<string, 'film' | 'tv' | 'music'> = { movie: 'film', show: 'tv', artist: 'music' };
	const SRC_ICON: Record<string, 'usenet' | 'torrent'> = { usenet: 'usenet', torrent: 'torrent' };
	const SOURCES = [{ key: 'usenet', label: 'Usenet' }, { key: 'torrent', label: 'Torrent' }];
	const MIN = 160;

	// Which groups are open, and the rail's width (or hidden), are kept in this browser.
	const GROUPS_KEY = 'holyarrs.downloads.groups';
	const RAIL_KEY = 'holyarrs.downloads.rail';
	let groups = $state<Record<string, boolean>>({});
	let railW = $state(196);
	let railOpen = $state(true);
	let narrow = $state(false);
	try {
		const g = typeof localStorage === 'undefined' ? null : JSON.parse(localStorage.getItem(GROUPS_KEY) ?? 'null');
		if (g && typeof g === 'object') for (const [k, v] of Object.entries(g)) if (typeof v === 'boolean') groups[k] = v;
		const v = typeof localStorage === 'undefined' ? null : JSON.parse(localStorage.getItem(RAIL_KEY) ?? 'null');
		if (v && typeof v === 'object') {
			if (Number.isFinite(v.w)) railW = Math.max(MIN, Math.min(360, Math.round(v.w)));
			if (v.open === false) railOpen = false;
		}
	} catch {
		/* private window or unreadable: the defaults */
	}
	const groupOpen = (k: string) => groups[k] ?? true;
	function flipGroup(k: string) {
		groups[k] = !groupOpen(k);
		try {
			localStorage.setItem(GROUPS_KEY, JSON.stringify(groups));
		} catch {
			/* private window: it lasts for this visit */
		}
	}
	function saveRail() {
		try {
			localStorage.setItem(RAIL_KEY, JSON.stringify({ w: Math.round(railW), open: railOpen }));
		} catch {
			/* private window: it lasts for this visit */
		}
	}
	// Narrow screens show the rail as a strip of rows; it is never the icon column there.
	$effect(() => {
		const mq = matchMedia('(max-width: 1099px)');
		const set = () => (narrow = mq.matches);
		set();
		mq.addEventListener('change', set);
		return () => mq.removeEventListener('change', set);
	});
	const shown = $derived(railOpen || narrow);
	$effect(() => {
		px = shown ? railW : 52;
	});
	function setRail(open: boolean) {
		railOpen = open;
		saveRail();
	}
	function down(e: PointerEvent) {
		if (e.button !== 0) return;
		e.preventDefault();
		const el = e.currentTarget as HTMLElement;
		el.setPointerCapture(e.pointerId);
		const x0 = e.clientX;
		const w0 = railW;
		let raw = w0;
		const mv = (m: PointerEvent) => {
			raw = Math.round(w0 + (m.clientX - x0));
			railW = Math.max(100, Math.min(360, raw));
		};
		const up = () => {
			el.removeEventListener('pointermove', mv);
			el.removeEventListener('pointerup', up);
			el.removeEventListener('pointercancel', up);
			// Under the minimum the labels would be cut, so the rail snaps to the icon strip.
			if (raw < MIN) {
				railW = Math.max(MIN, w0);
				railOpen = false;
			}
			saveRail();
		};
		el.addEventListener('pointermove', mv);
		el.addEventListener('pointerup', up);
		el.addEventListener('pointercancel', up);
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft' || e.key === 'ArrowRight') {
			e.preventDefault();
			e.stopPropagation();
			railW = Math.max(MIN, Math.min(360, railW + (e.key === 'ArrowRight' ? 16 : -16)));
			saveRail();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			setRail(false);
		}
	}
	const kinds = KIND_TABS.filter((t) => t.key);
</script>

{#snippet head(k: string, label: string)}
	{@const open = groupOpen(k)}
	<button type="button" class="gh" aria-expanded={open} onclick={() => flipGroup(k)} data-testid="rail-fold" data-key={k}><span class="lb">{label}</span><span class="chv" aria-hidden="true"><Icon name="chevronDown" size={14} stroke={2.4} /></span></button>
{/snippet}

<nav class="rail" class:shut={!shown} aria-label="Downloads filters" data-testid="dl-rail">
	{#if shown}
		<button type="button" class="rhide" aria-label="Hide filters" use:tip={'Hide filters'} onclick={() => setRail(false)} data-testid="rail-hide"><Icon name="chevronDown" size={14} stroke={2.2} /></button>
		<div class="rg" role="group" aria-label="Library" data-testid="dl-type-filter">
			{@render head('library', 'Library')}
			{#if groupOpen('library')}
				{#each kinds as t (t.key)}
					<button type="button" class="ri" class:on={kindOn === t.key} class:zero={kindCount(t.key) === 0} aria-pressed={kindOn === t.key} data-testid="rail-kind" data-key={t.key} data-count={kindCount(t.key)} onclick={() => (kindOn = kindOn === t.key ? '' : t.key)}><span class="ico"><Icon name={KIND_ICON[t.key]} size={16} /></span><span class="lb">{t.label}</span><b class="n">{kindCount(t.key)}</b></button>
				{/each}
			{/if}
		</div>
		<div class="rg" role="group" aria-label="Sources" data-testid="dl-source-filter">
			{@render head('sources', 'Sources')}
			{#if groupOpen('sources')}
				{#each SOURCES as t (t.key)}
					<button type="button" class="ri" class:on={srcOn === t.key} class:zero={srcCount(t.key) === 0} aria-pressed={srcOn === t.key} data-testid="rail-source" data-key={t.key} data-count={srcCount(t.key)} onclick={() => (srcOn = srcOn === t.key ? '' : t.key)}><span class="ico"><Icon name={SRC_ICON[t.key]} size={16} /></span><span class="lb">{t.label}</span><b class="n">{srcCount(t.key)}</b></button>
				{/each}
			{/if}
		</div>
	{:else}
		<button type="button" class="rshow" aria-label="Show filters" use:tip={{ text: 'Show filters', side: 'right' }} onclick={() => setRail(true)} data-testid="rail-show"><Icon name="chevronRight" size={14} stroke={2.2} /></button>
		<div class="ics" role="group" aria-label="Filters" data-testid="rail-icons">
			{#each kinds as t (t.key)}
				<button type="button" class="ic" class:on={kindOn === t.key} aria-pressed={kindOn === t.key} aria-label="{t.label}, {kindCount(t.key)}" use:tip={{ text: `${t.label} \u00b7 ${kindCount(t.key)}`, side: 'right' }} data-testid="rail-icon-kind" data-key={t.key} data-count={kindCount(t.key)} onclick={() => (kindOn = kindOn === t.key ? '' : t.key)}><Icon name={KIND_ICON[t.key]} size={16} /><b class="bd">{kindCount(t.key)}</b></button>
			{/each}
			<hr />
			{#each SOURCES as t (t.key)}
				<button type="button" class="ic" class:on={srcOn === t.key} aria-pressed={srcOn === t.key} aria-label="{t.label}, {srcCount(t.key)}" use:tip={{ text: `${t.label} \u00b7 ${srcCount(t.key)}`, side: 'right' }} data-testid="rail-icon-source" data-key={t.key} data-count={srcCount(t.key)} onclick={() => (srcOn = srcOn === t.key ? '' : t.key)}><Icon name={SRC_ICON[t.key]} size={16} /><b class="bd">{srcCount(t.key)}</b></button>
			{/each}
		</div>
	{/if}
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
	<div class="rz" role="separator" aria-orientation="vertical" aria-label="Resize filters" aria-valuenow={shown ? railW : 0} tabindex="0" class:fixed={!shown} onpointerdown={(e) => shown && railOpen && down(e)} ondblclick={() => setRail(!railOpen)} onkeydown={key} data-testid="rail-handle"><i aria-hidden="true"></i></div>
</nav>

<style>
	.rail { position: relative; display: grid; align-content: start; gap: 14px; overflow-x: hidden; overflow-y: auto; padding: 10px 0; border-right: 1px solid var(--line); min-width: 0; }
	.rz { position: absolute; top: 0; right: -4px; bottom: 0; z-index: 6; width: 9px; cursor: col-resize; touch-action: none; }
	.rz.fixed { cursor: pointer; }
	.rz i { position: absolute; top: 50%; left: 3px; width: 3px; height: 38px; margin-top: -19px; border-radius: 2px; background: var(--line-strong, var(--text-muted)); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.rz:hover i, .rz:focus-visible i, .rail:hover .rz i { opacity: 0.6; }
	.rz:hover i, .rz:focus-visible i { opacity: 1; }
	.rz:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rhide, .rshow { display: grid; place-items: center; width: 28px; height: 28px; padding: 0; border: 0; border-radius: 6px; background: none; color: var(--text-muted); cursor: pointer; }
	.rhide { justify-self: end; margin: -4px 0 -8px; }
	.rhide :global(svg) { transform: rotate(90deg); }
	.rshow :global(svg) { transform: none; }
	.rshow { margin: 0 auto; }
	.rhide:hover, .rshow:hover { background: var(--surface-3); color: var(--text); }
	.rhide:focus-visible, .rshow:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rail.shut { padding: 10px 0; overflow: hidden; }
	.ics { display: grid; justify-items: center; gap: 4px; }
	.ics hr { width: 28px; height: 1px; margin: 4px 0; border: 0; background: var(--line); }
	.ic { position: relative; display: grid; place-items: center; width: 40px; height: 36px; padding: 0; border: 0; border-radius: 8px; background: none; color: var(--text-muted); cursor: pointer; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out); }
	.ic:hover { background: var(--surface-3); color: var(--text); }
	.ic.on { background: var(--surface-1); color: var(--accent-text, var(--text)); box-shadow: 0 1px 2px rgb(0 0 0 / 0.12), 0 4px 10px rgb(0 0 0 / 0.08); }
	.ic:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.rail .rg { display: grid; gap: 2px; padding: 0 var(--dl-gutter); }
	.ri { display: grid; grid-template-columns: 16px minmax(0, 1fr) auto; align-items: center; column-gap: 8px; height: 28px; min-width: 0; padding: 0 8px; border: 0; border-radius: 6px; background: none; color: var(--text); font: var(--t-footnote); text-align: left; cursor: pointer; overflow: hidden; transition: background var(--dur-fast) var(--ease-out), color var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.ri .ico { display: grid; place-items: center; color: var(--text-muted); }
	.ri .lb { min-width: 0; overflow: hidden; white-space: nowrap; text-overflow: clip; }
	.ri .n { min-width: 2ch; text-align: right; color: var(--text-muted); font-weight: 600; font-variant-numeric: tabular-nums; }
	.ri:hover { background: var(--surface-3); }
	.ri.zero:not(.on) { opacity: 0.72; }
	.ri.on { background: var(--surface-3); color: var(--text); font-weight: 600; }
	.ri.on .ico { color: var(--accent-text, var(--text)); }
	.ri.on .n { color: var(--text); }
	.ri:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ri:active, .gh:active, .ic:active, .rhide:active, .rshow:active { transform: scale(0.97); }
	.gh { display: grid; grid-template-columns: minmax(0, 1fr) 24px; align-items: center; width: 100%; min-height: 26px; margin-top: 6px; padding: 0 0 0 8px; border: 0; border-radius: 6px; background: none; color: var(--text-muted); font: 600 10px/1.3 var(--font-ui); letter-spacing: 0.1em; text-transform: uppercase; text-align: left; cursor: pointer; }
	.gh .chv { display: grid; place-items: center; width: 24px; height: 24px; opacity: 0; transition: opacity var(--dur-fast) var(--ease-out), transform var(--dur-fast) var(--ease-out); }
	.gh:hover .chv, .gh:focus-visible .chv { opacity: 1; }
	@media (hover: none) { .gh .chv { opacity: 1; } }
	.gh[aria-expanded='false'] .chv { transform: rotate(-90deg); opacity: 1; }
	.gh:hover { color: var(--text); }
	.gh:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.ic .bd { position: absolute; top: 0; right: 0; min-width: 16px; height: 16px; padding: 0 4px; display: grid; place-items: center; border-radius: 999px; background: var(--surface-3); color: var(--text); font: 600 10px/1 var(--font-ui); font-variant-numeric: tabular-nums; }
	.ics .ic.on { box-shadow: inset 2px 0 0 var(--accent); }
	.chv :global(svg.icon) { width: var(--dl-icon-sm); height: var(--dl-icon-sm); }
	.rhide :global(svg.icon), .rshow :global(svg.icon) { width: var(--dl-icon-sm); height: var(--dl-icon-sm); }
	.ic :global(svg.icon), .ri :global(svg.icon) { width: var(--dl-icon); height: var(--dl-icon); }
	@media (max-width: 1099px) {
		.rz, .rhide, .rshow { display: none; }
		.rail.shut { padding: 8px; overflow-x: auto; }
		.rail.shut::before { content: none; }
		.rail { display: flex; gap: 6px; overflow-x: auto; padding: 8px var(--dl-gutter); border-right: 0; border-bottom: 1px solid var(--line); scrollbar-width: none; }
		.rail::-webkit-scrollbar { display: none; }
		.rail .rg { display: flex; flex: none; align-items: center; gap: 4px; padding: 0; }
		.ri { display: inline-grid; flex: none; width: auto; white-space: nowrap; }
		.gh { display: inline-flex; width: auto; margin: 0; padding: 0 4px; }
		.gh .chv { display: none; }
	}
</style>
