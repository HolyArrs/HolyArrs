<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from '../Icon.svelte';
	import IconButton from './IconButton.svelte';

	// A titled horizontal row: native scroll with snap, arrow buttons on hover
	// (desktop), arrow keys scroll when the row has focus, optional See all ›.
	let {
		title,
		href,
		seeAll = 'See all',
		subtitle,
		item = '180px',
		children
	}: { title: string; href?: string; seeAll?: string; subtitle?: string; /** Item width, e.g. 180px posters, 300px wide cards. */ item?: string; children: Snippet } = $props();

	const id = $props.id();
	let scroller = $state<HTMLElement>();
	let atStart = $state(true);
	let atEnd = $state(false);

	function measure() {
		if (!scroller) return;
		atStart = scroller.scrollLeft <= 2;
		atEnd = scroller.scrollLeft + scroller.clientWidth >= scroller.scrollWidth - 2;
	}
	$effect(() => {
		if (!scroller) return;
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(scroller);
		return () => ro.disconnect();
	});
	function page(dir: 1 | -1) {
		scroller?.scrollBy({ left: dir * scroller.clientWidth * 0.9, behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
	}
	function key(e: KeyboardEvent) {
		if (e.target !== scroller) return;
		if (e.key === 'ArrowRight') { e.preventDefault(); page(1); }
		if (e.key === 'ArrowLeft') { e.preventDefault(); page(-1); }
	}
</script>

<section class="ui-shelf" style="--item: {item}">
	<header>
		<div>
			<h2 id="{id}-t">{#if href}<a href={href}>{title}</a>{:else}{title}{/if}</h2>
			{#if subtitle}<p>{subtitle}</p>{/if}
		</div>
		{#if href}<a class="all" {href}>{seeAll}<Icon name="chevronRight" size={15} /><span class="sr-only"> {title}</span></a>{/if}
	</header>
	<div class="wrap">
		<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions, a11y_no_static_element_interactions -->
		<div class="row" bind:this={scroller} onscroll={measure} onkeydown={key} tabindex="0" role="group" aria-labelledby="{id}-t">
			{@render children()}
		</div>
		<span class="arrow left" class:hide={atStart}><IconButton icon="chevronLeft" label="Scroll {title} back" variant="tinted" tabindex={-1} tooltip={false} onclick={() => page(-1)} /></span>
		<span class="arrow right" class:hide={atEnd}><IconButton icon="chevronRight" label="Scroll {title} forward" variant="tinted" tabindex={-1} tooltip={false} onclick={() => page(1)} /></span>
	</div>
</section>

<style>
	.ui-shelf { min-width: 0; }
	header { display: flex; align-items: flex-end; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
	h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	h2 a:hover { color: var(--accent-text); }
	p { color: var(--text-muted); font: var(--t-callout); margin-top: 2px; }
	.all { display: inline-flex; align-items: center; gap: 2px; min-height: 32px; flex: none; color: var(--accent-text); font: var(--t-callout); font-weight: 500; border-radius: 6px; }
	.all:hover { text-decoration: underline; text-underline-offset: 3px; }
	.wrap { position: relative; }
	.row {
		display: grid; grid-auto-flow: column; grid-auto-columns: var(--item); gap: var(--shelf-gap);
		overflow-x: auto; overscroll-behavior-x: contain; scroll-snap-type: x mandatory; scroll-padding-inline: var(--gutter);
		margin-inline: calc(var(--gutter) * -1); padding: 6px var(--gutter) 14px; scrollbar-width: none; border-radius: var(--r-card);
	}
	.row::-webkit-scrollbar { display: none; }
	.row > :global(*) { scroll-snap-align: start; }
	.row:focus-visible { outline: none; box-shadow: inset var(--focus-ring); }
	.arrow { position: absolute; top: calc(50% - 26px); opacity: 0; transition: opacity var(--dur-fast) ease; }
	.arrow :global(.ui-ibtn) { width: 40px; height: 40px; box-shadow: var(--shadow-lift); background: var(--surface-1); color: var(--text); }
	.left { left: -12px; }
	.right { right: -12px; }
	.wrap:hover .arrow:not(.hide) { opacity: 1; }
	.hide { pointer-events: none; }
	@media (hover: none) { .arrow { display: none; } }
	@media (max-width: 767px) { .row { grid-auto-columns: calc(var(--item) * .78); } }
</style>
