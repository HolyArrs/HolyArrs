<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	import IconButton from '#lib/ui/IconButton.svelte';
	import { onDestroy } from 'svelte';
	import { closeTheater, holdScroll, runExit, theater, waitForResume } from '#lib/core/trailer/theater.svelte.ts';
	import { embedUrl, playingWord, setSoundOn } from '#lib/core/trailer/trailer.ts';
	import { flipFrames, reducedMotion, THEATER_CHROME_MS, THEATER_EASE, THEATER_FADE_MS, THEATER_GROW_MS, THEATER_SHRINK_MS, type Box } from '#lib/core/motion.ts';
	import { nativeHLS } from '#lib/core/trailer/autoplay.svelte.ts';
	import { missLine } from '#lib/core/trailer/trailer-miss.ts';
	import { attachHLS, screenTarget } from '#lib/core/trailer/hls-player.ts';

	// The full-screen trailer, one shared element from the card to full screen and back. The picture grows out of the small player's rect (the frame scales, the picture inside is counter-scaled to one uniform scale so it never stretches; transforms only, so the compositor runs it), carries on from the same second with the same sound, and shows the small player's last frame until its own first frame is ready. The scrim and controls fade in after the grow and out before the shrink. Esc or Close shrink it back where it came from (a hero carries on playing there); the end of the trailer opens the title page under it first and shrinks into that page's hero. Reduced motion: a plain crossfade.
	let dialog = $state<HTMLDialogElement>();
	let stage = $state<HTMLElement>();
	let pic = $state<HTMLElement>();
	// The controls and their scrims: off while it grows or shrinks.
	let chrome = $state(false);
	// The small player's last frame is up until this player shows its own.
	let stillUp = $state(false);
	// The big player's media starts once the grow is done (the held frame shows meanwhile), so the grow has the main thread.
	let mediaOn = $state(false);
	let video = $state<HTMLVideoElement>();
	let frame = $state<HTMLIFrameElement>();
	let paused = $state(false);
	let muted = $state(false);
	let time = $state(0);
	let duration = $state(0);
	let idle = $state(false);
	// Every source refused (removed, private or not allowed outside YouTube).
	let unavailable = $state(false);
	let leaving = false;
	let release: (() => void) | null = null;
	let idleTimer: ReturnType<typeof setTimeout> | undefined;
	let tick: ReturnType<typeof setInterval> | undefined;
	const origin = typeof location !== 'undefined' ? location.origin : '';
	const now = $derived(theater.now);
	// What plays, in its own word: music video, teaser, trailer.
	const word = $derived(now ? playingWord(now.kind, now.what) : 'trailer');
	// The source, then its fallbacks one by one while they fail; "can't play here" only after the last.
	let tried = $state(0);
	const list = $derived(now ? [now.source, ...(now.fallbacks ?? [])] : []);
	// Why it will not play, when the server said (the reason rides on the embed); the line says so in plain words.
	const why = $derived.by(() => {
		for (const s of list) if (s.kind === 'embed' && s.why) return s.why;
		return undefined;
	});
	const src = $derived(list[Math.min(tried, list.length - 1)]);
	function fail() {
		if (tried + 1 < list.length) tried += 1;
		else unavailable = true;
	}

	const vw = () => document.documentElement.clientWidth || window.innerWidth;
	const vh = () => window.innerHeight;
	/** The picture's shape: the video's own, else the small player's frame, else 16:9. */
	function aspect(): number {
		if (video?.videoWidth && video.videoHeight) return video.videoWidth / video.videoHeight;
		const st = now?.still;
		if (st?.width && st.height) return st.width / st.height;
		return 16 / 9;
	}
	function move(from: Box, grow: boolean): Promise<unknown> {
		if (!stage || !pic || !dialog) return Promise.resolve();
		const f = flipFrames(from, aspect(), vw(), vh(), !grow);
		const opts: KeyframeAnimationOptions = { duration: grow ? THEATER_GROW_MS : THEATER_SHRINK_MS, easing: 'linear', fill: 'forwards' };
		// The ease is in the frames, so they play at linear timing.
		const a = track(stage.animate(f.outer.map((transform) => ({ transform })), opts));
		track(pic.animate(f.inner.map((transform) => ({ transform })), opts));
		track(dialog.animate(grow ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 1 }, { opacity: 0 }], { ...opts, easing: THEATER_EASE, pseudoElement: '::backdrop' }));
		return a.finished.catch(() => {});
	}
	function fade(inOut: 'in' | 'out'): Promise<unknown> {
		if (!dialog) return Promise.resolve();
		const k = inOut === 'in' ? [{ opacity: 0 }, { opacity: 1 }] : [{ opacity: 1 }, { opacity: 0 }];
		const opts: KeyframeAnimationOptions = { duration: THEATER_FADE_MS, easing: 'linear', fill: 'forwards' };
		track(dialog.animate(k, { ...opts, pseudoElement: '::backdrop' }));
		return track(dialog.animate(k, opts)).finished.catch(() => {});
	}
	// Its own animations, so a new open or the close clears them without asking the page for all of them.
	let anims: Animation[] = [];
	function track<T extends Animation>(a: T): T {
		anims.push(a);
		return a;
	}
	function clearAnims() {
		for (const a of anims) a.cancel();
		anims = [];
	}
	const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));
	/** Puts the small player's last frame in the picture until this one plays. */
	function keepStill(node: HTMLElement) {
		const c = now?.still;
		if (c) node.replaceChildren(c);
		return () => node.replaceChildren();
	}

	// A pass-through trailer (HLS relayed by HolyArrs) plays on the same <video>.
	$effect(() => {
		const v = video;
		const s = src;
		if (!v || s?.kind !== 'pass') return;
		let gone = false;
		let detach: (() => void) | null = null;
		attachHLS(v, s.url, nativeHLS(), now?.time ?? 0, screenTarget('contain'))
			.then((d) => (gone ? d() : (detach = d)))
			.catch(() => !gone && fail());
		return () => {
			gone = true;
			detach?.();
		};
	});

	$effect(() => {
		if (!now || !dialog || dialog.open) return;
		leaving = false;
		// The server already said this will not play here: say why at once.
		unavailable = now.source.kind === 'embed' && !!now.source.told && !now.fallbacks?.length;
		tried = 0;
		paused = false;
		muted = now.muted ?? false;
		time = now.time;
		duration = 0;
		chrome = false;
		stillUp = !!now.still;
		mediaOn = !now.still || reducedMotion();
		release = holdScroll();
		clearAnims();
		dialog.showModal();
		const r = now.rect;
		const d = dialog;
		const grown = r && r.width && !reducedMotion() ? move(r, true) : fade('in');
		void grown.then(() => {
			if (leaving || !d.open) return;
			mediaOn = true;
			chrome = true;
			wake();
			// Real full screen where the browser allows it, after the grow so it never jumps mid-move.
			d.requestFullscreen?.().catch(() => {});
		});
	});

	function onFullscreen() {
		if (!document.fullscreenElement && dialog?.open && !leaving) void close();
	}

	async function close(ended = false) {
		if (leaving || !dialog || !now) return;
		leaving = true;
		const o = now;
		const reduce = reducedMotion();
		// The controls go first, then the picture moves.
		chrome = false;
		clearTimeout(idleTimer);
		const at = video ? video.currentTime : time;
		// What the hero it lands in carries on with (same second, same sound).
		theater.back = { kind: o.kind, tmdb: o.tmdb, time: at, muted, ended, from: o.from };
		if (document.fullscreenElement) await document.exitFullscreen().catch(() => {});
		if (!reduce) await sleep(THEATER_CHROME_MS);
		let target: Box | null = o.rect;
		// The end of the trailer: the title page opens under the player, then it shrinks into its hero.
		if (ended && (await runExit(o))) {
			await new Promise((r) => requestAnimationFrame(() => requestAnimationFrame(r)));
			const h = document.querySelector<HTMLElement>('[data-testid="title-hero"]')?.getBoundingClientRect();
			target = h && h.width ? { left: h.left, top: h.top, width: h.width, height: h.height } : null;
		}
		if (!reduce && target?.width) {
			// Still playing while it shrinks (no frozen frame), then the hero takes over.
			await move(target, false);
			if (!ended && o.from === 'hero') await waitForResume(250);
		}
		video?.pause();
		if (!video) post('pauseVideo');
		await fade('out');
		dialog.close();
		clearAnims();
		theater.back = null;
		release?.();
		release = null;
		clearInterval(tick);
		closeTheater();
	}

	function wake() {
		idle = false;
		clearTimeout(idleTimer);
		idleTimer = setTimeout(() => (idle = !paused), 2600);
	}

	// The embed: commands and its time through postMessage.
	function post(func: string, args: unknown[] = []) {
		frame?.contentWindow?.postMessage(JSON.stringify({ event: 'command', func, args }), 'https://www.youtube-nocookie.com');
	}
	function frameLoaded() {
		frame?.contentWindow?.postMessage(JSON.stringify({ event: 'listening', id: 'holyarrs-theater', channel: 'widget' }), 'https://www.youtube-nocookie.com');
		if (now && now.time > 1) post('seekTo', [now.time, true]);
		post(muted ? 'mute' : 'unMute');
		post('setVolume', [100]);
		post('playVideo');
	}
	function onMessage(e: MessageEvent) {
		if (e.origin !== 'https://www.youtube-nocookie.com' || !frame || e.source !== frame.contentWindow) return;
		let d: { event?: string; info?: Record<string, unknown> | number };
		try {
			d = typeof e.data === 'string' ? JSON.parse(e.data) : e.data;
		} catch {
			return;
		}
		if (d.event === 'onError') {
			fail();
			return;
		}
		const info = typeof d.info === 'object' && d.info ? d.info : {};
		if (typeof info.currentTime === 'number') time = info.currentTime;
		if (typeof info.duration === 'number' && info.duration > 0) duration = info.duration;
		if (typeof info.muted === 'boolean') muted = info.muted;
		const state = d.event === 'onStateChange' ? d.info : info.playerState;
		if (state === 1) {
			paused = false;
			stillUp = false;
		}
		else if (state === 2) paused = true;
		else if (state === 0) void close(true);
	}

	function videoReady() {
		if (!video || !now) return;
		if (now.time > 0.5 && now.time < (video.duration || Infinity)) video.currentTime = now.time;
		video.muted = muted;
		void video.play().catch(() => {
			// Sound refused without a fresh click: play muted; the sound button turns it on.
			if (!video) return;
			video.muted = muted = true;
			void video.play().catch(() => {});
		});
	}

	function togglePlay() {
		wake();
		if (video) {
			if (video.paused) void video.play();
			else video.pause();
		} else {
			post(paused ? 'playVideo' : 'pauseVideo');
			paused = !paused;
		}
	}
	function toggleSound() {
		wake();
		muted = !muted;
		setSoundOn(!muted);
		if (video) video.muted = muted;
		else post(muted ? 'mute' : 'unMute');
	}
	function seek(e: Event) {
		wake();
		const t = Number((e.currentTarget as HTMLInputElement).value);
		time = t;
		if (video) video.currentTime = t;
		else post('seekTo', [t, true]);
	}
	function key(e: KeyboardEvent) {
		if (e.key === ' ' && !(e.target as HTMLElement)?.closest?.('button, input')) {
			e.preventDefault();
			togglePlay();
		} else wake();
	}
	const clock = (s: number) => {
		const t = Math.max(0, Math.floor(s));
		return `${Math.floor(t / 60)}:${String(t % 60).padStart(2, '0')}`;
	};

	$effect(() => {
		if (!now || src?.kind !== 'embed') return;
		tick = setInterval(() => post('getCurrentTime'), 1000);
		return () => clearInterval(tick);
	});
	onDestroy(() => {
		release?.();
		clearTimeout(idleTimer);
		clearInterval(tick);
	});
</script>

<svelte:window onmessage={onMessage} />
<svelte:document onfullscreenchange={onFullscreen} />

<dialog
	bind:this={dialog}
	class="ui-theater"
	class:idle
	class:chrome
	aria-label={now ? t('frame.theater.label', { title: now.title, word }) : t('frame.theater.trailer')}
	data-testid="theater"
	oncancel={(e) => {
		e.preventDefault();
		void close();
	}}
	onpointermove={wake}
	onkeydown={key}
>
	{#if now && src}
		<div class="stage" bind:this={stage}>
			<div class="pic" bind:this={pic}>
			{#if !mediaOn || (src.kind === 'embed' && src.told)}
				<!-- The held frame alone while it grows (or for good, when nothing plays here). -->
			{:else if src.kind === 'direct' || src.kind === 'pass'}
				<!-- svelte-ignore a11y_media_has_caption -->
				<video
					bind:this={video}
					src={src.kind === 'direct' ? src.url : undefined}
					playsinline
					preload="auto"
					disablepictureinpicture
					onloadedmetadata={() => {
						duration = video?.duration ?? 0;
						videoReady();
					}}
					ontimeupdate={() => (time = video?.currentTime ?? 0)}
					onplay={() => (paused = false)}
					onplaying={() => (stillUp = false)}
					onpause={() => {
						paused = true;
						idle = false;
					}}
					onvolumechange={() => (muted = video?.muted ?? false)}
					onerror={fail}
					onended={() => void close(true)}
					onclick={wake}
				></video>
			{:else}
				<iframe
					bind:this={frame}
					src={embedUrl(src.key, origin, false)}
					title={t('frame.theater.label', { title: now.title, word })}
					allow="autoplay; encrypted-media; fullscreen"
					referrerpolicy="strict-origin-when-cross-origin"
					sandbox="allow-scripts allow-same-origin allow-presentation"
					tabindex="-1"
					onload={frameLoaded}
				></iframe>
			{/if}
			{#if now.still}
				<div class="still" class:off={!stillUp} aria-hidden="true" {@attach keepStill}></div>
			{/if}
			</div>
			{#if src.kind === 'embed'}
				<button type="button" class="catch" aria-label={t('frame.theater.controls')} onclick={wake} tabindex="-1"></button>
			{/if}
			{#if unavailable}
				<div class="gone" role="status">
					<p>{missLine(why, word)}</p>
					<button type="button" class="gone-close" onclick={() => void close()}>{t('frame.theater.close')}</button>
				</div>
			{/if}
			<div class="top">
				<p class="name">{now.title}</p>
				<IconButton icon="x" label={t('frame.theater.closeWord', { word })} variant="glass" size="lg" tipSide="below-end" onclick={() => void close()} />
			</div>
			<div class="bar">
				<IconButton icon={paused ? 'play' : 'pause'} label={paused ? t('frame.theater.play') : t('frame.theater.pause')} variant="glass" size="lg" tipSide="above" onclick={togglePlay} />
				<span class="t">{clock(time)}</span>
				<input class="seek" type="range" min="0" max={Math.max(duration, 1)} step="0.1" value={time} aria-label={t('frame.theater.seek')} aria-valuetext={t('frame.theater.of', { time: clock(time), total: clock(duration) })} oninput={seek} style:--p="{duration ? (time / duration) * 100 : 0}%" />
				<span class="t">{clock(duration)}</span>
				<IconButton icon={muted ? 'soundOff' : 'sound'} label={muted ? t('frame.theater.soundOn') : t('frame.theater.soundOff')} variant="glass" size="lg" tipSide="above" onclick={toggleSound} />
			</div>
		</div>
	{/if}
</dialog>

<style>
	:global(html.scroll-held) { overflow: hidden; padding-right: var(--held-scrollbar, 0px); }
	.ui-theater { position: fixed; inset: 0; width: auto; height: auto; max-width: none; max-height: none; margin: 0; padding: 0; border: 0; background: transparent; color: var(--on-art); overflow: hidden; }
	.ui-theater::backdrop { background: var(--on-art-scrim); }
	/* Its own grow and fades replace the app's dialog entrance, which faded the whole player in. */
	.ui-theater[open], .ui-theater[open]::backdrop { animation: none; }
	.ui-theater:fullscreen { width: 100%; height: 100%; }
	.stage { position: absolute; inset: 0; background: #000; overflow: hidden; container-type: size; transform-origin: 0 0; }
	.pic { position: absolute; inset: 0; transform-origin: 0 0; }
	.still { position: absolute; inset: 0; z-index: 1; pointer-events: none; transition: opacity var(--dur-fast) linear; }
	.still.off { opacity: 0; }
	.still :global(canvas) { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: contain; }
	video, iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; object-fit: contain; background: #000; }
	iframe { pointer-events: none; }
	.gone { position: absolute; inset: 0; z-index: 3; display: grid; place-content: center; justify-items: center; gap: 16px; padding: 24px; background: #0b0b0c; color: var(--on-art); text-align: center; }
	.gone p { max-width: 32ch; margin: 0; font: var(--t-title3); text-wrap: balance; }
	.gone-close { min-height: 44px; padding: 0 22px; border: 0; border-radius: var(--r-pill); background: #fff; color: #000; font: var(--t-headline); cursor: pointer; }
	/* A click on the picture only brings the controls back (pause is the button or Space); focus rings show for the keyboard only. */
	dialog:focus, dialog :focus:not(:focus-visible) { outline: none; }
	.catch { position: absolute; inset: 0; border: 0; background: transparent; padding: 0; }
	.top, .bar { position: absolute; left: 0; right: 0; z-index: 2; display: flex; align-items: center; gap: 12px; padding: 20px clamp(16px, 3vw, 40px); transition: opacity var(--dur) var(--ease-out), transform var(--dur) var(--ease-out); }
	.top { top: 0; justify-content: space-between; background: linear-gradient(to bottom, var(--on-art-scrim-mid), var(--on-art-scrim-none)); }
	.bar { bottom: 0; padding-bottom: calc(24px + env(safe-area-inset-bottom, 0px)); overflow: visible; background: linear-gradient(to top, var(--on-art-scrim), var(--on-art-scrim-none)); }
	.ui-theater:not(.chrome) .top, .ui-theater:not(.chrome) .bar { opacity: 0; pointer-events: none; transition-duration: 150ms; }
	.idle .top { opacity: 0; transform: translateY(-8px); }
	.idle .bar { opacity: 0; transform: translateY(8px); }
	.idle { cursor: none; }
	.name { margin: 0; font: var(--t-title3); letter-spacing: var(--t-title3-track); text-shadow: 0 1px 8px var(--on-art-scrim-mid); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.t { font: var(--t-subhead); font-variant-numeric: tabular-nums; color: var(--on-art-soft); min-width: 40px; text-align: center; }
	.seek { flex: 1; min-width: 0; height: 28px; appearance: none; -webkit-appearance: none; background: transparent; cursor: pointer; }
	.seek::-webkit-slider-runnable-track { height: 4px; border-radius: 2px; background: linear-gradient(to right, var(--on-art) var(--p), var(--on-art-fill-hover) var(--p)); }
	.seek::-moz-range-track { height: 4px; border-radius: 2px; background: linear-gradient(to right, var(--on-art) var(--p), var(--on-art-fill-hover) var(--p)); }
	.seek::-webkit-slider-thumb { -webkit-appearance: none; width: 14px; height: 14px; margin-top: -5px; border-radius: 50%; background: var(--on-art); box-shadow: 0 1px 4px var(--on-art-scrim-mid); transition: transform var(--dur-fast) var(--ease-out); }
	.seek::-moz-range-thumb { width: 14px; height: 14px; border: 0; border-radius: 50%; background: var(--on-art); }
	.seek:hover::-webkit-slider-thumb { transform: scale(1.2); }
	.seek:focus-visible { outline: none; }
	.seek:focus-visible::-webkit-slider-thumb { box-shadow: 0 0 0 3px var(--on-art-fill-strong); }
	@media (max-width: 767px) { .t { display: none; } .bar { gap: 8px; } }
	@media (prefers-reduced-motion: reduce) { .top, .bar { transition: opacity var(--dur-fast) linear; } .idle .top, .idle .bar { transform: none; } }
</style>
