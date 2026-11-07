<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import IconButton from './IconButton.svelte';
	import { claimPlayer, embedUrl, setSoundOn, soundOn } from '../trailer.ts';
	import { adoptWarm, hasSound, hasWarm, nativeHLS, onUnmute, type TrailerSource } from '../autoplay.svelte.ts';
	import type { TheaterHandoff } from '../theater.svelte.ts';
	import { attachHLS, screenTarget } from '../hls-player.ts';

	// A muted trailer over the hero or hover card art. Tries each source in
	// order (our own <video> on the direct stream, then the privacy-enhanced
	// YouTube embed) and fades in only once a picture is actually playing,
	// so a refused or blocked one never shows. Only one plays at a time.
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
		/** Full screen from a small expand button in the corner (hover cards); a click on the picture is left to the card's link. */
		corner?: boolean;
		/** Big heroes: the buttons sit top right with their tips below, the same spot on every page. */
		top?: boolean;
		sources: TrailerSource[];
		title: string;
		/** Loop (hover cards); the hero ends and shows its pictures again. */
		repeat?: boolean;
		controls?: boolean;
		/** A pause/play button beside the sound (the hero, which autoplays for longer than 5 s). */
		pausable?: boolean;
		/** Every source failed: stay on the pictures. */
		onfail?: () => void;
		onended?: () => void;
		onplaying?: () => void;
		/** The pause button was pressed (true) or play pressed again (false). */
		onpause?: (paused: boolean) => void;
		/** Clicking the playing trailer opens it full screen: the source, the time played and where it sits. */
		onexpand?: (o: TheaterHandoff) => void;
		/** Seconds to start from (a hero carrying on where the full-screen player left off). */
		start?: number;
		/** Fetch a picture as tall as the player needs, up to 1080p (the hero); else at most 720p (hover cards). */
		sharp?: boolean;
	} = $props();
	/** target is the tallest stream level worth fetching here. */
	const target = () => screenTarget('cover');
	/** Read once: a later change never seeks a running player. */
	const startAt = untrack(() => start);
	let seeded = false;
	/** seed starts the first picture at startAt (direct streams; pass-through starts there itself). */
	function seed(e: Event) {
		const v = e.currentTarget as HTMLVideoElement;
		if (seeded || startAt <= 0.5) return;
		seeded = true;
		if (startAt < (v.duration || Infinity) - 1) v.currentTime = startAt;
	}
	/** grab copies the playing frame, so the full-screen player never opens on black. */
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

	let at = $state(0);
	let shown = $state(false);
	// Trailers start muted; turning the sound on is remembered for this
	// browser session, and later trailers then unmute once they play.
	let muted = $state(true);
	let run = $state(0);
	let done = $state(false);
	let paused = $state(false);
	let video = $state<HTMLVideoElement>();
	let frame = $state<HTMLIFrameElement>();
	let box = $state<HTMLElement>();
	/** Seconds played (the embed reports it in its info messages). */
	let embedTime = 0;
	const src = $derived(sources[at]);
	const origin = typeof location !== 'undefined' ? location.origin : '';

	let release: (() => void) | null = null;
	let guard: ReturnType<typeof setTimeout> | null = null;
	let stopped = false;

	// Pass-through: the relayed stream on this source's <video> (kind pass),
	// or on a second <video> the direct one hands over to when sound is
	// turned on (kind direct with pass). The direct one stays visible and
	// playing until the second is playing at the same moment.
	let detach: (() => void) | null = null;
	let up = $state<{ url: string; start: number } | null>(null);
	let upVideo = $state<HTMLVideoElement>();
	let swapped = $state(false);
	let upDetach: (() => void) | null = null;
	const live = () => (swapped ? upVideo : video);

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

	function dropUp() {
		if (up) dbg('drop sound stream');
		upDetach?.();
		upDetach = null;
		up = null;
		swapped = false;
	}

	// switchToPass starts the relayed stream a little ahead of the direct
	// one, waits for the direct one to reach it, then plays it with sound.
	// ?trailerdebug=1 shows each step of the sound switch on screen.
	const debugOn = typeof location !== 'undefined' && location.search.includes('trailerdebug');
	let dbgLines = $state<string[]>([]);
	function dbg(m: string) {
		if (!debugOn) return;
		// untrack: dbg runs inside effects; reading dbgLines there would loop.
		untrack(() => (dbgLines = [...dbgLines.slice(-11), `${(performance.now() / 1000).toFixed(1)}s ${m}`]));
	}
	function switchToPass(url: string) {
		dbg(`switch to sound stream (video=${!!video} up=${!!up})`);
		if (!video || up) return;
		up = { url, start: video.currentTime + 1.2 };
	}

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
			// Too late (the relay took longer than the lead): aim further ahead.
			// After a few misses, take the sound stream where the picture is and
			// start it now (a tiny hitch beats falling back to mute).
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
			// This trailer stays quiet; the person's sound choice is kept for the next one.
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

	// Hover cards: take over the element warmed at hover start (already
	// playing, muted, unseen) instead of starting a new one.
	// Untracked: the attachment must not run again (and drop the element) when state changes.
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
		// The rendered element got no address (warm below), so it is simply swapped out.
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
		// Another player took over: for the hero this ends the trailer, so its words come back.
		onended?.();
	}

	function clearGuard() {
		if (guard) clearTimeout(guard);
		guard = null;
	}

	// Nothing playing within 8 s counts as a failure of that source.
	function armGuard() {
		clearGuard();
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

	// Unmuting without a recent click may pause the video; then it stays muted.
	// A picture-only stream switches to pass-through instead.
	function unmute() {
		if (src?.kind === 'direct' && !swapped && onUnmute(src) === 'switch' && src.pass && !video?.currentSrc.includes('/trailer/stream/')) {
			muted = false;
			// Native HLS (iOS Safari): unmuted play must start inside the tap,
			// so the same element takes the relayed stream right away.
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

	$effect(() => {
		void src;
		void run;
		if (src) armGuard();
	});

	// The embed talks through postMessage (enablejsapi): ready, state, errors.
	function post(func: string, args: unknown[] = []) {
		frame?.contentWindow?.postMessage(JSON.stringify({ event: 'command', func, args }), 'https://www.youtube-nocookie.com');
	}
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

	let ctlAt = 0;
	let ctlBox = $state<HTMLElement>();
	let expandBtn = $state<HTMLButtonElement>();
	// The controls sit inside a card's link: the app's link router sees a
	// click before the button's own handler could cancel it. So clicks on the
	// controls are taken first (window capture), run here, and go no further.
	$effect(() => {
		const take = (e: MouseEvent) => {
			// The full-screen click too: the card's link must not open the title page under the player.
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
	// heard is what the person actually hears: the playing element's own
	// muted flag (muted is the wish, which runs ahead while the sound stream
	// starts or after a browser refused sound). The button shows heard.
	let heard = $state(false);
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
	// A sound stream that has not started within 10 s gives up (quiet beats a wrong button).
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
	function toggleMute(e?: MouseEvent) {
		ctlAt = Date.now();
		// The hover card's art is a link: the button must not follow it.
		e?.preventDefault();
		e?.stopPropagation();
		// Sound is on its way: a second press does not cancel it.
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
	// Sound off before the switch finished: stay on the direct stream.
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

	function expand(e: MouseEvent) {
		// A click on the sound or pause buttons never opens the full-screen player.
		if (Date.now() - ctlAt < 600) return;
		e.preventDefault();
		e.stopPropagation();
		if (!src || !box || !onexpand) return;
		const v = live();
		const time = v ? v.currentTime : embedTime;
		const rect = box.getBoundingClientRect();
		// Full screen plays with sound: a picture-only direct trailer goes over pass-through.
		const source: TrailerSource = swapped && up ? { kind: 'pass', url: up.url, key: src.key } : src.kind === 'direct' && !src.audio && src.pass ? { kind: 'pass', url: src.pass, key: src.key } : src;
		// The sound and the frame on screen go with it.
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
</script>

<svelte:window onmessage={onMessage} />

<div class="ui-trailer" class:shown class:quick={repeat} bind:this={box}>
	{#key `${at}:${run}`}
		{#if src?.kind === 'direct'}
			<!-- svelte-ignore a11y_media_has_caption -->
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
				aria-label="{title} trailer"
				onplaying={started}
				onloadedmetadata={seed}
				onerror={next}
				onended={ended}
			></video>
			{#if up}
				<!-- svelte-ignore a11y_media_has_caption -->
				<video
					bind:this={upVideo}
					class="up"
					class:swapped
					playsinline
					loop={repeat}
					preload="auto"
					disablepictureinpicture
					disableremoteplayback
					aria-label="{title} trailer"
					onended={ended}
				></video>
			{/if}
		{:else if src?.kind === 'pass'}
			<!-- svelte-ignore a11y_media_has_caption -->
			<video
				bind:this={video}
				muted
				autoplay
				playsinline
				loop={repeat}
				preload="auto"
				disablepictureinpicture
				disableremoteplayback
				aria-label="{title} trailer"
				onplaying={started}
				onerror={next}
				onended={ended}
			></video>
		{:else if src?.kind === 'embed'}
			<iframe
				bind:this={frame}
				src={embedUrl(src.key, origin, repeat)}
				title="{title} trailer"
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
	<button type="button" class="expand" aria-label="Play the {title} trailer full screen" bind:this={expandBtn} onclick={expand}></button>
{/if}
{#if controls}
	<div class="ctl" class:on={shown} class:top data-testid="trailer-sound" role="group" aria-label="Trailer controls" bind:this={ctlBox} onpointerdown={(e) => { ctlAt = Date.now(); e.stopPropagation(); }} onclickcapture={() => (ctlAt = Date.now())}>
		{#if shown && pausable}
			<IconButton icon={paused ? 'play' : 'pause'} label={paused ? 'Play the trailer' : 'Pause the trailer'} variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="pause" />
		{/if}
		{#if shown && hasAudio()}
			<IconButton icon={heard ? 'sound' : 'soundOff'} label={heard ? 'Turn sound off' : 'Turn sound on'} variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="sound" />
		{:else if done && sources.length > 0}
			<IconButton icon="replay" label="Play the trailer again" variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="replay" />
		{/if}
		{#if onexpand && shown && !corner}
			<!-- Heroes: a click on the picture opens full screen too; this is the visible way in. -->
			<IconButton icon="expand" label="Play trailer full screen" variant="glass" size="md" tipSide={top ? 'below' : 'above'} data-act="expand" data-testid="trailer-expand" />
		{/if}
		{#if onexpand && shown && corner}
			<IconButton icon="expand" label="Play trailer full screen" variant="glass" size="md" tipSide={top ? 'below' : 'above'} class="expand corner" bind:node={expandBtn} data-testid="trailer-expand" />
		{/if}
	</div>
{/if}

{#if debugOn && dbgLines.length}
	<pre class="dbg">{dbgLines.join(String.fromCharCode(10))}</pre>
{/if}

<style>
	.dbg { position: fixed; left: 8px; bottom: 8px; z-index: 99999; max-width: 520px; margin: 0; padding: 8px 10px; font: 12px/1.35 ui-monospace, monospace; color: #9f9; background: rgb(0 0 0 / 85%); border-radius: 8px; white-space: pre-wrap; pointer-events: none; }
	.ui-trailer { position: absolute; inset: 0; overflow: hidden; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); pointer-events: none; container-type: size; }
	.ui-trailer.shown { opacity: 1; }
	/* Hover cards: the picture is already moving; a short fade. */
	.ui-trailer.quick { transition-duration: var(--dur-fast); }
	video, iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; object-fit: cover; }
	/* The embed is letterboxed 16:9; scale it to cover a wider or taller box. */
	iframe { left: 50%; top: 50%; width: max(100%, 177.78cqh); height: max(100%, 56.25cqw); translate: -50% -50%; }
	/* The pass-through video waits unseen until it plays, then covers the direct one. */
	video.up { opacity: 0; z-index: 1; }
	video.up.swapped { opacity: 1; }
	.expand { position: absolute; inset: 0; z-index: 2; padding: 0; border: 0; background: transparent; cursor: pointer; }
	.expand:focus-visible { outline: none; box-shadow: inset 0 0 0 3px var(--on-art); }
	.ctl { position: absolute; right: 12px; bottom: 12px; z-index: 4; display: flex; gap: 8px; }
	.ctl.top { top: 16px; bottom: auto; right: var(--gutter, 24px); }
	@media (max-width: 767px) { .ctl.top { top: 12px; right: 16px; } }
	@media (prefers-reduced-motion: reduce) { .ui-trailer { transition: none; } }
</style>
