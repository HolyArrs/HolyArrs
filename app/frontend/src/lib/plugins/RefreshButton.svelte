<script lang="ts">
	// Fetches the signed plugin and language lists now, not at the next 6-hour check.
	import IconButton from '#lib/ui/IconButton.svelte';
	import { api } from '#lib/core/api.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { listLanguages } from '#lib/core/i18n/language-packs.svelte.ts';
	import type { PluginOverview } from './types.ts';

	let { onlist }: { onlist: (ov: PluginOverview) => void } = $props();
	let busy = $state(false);

	async function refresh() {
		if (busy) return;
		busy = true;
		try {
			const r = await api.post<{ changed: boolean; plugins: PluginOverview }>('/plugins/refresh');
			onlist(r.plugins);
			void listLanguages().catch(() => {});
			toasts.push(t(r.changed ? 'plugins.refreshed' : 'plugins.refreshSame'), r.changed ? 'ok' : 'info');
		} catch {
			toasts.push(t('plugins.refreshFailed'), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

<IconButton icon="refresh" label={t('plugins.refresh')} class="prefresh" disabled={busy} aria-busy={busy || undefined} onclick={refresh} />

<style>
	/* The arrow turns while the lists are fetched. */
	:global(.prefresh[aria-busy='true'] svg) { animation: pspin 0.9s linear infinite; }
	@keyframes pspin { to { transform: rotate(1turn); } }
	@media (prefers-reduced-motion: reduce) { :global(.prefresh[aria-busy='true'] svg) { animation: none; } }
</style>
