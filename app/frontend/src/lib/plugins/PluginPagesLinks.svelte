<script lang="ts">
	// A running plugin's own pages, opened in a sandboxed frame.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { to } from '#lib/core/paths.ts';
	import { pluginPages, type PluginPageEntry } from '#lib/ui/plugin-cards.ts';

	let { id }: { id: string } = $props();
	let pages = $state<PluginPageEntry[]>([]);
	$effect(() => {
		const want = id;
		void pluginPages().then((ps) => (pages = ps.filter((p) => p.plugin === want)));
	});
</script>

{#if pages.length}
	<div class="pages" data-testid="plugin-pages">
		<h3>{t('plugins.ownPages')}</h3>
		<div class="row">
			{#each pages as p (p.id)}
				<Button variant="secondary" icon="arrowUpRight" href={to(`/plugins/${encodeURIComponent(p.plugin)}/${encodeURIComponent(p.id)}`)}>{p.title}</Button>
			{/each}
		</div>
	</div>
{/if}

<style>
	.pages { display: flex; flex-direction: column; gap: 8px; margin-top: var(--s-3); }
	.row { display: flex; flex-wrap: wrap; gap: 8px; }
	/* Same small section label as About and Can access. */
	h3 { margin: 0; font-size: 12px; font-weight: 600; color: var(--text-muted); letter-spacing: normal; }
</style>
