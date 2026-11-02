<script lang="ts">
	// A row's detail actions: "Find another" for rows about a release (turns
	// the release down, then searches its Slot again), then "Go to" links.
	import { tick } from 'svelte';
	import Button from '../ui/Button.svelte';
	import { toasts } from '../toasts.svelte.ts';
	import { api, message } from '../api.ts';
	import { to } from '../paths.ts';
	import { titleHref } from '../history-views.ts';
	import { canFind, findTarget, qualityOf, rowLinks } from './dirs.ts';
	import type { HistoryRow } from '../history.ts';
	import type { TitleView } from '../types.ts';

	let { row, titleById }: { row: HistoryRow; titleById: Map<number, TitleView> } = $props();
	const find = $derived(canFind(row));
	const links = $derived(rowLinks(row, titleHref(row.head.TitleID, titleById)));

	let asking = $state(false);
	let busy = $state(false);
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
	async function confirm() {
		const t = findTarget(row);
		if (!t) {
			toasts.push('HolyArrs no longer knows which release this was.', 'fail');
			return;
		}
		busy = true;
		try {
			const slot = t.slotId > 0 ? { slot_id: t.slotId } : undefined;
			await api.post(`/releases/${t.releaseId}/turn-down`, slot);
			await api.post(`/titles/${t.titleId}/search`, slot);
			toasts.push('Blocked. Searching for another release.', 'ok');
			await stop();
		} catch (e) {
			toasts.push(message(e), 'fail');
		} finally {
			busy = false;
		}
	}
	async function stop() {
		asking = false;
		await tick();
		opener?.querySelector<HTMLElement>('[data-testid="history-find"]')?.focus();
	}
</script>

<div class="da" data-testid="history-acts">
	{#if find}
		<div class="btns" bind:this={opener}>
			{#if !asking}<Button variant="secondary" icon="declined" data-testid="history-find" onclick={ask}>Find another</Button>{/if}
		</div>
		{#if asking}
			<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
			<div class="ask" role="group" aria-label="Find another release" bind:this={box} onkeydown={(ev) => ev.key === 'Escape' && (ev.stopPropagation(), stop())}>
				<p>Block this release and search again for {qualityOf(row) || 'the same quality'}?</p>
				<div class="btns">
					<Button variant="primary" data-testid="history-find-confirm" disabled={busy} busy={busy} onclick={confirm}>Confirm</Button>
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
