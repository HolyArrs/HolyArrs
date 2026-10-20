<script lang="ts">
	import { onDestroy } from 'svelte';
	import IconButton from './IconButton.svelte';
	import { claimPlayer, embedUrl, setSoundOn, soundOn } from '../trailer.ts';
	import type { TrailerSource } from '../autoplay.svelte.ts';

	// A muted trailer over the hero or hover card art. Tries each source in
	// order (our own <video> on the direct stream, then the privacy-enhanced
	// YouTube embed) and fades in only once a picture is actually playing,
	// so a refused or blocked one never shows. Only one plays at a time.
	let {
		sources,
		title,
		repeat = false,
		controls = true,
		onfail,
		onended,
		onplaying
	}: {
		sources: TrailerSource[];
		title: string;
		/** Loop (hover cards); the hero ends and shows its pictures again. */
		repeat?: boolean;
		controls?: boolean;
		/** Every source failed: stay on the pictures. */
		onfail?: () => void;
		onended?: () => void;
		onplaying?: () => void;
	} = $props();

	let at = $state(0);
	let shown = $state(false);
	// Trailers start muted; turning the sound on is remembered for this
	// browser session, and later trailers then unmute once they play.
	let muted = $state(true);
	let run = $state(0);
	let done = $state(false);
	let video = $state<HTMLVideoElement>();
	let frame = $state<HTMLIFrameElement>();
	const src = $derived(sources[at]);
	const origin = typeof location !== 'undefined' ? location.origin : '';

	let release: (() => void) | null = null;
	let guard: ReturnType<typeof setTimeout> | null = null;
	let stopped = false;

	function stop() {
		stopped = true;
		shown = false;
		video?.pause();
		at = sources.length;
		clearGuard();
		release?.();
		release = null;
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
		return !(src?.kind === 'direct' && !src.audio);
	}

	// Unmuting without a recent click may pause the video; then it stays muted.
	function unmute() {
		muted = false;
		if (video) {
			const v = video;
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
		const state = d.event === 'onStateChange' ? d.info : d.event === 'infoDelivery' ? (d.info as { playerState?: number })?.playerState : undefined;
		if (d.event === 'onError') next();
		else if (state === 1 && !shown) started();
		else if (state === 0) ended();
	}
	function frameLoaded() {
		frame?.contentWindow?.postMessage(JSON.stringify({ event: 'listening', id: 'holyarrs', channel: 'widget' }), 'https://www.youtube-nocookie.com');
	}

	function toggleMute(e?: MouseEvent) {
		// The hover card's art is a link: the button must not follow it.
		e?.preventDefault();
		e?.stopPropagation();
		muted = !muted;
		if (video) video.muted = muted;
		else post(muted ? 'mute' : 'unMute');
		setSoundOn(!muted);
	}
	function replay(e?: MouseEvent) {
		e?.preventDefault();
		e?.stopPropagation();
		muted = true;
		stopped = false;
		done = false;
		at = 0;
		run += 1;
	}

	onDestroy(() => {
		stopped = true;
		clearGuard();
		release?.();
	});
</script>

<svelte:window onmessage={onMessage} />

<div class="ui-trailer" class:shown>
	{#key `${at}:${run}`}
		{#if src?.kind === 'direct'}
			<!-- svelte-ignore a11y_media_has_caption -->
			<video
				bind:this={video}
				src={src.url}
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
				tabindex="-1"
				onload={frameLoaded}
			></iframe>
		{/if}
	{/key}
</div>
{#if controls}
	<div class="ctl" class:on={shown} data-testid="trailer-sound">
		{#if shown && hasAudio()}
			<IconButton icon={muted ? 'soundOff' : 'sound'} label={muted ? 'Turn sound on' : 'Turn sound off'} variant="glass" size="md" tipSide="above" aria-pressed={!muted} onclick={toggleMute} />
		{:else if done && sources.length > 0}
			<IconButton icon="replay" label="Play the trailer again" variant="glass" size="md" tipSide="above" onclick={replay} />
		{/if}
	</div>
{/if}

<style>
	.ui-trailer { position: absolute; inset: 0; overflow: hidden; opacity: 0; transition: opacity var(--dur-slow) var(--ease-out); pointer-events: none; container-type: size; }
	.ui-trailer.shown { opacity: 1; }
	video, iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; object-fit: cover; }
	/* The embed is letterboxed 16:9; scale it to cover a wider or taller box. */
	iframe { left: 50%; top: 50%; width: max(100%, 177.78cqh); height: max(100%, 56.25cqw); translate: -50% -50%; }
	.ctl { position: absolute; right: 12px; bottom: 12px; z-index: 2; display: flex; gap: 8px; }
	@media (prefers-reduced-motion: reduce) { .ui-trailer { transition: none; } }
</style>
