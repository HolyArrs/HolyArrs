<script lang="ts">
	// The sign-in page's backdrop: one full-bleed picture at a time, a slow Ken Burns move and a soft mosaic dissolve every 20 s. The first picture never comes up from black. Frame 0 is a coarse mosaic of it (24 blocks across), painted by app.html from a thumbnail kept on this device or put in the page by the server. It then resolves in a radial wave from the sign-in panel outward: square blocks (36, 12 and 6 px, the Search sweep's pixel language) sharpen ring by ring over about 1.8 s on an ease-out, each 36 px cell a little early or late so the ring is ragged. One WebGL canvas draws it in a single pass per frame; where the wave is done the canvas is clear and the real picture shows. Each picture is chosen for this screen (size × DPR) so it is never blurry.
	import { onMount } from 'svelte';
	import { api } from '#lib/core/api.ts';
	import { to } from '#lib/core/paths.ts';
	import {
		REVEAL_HOLD_MS,
		THUMB_W,
		WAVE_DONE_MS,
		SIGNIN_GL_EVENT,
		CYCLE_MS,
		SWAP_DONE_MS,
		SWAP_MS,
		SWAP_ORIGINS,
		interacting,
		nextOrigin,
		swapEase,
		pickArt,
		playlist,
		prepareSlide,
		sized,
		type ArtPick,
		type ArtShot
	} from './signin-art.ts';
	import { VERT, FRAG, SWAP_FRAG } from './signin-shaders.ts';
	import { screen, ready, local, ownShots, readKept, writeCache, makeThumb, unseed, type Thumb } from './signin-kept.ts';

	/** The first slide's mosaic sources: the 24 px frame-0 thumbnail when kept, and a small copy for detail until the full picture is in. */
	type Seed = { t24: string | null; small: string };
	type Slide = { pick: ArtPick; shot: ArtShot; logo: string | null; key: number; first: boolean; seed: Seed | null; origin: number };
	let layers = $state<Slide[]>([]);
	let still = $state(false);
	let n = 0;

	let thumb: Thumb | null = null;
	/** Shows the full-size first picture once it is decoded (a crossfade over the lighter one). */
	function hires(img: HTMLImageElement) {
		let gone = false;
		img.decode().then(
			() => !gone && img.classList.add('in'),
			() => {}
		);
		return { destroy: () => void (gone = true) };
	}

	function loadImage(src: string): Promise<HTMLImageElement> {
		const img = new Image();
		img.crossOrigin = 'anonymous'; // WebGL only takes CORS-clean pictures (same-origin here).
		img.decoding = 'async';
		img.src = src;
		return img.decode().then(() => img);
	}

	/** Lets the panel stagger start; routes/login waits for this so it never runs into a GPU compile. */
	function glReady() {
		document.documentElement.dataset.signinGl = '1';
		window.dispatchEvent(new Event(SIGNIN_GL_EVENT));
	}

	function reveal(node: HTMLElement, seed: Seed | null) {
		const pic = node.querySelector<HTMLImageElement>('img.pic');
		const canvas = node.querySelector<HTMLCanvasElement>('canvas.mz');
		let stop = false;
		const gl = canvas?.getContext('webgl2', { alpha: true, premultipliedAlpha: true, antialias: false });
		const sharp = () => {
			stop = true;
			node.classList.remove('mosaic');
			node.classList.add('sharp');
			unseed();
		};
		if (!pic || !canvas || !gl || !seed || still) {
			glReady();
			// No WebGL or reduced motion: sharp as soon as the picture is decoded.
			if (pic) pic.decode().then(sharp, sharp);
			else sharp();
			return {};
		}
		const prog = gl.createProgram()!;
		for (const [type, src] of [[gl.VERTEX_SHADER, VERT], [gl.FRAGMENT_SHADER, FRAG]] as const) {
			const sh = gl.createShader(type)!;
			gl.shaderSource(sh, src);
			gl.compileShader(sh);
			gl.attachShader(prog, sh);
		}
		gl.linkProgram(prog);
		// Frames wait for the GPU process while it compiles: once three frames in a row come on time, the compile is behind us.
		{
			let last = 0;
			let calm = 0;
			const watch = (now: number) => {
				calm = last && now - last < 20 ? calm + 1 : 0;
				last = now;
				if (calm >= 3) glReady();
				else requestAnimationFrame(watch);
			};
			requestAnimationFrame(watch);
		}
		// The shader compiles on a driver thread where the browser allows it (KHR_parallel_shader_compile). Asking for the link status at once would block the main thread in the middle of app boot. Until it is linked, frame 0's CSS mosaic stays up.
		const par = gl.getExtension('KHR_parallel_shader_compile');
		let linked = false;
		function link(): boolean | null {
			if (linked) return true;
			if (par && !gl!.getProgramParameter(prog, par.COMPLETION_STATUS_KHR)) return null;
			if (!gl!.getProgramParameter(prog, gl!.LINK_STATUS)) return false;
			linked = true;
			setup();
			return true;
		}
		const locs = new Map<string, WebGLUniformLocation | null>();
		const u = (n: string) => {
			if (!locs.has(n)) locs.set(n, gl!.getUniformLocation(prog, n));
			return locs.get(n)!;
		};
		// Texture sizes wait here until the program is linked.
		let seedSize = [1, 1];
		let detSize = [1, 1];
		const pos = (canvas.dataset.pos ?? '50% 50%').split(/\s+/).map((v) => parseFloat(v) / 100);
		function setup() {
			gl!.useProgram(prog);
			gl!.bindBuffer(gl!.ARRAY_BUFFER, gl!.createBuffer());
			gl!.bufferData(gl!.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl!.STATIC_DRAW);
			const a = gl!.getAttribLocation(prog, 'a');
			gl!.enableVertexAttribArray(a);
			gl!.vertexAttribPointer(a, 2, gl!.FLOAT, false, 0, 0);
			gl!.uniform1i(u('seedTex'), 0);
			gl!.uniform1i(u('detTex'), 1);
			gl!.uniform2f(u('pos'), pos[0] ?? 0.5, pos[1] ?? 0.5);
			fit();
		}

		const size = { w: 0, h: 0, dpr: 1 };
		function fit() {
			if (!linked) return;
			// The Ken Burns box at scale 1 (offsetWidth ignores the transform).
			size.w = node.offsetWidth || innerWidth;
			size.h = node.offsetHeight || innerHeight;
			// The mosaic draws at the device pixel ratio (capped at 2 for speed), so its block edges are crisp.
			size.dpr = 1 / Math.min(2, Math.max(1, window.devicePixelRatio || 1));
			canvas!.width = Math.ceil(size.w / size.dpr);
			canvas!.height = Math.ceil(size.h / size.dpr);
			canvas!.style.width = canvas!.width * size.dpr + 'px';
			canvas!.style.height = canvas!.height * size.dpr + 'px';
			gl!.viewport(0, 0, canvas!.width, canvas!.height);
			gl!.uniform2f(u('view'), size.w, size.h);
			gl!.uniform1f(u('dpr'), size.dpr);
			centre();
		}
		function centre() {
			// The ring starts at the panel's centre and ends at the farthest corner.
			const r = document.querySelector('[data-signin-panel]')?.getBoundingClientRect();
			const box = node.getBoundingClientRect();
			const sx = size.w / (box.width || size.w);
			const cx = r ? (r.left + r.width / 2 - box.left) * sx : size.w / 2;
			const cy = r ? (r.top + r.height / 2 - box.top) * sx : size.h / 2;
			gl!.uniform2f(u('centre'), cx, cy);
			gl!.uniform1f(u('maxD'), Math.max(Math.hypot(cx, cy), Math.hypot(size.w - cx, cy), Math.hypot(cx, size.h - cy), Math.hypot(size.w - cx, size.h - cy)));
		}
		function texture(unit: number, src: TexImageSource, w: number, h: number, mips: boolean) {
			const tx = gl!.createTexture();
			gl!.activeTexture(gl!.TEXTURE0 + unit);
			gl!.bindTexture(gl!.TEXTURE_2D, tx);
			gl!.pixelStorei(gl!.UNPACK_PREMULTIPLY_ALPHA_WEBGL, false);
			gl!.texImage2D(gl!.TEXTURE_2D, 0, gl!.RGBA, gl!.RGBA, gl!.UNSIGNED_BYTE, src);
			gl!.texParameteri(gl!.TEXTURE_2D, gl!.TEXTURE_WRAP_S, gl!.CLAMP_TO_EDGE);
			gl!.texParameteri(gl!.TEXTURE_2D, gl!.TEXTURE_WRAP_T, gl!.CLAMP_TO_EDGE);
			gl!.texParameteri(gl!.TEXTURE_2D, gl!.TEXTURE_MAG_FILTER, gl!.NEAREST);
			if (mips) gl!.generateMipmap(gl!.TEXTURE_2D);
			gl!.texParameteri(gl!.TEXTURE_2D, gl!.TEXTURE_MIN_FILTER, mips ? gl!.LINEAR_MIPMAP_LINEAR : gl!.NEAREST);
			return { w, h };
		}

		let seedReady = false;
		let detReady = false;
		let full = false;
		// Frame 0's source: the kept 24 px thumbnail (drawn exactly as app.html painted it), else the small copy averaged down to the same grid.
		loadImage(seed.t24 ?? seed.small)
			.then((img) => {
				if (stop) return;
				const s = texture(0, img, img.naturalWidth, img.naturalHeight, !seed.t24);
				seedSize = [s.w, s.h];
				seedReady = true;
				if (!detReady) {
					const d = texture(1, img, img.naturalWidth, img.naturalHeight, !seed.t24);
					detSize = [d.w, d.h];
				}
			})
			.catch(() => {});
		// Detail until the full picture is in: the 64 px thumbnail or the small size.
		if (seed.t24)
			loadImage(seed.small)
				.then((img) => {
					if (stop || full) return;
					const d = texture(1, img, img.naturalWidth, img.naturalHeight, true);
					detSize = [d.w, d.h];
					detReady = true;
				})
				.catch(() => {});
		// The full picture, shrunk off the main thread so a 4K original never costs a frame. The finest block is 6 px, so 512 texels across is more than any screen's mosaic needs and uploads in well under a frame. A CORS-mode copy of the same address (the browser cache answers it); on any failure the plain <img> shows.
		loadImage(pic.currentSrc || pic.src)
			.then((img) => createImageBitmap(img, { resizeWidth: Math.min(512, img.naturalWidth), resizeQuality: 'high' }))
			.then((b) => {
				if (stop) return;
				const d = texture(1, b, b.width, b.height, true);
				detSize = [d.w, d.h];
				detReady = full = true;
			})
			.catch(() => {
				if (!stop) sharp();
			});

		let start = 0; // When the wave started (0: not yet).
		const t0 = performance.now();
		let raf = 0;
		function frame(now: number) {
			if (stop) return;
			// The first status query is a sync round trip to the GPU process, which waits out the compile. Asked only once the still hold is over, the compile has long finished and the answer is instant.
			const ok = seedReady && now - t0 >= REVEAL_HOLD_MS ? link() : null;
			if (ok === false) {
				pic!.decode().then(sharp, sharp);
				return;
			}
			if (ok) {
				// The wave runs once the full picture is in and the still hold is over.
				if (!start && full && now - t0 >= REVEAL_HOLD_MS) {
					start = now;
					centre(); // The panel has its final size now (the form has replaced the placeholder).
				}
				const t = start ? now - start : 0;
				gl!.uniform2f(u('seedSize'), seedSize[0], seedSize[1]);
				gl!.uniform2f(u('detSize'), detSize[0], detSize[1]);
				gl!.uniform1f(u('t'), t);
				gl!.drawArrays(gl!.TRIANGLES, 0, 3);
				if (!node.classList.contains('mosaic')) node.classList.add('mosaic');
				if (start && t >= WAVE_DONE_MS) {
					sharp();
					return;
				}
			}
			raf = requestAnimationFrame(frame);
		}
		raf = requestAnimationFrame(frame);
		const onResize = () => fit();
		window.addEventListener('resize', onResize);
		return {
			destroy() {
				stop = true;
				cancelAnimationFrame(raf);
				window.removeEventListener('resize', onResize);
				gl.getExtension('WEBGL_lose_context')?.loseContext();
			}
		};
	}

	function swap(node: HTMLElement, origin: number) {
		const pic = node.querySelector<HTMLImageElement>('img.pic');
		const canvas = node.querySelector<HTMLCanvasElement>('canvas.mz');
		let stop = false;
		// The picture sits at opacity 0 under the canvas, where the browser may skip decoding it. Decode it now and hand over (canvas off, picture on) only once it is ready, in one frame, so the old photo never shows through.
		const decoded = pic ? pic.decode().catch(() => {}) : Promise.resolve();
		let over = false;
		const done = () => {
			stop = true;
			if (over) return;
			over = true;
			void decoded.then(() => {
				requestAnimationFrame(() => {
					node.classList.remove('mosaic');
					node.classList.add('sharp');
				});
			});
		};
		const gl = canvas?.getContext('webgl2', { alpha: true, premultipliedAlpha: true, antialias: false });
		if (!pic || !canvas || !gl) {
			done();
			return {};
		}
		const prog = gl.createProgram()!;
		for (const [type, src] of [[gl.VERTEX_SHADER, VERT], [gl.FRAGMENT_SHADER, SWAP_FRAG]] as const) {
			const sh = gl.createShader(type)!;
			gl.shaderSource(sh, src);
			gl.compileShader(sh);
			gl.attachShader(prog, sh);
		}
		gl.linkProgram(prog);
		const par = gl.getExtension('KHR_parallel_shader_compile');
		const u = (n: string) => gl.getUniformLocation(prog, n);
		const w = node.offsetWidth || innerWidth;
		const h = node.offsetHeight || innerHeight;
		const px = Math.min(2, window.devicePixelRatio || 1);
		canvas.width = Math.ceil(w * px);
		canvas.height = Math.ceil(h * px);
		const [ox, oy] = SWAP_ORIGINS[origin] ?? [0.5, 0.5];
		const cx = ox * w;
		const cy = oy * h;
		const maxD = Math.max(Math.hypot(cx, cy), Math.hypot(w - cx, cy), Math.hypot(cx, h - cy), Math.hypot(w - cx, h - cy));
		const pos = (canvas.dataset.pos ?? '50% 50%').split(/\s+/).map((v) => parseFloat(v) / 100);
		let texSize: [number, number] | null = null;
		// The picture is already decoded (prepareSlide); a CORS-mode copy comes from the cache.
		loadImage(pic.currentSrc || pic.src)
			.then((img) => createImageBitmap(img, { resizeWidth: Math.min(Math.ceil(w * px), img.naturalWidth), resizeQuality: 'high' }))
			.then((b) => {
				if (stop) return;
				const tx = gl.createTexture();
				gl.activeTexture(gl.TEXTURE0);
				gl.bindTexture(gl.TEXTURE_2D, tx);
				gl.texImage2D(gl.TEXTURE_2D, 0, gl.RGBA, gl.RGBA, gl.UNSIGNED_BYTE, b);
				gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
				gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
				gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
				gl.generateMipmap(gl.TEXTURE_2D);
				gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR_MIPMAP_LINEAR);
				texSize = [b.width, b.height];
			})
			.catch(() => {
				if (!stop) done();
			});
		let ready = false;
		let start = 0;
		let raf = 0;
		function frame(now: number) {
			if (stop) return;
			if (!ready && texSize) {
				if (par && !gl!.getProgramParameter(prog, par.COMPLETION_STATUS_KHR)) {
					raf = requestAnimationFrame(frame);
					return;
				}
				if (!gl!.getProgramParameter(prog, gl!.LINK_STATUS)) return done();
				gl!.useProgram(prog);
				gl!.bindBuffer(gl!.ARRAY_BUFFER, gl!.createBuffer());
				gl!.bufferData(gl!.ARRAY_BUFFER, new Float32Array([-1, -1, 3, -1, -1, 3]), gl!.STATIC_DRAW);
				const a = gl!.getAttribLocation(prog, 'a');
				gl!.enableVertexAttribArray(a);
				gl!.vertexAttribPointer(a, 2, gl!.FLOAT, false, 0, 0);
				gl!.viewport(0, 0, canvas!.width, canvas!.height);
				gl!.uniform1i(u('tex'), 0);
				gl!.uniform2f(u('view'), w, h);
				gl!.uniform2f(u('texSize'), texSize[0], texSize[1]);
				gl!.uniform2f(u('pos'), pos[0] ?? 0.5, pos[1] ?? 0.5);
				gl!.uniform2f(u('origin'), cx, cy);
				gl!.uniform1f(u('maxD'), maxD);
				gl!.uniform1f(u('px'), px);
				ready = true;
				start = now;
				node.classList.add('mosaic');
			}
			if (ready) {
				const t = now - start;
				gl!.uniform1f(u('pr'), swapEase(t / SWAP_MS));
				gl!.drawArrays(gl!.TRIANGLES, 0, 3);
				if (t >= SWAP_DONE_MS) return done();
			}
			raf = requestAnimationFrame(frame);
		}
		raf = requestAnimationFrame(frame);
		// Never stuck: if the GPU never answers, the picture simply shows.
		const guard = setTimeout(() => !ready && done(), 1500);
		return {
			destroy() {
				stop = true;
				clearTimeout(guard);
				cancelAnimationFrame(raf);
				gl.getExtension('WEBGL_lose_context')?.loseContext();
			}
		};
	}

	function slide(node: HTMLElement, l: Slide) {
		return l.first ? reveal(node, l.seed) : l.origin >= 0 ? swap(node, l.origin) : reveal(node, null);
	}

	onMount(() => {
		const kept = readKept();
		// A first-ever visit: the list and thumbnails the server put in the page (app.html).
		const inline = (window as { __holyarrsSigninInline?: { shots: ArtShot[]; thumb: Thumb } }).__holyarrsSigninInline;
		if (!kept.thumb && inline?.thumb && Array.isArray(inline.shots) && local(inline.thumb.url)) {
			kept.shots = ownShots(inline.shots, false);
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

		// The next slide sharp enough for the screen as it is now; its picture is loaded and decoded before it is shown. The first one is the exception when its thumbnail is kept: its mosaic shows straight away and refines as the picture arrives.
		let busy = false;
		let origin = -1;
		async function advance() {
			const list = playlist(shots, screen());
			if (!list.length || gone || busy) return;
			busy = true;
			for (let tries = 0; tries < list.length; tries++) {
				at = (at + 1) % list.length;
				const first = !layers.length;
				// The first slide's mosaic shows at once: from the kept thumbnail, or on a first visit from the small size (a few KB, loads well before the full picture).
				const p = list[at].pick;
				const kept = thumb?.url === p.url ? thumb : null;
				const seed: Seed | null = !first ? null : { t24: kept?.t24 ?? null, small: kept?.t64 ?? sized(p.url, p.kind === 'poster' ? 342 : 300) };
				if (seed) {
					const key = ++n;
					layers = [{ ...list[at], logo: null, key, first, seed, origin: -1 }];
					// The lighter size first; the full one crossfades in once decoded.
					const got = await prepareSlide({ ...list[at], pick: { ...p, url: p.low ?? p.url } }, ready);
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
					// Never the same edge or corner twice in a row; no mosaic with reduced motion.
					origin = still ? -1 : nextOrigin(origin);
					layers = [...layers.slice(-1), { ...got, pick: { ...got.pick, low: undefined }, key: ++n, first, seed: null, origin }];
				}
				// Next visit: start from the picture after this one, so the page turns over.
				const after = list[(at + 1) % list.length];
				writeCache(shots, after.pick.url, thumb);
				// The next picture loads at full size well before its turn.
				if (list.length > 1) {
					setTimeout(() => {
						if (!gone) void ready(after.pick.url);
					}, CYCLE_MS / 2);
				}
				if (first && thumb?.url !== after.pick.url) {
					// Its thumbnail is made after the reveal, off the busy first seconds.
					setTimeout(() => {
						void makeThumb(after.pick).then((t) => {
							if (!t || gone) return;
							thumb = t;
							writeCache(shots, after.pick.url, thumb);
						});
					}, 3200);
				}
				break;
			}
			busy = false;
			if (list.length > 1) schedule();
		}
		function schedule() {
			clearTimeout(timer);
			timer = setTimeout(tick, CYCLE_MS);
		}
		// Never change the picture while the user is signing in. A field they focused or typed in (a password manager filling counts as input) holds the cycle; it resumes 5 s after the last interaction or blur. The name field's own auto-focus does not count.
		let lastInput = 0;
		let engaged = false;
		const isField = (el: Element | null) => !!el && el.matches('input, textarea, select, [contenteditable="true"]');
		function tick() {
			if (document.hidden) return; // Resumes on visibilitychange.
			const busyUser = interacting(performance.now(), lastInput, isField(document.activeElement), engaged);
			if (busyUser) {
				clearTimeout(timer);
				timer = setTimeout(tick, 1000);
				return;
			}
			void advance();
		}
		function onInteract() {
			engaged = true;
			lastInput = performance.now();
		}
		function onBlur() {
			lastInput = performance.now();
		}
		function onVisible() {
			if (!document.hidden && shots.length > 1) schedule();
		}
		function onMotion() {
			still = motion.matches;
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

		// A repeat visit starts from the kept list straight away; a first visit fetches the list now (not in idle time). A fresh list replaces the kept one for the next slides.
		if (shots.length) {
			const list = playlist(shots, screen());
			const i = list.findIndex((e) => e.pick.url === kept.first);
			at = i > 0 ? i - 1 : -1;
			void advance();
		} else unseed();
		api
			.get<{ shots: ArtShot[] }>('/public/signin-art')
			.then((r) => {
				const fresh = ownShots(r?.shots, true);
				if (!fresh.length || gone) return;
				const had = layers.length > 0 || busy;
				shots = fresh;
				if (!had) {
					at = -1;
					void advance();
				} else writeCache(shots, playlist(shots, screen())[0]?.pick.url ?? '', thumb);
			})
			.catch(() => {
				/* No art: the page stays plain dark. */
			});

		document.addEventListener('visibilitychange', onVisible);
		for (const e of ['keydown', 'input', 'pointerdown', 'paste'] as const) document.addEventListener(e, onInteract, true);
		document.addEventListener('focusout', onBlur, true);
		motion.addEventListener('change', onMotion);
		window.addEventListener('resize', onResize);
		return () => {
			gone = true;
			clearTimeout(timer);
			clearTimeout(resizeT);
			unseed();
			document.removeEventListener('visibilitychange', onVisible);
			for (const e of ['keydown', 'input', 'pointerdown', 'paste'] as const) document.removeEventListener(e, onInteract, true);
			document.removeEventListener('focusout', onBlur, true);
			motion.removeEventListener('change', onMotion);
			window.removeEventListener('resize', onResize);
		};
	});
</script>

<div class="art" class:still aria-hidden="true" data-testid="signin-art">
	{#each layers as l, i (l.key)}
		<div class="layer" class:top={i === layers.length - 1} class:first={l.first} class:swap={l.origin >= 0}>
			<div class="kb" style:--kb-dur="{CYCLE_MS + 1600}ms" use:slide={l}>
				<img
					class="pic"
					src={l.first && l.pick.low ? l.pick.low : l.pick.url}
					alt=""
					decoding="async"
					fetchpriority={l.first ? 'high' : 'auto'}
					data-kind={l.pick.kind}
					data-need={l.pick.need}
					style:object-position={l.pick.position}
				/>
				{#if l.first && l.pick.low}
					<img
						class="pic hi"
						src={l.pick.url}
						alt=""
						decoding="async"
						fetchpriority="high"
						data-hi
						style:object-position={l.pick.position}
						use:hires
					/>
				{/if}
				{#if l.first || l.origin >= 0}
					<canvas class="mz" data-pos={l.pick.position}></canvas>
				{/if}
			</div>
		</div>
	{/each}
	<div class="shade"></div>
	<!-- Logos sit above the scrim in their own stack, keyed to the same slide as the picture: the old one fades out while the new one fades in. -->
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
		/* Shown until (or when no) pictures arrive, such as a new install while the metadata server is busy: a warm glow instead of flat black. */
		background:
			radial-gradient(70% 55% at 50% 105%, rgba(217, 48, 37, 0.28), transparent 70%),
			radial-gradient(60% 50% at 15% 0%, rgba(120, 70, 40, 0.22), transparent 70%),
			radial-gradient(50% 45% at 90% 10%, rgba(60, 70, 110, 0.18), transparent 70%),
			#050507;
	}
	.layer {
		position: absolute;
		inset: 0;
		z-index: 1;
		isolation: isolate;
		opacity: 0;
		animation: fade-in 1.6s var(--ease-in-out) forwards;
	}
	/* The first picture is there at once, as its own mosaic (no fade from dark); later ones crossfade slowly. */
	.layer.first, .layer.swap { animation: none; opacity: 1; }
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
		object-fit: fill;
		image-rendering: pixelated;
		opacity: 0;
		pointer-events: none;
	}
	.kb:global(.mosaic) .mz { opacity: 1; }
	/* The slow zoom waits for the canvas: app.html's frame-0 mosaic is unscaled, so the hand-over lands on the same pixels (no 1 to 3 px jump at the edges). */
	.layer.first .kb:not(:global(.mosaic)):not(:global(.sharp)) { animation-play-state: paused; }
	/* Until the mosaic has its first frame, nothing sharp peeks through. */
	.kb:not(:global(.mosaic)):not(:global(.sharp)) .pic { opacity: 0; }
	/* The full-size picture over the lighter one: a crossfade once decoded, no jump. */
	.hi { opacity: 0; transition: opacity var(--dur-hero) var(--ease-in-out); }
	.kb .hi:global(.in) { opacity: 1; }
	.kb:global(.sharp) .mz { display: none; }
	/* A change: the old picture stays sharp underneath until the wave is done; the new one is only the canvas's blocks until then (no cut). */
	.layer.swap .kb:not(:global(.sharp)) .pic { opacity: 0; }
	/* A repeat visit: app.html painted the thumbnail behind the page, so the dark backgrounds stay clear until the mosaic takes over. */
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
		animation: logo-out 1.6s var(--ease-in-out) forwards;
	}
	.logo.top { animation: logo-in 1.6s var(--ease-in-out) forwards; }
	/* The scrim: a soft vignette, a darker pool behind the centred card so it reads on bright frames, and a floor for the logo. */
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
	.still .layer:not(.first) { animation: fade-in 2.4s var(--ease-in-out) forwards; }
	.still .layer:not(.top) { animation: none; opacity: 1; }
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
	/* Phones: the card floats low, so the logo goes to the top. */
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
				linear-gradient(0deg, rgb(5 5 7 / 0.55) 0%, rgb(5 5 7 / 0.2) 40%, rgb(5 5 7 / 0) 60%);
		}
	}
</style>
