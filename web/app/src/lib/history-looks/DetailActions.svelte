<script lang="ts">
	// The detail actions of the search looks, in three versions to compare
	// (?acts=find|retry|view): buttons for the row, then "Go to" links.
	// Preview only: the buttons say they are coming soon.
	import { tick } from 'svelte';
	import { page } from '$app/state';
	import Button from '../ui/Button.svelte';
	import { toasts } from '../toasts.svelte.ts';
	import { to } from '../paths.ts';
	import { titleHref } from '../history-views.ts';
	import { actsOf, qualityOf, rowActs, rowLinks } from './dirs.ts';
	import type { HistoryRow } from '../history.ts';
	import type { TitleView } from '../types.ts';

	let { row, titleById }: { row: HistoryRow; titleById: Map<number, TitleView> } = $props();
	const acts = $derived(rowActs(row, actsOf(page.url.searchParams.get('acts'))));
	const links = $derived(rowLinks(row, titleHref(row.head.TitleID, titleById)));
	const soon = (what: string) => toasts.push(`${what}: coming soon.`, 'info');

	let asking = $state(false);
	let box = $state<HTMLDivElement>();
	let opener = $state<HTMLDivElement>();
	$effect(() => {
		void row.key;
		asking = false;
	});
	async function ask() {
		asking = true;
		await tick();
		box?.querySelector<HTMLButtonElement>('[data-testid="history-find-confirm"]')?.focus();
	}
	async function stop(done = false) {
		asking = false;
		if (done) soon('Find another');
		await tick();
		opener?.querySelector<HTMLElement>('[data-testid="history-find"]')?.focus();
	}
</script>

<div class="da" data-testid="history-acts">
	{#if acts.length}
		<div class="btns" bind:this={opener}>
			{#if acts.includes('retry')}<Button variant="secondary" icon="refresh" data-testid="history-retry" onclick={() => soon('Retry')}>Retry</Button>{/if}
			{#if acts.includes('find') && !asking}<Button variant="secondary" icon="declined" data-testid="history-find" onclick={ask}>Find another</Button>{/if}
		</div>
		{#if asking}
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<div class="ask" role="group" aria-label="Find another release" bind:this={box} onkeydown={(ev) => ev.key === 'Escape' && (ev.stopPropagation(), stop())}>
				<p>Block this release and search again for {qualityOf(row) || 'the same quality'}?</p>
				<div class="btns">
					<Button variant="primary" data-testid="history-find-confirm" onclick={() => stop(true)}>Confirm</Button>
					<Button variant="plain" data-testid="history-find-cancel" onclick={() => stop()}>Cancel</Button>
				</div>
			</div>
		{/if}
	{/if}
	<nav class="go" aria-label="Go to">
		<span class="gl">Go to</span>
		{#each links as l (l.key)}<Button variant="plain" href={to(l.href)} iconAfter="chevronRight" data-testid="history-go-{l.key}">{l.label}</Button>{/each}
	</nav>
</div>

<style>
	.da { display: flex; flex-direction: column; gap: 10px; width: 100%; min-width: 0; }
	.btns, .go { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
	.ask { display: flex; flex-direction: column; gap: 8px; padding: 12px; border-radius: var(--r-card); background: var(--surface-2); outline: 1px solid var(--line); }
	.ask p { margin: 0; font: var(--t-callout); color: var(--text); overflow-wrap: anywhere; }
	.gl { color: var(--text-muted); font: var(--t-footnote); }
	@media (pointer: coarse) { .da :global(.ui-btn) { min-height: 44px; } }
</style>
