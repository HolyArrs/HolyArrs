<script lang="ts">
	import SegmentedControl from './ui/SegmentedControl.svelte';
	import { setTitleLook, titleLook, TITLE_LOOKS, type TitleLook } from './title-look.svelte.ts';

	// A floating switch between the title page layouts being compared.
	let value = $state<TitleLook>(titleLook.value);
	$effect(() => {
		if (value !== titleLook.value) setTitleLook(value);
	});
</script>

<div class="look" data-testid="title-look">
	<SegmentedControl options={TITLE_LOOKS} bind:value label="Title page layout" size="sm" announce={(l) => `Showing layout ${l}`} />
</div>

<style>
	.look { position: fixed; z-index: 30; left: 0; right: 0; bottom: 20px; width: max-content; max-width: calc(100vw - 24px); margin-inline: auto; padding: 5px; border-radius: 999px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); border: 1px solid var(--line); box-shadow: var(--shadow-lift); backdrop-filter: blur(20px) saturate(160%); -webkit-backdrop-filter: blur(20px) saturate(160%); animation: rise 420ms cubic-bezier(0.34, 1.56, 0.64, 1) both; }
	@media (max-width: 767px) { .look { bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom) + 16px); } }
	@keyframes rise { from { opacity: 0; transform: translateY(16px); } }
	@media (prefers-reduced-motion: reduce) { .look { animation: none; } }
</style>
