<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import Shelf from '#lib/ui/Shelf.svelte';
	import { musicCardHref, musicImage, type MusicCardData } from './music-trending.ts';
	import { onMount } from 'svelte';
	import { artistVideo, autoplayHere, loadUISettings, nativeHLS, prefetchArtistVideo } from '../core/trailer/autoplay.svelte.ts';
	import { attachHLS } from '../core/trailer/hls-player.ts';
	import { heroVideoStart, type HeroVideo } from '../core/trailer/hero-video.ts';

	// Music videos people watch: wide 16:9 cards of charting artists whose videos the server already knows. A card shows the artist's picture until its video is ready; then, on hover or focus, the video plays muted.
	let { title, reason, items }: { title: string; reason: string; items: MusicCardData[] } = $props();

	/** Ready videos by artist id; a card without one keeps its picture. */
	let ready = $state<Record<string, HeroVideo>>({});
	let playing = $state('');
	let shown = $state('');
	let videoEl: HTMLVideoElement | undefined = $state();
	let noPic = $state<Record<string, boolean>>({});

	// Fetched ahead: the first cards' streams resolve while the row is on screen, within the trailers' budget, so hover plays at once.
	onMount(() => {
		const ctl = new AbortController();
		void loadUISettings().then(() => {
			if (ctl.signal.aborted || !autoplayHere()) return;
			for (const c of items.slice(0, 8)) {
				const id = c.mbid ?? '';
				if (!id) continue;
				prefetchArtistVideo(id);
				void artistVideo(id, ctl.signal).then((v) => !ctl.signal.aborted && v?.proxy_url && (ready = { ...ready, [id]: v }));
			}
		});
		return () => ctl.abort();
	});

	function start(c: MusicCardData) {
		const id = c.mbid ?? '';
		if (!id || !autoplayHere()) return;
		if (ready[id]) {
			playing = id;
			return;
		}
		void artistVideo(id).then((v) => {
			if (!v?.proxy_url) return;
			ready = { ...ready, [id]: v };
		});
	}
	function stop(c: MusicCardData) {
		if (playing === (c.mbid ?? '')) {
			playing = '';
			shown = '';
		}
	}

	$effect(() => {
		const id = playing;
		const v = ready[id];
		const el = videoEl;
		if (!id || !v || !el) return;
		let detach: (() => void) | null = null;
		let stopped = false;
		el.muted = true;
		const fail = () => {
			shown = '';
			playing = '';
		};
		el.addEventListener('error', fail);
		void attachHLS(el, v.proxy_url, nativeHLS(), heroVideoStart(v), 480)
			.then((d) => {
				if (stopped) return d();
				detach = d;
			})
			.catch(fail);
		return () => {
			stopped = true;
			el.removeEventListener('error', fail);
			detach?.();
		};
	});
</script>

<div class="row" data-testid="music-row-videos">
	<Shelf {title} subtitle={reason} item="var(--video-item)" {items} key={(c) => c.key} placeholder="wide" lines={1}>
		{#snippet card(c: MusicCardData)}
			{@const id = c.mbid ?? ''}
			{@const pic = musicImage(c)}
			<a class="vcard" href={musicCardHref(c)} onpointerenter={(e) => e.pointerType === 'mouse' && start(c)} onpointerleave={() => stop(c)} onfocus={() => start(c)} onblur={() => stop(c)}>
				<span class="frame">
					{#if pic && !noPic[c.key]}
						<img src={pic} alt="" loading="lazy" decoding="async" draggable="false" onerror={() => (noPic = { ...noPic, [c.key]: true })} />
					{/if}
					{#if playing === id && ready[id]}
						<video bind:this={videoEl} class:in={shown === id} muted loop playsinline disablepictureinpicture preload="auto" onplaying={() => (shown = id)}></video>
					{/if}
					{#if ready[id]}<span class="badge" aria-hidden="true">{t('music.video')}</span>{/if}
				</span>
				<span class="name">{c.name}</span>
			</a>
		{/snippet}
	</Shelf>
</div>

<style>
	.row { --video-item: min(78vw, 320px); }
	.vcard { display: grid; gap: 8px; color: inherit; text-decoration: none; }
	.frame { position: relative; display: block; aspect-ratio: 16 / 9; border-radius: 10px; overflow: hidden; background: var(--surface-2, #1c1c1e); }
	.frame img, .frame video { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center 25%; }
	.frame video { opacity: 0; transition: opacity var(--dur) var(--ease-out); }
	.frame video.in { opacity: 1; }
	.badge { position: absolute; left: 8px; bottom: 8px; padding: 2px 8px; border-radius: 999px; font-size: 11px; font-weight: 600; background: rgb(0 0 0 / 0.55); color: #fff; }
	.name { font-size: 14px; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.vcard:focus-visible .frame { outline: 2px solid var(--accent, #fa2d48); outline-offset: 2px; }
	@media (min-width: 768px) { .row { --video-item: 340px; } }
</style>
