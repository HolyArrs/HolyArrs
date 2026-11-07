<script lang="ts">
	import SegmentedControl from './ui/SegmentedControl.svelte';
	import { setTagsAt, setTitleLook, TAG_SPOTS, tagsAt, titleLook, TITLE_LOOKS, type TagsAt, type TitleLook } from './discover/index.ts';

	// A floating switch between the hero options being compared.
	let value = $state<TitleLook>(titleLook.value);
	$effect(() => {
		if (value !== titleLook.value) setTitleLook(value);
	});
	let tags = $state<TagsAt>(tagsAt.value);
	$effect(() => {
		if (tags !== tagsAt.value) setTagsAt(tags);
	});
</script>

<div class="look" data-testid="title-look">
	<SegmentedControl options={TITLE_LOOKS} bind:value label="Hero layout" size="sm" announce={(l) => `Showing ${l}`} />
	<SegmentedControl options={TAG_SPOTS} bind:value={tags} label="Tags placement" size="sm" announce={(l) => `Tags: ${l}`} />
</div>

<style>
	.look { position: fixed; z-index: 30; left: 0; right: 0; bottom: 20px; width: max-content; max-width: calc(100vw - 24px); overflow-x: auto; margin-inline: auto; display: grid; gap: 4px; justify-items: center; padding: 5px; border-radius: 20px; background: color-mix(in srgb, var(--surface-1) 72%, transparent); border: 1px solid var(--line); box-shadow: var(--shadow-lift); backdrop-filter: blur(20px) saturate(160%); -webkit-backdrop-filter: blur(20px) saturate(160%); animation: rise var(--dur-slow) var(--ease-spring) both; }
	@media (max-width: 767px) { .look { bottom: calc(var(--tabbar, 56px) + env(safe-area-inset-bottom) + 16px); } }
	@keyframes rise { from { opacity: 0; transform: translateY(16px); } }
	@media (prefers-reduced-motion: reduce) { .look { animation: none; } }
</style>
