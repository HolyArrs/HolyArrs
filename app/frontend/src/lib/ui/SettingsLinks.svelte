<!-- Boxed list of links to related settings pages, with optional status chips. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Icon from './Icon.svelte';
	import { to } from '../core/paths.ts';

	let { links, label = t('ui.settingsLinks.relatedPages') }: { links: { href: string; label: string; note?: string; tone?: string }[]; label?: string } = $props();
</script>

<ul class="settings-links" aria-label={label} data-testid="settings-links">
	{#each links as l (l.href)}
		<li>
			<a href={to(l.href)}>
				<span class="copy"><b>{l.label}</b>{#if l.note}<span class="note">{l.note}</span>{/if}</span>
				{#if l.tone}<span class="chip {l.tone}">{l.tone === 'ok' ? t('ui.settingsLinks.ok') : l.tone === 'wait' ? t('ui.settingsLinks.check') : t('ui.settingsLinks.problem')}</span>{/if}
				<Icon name="chevronRight" size={16} />
			</a>
		</li>
	{/each}
</ul>

<style>
	.settings-links { margin: 16px 0 0; padding: 4px; list-style: none; border-radius: var(--r-panel); background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	li + li { border-top: 1px solid var(--line); }
	a { display: flex; align-items: center; gap: 12px; min-height: 48px; padding: 6px 12px; border-radius: var(--r-btn); color: var(--text); }
	a:hover { background: var(--surface-2); }
	.copy { flex: 1; min-width: 0; }
	.copy b { display: block; font: var(--t-headline); font-size: 15px; }
	.note { display: block; color: var(--text-muted); font: var(--t-subhead); font-weight: 400; overflow-wrap: anywhere; }
	/* Chevron keeps its size and reads as secondary. */
	a > :global(.icon) { flex: none; color: var(--text-muted); }
</style>
