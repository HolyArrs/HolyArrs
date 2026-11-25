<script lang="ts">
	// The file lock card on the Plugins page: one choice for the whole install, shown until answered and again after Change.
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Button from '#lib/ui/Button.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { plainMessage as message } from '#lib/core/settings.ts';
	import { toasts } from '#lib/core/toasts.svelte.ts';
	import { noLockNotice } from './view.ts';
	import type { PluginOverview } from './types.ts';

	let { ov, reload }: { ov: PluginOverview; reload: () => Promise<void> } = $props();
	let lockBusy = $state(false);
	let askLock = $state(false);
	const asking = $derived(askLock || !ov?.no_file_lock_asked);

	// One choice for the whole install: signed plugins may run without the file lock, with the other limits on.
	async function setNoLock(allow: boolean) {
		if (lockBusy) return;
		lockBusy = true;
		try {
			await api.put('/plugins/file-lock', { allow_without: allow });
			toasts.push(t(allow ? 'plugins.noLockAllowed' : 'plugins.noLockKeptOff'), 'ok');
			askLock = false;
		} catch (e) {
			toasts.push(t('plugins.noLockFailed'), 'fail', { body: message(e) });
		} finally {
			await reload();
			lockBusy = false;
		}
	}
</script>

<section class="lockcard" data-testid="plugins-nolock" aria-labelledby="nolock-h">
	<h2 id="nolock-h" class="lockh"><Icon name="shield" size={18} />{t('plugins.noLockCardTitle')}</h2>
	{#if asking}
		<p class="small body">{noLockNotice(ov.file_lock_reason)}</p>
		<p class="small muted body">{t('plugins.noLockRisk')}</p>
		<p class="small ask">{t('plugins.noLockAsk')}</p>
		<div class="acts">
			<Button variant="primary" busy={lockBusy} disabled={lockBusy} onclick={() => setNoLock(true)} data-testid="plugins-nolock-allow">{t('plugins.noLockAllow')}</Button>
			<Button variant="plain" disabled={lockBusy} onclick={() => (ov?.no_file_lock_asked && !ov.allow_no_file_lock ? (askLock = false) : setNoLock(false))} data-testid="plugins-nolock-keep">{t('plugins.noLockKeepOff')}</Button>
		</div>
	{:else}
		<div class="lockrow">
			<p class="small body">{ov.allow_no_file_lock ? t('plugins.noLockOnNow') : t('plugins.noLockOffNow')}</p>
			<Button variant="plain" onclick={() => (askLock = true)} data-testid="plugins-nolock-change">{t('plugins.noLockChange')}</Button>
		</div>
	{/if}
</section>

<style>
	.lockcard { grid-column: 1 / -1; display: grid; gap: var(--s-2); padding: var(--s-4); border-radius: 12px; background: var(--surface-1); box-shadow: 0 0 0 1px var(--line); }
	.lockh { display: flex; align-items: center; gap: var(--s-2); margin: 0; font-size: 15px; font-weight: 600; letter-spacing: normal; }
	.lockh :global(svg) { color: var(--wait); flex: none; }
	.ask { margin: 0; font-weight: var(--weight-semibold); }
	.lockrow { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--s-2); }
</style>
