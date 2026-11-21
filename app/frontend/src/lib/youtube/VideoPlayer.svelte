<script lang="ts">
	// Plays one video file with the browser's own controls and fullscreen, starting at a given second.
	// Tells the page where playing stands: on every time update, and as final on pause, end and leaving the page.
	import { beforeNavigate } from '$app/navigation';

	let {
		src,
		poster,
		startAt = 0,
		label,
		onposition
	}: {
		src: string;
		poster?: string;
		startAt?: number;
		label?: string;
		onposition?: (seconds: number, duration: number, final: boolean) => void;
	} = $props();

	let el = $state<HTMLVideoElement>();
	// Nothing is reported until the start second is set, so leaving early never saves the start of the file.
	let ready = false;

	function report(final: boolean) {
		if (!el || !ready) return;
		onposition?.(el.currentTime, el.duration, final);
	}

	function loaded() {
		if (!el) return;
		if (startAt > 0 && Number.isFinite(el.duration) && startAt < el.duration) el.currentTime = startAt;
		ready = true;
	}

	beforeNavigate(() => report(true));

	$effect(() => {
		const hidden = () => document.visibilityState === 'hidden' && report(true);
		const gone = () => report(true);
		window.addEventListener('pagehide', gone);
		document.addEventListener('visibilitychange', hidden);
		return () => {
			window.removeEventListener('pagehide', gone);
			document.removeEventListener('visibilitychange', hidden);
			report(true);
		};
	});
</script>

<!-- svelte-ignore a11y_media_has_caption -->
<video
	bind:this={el}
	class="player"
	{src}
	{poster}
	aria-label={label}
	controls
	playsinline
	preload="metadata"
	onemptied={() => (ready = false)}
	onloadedmetadata={loaded}
	ontimeupdate={() => report(false)}
	onpause={() => report(true)}
	onended={() => report(true)}
	data-testid="video-player"
></video>

<style>
	.player { display: block; width: 100%; max-height: 80vh; aspect-ratio: 16 / 9; background: #000; border-radius: var(--r-card); }
</style>
