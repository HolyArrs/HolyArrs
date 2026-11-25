<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import { api } from '#lib/core/api.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	// A Test found one of the Downloader's categories missing: "Create it" makes it there, or says how HolyArrs may.
	let { id, name, app, canMake, makeOff, onmade }: { id: number; name: string; app: string; canMake: boolean; makeOff: boolean; onmade: () => void } = $props();
	let busy = $state(false);

	async function make() {
		busy = true;
		try {
			await api.post(`/downloaders/${id}/category`, { name });
			toasts.push(t('settings.downloaders.madeCategory', { name, app }), 'ok');
			onmade();
		} catch (err) {
			toasts.push(message(err), 'fail');
		} finally {
			busy = false;
		}
	}
</script>

{#if canMake}
	<p class="note"><Button variant="secondary" size="sm" type="button" disabled={busy} aria-busy={busy} onclick={make} data-testid="downloader-make-category">{t('settings.downloaders.makeCategory', { name })}</Button></p>
{:else if makeOff}
	<p class="note" data-testid="downloader-make-category-off">{t('settings.downloaders.makeCategoryOff')}</p>
{/if}

<style>
	.note { margin: 8px 0 0 40px; font: 400 13px/1.35 var(--font-ui); color: var(--text-muted); }
</style>
