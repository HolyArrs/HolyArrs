<script lang="ts">
	import { beforeNavigate, goto } from '$app/navigation';
	import Button from './Button.svelte';
	import Sheet from './Sheet.svelte';

	// The bar that appears at the bottom of a settings page once something
	// changed: "Unsaved changes", Discard and Save. Save submits the page's own
	// form (by id), so its validation and save order stay the form's. Leaving
	// the page with changes asks first.
	let {
		dirty,
		form,
		saving = false,
		disabled = false,
		ondiscard,
		label = 'Save'
	}: { dirty: boolean; form: string; saving?: boolean; disabled?: boolean; ondiscard: () => void; label?: string } = $props();

	let asking = $state(false);
	let target = $state<URL | null>(null);
	let leaving = false;

	beforeNavigate((nav) => {
		if (!dirty || leaving) return;
		if (nav.type === 'leave') {
			// Closing or reloading the tab: the browser asks in its own words.
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
	<div class="savebar" role="region" aria-label="Unsaved changes" data-testid="savebar">
		<span class="words"><i aria-hidden="true"></i>Unsaved changes</span>
		<div class="acts">
			<Button variant="plain" type="button" onclick={ondiscard} disabled={saving}>Discard</Button>
			<Button variant="primary" type="submit" {form} disabled={disabled || saving} busy={saving}>{label}</Button>
		</div>
	</div>
{/if}

<Sheet bind:open={asking} title="Leave without saving?" description="Your changes on this page are not saved yet." size="sm">
	<p class="small muted">Stay to save them, or leave and lose them.</p>
	{#snippet footer()}
		<Button variant="plain" type="button" onclick={() => (asking = false)}>Stay</Button>
		<Button variant="destructive" type="button" onclick={leave}>Leave without saving</Button>
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
	.acts { display: flex; gap: 8px; margin-left: auto; }
	@keyframes rise { from { opacity: 0; transform: translateY(12px); } }
	@media (max-width: 767px) {
		.savebar { bottom: calc(var(--tabbar, 60px) + env(safe-area-inset-bottom, 0px) + 8px); }
		.acts :global(.ui-btn) { min-height: 44px; }
	}
	@media (prefers-reduced-motion: reduce) { .savebar { animation: none; } }
</style>
