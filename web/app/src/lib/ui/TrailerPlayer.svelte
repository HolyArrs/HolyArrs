<script lang="ts">
	import { onDestroy, untrack } from 'svelte';
	import IconButton from './IconButton.svelte';
	import { claimPlayer, embedUrl, setSoundOn, soundOn } from '../trailer.ts';
	import { adoptWarm, hasSound, hasWarm, nativeHLS, onUnmute, type TrailerSource } from '../autoplay.svelte.ts';
	import { attachHLS } from '../hls-player.ts';

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
		onexpand
	}: {
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
		/** Clicking the playing trailer opens it full screen: the source, the time played and where it sits. */
		onexpand?: (o: { source: TrailerSource; time: number; rect: DOMRect }) => void;
	} = $props();

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
		attachHLS(v, src.url, nativeHLS())
			.then((d) => (gone ? d() : (detach = d)))
			.catch(() => !gone && next());
		return () => {
			gone = true;
			detach?.();
			detach = null;
		};
	});

	function dropUp() {
		upDetach?.();
		upDetach = null;
		up = null;
		swapped = false;
	}

	// switchToPass starts the relayed stream a little ahead of the direct
	// one, waits for the direct one to reach it, then plays it with sound.
	function switchToPass(url: string) {
		if (!video || up) return;
		up = { url, start: video.currentTime + 1.2 };
	}

	$effect(() => {
		const v = upVideo;
		const u = up;
		if (!v || !u) return;
		let gone = false;
		let tries = 0;
		let asked = 0;
		const old = video;
		const go = () => {
			if (gone || !old || swapped || asked) return;
			// Too late (the relay took longer than the lead): aim further ahead.
			if (old.currentTime > v.currentTime + 0.05) {
				if (++tries > 3) return fail();
				v.currentTime = old.currentTime + 0.8;
				return;
			}
			const wait = () => {
				if (gone) return;
				if (old.currentTime + 0.02 < v.currentTime && !old.paused && !old.ended) return void requestAnimationFrame(wait);
				asked = performance.now();
				v.muted = false;
				v.play().catch(fail);
			};
			wait();
		};
		const playing = () => {
			if (gone || swapped) return;
			const el = v.closest('.ui-trailer');
			el?.setAttribute('data-switch-ms', String(Math.round(performance.now() - asked)));
			el?.setAttribute('data-switch-drift-ms', String(Math.round(Math.abs((old?.currentTime ?? 0) - v.currentTime) * 1000)));
			swapped = true;
			old?.pause();
		};
		const fail = () => {
			if (gone) return;
			gone = true;
			dropUp();
			muted = true;
			setSoundOn(false);
		};
		v.addEventListener('canplay', go);
		v.addEventListener('playing', playing);
		v.addEventListener('error', fail);
		attachHLS(v, u.url, nativeHLS(), u.start)
			.then((d) => (gone ? d() : (upDetach = d)))
			.catch(fail);
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
		const lv = live();
		if (lv) {
			const v = lv;
			v.muted = false;
			if (v.paused) {
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
	}

	let ctlAt = 0;
	let ctlBox = $state<HTMLElement>();
	// A press on the controls may swap the button (sound on/off) before the
	// click lands; that click must never fall through to the card's link.
	function guardClick() {
		const stop = (e: MouseEvent) => {
			window.removeEventListener('click', stop, true);
			if (ctlBox?.contains(e.target as Node) && (e.target as Element).closest('button')) return;
			e.preventDefault();
			e.stopPropagation();
		};
		window.addEventListener('click', stop, true);
		setTimeout(() => window.removeEventListener('click', stop, true), 800);
	}
	function toggleMute(e?: MouseEvent) {
		ctlAt = Date.now();
		// The hover card's art is a link: the button must not follow it.
		e?.preventDefault();
		e?.stopPropagation();
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
		stop();
		onexpand({ source, time, rect });
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
{#if onexpand && shown}
	<button type="button" class="expand" aria-label="Play the {title} trailer full screen" onclick={expand}></button>
{/if}
{#if controls}
	<div class="ctl" class:on={shown} data-testid="trailer-sound" role="group" aria-label="Trailer controls" bind:this={ctlBox} onpointerdown={(e) => { ctlAt = Date.now(); e.stopPropagation(); guardClick(); }} onclickcapture={() => (ctlAt = Date.now())}>
		{#if shown && pausable}
			<IconButton icon={paused ? 'play' : 'pause'} label={paused ? 'Play the trailer' : 'Pause the trailer'} variant="glass" size="md" tipSide="above" onclick={togglePause} />
		{/if}
		{#if shown && hasAudio()}
			<IconButton icon={muted ? 'soundOff' : 'sound'} label={muted ? 'Turn sound on' : 'Turn sound off'} variant="glass" size="md" tipSide="above" onclick={toggleMute} />
		{:else if done && sources.length > 0}
			<IconButton icon="replay" label="Play the trailer again" variant="glass" size="md" tipSide="above" onclick={replay} />
		{/if}
	</div>
{/if}

<style>
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
	.expand { position: absolute; inset: 0; z-index: 2; padding: 0; border: 0; background: transparent; cursor: zoom-in; }
	.expand:focus-visible { outline: none; box-shadow: inset 0 0 0 3px var(--on-art); }
	.ctl { position: absolute; right: 12px; bottom: 12px; z-index: 4; display: flex; gap: 8px; }
	@media (prefers-reduced-motion: reduce) { .ui-trailer { transition: none; } }
</style>
