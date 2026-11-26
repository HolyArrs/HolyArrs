<script lang="ts">
	// "From a plugin" on Download clients and Sources: installed plugins that serve as a download client
	// (sources.downloader) or a search source (sources.releases), each added with one tap.
	import { onMount } from 'svelte';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { fromPluginBody, servesAs, type FromPluginKind } from './from-plugin.ts';
	import type { Plugin, PluginOverview } from './types.ts';

	let { kind, taken = [], onadded }: { kind: FromPluginKind; taken?: string[]; onadded: () => void } = $props();
	let plugins = $state<Plugin[]>([]);
	let busy = $state('');

	onMount(async () => {
		const ov = await api.get<PluginOverview>('/plugins').catch(() => null);
		plugins = (ov?.plugins ?? []).filter((p) => servesAs(p, kind));
	});
	const shown = $derived(plugins.filter((p) => !taken.includes(`plugin://${p.id}`)));

	async function add(p: Plugin) {
		busy = p.id;
		try {
			await api.post(kind === 'downloader' ? '/downloaders' : '/sources', fromPluginBody({ id: p.id, name: p.name ?? p.id, download_types: p.download_types }, kind));
			onadded();
		} catch (e) {
			toasts.push(t('plugins.fromPluginFailed'), 'fail', { body: message(e) });
		} finally {
			busy = '';
		}
	}
</script>

{#if shown.length}
	<section class="from" data-testid="from-plugin-{kind}">
		<h2 class="small">{t('plugins.fromPlugin')}</h2>
		<div class="row">
			{#each shown as p (p.id)}
				<Button variant="secondary" icon="plug" disabled={!!busy} busy={busy === p.id} onclick={() => add(p)}>{t('plugins.addFrom', { name: p.name ?? p.id })}</Button>
			{/each}
		</div>
	</section>
{/if}

<style>
	.from { display: flex; flex-direction: column; gap: 8px; margin: 0 0 var(--s-4); }
	.row { display: flex; flex-wrap: wrap; gap: 8px; }
	h2 { font-weight: 600; }
</style>
