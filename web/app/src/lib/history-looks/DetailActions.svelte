<script lang="ts">
	// A row's detail actions, in one row: "Find another" for rows about a
	// release (blocks the release, then searches its Slot again), then the
	// places to go (Import plan, the title, Downloads, Requests, Problems).
	import { tick } from 'svelte';
	import Button from '../ui/Button.svelte';
	import { toasts } from '../toasts.svelte.ts';
	import { api, message } from '../api.ts';
	import { to } from '../paths.ts';
	import { titleHref } from '../history-views.ts';
	import { canFind, findTarget, qualityOf, rowLinks, rowStep } from './dirs.ts';
	import type { HistoryRow } from '../history.ts';
	import type { TitleView } from '../types.ts';

	let { row, titleById }: { row: HistoryRow; titleById: Map<number, TitleView> } = $props();
	const find = $derived(canFind(row));
	// On a download that failed it finds another; on one that came in it replaces it.
	const findWord = $derived(rowStep(row).failed ? 'Find another' : 'Replace');
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
			toasts.push('Release not found', 'fail');
			return;
		}
		busy = true;
		try {
			const slot = t.slotId > 0 ? { slot_id: t.slotId } : undefined;
			await api.post(`/releases/${t.releaseId}/turn-down`, slot);
			await api.post(`/titles/${t.titleId}/search`, slot);
			toasts.push('Release turned down', 'ok', { body: 'Searching again' });
			await stop();
		} catch (e) {
			toasts.push("Couldn't turn down Release", 'fail', { body: message(e) });
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

{#if find || links.length}
<div class="da" data-testid="history-acts" bind:this={opener}>
	{#if find && asking}
		<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
		<div class="ask" role="group" aria-label="Find another release" bind:this={box} onkeydown={(ev) => ev.key === 'Escape' && (ev.stopPropagation(), stop())}>
			<p>Block this release and search again for {qualityOf(row) || 'the same quality'}?</p>
			<div class="btns">
				<Button variant="primary" size="sm" data-testid="history-find-confirm" disabled={busy} busy={busy} onclick={confirm}>Block and search</Button>
				<Button variant="plain" size="sm" data-testid="history-find-cancel" onclick={() => stop()}>Cancel</Button>
			</div>
		</div>
	{:else if find}
		<Button variant="secondary" size="sm" icon="refresh" data-testid="history-find" onclick={ask}>{findWord}</Button>
	{/if}
	{#each links as l (l.key)}<Button variant="secondary" size="sm" href={to(l.href)} iconAfter="chevronRight" data-testid="history-go-{l.key}">{l.label}</Button>{/each}
</div>
{/if}

<style>
	.da { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; width: 100%; min-width: 0; }
	.da > :global(.ui-btn) { flex: 1 1 auto; border-radius: var(--r-pill); max-width: 100%; }
	.da > :global(.ui-btn span) { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
	.btns { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
	.ask { flex: 1 0 100%; display: flex; flex-direction: column; gap: 10px; padding: 12px; border-radius: var(--r-card); background: var(--well, var(--surface-2)); box-shadow: inset 0 0 0 1px var(--line); }
	.ask p { margin: 0; font: var(--t-subhead); color: var(--text); overflow-wrap: anywhere; }
	.ask :global(.ui-btn) { border-radius: var(--r-pill); }
	@media (pointer: coarse) { .da :global(.ui-btn) { min-height: 44px; } }
</style>
