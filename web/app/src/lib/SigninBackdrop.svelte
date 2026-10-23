<script lang="ts">
	// The sign-in page's cinematic backdrop: one full-bleed picture at a
	// time, a slow Ken Burns move and a crossfade every 12 s.
	//
	// The first picture never comes up from black. It starts as a coarse
	// mosaic of itself and resolves to sharp over about 2.3 s on an ease-out:
	// two small canvases (24 to 272 blocks across) scaled up with
	// image-rendering: pixelated, the finer level crossfading over the coarser
	// one, and at the end the last level fading off the sharp picture. Per
	// frame only opacity changes; a canvas is redrawn (a few hundred pixels)
	// only when its level steps. A repeat visit seeds the mosaic from a 64 px
	// thumbnail kept on this device, and app.html paints that thumbnail before
	// any script, so the very first frame is the photo. Each picture is chosen
	// for this screen (size x DPR) so it is never blurry.
	import { onMount } from 'svelte';
	import { api } from '#lib/api.ts';
	import {
		MOSAIC_LEVELS,
		THUMB_W,
		pickArt,
		playlist,
		prepareSlide,
		revealAt,
		sized,
		type ArtPick,
		type ArtShot
	} from '#lib/signinArt.ts';

	const CYCLE_MS = 12000; // Long enough to take each picture in; the zoom stays slow.
	const CACHE = 'holyarrs-signin-art';
	const SEED = 'holyarrs-signin-seed'; // the style element app.html adds for the first frame

	type Slide = { pick: ArtPick; shot: ArtShot; logo: string | null; key: number; first: boolean; seed: string | null };
	type Thumb = { url: string; t24: string; t64: string; pos: string };
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

	function readKept(): { shots: ArtShot[]; first?: string; thumb: Thumb | null } {
		try {
			const v = JSON.parse(localStorage.getItem(CACHE) ?? 'null');
			const t = v?.thumb;
			const ok = t && typeof t.url === 'string' && /^data:image\/jpeg;base64,/.test(t.t64 ?? '');
			return { shots: Array.isArray(v?.shots) ? v.shots : [], first: v?.first, thumb: ok ? t : null };
		} catch {
			return { shots: [], thumb: null };
		}
	}

	let thumb: Thumb | null = null;
	function writeCache(shots: ArtShot[], first: string) {
		try {
			localStorage.setItem(CACHE, JSON.stringify({ shots, first, thumb: thumb?.url === first ? thumb : null }));
		} catch {
			/* private window or full storage: the next visit fetches again */
		}
	}

	// A tiny copy of the next visit's first picture: 24 and 64 px wide JPEGs
	// (about 2 KB together). TMDB answers CORS, so the canvas can be read back.
	async function makeThumb(pick: ArtPick): Promise<Thumb | null> {
		const img = new Image();
		img.crossOrigin = 'anonymous';
		img.src = sized(pick.url, pick.kind === 'poster' ? 342 : 300);
		try {
			await img.decode();
			const at = (w: number) => {
				const c = document.createElement('canvas');
				c.width = w;
				c.height = Math.max(1, Math.round((w * img.naturalHeight) / img.naturalWidth));
				const g = c.getContext('2d');
				if (!g) throw new Error('no 2d context');
				g.imageSmoothingQuality = 'high';
				g.drawImage(img, 0, 0, c.width, c.height);
				return c.toDataURL('image/jpeg', 0.72);
			};
			return { url: pick.url, t24: at(24), t64: at(THUMB_W), pos: pick.position };
		} catch {
			return null;
		}
	}

	function unseed() {
		document.getElementById(SEED)?.remove();
		document.documentElement.classList.remove('sa-seeded');
	}

	// The mosaic reveal on the first slide. node holds the sharp picture and
	// the two canvases. Levels finer than the thumbnail wait for the full
	// picture: the clock holds rather than stretching the thumbnail.
	function reveal(node: HTMLElement, seed: string | null) {
		const pic = node.querySelector<HTMLImageElement>('img.pic');
		const [lo, hi] = Array.from(node.querySelectorAll<HTMLCanvasElement>('canvas.mz'));
		if (!pic || !lo || !hi || still) {
			node.classList.add('sharp');
			if (still) pic?.decode().then(unseed, unseed);
			return {};
		}
		let src: CanvasImageSource | null = null;
		let srcW = 0;
		let srcH = 0;
		let full = false;
		let stop = false;
		let gen = 0; // bumps when the source gets better, so the canvases redraw
		const drawn = new WeakMap<HTMLCanvasElement, string>();
		function draw(c: HTMLCanvasElement, level: number) {
			if (!src) return;
			const tag = `${level}:${gen}`;
			if (drawn.get(c) === tag) return;
			drawn.set(c, tag);
			const w = Math.min(level, srcW);
			const h = Math.max(1, Math.round((w * srcH) / srcW));
			if (c.width !== w) c.width = w;
			if (c.height !== h) c.height = h;
			const g = c.getContext('2d');
			if (!g) return;
			g.imageSmoothingEnabled = true;
			g.imageSmoothingQuality = 'high';
			g.drawImage(src, 0, 0, w, h);
		}
		function use(img: CanvasImageSource, w: number, h: number, isFull: boolean) {
			if (stop || full || !w) return;
			src = img;
			srcW = w;
			srcH = h;
			full = isFull;
			gen++;
		}
		if (seed) {
			const t = new Image();
			t.src = seed;
			t.decode().then(() => use(t, t.naturalWidth, t.naturalHeight, false), () => {});
		}
		// Shrinking a 4K original inside a frame costs ~100 ms; the browser
		// makes a 2x copy of the finest level off the main thread instead.
		const top = MOSAIC_LEVELS[MOSAIC_LEVELS.length - 1] * 2;
		pic
			.decode()
			.then(() => createImageBitmap(pic, { resizeWidth: Math.min(top, pic.naturalWidth), resizeQuality: 'high' }))
			.then(
				(b) => use(b, b.width, b.height, true),
				() => use(pic, pic.naturalWidth, pic.naturalHeight, true)
			);

		let clock = 0;
		let last = 0;
		let raf = 0;
		function frame(now: number) {
			if (stop) return;
			const dt = last ? Math.min(now - last, 34) : 0;
			last = now;
			if (src) {
				// Hold when the next step needs more pixels than we have yet.
				const peek = revealAt(clock + dt);
				const needs = peek.hi === null ? Infinity : MOSAIC_LEVELS[peek.hi];
				if (full || needs <= srcW) clock += dt;
				const r = revealAt(clock);
				draw(lo, MOSAIC_LEVELS[r.lo]);
				if (r.hi !== null) draw(hi, MOSAIC_LEVELS[r.hi]);
				lo.style.opacity = r.hi === null ? String(1 - r.mix) : '1';
				hi.style.opacity = r.hi === null ? '0' : String(r.mix);
				if (!node.classList.contains('mosaic')) node.classList.add('mosaic');
				if (r.done) {
					node.classList.replace('mosaic', 'sharp');
					unseed();
					return;
				}
			}
			raf = requestAnimationFrame(frame);
		}
		raf = requestAnimationFrame(frame);
		return {
			destroy() {
				stop = true;
				cancelAnimationFrame(raf);
			}
		};
	}

	onMount(() => {
		const kept = readKept();
		// A first-ever visit: the list and thumbnails the server put in the page (app.html).
		const inline = (window as { __holyarrsSigninInline?: { shots: ArtShot[]; thumb: Thumb } }).__holyarrsSigninInline;
		if (!kept.thumb && inline?.thumb && Array.isArray(inline.shots)) {
			kept.shots = inline.shots;
			kept.thumb = inline.thumb;
			kept.first = inline.thumb.url;
		}
		let shots = kept.shots;
		thumb = kept.thumb;
		let at = -1;
		let timer: ReturnType<typeof setTimeout> | undefined;
		let gone = false;
		const motion = window.matchMedia('(prefers-reduced-motion: reduce)');
		still = motion.matches;

		// The next slide sharp enough for the screen as it is now; its picture
		// is loaded and decoded before it is shown. The first one is the
		// exception when its thumbnail is kept: its mosaic shows straight away
		// and refines as the picture arrives.
		let busy = false;
		async function advance() {
			const list = playlist(shots, screen());
			if (!list.length || gone || busy) return;
			busy = true;
			for (let tries = 0; tries < list.length; tries++) {
				at = (at + 1) % list.length;
				const first = !layers.length;
				// The first slide's mosaic shows at once: from the kept thumbnail,
				// or on a first visit from TMDB's small size (a few KB, loads well
				// before the full picture). Display only, so no CORS is needed.
				const p = list[at].pick;
				const seed = !first ? null : thumb?.url === p.url ? thumb.t64 : sized(p.url, p.kind === 'poster' ? 342 : 300);
				if (seed) {
					const key = ++n;
					layers = [{ ...list[at], logo: null, key, first, seed }];
					const got = await prepareSlide(list[at], ready);
					if (gone) break;
					if (!got) {
						layers = [];
						continue;
					}
					if (got.logo) layers = layers.map((l) => (l.key === key ? { ...l, logo: got.logo } : l));
				} else {
					// Backdrop and logo load together and switch as one unit.
					const got = await prepareSlide(list[at], ready);
					if (!got) continue;
					if (gone) break;
					layers = [...layers.slice(-1), { ...got, key: ++n, first, seed: null }];
				}
				// Next visit: start from the picture after this one, so the page turns over.
				const after = list[(at + 1) % list.length];
				writeCache(shots, after.pick.url);
				if (first && thumb?.url !== after.pick.url) {
					// Its thumbnail is made after the reveal, off the busy first seconds.
					setTimeout(() => {
						void makeThumb(after.pick).then((t) => {
							if (!t || gone) return;
							thumb = t;
							writeCache(shots, after.pick.url);
						});
					}, 3200);
				}
				break;
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
			const i = list.findIndex((e) => e.pick.url === kept.first);
			at = i > 0 ? i - 1 : -1;
			void advance();
		} else unseed();
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
			unseed();
			document.removeEventListener('visibilitychange', onVisible);
			motion.removeEventListener('change', onMotion);
			window.removeEventListener('resize', onResize);
		};
	});
</script>

<div class="art" class:still aria-hidden="true" data-testid="signin-art">
	{#each layers as l, i (l.key)}
		<div class="layer" class:top={i === layers.length - 1} class:first={l.first}>
			<div class="kb" style:--kb-dur="{CYCLE_MS + 1600}ms" use:reveal={l.first ? l.seed : null}>
				<img
					class="pic"
					src={l.pick.url}
					alt=""
					decoding="async"
					data-kind={l.pick.kind}
					data-need={l.pick.need}
					style:object-position={l.pick.position}
				/>
				{#if l.first}
					<canvas class="mz" style:object-position={l.pick.position}></canvas>
					<canvas class="mz" style:object-position={l.pick.position}></canvas>
				{/if}
			</div>
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
	/* The first picture is there at once, as its own mosaic (no fade from
	   dark); later ones crossfade slowly. */
	.layer.first { animation: none; opacity: 1; }
	.layer:not(.top) { animation: none; opacity: 1; }
	/* The slow zoom moves the picture and its mosaic together. */
	.kb {
		position: absolute;
		inset: 0;
		transform-origin: 50% 35%;
		animation: ken-burns var(--kb-dur) linear forwards;
		will-change: transform;
	}
	.pic,
	.mz {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.mz {
		image-rendering: pixelated;
		opacity: 0;
		will-change: opacity;
		pointer-events: none;
	}
	/* Until the mosaic has its first frame, nothing sharp peeks through. */
	.kb:not(:global(.mosaic)):not(:global(.sharp)) .pic { opacity: 0; }
	.kb:global(.sharp) .mz { display: none; }
	/* A repeat visit: app.html painted the thumbnail behind the page, so the
	   dark backgrounds stay clear until the mosaic takes over. */
	:global(html.sa-seeded) .art { background: transparent; }
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
	.still .kb { animation: none; transform: none; }
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
		.kb, .layer { animation: none !important; transform: none !important; opacity: 1; }
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
