<!-- Titled horizontal row of cards with paging arrows and lazy mounting. -->
<script lang="ts" generics="T = unknown">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import { untrack, type Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import { dragScroll } from './drag-scroll.ts';
	import IconButton from './IconButton.svelte';
	import { nearView, setShelfNear, whenIdle } from './lazy-art.ts';
	import { mountNow, nextLimit, ROW_FIRST } from './row-window.ts';
	import { rove } from './rove.ts';

	let {
		title,
		href,
		seeAll = t('ui.shelf.seeAll'),
		subtitle,
		item = '180px',
		anchor,
		onseeall,
		tools,
		items,
		card,
		key: keyOf = (_it: T, i: number) => i,
		defer = false,
		placeholder = 'poster',
		lines = 2,
		whole = false,
		centerFew = false,
		children
	}: {
		title: string;
		href?: string;
		seeAll?: string;
		subtitle?: string;
		item?: string;
		anchor?: string;
		onseeall?: () => void;
		tools?: Snippet;
		items?: readonly T[];
		card?: Snippet<[T, number]>;
		key?: (it: T, i: number) => string | number;
		defer?: boolean;
		placeholder?: 'poster' | 'square' | 'round' | 'wide';
		lines?: 1 | 2;
		whole?: boolean;
		centerFew?: boolean;
		children?: Snippet;
	} = $props();

	const id = $props.id();

	let near = $state(false);
	let scroller = $state<HTMLElement>();
	let atStart = $state(true);
	let atEnd = $state(false);
	// Deferred shelves start as a skeleton until near the viewport or idle.
	let mounted = $state(untrack(() => !defer));
	// Cards render in growing chunks to keep the first paint light.
	let limit = $state(ROW_FIRST);
	let probe = $state<HTMLElement>();
	let per = $state(0);
	let few = $state(false);

	setShelfNear(() => near);

	const shown = $derived(items ? (items.length > limit ? items.slice(0, limit) : items) : []);
	const more = $derived(!!items && items.length > limit);

	$effect(() => {
		if (!defer) mounted = true;
	});
	$effect(() => {
		if (!scroller) return;
		void shown.length;
		measure();
		const ro = new ResizeObserver(measure);
		ro.observe(scroller);
		return () => ro.disconnect();
	});

	// For whole-page shelves: count how many cards fit so pages align to card edges.
	function fit() {
		if (!whole || !scroller || !probe) return;
		const gap = parseFloat(getComputedStyle(scroller).columnGap) || 0;
		const want = probe.offsetWidth || 1;
		per = Math.max(1, Math.round((scroller.clientWidth + gap) / (want + gap)));
		few = scroller.children.length < per;
	}
	function measure() {
		if (!scroller) return;
		fit();
		atStart = Math.abs(scroller.scrollLeft) <= 2;
		atEnd = !more && Math.abs(scroller.scrollLeft) + scroller.clientWidth >= scroller.scrollWidth - 2;
		if (items) limit = nextLimit(limit, items.length, scroller);
	}
	// Mounts the row when it nears the screen, or when the browser is idle.
	function soon(node: Element) {
		if (mounted) return {};
		if (typeof IntersectionObserver === 'undefined' || mountNow(node.getBoundingClientRect().top, innerHeight)) {
			mounted = true;
			return {};
		}
		const go = () => {
			io.disconnect();
			stop();
			mounted = true;
		};
		const io = new IntersectionObserver((es) => es.some((e) => e.isIntersecting) && go(), { rootMargin: '0px 0px 150% 0px' });
		io.observe(node);
		const stop = whenIdle(go);
		return { destroy: () => { io.disconnect(); stop(); } };
	}
	// Whole shelves page by exact rows; others scroll a bit less than a screen.
	function page(dir: 1 | -1) {
		if (!scroller) return;
		const step = whole ? scroller.clientWidth + (parseFloat(getComputedStyle(scroller).columnGap) || 0) : scroller.clientWidth * 0.9;
		scroller.scrollBy({ left: dir * step * (getComputedStyle(scroller).direction === 'rtl' ? -1 : 1), behavior: matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth' });
	}
</script>

<section class="ui-shelf" id={anchor} style="--item: {item}" use:nearView={() => (near = true)} use:soon>
	<header class:bare={!subtitle}>
		<div>
			<h2 id="{id}-t">{#if href}<a href={href}>{title}</a>{:else}{title}{/if}</h2>
			{#if subtitle}<p>{subtitle}</p>{/if}
		</div>
		{#if href}<a class="all" {href}>{seeAll}<Icon name="chevronRight" size={15} /><span class="sr-only"> {title}</span></a>
		{:else if onseeall}<button class="all" type="button" aria-haspopup="dialog" onclick={onseeall}>{seeAll}<Icon name="chevronRight" size={15} /><span class="sr-only"> {title}</span></button>{/if}
		{#if tools}<div class="tools">{@render tools()}</div>{/if}
	</header>
	{#if !mounted}
		<div class="sk {placeholder}" class:one={lines === 1} aria-hidden="true"></div>
	{:else}
	<div class="wrap" class:whole class:few={whole && centerFew && few} style={whole && per ? `--per: ${per}` : undefined}>
		{#if whole}<span class="probe" bind:this={probe} aria-hidden="true"></span>{/if}
		<div class="row" use:dragScroll use:rove bind:this={scroller} onscroll={measure} role="group" aria-labelledby="{id}-t">
			{#if items && card}{#each shown as it, i (keyOf(it, i))}{@render card(it, i)}{/each}{:else}{@render children?.()}{/if}
		</div>
		<span class="arrow left" class:hide={atStart}><IconButton icon="chevronLeft" label={t('ui.shelf.scrollBack', { title })} variant="tinted" tabindex={-1} tooltip={false} onclick={() => page(-1)} /></span>
		<span class="arrow right" class:hide={atEnd}><IconButton icon="chevronRight" label={t('ui.shelf.scrollForward', { title })} variant="tinted" tabindex={-1} tooltip={false} onclick={() => page(1)} /></span>
	</div>
	{/if}
</section>

<style>
	.ui-shelf { min-width: 0; }
	/* Hide the shelf when every card has removed itself. */
	.ui-shelf:has(.row > :global(.gone)):not(:has(.row > :global(:not(.gone)))) { display: none; }
	header { display: flex; flex-wrap: wrap; align-items: flex-end; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
	header.bare { align-items: center; }
	.tools { flex: 1 0 100%; min-width: 0; }
	h2 { font: var(--t-title2); letter-spacing: var(--t-title2-track); }
	h2 a:hover { color: var(--accent-text); }
	@media (max-width: 767px) and (pointer: coarse) {
		h2 a { position: relative; }
		h2 a::after { content: ''; position: absolute; inset-inline: 0; top: 50%; height: 44px; transform: translateY(-50%); }
	}
	p { color: var(--text-muted); font: var(--t-callout); margin-top: 2px; }
	.all { display: inline-flex; align-items: center; gap: 2px; min-height: 44px; flex: none; color: var(--accent-text); font: var(--t-callout); font-weight: 500; border-radius: 6px; }
	.all:hover { text-decoration: underline; text-underline-offset: 3px; }
	.wrap { position: relative; }
	.row {
		display: grid; grid-auto-flow: column; grid-auto-columns: var(--item); gap: var(--shelf-gap);
		overflow-x: auto; overscroll-behavior-x: contain; scroll-snap-type: x mandatory; scroll-padding-inline: var(--gutter);
		margin-inline: calc(var(--gutter) * -1); padding: 6px var(--gutter) 14px; scrollbar-width: none; border-radius: var(--r-card);
	}
	.row::-webkit-scrollbar { display: none; }
	/* Skeleton sized like real cards so the page does not jump. */
	.sk { --w: var(--item); --art: calc(var(--w) * 1.5); --text-h: 52.5px; height: calc(var(--art) + var(--text-h)); margin: 6px 0 14px; overflow: hidden;
		background: repeating-linear-gradient(90deg, var(--surface-2) 0 var(--w), transparent var(--w) calc(var(--w) + var(--shelf-gap))) 0 0 / 100% var(--art) no-repeat; }
	.sk.one { --text-h: 35px; }
	.sk.square, .sk.round { --art: var(--w); }
	.sk.wide { --art: calc(var(--w) * 9 / 16); --text-h: 0px; }
	@media (max-width: 767px) { .sk { --w: calc(var(--item) * .78); } }
	.row > :global(*) { scroll-snap-align: start; }
	.arrow { position: absolute; top: calc(50% - 26px); opacity: 0; transition: opacity var(--dur-fast) var(--ease-out); }
	.arrow :global(.ui-ibtn) { width: 40px; height: 40px; box-shadow: var(--shadow-lift); background: var(--surface-1); color: var(--text); }
	.left { inset-inline-start: -12px; }
	.right { inset-inline-end: -12px; }
	.wrap:hover .arrow:not(.hide) { opacity: 1; }
	.hide { pointer-events: none; }
	@media (hover: none) { .arrow { display: none; } }
	@media (max-width: 767px) { .row { grid-auto-columns: calc(var(--item) * .78); } }
	.row:global(.dragging) { cursor: grabbing; user-select: none; }
	.probe { position: absolute; visibility: hidden; height: 0; width: var(--item); pointer-events: none; }
	@media (max-width: 767px) { .probe { width: calc(var(--item) * .78); } }
	.whole .row { margin-inline: 0; padding-inline: 0; scroll-padding-inline: 0; grid-auto-columns: calc((100% - (var(--per, 1) - 1) * var(--shelf-gap)) / var(--per, 1)); }
	.whole .row > :global(*) { width: 100%; min-width: 0; }
	.whole.few .row { justify-content: center; }
	.whole .left { inset-inline-start: 4px; }
	.whole .right { inset-inline-end: 4px; }
	.row:global(.dragging) :global(*) { pointer-events: none; }
</style>
