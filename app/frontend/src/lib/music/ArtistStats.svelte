<script lang="ts">
	import Words from '#lib/ui/Words.svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { aboutSize, changeWords, shortCount, listening } from './music-pages.ts';

	// Where concerts would be: the listening figures and the ask-for-all-missing button.
	let { listen, missing, mayAsk, manage, titleId, allAsked, confirmAll = $bindable(), askingAll, onask }: {
		listen: ReturnType<typeof listening>;
		missing: { count: number; size_bytes: number; albums: string[] } | undefined;
		mayAsk: boolean;
		manage: boolean;
		titleId: number;
		allAsked: boolean;
		confirmAll: boolean;
		askingAll: boolean;
		onask: () => void;
	} = $props();
</script>

{#if (listen || (missing?.count && mayAsk && !manage && !titleId && !allAsked))}
	{@const up = (listen?.today ?? 0) > 0}
	<section class="stats" data-testid="artist-stats" aria-label={t('music.listeningAndReleases')}>
		{#if listen}
			<p class="st1">
				<span><b>{shortCount(listen.n)}</b> {listen.unit}</span>
				{#if listen.rank}<span><Words key="music.rankInWorld" vars={{ rank: listen.rank }} /></span>{/if}
				{#if listen.today}<span class="chg" class:up class:down={!up}>{changeWords(listen.today)}</span>{/if}
			</p>
		{/if}
		{#if missing?.count && mayAsk && !manage && !titleId && !allAsked}
			<div class="st-ask">
				{#if confirmAll}
					<span class="confirm" role="group" aria-label={t('music.confirmAskingForAll')}>
						<span>{missing.count === 1 ? t('music.askForRelease', { n: missing.count, size: aboutSize(missing.size_bytes) }) : t('music.askForReleases', { n: missing.count, size: aboutSize(missing.size_bytes) })}</span>
						<Button variant="primary" onclick={onask} disabled={askingAll}>{t('music.ask')}</Button>
						<Button onclick={() => (confirmAll = false)}>{t('music.cancel')}</Button>
					</span>
				{:else}
					<Button icon="plus" onclick={() => (confirmAll = true)} data-testid="ask-all-missing">{t('music.askForAllMissing', { n: missing.count })}</Button>
				{/if}
			</div>
		{/if}
	</section>
{/if}

<style>
	.chg { font-style: normal; }
	.stats { display: flex; flex-direction: column; align-items: center; gap: 10px; padding: 28px 16px; border-block: 1px solid var(--line); text-align: center; font-variant-numeric: tabular-nums; }
	.stats p { display: flex; flex-wrap: wrap; justify-content: center; align-items: baseline; margin: 0; }
	.stats p > span { display: inline-flex; align-items: center; gap: 6px; white-space: nowrap; }
	.stats p > span + span::before { content: '·'; margin: 0 14px; color: var(--text-faint); }
	.st1 { font: 500 20px/1.35 var(--font-ui); color: var(--text-muted); }
	.st1 :global(b) { color: var(--text); font-weight: 700; }
	.chg.up { color: var(--ok-text); font-weight: 600; }
	.chg.down { color: var(--fail-text); font-weight: 600; }
	/* On the artist colour, use light tones that read on any deep page colour. */
	:global(.artist.tinted) .chg.up { color: #9df5b4; }
	:global(.artist.tinted) .chg.down { color: #ffb3b3; }
	.st-ask { margin-top: 6px; }
	.confirm { display: inline-flex; align-items: center; gap: 10px; flex-wrap: wrap; justify-content: center; font: var(--t-subhead); }
	@media (max-width: 767px) {
		.stats { padding: 22px 8px; }
		.st1 { font-size: 17px; }
		.stats p > span + span::before { margin: 0 8px; }
	}
</style>
