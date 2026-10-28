<script lang="ts">
	// History's bar, built like the Downloads table's:
	// five status tabs with counts (All · Came in · Problems · Changes ·
	// Routine), toggle sub-tabs under the picked one, and a finder that keeps
	// the page to one movie, show or artist. The tabs live in the address
	// (&show=tab or tab.sub).
	import type { Snippet } from 'svelte';
	import { page } from '$app/state';
	import { replaceState } from '$app/navigation';
	import { fade, slide, type TransitionConfig } from 'svelte/transition';
	import Chip from '../ui/Chip.svelte';
	import Icon from '../Icon.svelte';
	import { reducedMotion } from '../motion.ts';
	import { DUR, easeIn, easeOut } from '../ui/transitions.ts';
	import { ALL, lookTabs, showParam } from '../history-looks.ts';
	import type { HistoryFilter } from '../history.ts';
	import type { TitleView } from '../types.ts';

	let {
		show = $bindable(ALL),
		sub = $bindable(''),
		counts,
		subCounts,
		filter,
		titles,
		episodeName = '',
		onfilter,
		tools
	}: {
		show: string;
		sub: string;
		counts: Record<string, number>;
		subCounts: Record<string, number>;
		filter: HistoryFilter;
		titles: TitleView[];
		episodeName?: string;
		onfilter: (change: Partial<HistoryFilter>) => void;
		/** Page controls beside the finder (poster size, details panel). */
		tools?: Snippet;
	} = $props();

	const uid = $props.id();
	const tab = $derived(lookTabs.find((t) => t.key === show) ?? lookTabs[0]);

	function remember() {
		try {
			const u = new URL(page.url.href);
			const v = showParam(show, sub);
			if (v) u.searchParams.set('show', v);
			else u.searchParams.delete('show');
			if (u.href !== page.url.href) replaceState(u, page.state);
		} catch {
			/* the router is not ready yet; the next pick writes it */
		}
	}
	function choose(k: string) {
		show = k;
		sub = '';
		remember();
	}
	function chooseSub(k: string) {
		sub = sub === k ? '' : k;
		remember();
	}

	function subIn(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.base, easing: easeOut });
	}
	function subOut(node: Element): TransitionConfig {
		return reducedMotion() ? fade(node, { duration: DUR.fast }) : slide(node, { duration: DUR.fast, easing: easeIn });
	}
	function swapIn(node: Element): TransitionConfig {
		return fade(node, { duration: reducedMotion() ? 0 : DUR.fast, easing: easeOut });
	}

	// The title finder.
	let q = $state('');
	let finding = $state(false);
	let hi = $state(0);
	const titleName = $derived(titles.find((t) => t.id === filter.title)?.name ?? '');
	const found = $derived.by(() => {
		const s = q.trim().toLowerCase();
		const list = s ? titles.filter((t) => t.name.toLowerCase().includes(s)) : titles;
		return [...list].sort((a, b) => (s ? Number(!a.name.toLowerCase().startsWith(s)) - Number(!b.name.toLowerCase().startsWith(s)) : 0) || a.name.localeCompare(b.name)).slice(0, 8);
	});
	function pickTitle(t: TitleView) {
		onfilter({ title: t.id, episode: undefined });
		q = '';
		finding = false;
	}
	function findKey(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			finding = true;
			hi = Math.min(hi + 1, found.length - 1);
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			hi = Math.max(hi - 1, 0);
		} else if (e.key === 'Enter' && finding && found[hi]) {
			e.preventDefault();
			pickTitle(found[hi]);
		} else if (e.key === 'Escape') finding = false;
	}
	const dates = $derived(filter.from && filter.to ? `${filter.from} to ${filter.to}` : filter.from ? `From ${filter.from}` : filter.to ? `Until ${filter.to}` : '');
</script>

<div class="lb" data-testid="look-bar">
	<div class="top">
		<div class="chips" role="group" aria-label="Show" data-testid="look-tabs">
			{#each lookTabs as t (t.key)}
				{@const n = counts[t.key]}
				<span class="cw" class:zero={n === 0} data-key={t.key} data-tip={t.hint}>
					<Chip label={t.label} count={n} selected={show === t.key} describedby="{uid}-h-{t.key}" onclick={() => choose(t.key)} />
					<span class="sr-only" id="{uid}-h-{t.key}">{t.hint}</span>
				</span>
			{/each}
		</div>
		<div class="end">
		{#if tools}<div class="tools">{@render tools()}</div>{/if}
		<div class="finder" class:has={!!filter.title}>
			<Icon name="search" size={16} />
			{#if filter.title}
				<span class="chosen" data-testid="look-title">{titleName || 'One title'}{episodeName && filter.episode ? ` · ${episodeName}` : ''}</span>
				<button type="button" class="clear" aria-label="Every title" onclick={() => onfilter({ title: undefined, episode: undefined })}><Icon name="x" size={14} /></button>
			{:else}
				<input
					class="find"
					type="search"
					placeholder="Movie, show or artist"
					autocomplete="off"
					role="combobox"
					aria-expanded={finding && found.length > 0}
					aria-controls="{uid}-list"
					aria-autocomplete="list"
					aria-activedescendant={finding && found[hi] ? `${uid}-o${hi}` : undefined}
					aria-label="Only one movie, show or artist"
					bind:value={q}
					oninput={() => {
						finding = true;
						hi = 0;
					}}
					onfocus={() => (finding = true)}
					onblur={() => setTimeout(() => (finding = false), 150)}
					onkeydown={findKey}
				/>
				{#if finding && found.length}
					<ul id="{uid}-list" class="found" role="listbox" aria-label="Titles">
						{#each found as t, i (t.id)}
							<!-- svelte-ignore a11y_click_events_have_key_events -->
							<li id="{uid}-o{i}" role="option" aria-selected={i === hi} class:hi={i === hi} onpointerdown={(e) => { e.preventDefault(); pickTitle(t); }}>
								<span class="f-name">{t.name}</span><span class="f-kind">{t.kind === 'artist' ? 'Artist' : t.kind === 'show' ? 'Show' : 'Movie'}{t.year ? ` · ${t.year}` : ''}</span>
							</li>
						{/each}
					</ul>
				{/if}
			{/if}
		</div>
		</div>
	</div>

	{#if tab.subs.length}
		<div class="subrow" in:subIn out:subOut>
			{#key show}
				<div class="subs" role="group" aria-label="{tab.label}: show" data-testid="look-subtabs" in:swapIn>
					{#each tab.subs as s (s.key)}
						{@const n = subCounts[s.key]}
						<span class="cw sm" class:zero={n === 0} data-key={s.key} data-tip={s.hint}>
							<Chip label={s.label} count={n} selected={sub === s.key} describedby="{uid}-s-{s.key}" onclick={() => chooseSub(s.key)} />
							<span class="sr-only" id="{uid}-s-{s.key}">{s.hint}</span>
						</span>
					{/each}
				</div>
			{/key}
		</div>
	{/if}

	{#if dates}
		<p class="dates"><span>{dates}</span><button type="button" class="clear" aria-label="Every date" onclick={() => onfilter({ from: undefined, to: undefined })}><Icon name="x" size={14} /></button></p>
	{/if}
</div>

<style>
	.lb { display: flex; flex-direction: column; gap: 10px; min-width: 0; margin-bottom: 16px; }
	.top { display: flex; align-items: center; justify-content: space-between; gap: 10px; flex-wrap: wrap; }
	.chips, .subs { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.subs { gap: 6px; }
	.cw { position: relative; display: inline-flex; transition: opacity var(--dur-fast) var(--ease-out); }
	.cw[data-tip]::after {
		content: attr(data-tip); position: absolute; z-index: 60; left: 0; top: calc(100% + 6px);
		display: none; width: max-content; max-width: min(280px, calc(100vw - 32px)); padding: 6px 10px; border-radius: 8px;
		background: var(--text); color: var(--bg); font: var(--t-footnote); white-space: normal; pointer-events: none;
	}
	.cw[data-tip]:hover::after, .cw[data-tip]:has(:focus-visible)::after { display: block; animation: tip-in var(--dur-fast) var(--ease-out) 500ms backwards; }
	.chips .cw:nth-last-child(-n + 2)[data-tip]::after { left: auto; right: 0; }
	@keyframes tip-in { from { opacity: 0; translate: 0 -2px; } }
	@media (hover: none) { .cw[data-tip]:hover::after { display: none; } }
	/* Counts keep one width per digit, so a count changing does not move the chips after it. */
	.cw :global(.n) { font-variant-numeric: tabular-nums; }
	.cw.zero { opacity: 0.5; }
	.cw.zero:hover, .cw.zero:focus-within { opacity: 0.8; }
	.cw.sm :global(.main) { min-height: 28px; padding: 0 11px; font-size: 13px; }
	.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip-path: inset(50%); white-space: nowrap; }

	.end { display: flex; align-items: center; justify-content: flex-end; gap: 8px; flex: 0 1 auto; min-width: 0; }
	.tools { display: flex; align-items: center; gap: 8px; flex: none; }
	.finder { position: relative; display: flex; align-items: center; gap: 8px; flex: 0 1 280px; min-width: 0; height: 36px; padding: 0 4px 0 12px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); box-shadow: inset 0 0 0 1px var(--line); transition: background var(--dur-fast) var(--ease-out); }
	.finder:focus-within { background: var(--surface-1); box-shadow: inset 0 0 0 1px var(--line-strong, var(--line)); }
	.finder.has { background: var(--accent-soft); color: var(--accent-text); box-shadow: none; }
	.find { flex: 1; min-width: 0; height: 100%; border: 0; background: none; color: var(--text); font: var(--t-callout); outline: none; }
	.find::placeholder { color: var(--text-faint); }
	.chosen { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font: var(--t-callout); font-weight: 600; }
	.clear { display: grid; place-items: center; flex: none; width: 28px; height: 28px; border: 0; border-radius: 50%; background: none; color: var(--text-muted); cursor: pointer; }
	.clear:hover { background: var(--surface-3); color: var(--text); }
	.clear:focus-visible { outline: none; box-shadow: var(--focus-ring); }
	.found { position: absolute; z-index: 60; top: calc(100% + 6px); left: 0; right: 0; min-width: 240px; max-height: 320px; overflow: auto; margin: 0; padding: 6px; list-style: none; border-radius: 14px; background: var(--surface-1); box-shadow: var(--shadow-lift), 0 0 0 1px var(--line); }
	.found li { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; padding: 8px 10px; border-radius: var(--r-btn); cursor: pointer; }
	.found li.hi, .found li:hover { background: var(--surface-2); }
	.f-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font: var(--t-callout); }
	.f-kind { flex: none; color: var(--text-muted); font: var(--t-footnote); }
	.dates { display: inline-flex; align-items: center; gap: 4px; align-self: flex-start; padding-left: 12px; border-radius: var(--r-pill); background: var(--accent-soft); color: var(--accent-text); font: var(--t-footnote); font-weight: 600; }

	@media (max-width: 767px) {
		.top { flex-direction: column; flex-wrap: nowrap; align-items: stretch; }
		.end { order: -1; flex-direction: row-reverse; }
		.finder { flex: 1 1 0; height: 44px; }
		.chips { width: 100%; display: grid; grid-template-columns: repeat(5, minmax(0, 1fr)); align-items: stretch; gap: 4px; }
		.chips .cw, .chips .cw :global(.ui-chip) { width: 100%; min-width: 0; }
		.chips .cw :global(.ui-chip), .chips .cw :global(.main) { height: 100%; }
		.chips .cw :global(.main) { width: 100%; flex-direction: column; justify-content: center; gap: 1px; min-height: 48px; padding: 4px 2px; font-size: 12.5px; line-height: 1.15; text-align: center; white-space: normal; overflow-wrap: anywhere; }
		.subs { flex-wrap: nowrap; overflow-x: auto; scrollbar-width: none; margin: 0 calc(var(--gutter, 16px) * -1); padding: 2px var(--gutter, 16px); }
		.subs::-webkit-scrollbar { display: none; }
		.subs .cw { flex: none; }
	}
	@media (pointer: coarse) { .clear { width: 40px; height: 40px; } }
	@media (prefers-reduced-motion: reduce) { .cw, .finder { transition: none; } .cw[data-tip]::after { animation-name: none; } }
</style>
