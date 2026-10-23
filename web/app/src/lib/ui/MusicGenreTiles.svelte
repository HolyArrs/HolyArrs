<script lang="ts">
	import Icon from '../Icon.svelte';
	import { to } from '../paths.ts';
	import { genreTintToken } from '../music/genre-tint.ts';

	// The Music genre tiles (Explore's grid and the Genres button's picker): a
	// 16:9 tile in the genre's own colour with its artist photos laid over it in
	// grey, crossfading one to the next. Each tile runs on its own random timer,
	// loads and decodes the next photo before it fades, and rests while off screen,
	// in a hidden tab, or with reduced motion. No photos: the colour's gradient.
	// The last tile, All genres, is a 2x2 collage of the first four genres' photos.
	type Genre = { key: string; name: string; href: string; art: string; images?: string[]; hue: number };
	let { genres, allHref, compact = false, onopen }: { genres: Genre[]; allHref: string; compact?: boolean; onopen?: () => void } = $props();

	const slugOf = (key: string) => key.replace(/^genre:/, '');
	const photos = (g: Genre) => (g.images?.length ? g.images : g.art ? [g.art] : []).map((u) => to(u));
	const collage = $derived(genres.slice(0, 4).map((g) => ({ key: g.key, src: photos(g)[0] ?? '', tone: genreTintToken(slugOf(g.key), g.hue) })));
	let failedAll = $state<Record<string, boolean>>({});

	// One tile's slideshow, as an action on its element.
	function show(node: HTMLElement, srcs: string[]) {
		const imgs = Array.from(node.querySelectorAll<HTMLImageElement>('img.ph'));
		if (imgs.length < 2) return;
		const ready = imgs.map((_, i) => i === 0);
		const dead = imgs.map(() => false);
		let cur = 0;
		let timer: ReturnType<typeof setTimeout> | undefined;
		let seen = false;
		const reduce = matchMedia('(prefers-reduced-motion: reduce)');
		const awake = () => seen && !document.hidden && !reduce.matches;
		const load = (i: number) => new Promise<boolean>((resolve) => {
			if (ready[i]) return resolve(true);
			if (dead[i]) return resolve(false);
			const pre = new Image();
			pre.src = srcs[i];
			pre.decode().then(() => { imgs[i].src = srcs[i]; ready[i] = true; resolve(true); }, () => { dead[i] = true; resolve(false); });
		});
		const swap = (to: number) => {
			imgs[cur].classList.remove('on');
			imgs[to].classList.add('on');
			cur = to;
		};
		const wait = (first: boolean) => {
			clearTimeout(timer);
			if (!awake()) return;
			// 6 to 8 seconds; the first wait gets a random head start so tiles never change together.
			const ms = 6000 + Math.random() * 2000 + (first ? Math.random() * 6000 : 0);
			timer = setTimeout(async () => {
				for (let step = 1; step < imgs.length; step++) {
					const next = (cur + step) % imgs.length;
					if (await load(next)) { if (awake()) swap(next); break; }
				}
				wait(false);
			}, ms);
		};
		const io = new IntersectionObserver((es) => { seen = es.some((e) => e.isIntersecting); if (seen) wait(true); else clearTimeout(timer); });
		io.observe(node);
		const refresh = () => (awake() ? wait(true) : clearTimeout(timer));
		document.addEventListener('visibilitychange', refresh);
		reduce.addEventListener('change', refresh);
		return { destroy() { clearTimeout(timer); io.disconnect(); document.removeEventListener('visibilitychange', refresh); reduce.removeEventListener('change', refresh); } };
	}
</script>

<div class="mgt" class:compact data-testid="music-genre-tiles">
	{#each genres as g (g.key)}
		{@const srcs = photos(g)}
		<a class="tile" href={g.href} style:--g="var({genreTintToken(slugOf(g.key), g.hue)})" onclick={() => onopen?.()}>
			<span class="show" use:show={srcs}>
				{#each srcs as s, i (s)}
					<!-- Only the first photo is requested now; the others load just before their fade. -->
					<img class="ph" class:on={i === 0} src={i === 0 ? s : undefined} alt="" decoding="async" draggable="false" onerror={(e) => ((e.currentTarget as HTMLImageElement).hidden = true)} />
				{/each}
			</span>
			<strong>{g.name}</strong>
		</a>
	{/each}
	<a class="tile all" href={allHref} onclick={() => onopen?.()}>
		<span class="quad" aria-hidden="true">
			{#each collage as c (c.key)}
				<span class="q" style:--g="var({c.tone})">{#if c.src && !failedAll[c.key]}<img src={c.src} alt="" loading="lazy" decoding="async" draggable="false" onerror={() => (failedAll[c.key] = true)} />{/if}</span>
			{/each}
		</span>
		<span class="txt"><small>Browse</small><strong>All genres<Icon name="chevronRight" size={compact ? 14 : 18} /></strong></span>
	</a>
</div>

<style>
	/* Our own palette: deep hues around the HolyArrs red, one fixed colour per genre. Light mode steps a little brighter. */
	:global(:root) {
		--mg-crimson: #a31d2b; --mg-amber: #9a4a14; --mg-raspberry: #b0245a; --mg-plum: #6a1f5c; --mg-teal: #11585f; --mg-vermilion: #b3341d; --mg-wine: #6e1630;
		--mg-midnight: #2a2f6b; --mg-magenta: #8f1f7a; --mg-forest: #1f5a3a; --mg-indigo: #4b2370; --mg-blue: #1f4f7a; --mg-charcoal: #3b3438; --mg-rust: #7c3a1d;
	}
	:global(:root[data-theme='light']) {
		--mg-crimson: #c0283a; --mg-amber: #b05a1a; --mg-raspberry: #c92c6c; --mg-plum: #82297a; --mg-teal: #17707a; --mg-vermilion: #cc4024; --mg-wine: #8a1f3d;
		--mg-midnight: #3a4190; --mg-magenta: #a82a92; --mg-forest: #26734a; --mg-indigo: #6030a0; --mg-blue: #2a67a0; --mg-charcoal: #4f484c; --mg-rust: #944a24;
	}
	@media (prefers-color-scheme: light) {
		:global(:root:not([data-theme='dark'])) {
			--mg-crimson: #c0283a; --mg-amber: #b05a1a; --mg-raspberry: #c92c6c; --mg-plum: #82297a; --mg-teal: #17707a; --mg-vermilion: #cc4024; --mg-wine: #8a1f3d;
			--mg-midnight: #3a4190; --mg-magenta: #a82a92; --mg-forest: #26734a; --mg-indigo: #6030a0; --mg-blue: #2a67a0; --mg-charcoal: #4f484c; --mg-rust: #944a24;
		}
	}
	/* 5, 4, 3, then 2 columns by the room there is; tiles are 16:9 and at most 250 wide. */
	.mgt { container-type: inline-size; display: grid; gap: 14px; grid-template-columns: repeat(2, minmax(0, 250px)); justify-content: start; padding-inline: var(--page-gutter, 0); }
	@container (min-width: 560px) { .mgt:not(.compact) { grid-template-columns: repeat(3, minmax(0, 250px)); } }
	@container (min-width: 820px) { .mgt:not(.compact) { grid-template-columns: repeat(4, minmax(0, 250px)); } }
	@container (min-width: 1100px) { .mgt:not(.compact) { grid-template-columns: repeat(5, minmax(0, 250px)); } }
	.mgt.compact { gap: 10px; grid-template-columns: repeat(2, minmax(0, 1fr)); padding-inline: 0; }
	.tile {
		position: relative; isolation: isolate; display: block; aspect-ratio: 16 / 9; overflow: hidden; border-radius: 12px; color: #fff;
		background: linear-gradient(135deg, var(--g, var(--mg-charcoal)), color-mix(in srgb, var(--g, var(--mg-charcoal)) 55%, #000));
		transition: transform .35s cubic-bezier(.16, 1, .3, 1), box-shadow .35s;
	}
	.show { position: absolute; inset: 0; z-index: -2; }
	/* The server's photos are already grey; they sit over the colour at .55 and trade places by opacity. */
	.ph { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: 0; transition: opacity 1.2s ease; }
	.ph.on { opacity: .55; }
	/* A scrim from the bottom keeps the label at AA contrast over any photo. */
	.tile::after { content: ''; position: absolute; inset: 0; z-index: -1; background: linear-gradient(0deg, rgba(0, 0, 0, .62), rgba(0, 0, 0, .12) 55%, transparent); }
	strong { position: absolute; left: 12px; right: 10px; bottom: 9px; font: 700 clamp(14px, 1.25vw, 17px) / 1.15 var(--font-ui); letter-spacing: .005em; overflow-wrap: anywhere; display: flex; align-items: center; gap: 2px; }
	.compact strong { left: 9px; bottom: 7px; font-size: 13px; }
	.tile:is(:hover, :focus-visible) { transform: translateY(-2px); box-shadow: 0 10px 22px -12px rgba(0, 0, 0, .6); }
	.tile:active { transform: scale(.98); }
	.tile:focus-visible { outline: none; box-shadow: 0 0 0 2px var(--bg, #0b0708), 0 0 0 4px #d0202a; }
	.all { background: #120a0b; }
	.all::after { background: linear-gradient(0deg, rgba(0, 0, 0, .78), rgba(0, 0, 0, .45)); }
	.quad { position: absolute; inset: 0; z-index: -2; display: grid; grid-template-columns: 1fr 1fr; grid-template-rows: 1fr 1fr; gap: 2px; background: #120a0b; }
	.q { position: relative; overflow: hidden; background: linear-gradient(135deg, var(--g), color-mix(in srgb, var(--g) 55%, #000)); }
	.q img { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; opacity: .55; }
	.txt { position: absolute; left: 12px; right: 10px; bottom: 9px; display: flex; flex-direction: column; gap: 4px; }
	.txt strong { position: static; }
	small { font: 600 10px/1 var(--font-ui); letter-spacing: .14em; text-transform: uppercase; color: #ffc9cb; }
	@media (prefers-reduced-motion: reduce) {
		.tile, .ph { transition: none; }
		.tile:is(:hover, :focus-visible), .tile:active { transform: none; }
	}
</style>
