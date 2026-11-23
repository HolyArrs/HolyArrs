<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import type { ComponentProps } from 'svelte';
	import DoneStep from './DoneStep.svelte';
	import { running as isRunning, summary } from './move.ts';

	// First-run setup only: a short summary that fits the setup card, with the full Done step folded away.
	let props: ComponentProps<typeof DoneStep> = $props();
	let open = $state(false);

	const rep = $derived(props.move.report);
	const failed = $derived(rep?.counts?.failed ?? 0);
	const undo = $derived(!!rep?.undo);
	const busy = $derived(isRunning(props.move));
</script>

{#if busy || !rep}
	<DoneStep {...props} />
{:else}
	<div class="sd" id="move-step" tabindex="-1" aria-label={t('migrate.done')} data-testid="setup-done">
		<ul class="sd-list">
			{#each props.groups as g (g.group)}
				<li><b>{g.label}</b><span>{summary(g.counts, true, undo)}</span></li>
			{/each}
		</ul>
		{#if failed > 0}
			<p class="sd-fail" role="status">{failed === 1 ? t('migrate.itemFailed', { n: failed }) : t('migrate.itemsFailed', { n: failed })} {t('migrate.matchLater')}</p>
		{/if}
		<button type="button" class="sd-toggle" aria-expanded={open} aria-controls="setup-done-more" onclick={() => (open = !open)}>{open ? t('migrate.hideDetails') : t('migrate.showDetails')}</button>
		{#if open}<div class="sd-more" id="setup-done-more"><DoneStep {...props} /></div>{/if}
	</div>
{/if}

<style>
	.sd:focus { outline: none; }
	.sd { display: flex; flex-direction: column; gap: 10px; min-height: 0; }
	.sd-list { list-style: none; margin: 0; padding: 0 16px; border-radius: 16px; background: var(--surface-2); border: 1px solid var(--line); }
	.sd-list li { display: flex; flex-wrap: wrap; align-items: baseline; gap: 10px; min-height: 40px; padding: 8px 0; border-top: 1px solid var(--line); font: var(--t-callout); color: var(--text); min-width: 0; }
	.sd-list li:first-child { border-top: 0; }
	.sd-list b { font-weight: 600; flex: none; }
	.sd-list span { color: var(--text-muted); font: var(--t-footnote); min-width: 0; overflow-wrap: anywhere; }
	.sd-fail { margin: 0; font: var(--t-footnote); color: var(--fail-text); }
	.sd-toggle { align-self: center; border: 0; padding: 0 18px; min-height: 44px; border-radius: var(--r-sm); background: none; color: var(--text-muted); font: 500 15px/1.2 var(--font-ui); cursor: pointer; }
	.sd-toggle:hover { color: var(--text); background: color-mix(in srgb, var(--text) 6%, transparent); }
	.sd-toggle:focus-visible { outline: none; box-shadow: var(--focus-ring); border-radius: 6px; }
	.sd-more { max-height: 240px; overflow: auto; overscroll-behavior: contain; display: flex; flex-direction: column; gap: 12px; }
</style>
