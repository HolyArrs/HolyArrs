<script lang="ts">
	import { onDestroy } from 'svelte';
	import IconButton from './IconButton.svelte';
	import { closeTheater, holdScroll, theater } from '../theater.svelte.ts';
	import { embedUrl } from '../trailer.ts';
	import { reducedMotion } from '../motion.ts';

	// The full-screen trailer. It grows from the small player's rect to fill
	// the window (and asks the browser for real full screen where allowed),
	// plays with sound from where the small one was, and has our own small
	// controls: play/pause, a seek bar, sound and close. Esc, close or the
	// end of the trailer shrink it away and hand over to the More info panel.
	let dialog = $state<HTMLDialogElement>();
	let stage = $state<HTMLElement>();
	let video = $state<HTMLVideoElement>();
	let frame = $state<HTMLIFrameElement>();
	let paused = $state(false);
	let muted = $state(false);
	let time = $state(0);
	let duration = $state(0);
	let idle = $state(false);
	let leaving = false;
	let release: (() => void) | null = null;
	let idleTimer: ReturnType<typeof setTimeout> | undefined;
	let tick: ReturnType<typeof setInterval> | undefined;
	const origin = typeof location !== 'undefined' ? location.origin : '';
	const now = $derived(theater.now);
	const src = $derived(now?.source);

	const GROW_MS = 420;

	function fromRect(): string | null {
		const r = now?.rect;
		if (!r || !r.width) return null;
		const W = window.innerWidth;
		const H = window.innerHeight;
		return `translate(${r.left}px, ${r.top}px) scale(${r.width / W}, ${r.height / H})`;
	}

	$effect(() => {
		if (!now || !dialog || dialog.open) return;
		leaving = false;
		paused = false;
		muted = false;
		time = now.time;
		duration = 0;
		release = holdScroll();
		dialog.showModal();
		wake();
		const from = fromRect();
		if (stage && from && !reducedMotion()) {
			stage.animate([{ transform: from, borderRadius: '12px' }, { transform: 'none', borderRadius: '0px' }], { duration: GROW_MS, easing: 'cubic-bezier(.2, .8, .2, 1)' });
		} else if (stage) {
			stage.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 200 });
		}
		// Real full screen where the browser allows it; the window-filling player works without.
		dialog.requestFullscreen?.().catch(() => {});
	});

	function onFullscreen() {
		if (!document.fullscreenElement && dialog?.open && !leaving) void close();
	}

	async function close() {
		if (leaving || !dialog) return;
		leaving = true;
		video?.pause();
		if (document.fullscreenElement) await document.exitFullscreen().catch(() => {});
		const from = fromRect();
		if (stage && !reducedMotion()) {
			const a = stage.animate(from ? [{ transform: 'none', opacity: 1 }, { transform: from, opacity: 0.2 }] : [{ opacity: 1 }, { opacity: 0 }], { duration: from ? 320 : 200, easing: 'cubic-bezier(.4, 0, .6, 1)', fill: 'forwards' });
			await a.finished.catch(() => {});
		}
		dialog.close();
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
		post('unMute');
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
		const info = typeof d.info === 'object' && d.info ? d.info : {};
		if (typeof info.currentTime === 'number') time = info.currentTime;
		if (typeof info.duration === 'number' && info.duration > 0) duration = info.duration;
		if (typeof info.muted === 'boolean') muted = info.muted;
		const state = d.event === 'onStateChange' ? d.info : info.playerState;
		if (state === 1) paused = false;
		else if (state === 2) paused = true;
		else if (state === 0) void close();
	}

	function videoReady() {
		if (!video || !now) return;
		if (now.time > 0.5 && now.time < (video.duration || Infinity)) video.currentTime = now.time;
		video.muted = false;
		void video.play().catch(() => {
			// Sound refused without a fresh click: play muted, the sound button turns it on.
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
	aria-label={now ? `${now.title} trailer` : 'Trailer'}
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
			{#if src.kind === 'direct'}
				<!-- svelte-ignore a11y_media_has_caption -->
				<video
					bind:this={video}
					src={src.url}
					playsinline
					preload="auto"
					disablepictureinpicture
					onloadedmetadata={() => {
						duration = video?.duration ?? 0;
						videoReady();
					}}
					ontimeupdate={() => (time = video?.currentTime ?? 0)}
					onplay={() => (paused = false)}
					onpause={() => {
						paused = true;
						idle = false;
					}}
					onvolumechange={() => (muted = video?.muted ?? false)}
					onended={() => void close()}
					onclick={togglePlay}
				></video>
			{:else}
				<iframe
					bind:this={frame}
					src={embedUrl(src.key, origin, false)}
					title="{now.title} trailer"
					allow="autoplay; encrypted-media; fullscreen"
					referrerpolicy="strict-origin-when-cross-origin"
					tabindex="-1"
					onload={frameLoaded}
				></iframe>
				<button type="button" class="catch" aria-label={paused ? 'Play' : 'Pause'} onclick={togglePlay} tabindex="-1"></button>
			{/if}
			<div class="top">
				<p class="name">{now.title}</p>
				<IconButton icon="x" label="Close the trailer" variant="glass" size="lg" tipSide="below" onclick={() => void close()} />
			</div>
			<div class="bar">
				<IconButton icon={paused ? 'play' : 'pause'} label={paused ? 'Play' : 'Pause'} variant="glass" size="lg" tipSide="above" onclick={togglePlay} />
				<span class="t">{clock(time)}</span>
				<input class="seek" type="range" min="0" max={Math.max(duration, 1)} step="0.1" value={time} aria-label="Seek" aria-valuetext="{clock(time)} of {clock(duration)}" oninput={seek} style:--p="{duration ? (time / duration) * 100 : 0}%" />
				<span class="t">{clock(duration)}</span>
				<IconButton icon={muted ? 'soundOff' : 'sound'} label={muted ? 'Turn sound on' : 'Turn sound off'} variant="glass" size="lg" tipSide="above" aria-pressed={!muted} onclick={toggleSound} />
			</div>
		</div>
	{/if}
</dialog>

<style>
	:global(html.scroll-held) { overflow: hidden; padding-right: var(--held-scrollbar, 0px); }
	.ui-theater { position: fixed; inset: 0; width: 100vw; height: 100dvh; max-width: none; max-height: none; margin: 0; padding: 0; border: 0; background: transparent; color: var(--on-art); overflow: hidden; }
	.ui-theater::backdrop { background: var(--on-art-scrim); }
	.ui-theater:fullscreen { width: 100vw; height: 100vh; }
	.stage { position: absolute; inset: 0; background: #000; transform-origin: 0 0; overflow: hidden; container-type: size; }
	video, iframe { position: absolute; inset: 0; width: 100%; height: 100%; border: 0; object-fit: contain; background: #000; }
	iframe { pointer-events: none; }
	.catch { position: absolute; inset: 0; border: 0; background: transparent; padding: 0; }
	.top, .bar { position: absolute; left: 0; right: 0; z-index: 2; display: flex; align-items: center; gap: 12px; padding: 20px clamp(16px, 3vw, 40px); transition: opacity 400ms var(--ease-out), transform 400ms var(--ease-out); }
	.top { top: 0; justify-content: space-between; background: linear-gradient(to bottom, var(--on-art-scrim-mid), var(--on-art-scrim-none)); }
	.bar { bottom: 0; padding-bottom: max(20px, env(safe-area-inset-bottom)); background: linear-gradient(to top, var(--on-art-scrim), var(--on-art-scrim-none)); }
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
	@media (prefers-reduced-motion: reduce) { .top, .bar { transition: opacity 200ms linear; } .idle .top, .idle .bar { transform: none; } }
</style>
