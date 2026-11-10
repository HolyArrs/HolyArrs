<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Detail actions for a history row: "Find another" for rows about a release (blocks it, then searches its Slot again), then links to the Import plan and the title.
	import Button from '#lib/ui/Button.svelte';
	import { titleHref } from './history-kinds.ts';
	import { canFind, findTarget, qualityOf, rowLinks, rowStep } from './history-search.ts';
	import type { HistoryRow } from './history.ts';
	import { tick } from 'svelte';
	import { toasts } from '../core/toasts.svelte.ts';
	import { api, message } from '../core/api.ts';
	import { to } from '../core/paths.ts';
	import type { TitleView } from '../core/types.ts';

	let { row, titleById }: { row: HistoryRow; titleById: Map<number, TitleView> } = $props();
	const find = $derived(canFind(row));
	// On a failed download this finds another; on one that came in it replaces it.
	const findWord = $derived(rowStep(row).failed ? t('downloads.ha.findAnother') : t('downloads.ha.replace'));
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
		const target = findTarget(row);
		if (!target) {
			toasts.push(t('downloads.ha.notFound'), 'fail');
			return;
		}
		busy = true;
		try {
			const slot = target.slotId > 0 ? { slot_id: target.slotId } : undefined;
			await api.post(`/releases/${target.releaseId}/turn-down`, slot);
			await api.post(`/titles/${target.titleId}/search`, slot);
			toasts.push(t('downloads.ha.turnedDown'), 'ok', { body: t('downloads.ha.searchingAgain') });
			await stop();
		} catch (e) {
			toasts.push(t('downloads.ha.turnDownFailed'), 'fail', { body: message(e) });
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
		<div class="ask" role="group" aria-label={t('downloads.ha.findAnotherLabel')} bind:this={box} onkeydown={(ev) => ev.key === 'Escape' && (ev.stopPropagation(), stop())}>
			<p>{t('downloads.ha.ask', { quality: qualityOf(row) || t('downloads.ha.sameQuality') })}</p>
			<div class="btns">
				<Button variant="primary" size="sm" data-testid="history-find-confirm" disabled={busy} busy={busy} onclick={confirm}>{t('downloads.ha.blockSearch')}</Button>
				<Button variant="plain" size="sm" data-testid="history-find-cancel" onclick={() => stop()}>{t('downloads.sel.cancel')}</Button>
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
