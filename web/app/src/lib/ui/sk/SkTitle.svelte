<script lang="ts">
	import SkHero from './SkHero.svelte';
	import SkRail from './SkRail.svelte';

	// A title page placeholder (TitleHero is a Billboard): the hero, the story lines under
	// it, then the cast and a wide row, in the same rhythm as lib/title-page.css.
	let { label = 'Loading the title' }: { label?: string } = $props();
</script>

<div class="sk-title" aria-busy="true" aria-label={label} data-testid="title-skeleton">
	<div>
		<SkHero />
		<div class="story"><span class="sk sk-line"></span><span class="sk sk-line"></span><span class="sk sk-line" style="width: 55%"></span></div>
	</div>
	<SkRail kind="person" count={10} titleWidth="80px" />
	<SkRail kind="wide" item="var(--title-item, 279px)" count={6} titleWidth="150px" />
</div>

<style>
	.sk-title { display: flex; flex-direction: column; gap: clamp(28px, 4vw, 44px); min-width: 0; padding-bottom: 48px; }
	.story { display: flex; flex-direction: column; gap: 9px; max-width: 72ch; margin-top: var(--s-5, 24px); }
	.story .sk-line { width: 100%; height: 14px; }
</style>
