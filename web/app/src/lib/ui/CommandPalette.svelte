<script lang="ts">
	import Icon from '../Icon.svelte';
	import Sheet from './Sheet.svelte';
	import { filterDestinations, type Destination } from './nav.ts';

	// ⌘K / Ctrl+K: jump to a place, or search titles (routes to /search?q=…).
	let {
		open = $bindable(false),
		items,
		go,
		onsearch
	}: { open?: boolean; items: Destination[]; go: (href: string) => void; onsearch: (q: string) => void } = $props();

	const id = $props.id();
	let q = $state('');
	let active = $state(0);
	let input = $state<HTMLInputElement>();
	const matches = $derived(filterDestinations(items, q).slice(0, 8));
	// The last option searches titles for the typed words.
	const count = $derived(matches.length + (q.trim() ? 1 : 0));

	$effect(() => {
		if (open) {
			q = '';
			active = 0;
			queueMicrotask(() => input?.focus());
		}
	});
	$effect(() => {
		void q;
		active = 0;
	});

	function choose(i: number) {
		open = false;
		if (i < matches.length) go(matches[i].href);
		else if (q.trim()) onsearch(q.trim());
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') { e.preventDefault(); active = count ? (active + 1) % count : 0; }
		else if (e.key === 'ArrowUp') { e.preventDefault(); active = count ? (active - 1 + count) % count : 0; }
		else if (e.key === 'Enter') { e.preventDefault(); if (count) choose(active); }
	}
</script>

<Sheet bind:open title="Search and go" hideTitle size="md" placement="top">
	<div class="ui-palette">
		<div class="field">
			<Icon name="search" size={18} />
			<input
				bind:this={input}
				bind:value={q}
				type="text"
				role="combobox"
				aria-label="Search titles or go to a place"
				aria-expanded={count > 0}
				aria-controls="{id}-list"
				aria-activedescendant={count ? `${id}-o${active}` : undefined}
				aria-autocomplete="list"
				placeholder="Search titles or go to…"
				autocomplete="off"
				spellcheck="false"
				onkeydown={key}
			/>
			<kbd>esc</kbd>
		</div>
		<ul id="{id}-list" role="listbox" aria-label="Results">
			{#each matches as d, i (d.href + d.label)}
				<li id="{id}-o{i}" role="option" aria-selected={i === active} class:active={i === active} onpointermove={() => (active = i)} onclick={() => choose(i)} onkeydown={key}>
					<span class="ic"><Icon name={d.icon} size={17} /></span><span class="grow">{d.label}</span><span class="sec">{d.section}</span>
				</li>
			{/each}
			{#if q.trim()}
				<li id="{id}-o{matches.length}" role="option" aria-selected={active === matches.length} class:active={active === matches.length} onpointermove={() => (active = matches.length)} onclick={() => choose(matches.length)} onkeydown={key}>
					<span class="ic"><Icon name="search" size={17} /></span><span class="grow">Search titles for “{q.trim()}”</span><Icon name="arrow" size={15} />
				</li>
			{/if}
		</ul>
		{#if !count}<p class="hint">Type a title, or a place like Downloads or Settings.</p>{/if}
	</div>
</Sheet>

<style>
	.ui-palette { margin: -8px -22px -22px; }
	.field { display: flex; align-items: center; gap: 10px; padding: 0 16px; height: 56px; border-bottom: 1px solid var(--line); color: var(--text-muted); }
	input { flex: 1; min-width: 0; height: 100%; border: 0; background: transparent; color: var(--text); font: var(--t-body); font-size: 17px; outline: none; }
	input::placeholder { color: var(--text-muted); }
	kbd { padding: 2px 6px; border-radius: 5px; background: var(--surface-2); color: var(--text-muted); font: var(--t-footnote); }
	ul { padding: 6px; max-height: 50dvh; overflow: auto; }
	li { display: flex; align-items: center; gap: 10px; min-height: 42px; padding: 0 10px; border-radius: 9px; cursor: pointer; font: var(--t-callout); font-size: 15px; }
	li.active { background: var(--accent-fill); color: var(--text-on-red); }
	li.active .ic, li.active .sec { color: inherit; }
	.ic { width: 22px; display: grid; place-items: center; color: var(--text-muted); }
	.grow { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.sec { color: var(--text-muted); font: var(--t-footnote); }
	.hint { padding: 16px 18px 20px; color: var(--text-muted); font: var(--t-callout); }
</style>
