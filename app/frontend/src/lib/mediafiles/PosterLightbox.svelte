<!-- A poster shown large over the page. Escape, the close button or a click beside it closes it, and focus goes back to what opened it. -->
<script lang="ts">
	import Icon from '#lib/ui/Icon.svelte';
	import { posterSrc } from '#lib/ui/library-art.ts';
	import { proxied } from '../core/tmdb-image.ts';
	import { t } from '#lib/core/i18n/i18n.svelte.ts';

	let { art, title, sub = '', square = false, onclose }: { art: string; title: string; sub?: string; square?: boolean; onclose: () => void } = $props();

	let dialog = $state<HTMLDialogElement>();
	let opener: Element | null = null;
	const src = $derived(proxied(posterSrc(art, 480)));

	$effect(() => {
		if (!dialog || dialog.open) return;
		opener = document.activeElement;
		dialog.showModal();
		return () => {
			if (opener instanceof HTMLElement && opener.isConnected) opener.focus({ preventScroll: true });
		};
	});

	// A click on the dialog itself, not on the poster or its words, is a click beside it.
	function beside(e: MouseEvent) {
		if (e.target === dialog || (e.target as HTMLElement).classList.contains('fig')) onclose();
	}
</script>

<dialog bind:this={dialog} class="lb" aria-label={title} onclose={onclose} onclick={beside} data-testid="poster-lightbox">
	<figure class="fig">
		<img class:square {src} alt="" decoding="async" referrerpolicy="no-referrer" />
		<figcaption><b>{title}</b>{#if sub}<span>{sub}</span>{/if}</figcaption>
	</figure>
	<button type="button" class="x" aria-label={t('ui.sheet.close')} onclick={onclose}><Icon name="x" size={20} /></button>
</dialog>

<style>
	.lb { width: 100vw; height: 100dvh; max-width: none; max-height: none; margin: 0; padding: 0; border: 0; background: transparent; overflow: hidden; }
	.lb::backdrop { background: rgb(10 10 14 / 72%); backdrop-filter: blur(2px); animation: fade var(--dur-fast) var(--ease-out) both; }
	.fig { position: relative; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 14px; height: 100%; margin: 0; padding: 24px 16px; box-sizing: border-box; animation: rise var(--dur) var(--ease-out) both; }
	img { display: block; height: min(720px, calc(100dvh - 140px)); max-width: calc(100vw - 32px); aspect-ratio: 2 / 3; object-fit: cover; border-radius: 14px; background: rgb(255 255 255 / 6%); box-shadow: 0 30px 80px rgb(0 0 0 / 60%), 0 0 0 1px rgb(255 255 255 / 8%); }
	img.square { aspect-ratio: 1; }
	figcaption { display: flex; flex-direction: column; align-items: center; gap: 2px; max-width: min(560px, calc(100vw - 32px)); color: #fff; text-align: center; font: var(--t-callout); }
	figcaption b { font: var(--t-headline); color: #fff; }
	figcaption span { color: rgb(255 255 255 / 72%); font: var(--t-footnote); overflow-wrap: anywhere; }
	.x { position: fixed; top: 16px; inset-inline-end: 16px; display: grid; place-items: center; width: 44px; height: 44px; border: 0; border-radius: 50%; background: rgb(255 255 255 / 14%); color: #fff; cursor: pointer; }
	.x:hover { background: rgb(255 255 255 / 22%); }
	.x:focus-visible { outline: none; box-shadow: 0 0 0 3px rgb(255 255 255 / 70%); }
	@keyframes fade { from { opacity: 0; } }
	@keyframes rise { from { opacity: 0; transform: scale(0.97); } }
	@media (prefers-reduced-motion: reduce) { .fig, .lb::backdrop { animation: none; } }
</style>
