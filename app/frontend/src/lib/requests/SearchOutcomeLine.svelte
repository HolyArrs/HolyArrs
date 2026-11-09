<script lang="ts">
	import { outcomeLine, whyWords, type SearchOutcome } from './search-outcome.ts';
	import { api } from '../core/api.ts';
	import { to } from '../core/paths.ts';

	// One line when a Title's newest search found nothing. Owners and admins see which Sources were paused or failed, each a link to that Source; everyone else sees "No results yet".
	let { titleId }: { titleId: number } = $props();
	let o = $state<SearchOutcome | null>(null);

	$effect(() => {
		const id = titleId;
		o = null;
		if (!id) return;
		let live = true;
		api.get<SearchOutcome>(`/titles/${id}/search-outcome`).then((r) => live && (o = r), () => {});
		return () => (live = false);
	});
	const line = $derived(outcomeLine(o));
</script>

{#if line}
	<p class="outcome" data-testid="search-outcome">
		{line}
		{#each o?.sources ?? [] as s, i (s.id)}
			<span>{i ? ', ' : ' '}<a href={to('/settings/sources/' + s.id)}>{s.name}</a> {whyWords(s.why)}</span>
		{/each}
	</p>
{/if}

<style>
	.outcome { margin: 0; font: 500 14px/1.35 var(--font-ui); color: var(--text-muted); overflow-wrap: anywhere; }
	a { color: inherit; text-decoration: underline; text-underline-offset: 3px; }
	a:focus-visible { box-shadow: var(--focus-ring); border-radius: 4px; }
</style>
