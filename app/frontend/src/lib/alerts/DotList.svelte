<script lang="ts">
	// What lit the red dot: one short line each, the whole line a link to the fix (new alerts open the alerts panel).
	import { serverWords } from '#lib/core/i18n/server-words.ts';
	import { tn } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from '#lib/ui/Icon.svelte';
	import AlertSymbol from './AlertSymbol.svelte';
	import type { DotReason } from './dot-reasons.ts';

	let { reasons, link, onpick, onalerts, lead = false, card = false }: {
		reasons: DotReason[];
		link: (href: string) => string;
		onpick?: () => void;
		onalerts?: () => void;
		/** Lights the first line, the one to do first (the popover). */
		lead?: boolean;
		/** A grey card (the phone's Manage sheet). */
		card?: boolean;
	} = $props();
</script>

<div class="ui-dotlist" class:card data-testid="dot-list">
	<h2 class="head" data-testid="dot-count">{tn('alerts.dot.count', reasons.length)}</h2>
	<ul class:lead>
		{#each reasons as r (r.key)}
			<li>
				{#if r.href}
					<a class="row" href={link(r.href)} onclick={onpick}>
						<AlertSymbol tone={r.tone} size={20} />
						<span class="words"><span class="t">{serverWords(r.title)}</span>{#if r.body}<span class="b">{serverWords(r.body)}</span>{/if}</span>
						<Icon name="chevronRight" size={14} />
					</a>
				{:else}
					<button class="row" type="button" onclick={() => { onpick?.(); onalerts?.(); }}>
						<AlertSymbol tone={r.tone} size={20} />
						<span class="words"><span class="t">{r.title}</span></span>
						<Icon name="chevronRight" size={14} />
					</button>
				{/if}
			</li>
		{/each}
	</ul>
</div>

<style>
	.head { padding: 4px 8px 6px; color: var(--text-muted); font: 600 12px/1.3 var(--font-ui); }
	ul { display: flex; flex-direction: column; gap: 2px; }
	.row { width: 100%; min-height: 44px; display: flex; align-items: center; gap: 10px; padding: 6px 8px; border-radius: 9px; color: var(--text); text-align: start; }
	.row:hover, .lead li:first-child .row { background: var(--surface-3); }
	.card { padding: 6px; border-radius: var(--r-card); background: var(--surface-2); }
	.row:focus-visible { outline: none; box-shadow: inset 0 0 0 2px var(--accent); }
	.row :global(.icon) { flex: none; color: var(--text-muted); }
	.words { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
	.t { font: var(--t-callout); font-weight: 600; overflow-wrap: anywhere; }
	.b { color: var(--text-muted); font: var(--t-footnote); overflow-wrap: anywhere; }
</style>
