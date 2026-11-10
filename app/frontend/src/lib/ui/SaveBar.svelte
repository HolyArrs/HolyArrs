<!-- Sticky bar shown when a form has unsaved changes, with a guard before leaving. -->
<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { beforeNavigate, goto } from '$app/navigation';
	import Button from './Button.svelte';
	import Sheet from './Sheet.svelte';

	let {
		dirty,
		form,
		saving = false,
		disabled = false,
		ondiscard,
		label = t('ui.saveBar.save')
	}: { dirty: boolean; form: string; saving?: boolean; disabled?: boolean; ondiscard: () => void; label?: string } = $props();

	let asking = $state(false);
	let target = $state<URL | null>(null);
	// Plain flag so our own goto is not blocked by the guard below.
	let leaving = false;

	beforeNavigate((nav) => {
		if (!dirty || leaving || saving) return;
		// Closing the tab cannot be paused, so ask the browser to stop it.
		if (nav.type === 'leave') {
			nav.cancel();
			return;
		}
		if (!nav.to) return;
		nav.cancel();
		target = nav.to.url;
		asking = true;
	});

	async function leave() {
		const url = target;
		asking = false;
		if (!url) return;
		leaving = true;
		try {
			await goto(url);
		} finally {
			leaving = false;
		}
	}
</script>

{#if dirty}
	<div class="savebar" role="region" aria-label={t('ui.saveBar.unsavedChanges')} data-testid="savebar">
		<span class="words"><i aria-hidden="true"></i>{t('ui.saveBar.unsavedChanges')}</span>
		<div class="acts">
			<Button variant="plain" type="button" onclick={ondiscard} disabled={saving}>{t('ui.saveBar.discard')}</Button>
			<Button variant="primary" type="submit" {form} disabled={disabled || saving} busy={saving}>{label}</Button>
		</div>
	</div>
{/if}

<Sheet bind:open={asking} title={t('ui.saveBar.leaveWithoutSaving')} description={t('ui.saveBar.yourChangesOnThis')} size="sm">
	<p class="small muted">{t('ui.saveBar.stayToSaveThem')}</p>
	{#snippet footer()}
		<Button variant="plain" type="button" onclick={() => (asking = false)}>{t('ui.saveBar.stay')}</Button>
		<Button variant="destructive" type="button" onclick={leave}>{t('ui.saveBar.leaveWithoutSaving2')}</Button>
	{/snippet}
</Sheet>

<style>
	.savebar {
		position: sticky;
		bottom: 16px;
		z-index: 20;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		flex-wrap: wrap;
		margin-top: 20px;
		padding: 10px 12px 10px 18px;
		border-radius: var(--r-panel);
		background: var(--surface-1);
		box-shadow: 0 0 0 1px var(--line-strong), var(--shadow-lift);
		animation: rise var(--dur) var(--ease-out);
	}
	.words { display: inline-flex; align-items: center; gap: 8px; font: var(--t-callout); font-weight: 600; }
	i { width: 8px; height: 8px; border-radius: 50%; background: var(--wait); }
	.acts { display: flex; gap: 8px; margin-inline-start: auto; }
	@keyframes rise { from { opacity: 0; transform: translateY(12px); } }
	/* Sit above the mobile tab bar. */
	@media (max-width: 767px) {
		.savebar { bottom: calc(var(--tabbar, 60px) + env(safe-area-inset-bottom, 0px) + 8px); }
		.acts :global(.ui-btn) { min-height: 44px; }
	}
	@media (prefers-reduced-motion: reduce) { .savebar { animation: none; } }
</style>
