<script lang="ts" module>
	/** One row of a list sheet. */
	export interface SheetRow {
		key: string;
		name: string;
		hint?: string;
		/** The pill's word and tone (a fixed-width column on the right). */
		pill?: { word: string; tone: 'ok' | 'warn' | 'bad' | 'quiet' };
		/** Wants a look (failed, not found, left out): sorted first and kept by "Needs a look". */
		look?: boolean;
	}
	/** ShowList opens the list sheet on items, each made into a row. */
	export type ShowList = <T>(title: string, items: readonly T[], row: (t: T, i: number) => SheetRow) => void;
</script>

<script lang="ts" generics="T">
	import Sheet from '../ui/Sheet.svelte';
	import Button from '../ui/Button.svelte';
	import SegmentedControl from '../ui/SegmentedControl.svelte';
	import StatusPill, { type StatusKind } from '../ui/StatusPill.svelte';

	// One paged list for every "Show": search, "Needs a look" first, 100 rows
	// at a time. Rows are built only while the sheet is open, and only 100
	// are drawn, so a list of thousands opens at once.
	let {
		open = $bindable(false),
		title,
		items,
		row,
		empty = 'Nothing here.'
	}: {
		open?: boolean;
		title: string;
		items: readonly T[];
		row: (t: T, i: number) => SheetRow;
		empty?: string;
	} = $props();

	const PAGE = 100;
	let q = $state('');
	let shown = $state(PAGE);
	let which = $state<'look' | 'all'>('look');
	const uid = $props.id();

	// Built once per list (not per keystroke): rows, and whether "Needs a look" is worth offering.
	const rows = $derived.by(() => {
		const out = open ? items.map((t, i) => row(t, i)) : [];
		const look = out.filter((r) => r.look);
		const mixed = look.length > 0 && look.length < out.length;
		// Lines that want a look come first; the rest keep their order.
		return { all: mixed ? [...look, ...out.filter((r) => !r.look)] : out, look, mixed };
	});
	const list = $derived.by(() => {
		const base = rows.mixed && which === 'look' ? rows.look : rows.all;
		const t = q.trim().toLowerCase();
		return t ? base.filter((r) => r.name.toLowerCase().includes(t) || (r.hint ?? '').toLowerCase().includes(t)) : base;
	});
	const page = $derived(list.slice(0, shown));

	// A new list starts at the top: first page, no search, problems first.
	$effect(() => {
		if (open) {
			void items;
			q = '';
			shown = PAGE;
			which = 'look';
		}
	});

	const kinds: Record<string, StatusKind> = { ok: 'in_library', warn: 'missing', bad: 'failed', quiet: 'requested' };
</script>

<Sheet bind:open {title} size="lg" placement="auto">
	<div class="tools">
		<label class="sr-only" for="{uid}-q">Search this list</label>
		<input id="{uid}-q" class="input" type="search" placeholder="Search" bind:value={q} oninput={() => (shown = PAGE)} autocomplete="off" />
		{#if rows.mixed}
			<SegmentedControl
				label="Which lines"
				options={[
					{ value: 'look', label: 'Needs a look' },
					{ value: 'all', label: 'All' }
				]}
				bind:value={which}
				onchange={() => (shown = PAGE)}
			/>
		{/if}
	</div>
	<p class="hint count" aria-live="polite" data-testid="items-count">Showing {page.length} of {list.length}</p>
	{#if list.length === 0}
		<p class="hint">{q.trim() ? 'Nothing matches.' : empty}</p>
	{:else}
		<div class="rows" data-testid="items-list">
			{#each page as r (r.key)}
				<div class="srow">
					<div class="txt">
						<span class="label name">{r.name}</span>
						{#if r.hint}<span class="hint">{r.hint}</span>{/if}
					</div>
					{#if r.pill}
						<div class="ctl"><span class="pill"><StatusPill kind={kinds[r.pill.tone]} label={r.pill.word} /></span></div>
					{/if}
				</div>
			{/each}
		</div>
		{#if list.length > shown}
			<div class="more"><Button onclick={() => (shown += PAGE)}>Show 100 more</Button></div>
		{/if}
	{/if}
</Sheet>

<style>
	.tools { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
	.tools .input { flex: 1 1 200px; min-width: 0; }
	.count { margin: 8px 0 0; }
	.rows { margin-top: 4px; }
	.name { display: -webkit-box; -webkit-line-clamp: 2; line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; overflow-wrap: anywhere; }
	.rows .hint { overflow-wrap: anywhere; }
	/* One fixed column for the pills, so they line up row under row. */
	.pill { display: flex; justify-content: flex-start; width: 184px; min-width: 0; }
	.more { display: flex; justify-content: center; padding-top: 12px; }
	@media (max-width: 600px) {
		/* Phones: the pill goes under the name, so no word is cut. */
		.rows .srow { grid-template-columns: minmax(0, 1fr); }
		.rows .ctl { justify-self: start; }
		.pill { width: auto; }
	}
</style>
