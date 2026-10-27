<script lang="ts">
	import ListRow from '../ui/ListRow.svelte';
	import StatusPill, { type StatusKind } from '../ui/StatusPill.svelte';
	import { ago, tone, words } from '../format.ts';
	import type { ImportRecord } from '../types.ts';

	// Imports, as on today's page: what HolyArrs moved into the Library.
	let { imports, compact = false }: { imports: ImportRecord[]; compact?: boolean } = $props();
	function toneKind(t: string): StatusKind {
		return t === 'ok' ? 'in_library' : t === 'fail' ? 'failed' : t === 'idle' ? 'turned_down' : 'missing';
	}
</script>

<section class="imp" class:compact aria-labelledby="h-imports">
	<header><h2 id="h-imports">Imports</h2><span class="count">{imports.length}</span></header>
	{#if imports.length === 0}
		<p class="panel empty">No imports yet. A finished download is moved into the Library here.</p>
	{:else}
		<ul class="panel">
			{#each imports as im (im.ID)}
				{@const placed = im.Result?.find((r) => r.outcome === 'placed')}
				<li>
					<ListRow icon="folder" title={placed?.source_file ?? im.Plan?.[0]?.source_file ?? `Import ${im.ID}`} subtitle={placed?.placed_by ? `Placed by ${placed.placed_by}` : undefined} meta={ago(im.CreatedAt)}>
						{#snippet trailing()}<StatusPill kind={toneKind(tone(im.State))} label={words(im.State)} />{/snippet}
					</ListRow>
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.imp { min-width: 0; }
	header { display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }
	h2 { font: var(--t-title3); letter-spacing: var(--t-title3-track); }
	.compact h2 { font: var(--t-headline); }
	.count { min-width: 24px; height: 24px; padding: 0 8px; display: inline-grid; place-items: center; border-radius: 999px; background: var(--surface-2); color: var(--text-muted); font: var(--t-subhead); font-weight: 600; font-variant-numeric: tabular-nums; }
	.panel { margin: 0; list-style: none; padding: 6px; border-radius: var(--r-panel); background: var(--surface-1); outline: 1px solid var(--line); box-shadow: var(--shadow-card); }
	.empty { padding: 16px; color: var(--text-muted); font: var(--t-callout); }
</style>
