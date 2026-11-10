<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import Icon from '#lib/ui/Icon.svelte';
	import { api } from '#lib/core/api.ts';
	import { attachHLS, holdStart, screenTarget } from '#lib/core/trailer/hls-player.ts';
	import { HERO_DELAY, setSoundOn, soundOn } from '#lib/core/trailer/trailer.ts';
	import { artistVideo, autoplayHere, loadUISettings, nativeHLS } from '#lib/core/trailer/autoplay.svelte.ts';
	import { heroFullscreen, heroVideoStart, type HeroVideo } from '#lib/core/trailer/hero-video.ts';

	// The artist page's short banner: the wide photo or the music video, with the video's controls and full screen.
	let { mbid, ready, list, picture, backgrounds, gone, box = $bindable(), onimage }: { mbid: string; ready: boolean; list: string[]; picture: string | null | undefined; backgrounds: string[]; gone: number; box?: HTMLElement; onimage: (img: HTMLImageElement) => void } = $props();
	let heroIn = $state(false);
	let heroTry = $state(0);
	$effect(() => {
		void list.length;
		void mbid;
		heroTry = 0;
	});
	const heroSrc = $derived(list[heroTry] ?? null);
	// A new hero picture (a late wide photo) fades in from dark instead of popping.
	$effect(() => {
		void heroSrc;
		heroIn = false;
	});
	const heroSoft = $derived(!!heroSrc && heroSrc === picture && !backgrounds.includes(heroSrc));
	$effect(() => {
		void mbid;
		heroIn = false;
		heroVideo = null;
		videoIn = false;
	});
	// The hero's music video (muted, looping, relayed); none keeps the photo.
	let heroVideo = $state<HeroVideo | null>(null);
	let videoEl: HTMLVideoElement | undefined = $state();
	let videoMuted = $state(true);
	let videoPaused = $state(false);
	// Sound is one choice for the whole app, shared with trailers: turning it on or off here carries to every later video and trailer.
	function toggleMute() {
		if (!videoEl) return;
		videoEl.muted = !videoEl.muted;
		videoMuted = videoEl.muted;
		setSoundOn(!videoEl.muted);
	}
	// When a video ends: the artist's next unplayed video, else the same one again.
	let playedVideos: string[] = [];
	// Videos that would not play here (twice: as picked, then at the tallest picture) are never attached again on this page, so a failing stream drops back to the photo.
	let heroBad = new Set<string>();
	// The key of the video being tried once more at the tallest picture.
	let heroTall = $state('');
	$effect(() => {
		void mbid;
		playedVideos = [];
		heroBad = new Set();
		heroTall = '';
	});
	async function nextVideo() {
		const cur = heroVideo;
		const el = videoEl;
		if (!cur || !el) return;
		if (!playedVideos.includes(cur.key)) playedVideos = [...playedVideos, cur.key];
		const want = mbid;
		const next = await api.get<HeroVideo>(`/music/artists/${want}?part=video&skip=${encodeURIComponent(playedVideos.join(','))}`).catch(() => null);
		if (want !== mbid || heroVideo !== cur) return;
		if (next?.proxy_url && next.key !== cur.key) {
			heroVideo = next;
			return;
		}
		// None left: start over from the first, repeating this one now.
		playedVideos = [cur.key];
		el.currentTime = heroVideoStart(cur);
		void el.play().catch(() => {});
	}
	function togglePause() {
		if (!videoEl) return;
		if (videoEl.paused) void videoEl.play().catch(() => {});
		else videoEl.pause();
	}
	// Full screen is the browser's own, on the hero: the video fits whole (black bars, never stretched or cut) with exit, pause and sound. iPhone Safari lets only the video itself go full screen, in its own player.
	let heroFull = $state(false);
	function fullScreen() {
		const el = videoEl as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | undefined;
		if (!el) return;
		if (document.fullscreenElement) void document.exitFullscreen().catch(() => {});
		else if (heroFullscreen(document.fullscreenEnabled, box, el) === 'page') void box?.requestFullscreen().catch(() => {});
		else el.webkitEnterFullscreen?.();
	}
	$effect(() => {
		if (heroFull && !heroVideo) void document.exitFullscreen().catch(() => {});
	});
	// The video keeps the stage while paused or between two videos (a video that ends also fires pause), so the banner keeps its size either way.
	let videoIn = $state(false);

	// As with a trailer hero, the beat (HERO_DELAY) starts when the hero shows. The video, looked up ahead of time when possible, is asked for at the same moment, only where trailers may autoplay.
	let beatDone = $state(false);
	$effect(() => {
		const want = mbid;
		if (!ready || mbid !== want) return;
		beatDone = false;
		const beat = setTimeout(() => (beatDone = true), HERO_DELAY);
		return () => clearTimeout(beat);
	});
	$effect(() => {
		const want = mbid;
		if (!ready || mbid !== want || heroVideo) return;
		const ctl = new AbortController();
		void loadUISettings().then(() => {
			if (ctl.signal.aborted || want !== mbid || !autoplayHere()) return;
			void artistVideo(want, ctl.signal).then((v) => !ctl.signal.aborted && want === mbid && v?.proxy_url && !heroBad.has(v.key) && (heroVideo = v));
		});
		return () => ctl.abort();
	});

	// Play it on the banner; an error drops back to the photo.
	let attached = $state(false);
	$effect(() => {
		const v = heroVideo;
		const el = videoEl;
		if (!v || !el) return;
		// One more try at the tallest picture: a stream can carry one rendition the browser will not decode (one music video's 720p did, its 1080p played).
		const tall = heroTall === v.key;
		let detach: (() => void) | null = null;
		let stopped = false;
		const fail = () => {
			if (stopped) return;
			if (!tall) {
				heroTall = v.key;
				return;
			}
			heroBad.add(v.key);
			videoIn = false;
			heroVideo = null;
		};
		el.muted = true;
		videoMuted = true;
		el.addEventListener('error', fail);
		// Never the opening: a start the browser dropped (Safari's own HLS) jumps back to it.
		const unhold = holdStart(el, heroVideoStart(v));
		void attachHLS(el, v.proxy_url, nativeHLS(), heroVideoStart(v), tall ? 1080 : screenTarget('cover'), tall ? 1080 : 0)
			.then((d) => {
				if (stopped) return d();
				detach = d;
				attached = true;
			})
			.catch(fail);
		return () => {
			stopped = true;
			attached = false;
			el.removeEventListener('error', fail);
			unhold();
			detach?.();
		};
	});

	// The photo and name hold for the beat; then the video takes over.
	$effect(() => {
		if (!(attached && beatDone && videoEl && heroVideo)) return;
		const el = videoEl;
		void el
			.play()
			.then(() => {
				// Sound was left on elsewhere, so unmute. A browser that pauses an unmuted video without a recent click keeps it muted, and the button shows the truth either way.
				if (!soundOn()) return;
				el.muted = false;
				if (el.paused) {
					el.muted = true;
					void el.play().catch(() => {});
				}
				videoMuted = el.muted;
			})
			.catch((e: unknown) => {
				// A play the page itself cut short (the stream was swapped or detached) is not the video failing.
				if (e instanceof DOMException && e.name === 'AbortError') return;
				if (heroVideo) heroBad.add(heroVideo.key);
				videoIn = false;
				heroVideo = null;
			});
	});

	// Some music videos carry black bars in the picture itself (an old 4:3 video in a wide frame). The frame is read a few times as it plays; when both sides are black the video is scaled up just enough to hide them.
	let crop = $state(1);
	let barTimers: ReturnType<typeof setTimeout>[] = [];
	function sideBars(v: HTMLVideoElement): number {
		try {
			const w = 64, h = 36, c = document.createElement('canvas');
			c.width = w; c.height = h;
			const g = c.getContext('2d', { willReadFrequently: true });
			if (!g || !v.videoWidth) return 0;
			g.drawImage(v, 0, 0, w, h);
			const d = g.getImageData(0, 0, w, h).data;
			const lit = (x: number) => {
				let l = 0;
				for (let y = 0; y < h; y++) { const k = (y * w + x) * 4; l += d[k] + d[k + 1] + d[k + 2]; }
				return l / (h * 3 * 255) > 0.05;
			};
			let left = 0, right = 0;
			while (left < w / 3 && !lit(left)) left++;
			while (right < w / 3 && !lit(w - 1 - right)) right++;
			// Only bars on both sides count; a dark scene on one side is not a bar.
			return left >= 2 && right >= 2 ? Math.min(left, right) / w : 0;
		} catch {
			return 0;
		}
	}
	function findBars() {
		barTimers.forEach(clearTimeout);
		const v = videoEl;
		if (!v) return;
		let least = Infinity;
		barTimers = [1500, 4000, 8000].map((ms) => setTimeout(() => {
			// The smallest bar seen wins, so a dark moment never over-crops.
			least = Math.min(least, sideBars(v));
			crop = least > 0 && least < 0.3 ? Math.min(1 / (1 - 2 * least), 1.5) : 1;
		}, ms));
	}
	$effect(() => {
		void heroVideo;
		crop = 1;
		return () => barTimers.forEach(clearTimeout);
	});
</script>

<svelte:document onfullscreenchange={() => (heroFull = !!box && document.fullscreenElement === box)} />

{#snippet stageArt()}
	{#if heroSrc}
		{#key heroSrc}
			<img class="art" class:soft={heroSoft} class:in={heroIn} class:under={videoIn} src={heroSrc} alt="" decoding="async" fetchpriority="high" onload={(e) => { heroIn = true; onimage(e.currentTarget as HTMLImageElement); }} onerror={() => { heroIn = false; heroTry++; }} />
		{/key}
	{/if}
{/snippet}

<header class="hero" class:paused={videoIn && videoPaused && !heroFull} class:vid={videoIn} class:bare={!heroSrc && !heroVideo} class:full={heroFull} style:--gone={gone.toFixed(3)} bind:this={box} data-hero data-testid="artist-hero">
	<div class="stage" aria-hidden="true">
		{@render stageArt()}
		{#if heroVideo}
			<video class="clip" class:in={videoIn || heroFull} style:--crop={heroFull ? 1 : crop} bind:this={videoEl} muted playsinline disablepictureinpicture preload="metadata" onplaying={() => { videoIn = true; videoPaused = false; findBars(); }} onpause={() => (videoPaused = true)} onended={() => void nextVideo()} onplay={() => (videoPaused = false)} onvolumechange={() => videoEl && (videoMuted = videoEl.muted)} data-testid="artist-hero-video"></video>
		{/if}
	</div>
	<div class="shade" aria-hidden="true"></div>
	<div class="dim" aria-hidden="true"></div>
	{#if heroVideo && videoIn}
		<div class="vctl" data-testid="artist-hero-video-controls">
			<IconButton icon={videoPaused ? 'play' : 'pause'} label={videoPaused ? t('music.playTheVideo') : t('music.pauseTheVideo')} variant="glass" size="md" tipSide="below" onclick={togglePause} />
			<IconButton icon={videoMuted ? 'soundOff' : 'sound'} label={videoMuted ? t('music.turnSoundOn') : t('music.turnSoundOff')} variant="glass" size="md" tipSide="below" onclick={toggleMute} />
			{#if !heroFull}<IconButton icon="expand" label={t('music.playFullScreen')} variant="glass" size="md" tipSide="below" onclick={fullScreen} />{/if}
		</div>
		{#if heroFull}<button type="button" class="vexit" onclick={fullScreen} aria-label={t('music.exitFullScreen')} data-testid="artist-hero-video-exit" {@attach (n) => n.focus()}><Icon name="x" size={20} /></button>{/if}
	{/if}
</header>

<style>
	:global(.artist.tinted) .hero { --hero-ink: var(--ar-bg); }
	.hero { --hero-ink: #07080a; position: relative; isolation: isolate; overflow: hidden; display: flex; align-items: flex-end; height: clamp(280px, 44vh, 520px); transition: background-color var(--dur-slow) var(--ease-out); -webkit-mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .82) 68%, rgb(0 0 0 / .5) 80%, rgb(0 0 0 / .18) 91%, transparent 100%); mask-image: linear-gradient(to bottom, #000 55%, rgb(0 0 0 / .82) 68%, rgb(0 0 0 / .5) 80%, rgb(0 0 0 / .18) 91%, transparent 100%); margin: calc(var(--s-6) * -1) calc(var(--gutter) * -1) 0; background: var(--hero-ink); color: #fff; }
	.stage { position: absolute; inset: 0; z-index: -2; scale: calc(1.1 - .1 * var(--gone, 0)); transform-origin: 50% 40%; will-change: scale; }
	.art { width: 100%; height: 100%; object-fit: cover; object-position: center 22%; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); }
	.art.in { opacity: .82; }
	.art.soft { filter: blur(56px) saturate(150%); transform: scale(1.3); }
	.art.soft.in { opacity: .6; }
	.shade { position: absolute; inset: 0; z-index: -1; pointer-events: none; background: linear-gradient(to top, color-mix(in srgb, var(--hero-ink) 60%, transparent) 0%, color-mix(in srgb, var(--hero-ink) 25%, transparent) 35%, transparent 60%), linear-gradient(to bottom, rgb(7 8 10 / .4), rgb(7 8 10 / 0) 30%); }
	.dim { position: absolute; inset: 0; z-index: -1; background: var(--hero-ink); opacity: calc(var(--gone, 0) * .85); pointer-events: none; }
	.hero.bare { background: linear-gradient(180deg, color-mix(in srgb, var(--hero-ink) 60%, #3a3d44), var(--hero-ink)); }
	@media (prefers-reduced-motion: reduce) { .stage { scale: 1; } }
	.clip { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); pointer-events: none; }
	.clip.in { opacity: 1; }
	.clip { scale: var(--crop, 1); transition: opacity var(--dur-slow) var(--ease-out), scale 600ms var(--ease-out); }
	.art.under { opacity: 0; transition-duration: 900ms; }
	.hero.paused .clip { opacity: 0; }
	.hero.paused .art.in.under { opacity: .82; }
	.hero.paused .art.soft.in.under { opacity: .6; }
	@media (prefers-reduced-motion: reduce) { .clip, .art { transition: none; } }
	.vctl { position: absolute; top: 16px; right: var(--gutter); z-index: 4; display: flex; gap: 8px; }
	@media (max-width: 767px) { .vctl { top: 12px; right: 16px; } }
	.vexit { display: grid; place-items: center; width: 38px; height: 38px; border-radius: 50%; background: rgb(255 255 255 / 14%); border: 1px solid rgb(255 255 255 / 18%); backdrop-filter: blur(18px) saturate(160%); -webkit-backdrop-filter: blur(18px) saturate(160%); color: #fff; cursor: pointer; transition: background var(--dur-fast) var(--ease-out); }
	.vexit:hover { background: rgb(255 255 255 / 26%); }
	.vexit:focus-visible { outline: 2px solid #fff; outline-offset: 2px; }
	.hero.full { height: auto; background: #000; -webkit-mask-image: none; mask-image: none; }
	.hero.full .stage { scale: 1; }
	.hero.full .art, .hero.full .shade, .hero.full .dim { display: none; }
	.hero.full .clip { object-fit: contain; background: #000; }
	.hero.full .vctl { right: max(24px, env(safe-area-inset-right)); bottom: max(24px, env(safe-area-inset-bottom)); top: auto; gap: 10px; }
	.vexit { width: 44px; height: 44px; }
	.vexit { position: absolute; top: max(20px, env(safe-area-inset-top)); right: max(24px, env(safe-area-inset-right)); z-index: 2; }
	@media (max-width: 767px) {
		.hero { height: 180px; margin-top: calc(var(--s-4) * -1); }
		.vctl { top: 12px; right: 12px; gap: 6px; }
	}
</style>
