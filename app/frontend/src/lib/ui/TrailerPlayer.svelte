<script lang="ts">
	import { t } from '#lib/core/i18n/i18n.svelte.ts';
	// Plays one trailer over a card or hero, trying each source in turn until one works.
	import { onDestroy, untrack } from 'svelte';
	import IconButton from './IconButton.svelte';
	import { claimPlayer, embedUrl, setSoundOn, soundOn } from '../core/trailer/trailer.ts';
	import { adoptWarm, hasSound, hasWarm, nativeHLS, onUnmute, type TrailerSource } from '../core/trailer/autoplay.svelte.ts';
	import type { TheaterHandoff } from '../core/trailer/theater.svelte.ts';
	import { attachHLS, screenTarget } from '../core/trailer/hls-player.ts';

	let {
		sources,
		title,
		repeat = false,
		controls = true,
		pausable = false,
		onfail,
		onended,
		onplaying,
		onpause,
		onexpand,
		start = 0,
		sharp = false,
		corner = false,
		top = false
	}: {
		corner?: boolean;
		top?: boolean;
		sources: TrailerSource[];
		title: string;
		repeat?: boolean;
		controls?: boolean;
		pausable?: boolean;
		onfail?: () => void;
		onended?: () => void;
		onplaying?: () => void;
		onpause?: (paused: boolean) => void;
		onexpand?: (o: TheaterHandoff) => void;
		start?: number;
		sharp?: boolean;
	} = $props();

	const target = () => screenTarget('cover');
	// Read once so later prop changes do not seek a playing trailer.
	const startAt = untrack(() => start);
	// Adds a small on-screen log when the page url has trailerdebug.
	const debugOn = typeof location !== 'undefined' && location.search.includes('trailerdebug');
	const origin = typeof location !== 'undefined' ? location.origin : '';

	let seeded = false;
	let embedTime = 0;
	let release: (() => void) | null = null;
	// Timer that moves to the next source when a start never arrives.
	let guard: ReturnType<typeof setTimeout> | null = null;
	let stopped = false;

	// Stream cleanups for the picture video and the sound video.
	let detach: (() => void) | null = null;
	let upDetach: (() => void) | null = null;
	// Time of the last control press, so the same click does not also expand.
	let ctlAt = 0;

	let at = $state(0);
	let shown = $state(false);
	let muted = $state(true);
	let run = $state(0);
	let done = $state(false);
	let paused = $state(false);
	let video = $state<HTMLVideoElement>();
	let frame = $state<HTMLIFrameElement>();
	let box = $state<HTMLElement>();
	// Second video that carries sound while the muted picture video keeps playing.
	let up = $state<{ url: string; start: number } | null>(null);
	let upVideo = $state<HTMLVideoElement>();
	let swapped = $state(false);
	let dbgLines = $state<string[]>([]);
	let ctlBox = $state<HTMLElement>();
	let expandBtn = $state<HTMLButtonElement>();
	let heard = $state(false);

	const src = $derived(sources[at]);

	$effect(() => {
		const v = video;
		if (!v || src?.kind !== 'pass') return;
		let gone = false;
		attachHLS(v, src.url, nativeHLS(), seeded ? 0 : ((seeded = true), startAt > 0.5 ? startAt : 0), target())
			.then((d) => (gone ? d() : (detach = d)))
			.catch(() => !gone && next());
		return () => {
			gone = true;
			detach?.();
			detach = null;
		};
	});

	// Starts the sound stream, waits until it is in step with the picture, then swaps them.
	$effect(() => {
		const v = upVideo;
		const u = up;
		if (!v || !u) {
			if (u) dbg('sound stream waiting for its video element');
			return;
		}
		dbg('sound stream: attaching');
		let gone = false;
		let tries = 0;
		let asked = 0;
		const old = video;
		const go = () => {
			if (gone || !old || swapped || asked) return;
			if (old.currentTime > v.currentTime + 0.05) {
				dbg(`picture ahead ${(old.currentTime - v.currentTime).toFixed(2)}s, try ${tries + 1}`);
				if (++tries <= 3) {
					v.currentTime = old.currentTime + 0.8;
					return;
				}
				v.currentTime = old.currentTime;
				asked = performance.now();
				v.muted = false;
				dbg('joining sound stream at picture time');
				v.play().catch((e) => fail('play refused: ' + e?.name));
				return;
			}
			const wait = () => {
				if (gone) return;
				if (old.currentTime + 0.02 < v.currentTime && !old.paused && !old.ended) return void requestAnimationFrame(wait);
				asked = performance.now();
				v.muted = false;
				dbg('sound stream ready, playing');
				v.play().catch((e) => fail('play refused: ' + e?.name));
			};
			wait();
		};
		const playing = () => {
			if (gone || swapped) return;
			const el = v.closest('.ui-trailer');
			el?.setAttribute('data-switch-ms', String(Math.round(performance.now() - asked)));
			el?.setAttribute('data-switch-drift-ms', String(Math.round(Math.abs((old?.currentTime ?? 0) - v.currentTime) * 1000)));
			swapped = true;
			dbg('SWAPPED: sound on');
			old?.pause();
		};
		const fail = (why?: unknown) => {
			if (gone) return;
			dbg('FAIL ' + (typeof why === 'string' ? why : (v.error?.message || v.dataset.hlsError || (why as Error)?.message || 'error event')));
			gone = true;
			dropUp();
			muted = true;
		};
		v.addEventListener('canplay', go);
		v.addEventListener('playing', playing);
		v.addEventListener('error', fail);
		attachHLS(v, u.url, nativeHLS(), u.start, target())
			.then((d) => (gone ? d() : (upDetach = d)))
			.catch((e) => fail('attach: ' + (e as Error)?.message));
		return () => {
			gone = true;
			v.removeEventListener('canplay', go);
			v.removeEventListener('playing', playing);
			v.removeEventListener('error', fail);
		};
	});

	// Every new source or replay gets a fresh time limit to start playing.
	$effect(() => {
		void src;
		void run;
		if (src) armGuard();
	});

	// Controls sit above a clickable cover, so clicks are caught first and sent to the right action.
	$effect(() => {
		const take = (e: MouseEvent) => {
			if (expandBtn && (e.target as Element | null)?.closest?.('button.expand') === expandBtn) {
				e.stopImmediatePropagation();
				if (corner) ctlAt = 0;
				return expand(e);
			}
			const b = (e.target as Element | null)?.closest?.('button[data-act]');
			if (!b || !ctlBox?.contains(b)) return;
			e.preventDefault();
			e.stopImmediatePropagation();
			ctlAt = Date.now();
			const act = b.getAttribute('data-act');
			if (act === 'sound') toggleMute();
			else if (act === 'pause') togglePause();
			else if (act === 'replay') replay();
			else if (act === 'expand') ((ctlAt = 0), expand(e));
		};
		window.addEventListener('click', take, true);
		return () => window.removeEventListener('click', take, true);
	});
	// The browser can mute on its own, so the real sound state is polled.
	$effect(() => {
		if (!shown) return;
		const read = () => {
			const lv = live();
			heard = src?.kind === 'embed' ? !muted : !!lv && !lv.muted;
		};
		read();
		const t = setInterval(read, 300);
		return () => clearInterval(t);
	});
	// Gives up on the sound stream if it has not taken over in 10 seconds.
	$effect(() => {
		if (!up || swapped) return;
		const t = setTimeout(() => {
			if (up && !swapped) {
				dbg('sound stream timed out');
				dropUp();
				muted = true;
			}
		}, 10000);
		return () => clearTimeout(t);
	});

	// Jumps to the start time once the length is known, unless that is near the end.
	function seed(e: Event) {
		const v = e.currentTarget as HTMLVideoElement;
		if (seeded || startAt <= 0.5) return;
		seeded = true;
		if (startAt < (v.duration || Infinity) - 1) v.currentTime = startAt;
	}
	// Reuses a video that was already started in the background so playback begins at once.
	function takeWarm(node: HTMLVideoElement) {
		return untrack(() => takeWarmNow(node));
	}
	function takeWarmNow(node: HTMLVideoElement) {
		const url = src?.kind === 'direct' ? src.url : '';
		const w = repeat && url ? adoptWarm(url) : null;
		if (!w) return;
		w.className = node.className;
		w.removeAttribute('style');
		w.removeAttribute('aria-hidden');
		w.setAttribute('aria-label', node.getAttribute('aria-label') ?? '');
		w.disablePictureInPicture = true;
		node.replaceWith(w);
		video = w;
		w.addEventListener('playing', started);
		w.addEventListener('error', next);
		if (!w.paused && w.readyState >= 3) started();
		else void w.play().catch(() => {});
		return () => {
			w.removeEventListener('playing', started);
			w.removeEventListener('error', next);
			w.pause();
			w.removeAttribute('src');
			w.load();
			w.remove();
		};
	}

	function stop() {
		stopped = true;
		shown = false;
		video?.pause();
		dropUp();
		at = sources.length;
		clearGuard();
		release?.();
		release = null;
		onended?.();
	}

	function clearGuard() {
		if (guard) clearTimeout(guard);
		guard = null;
	}

	function armGuard() {
		clearGuard();
		// Eight seconds is long enough for a slow start, short enough to try the next source.
		guard = setTimeout(next, 8000);
	}

	function next() {
		clearGuard();
		dropUp();
		shown = false;
		if (at + 1 < sources.length) at += 1;
		else {
			at = sources.length;
			release?.();
			release = null;
			if (!stopped) onfail?.();
			done = true;
		}
	}

	function started() {
		clearGuard();
		if (stopped) return;
		release ??= claimPlayer(stop);
		const first = !shown;
		shown = true;
		onplaying?.();
		if (first && muted && soundOn() && hasAudio()) unmute();
	}

	function hasAudio() {
		return hasSound(src);
	}

	// Direct sources switch to the sound stream; touch devices on native HLS swap the source in place.
	function unmute() {
		if (src?.kind === 'direct' && !swapped && onUnmute(src) === 'switch' && src.pass && !video?.currentSrc.includes('/trailer/stream/')) {
			muted = false;
			if (nativeHLS() && video && matchMedia('(pointer: coarse)').matches) {
				const v = video;
				const t = v.currentTime;
				v.addEventListener('loadedmetadata', () => (v.currentTime = t), { once: true });
				v.src = src.pass;
				v.muted = false;
				void v.play().catch(() => {
					v.muted = muted = true;
				});
				return;
			}
			switchToPass(src.pass);
			return;
		}
		muted = false;
		dbg(`direct unmute (live=${!!live()})`);
		const lv = live();
		if (lv) {
			const v = lv;
			v.muted = false;
			if (v.paused) {
				dbg('video paused by browser on unmute: back to mute');
				v.muted = muted = true;
				void v.play().catch(() => {});
			}
		} else post('unMute');
	}

	function ended() {
		shown = false;
		done = true;
		release?.();
		release = null;
		onended?.();
	}

	function post(func: string, args: unknown[] = []) {
		frame?.contentWindow?.postMessage(JSON.stringify({ event: 'command', func, args }), 'https://www.youtube-nocookie.com');
	}
	// Only messages from our own embed frame are trusted.
	function onMessage(e: MessageEvent) {
		if (e.origin !== 'https://www.youtube-nocookie.com' || !frame || e.source !== frame.contentWindow) return;
		let d: { event?: string; info?: unknown };
		try {
			d = typeof e.data === 'string' ? JSON.parse(e.data) : e.data;
		} catch {
			return;
		}
		const t = (d.info as { currentTime?: unknown } | undefined)?.currentTime;
		if (typeof t === 'number') embedTime = t;
		const state = d.event === 'onStateChange' ? d.info : d.event === 'infoDelivery' ? (d.info as { playerState?: number })?.playerState : undefined;
		if (d.event === 'onError') next();
		else if (state === 1 && !shown) started();
		else if (state === 0) ended();
	}
	function frameLoaded() {
		frame?.contentWindow?.postMessage(JSON.stringify({ event: 'listening', id: 'holyarrs', channel: 'widget' }), 'https://www.youtube-nocookie.com');
		if (startAt > 1 && !seeded) {
			seeded = true;
			post('seekTo', [startAt, true]);
		}
	}

	function toggleMute(e?: MouseEvent) {
		ctlAt = Date.now();
		e?.preventDefault();
		e?.stopPropagation();
		if (!heard && !muted && up && !swapped) return;
		if (!heard) muted = true;
		dbg(`sound button: ${muted ? 'unmute' : 'mute'} (src=${src?.kind}, swapped=${swapped})`);
		setSoundOn(muted);
		if (muted) return unmute();
		muted = true;
		dropUpIfWaiting();
		const lv = live();
		if (lv) lv.muted = true;
		else post('mute');
	}
	function togglePause(e?: MouseEvent) {
		e?.preventDefault();
		e?.stopPropagation();
		paused = !paused;
		onpause?.(paused);
		const lv = live();
		if (lv) {
			if (paused) lv.pause();
			else void lv.play().catch(() => {});
		} else post(paused ? 'pauseVideo' : 'playVideo');
	}
	function dropUpIfWaiting() {
		if (up && !swapped) dropUp();
	}
	function replay(e?: MouseEvent) {
		e?.preventDefault();
		e?.stopPropagation();
		dropUp();
		muted = true;
		stopped = false;
		done = false;
		paused = false;
		at = 0;
		run += 1;
	}

	// Hands the current time, size and a still frame to the full screen player.
	function expand(e: MouseEvent) {
		if (Date.now() - ctlAt < 600) return;
		e.preventDefault();
		e.stopPropagation();
		if (!src || !box || !onexpand) return;
		const v = live();
		const time = v ? v.currentTime : embedTime;
		const rect = box.getBoundingClientRect();
		const source: TrailerSource = swapped && up ? { kind: 'pass', url: up.url, key: src.key } : src.kind === 'direct' && !src.audio && src.pass ? { kind: 'pass', url: src.pass, key: src.key } : src;
		const still = grab(v);
		const quiet = !heard;
		stop();
		onexpand({ source, time, rect, muted: quiet, still });
	}

	onDestroy(() => {
		stopped = true;
		clearGuard();
		dropUp();
		release?.();
	});

	// Copies the current frame so full screen can show it while it loads; may fail for cross-origin video.
	function grab(v: HTMLVideoElement | undefined | null): HTMLCanvasElement | null {
		if (!v || !v.videoWidth || !v.videoHeight) return null;
		const c = document.createElement('canvas');
		c.width = v.videoWidth;
		c.height = v.videoHeight;
		try {
			c.getContext('2d')?.drawImage(v, 0, 0);
		} catch {
			return null;
		}
		return c;
	}

	function dropUp() {
		if (up) dbg('drop sound stream');
		upDetach?.();
		upDetach = null;
		up = null;
		swapped = false;
	}

	function dbg(m: string) {
		if (!debugOn) return;
		untrack(() => (dbgLines = [...dbgLines.slice(-11), `${(performance.now() / 1000).toFixed(1)}s ${m}`]));
	}
	// Starts the sound stream a little ahead of the picture to cover its load time.
	function switchToPass(url: string) {
		dbg(`switch to sound stream (video=${!!video} up=${!!up})`);
		if (!video || up) return;
		up = { url, start: video.currentTime + 1.2 };
	}

	const live = () => (swapped ? upVideo : video);
</script>

<svelte:window onmessage={onMessage} />

<div class="ui-trailer" class:shown class:quick={repeat} bind:this={box}>
	{#key `${at}:${run}`}
		{#if src?.kind === 'direct'}
			<video
				bind:this={video}
				{@attach takeWarm}
				src={repeat && hasWarm(src.url) ? undefined : src.url}
				muted
				autoplay
				playsinline
				loop={repeat}
				preload="auto"
				disablepictureinpicture
				disableremoteplayback
				aria-label={t('ui.trailerPlayer.trailer', { title })}
				onplaying={started}
				onloadedmetadata={seed}
				onerror={next}
				onended={ended}
			></video>
			{#if up}
				<video
					bind:this={upVideo}
					class="up"
					class:swapped
					playsinline
					loop={repeat}
					preload="auto"
					disablepictureinpicture
					disableremoteplayback
					aria-label={t('ui.trailerPlayer.trailer', { title })}
					onended={ended}
				></video>
			{/if}
		{:else if src?.kind === 'pass'}
			<video
				bind:this={video}
				muted
				autoplay
				playsinline
				loop={repeat}
				preload="auto"
				disablepictureinpicture
				disableremoteplayback
				aria-label={t('ui.trailerPlayer.trailer', { title })}
				onplaying={started}
				onerror={next}
				onended={ended}
			></video>
		{:else if src?.kind === 'embed'}
			<iframe
				bind:this={frame}
				src={embedUrl(src.key, origin, repeat)}
				title={t('ui.trailerPlayer.trailer', { title })}
				allow="autoplay; encrypted-media; picture-in-picture"
				referrerpolicy="strict-origin-when-cross-origin"
				sandbox="allow-scripts allow-same-origin allow-presentation"
				tabindex="-1"
				onload={frameLoaded}
			></iframe>
		{/if}
	{/key}
</div>
{#if onexpand && shown && !corner}
	<button type="button" class="expand" aria-label={t('ui.trailerPlayer.playTheTrailerFull', { title })} bind:this={expandBtn} onclick={expand}></button>
{/if}
{#if controls}
	<div class="ctl" class:on={shown} class:top data-testid="trailer-sound" role="group" aria-label={t('ui.trailerPlayer.trailerControls')} bind:this={ctlBox} onpointerdown={(e) => { ctlAt = Date.now(); e.stopPropagation(); }} onclickcapture={() => (ctlAt = Date.now())}>
		{#if shown && pausable}
			<IconButton icon={paused ? 'play' : 'pause'} label={paused ? t('ui.trailerPlayer.play') : t('ui.trailerPlayer.pause')} variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="pause" />
		{/if}
		{#if shown && hasAudio()}
			<IconButton icon={heard ? 'sound' : 'soundOff'} label={heard ? t('ui.trailerPlayer.soundOff') : t('ui.trailerPlayer.soundOn')} variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="sound" />
		{:else if done && sources.length > 0}
			<IconButton icon="replay" label={t('ui.trailerPlayer.playTheTrailerAgain')} variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="replay" />
		{/if}
		{#if onexpand && shown && !corner}
			<IconButton icon="expand" label={t('ui.trailerPlayer.playTrailerFullScreen')} variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="expand" data-testid="trailer-expand" />
		{/if}
		{#if onexpand && shown && corner}
			<IconButton icon="expand" label={t('ui.trailerPlayer.playTrailerFullScreen')} variant="glass" size="md" tipSide={top ? 'below' : 'above'} class="expand corner" bind:node={expandBtn} data-testid="trailer-expand" />
		{/if}
	</div>
{/if}

{#if debugOn && dbgLines.length}
	<pre class="dbg">{dbgLines.join(String.fromCharCode(10))}</pre>
{/if}

<style>
	/* Debug log only; never shown without the debug flag. */
	.dbg { position: fixed; left: 8px; bottom: 8px; z-index: 99999; max-width: 520px; margin: 0; padding: 8px 10px; font: 12px/1.35 ui-monospace, monospace; color: #9f9; background: rgb(0 0 0 / 85%); border-radius: 8px; white-space: pre-wrap; pointer-events: none; }
	.ui-trailer { position: absolute; inset: 0; overflow: hidden; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); pointer-events: none; container-type: size; }
	.ui-trailer.shown { opacity: 1; }
	.ui-trailer.quick { transition-duration: var(--dur-fast); }
	video, iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; object-fit: cover; }
	/* Scales the 16:9 embed so it covers the box like the video does. */
	iframe { left: 50%; top: 50%; width: max(100%, 177.78cqh); height: max(100%, 56.25cqw); translate: -50% -50%; }
	/* The sound video sits hidden above the picture until it takes over. */
	video.up { opacity: 0; z-index: 1; }
	video.up.swapped { opacity: 1; }
	.expand { position: absolute; inset: 0; z-index: 2; padding: 0; border: 0; background: transparent; cursor: pointer; }
	.expand:focus-visible { outline: none; box-shadow: inset 0 0 0 3px var(--on-art); }
	.ctl { position: absolute; right: 12px; bottom: 12px; z-index: 4; display: flex; gap: 8px; }
	.ctl.top { top: 16px; bottom: auto; right: var(--gutter, 24px); }
	@media (max-width: 767px) { .ctl.top { top: 12px; right: 16px; } }
	@media (prefers-reduced-motion: reduce) { .ui-trailer { transition: none; } }
</style>
