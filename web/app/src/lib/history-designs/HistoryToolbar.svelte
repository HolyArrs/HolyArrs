<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from '#lib/Icon.svelte';
	import Button from '#lib/ui/Button.svelte';
	import Popover from '#lib/ui/Popover.svelte';
	import Sheet from '#lib/ui/Sheet.svelte';
	import { dateRangeProblem, kindGroups, type HistoryFilter } from '#lib/history.ts';
	import { tiers, groupsOfTier } from '#lib/history-views.ts';
	import type { TitleView } from '#lib/types.ts';

	// The slim filter bar the History designs share: a title finder, a date
	// range and a menu of every kind of Event (grouped by how much it matters).
	// On phones it is the title finder and one Filters button opening a sheet.
	let {
		filter,
		titles,
		onfilter,
		groups,
		phone = false,
		extra,
		sheetExtra
	}: {
		filter: HistoryFilter;
		titles: TitleView[];
		onfilter: (change: Partial<HistoryFilter>) => void;
		/** The kinds offered (kindGroups keys); every kind when left out. */
		groups?: string[];
		phone?: boolean;
		/** More controls at the end of the bar (wide screens) or the top of the sheet. */
		extra?: Snippet;
		/** Controls for the phone sheet only. */
		sheetExtra?: Snippet;
	} = $props();

	const uid = $props.id();
	const offered = $derived(groups ?? kindGroups.map((g) => g.key));
	const sections = $derived(
		tiers
			// Imported sits under Important only (an upgrade is a change, but it is filtered as an import).
			.map((t) => ({ ...t, groups: kindGroups.filter((g) => offered.includes(g.key) && groupsOfTier(t.key).includes(g.key) && !(g.key === 'import' && t.key === 'changes')) }))
			.map((t) => (t.groups.length ? t : { ...t, groups: t.key === 'changes' && offered.includes('import') && !groupsOfTier('important').some((k) => offered.includes(k)) ? kindGroups.filter((g) => g.key === 'import') : [] }))
			.filter((t) => t.groups.length)
	);
	const picked = $derived(filter.groups.filter((g) => offered.includes(g)));
	const titleName = $derived(titles.find((t) => t.id === filter.title)?.name ?? '');
	const problem = $derived(dateRangeProblem(filter));
	const active = $derived((filter.title ? 1 : 0) + picked.length + (filter.from ? 1 : 0) + (filter.to ? 1 : 0));

	// The title finder.
	let q = $state('');
	let finding = $state(false);
	let hi = $state(0);
	const matches = $derived.by(() => {
		const s = q.trim().toLowerCase();
		const list = s ? titles.filter((t) => t.name.toLowerCase().includes(s)) : titles;
		return [...list].sort((a, b) => (s ? Number(!a.name.toLowerCase().startsWith(s)) - Number(!b.name.toLowerCase().startsWith(s)) : 0) || a.name.localeCompare(b.name)).slice(0, 8);
	});
	function choose(t: TitleView) {
		onfilter({ title: t.id, episode: undefined });
		q = '';
		finding = false;
	}
	function findKey(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') { e.preventDefault(); finding = true; hi = Math.min(hi + 1, matches.length - 1); }
		else if (e.key === 'ArrowUp') { e.preventDefault(); hi = Math.max(hi - 1, 0); }
		else if (e.key === 'Enter' && finding && matches[hi]) { e.preventDefault(); choose(matches[hi]); }
		else if (e.key === 'Escape') { finding = false; }
	}
	function toggle(key: string, on: boolean) {
		onfilter({ groups: on ? [...filter.groups, key] : filter.groups.filter((g) => g !== key) });
	}
	function clear() {
		onfilter({ title: undefined, episode: undefined, groups: [], from: undefined, to: undefined });
	}
	let sheet = $state(false);
	const kindsText = $derived(picked.length === 0 ? 'Every kind' : picked.length === 1 ? (kindGroups.find((g) => g.key === picked[0])?.label ?? '1 kind') : `${picked.length} kinds`);
</script>

{#snippet finder()}
	<div class="finder" class:has={!!filter.title}>
		<Icon name="search" size={16} />
		{#if filter.title}
			<span class="chosen" data-testid="hv-title-chosen">{titleName || 'One title'}</span>
			<button type="button" class="clear-title" aria-label="Every title" onclick={() => onfilter({ title: undefined, episode: undefined })}><Icon name="x" size={14} /></button>
		{:else}
			<input
				id="{uid}-find"
				class="find"
				type="search"
				placeholder="Movie, show or artist"
				autocomplete="off"
				role="combobox"
				aria-expanded={finding && matches.length > 0}
				aria-controls="{uid}-list"
				aria-autocomplete="list"
				aria-activedescendant={finding && matches[hi] ? `${uid}-o${hi}` : undefined}
				aria-label="Only one movie, show or artist"
				data-testid="hv-title-find"
				bind:value={q}
				oninput={() => { finding = true; hi = 0; }}
				onfocus={() => (finding = true)}
				onblur={() => setTimeout(() => (finding = false), 150)}
				onkeydown={findKey}
			/>
			{#if finding && matches.length}
				<ul id="{uid}-list" class="matches" role="listbox" aria-label="Titles">
					{#each matches as t, i (t.id)}
						<li id="{uid}-o{i}" role="option" aria-selected={i === hi} class:hi={i === hi} onpointerdown={(e) => { e.preventDefault(); choose(t); }}>
							<span class="m-name">{t.name}</span><span class="m-kind">{t.kind === 'artist' ? 'Artist' : t.kind === 'show' ? 'Show' : 'Movie'}{t.year ? ` · ${t.year}` : ''}</span>
						</li>
					{/each}
				</ul>
			{/if}
		{/if}
	</div>
{/snippet}

{#snippet dates()}
	<div class="dates" role="group" aria-label="Dates">
		<label class="date"><span>From</span><input class="input" type="date" value={filter.from ?? ''} aria-invalid={problem ? 'true' : undefined} onchange={(e) => onfilter({ from: e.currentTarget.value || undefined })} /></label>
		<label class="date"><span>To</span><input class="input" type="date" value={filter.to ?? ''} aria-invalid={problem ? 'true' : undefined} onchange={(e) => onfilter({ to: e.currentTarget.value || undefined })} /></label>
	</div>
{/snippet}

{#snippet kinds()}
	<div class="kinds">
		{#each sections as s (s.key)}
			<p class="k-head">{s.label}</p>
			{#each s.groups as g (g.key)}
				<label class="k-row"><input type="checkbox" checked={filter.groups.includes(g.key)} onchange={(e) => toggle(g.key, e.currentTarget.checked)} /><span>{g.label}</span></label>
			{/each}
		{/each}
	</div>
{/snippet}

{#if phone}
	<div class="hv-bar phone" data-testid="hv-toolbar">
		{@render finder()}
		<Button variant="secondary" icon="grid" onclick={() => (sheet = true)}>Filters{active - (filter.title ? 1 : 0) ? ` (${active - (filter.title ? 1 : 0)})` : ''}</Button>
	</div>
	{#if extra}<div class="phone-extra">{@render extra()}</div>{/if}
	<Sheet bind:open={sheet} title="Filter the history" size="md">
		<div class="sheet">
			{#if sheetExtra}{@render sheetExtra()}{/if}
			{@render dates()}
			{#if problem}<p class="err" role="alert">{problem}</p>{/if}
			<p class="legend">What happened</p>
			{@render kinds()}
		</div>
		{#snippet footer()}
			{#if active}<Button variant="plain" onclick={clear}>Show everything</Button>{/if}
			<Button variant="primary" onclick={() => (sheet = false)}>Done</Button>
		{/snippet}
	</Sheet>
{:else}
	<div class="hv-bar" data-testid="hv-toolbar">
		{@render finder()}
		{@render dates()}
		<Popover label="What happened" variant="button" text={kindsText} width={260}>
			{#snippet children()}{@render kinds()}{/snippet}
		</Popover>
		{#if extra}{@render extra()}{/if}
		{#if active}<Button variant="plain" size="sm" icon="x" onclick={clear}>Show everything</Button>{/if}
	</div>
	{#if problem}<p class="err" role="alert">{problem}</p>{/if}
{/if}

<style>
	.hv-bar { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; min-width: 0; }
	.hv-bar.phone { flex-wrap: nowrap; }
	.hv-bar.phone > :global(:last-child) { flex: none; }
	.phone-extra { margin-top: 10px; }
	.finder { position: relative; display: flex; align-items: center; gap: 8px; flex: 1 1 240px; min-width: 0; max-width: 360px; height: 36px; padding: 0 6px 0 12px; border-radius: var(--r-pill); background: var(--surface-2); color: var(--text-muted); box-shadow: inset 0 0 0 1px var(--line); }
	.hv-bar.phone .finder { max-width: none; height: 44px; }
	.finder:focus-within { box-shadow: inset 0 0 0 1px var(--line-strong); background: var(--surface-1); }
	.finder.has { background: var(--accent-soft); color: var(--accent-text); box-shadow: none; }
	.find { flex: 1; min-width: 0; height: 100%; border: 0; background: none; color: var(--text); font: var(--t-callout); outline: none; }
	.find::placeholder { color: var(--text-faint); }
	.chosen { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font: var(--t-callout); font-weight: 600; }
	.clear-title { display: grid; place-items: center; width: 28px; height: 28px; border-radius: 50%; color: var(--text-muted); }
	.clear-title:hover { background: var(--surface-3); color: var(--text); }
	.matches { position: absolute; z-index: 60; top: calc(100% + 6px); left: 0; right: 0; min-width: 240px; max-height: 320px; overflow: auto; margin: 0; padding: 6px; list-style: none; border-radius: 14px; background: var(--surface-1); box-shadow: var(--shadow-lift), 0 0 0 1px var(--line); }
	.matches li { display: flex; align-items: baseline; justify-content: space-between; gap: 10px; padding: 8px 10px; border-radius: var(--r-btn); cursor: pointer; }
	.matches li.hi, .matches li:hover { background: var(--surface-2); }
	.m-name { min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; color: var(--text); font: var(--t-callout); }
	.m-kind { flex: none; color: var(--text-muted); font: var(--t-footnote); }
	.dates { display: flex; gap: 6px; }
	.date { display: flex; align-items: center; gap: 6px; color: var(--text-muted); font: var(--t-footnote); }
	.date .input { width: 142px; min-height: 36px; padding: 0 8px; border-radius: var(--r-pill); font: var(--t-footnote); }
	.kinds { display: flex; flex-direction: column; padding: 2px; }
	.k-head { margin: 8px 8px 2px; color: var(--text-muted); font: var(--t-caption); font-weight: 600; letter-spacing: .04em; text-transform: uppercase; }
	.k-head:first-child { margin-top: 2px; }
	.k-row { display: flex; align-items: center; gap: 10px; min-height: 36px; padding: 0 8px; border-radius: var(--r-btn); font: var(--t-callout); cursor: pointer; }
	.k-row:hover { background: var(--surface-2); }
	.k-row input { width: 16px; height: 16px; accent-color: var(--accent); }
	.err { margin-top: 6px; color: var(--fail); font: var(--t-callout); }
	.sheet { display: flex; flex-direction: column; gap: 14px; }
	.sheet .dates { flex-direction: column; }
	.sheet .date { justify-content: space-between; font: var(--t-callout); }
	.sheet .date .input { width: 60%; min-height: 44px; }
	.sheet .k-row { min-height: 44px; }
	.legend { color: var(--text-muted); font: var(--t-subhead); }
	@media (pointer: coarse) { .finder { height: 44px; } }
</style>
