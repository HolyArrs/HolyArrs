<script lang="ts">
	// The network lock card on the Plugins page: one choice for the whole install, shown until answered and again after Change.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { netLockNotice } from './view.ts';
	import type { PluginOverview } from './types.ts';

	let { ov, reload }: { ov: PluginOverview; reload: () => Promise<void> } = $props();
	let busy = $state(false);
	let ask = $state(false);
	const asking = $derived(ask || !ov?.no_net_lock_asked);

	// One choice for the whole install: plugins that use the internet may start without the network lock; UDP stays blocked.
	async function setNoNet(allow: boolean) {
		if (busy) return;
		busy = true;
		try {
			await api.put('/plugins/net-lock', { allow_without: allow });
			toasts.push(t(allow ? 'plugins.noNetAllowed' : 'plugins.noNetKeptOff'), 'ok');
			ask = false;
		} catch (e) {
			toasts.push(t('plugins.noNetFailed'), 'fail', { body: message(e) });
		} finally {
			await reload();
			busy = false;
		}
	}
</script>

<section class="lockcard" data-testid="plugins-nonet" aria-labelledby="nonet-h">
	<h2 id="nonet-h" class="lockh"><Icon name="shield" size={18} />{t('plugins.noNetCardTitle')}</h2>
	{#if asking}
		<p class="small body">{netLockNotice(ov.net_lock_reason)}</p>
		<p class="small muted body">{t('plugins.noNetRisk')}</p>
		<div class="acts">
			<Button variant="primary" disabled={busy} onclick={() => (ov?.no_net_lock_asked && !ov.allow_no_net_lock ? (ask = false) : setNoNet(false))} data-testid="plugins-nonet-keep">{t('plugins.noNetKeepOff')}</Button>
			<Button variant="plain" busy={busy} disabled={busy} onclick={() => setNoNet(true)} data-testid="plugins-nonet-allow">{t('plugins.noNetAllow')}</Button>
		</div>
	{:else}
		<div class="lockrow">
			<p class="small body">{ov.allow_no_net_lock ? t('plugins.noNetOnNow') : t('plugins.noNetOffNow')}</p>
			<Button variant="plain" onclick={() => (ask = true)} data-testid="plugins-nonet-change">{t('plugins.noLockChange')}</Button>
		</div>
	{/if}
</section>

<style>
	.lockcard { grid-column: 1 / -1; display: grid; gap: var(--s-2); padding: var(--s-4); border-radius: 12px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.lockh { display: flex; align-items: center; gap: var(--s-2); margin: 0; font-size: 15px; font-weight: 600; letter-spacing: normal; }
	.lockh :global(svg) { color: var(--wait); flex: none; }
	.acts { display: flex; flex-wrap: wrap; gap: var(--s-2); }
	.lockrow { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--s-2); }
</style>
