<script lang="ts">
	// The sign-in page's cinematic backdrop: one full-bleed picture at a
	// time, a slow Ken Burns move and a crossfade every 12 s. The card is
	// there on the first frame over plain dark; the first picture crossfades
	// in (about 300 ms) once it has decoded. Each picture is chosen for this
	// screen (size x DPR) so it is never blurry. The last list is kept on
	// this device so a repeat visit starts its picture at once (app.html
	// preloads it before any script).
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import { pickArt, playlist, prepareSlide, type ArtPick, type ArtShot } from '#lib/signinArt.ts';

	const CYCLE_MS = 12000; // Long enough to take each picture in; the zoom stays slow.
	const CACHE = 'holyarrs-signin-art';

	type Slide = { pick: ArtPick; shot: ArtShot; logo: string | null; key: number; first: boolean };
	let layers = $state<Slide[]>([]);
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

	function readCache(): ArtShot[] {
		try {
			const v = JSON.parse(localStorage.getItem(CACHE) ?? 'null');
			return Array.isArray(v?.shots) ? v.shots : [];
		} catch {
			return [];
		}
	}

	function writeCache(shots: ArtShot[], first: string) {
		try {
			localStorage.setItem(CACHE, JSON.stringify({ shots, first }));
		} catch {
			/* private window or full storage: the next visit fetches again */
		}
	}

	onMount(() => {
		let shots = readCache();
		let at = -1;
		let timer: ReturnType<typeof setTimeout> | undefined;
		let gone = false;
		const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
		still = motion.matches;

		// The next slide sharp enough for the screen as it is now; its picture
		// is loaded and decoded before it is shown.
		let busy = false;
		async function advance() {
			const list = playlist(shots, screen());
			if (!list.length || gone || busy) return;
			busy = true;
			for (let tries = 0; tries < list.length; tries++) {
				at = (at + 1) % list.length;
				// Backdrop and logo load together and switch as one unit.
				const got = await prepareSlide(list[at], ready);
				if (got) {
					if (gone) break;
					const first = !layers.length;
					layers = [...layers.slice(-1), { ...got, key: ++n, first }];
					// Next visit: preload the picture after this one, so the page turns over.
					const after = list[(at + 1) % list.length];
					writeCache(shots, after.pick.url);
					break;
				}
			}
			busy = false;
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

		// A repeat visit starts from the kept list straight away; a first visit
		// fetches the list now (not in idle time). A fresh list replaces the kept
		// one for the next slides.
		if (shots.length) {
			const list = playlist(shots, screen());
			const want = (() => {
				try {
					return JSON.parse(localStorage.getItem(CACHE) ?? 'null')?.first;
				} catch {
					return undefined;
				}
			})();
			const i = list.findIndex((e) => e.pick.url === want);
			at = i > 0 ? i - 1 : -1;
			void advance();
		}
		api
			.get<{ shots: ArtShot[] }>('/public/signin-art')
			.then((r) => {
				const fresh = Array.isArray(r?.shots) ? r.shots : [];
				if (!fresh.length || gone) return;
				const had = layers.length > 0 || busy;
				shots = fresh;
				if (!had) {
					at = -1;
					void advance();
				} else writeCache(shots, playlist(shots, screen())[0]?.pick.url ?? '');
			})
			.catch(() => {
				/* no art: the page stays plain dark */
			});

		document.addEventListener('visibilitychange', onVisible);
		motion.addEventListener('change', onMotion);
		window.addEventListener('resize', onResize);
		return () => {
			gone = true;
			clearTimeout(timer);
			clearTimeout(resizeT);
			document.removeEventListener('visibilitychange', onVisible);
			motion.removeEventListener('change', onMotion);
			window.removeEventListener('resize', onResize);
		};
	});
</script>

<div class="art" class:still aria-hidden="true" data-testid="signin-art">
	{#each layers as l, i (l.key)}
		<div class="layer" class:top={i === layers.length - 1} class:first={l.first}>
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
		</div>
	{/each}
	<div class="shade"></div>
	<!-- Logos sit above the scrim in their own stack, keyed to the same slide
	     as the picture: the old one fades out while the new one fades in. -->
	{#each layers as l, i (l.key)}
		{#if l.logo}
			<img class="logo" class:top={i === layers.length - 1} src={l.logo} alt="" decoding="async" />
		{/if}
	{/each}
</div>

<style>
	.art {
		position: fixed;
		inset: 0;
		z-index: 0;
		overflow: hidden;
		background: #050507;
	}
	.layer {
		position: absolute;
		inset: 0;
		z-index: 1;
		isolation: isolate;
		opacity: 0;
		animation: fade-in 1.6s ease-in-out forwards;
	}
	/* The first picture comes up from plain dark quickly; later ones crossfade slowly. */
	.layer.first { animation-duration: 300ms; animation-timing-function: ease-out; }
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
		left: max(clamp(24px, 3.2vw, 72px), env(safe-area-inset-left));
		bottom: max(clamp(24px, 4.5vh, 64px), env(safe-area-inset-bottom));
		z-index: 3;
		width: auto;
		height: auto;
		max-width: clamp(160px, 16vw, 420px);
		max-height: clamp(56px, 11vh, 160px);
		object-fit: contain;
		object-position: left bottom;
		filter: drop-shadow(0 2px 14px rgb(0 0 0 / 0.65));
		opacity: 0;
		animation: logo-out 1.6s ease-in-out forwards;
	}
	.logo.top { animation: logo-in 1.6s ease-in-out forwards; }
	/* The scrim: a soft vignette, a darker pool behind the centred card so it
	   reads on bright frames, and a floor for the logo. */
	.shade {
		position: absolute;
		inset: 0;
		z-index: 2;
		background:
			radial-gradient(ellipse 46% 62% at 50% 50%, rgb(5 5 7 / 0.55) 0%, rgb(5 5 7 / 0.3) 60%, rgb(5 5 7 / 0) 100%),
			radial-gradient(ellipse 120% 100% at 50% 45%, rgb(5 5 7 / 0) 45%, rgb(5 5 7 / 0.6) 100%),
			linear-gradient(0deg, rgb(5 5 7 / 0.7) 0%, rgb(5 5 7 / 0) 32%),
			linear-gradient(180deg, rgb(5 5 7 / 0.35) 0%, rgb(5 5 7 / 0) 22%);
	}
	@keyframes logo-in { from { opacity: 0; } to { opacity: 0.92; } }
	@keyframes logo-out { from { opacity: 0.92; } to { opacity: 0; } }
	.still .pic { animation: none; transform: none; }
	.still .layer { animation: none; opacity: 1; }
	.still .logo { animation: none; opacity: 0; }
	.still .logo.top { opacity: 0.92; }
	@keyframes ken-burns {
		from { transform: scale(1); }
		to { transform: scale(1.06); }
	}
	@keyframes fade-in {
		to { opacity: 1; }
	}
	@media (prefers-reduced-motion: reduce) {
		.pic, .layer { animation: none !important; transform: none !important; opacity: 1; }
		.logo { animation: none !important; opacity: 0; }
		.logo.top { opacity: 0.92; }
		.art { transition: none; }
	}
	/* Phones: the card is a bottom sheet, so the logo goes to the top. */
	@media (max-width: 600px) {
		.logo {
			top: max(20px, env(safe-area-inset-top));
			bottom: auto;
			left: max(20px, env(safe-area-inset-left));
			max-width: 44vw;
			max-height: 56px;
			object-position: left top;
		}
		.shade {
			background:
				linear-gradient(180deg, rgb(5 5 7 / 0.55) 0%, rgb(5 5 7 / 0) 22%),
				linear-gradient(0deg, rgb(5 5 7 / 0.9) 0%, rgb(5 5 7 / 0.55) 45%, rgb(5 5 7 / 0.1) 75%);
		}
	}
</style>
