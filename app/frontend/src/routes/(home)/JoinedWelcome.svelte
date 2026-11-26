<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';
	import { views } from '#lib/core/views.svelte.ts';
	import { limitLines, type AskLimit } from '#lib/requests/index.ts';
	import type { AskKind } from './joined-welcome.ts';

	// The first-visit card for a person who joined: how to find and ask, what they can ask for and their weekly limit. Got it hides it on every device.
	let { name, kinds }: { name: string; kinds: AskKind[] } = $props();

	let limit = $state<AskLimit | null>(null);

	const canAsk = $derived(kinds.length > 0);
	const limits = $derived(limitLines(limit, kinds));

	$effect(() => {
		if (canAsk) void api.get<AskLimit>('/asks/limit').then((l) => (limit = l)).catch(() => {});
	});

	const kindWord = (k: AskKind) => t(`library.stats.kind.${k}`);
</script>

<section class="joined" aria-labelledby="h-joined" data-testid="joined-welcome">
	<h2 id="h-joined">{name ? t('home.joined.title', { name }) : t('home.welcomeToHolyarrs')}</h2>
	<ul>
		<li><span class="ic"><Icon name="search" size={18} /></span><span>{t('home.joined.find', { explore: t('frame.nav.explore'), search: t('frame.layout.search') })}</span></li>
		{#if canAsk}
			<li>
				<span class="ic"><Icon name="plus" size={18} /></span>
				<span class="col">
					<span>{t('home.joined.ask', { request: t('discover.titlePage.request') })}</span>
					<span class="kinds">{#each kinds as k (k)}<span class="kind">{kindWord(k)}</span>{/each}</span>
				</span>
			</li>
			<li><span class="ic"><Icon name="requests" size={18} /></span><span>{t('home.joined.track', { requests: t('frame.nav.requests') })}</span></li>
			{#if limit}
				<li data-testid="joined-limits">
					<span class="ic"><Icon name="clock" size={18} /></span>
					<span class="col">
						{#each limits as l (l.kind)}<span>{l.text}</span>{:else}<span>{t('home.joined.noLimit')}</span>{/each}
					</span>
				</li>
			{/if}
		{/if}
	</ul>
	<div class="acts">
		<Button variant="primary" onclick={() => views.set('welcome.seen', '1')}>{t('home.joined.gotIt')}</Button>
		<Button variant="secondary" icon="compass" href={to('/explore')}>{t('home.joined.explore', { explore: t('frame.nav.explore') })}</Button>
	</div>
</section>

<style>
	.joined { display: flex; flex-direction: column; gap: 14px; padding: clamp(16px, 2.4vw, 22px); border-radius: 20px; background: linear-gradient(180deg, var(--sheet-a), var(--sheet-b)); box-shadow: var(--edge), var(--drop); }
	h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); overflow-wrap: anywhere; }
	ul { list-style: none; margin: 0; padding: 0; display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px 20px; }
	li { display: flex; align-items: flex-start; gap: 10px; min-width: 0; font: var(--t-subhead); color: var(--text); }
	.ic { flex: none; width: 32px; height: 32px; display: grid; place-items: center; border-radius: 50%; background: var(--well); color: var(--text-muted); }
	li > span:last-child { padding-top: 6px; min-width: 0; overflow-wrap: anywhere; }
	.col { display: flex; flex-direction: column; gap: 6px; }
	.kinds { display: flex; flex-wrap: wrap; gap: 6px; }
	.kind { padding: 2px 10px; border-radius: 999px; background: var(--well); color: var(--text-muted); font: var(--t-footnote); font-weight: 600; }
	.acts { display: flex; flex-wrap: wrap; gap: 8px; }
	@container home (max-width: 560px) {
		ul { grid-template-columns: minmax(0, 1fr); }
		.acts :global(a), .acts :global(button) { flex: 1 0 auto; }
	}
	@media (forced-colors: active) { .joined { border: 1px solid CanvasText; } }
</style>
