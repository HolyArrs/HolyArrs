<script lang="ts">
	// The sign-in page's cinematic backdrop: one full-bleed picture at a
	// time, a slow Ken Burns move and a crossfade every 8 s. The card paints
	// first; the art fades in once its first picture has decoded. Each
	// picture is chosen for this screen (size x DPR) so it is never blurry.
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { pickArt, playlist, type ArtPick, type ArtShot } from '#lib/signinArt.ts';

	const CYCLE_MS = 8000;

	type Slide = { pick: ArtPick; shot: ArtShot; key: number };
	let layers = $state<Slide[]>([]);
	let shown = $state(false);
	let still = $state(false);
	let n = 0;

	function screen() {
		return { vw: window.innerWidth, vh: window.innerHeight, dpr: window.devicePixelRatio || 1 };
	}

	async function ready(url: string): Promise<boolean> {
		const img = new Image();
		img.decoding = 'async';
		img.src = url;
		try {
			await img.decode();
			return true;
		} catch {
			return false;
		}
	}

	onMount(() => {
		let shots: ArtShot[] = [];
		let at = -1;
		let timer: ReturnType<typeof setTimeout> | undefined;
		let gone = false;
		const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
		still = motion.matches;

		// The next slide sharp enough for the screen as it is now; its picture
		// is loaded and decoded before it is shown.
		async function advance() {
			const list = playlist(shots, screen());
			if (!list.length || gone) return;
			for (let tries = 0; tries < list.length; tries++) {
				at = (at + 1) % list.length;
				const { shot, pick } = list[at];
				if (await ready(pick.url)) {
					if (gone) return;
					const slide = { pick, shot, key: ++n };
					layers = [...layers.slice(-1), slide];
					shown = true;
					break;
				}
			}
			if (!still && list.length > 1) schedule();
		}
		function schedule() {
			clearTimeout(timer);
			timer = setTimeout(() => {
				if (document.hidden) return; // resumes on visibilitychange
				void advance();
			}, CYCLE_MS);
		}
		function onVisible() {
			if (!document.hidden && !still && shots.length > 1) schedule();
		}
		function onMotion() {
			still = motion.matches;
			if (still) clearTimeout(timer);
			else schedule();
		}
		let resizeT: ReturnType<typeof setTimeout> | undefined;
		function onResize() {
			// A bigger window (or another monitor) may need a sharper picture.
			clearTimeout(resizeT);
			resizeT = setTimeout(() => {
				const cur = layers.at(-1);
				if (!cur) return;
				const now = pickArt(cur.shot, screen());
				if (!now || now.url !== cur.pick.url) {
					at = Math.max(-1, at - 1);
					void advance();
				}
			}, 400);
		}

		// Start after the card has painted, in idle time.
		const start = () =>
			api
				.get<{ shots: ArtShot[] }>('/public/signin-art')
				.then((r) => {
					shots = Array.isArray(r?.shots) ? r.shots : [];
					return advance();
				})
				.catch(() => {
					/* no art: the page stays plain dark */
				});
		const idle = 'requestIdleCallback' in window ? window.requestIdleCallback(() => void start(), { timeout: 600 }) : setTimeout(start, 200);

		document.addEventListener('visibilitychange', onVisible);
		motion.addEventListener('change', onMotion);
		window.addEventListener('resize', onResize);
		return () => {
			gone = true;
			clearTimeout(timer);
			clearTimeout(resizeT);
			if ('cancelIdleCallback' in window && typeof idle === 'number') window.cancelIdleCallback(idle);
			document.removeEventListener('visibilitychange', onVisible);
			motion.removeEventListener('change', onMotion);
			window.removeEventListener('resize', onResize);
		};
	});
</script>

<div class="art" class:shown class:still aria-hidden="true" data-testid="signin-art">
	{#each layers as l, i (l.key)}
		<div class="layer" class:top={i === layers.length - 1}>
			<img
				class="pic"
				src={l.pick.url}
				alt=""
				decoding="async"
				data-kind={l.pick.kind}
				data-need={l.pick.need}
				style:object-position={l.pick.position}
				style:--kb-dur="{CYCLE_MS + 1600}ms"
			/>
			<img class="logo" src={l.shot.logo} alt="" decoding="async" />
		</div>
	{/each}
	<div class="shade"></div>
</div>

<style>
	.art {
		position: fixed;
		inset: 0;
		z-index: 0;
		overflow: hidden;
		background: #050507;
		opacity: 0;
		transition: opacity 1.2s ease;
	}
	.art.shown { opacity: 1; }
	.layer {
		position: absolute;
		inset: 0;
		opacity: 0;
		animation: fade-in 1.6s ease-in-out forwards;
	}
	.layer:not(.top) { animation: none; opacity: 1; }
	.pic {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
		transform-origin: 50% 35%;
		animation: ken-burns var(--kb-dur) linear forwards;
		will-change: transform;
	}
	.logo {
		position: absolute;
		left: max(32px, env(safe-area-inset-left));
		bottom: max(32px, env(safe-area-inset-bottom));
		z-index: 2;
		max-width: min(200px, 22vw);
		max-height: 72px;
		object-fit: contain;
		object-position: left bottom;
		opacity: 0.88;
		filter: drop-shadow(0 2px 12px rgb(0 0 0 / 0.6));
	}
	/* Heavy enough that the card always reads: a darker right side where the
	   card sits, and a floor for the logo. */
	.shade {
		position: absolute;
		inset: 0;
		z-index: 1;
		background:
			linear-gradient(90deg, rgb(5 5 7 / 0.25) 0%, rgb(5 5 7 / 0.35) 45%, rgb(5 5 7 / 0.82) 100%),
			linear-gradient(0deg, rgb(5 5 7 / 0.75) 0%, rgb(5 5 7 / 0) 35%),
			linear-gradient(180deg, rgb(5 5 7 / 0.45) 0%, rgb(5 5 7 / 0) 25%);
	}
	.still .pic { animation: none; transform: none; }
	.still .layer { animation: none; opacity: 1; }
	@keyframes ken-burns {
		from { transform: scale(1); }
		to { transform: scale(1.06); }
	}
	@keyframes fade-in {
		to { opacity: 1; }
	}
	@media (prefers-reduced-motion: reduce) {
		.pic, .layer { animation: none !important; transform: none !important; opacity: 1; }
		.art { transition: none; }
	}
	/* Phones: the card is a bottom sheet, so the logo goes to the top. */
	@media (max-width: 600px) {
		.logo {
			top: max(20px, env(safe-area-inset-top));
			bottom: auto;
			left: max(20px, env(safe-area-inset-left));
			max-width: 44vw;
			max-height: 48px;
			object-position: left top;
		}
		.shade {
			background:
				linear-gradient(180deg, rgb(5 5 7 / 0.55) 0%, rgb(5 5 7 / 0) 22%),
				linear-gradient(0deg, rgb(5 5 7 / 0.9) 0%, rgb(5 5 7 / 0.55) 45%, rgb(5 5 7 / 0.1) 75%);
		}
	}
</style>
